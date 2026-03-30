package server

import (
	"context"
	"log/slog"
	"testing"
	"time"
)

// discardHandler is a slog.Handler that discards all records.
type discardHandler struct{}

func (discardHandler) Enabled(context.Context, slog.Level) bool  { return true }
func (discardHandler) Handle(context.Context, slog.Record) error { return nil }
func (d discardHandler) WithAttrs([]slog.Attr) slog.Handler      { return d }
func (d discardHandler) WithGroup(string) slog.Handler           { return d }

func TestLogBufferEmpty(t *testing.T) {
	lb := NewLogBuffer(discardHandler{}, 10)
	entries := lb.Entries()
	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}
}

func TestLogBufferCapture(t *testing.T) {
	lb := NewLogBuffer(discardHandler{}, 10)
	logger := slog.New(lb)

	logger.Info("hello")
	logger.Warn("world")

	entries := lb.Entries()
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].Message != "hello" {
		t.Errorf("expected first message 'hello', got %q", entries[0].Message)
	}
	if entries[0].Level != "INFO" {
		t.Errorf("expected level INFO, got %q", entries[0].Level)
	}
	if entries[1].Message != "world" {
		t.Errorf("expected second message 'world', got %q", entries[1].Message)
	}
	if entries[1].Level != "WARN" {
		t.Errorf("expected level WARN, got %q", entries[1].Level)
	}
}

func TestLogBufferWithAttrs(t *testing.T) {
	lb := NewLogBuffer(discardHandler{}, 10)
	logger := slog.New(lb)

	logger.Info("event", "key", "value", "count", 42)

	entries := lb.Entries()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Attrs["key"] != "value" {
		t.Errorf("expected attr key=value, got %v", entries[0].Attrs["key"])
	}
	if entries[0].Attrs["count"] != int64(42) {
		t.Errorf("expected attr count=42, got %v (%T)", entries[0].Attrs["count"], entries[0].Attrs["count"])
	}
}

func TestLogBufferRingOverflow(t *testing.T) {
	// Buffer of size 3.
	lb := NewLogBuffer(discardHandler{}, 3)
	logger := slog.New(lb)

	// Write 5 entries — only the last 3 should be retained.
	for i := 0; i < 5; i++ {
		logger.Info("msg", "i", i)
	}

	entries := lb.Entries()
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}

	// Entries should be in chronological order (oldest first).
	// After writing 5 entries to a size-3 buffer, entries 2,3,4 remain.
	for idx, entry := range entries {
		got, ok := entry.Attrs["i"].(int64)
		if !ok {
			t.Fatalf("entry %d: expected int64 attr, got %T", idx, entry.Attrs["i"])
		}
		want := int64(idx + 2)
		if got != want {
			t.Errorf("entry %d: expected i=%d, got i=%d", idx, want, got)
		}
	}
}

func TestLogBufferExactFill(t *testing.T) {
	lb := NewLogBuffer(discardHandler{}, 3)
	logger := slog.New(lb)

	// Write exactly 3 entries.
	logger.Info("a")
	logger.Info("b")
	logger.Info("c")

	entries := lb.Entries()
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}
	if entries[0].Message != "a" || entries[1].Message != "b" || entries[2].Message != "c" {
		t.Errorf("expected a,b,c got %q,%q,%q", entries[0].Message, entries[1].Message, entries[2].Message)
	}
}

func TestLogBufferTimestamp(t *testing.T) {
	lb := NewLogBuffer(discardHandler{}, 10)

	before := time.Now()
	slog.New(lb).Info("timestamped")
	after := time.Now()

	entries := lb.Entries()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Time.Before(before) || entries[0].Time.After(after) {
		t.Errorf("entry time %v not between %v and %v", entries[0].Time, before, after)
	}
}

func TestLogBufferWithGroup(t *testing.T) {
	lb := NewLogBuffer(discardHandler{}, 10)

	// WithGroup should return a LogBuffer that shares the same state.
	grouped := lb.WithGroup("grp")
	if _, ok := grouped.(*LogBuffer); !ok {
		t.Fatal("WithGroup should return a *LogBuffer")
	}
}

func TestLogBufferWithAttrsSharesState(t *testing.T) {
	lb := NewLogBuffer(discardHandler{}, 10)

	// WithAttrs should return a LogBuffer that shares the same state.
	child := lb.WithAttrs([]slog.Attr{slog.String("k", "v")})
	childLB, ok := child.(*LogBuffer)
	if !ok {
		t.Fatal("WithAttrs should return a *LogBuffer")
	}

	// Write through parent and child — both should appear in shared buffer.
	slog.New(lb).Info("from parent")
	slog.New(childLB).Info("from child")

	entries := lb.Entries()
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries in shared buffer, got %d", len(entries))
	}
}

func TestLogBufferZeroSize(t *testing.T) {
	// A zero-size buffer should not panic and should always return empty.
	lb := NewLogBuffer(discardHandler{}, 0)
	slog.New(lb).Info("ignored")
	entries := lb.Entries()
	if len(entries) != 0 {
		t.Errorf("zero-size buffer should return 0 entries, got %d", len(entries))
	}
}
