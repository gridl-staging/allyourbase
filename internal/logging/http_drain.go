// Package logging provides HTTPDrain, a log drain that ships log entries via HTTP POST to a configurable endpoint with custom headers and statistics tracking.
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

// HTTPDrain sends log entries as a JSON array via POST to a configurable URL.
type HTTPDrain struct {
	cfg     DrainConfig
	client  *http.Client
	sent    atomic.Int64
	failed  atomic.Int64
	dropped atomic.Int64
}

// NewHTTPDrain constructs an HTTPDrain from the given config.
func NewHTTPDrain(cfg DrainConfig) *HTTPDrain {
	return &HTTPDrain{
		cfg: cfg,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Send marshals entries to JSON and POSTs them to the configured URL with custom headers, updating the sent counter on success and the failed counter on any error.
func (d *HTTPDrain) Send(entries []LogEntry) error {
	body, err := json.Marshal(entries)
	if err != nil {
		d.failed.Add(int64(len(entries)))
		return fmt.Errorf("http drain marshal: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, d.cfg.URL, bytes.NewReader(body))
	if err != nil {
		d.failed.Add(int64(len(entries)))
		return fmt.Errorf("http drain request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range d.cfg.Headers {
		req.Header.Set(k, v)
	}

	resp, err := d.client.Do(req)
	if err != nil {
		d.failed.Add(int64(len(entries)))
		return fmt.Errorf("http drain send: %w", err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	if resp.StatusCode >= 400 {
		d.failed.Add(int64(len(entries)))
		return fmt.Errorf("http drain: status %d", resp.StatusCode)
	}

	d.sent.Add(int64(len(entries)))
	return nil
}

func (d *HTTPDrain) Name() string { return d.cfg.ID }

// SetHTTPTransport overrides the HTTP transport used for outbound requests.
func (d *HTTPDrain) SetHTTPTransport(rt http.RoundTripper) {
	if d.client == nil || rt == nil {
		return
	}
	d.client.Transport = rt
}

func (d *HTTPDrain) ReportDropped(n int64) {
	d.dropped.Add(n)
}

func (d *HTTPDrain) Stats() DrainStats {
	return DrainStats{
		Sent:    d.sent.Load(),
		Failed:  d.failed.Load(),
		Dropped: d.dropped.Load(),
	}
}
