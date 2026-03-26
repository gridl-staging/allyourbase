//go:build integration

package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/server"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/jackc/pgx/v5/pgxpool"
)

// assertErrorEnvelope unmarshals a JSON error response body, asserts the
// status code field matches wantCode, and returns the message string.
func assertErrorEnvelope(t *testing.T, body []byte, wantCode int) string {
	t.Helper()
	var errBody map[string]any
	testutil.NoError(t, json.Unmarshal(body, &errBody))
	code, ok := errBody["code"].(float64)
	testutil.True(t, ok, "expected numeric code in response envelope")
	testutil.Equal(t, float64(wantCode), code)
	msg, ok := errBody["message"].(string)
	testutil.True(t, ok, "expected string error message in response envelope")
	return msg
}

func TestAdminSQLCancelsLongRunningQueryOnRequestCancel(t *testing.T) {
	ctx := context.Background()
	createIntegrationTestSchema(t, ctx)

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	testutil.NoError(t, ch.Load(ctx))

	cfg := config.Default()
	cfg.Admin.Password = "testpass"
	srv := server.New(cfg, logger, ch, sharedPG.Pool, nil, nil)

	token := adminLogin(t, srv)

	requestCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	req := httptest.NewRequest(http.MethodPost, "/api/admin/sql/", strings.NewReader(`{"query":"SELECT pg_sleep(10)"}`))
	req = req.WithContext(requestCtx)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	w := httptest.NewRecorder()
	done := make(chan struct{})

	go func() {
		srv.Router().ServeHTTP(w, req)
		close(done)
	}()

	select {
	case <-done:
		t.Fatal("admin SQL request returned before cancellation")
	case <-time.After(300 * time.Millisecond):
	}

	cancel()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("admin SQL request did not return promptly after cancellation")
	}

	testutil.StatusCode(t, http.StatusBadRequest, w.Code)

	msg := assertErrorEnvelope(t, w.Body.Bytes(), http.StatusBadRequest)
	lowerMsg := strings.ToLower(msg)
	hasCancelMessage := strings.Contains(lowerMsg, "context canceled") ||
		strings.Contains(lowerMsg, "canceling statement due to user request") ||
		strings.Contains(lowerMsg, "statement canceled")
	testutil.True(t, hasCancelMessage, "expected cancellation message, got %q", msg)
}

func TestAdminSQLReturnsPromptErrorWhenPoolIsStarved(t *testing.T) {
	ctx := context.Background()
	createIntegrationTestSchema(t, ctx)

	singleConnCfg := sharedPG.Pool.Config().Copy()
	singleConnCfg.MaxConns = 1
	singleConnCfg.MinConns = 0

	singleConnPool, err := pgxpool.NewWithConfig(ctx, singleConnCfg)
	testutil.NoError(t, err)
	t.Cleanup(singleConnPool.Close)

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(singleConnPool, logger)
	testutil.NoError(t, ch.Load(ctx))

	cfg := config.Default()
	cfg.Admin.Password = "testpass"
	srv := server.New(cfg, logger, ch, singleConnPool, nil, nil)

	token := adminLogin(t, srv)

	pinnedConn, err := singleConnPool.Acquire(ctx)
	testutil.NoError(t, err)
	t.Cleanup(pinnedConn.Release)

	const requestTimeout = 350 * time.Millisecond
	requestCtx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	req := httptest.NewRequest(http.MethodPost, "/api/admin/sql/", strings.NewReader(`{"query":"SELECT 1"}`))
	req = req.WithContext(requestCtx)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	w := httptest.NewRecorder()
	start := time.Now()
	srv.Router().ServeHTTP(w, req)
	elapsed := time.Since(start)

	const maxCompletionSlack = 650 * time.Millisecond
	testutil.True(t, elapsed >= requestTimeout/2, "expected request to block on pool starvation before timeout, took %s", elapsed)
	testutil.True(
		t,
		elapsed <= requestTimeout+maxCompletionSlack,
		"expected request to fail near request deadline (%s) while pool starved, took %s",
		requestTimeout,
		elapsed,
	)
	testutil.StatusCode(t, http.StatusBadRequest, w.Code)

	msg := assertErrorEnvelope(t, w.Body.Bytes(), http.StatusBadRequest)
	lowerMsg := strings.ToLower(msg)
	hasDeadlineMessage := strings.Contains(lowerMsg, "context deadline exceeded") ||
		strings.Contains(lowerMsg, "context canceled") ||
		strings.Contains(lowerMsg, "canceled")
	testutil.True(t, hasDeadlineMessage, "expected deadline/cancellation message, got %q", msg)
}
