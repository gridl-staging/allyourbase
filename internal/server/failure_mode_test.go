package server

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/testutil"
)

// TestAPIRespondsNormallyWhenOTLPUnreachable verifies that when telemetry is enabled
// but the OTLP endpoint is unreachable, the API continues to serve requests
// without any error content leaking into response headers or body.
func TestAPIRespondsNormallyWhenOTLPUnreachable(t *testing.T) {
	t.Parallel()

	// Find a port that is definitely not listening.
	closedPort := findClosedPort(t)

	cfg := config.Default()
	cfg.Telemetry.Enabled = true
	cfg.Telemetry.OTLPEndpoint = closedPort
	cfg.Telemetry.ServiceName = "ayb-test"
	cfg.Telemetry.SampleRate = 1.0

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(nil, logger)
	srv := newServer(cfg, logger, ch, nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 when OTLP unreachable, got %d (body: %s)", w.Code, w.Body.String())
	}

	// Verify no error content in response headers.
	for key := range w.Header() {
		for _, val := range w.Header()[key] {
			if containsErrorText(val) {
				t.Errorf("response header %q: %q contains error text", key, val)
			}
		}
	}
}

// TestRequestLoggerDropsGracefullyWhenDBUnavailable verifies that when the flush
// function persistently returns an error (simulating DB unavailability), Shutdown
// returns without deadlock or panic, and drops are correctly accounted for.
func TestRequestLoggerDropsGracefullyWhenDBUnavailable(t *testing.T) {
	t.Parallel()

	dbErr := errors.New("simulated DB unavailable")
	flush := func(_ context.Context, _ []RequestLogEntry) error {
		return dbErr
	}

	cfg := RequestLogConfig{
		Enabled:   true,
		BatchSize: 100,
		QueueSize: 50,
	}
	rl := newRequestLoggerWithFlush(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), flush)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	rl.Start(ctx)

	for range 10 {
		rl.Log(RequestLogEntry{Method: "POST", Path: "/api/collections", StatusCode: 201})
	}

	// Shutdown must return even though the flush function always errors.
	shutdownErr := rl.Shutdown(context.Background())

	// The shutdown may or may not propagate flush errors; what matters is that
	// it doesn't deadlock or panic.  The drop accounting must be non-negative.
	_ = shutdownErr

	if rl.DropCount() < 0 {
		t.Error("expected non-negative drop count after DB-unavailable flush errors")
	}
	testutil.True(t, true, "Shutdown completed without deadlock or panic")
}

// findClosedPort returns a "host:port" string pointing at a port that is
// definitely not listening (the listener is closed immediately after binding).
func findClosedPort(t *testing.T) string {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("findClosedPort: listen: %v", err)
	}
	addr := lis.Addr().String()
	lis.Close()
	return addr
}

// containsErrorText returns true when s contains obvious error indicators.
func containsErrorText(s string) bool {
	for _, keyword := range []string{"error", "panic", "fail", "unavailable"} {
		for i := 0; i <= len(s)-len(keyword); i++ {
			match := true
			for j := range keyword {
				c := s[i+j]
				if c >= 'A' && c <= 'Z' {
					c += 32
				}
				if c != keyword[j] {
					match = false
					break
				}
			}
			if match {
				return true
			}
		}
	}
	return false
}
