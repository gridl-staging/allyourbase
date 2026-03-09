//go:build integration

package server_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/server"
	"github.com/allyourbase/ayb/internal/testutil"
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
	var connected []string
	for scanner.Scan() {
		line := scanner.Text()
		connected = append(connected, line)
		if line == "" && len(connected) > 1 {
			break
		}
	}
	testutil.Equal(t, "event: connected", connected[0])

	// Create a record via the API.
	body, _ := json.Marshal(map[string]any{"name": "Charlie", "email": "charlie@example.com"})
	createResp, err := http.Post(ts.URL+"/api/collections/users/", "application/json", bytes.NewReader(body))
	testutil.NoError(t, err)
	testutil.StatusCode(t, http.StatusCreated, createResp.StatusCode)
	createResp.Body.Close()

	// Read the create event from SSE with a timeout.
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
		testutil.True(t, len(lines) >= 1, "should have event lines")
		joined := strings.Join(lines, "\n")
		testutil.Contains(t, joined, `"action":"create"`)
		testutil.Contains(t, joined, `"table":"users"`)
		testutil.Contains(t, joined, `"Charlie"`)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for SSE create event")
	}
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
	for scanner.Scan() {
		if scanner.Text() == "" {
			break
		}
	}

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
		joined := strings.Join(lines, "\n")
		testutil.Contains(t, joined, `"table":"users"`)
		testutil.Contains(t, joined, `"Dave"`)
		// Should NOT contain logs data.
		testutil.False(t, strings.Contains(joined, `"logs"`), "should not receive logs events")
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for SSE event")
	}
}
