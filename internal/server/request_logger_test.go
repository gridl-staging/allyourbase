package server

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/logging"
	"github.com/allyourbase/ayb/internal/observability"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/golang-jwt/jwt/v5"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// mockFlushFn is a test-only flush function that accumulates batches.
type mockFlushFn struct {
	mu      sync.Mutex
	batches [][]RequestLogEntry
	calls   int
	errOnce error // return this error on the next call, then clear
}

func (m *mockFlushFn) fn(ctx context.Context, entries []RequestLogEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.errOnce != nil {
		err := m.errOnce
		m.errOnce = nil
		return err
	}
	m.calls++
	cp := make([]RequestLogEntry, len(entries))
	copy(cp, entries)
	m.batches = append(m.batches, cp)
	return nil
}

func (m *mockFlushFn) allEntries() []RequestLogEntry {
	m.mu.Lock()
	defer m.mu.Unlock()
	var all []RequestLogEntry
	for _, b := range m.batches {
		all = append(all, b...)
	}
	return all
}

func (m *mockFlushFn) batchCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.batches)
}

type mockDrainCapture struct {
	mu       sync.Mutex
	batches  [][]logging.LogEntry
	received int
}

func (d *mockDrainCapture) Send(entries []logging.LogEntry) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	cp := make([]logging.LogEntry, len(entries))
	copy(cp, entries)
	d.batches = append(d.batches, cp)
	d.received += len(entries)
	return nil
}

func (d *mockDrainCapture) Name() string { return "mock" }

func (d *mockDrainCapture) Stats() logging.DrainStats {
	d.mu.Lock()
	defer d.mu.Unlock()
	return logging.DrainStats{Sent: int64(d.received)}
}

type captureSlogHandler struct {
	mu      sync.Mutex
	records []map[string]any
}

func (h *captureSlogHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *captureSlogHandler) Handle(_ context.Context, record slog.Record) error {
	fields := map[string]any{}
	record.Attrs(func(a slog.Attr) bool {
		fields[a.Key] = a.Value.Resolve().Any()
		return true
	})
	h.mu.Lock()
	h.records = append(h.records, fields)
	h.mu.Unlock()
	return nil
}

func (h *captureSlogHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }

func (h *captureSlogHandler) WithGroup(_ string) slog.Handler { return h }

func (h *captureSlogHandler) lastRecord() map[string]any {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.records) == 0 {
		return nil
	}
	return h.records[len(h.records)-1]
}

// newTestRequestLogger creates a RequestLogger with an injectable flush function.
func newTestRequestLogger(cfg RequestLogConfig, flush flushFn) *RequestLogger {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	rl := newRequestLoggerWithFlush(cfg, logger, flush)
	return rl
}

// TestRequestLoggerLogEnqueuesEntry verifies that Log() enqueues entries.
func TestRequestLoggerLogEnqueuesEntry(t *testing.T) {
	t.Parallel()

	mock := &mockFlushFn{}
	cfg := RequestLogConfig{
		Enabled:           true,
		BatchSize:         10,
		FlushIntervalSecs: 60, // long interval so only size-based flush triggers
		QueueSize:         100,
	}
	rl := newTestRequestLogger(cfg, mock.fn)
	ctx, cancel := context.WithCancel(context.Background())
	rl.Start(ctx)
	defer cancel()

	entry := RequestLogEntry{
		Method:     "GET",
		Path:       "/api/collections",
		StatusCode: 200,
		DurationMS: 5,
	}
	rl.Log(entry)

	// Force flush.
	testutil.NoError(t, rl.Shutdown(context.Background()))

	entries := mock.allEntries()
	testutil.Equal(t, 1, len(entries))
	testutil.Equal(t, "GET", entries[0].Method)
	testutil.Equal(t, "/api/collections", entries[0].Path)
	testutil.Equal(t, 200, entries[0].StatusCode)
}

// TestRequestLoggerBatchFlushesAtSizeThreshold verifies that batch flushes when it hits BatchSize.
func TestRequestLoggerBatchFlushesAtSizeThreshold(t *testing.T) {
	t.Parallel()

	mock := &mockFlushFn{}
	batchSize := 5
	cfg := RequestLogConfig{
		Enabled:           true,
		BatchSize:         batchSize,
		FlushIntervalSecs: 60,
		QueueSize:         1000,
	}
	rl := newTestRequestLogger(cfg, mock.fn)
	ctx, cancel := context.WithCancel(context.Background())
	rl.Start(ctx)
	defer cancel()

	// Enqueue exactly batchSize entries.
	for i := range batchSize {
		rl.Log(RequestLogEntry{Method: "GET", Path: "/health", StatusCode: 200, DurationMS: int64(i)})
	}

	// Wait for the batch flush to happen (worker is running, should flush quickly).
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if mock.batchCount() >= 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	testutil.NoError(t, rl.Shutdown(context.Background()))

	testutil.Equal(t, batchSize, len(mock.allEntries()))
}

// TestRequestLoggerFlushesAtTimeInterval verifies batch flushes when interval fires.
func TestRequestLoggerFlushesAtTimeInterval(t *testing.T) {
	t.Parallel()

	mock := &mockFlushFn{}
	cfg := RequestLogConfig{
		Enabled:           true,
		BatchSize:         1000, // very large so size threshold won't fire
		FlushIntervalSecs: 0,    // will be set to very short below via flushInterval field
		QueueSize:         100,
	}
	rl := newTestRequestLogger(cfg, mock.fn)
	rl.flushInterval = 50 * time.Millisecond // override for fast test
	ctx, cancel := context.WithCancel(context.Background())
	rl.Start(ctx)
	defer cancel()

	rl.Log(RequestLogEntry{Method: "POST", Path: "/api/auth/register", StatusCode: 201})

	// Wait for time-based flush.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if mock.batchCount() >= 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	testutil.NoError(t, rl.Shutdown(context.Background()))

	testutil.Equal(t, 1, len(mock.allEntries()))
	testutil.Equal(t, "POST", mock.allEntries()[0].Method)
}

// TestRequestLoggerNonBlockingWhenFull verifies Log() drops entries and doesn't block when channel full.
func TestRequestLoggerNonBlockingWhenFull(t *testing.T) {
	t.Parallel()

	blocked := make(chan struct{})
	var flushCalled atomic.Bool
	blockingFlush := func(ctx context.Context, entries []RequestLogEntry) error {
		if !flushCalled.Swap(true) {
			// Block the first flush to cause channel saturation.
			select {
			case <-blocked:
			case <-ctx.Done():
			}
		}
		return nil
	}

	cfg := RequestLogConfig{
		Enabled:           true,
		BatchSize:         1,
		FlushIntervalSecs: 60,
		QueueSize:         2, // very small channel
	}
	rl := newTestRequestLogger(cfg, blockingFlush)
	ctx, cancel := context.WithCancel(context.Background())
	rl.Start(ctx)
	defer cancel()

	start := time.Now()
	// Fill channel to capacity (channel size = 2, batch = 1) while the flush
	// path is blocked so additional entries must be dropped immediately.
	for range 20 {
		rl.Log(RequestLogEntry{Method: "GET", Path: "/flood", StatusCode: 200})
	}
	elapsed := time.Since(start)

	testutil.True(t, elapsed < 200*time.Millisecond, "Log() should remain non-blocking under queue saturation")
	drops := rl.DropCount()
	testutil.True(t, drops > 0, "expected dropped entries when queue is saturated")

	// Unblock flush.
	close(blocked)

	testutil.NoError(t, rl.Shutdown(context.Background()))
}

// TestRequestLoggerShutdownFlushesPending verifies that Shutdown drains remaining entries.
func TestRequestLoggerShutdownFlushesPending(t *testing.T) {
	t.Parallel()

	mock := &mockFlushFn{}
	cfg := RequestLogConfig{
		Enabled:           true,
		BatchSize:         1000, // large — won't flush until shutdown
		FlushIntervalSecs: 60,
		QueueSize:         100,
	}
	rl := newTestRequestLogger(cfg, mock.fn)
	ctx, cancel := context.WithCancel(context.Background())
	rl.Start(ctx)
	defer cancel()

	for range 3 {
		rl.Log(RequestLogEntry{Method: "DELETE", Path: "/api/records/1", StatusCode: 204})
	}

	// Shutdown should flush remaining entries before returning.
	testutil.NoError(t, rl.Shutdown(context.Background()))

	testutil.Equal(t, 3, len(mock.allEntries()))
}

// TestRequestLoggerShutdownFlushesAfterCancel verifies shutdown flushes entries even though the worker context is canceled.
func TestRequestLoggerShutdownFlushesAfterCancel(t *testing.T) {
	t.Parallel()

	type flushEvent struct {
		ctxErr error
		count  int
	}
	flushCh := make(chan flushEvent, 4)
	flush := func(ctx context.Context, entries []RequestLogEntry) error {
		flushCh <- flushEvent{ctxErr: ctx.Err(), count: len(entries)}
		return nil
	}

	cfg := RequestLogConfig{
		Enabled:           true,
		BatchSize:         1000, // won't flush until shutdown
		FlushIntervalSecs: 60,
		QueueSize:         10,
	}
	rl := newRequestLoggerWithFlush(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), flush)
	ctx, cancel := context.WithCancel(context.Background())
	rl.Start(ctx)
	defer cancel()

	rl.Log(RequestLogEntry{Method: "GET", Path: "/api/one", StatusCode: 200})

	testutil.NoError(t, rl.Shutdown(context.Background()))

	select {
	case got := <-flushCh:
		testutil.Equal(t, 1, got.count)
		testutil.NoError(t, got.ctxErr)
	case <-time.After(2 * time.Second):
		t.Fatal("shutdown did not flush queued entries")
	}
}

// TestRequestLoggerLogAfterShutdownDropsEntries verifies calls to Log after Shutdown are dropped.
func TestRequestLoggerLogAfterShutdownDropsEntries(t *testing.T) {
	t.Parallel()

	mock := &mockFlushFn{}
	cfg := RequestLogConfig{
		Enabled:           true,
		BatchSize:         1000, // large — won't flush until shutdown
		FlushIntervalSecs: 60,
		QueueSize:         10,
	}
	rl := newTestRequestLogger(cfg, mock.fn)
	ctx, cancel := context.WithCancel(context.Background())
	rl.Start(ctx)
	defer cancel()

	rl.Log(RequestLogEntry{Method: "GET", Path: "/api/before", StatusCode: 200})

	testutil.NoError(t, rl.Shutdown(context.Background()))
	rl.Log(RequestLogEntry{Method: "GET", Path: "/api/after", StatusCode: 200})

	testutil.Equal(t, int64(1), rl.DropCount())
	testutil.Equal(t, 1, len(mock.allEntries()))
	testutil.Equal(t, "/api/before", mock.allEntries()[0].Path)
}

// TestRequestLogMiddlewareCapturesFields verifies the middleware enqueues log entries with correct fields.
func TestRequestLogMiddlewareCapturesFields(t *testing.T) {
	t.Parallel()

	mock := &mockFlushFn{}
	cfg := RequestLogConfig{
		Enabled:           true,
		BatchSize:         100,
		FlushIntervalSecs: 60,
		QueueSize:         100,
	}
	rl := newTestRequestLogger(cfg, mock.fn)
	ctx, cancel := context.WithCancel(context.Background())
	rl.Start(ctx)
	defer cancel()

	// Build a tiny router with the middleware and a simple handler.
	handler := requestLogMiddleware(rl, func() *logging.DrainManager { return nil })(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "42")
		w.WriteHeader(http.StatusCreated)
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/admin/status", nil)
	req.RemoteAddr = "192.0.2.10:1234"
	req.ContentLength = 15
	req.Header.Set("X-Tenant-ID", "tenant-log-fields")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.NoError(t, rl.Shutdown(context.Background()))

	entries := mock.allEntries()
	testutil.Equal(t, 1, len(entries))
	e := entries[0]
	testutil.Equal(t, "POST", e.Method)
	testutil.Equal(t, "/api/admin/status", e.Path)
	testutil.Equal(t, 201, e.StatusCode)
	testutil.Equal(t, int64(15), e.RequestSize)
	// IP should be extracted from RemoteAddr.
	testutil.Equal(t, "192.0.2.10", e.IPAddress)
	testutil.Equal(t, "tenant-log-fields", e.TenantID)
}

func TestRequestLogMiddlewareNormalizesUnknownRequestSize(t *testing.T) {
	t.Parallel()

	mock := &mockFlushFn{}
	cfg := RequestLogConfig{
		Enabled:           true,
		BatchSize:         100,
		FlushIntervalSecs: 60,
		QueueSize:         100,
	}
	rl := newTestRequestLogger(cfg, mock.fn)
	ctx, cancel := context.WithCancel(context.Background())
	rl.Start(ctx)
	defer cancel()

	handler := requestLogMiddleware(rl, func() *logging.DrainManager { return nil })(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	req.ContentLength = -1
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.NoError(t, rl.Shutdown(context.Background()))

	entries := mock.allEntries()
	testutil.Equal(t, 1, len(entries))
	testutil.Equal(t, int64(0), entries[0].RequestSize)
}

func TestRequestLogMiddlewareFansOutToDrainManager(t *testing.T) {
	t.Parallel()

	drain := &mockDrainCapture{}
	dm := logging.NewDrainManager()
	dm.AddDrain("fanout", drain, logging.DrainWorkerConfig{
		BatchSize:     1,
		FlushInterval: 20 * time.Millisecond,
		QueueSize:     10,
	})
	dm.Start()

	handler := requestLogMiddleware(nil, func() *logging.DrainManager { return dm })(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/admin/status", nil)
	req.RemoteAddr = "198.51.100.1:4444"
	req.Header.Set("X-Tenant-ID", "tenant-fanout")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	deadline := time.Now().Add(2 * time.Second)
	var captured []logging.LogEntry
	for time.Now().Before(deadline) {
		drain.mu.Lock()
		if len(drain.batches) > 0 && len(drain.batches[0]) > 0 {
			captured = append(captured, drain.batches[0]...)
			drain.mu.Unlock()
			break
		}
		drain.mu.Unlock()
		time.Sleep(10 * time.Millisecond)
	}
	dm.Stop()

	testutil.Equal(t, 1, len(captured))
	testutil.Equal(t, "request", captured[0].Source)
	testutil.Equal(t, "GET", captured[0].Fields["method"])
	testutil.Equal(t, "/api/admin/status", captured[0].Fields["path"])
	testutil.Equal(t, "198.51.100.1", captured[0].Fields["ip_address"])
	testutil.Equal(t, "tenant-fanout", captured[0].Fields["tenant_id"])
}

func TestRequestLogMiddlewareIgnoresAnonymousTenantHeaderOnNonAllowlistedAPIPath(t *testing.T) {
	t.Parallel()

	mock := &mockFlushFn{}
	cfg := RequestLogConfig{
		Enabled:           true,
		BatchSize:         100,
		FlushIntervalSecs: 60,
		QueueSize:         100,
	}
	rl := newTestRequestLogger(cfg, mock.fn)
	ctx, cancel := context.WithCancel(context.Background())
	rl.Start(ctx)
	defer cancel()

	handler := requestLogMiddleware(rl, func() *logging.DrainManager { return nil })(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/collections", nil)
	req.Header.Set("X-Tenant-ID", "tenant-log-fields")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.NoError(t, rl.Shutdown(context.Background()))

	entries := mock.allEntries()
	testutil.Equal(t, 1, len(entries))
	testutil.Equal(t, "", entries[0].TenantID)
}

// TestRequestLogMiddlewareExtractsAuthClaims verifies user_id and api_key_id are captured from context.
func TestRequestLogMiddlewareExtractsAuthClaims(t *testing.T) {
	t.Parallel()

	mock := &mockFlushFn{}
	cfg := RequestLogConfig{
		Enabled:           true,
		BatchSize:         100,
		FlushIntervalSecs: 60,
		QueueSize:         100,
	}
	rl := newTestRequestLogger(cfg, mock.fn)
	ctx, cancel := context.WithCancel(context.Background())
	rl.Start(ctx)
	defer cancel()

	handler := requestLogMiddleware(rl, func() *logging.DrainManager { return nil })(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	claims := &auth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{Subject: "user-uuid-1234"},
		APIKeyID:         "apikey-uuid-5678",
	}
	req := httptest.NewRequest(http.MethodGet, "/api/collections", nil)
	req = req.WithContext(auth.ContextWithClaims(req.Context(), claims))

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.NoError(t, rl.Shutdown(context.Background()))

	entries := mock.allEntries()
	testutil.Equal(t, 1, len(entries))
	testutil.Equal(t, "user-uuid-1234", entries[0].UserID)
	testutil.Equal(t, "apikey-uuid-5678", entries[0].APIKeyID)
}

// TestRequestLoggerDisabledSkipsLogging verifies that disabled config causes Log() to be a no-op.
func TestRequestLoggerDisabledSkipsLogging(t *testing.T) {
	t.Parallel()

	mock := &mockFlushFn{}
	cfg := RequestLogConfig{
		Enabled:           false,
		BatchSize:         10,
		FlushIntervalSecs: 5,
		QueueSize:         100,
	}
	rl := newTestRequestLogger(cfg, mock.fn)
	ctx, cancel := context.WithCancel(context.Background())
	rl.Start(ctx)
	defer cancel()

	rl.Log(RequestLogEntry{Method: "GET", Path: "/test", StatusCode: 200})

	testutil.NoError(t, rl.Shutdown(context.Background()))

	testutil.Equal(t, 0, len(mock.allEntries()))
}

// TestRequestLoggerConfigDefaults verifies that zero-value config is populated with sane defaults.
func TestRequestLoggerConfigDefaults(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	testutil.Equal(t, true, cfg.Logging.RequestLogEnabled)
	testutil.Equal(t, 7, cfg.Logging.RequestLogRetentionDays)
	testutil.Equal(t, 100, cfg.Logging.RequestLogBatchSize)
	testutil.Equal(t, 5, cfg.Logging.RequestLogFlushIntervalSecs)
	testutil.Equal(t, 10000, cfg.Logging.RequestLogQueueSize)
}

func TestRequestLoggerMiddlewareDefaultsStatusTo200(t *testing.T) {
	t.Parallel()

	capture := &captureSlogHandler{}
	logger := slog.New(capture)
	handler := requestLogger(func() *slog.Logger { return logger })(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		// intentionally do not write headers/body
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	got := capture.lastRecord()
	if got == nil {
		t.Fatal("expected captured request log record")
	}
	switch v := got["status"].(type) {
	case int:
		testutil.Equal(t, 200, v)
	case int64:
		testutil.Equal(t, int64(200), v)
	default:
		t.Fatalf("expected status int/int64, got %T", got["status"])
	}
}

func TestRequestLoggerMiddlewareIncludesTenantIDFromRequest(t *testing.T) {
	t.Parallel()

	capture := &captureSlogHandler{}
	logger := slog.New(capture)
	handler := requestLogger(func() *slog.Logger { return logger })(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/admin/status", nil)
	req.Header.Set("X-Tenant-ID", "tenant-from-header")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	got := capture.lastRecord()
	if got == nil {
		t.Fatal("expected captured request log record")
	}
	testutil.Equal(t, "tenant-from-header", got["tenant_id"])
}

func TestRequestLoggerMiddlewareIgnoresAnonymousTenantIDOnNonAllowlistedAPIPath(t *testing.T) {
	t.Parallel()

	capture := &captureSlogHandler{}
	logger := slog.New(capture)
	handler := requestLogger(func() *slog.Logger { return logger })(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/collections", nil)
	req.Header.Set("X-Tenant-ID", "tenant-from-header")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	got := capture.lastRecord()
	if got == nil {
		t.Fatal("expected captured request log record")
	}
	_, ok := got["tenant_id"]
	testutil.False(t, ok, "anonymous non-allowlisted API paths must not log tenant_id from headers")
}

func TestRequestLoggerMiddlewareIncludesTraceFields(t *testing.T) {
	origTP := otel.GetTracerProvider()
	origProp := otel.GetTextMapPropagator()
	t.Cleanup(func() {
		otel.SetTracerProvider(origTP)
		otel.SetTextMapPropagator(origProp)
	})

	tp := sdktrace.NewTracerProvider()
	observability.SetGlobalTracerAndPropagator(tp)
	t.Cleanup(func() {
		_ = tp.Shutdown(context.Background())
	})

	tracer := tp.Tracer("test")
	ctx, span := tracer.Start(context.Background(), "request")
	defer span.End()

	capture := &captureSlogHandler{}
	logger := slog.New(capture)
	handler := requestLogger(func() *slog.Logger { return logger })(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))

	req := httptest.NewRequest(http.MethodGet, "/health", nil).WithContext(ctx)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	got := capture.lastRecord()
	if got == nil {
		t.Fatal("expected captured request log record")
	}
	traceID, ok := got["trace_id"].(string)
	testutil.True(t, ok, "expected trace_id string field")
	testutil.True(t, traceID != "", "expected trace_id to be populated")
	spanID, ok := got["span_id"].(string)
	testutil.True(t, ok, "expected span_id string field")
	testutil.True(t, spanID != "", "expected span_id to be populated")
}

func TestRequestLogMiddlewareFanoutIncludesTraceFields(t *testing.T) {
	origTP := otel.GetTracerProvider()
	origProp := otel.GetTextMapPropagator()
	t.Cleanup(func() {
		otel.SetTracerProvider(origTP)
		otel.SetTextMapPropagator(origProp)
	})

	tp := sdktrace.NewTracerProvider()
	observability.SetGlobalTracerAndPropagator(tp)
	t.Cleanup(func() {
		_ = tp.Shutdown(context.Background())
	})

	tracer := tp.Tracer("test")
	ctx, span := tracer.Start(context.Background(), "request")
	defer span.End()

	drain := &mockDrainCapture{}
	dm := logging.NewDrainManager()
	dm.AddDrain("fanout", drain, logging.DrainWorkerConfig{
		BatchSize:     1,
		FlushInterval: 20 * time.Millisecond,
		QueueSize:     10,
	})
	dm.Start()
	defer dm.Stop()

	handler := requestLogMiddleware(nil, func() *logging.DrainManager { return dm })(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/collections", nil).WithContext(ctx)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	deadline := time.Now().Add(2 * time.Second)
	var captured logging.LogEntry
	for time.Now().Before(deadline) {
		drain.mu.Lock()
		if len(drain.batches) > 0 && len(drain.batches[0]) > 0 {
			captured = drain.batches[0][0]
			drain.mu.Unlock()
			break
		}
		drain.mu.Unlock()
		time.Sleep(10 * time.Millisecond)
	}

	traceID, ok := captured.Fields["trace_id"].(string)
	testutil.True(t, ok, "expected trace_id string field")
	testutil.True(t, traceID != "", "expected trace_id to be populated")
	spanID, ok := captured.Fields["span_id"].(string)
	testutil.True(t, ok, "expected span_id string field")
	testutil.True(t, spanID != "", "expected span_id to be populated")
}
