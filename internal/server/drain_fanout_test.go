package server

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/logging"
	"github.com/allyourbase/ayb/internal/testutil"
)

// ---------------------------------------------------------------------------
// drainSlogHandler — verifies slog output fans out to drain manager
// ---------------------------------------------------------------------------

func TestDrainSlogHandlerForwardsToManager(t *testing.T) {
	t.Parallel()

	drain := &mockDrainCapture{}
	dm := logging.NewDrainManager()
	dm.AddDrain("slog-test", drain, logging.DrainWorkerConfig{
		BatchSize:     1,
		FlushInterval: 20 * time.Millisecond,
		QueueSize:     100,
	})
	dm.Start()
	defer dm.Stop()

	base := slog.New(slog.NewTextHandler(io.Discard, nil))
	logger := wrapLoggerForDrainFanout(base, dm)

	logger.Info("test message", "request_id", "r-001", "status", 200)

	// Wait for drain to receive the entry.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		drain.mu.Lock()
		n := drain.received
		drain.mu.Unlock()
		if n >= 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	drain.mu.Lock()
	defer drain.mu.Unlock()
	testutil.True(t, len(drain.batches) >= 1, "expected at least 1 batch from slog handler")

	var entry logging.LogEntry
	for _, b := range drain.batches {
		for _, e := range b {
			if e.Message == "test message" {
				entry = e
			}
		}
	}
	testutil.Equal(t, "test message", entry.Message)
	testutil.Equal(t, "app", entry.Source)
	testutil.Equal(t, "info", entry.Level)
	testutil.Equal(t, "r-001", entry.Fields["request_id"])
	testutil.True(t, entry.Fields["status"] == int64(200), "expected status=200 in fields")
}

func TestDrainSlogHandlerWithAttrsPreservesFields(t *testing.T) {
	t.Parallel()

	drain := &mockDrainCapture{}
	dm := logging.NewDrainManager()
	dm.AddDrain("attrs-test", drain, logging.DrainWorkerConfig{
		BatchSize:     1,
		FlushInterval: 20 * time.Millisecond,
		QueueSize:     100,
	})
	dm.Start()
	defer dm.Stop()

	base := slog.New(slog.NewTextHandler(io.Discard, nil))
	logger := wrapLoggerForDrainFanout(base, dm)

	// Create a sub-logger with pre-set attrs.
	subLogger := logger.With("component", "auth", "version", 2)
	subLogger.Warn("auth failed", "user_id", "u-123")

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		drain.mu.Lock()
		n := drain.received
		drain.mu.Unlock()
		if n >= 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	drain.mu.Lock()
	defer drain.mu.Unlock()
	testutil.True(t, len(drain.batches) >= 1, "expected drain to receive entry")

	var entry logging.LogEntry
	for _, b := range drain.batches {
		for _, e := range b {
			if e.Message == "auth failed" {
				entry = e
			}
		}
	}
	testutil.Equal(t, "auth failed", entry.Message)
	testutil.Equal(t, "warn", entry.Level)
	testutil.Equal(t, "auth", entry.Fields["component"])
	testutil.True(t, entry.Fields["version"] == int64(2), "expected version=2 in fields")
	testutil.Equal(t, "u-123", entry.Fields["user_id"])
}

func TestDrainSlogHandlerWithGroupPrefixesKeys(t *testing.T) {
	t.Parallel()

	drain := &mockDrainCapture{}
	dm := logging.NewDrainManager()
	dm.AddDrain("group-test", drain, logging.DrainWorkerConfig{
		BatchSize:     1,
		FlushInterval: 20 * time.Millisecond,
		QueueSize:     100,
	})
	dm.Start()
	defer dm.Stop()

	base := slog.New(slog.NewTextHandler(io.Discard, nil))
	logger := wrapLoggerForDrainFanout(base, dm)

	groupLogger := logger.WithGroup("db")
	groupLogger.Error("connection failed", "host", "localhost", "port", 5432)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		drain.mu.Lock()
		n := drain.received
		drain.mu.Unlock()
		if n >= 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	drain.mu.Lock()
	defer drain.mu.Unlock()
	testutil.True(t, len(drain.batches) >= 1, "expected drain to receive entry")

	var entry logging.LogEntry
	for _, b := range drain.batches {
		for _, e := range b {
			if e.Message == "connection failed" {
				entry = e
			}
		}
	}
	testutil.Equal(t, "error", entry.Level)
	testutil.Equal(t, "localhost", entry.Fields["db.host"])
	testutil.True(t, entry.Fields["db.port"] == int64(5432), "expected db.port=5432 in fields")
}

func TestWrapLoggerForDrainFanoutNilManagerReturnsBase(t *testing.T) {
	t.Parallel()

	base := slog.New(slog.NewTextHandler(io.Discard, nil))
	result := wrapLoggerForDrainFanout(base, nil)
	testutil.True(t, result == base, "nil manager should return base logger unchanged")
}

func TestWrapLoggerForDrainFanoutNilBaseReturnsNil(t *testing.T) {
	t.Parallel()

	dm := logging.NewDrainManager()
	result := wrapLoggerForDrainFanout(nil, dm)
	testutil.True(t, result == nil, "nil base should return nil")
}

// ---------------------------------------------------------------------------
// logEntryToDrain — verifies RequestLogEntry → LogEntry conversion
// ---------------------------------------------------------------------------

func TestLogEntryToDrainConvertsAllFields(t *testing.T) {
	t.Parallel()

	entry := RequestLogEntry{
		Method:       "POST",
		Path:         "/api/collections/users",
		StatusCode:   201,
		DurationMS:   42,
		RequestSize:  1024,
		ResponseSize: 512,
		UserID:       "user-abc",
		APIKeyID:     "key-xyz",
		RequestID:    "req-001",
		IPAddress:    "203.0.113.5",
	}

	result := logEntryToDrain(entry)

	testutil.Equal(t, "info", result.Level)
	testutil.Equal(t, "POST /api/collections/users", result.Message)
	testutil.Equal(t, "request", result.Source)

	testutil.Equal(t, "POST", result.Fields["method"])
	testutil.Equal(t, "/api/collections/users", result.Fields["path"])
	testutil.True(t, result.Fields["status"] == 201, "expected status=201")
	testutil.True(t, result.Fields["duration_ms"] == int64(42), "expected duration_ms=42")
	testutil.True(t, result.Fields["request_size"] == int64(1024), "expected request_size=1024")
	testutil.True(t, result.Fields["response_size"] == int64(512), "expected response_size=512")
	testutil.Equal(t, "user-abc", result.Fields["user_id"])
	testutil.Equal(t, "key-xyz", result.Fields["api_key_id"])
	testutil.Equal(t, "req-001", result.Fields["request_id"])
	testutil.Equal(t, "203.0.113.5", result.Fields["ip_address"])

	testutil.True(t, !result.Timestamp.IsZero(), "timestamp should be set")
}

func TestLogEntryToDrainOmitsEmptyOptionalFields(t *testing.T) {
	t.Parallel()

	entry := RequestLogEntry{
		Method:     "GET",
		Path:       "/health",
		StatusCode: 200,
		DurationMS: 1,
	}

	result := logEntryToDrain(entry)

	// Required fields present.
	testutil.Equal(t, "GET", result.Fields["method"])
	testutil.Equal(t, "/health", result.Fields["path"])
	testutil.Equal(t, 200, result.Fields["status"])

	// Optional fields should be absent, not empty strings.
	_, hasUser := result.Fields["user_id"]
	_, hasAPIKey := result.Fields["api_key_id"]
	_, hasReqID := result.Fields["request_id"]
	_, hasIP := result.Fields["ip_address"]
	testutil.True(t, !hasUser, "user_id should be omitted when empty")
	testutil.True(t, !hasAPIKey, "api_key_id should be omitted when empty")
	testutil.True(t, !hasReqID, "request_id should be omitted when empty")
	testutil.True(t, !hasIP, "ip_address should be omitted when empty")
}

// ---------------------------------------------------------------------------
// Edge function log fan-out to drain manager
// ---------------------------------------------------------------------------

func TestEdgeFuncLogWriterForwardsToDrainManager(t *testing.T) {
	t.Parallel()

	drain := &mockDrainCapture{}
	dm := logging.NewDrainManager()
	dm.AddDrain("ef-test", drain, logging.DrainWorkerConfig{
		BatchSize:     1,
		FlushInterval: 20 * time.Millisecond,
		QueueSize:     100,
	})
	dm.Start()
	defer dm.Stop()

	writer := &edgeFuncDrainWriter{manager: dm}
	writer.WriteLog(context.Background(), "my-function", "inv-001", "success", 150, "hello from edge func\nline2", "")

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		drain.mu.Lock()
		n := drain.received
		drain.mu.Unlock()
		if n >= 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	drain.mu.Lock()
	defer drain.mu.Unlock()
	testutil.True(t, len(drain.batches) >= 1, "expected drain to receive edge func log")

	var entry logging.LogEntry
	for _, b := range drain.batches {
		for _, e := range b {
			if e.Source == "edge_function" {
				entry = e
			}
		}
	}
	testutil.Equal(t, "edge_function", entry.Source)
	testutil.Equal(t, "info", entry.Level)
	testutil.Equal(t, "my-function", entry.Fields["function"])
	testutil.Equal(t, "inv-001", entry.Fields["invocation_id"])
	testutil.Equal(t, "success", entry.Fields["status"])
	testutil.Equal(t, 150, entry.Fields["duration_ms"])
	testutil.Equal(t, "hello from edge func\nline2", entry.Fields["stdout"])
}

func TestEdgeFuncDrainWriterErrorEntryHasErrorLevel(t *testing.T) {
	t.Parallel()

	drain := &mockDrainCapture{}
	dm := logging.NewDrainManager()
	dm.AddDrain("ef-err", drain, logging.DrainWorkerConfig{
		BatchSize:     1,
		FlushInterval: 20 * time.Millisecond,
		QueueSize:     100,
	})
	dm.Start()
	defer dm.Stop()

	writer := &edgeFuncDrainWriter{manager: dm}
	writer.WriteLog(context.Background(), "failing-func", "inv-002", "error", 500, "", "ReferenceError: x is not defined")

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		drain.mu.Lock()
		n := drain.received
		drain.mu.Unlock()
		if n >= 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	drain.mu.Lock()
	defer drain.mu.Unlock()

	var entry logging.LogEntry
	for _, b := range drain.batches {
		for _, e := range b {
			if e.Source == "edge_function" {
				entry = e
			}
		}
	}
	testutil.Equal(t, "error", entry.Level)
	testutil.Equal(t, "failing-func", entry.Fields["function"])
	testutil.Equal(t, "ReferenceError: x is not defined", entry.Fields["error"])
}
