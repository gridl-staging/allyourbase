package logging

import (
	"time"
)

// LogEntry is a unified log entry that drains receive.
type LogEntry struct {
	Timestamp time.Time      `json:"timestamp"`
	Level     string         `json:"level"` // debug, info, warn, error
	Message   string         `json:"message"`
	Source    string         `json:"source"` // "app", "request", "edge_function"
	Fields    map[string]any `json:"fields,omitempty"`
}

// DrainStats holds delivery statistics for a drain.
type DrainStats struct {
	Sent    int64 `json:"sent"`
	Failed  int64 `json:"failed"` // failed delivery attempts; see Dropped for terminal loss
	Dropped int64 `json:"dropped"`
}

// DrainConfig configures a single log drain.
type DrainConfig struct {
	ID                string            `json:"id" toml:"id"`
	Type              string            `json:"type" toml:"type"` // "http", "datadog", "loki"
	URL               string            `json:"url" toml:"url"`
	Headers           map[string]string `json:"headers" toml:"headers"`
	BatchSize         int               `json:"batch_size" toml:"batch_size"`
	FlushIntervalSecs int               `json:"flush_interval_seconds" toml:"flush_interval_seconds"`
	Enabled           bool              `json:"enabled" toml:"enabled"`
}

// LogDrain sends batched log entries to an external destination.
type LogDrain interface {
	// Send delivers a batch of entries. Returns an error on failure.
	Send(entries []LogEntry) error
	// Name returns a human-readable identifier for this drain.
	Name() string
	// Stats returns delivery statistics.
	Stats() DrainStats
}

// DropReporter is optionally implemented by drains that track dropped entries.
type DropReporter interface {
	ReportDropped(n int64)
}
