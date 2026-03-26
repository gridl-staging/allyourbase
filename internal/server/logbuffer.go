package server

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// LogEntry represents a single captured log line.
type LogEntry struct {
	Time    time.Time      `json:"time"`
	Level   string         `json:"level"`
	Message string         `json:"message"`
	Attrs   map[string]any `json:"attrs,omitempty"`
}

type logBufferState struct {
	mu      sync.Mutex
	entries []LogEntry
	pos     int
	full    bool
}

// LogBuffer is a ring-buffer slog.Handler that captures recent log entries
// while forwarding them to a wrapped handler.
type LogBuffer struct {
	inner slog.Handler
	state *logBufferState
}

// NewLogBuffer creates a LogBuffer wrapping the given handler, retaining up to maxSize entries.
func NewLogBuffer(inner slog.Handler, maxSize int) *LogBuffer {
	return &LogBuffer{
		inner: inner,
		state: &logBufferState{
			entries: make([]LogEntry, maxSize),
		},
	}
}

// Enabled delegates to the inner handler.
func (lb *LogBuffer) Enabled(ctx context.Context, level slog.Level) bool {
	return lb.inner.Enabled(ctx, level)
}

// Handle captures the log record into the ring buffer and forwards to the inner handler.
func (lb *LogBuffer) Handle(ctx context.Context, r slog.Record) error {
	entry := LogEntry{
		Time:    r.Time,
		Level:   r.Level.String(),
		Message: r.Message,
	}

	if r.NumAttrs() > 0 {
		entry.Attrs = make(map[string]any, r.NumAttrs())
		r.Attrs(func(a slog.Attr) bool {
			entry.Attrs[a.Key] = a.Value.Any()
			return true
		})
	}

	lb.state.mu.Lock()
	if len(lb.state.entries) > 0 {
		lb.state.entries[lb.state.pos] = entry
		lb.state.pos++
		if lb.state.pos >= len(lb.state.entries) {
			lb.state.pos = 0
			lb.state.full = true
		}
	}
	lb.state.mu.Unlock()

	return lb.inner.Handle(ctx, r)
}

// WithAttrs delegates to the inner handler.
func (lb *LogBuffer) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &LogBuffer{
		inner: lb.inner.WithAttrs(attrs),
		state: lb.state,
	}
}

// WithGroup delegates to the inner handler.
func (lb *LogBuffer) WithGroup(name string) slog.Handler {
	return &LogBuffer{
		inner: lb.inner.WithGroup(name),
		state: lb.state,
	}
}

// Entries returns the buffered log entries in chronological order.
func (lb *LogBuffer) Entries() []LogEntry {
	lb.state.mu.Lock()
	defer lb.state.mu.Unlock()

	if !lb.state.full {
		result := make([]LogEntry, lb.state.pos)
		copy(result, lb.state.entries[:lb.state.pos])
		return result
	}

	// Ring buffer is full: entries from pos..end, then 0..pos.
	size := len(lb.state.entries)
	result := make([]LogEntry, size)
	copy(result, lb.state.entries[lb.state.pos:])
	copy(result[size-lb.state.pos:], lb.state.entries[:lb.state.pos])
	return result
}
