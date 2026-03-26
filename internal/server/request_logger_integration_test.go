//go:build integration

package server

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/logging"
	"github.com/allyourbase/ayb/internal/migrations"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func newRequestLoggerTestDB(t *testing.T) *testutil.PGContainer {
	t.Helper()
	ctx := context.Background()
	db, cleanup := testutil.StartPostgresForTestMain(ctx)
	t.Cleanup(cleanup)
	return db
}

func ensureIntegrationMigrations(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()

	runner := migrations.NewRunner(pool, testutil.DiscardLogger())
	if err := runner.Bootstrap(ctx); err != nil {
		t.Fatalf("bootstrap migrations: %v", err)
	}
	if _, err := runner.Run(ctx); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
}

func newRequestLoggerServerForIntegration(
	t *testing.T,
	pool *pgxpool.Pool,
	batchSize,
	flushIntervalSecs int,
) *Server {
	t.Helper()
	ctx := context.Background()
	ensureIntegrationMigrations(t, ctx, pool)

	_, err := pool.Exec(ctx, "TRUNCATE TABLE _ayb_request_logs")
	testutil.NoError(t, err)

	cfg := config.Default()
	cfg.Admin.Password = "testpass"
	cfg.Logging.RequestLogBatchSize = batchSize
	cfg.Logging.RequestLogFlushIntervalSecs = flushIntervalSecs
	cfg.Logging.RequestLogQueueSize = 32

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cache := schema.NewCacheHolder(pool, logger)
	testutil.NoError(t, cache.Load(ctx))

	srv := newServer(cfg, logger, cache, pool, nil, nil, nil)
	srv.startRequestLogger(context.Background())
	t.Cleanup(func() {
		_ = srv.requestLogger.Shutdown(context.Background())
	})

	return srv
}

func requestAdminToken(t *testing.T, srv *Server) string {
	t.Helper()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/auth", strings.NewReader(`{"password":"testpass"}`))
	req.Header.Set("Content-Type", "application/json")
	srv.Router().ServeHTTP(w, req)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	var body map[string]string
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	token := body["token"]
	testutil.True(t, token != "", "expected admin token")
	return token
}

func waitForRequestLogCount(t *testing.T, pool *pgxpool.Pool, requestIDs []string, expected int) {
	t.Helper()
	ctx := context.Background()
	deadline := time.Now().Add(2 * time.Second)
	var count int

	query := "SELECT COUNT(*) FROM _ayb_request_logs"
	var args []any
	if len(requestIDs) > 0 {
		query = "SELECT COUNT(*) FROM _ayb_request_logs WHERE request_id = ANY($1::text[])"
		args = []any{requestIDs}
	}
	for {
		err := pool.QueryRow(ctx, query, args...).Scan(&count)
		if err != nil {
			t.Fatalf("query request logs count: %v", err)
		}
		if count >= expected {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for request logs count, got %d want %d", count, expected)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func waitForRequestLogByRequestID(
	t *testing.T,
	pool *pgxpool.Pool,
	requestID string,
) (method, path string, status int, requestSize, responseSize int64, remoteIP string) {
	t.Helper()
	ctx := context.Background()
	deadline := time.Now().Add(2 * time.Second)
	for {
		err := pool.QueryRow(ctx,
			`SELECT method, path, status_code, request_size, response_size, COALESCE(host(ip_address), '')
			 FROM _ayb_request_logs WHERE request_id = $1`,
			requestID,
		).Scan(&method, &path, &status, &requestSize, &responseSize, &remoteIP)
		if err == nil {
			return
		}
		if err != pgx.ErrNoRows {
			t.Fatalf("query request log by request_id: %v", err)
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for request log row %q", requestID)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func seedRequestLogs(t *testing.T, pool *pgxpool.Pool, rows []struct {
	method    string
	path      string
	status    int
	timestamp time.Time
	requestID string
}) {
	t.Helper()
	ctx := context.Background()
	for _, row := range rows {
		_, err := pool.Exec(ctx,
			`INSERT INTO _ayb_request_logs (method, path, status_code, duration_ms, request_size, response_size, timestamp, request_id)
			 VALUES ($1, $2, $3, 0, 0, 0, $4, $5)`,
			row.method, row.path, row.status, row.timestamp, row.requestID,
		)
		testutil.NoError(t, err)
	}
}

func TestRequestLoggerIntegrationWritesRequestLogRowAfterHTTP(t *testing.T) {
	db := newRequestLoggerTestDB(t)
	srv := newRequestLoggerServerForIntegration(t, db.Pool, 100, 1)

	reqID := "integration-req-fields"
	req := httptest.NewRequest(http.MethodGet, "/health", strings.NewReader(`{"ping":"pong"}`))
	req.RemoteAddr = "198.51.100.1:1234"
	req.Header.Set("X-Request-Id", reqID)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	waitForRequestLogCount(t, db.Pool, []string{reqID}, 1)
	method, path, status, requestSize, responseSize, remoteIP := waitForRequestLogByRequestID(t, db.Pool, reqID)
	testutil.Equal(t, http.MethodGet, method)
	testutil.Equal(t, "/health", path)
	testutil.Equal(t, http.StatusOK, status)
	testutil.Equal(t, int64(len(`{"ping":"pong"}`)), requestSize)
	testutil.True(t, responseSize > 0, "response size should be tracked")
	testutil.Equal(t, "198.51.100.1", remoteIP)
}

func TestRequestLoggerIntegrationFlushesAtBatchSize(t *testing.T) {
	db := newRequestLoggerTestDB(t)
	srv := newRequestLoggerServerForIntegration(t, db.Pool, 2, 60)

	reqIDs := []string{"batch-1", "batch-2"}
	for _, reqID := range reqIDs {
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		req.Header.Set("X-Request-Id", reqID)
		w := httptest.NewRecorder()
		srv.Router().ServeHTTP(w, req)
		testutil.StatusCode(t, http.StatusOK, w.Code)
	}
	waitForRequestLogCount(t, db.Pool, reqIDs, 2)
}

func TestRequestLoggerIntegrationFlushesAtInterval(t *testing.T) {
	db := newRequestLoggerTestDB(t)
	srv := newRequestLoggerServerForIntegration(t, db.Pool, 100, 1)
	reqID := "interval-flush"

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("X-Request-Id", reqID)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	waitForRequestLogCount(t, db.Pool, []string{reqID}, 1)
}

func TestRequestLoggerIntegrationHighConcurrencyDoesNotBlockWithFullQueue(t *testing.T) {
	flush := make(chan struct{})
	cfg := RequestLogConfig{
		Enabled:           true,
		BatchSize:         1,
		FlushIntervalSecs: 60,
		QueueSize:         2,
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	rl := newRequestLoggerWithFlush(cfg, logger, func(ctx context.Context, entries []RequestLogEntry) error {
		<-flush
		return nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	rl.Start(ctx)
	t.Cleanup(func() {
		cancel()
		close(flush)
		_ = rl.Shutdown(context.Background())
	})

	handler := requestLogMiddleware(rl, func() *logging.DrainManager { return nil })(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	start := time.Now()
	for range 50 {
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
	}
	duration := time.Since(start)
	testutil.True(t, duration < 500*time.Millisecond, "request logging should not block the request path when queue is full")
	testutil.True(t, rl.DropCount() > 0, "expected at least one dropped request log entry when queue is full")
}

func TestRequestLoggerIntegrationShutdownFlushesPending(t *testing.T) {
	db := newRequestLoggerTestDB(t)
	srv := newRequestLoggerServerForIntegration(t, db.Pool, 1000, 60)
	reqID := "shutdown-flush"

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("X-Request-Id", reqID)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	testutil.NoError(t, srv.requestLogger.Shutdown(context.Background()))
	waitForRequestLogCount(t, db.Pool, []string{reqID}, 1)
}

func TestAdminRequestLogsEndpointFiltersByMethodPathStatusAndTime(t *testing.T) {
	db := newRequestLoggerTestDB(t)
	srv := newRequestLoggerServerForIntegration(t, db.Pool, 100, 60)

	seedRequestLogs(t, db.Pool, []struct {
		method    string
		path      string
		status    int
		timestamp time.Time
		requestID string
	}{
		{method: http.MethodGet, path: "/api/collections/orders", status: 200, timestamp: time.Date(2026, 2, 1, 10, 0, 0, 0, time.UTC), requestID: "seed-a"},
		{method: http.MethodPost, path: "/api/collections/orders", status: 500, timestamp: time.Date(2026, 2, 1, 11, 0, 0, 0, time.UTC), requestID: "seed-b"},
		{method: http.MethodGet, path: "/api/collections/users", status: 500, timestamp: time.Date(2026, 2, 2, 12, 0, 0, 0, time.UTC), requestID: "seed-c"},
		{method: http.MethodGet, path: "/api/health", status: 500, timestamp: time.Date(2026, 2, 3, 9, 30, 0, 0, time.UTC), requestID: "seed-d"},
	})

	token := requestAdminToken(t, srv)

	q := url.Values{}
	q.Set("method", "GET")
	q.Set("path", "/api/collections/users")
	q.Set("status", "500")
	q.Set("from", "2026-02-02T00:00:00Z")
	q.Set("to", "2026-02-02T23:59:59Z")
	q.Set("limit", "20")
	q.Set("offset", "0")

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/analytics/requests?"+q.Encode(), nil)
	req.Header.Set("Authorization", "Bearer "+token)
	srv.Router().ServeHTTP(w, req)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	var resp struct {
		Items []struct {
			ID         string `json:"id"`
			Method     string `json:"method"`
			Path       string `json:"path"`
			StatusCode int    `json:"status_code"`
		} `json:"items"`
		Count  int `json:"count"`
		Limit  int `json:"limit"`
		Offset int `json:"offset"`
	}
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	testutil.Equal(t, 1, resp.Count)
	testutil.Equal(t, 1, len(resp.Items))
	testutil.Equal(t, "GET", resp.Items[0].Method)
	testutil.Equal(t, "/api/collections/users", resp.Items[0].Path)
	testutil.Equal(t, 500, resp.Items[0].StatusCode)
}
