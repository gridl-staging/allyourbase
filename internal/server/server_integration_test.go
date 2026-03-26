//go:build integration

package server_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/edgefunc"
	"github.com/allyourbase/ayb/internal/migrations"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/server"
	"github.com/allyourbase/ayb/internal/tenant"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/gorilla/websocket"
)

var sharedPG *testutil.PGContainer

func TestMain(m *testing.M) {
	ctx := context.Background()
	pg, cleanup := testutil.StartPostgresForTestMain(ctx)
	sharedPG = pg
	code := m.Run()
	cleanup()
	os.Exit(code)
}

func createIntegrationTestSchema(t *testing.T, ctx context.Context) {
	t.Helper()

	_, err := sharedPG.Pool.Exec(ctx, "DROP SCHEMA public CASCADE; CREATE SCHEMA public")
	if err != nil {
		t.Fatalf("resetting schema: %v", err)
	}

	_, err = sharedPG.Pool.Exec(ctx, `
		CREATE TABLE users (
			id SERIAL PRIMARY KEY,
			name TEXT NOT NULL,
			email VARCHAR(255) UNIQUE
		)
	`)
	if err != nil {
		t.Fatalf("creating test schema: %v", err)
	}
}

func setupServerWithTenantForRateLimit(t *testing.T, reqRateHard, reqRateSoft *int) (*httptest.Server, *tenant.TenantRateLimiter, string) {
	t.Helper()

	ctx := context.Background()
	ensureIntegrationMigrations(t, ctx)

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	testutil.NoError(t, ch.Load(ctx))

	cfg := config.Default()
	srv := server.New(cfg, logger, ch, sharedPG.Pool, nil, nil)

	tenantSvc := tenant.NewService(sharedPG.Pool, logger)
	usageAcc := tenant.NewUsageAccumulator(sharedPG.Pool, logger)
	srv.SetTenantService(tenantSvc)
	srv.SetUsageAccumulator(usageAcc)
	srv.SetQuotaChecker(tenant.DefaultQuotaChecker{})

	rl := tenant.NewTenantRateLimiter(time.Minute)
	srv.SetTenantRateLimiter(rl)

	connCounter := tenant.NewTenantConnCounter()
	srv.SetTenantConnCounter(connCounter)

	tenantEnt, err := tenantSvc.CreateTenant(ctx, "rate-limit-tenant", fmt.Sprintf("rate-limit-tenant-%d", time.Now().UnixNano()), "schema", "free", "default", nil, "")
	testutil.NoError(t, err)

	_, err = tenantSvc.SetQuotas(ctx, tenantEnt.ID, tenant.TenantQuotas{
		RequestRateRPSHard:      reqRateHard,
		RequestRateRPSSoft:      reqRateSoft,
		DBSizeBytesHard:         nil,
		DBSizeBytesSoft:         nil,
		RealtimeConnectionsHard: nil,
		RealtimeConnectionsSoft: nil,
	})
	testutil.NoError(t, err)

	return httptest.NewServer(srv.Router()), rl, tenantEnt.ID
}

func setupServerWithTenantForRealtimeLimit(t *testing.T, realtimeHard, realtimeSoft *int) (*httptest.Server, string) {
	t.Helper()

	ctx := context.Background()
	ensureIntegrationMigrations(t, ctx)

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	testutil.NoError(t, ch.Load(ctx))

	cfg := config.Default()
	srv := server.New(cfg, logger, ch, sharedPG.Pool, nil, nil)

	tenantSvc := tenant.NewService(sharedPG.Pool, logger)
	usageAcc := tenant.NewUsageAccumulator(sharedPG.Pool, logger)
	srv.SetTenantService(tenantSvc)
	srv.SetUsageAccumulator(usageAcc)
	srv.SetQuotaChecker(tenant.DefaultQuotaChecker{})
	srv.SetTenantConnCounter(tenant.NewTenantConnCounter())

	tenantEnt, err := tenantSvc.CreateTenant(ctx, "realtime-tenant", fmt.Sprintf("realtime-tenant-%d", time.Now().UnixNano()), "schema", "free", "default", nil, "")
	testutil.NoError(t, err)

	_, err = tenantSvc.SetQuotas(ctx, tenantEnt.ID, tenant.TenantQuotas{
		RealtimeConnectionsHard: realtimeHard,
		RealtimeConnectionsSoft: realtimeSoft,
		RequestRateRPSHard:      nil,
		RequestRateRPSSoft:      nil,
		DBSizeBytesHard:         nil,
		DBSizeBytesSoft:         nil,
	})
	testutil.NoError(t, err)

	return httptest.NewServer(srv.Router()), tenantEnt.ID
}

func dialTenantRealtimeWS(t *testing.T, baseURL, tenantID string) (*websocket.Conn, *http.Response, error) {
	t.Helper()
	wsURL := "ws" + strings.TrimPrefix(baseURL, "http") + "/api/realtime/ws"
	header := http.Header{}
	header.Set("X-Tenant-ID", tenantID)

	return websocket.DefaultDialer.Dial(wsURL, header)
}

func ensureIntegrationMigrations(t *testing.T, ctx context.Context) {
	t.Helper()

	runner := migrations.NewRunner(sharedPG.Pool, testutil.DiscardLogger())
	if err := runner.Bootstrap(ctx); err != nil {
		t.Fatalf("bootstrap migrations: %v", err)
	}
	if _, err := runner.Run(ctx); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
}

func readNextSSEEvent(t *testing.T, scanner *bufio.Scanner, timeout time.Duration, timeoutMessage string) []string {
	t.Helper()
	eventCh := make(chan []string, 1)
	go func() {
		var lines []string
		for scanner.Scan() {
			line := scanner.Text()
			lines = append(lines, line)
			if line == "" && len(lines) > 1 {
				break
			}
		}
		eventCh <- lines
	}()

	select {
	case lines := <-eventCh:
		return lines
	case <-time.After(timeout):
		t.Fatal(timeoutMessage)
		return nil
	}
}

func decodeCommittedRPCRow(t *testing.T, body []byte) map[string]any {
	t.Helper()

	var payload any
	testutil.NoError(t, json.Unmarshal(body, &payload))

	switch value := payload.(type) {
	case map[string]any:
		return value
	case []any:
		testutil.True(t, len(value) == 1, "expected one row from RPC notify function")
		row, ok := value[0].(map[string]any)
		testutil.True(t, ok, "expected first RPC row to be an object")
		return row
	default:
		t.Fatalf("unexpected RPC payload shape %T: %s", payload, string(body))
		return nil
	}
}

func TestSchemaEndpointReturnsValidJSON(t *testing.T) {
	ctx := context.Background()
	createIntegrationTestSchema(t, ctx)

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	err := ch.Load(ctx)
	testutil.NoError(t, err)

	cfg := config.Default()
	srv := server.New(cfg, logger, ch, sharedPG.Pool, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/schema", nil)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusOK, w.Code)
	testutil.Equal(t, "application/json", w.Header().Get("Content-Type"))

	// Should be valid JSON with tables.
	var result schema.SchemaCache
	err = json.Unmarshal(w.Body.Bytes(), &result)
	testutil.NoError(t, err)
	testutil.True(t, len(result.Tables) >= 1, "expected at least 1 table")
	testutil.NotNil(t, result.Tables["public.users"])
}

func TestTenantRequestRateQuotaRejectsHardLimit(t *testing.T) {
	hard := 1
	ts, rl, tenantID := setupServerWithTenantForRateLimit(t, &hard, nil)
	defer ts.Close()
	defer rl.Stop()

	for i := 0; i < 60; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/admin/status", nil)
		req.Header.Set("X-Tenant-ID", tenantID)
		ts.Config.Handler.ServeHTTP(w, req)
		testutil.StatusCode(t, http.StatusOK, w.Code)
	}

	over := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/status", nil)
	req.Header.Set("X-Tenant-ID", tenantID)
	ts.Config.Handler.ServeHTTP(over, req)
	testutil.Equal(t, http.StatusTooManyRequests, over.Code)
	retryAfter := over.Header().Get("Retry-After")
	testutil.True(t, retryAfter != "")
	_, parseErr := strconv.Atoi(retryAfter)
	testutil.NoError(t, parseErr)
}

func TestTenantRealtimeConnectionsQuotaRejectsOverHardLimit(t *testing.T) {
	hard := 1
	ts, tenantID := setupServerWithTenantForRealtimeLimit(t, &hard, nil)
	defer ts.Close()

	c1, _, err := dialTenantRealtimeWS(t, ts.URL, tenantID)
	testutil.NoError(t, err)
	defer c1.Close()

	_, cResp, err := dialTenantRealtimeWS(t, ts.URL, tenantID)
	testutil.True(t, err != nil)
	testutil.NotNil(t, cResp)
	testutil.Equal(t, http.StatusTooManyRequests, cResp.StatusCode)

	testutil.NoError(t, c1.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), time.Now().Add(time.Second)))
	time.Sleep(100 * time.Millisecond)

	c2, _, err := dialTenantRealtimeWS(t, ts.URL, tenantID)
	testutil.NoError(t, err)
	_ = c2.Close()
}

// TestRealtimeSSEReceivesCreateEvent verifies the full end-to-end flow:
// connect SSE → create record via API → receive the realtime event.
func TestRealtimeSSEReceivesCreateEvent(t *testing.T) {
	ctx := context.Background()
	createIntegrationTestSchema(t, ctx)

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	testutil.NoError(t, ch.Load(ctx))

	cfg := config.Default()
	srv := server.New(cfg, logger, ch, sharedPG.Pool, nil, nil)

	// Start a real HTTP server so SSE streaming works.
	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	// Connect to SSE endpoint.
	resp, err := http.Get(ts.URL + "/api/realtime?tables=users")
	testutil.NoError(t, err)
	defer resp.Body.Close()
	testutil.StatusCode(t, http.StatusOK, resp.StatusCode)
	testutil.Equal(t, "text/event-stream", resp.Header.Get("Content-Type"))

	scanner := bufio.NewScanner(resp.Body)

	// Read and verify connected event.
	connected := readNextSSEEvent(t, scanner, 5*time.Second, "timed out waiting for SSE connected event")
	testutil.Equal(t, "event: connected", connected[0])

	// Create a record via the API.
	body, _ := json.Marshal(map[string]any{"name": "Charlie", "email": "charlie@example.com"})
	createResp, err := http.Post(ts.URL+"/api/collections/users/", "application/json", bytes.NewReader(body))
	testutil.NoError(t, err)
	testutil.StatusCode(t, http.StatusCreated, createResp.StatusCode)
	createResp.Body.Close()

	// Read the create event from SSE.
	lines := readNextSSEEvent(t, scanner, 5*time.Second, "timed out waiting for SSE create event")
	testutil.True(t, len(lines) >= 1, "should have event lines")
	joined := strings.Join(lines, "\n")
	testutil.Contains(t, joined, `"action":"create"`)
	testutil.Contains(t, joined, `"table":"users"`)
	testutil.Contains(t, joined, `"Charlie"`)
}

// TestAdminStatsWithDBPool verifies that the stats endpoint includes DB pool
// fields when a real database pool is available.
func TestAdminStatsWithDBPool(t *testing.T) {
	ctx := context.Background()
	createIntegrationTestSchema(t, ctx)

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	testutil.NoError(t, ch.Load(ctx))

	cfg := config.Default()
	cfg.Admin.Password = "testpass"
	srv := server.New(cfg, logger, ch, sharedPG.Pool, nil, nil)

	token := adminLogin(t, srv)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/stats/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	srv.Router().ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusOK, w.Code)
	var stats map[string]any
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &stats))

	// With a real pool, DB stats should be present.
	testutil.NotNil(t, stats["db_pool_total"])
	testutil.NotNil(t, stats["db_pool_idle"])
	testutil.NotNil(t, stats["db_pool_in_use"])
	testutil.NotNil(t, stats["db_pool_max"])

	// Pool max should be positive.
	maxConns := stats["db_pool_max"].(float64)
	testutil.True(t, maxConns > 0, "db_pool_max should be positive")

	// Standard fields should also be present.
	testutil.NotNil(t, stats["go_version"])
	testutil.NotNil(t, stats["goroutines"])
}

// TestRealtimeSSEDoesNotReceiveUnsubscribedTable verifies that SSE clients
// only receive events for tables they subscribed to.
func TestRealtimeSSEDoesNotReceiveUnsubscribedTable(t *testing.T) {
	ctx := context.Background()

	// Reset schema with two tables.
	_, err := sharedPG.Pool.Exec(ctx, "DROP SCHEMA public CASCADE; CREATE SCHEMA public")
	testutil.NoError(t, err)
	_, err = sharedPG.Pool.Exec(ctx, `
		CREATE TABLE users (id SERIAL PRIMARY KEY, name TEXT NOT NULL);
		CREATE TABLE logs (id SERIAL PRIMARY KEY, message TEXT NOT NULL);
	`)
	testutil.NoError(t, err)

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	testutil.NoError(t, ch.Load(ctx))

	cfg := config.Default()
	srv := server.New(cfg, logger, ch, sharedPG.Pool, nil, nil)

	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	// Subscribe only to "users".
	resp, err := http.Get(ts.URL + "/api/realtime?tables=users")
	testutil.NoError(t, err)
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	// Skip connected event.
	readNextSSEEvent(t, scanner, 5*time.Second, "timed out waiting for SSE connected event")

	// Create a log record (not subscribed).
	body, err := json.Marshal(map[string]any{"message": "hello"})
	testutil.NoError(t, err)
	cr, err := http.Post(ts.URL+"/api/collections/logs/", "application/json", bytes.NewReader(body))
	testutil.NoError(t, err)
	testutil.StatusCode(t, http.StatusCreated, cr.StatusCode)
	cr.Body.Close()

	// Create a user record (subscribed).
	body, err = json.Marshal(map[string]any{"name": "Dave"})
	testutil.NoError(t, err)
	cr, err = http.Post(ts.URL+"/api/collections/users/", "application/json", bytes.NewReader(body))
	testutil.NoError(t, err)
	testutil.StatusCode(t, http.StatusCreated, cr.StatusCode)
	cr.Body.Close()

	// The next event should be for users, not logs.
	lines := readNextSSEEvent(t, scanner, 5*time.Second, "timed out waiting for SSE event")
	joined := strings.Join(lines, "\n")
	testutil.Contains(t, joined, `"table":"users"`)
	testutil.Contains(t, joined, `"Dave"`)
	// Should NOT contain logs data.
	testutil.False(t, strings.Contains(joined, `"logs"`), "should not receive logs events")
}

func TestEdgeFuncAdminDeployInvokeAndPersistLogs(t *testing.T) {
	ctx := context.Background()
	createIntegrationTestSchema(t, ctx)
	ensureIntegrationMigrations(t, ctx)

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	testutil.NoError(t, ch.Load(ctx))

	cfg := config.Default()
	cfg.Admin.Password = "testpass"

	srv := server.New(cfg, logger, ch, sharedPG.Pool, nil, nil)
	execPool := edgefunc.NewPool(cfg.EdgeFunctions.PoolSize)
	defer execPool.Close()

	store := edgefunc.NewPostgresStore(sharedPG.Pool)
	logStore := edgefunc.NewPostgresLogStore(sharedPG.Pool)
	svc := edgefunc.NewService(store, execPool, logStore,
		edgefunc.WithDefaultTimeout(time.Duration(cfg.EdgeFunctions.DefaultTimeoutMs)*time.Millisecond),
	)
	srv.SetEdgeFuncService(svc)

	adminToken := adminLogin(t, srv)

	fnName := fmt.Sprintf("itest-fn-%d", time.Now().UnixNano())
	source := `function handler(req) { return { statusCode: 201, body: req.method + "|" + req.path + "|" + req.query + "|" + req.body }; }`
	deployPayload := map[string]any{
		"name":   fnName,
		"source": source,
		"public": true,
	}
	deployBody, err := json.Marshal(deployPayload)
	testutil.NoError(t, err)

	deployReq := httptest.NewRequest(http.MethodPost, "/api/admin/functions", bytes.NewReader(deployBody))
	deployReq.Header.Set("Content-Type", "application/json")
	deployReq.Header.Set("Authorization", "Bearer "+adminToken)
	deployW := httptest.NewRecorder()
	srv.Router().ServeHTTP(deployW, deployReq)

	testutil.StatusCode(t, http.StatusCreated, deployW.Code)
	var deployed edgefunc.EdgeFunction
	testutil.NoError(t, json.Unmarshal(deployW.Body.Bytes(), &deployed))
	testutil.Equal(t, fnName, deployed.Name)

	invokeReq := httptest.NewRequest(http.MethodPost, "/functions/v1/"+fnName+"/nested/path?q=1", strings.NewReader("payload"))
	invokeReq.Header.Set("Content-Type", "text/plain")
	invokeW := httptest.NewRecorder()
	srv.Router().ServeHTTP(invokeW, invokeReq)

	testutil.StatusCode(t, http.StatusCreated, invokeW.Code)
	testutil.Equal(t, "POST|/"+fnName+"/nested/path|q=1|payload", invokeW.Body.String())

	logsReq := httptest.NewRequest(http.MethodGet, "/api/admin/functions/"+deployed.ID.String()+"/logs?page=1&perPage=10", nil)
	logsReq.Header.Set("Authorization", "Bearer "+adminToken)
	logsW := httptest.NewRecorder()
	srv.Router().ServeHTTP(logsW, logsReq)

	testutil.StatusCode(t, http.StatusOK, logsW.Code)
	var logs []edgefunc.LogEntry
	testutil.NoError(t, json.Unmarshal(logsW.Body.Bytes(), &logs))
	testutil.SliceLen(t, logs, 1)
	testutil.Equal(t, "success", logs[0].Status)
	testutil.True(t, logs[0].DurationMs >= 0, "duration must be non-negative")
	testutil.Equal(t, "POST", logs[0].RequestMethod)
	testutil.Equal(t, "/"+fnName+"/nested/path", logs[0].RequestPath)
}

// TestRealtimeSSEConfigRegression verifies that SSE realtime behavior is not
// regressed by config/metrics changes. This test ensures:
// 1. SSE connections work with default realtime config
// 2. SSE events (create/update/delete) are delivered correctly
// 3. Metrics correctly count SSE connections
func TestRealtimeSSEConfigRegression(t *testing.T) {
	ctx := context.Background()
	createIntegrationTestSchema(t, ctx)

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	testutil.NoError(t, ch.Load(ctx))

	cfg := config.Default()
	cfg.Metrics.Enabled = true
	cfg.Admin.Password = "testpass"
	srv := server.New(cfg, logger, ch, sharedPG.Pool, nil, nil)

	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	sseURL := ts.URL + "/api/realtime?tables=users"
	resp, err := http.Get(sseURL)
	testutil.NoError(t, err)
	defer resp.Body.Close()
	testutil.StatusCode(t, http.StatusOK, resp.StatusCode)
	testutil.Equal(t, "text/event-stream", resp.Header.Get("Content-Type"))

	scanner := bufio.NewScanner(resp.Body)

	connected := readNextSSEEvent(t, scanner, 5*time.Second, "timed out waiting for SSE connected event")
	testutil.Equal(t, "event: connected", connected[0])

	body, _ := json.Marshal(map[string]any{"name": "SSE_Test", "email": "sse@test.com"})
	createResp, err := http.Post(ts.URL+"/api/collections/users/", "application/json", bytes.NewReader(body))
	testutil.NoError(t, err)
	testutil.StatusCode(t, http.StatusCreated, createResp.StatusCode)
	createResp.Body.Close()

	lines := readNextSSEEvent(t, scanner, 5*time.Second, "timed out waiting for SSE create event")
	joined := strings.Join(lines, "\n")
	testutil.Contains(t, joined, `"action":"create"`)
	testutil.Contains(t, joined, `"table":"users"`)

	client := &http.Client{}

	updateReq, err := http.NewRequest(http.MethodPatch, ts.URL+"/api/collections/users/1", bytes.NewReader(body))
	testutil.NoError(t, err)
	updateReq.Header.Set("Content-Type", "application/json")
	updateResp, err := client.Do(updateReq)
	testutil.NoError(t, err)
	testutil.StatusCode(t, http.StatusOK, updateResp.StatusCode)
	updateResp.Body.Close()

	lines = readNextSSEEvent(t, scanner, 5*time.Second, "timed out waiting for SSE update event")
	joined = strings.Join(lines, "\n")
	testutil.Contains(t, joined, `"action":"update"`)
	testutil.Contains(t, joined, `"table":"users"`)

	delReq, err := http.NewRequest(http.MethodDelete, ts.URL+"/api/collections/users/1", nil)
	testutil.NoError(t, err)
	delResp, err := client.Do(delReq)
	testutil.NoError(t, err)
	testutil.StatusCode(t, http.StatusNoContent, delResp.StatusCode)
	delResp.Body.Close()

	lines = readNextSSEEvent(t, scanner, 5*time.Second, "timed out waiting for SSE delete event")
	joined = strings.Join(lines, "\n")
	testutil.Contains(t, joined, `"action":"delete"`)
	testutil.Contains(t, joined, `"table":"users"`)

	adminToken := adminLogin(t, srv)
	statsReq := httptest.NewRequest(http.MethodGet, "/api/admin/realtime/stats", nil)
	statsReq.Header.Set("Authorization", "Bearer "+adminToken)
	statsW := httptest.NewRecorder()
	srv.Router().ServeHTTP(statsW, statsReq)

	testutil.StatusCode(t, http.StatusOK, statsW.Code)
	var stats map[string]any
	testutil.NoError(t, json.Unmarshal(statsW.Body.Bytes(), &stats))

	conns := stats["connections"].(map[string]any)
	testutil.True(t, conns["sse"].(float64) >= 1, "expected at least 1 SSE connection in stats")
}

func TestRealtimeSSERPCNotifyPublishesCRUDShape(t *testing.T) {
	ctx := context.Background()
	createIntegrationTestSchema(t, ctx)

	_, err := sharedPG.Pool.Exec(ctx, `
		CREATE OR REPLACE FUNCTION public.rpc_insert_user_notify(p_name text, p_email text)
		RETURNS TABLE (id integer, name text, email varchar(255))
		LANGUAGE sql
		AS $$
			INSERT INTO public.users (name, email)
			VALUES (p_name, p_email)
			RETURNING users.id, users.name, users.email;
		$$;
	`)
	testutil.NoError(t, err)

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	testutil.NoError(t, ch.Load(ctx))

	cfg := config.Default()
	srv := server.New(cfg, logger, ch, sharedPG.Pool, nil, nil)

	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/realtime?tables=users")
	testutil.NoError(t, err)
	defer resp.Body.Close()
	testutil.StatusCode(t, http.StatusOK, resp.StatusCode)
	testutil.Equal(t, "text/event-stream", resp.Header.Get("Content-Type"))

	scanner := bufio.NewScanner(resp.Body)
	connected := readNextSSEEvent(t, scanner, 5*time.Second, "timed out waiting for SSE connected event")
	testutil.True(t, len(connected) >= 1, "expected SSE connected event frame")
	testutil.Equal(t, "event: connected", connected[0])

	rpcArgs, err := json.Marshal(map[string]any{
		"p_name":  "RPC Notify User",
		"p_email": "rpc-notify@example.com",
	})
	testutil.NoError(t, err)

	rpcReq, err := http.NewRequest(http.MethodPost, ts.URL+"/api/rpc/rpc_insert_user_notify", bytes.NewReader(rpcArgs))
	testutil.NoError(t, err)
	rpcReq.Header.Set("Content-Type", "application/json")
	rpcReq.Header.Set("X-Notify-Table", "users")
	rpcReq.Header.Set("X-Notify-Action", "create")

	rpcResp, err := http.DefaultClient.Do(rpcReq)
	testutil.NoError(t, err)
	defer rpcResp.Body.Close()
	testutil.StatusCode(t, http.StatusOK, rpcResp.StatusCode)

	rpcBody, err := io.ReadAll(rpcResp.Body)
	testutil.NoError(t, err)

	committedRow := decodeCommittedRPCRow(t, rpcBody)
	testutil.NotNil(t, committedRow)

	lines := readNextSSEEvent(t, scanner, 5*time.Second, "timed out waiting for SSE RPC notify event")
	var dataFrame string
	for _, line := range lines {
		if strings.HasPrefix(line, "data: ") {
			dataFrame = strings.TrimPrefix(line, "data: ")
			break
		}
	}
	testutil.True(t, dataFrame != "", "expected SSE data frame in event")

	var event map[string]any
	testutil.NoError(t, json.Unmarshal([]byte(dataFrame), &event))

	action, ok := event["action"].(string)
	testutil.True(t, ok, "expected action to be a string")
	table, ok := event["table"].(string)
	testutil.True(t, ok, "expected table to be a string")
	testutil.Equal(t, "create", action)
	testutil.Equal(t, "users", table)

	record, ok := event["record"].(map[string]any)
	testutil.True(t, ok, "expected record object in SSE payload")
	if !reflect.DeepEqual(committedRow, record) {
		t.Fatalf("SSE record mismatch: got %#v, want %#v", record, committedRow)
	}

	_, hasOldRecord := event["old_record"]
	testutil.False(t, hasOldRecord, "old_record should not be present in SSE payload")
}

func TestRealtimeSSERPCNotifyFilteredDelivery(t *testing.T) {
	ctx := context.Background()
	createIntegrationTestSchema(t, ctx)

	_, err := sharedPG.Pool.Exec(ctx, `
		CREATE OR REPLACE FUNCTION public.rpc_insert_user_notify_filtered(p_name text, p_email text)
		RETURNS TABLE (id integer, name text, email varchar(255))
		LANGUAGE sql
		AS $$
			INSERT INTO public.users (name, email)
			VALUES (p_name, p_email)
			RETURNING users.id, users.name, users.email;
		$$;
	`)
	testutil.NoError(t, err)

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	testutil.NoError(t, ch.Load(ctx))

	cfg := config.Default()
	srv := server.New(cfg, logger, ch, sharedPG.Pool, nil, nil)

	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	filteredResp, err := http.Get(ts.URL + "/api/realtime?tables=users&filter=name=eq.Bob")
	testutil.NoError(t, err)
	defer filteredResp.Body.Close()
	testutil.StatusCode(t, http.StatusOK, filteredResp.StatusCode)

	allResp, err := http.Get(ts.URL + "/api/realtime?tables=users")
	testutil.NoError(t, err)
	defer allResp.Body.Close()
	testutil.StatusCode(t, http.StatusOK, allResp.StatusCode)

	filteredScanner := bufio.NewScanner(filteredResp.Body)
	allScanner := bufio.NewScanner(allResp.Body)
	readNextSSEEvent(t, filteredScanner, 5*time.Second, "timed out waiting for filtered SSE connected event")
	readNextSSEEvent(t, allScanner, 5*time.Second, "timed out waiting for unfiltered SSE connected event")

	decodeSSEPayload := func(lines []string) map[string]any {
		var dataFrame string
		for _, line := range lines {
			if strings.HasPrefix(line, "data: ") {
				dataFrame = strings.TrimPrefix(line, "data: ")
				break
			}
		}
		testutil.True(t, dataFrame != "", "expected SSE data frame in event")

		var event map[string]any
		testutil.NoError(t, json.Unmarshal([]byte(dataFrame), &event))
		return event
	}

	callRPC := func(name, email string) {
		t.Helper()

		rpcArgs, marshalErr := json.Marshal(map[string]any{
			"p_name":  name,
			"p_email": email,
		})
		testutil.NoError(t, marshalErr)

		rpcReq, reqErr := http.NewRequest(http.MethodPost, ts.URL+"/api/rpc/rpc_insert_user_notify_filtered", bytes.NewReader(rpcArgs))
		testutil.NoError(t, reqErr)
		rpcReq.Header.Set("Content-Type", "application/json")
		rpcReq.Header.Set("X-Notify-Table", "users")
		rpcReq.Header.Set("X-Notify-Action", "create")

		rpcResp, doErr := http.DefaultClient.Do(rpcReq)
		testutil.NoError(t, doErr)
		defer rpcResp.Body.Close()
		testutil.StatusCode(t, http.StatusOK, rpcResp.StatusCode)
	}

	callRPC("Alice", "alice-rpc-filter@example.com")

	allAliceLines := readNextSSEEvent(t, allScanner, 5*time.Second, "timed out waiting for unfiltered SSE RPC Alice event")
	allAliceEvent := decodeSSEPayload(allAliceLines)
	testutil.Equal(t, "create", allAliceEvent["action"])
	testutil.Equal(t, "users", allAliceEvent["table"])
	allAliceRecord, ok := allAliceEvent["record"].(map[string]any)
	testutil.True(t, ok, "expected unfiltered SSE record payload")
	testutil.Equal(t, "Alice", allAliceRecord["name"])

	callRPC("Bob", "bob-rpc-filter@example.com")

	// If the filtered client incorrectly received Alice, that stale frame is read first here and fails this assertion.
	allBobLines := readNextSSEEvent(t, allScanner, 5*time.Second, "timed out waiting for unfiltered SSE RPC Bob event")
	filteredBobLines := readNextSSEEvent(t, filteredScanner, 5*time.Second, "timed out waiting for filtered SSE RPC Bob event")

	allBobEvent := decodeSSEPayload(allBobLines)
	testutil.Equal(t, "create", allBobEvent["action"])
	testutil.Equal(t, "users", allBobEvent["table"])
	allBobRecord, ok := allBobEvent["record"].(map[string]any)
	testutil.True(t, ok, "expected unfiltered SSE Bob record payload")
	testutil.Equal(t, "Bob", allBobRecord["name"])

	filteredBobEvent := decodeSSEPayload(filteredBobLines)
	testutil.Equal(t, "create", filteredBobEvent["action"])
	testutil.Equal(t, "users", filteredBobEvent["table"])
	filteredBobRecord, ok := filteredBobEvent["record"].(map[string]any)
	testutil.True(t, ok, "expected filtered SSE Bob record payload")
	testutil.Equal(t, "Bob", filteredBobRecord["name"])
}

func TestRealtimeSSEVoidRPCNotifyProducesNoEvent(t *testing.T) {
	ctx := context.Background()
	createIntegrationTestSchema(t, ctx)

	_, err := sharedPG.Pool.Exec(ctx, `
		CREATE OR REPLACE FUNCTION public.rpc_void_notify_no_event()
		RETURNS void
		LANGUAGE sql
		AS $$
			SELECT;
		$$;
	`)
	testutil.NoError(t, err)

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	testutil.NoError(t, ch.Load(ctx))

	cfg := config.Default()
	srv := server.New(cfg, logger, ch, sharedPG.Pool, nil, nil)

	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/realtime?tables=users")
	testutil.NoError(t, err)
	defer resp.Body.Close()
	testutil.StatusCode(t, http.StatusOK, resp.StatusCode)

	scanner := bufio.NewScanner(resp.Body)
	readNextSSEEvent(t, scanner, 5*time.Second, "timed out waiting for SSE connected event")

	voidRPCReq, err := http.NewRequest(http.MethodPost, ts.URL+"/api/rpc/rpc_void_notify_no_event", nil)
	testutil.NoError(t, err)
	voidRPCReq.Header.Set("X-Notify-Table", "users")
	voidRPCReq.Header.Set("X-Notify-Action", "delete")

	voidRPCResp, err := http.DefaultClient.Do(voidRPCReq)
	testutil.NoError(t, err)
	testutil.StatusCode(t, http.StatusNoContent, voidRPCResp.StatusCode)
	testutil.NoError(t, voidRPCResp.Body.Close())

	createBody, err := json.Marshal(map[string]any{
		"name":  "PostVoidCreate",
		"email": "post-void-create@example.com",
	})
	testutil.NoError(t, err)
	createResp, err := http.Post(ts.URL+"/api/collections/users/", "application/json", bytes.NewReader(createBody))
	testutil.NoError(t, err)
	testutil.StatusCode(t, http.StatusCreated, createResp.StatusCode)
	testutil.NoError(t, createResp.Body.Close())

	lines := readNextSSEEvent(t, scanner, 5*time.Second, "timed out waiting for SSE create event after void RPC")
	joined := strings.Join(lines, "\n")
	testutil.Contains(t, joined, `"action":"create"`)
	testutil.Contains(t, joined, `"table":"users"`)
	testutil.Contains(t, joined, `"PostVoidCreate"`)
	testutil.False(t, strings.Contains(joined, `"action":"delete"`), "void RPC should not produce notify event")
}

func TestRealtimeSSEFilteredDeliveryParity(t *testing.T) {
	ctx := context.Background()
	createIntegrationTestSchema(t, ctx)

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	testutil.NoError(t, ch.Load(ctx))

	cfg := config.Default()
	srv := server.New(cfg, logger, ch, sharedPG.Pool, nil, nil)

	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	filteredResp, err := http.Get(ts.URL + "/api/realtime?tables=users&filter=name=eq.Bob")
	testutil.NoError(t, err)
	defer filteredResp.Body.Close()
	testutil.StatusCode(t, http.StatusOK, filteredResp.StatusCode)

	allResp, err := http.Get(ts.URL + "/api/realtime?tables=users")
	testutil.NoError(t, err)
	defer allResp.Body.Close()
	testutil.StatusCode(t, http.StatusOK, allResp.StatusCode)

	filteredScanner := bufio.NewScanner(filteredResp.Body)
	allScanner := bufio.NewScanner(allResp.Body)
	readNextSSEEvent(t, filteredScanner, 5*time.Second, "timed out waiting for filtered SSE connected event")
	readNextSSEEvent(t, allScanner, 5*time.Second, "timed out waiting for unfiltered SSE connected event")

	aliceBody, err := json.Marshal(map[string]any{"name": "Alice", "email": "alice-filter-parity@example.com"})
	testutil.NoError(t, err)
	aliceResp, err := http.Post(ts.URL+"/api/collections/users/", "application/json", bytes.NewReader(aliceBody))
	testutil.NoError(t, err)
	testutil.StatusCode(t, http.StatusCreated, aliceResp.StatusCode)
	testutil.NoError(t, aliceResp.Body.Close())

	allAliceLines := readNextSSEEvent(t, allScanner, 5*time.Second, "timed out waiting for unfiltered SSE Alice event")
	allAliceJoined := strings.Join(allAliceLines, "\n")
	testutil.Contains(t, allAliceJoined, `"action":"create"`)
	testutil.Contains(t, allAliceJoined, `"table":"users"`)
	testutil.Contains(t, allAliceJoined, `"Alice"`)

	bobBody, err := json.Marshal(map[string]any{"name": "Bob", "email": "bob-filter-parity@example.com"})
	testutil.NoError(t, err)
	bobResp, err := http.Post(ts.URL+"/api/collections/users/", "application/json", bytes.NewReader(bobBody))
	testutil.NoError(t, err)
	testutil.StatusCode(t, http.StatusCreated, bobResp.StatusCode)
	testutil.NoError(t, bobResp.Body.Close())

	// Mirrors the WS pattern: if filtered SSE incorrectly got Alice, that stale event is read first and fails this assertion.
	allBobLines := readNextSSEEvent(t, allScanner, 5*time.Second, "timed out waiting for unfiltered SSE Bob event")
	filteredBobLines := readNextSSEEvent(t, filteredScanner, 5*time.Second, "timed out waiting for filtered SSE Bob event")

	allBobJoined := strings.Join(allBobLines, "\n")
	testutil.Contains(t, allBobJoined, `"action":"create"`)
	testutil.Contains(t, allBobJoined, `"table":"users"`)
	testutil.Contains(t, allBobJoined, `"Bob"`)

	filteredBobJoined := strings.Join(filteredBobLines, "\n")
	testutil.Contains(t, filteredBobJoined, `"action":"create"`)
	testutil.Contains(t, filteredBobJoined, `"table":"users"`)
	testutil.Contains(t, filteredBobJoined, `"Bob"`)
}
