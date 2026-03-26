// Package logging DatadogDrain sends log entries to the Datadog Logs API and tracks delivery statistics.
package logging

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync/atomic"
	"time"
)

// datadogEntry is the JSON format expected by the Datadog Logs API.
type datadogEntry struct {
	DDSource  string         `json:"ddsource"`
	Service   string         `json:"service"`
	Hostname  string         `json:"hostname,omitempty"`
	Status    string         `json:"status"`
	Message   string         `json:"message"`
	Timestamp int64          `json:"timestamp"` // Unix milliseconds
	Fields    map[string]any `json:"attributes,omitempty"`
}

// DatadogDrain sends log entries to the Datadog Logs API in JSON array format.
type DatadogDrain struct {
	cfg     DrainConfig
	client  *http.Client
	sent    atomic.Int64
	failed  atomic.Int64
	dropped atomic.Int64
}

// NewDatadogDrain constructs a DatadogDrain from the given config.
func NewDatadogDrain(cfg DrainConfig) *DatadogDrain {
	return &DatadogDrain{
		cfg: cfg,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Send marshals log entries to JSON in Datadog Logs API format and posts them to the configured endpoint. It updates the sent counter on success and the failed counter on any error, returning an error if marshaling fails, the HTTP request fails, or the response status is 400 or higher.
func (d *DatadogDrain) Send(entries []LogEntry) error {
	ddEntries := make([]datadogEntry, len(entries))
	for i, e := range entries {
		ddEntries[i] = datadogEntry{
			DDSource:  "ayb",
			Service:   "ayb",
			Status:    e.Level,
			Message:   e.Message,
			Timestamp: e.Timestamp.UnixMilli(),
			Fields:    e.Fields,
		}
	}

	body, err := json.Marshal(ddEntries)
	if err != nil {
		d.failed.Add(int64(len(entries)))
		return fmt.Errorf("datadog drain marshal: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, d.cfg.URL, bytes.NewReader(body))
	if err != nil {
		d.failed.Add(int64(len(entries)))
		return fmt.Errorf("datadog drain request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range d.cfg.Headers {
		req.Header.Set(k, v)
	}

	resp, err := d.client.Do(req)
	if err != nil {
		d.failed.Add(int64(len(entries)))
		return fmt.Errorf("datadog drain send: %w", err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	if resp.StatusCode >= 400 {
		d.failed.Add(int64(len(entries)))
		return fmt.Errorf("datadog drain: status %d", resp.StatusCode)
	}

	d.sent.Add(int64(len(entries)))
	return nil
}

func (d *DatadogDrain) Name() string { return d.cfg.ID }

// SetHTTPTransport overrides the HTTP transport used for outbound requests.
func (d *DatadogDrain) SetHTTPTransport(rt http.RoundTripper) {
	if d.client == nil || rt == nil {
		return
	}
	d.client.Transport = rt
}

func (d *DatadogDrain) ReportDropped(n int64) {
	d.dropped.Add(n)
}

func (d *DatadogDrain) Stats() DrainStats {
	return DrainStats{
		Sent:    d.sent.Load(),
		Failed:  d.failed.Load(),
		Dropped: d.dropped.Load(),
	}
}
