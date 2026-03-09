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

// LogBuffer is a ring-buffer slog.Handler that captures recent log entries
// while forwarding them to a wrapped handler.
type LogBuffer struct {
	inner   slog.Handler
	mu      sync.Mutex
	entries []LogEntry
	maxSize int
	pos     int
	full    bool
}

// NewLogBuffer creates a LogBuffer wrapping the given handler, retaining up to maxSize entries.
func NewLogBuffer(inner slog.Handler, maxSize int) *LogBuffer {
	return &LogBuffer{
		inner:   inner,
		entries: make([]LogEntry, maxSize),
		maxSize: maxSize,
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

	lb.mu.Lock()
	lb.entries[lb.pos] = entry
	lb.pos++
	if lb.pos >= lb.maxSize {
		lb.pos = 0
		lb.full = true
	}
	lb.mu.Unlock()

	return lb.inner.Handle(ctx, r)
}

// WithAttrs delegates to the inner handler.
func (lb *LogBuffer) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &LogBuffer{
		inner:   lb.inner.WithAttrs(attrs),
		entries: lb.entries,
		maxSize: lb.maxSize,
		pos:     lb.pos,
		full:    lb.full,
	}
}

// WithGroup delegates to the inner handler.
func (lb *LogBuffer) WithGroup(name string) slog.Handler {
	return &LogBuffer{
		inner:   lb.inner.WithGroup(name),
		entries: lb.entries,
		maxSize: lb.maxSize,
		pos:     lb.pos,
		full:    lb.full,
	}
}

// Entries returns the buffered log entries in chronological order.
func (lb *LogBuffer) Entries() []LogEntry {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	if !lb.full {
		result := make([]LogEntry, lb.pos)
		copy(result, lb.entries[:lb.pos])
		return result
	}

	// Ring buffer is full: entries from pos..end, then 0..pos.
	result := make([]LogEntry, lb.maxSize)
	copy(result, lb.entries[lb.pos:])
	copy(result[lb.maxSize-lb.pos:], lb.entries[:lb.pos])
	return result
}
