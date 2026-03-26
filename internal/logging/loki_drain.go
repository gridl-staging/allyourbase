// Package logging LokiDrain sends log entries to Grafana Loki, grouping them by level and source into labeled streams for efficient storage.
package logging

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync/atomic"
	"time"
)

// lokiPush is the top-level Loki push API request body.
type lokiPush struct {
	Streams []lokiStream `json:"streams"`
}

// lokiStream groups log values under a set of stream labels.
type lokiStream struct {
	Stream map[string]string `json:"stream"`
	Values [][]string        `json:"values"` // each value is [unix_nano_string, line]
}

// LokiDrain sends log entries to Grafana Loki via /loki/api/v1/push.
type LokiDrain struct {
	cfg     DrainConfig
	client  *http.Client
	sent    atomic.Int64
	failed  atomic.Int64
	dropped atomic.Int64
}

// NewLokiDrain constructs a LokiDrain from the given config.
func NewLokiDrain(cfg DrainConfig) *LokiDrain {
	return &LokiDrain{
		cfg: cfg,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Send sends log entries to Loki grouped by level and source, updating the sent counter on success or the failed counter on error.
func (d *LokiDrain) Send(entries []LogEntry) error {
	// Group entries by level+source for efficient Loki label streams.
	streams := map[string]*lokiStream{}
	for _, e := range entries {
		key := e.Level + "|" + e.Source
		s, ok := streams[key]
		if !ok {
			s = &lokiStream{
				Stream: map[string]string{
					"source": "ayb",
					"level":  e.Level,
				},
			}
			if e.Source != "" {
				s.Stream["log_source"] = e.Source
			}
			streams[key] = s
		}
		ns := strconv.FormatInt(e.Timestamp.UnixNano(), 10)
		line, err := lokiLineForEntry(e)
		if err != nil {
			d.failed.Add(int64(len(entries)))
			return fmt.Errorf("loki drain marshal line: %w", err)
		}
		s.Values = append(s.Values, []string{ns, line})
	}

	push := lokiPush{Streams: make([]lokiStream, 0, len(streams))}
	for _, s := range streams {
		push.Streams = append(push.Streams, *s)
	}

	body, err := json.Marshal(push)
	if err != nil {
		d.failed.Add(int64(len(entries)))
		return fmt.Errorf("loki drain marshal: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, d.cfg.URL, bytes.NewReader(body))
	if err != nil {
		d.failed.Add(int64(len(entries)))
		return fmt.Errorf("loki drain request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range d.cfg.Headers {
		req.Header.Set(k, v)
	}

	resp, err := d.client.Do(req)
	if err != nil {
		d.failed.Add(int64(len(entries)))
		return fmt.Errorf("loki drain send: %w", err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	if resp.StatusCode >= 400 {
		d.failed.Add(int64(len(entries)))
		return fmt.Errorf("loki drain: status %d", resp.StatusCode)
	}

	d.sent.Add(int64(len(entries)))
	return nil
}

// lokiLineForEntry marshals a LogEntry to JSON for Loki, including message, level, source, and custom fields, with colliding field names prefixed as field_.
func lokiLineForEntry(e LogEntry) (string, error) {
	line := map[string]any{
		"message": e.Message,
	}
	if e.Level != "" {
		line["level"] = e.Level
	}
	if e.Source != "" {
		line["log_source"] = e.Source
	}
	for k, v := range e.Fields {
		if _, exists := line[k]; exists {
			line["field_"+k] = v
			continue
		}
		line[k] = v
	}

	b, err := json.Marshal(line)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (d *LokiDrain) Name() string { return d.cfg.ID }

// SetHTTPTransport overrides the HTTP transport used for outbound requests.
func (d *LokiDrain) SetHTTPTransport(rt http.RoundTripper) {
	if d.client == nil || rt == nil {
		return
	}
	d.client.Transport = rt
}

func (d *LokiDrain) ReportDropped(n int64) {
	d.dropped.Add(n)
}

func (d *LokiDrain) Stats() DrainStats {
	return DrainStats{
		Sent:    d.sent.Load(),
		Failed:  d.failed.Load(),
		Dropped: d.dropped.Load(),
	}
}
