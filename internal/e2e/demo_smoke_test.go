//go:build integration

// Package e2e — demo smoke tests verify that every demo's schema.sql can be
// applied via the admin SQL endpoint and that the resulting tables are
// functional. These are the tests that would have caught "cannot insert
// multiple commands into a prepared statement" before it reached a user.
package e2e

import (
	"context"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/examples"
	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/server"
	"github.com/allyourbase/ayb/internal/testutil"
)

// allDemos lists every registered demo. Keep in sync with demoRegistry in
// internal/cli/demo.go — TestDemoValidArgsMatchRegistry catches drift.
var allDemos = []string{"kanban", "live-polls"}

// newDemoServer creates a fresh server with auth enabled and migrations applied.
// Demos reference _ayb_users so migrations must run first.
func newDemoServer(t *testing.T) *httptest.Server {
	t.Helper()
	ctx := context.Background()
	resetDB(t, ctx)
	runMigrations(t, ctx)

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	testutil.NoError(t, ch.Load(ctx))

	cfg := config.Default()
	cfg.Auth.Enabled = true
	cfg.Auth.JWTSecret = testJWTSecret
	cfg.Admin.Password = testAdminPass

	authSvc := auth.NewService(sharedPG.Pool, testJWTSecret, 15*time.Minute, 7*24*time.Hour, 8, logger)
	srv := server.New(cfg, logger, ch, sharedPG.Pool, authSvc, nil)
	return httptest.NewServer(srv.Router())
}

// ---------------------------------------------------------------------------
// Demo schema smoke tests
// ---------------------------------------------------------------------------

func TestDemoSmoke_SchemaApply(t *testing.T) {
	for _, name := range allDemos {
		t.Run(name, func(t *testing.T) {
			ts := newDemoServer(t)
			defer ts.Close()

			schemaSQL, err := fs.ReadFile(examples.FS, name+"/schema.sql")
			testutil.NoError(t, err)

			token := adminToken(t, ts.URL)
			resp, body := httpJSON(t, "POST", ts.URL+"/api/admin/sql/",
				map[string]string{"query": string(schemaSQL)}, token)
			testutil.StatusCode(t, http.StatusOK, resp.StatusCode)

			if msg, ok := body["message"].(string); ok {
				t.Fatalf("schema apply returned error: %s", msg)
			}
		})
	}
}

func TestDemoSmoke_SchemaIdempotent(t *testing.T) {
	for _, name := range allDemos {
		t.Run(name, func(t *testing.T) {
			ts := newDemoServer(t)
			defer ts.Close()

			schemaSQL, err := fs.ReadFile(examples.FS, name+"/schema.sql")
			testutil.NoError(t, err)
			token := adminToken(t, ts.URL)

			resp, _ := httpJSON(t, "POST", ts.URL+"/api/admin/sql/",
				map[string]string{"query": string(schemaSQL)}, token)
			testutil.StatusCode(t, http.StatusOK, resp.StatusCode)

			resp2, body2 := httpJSON(t, "POST", ts.URL+"/api/admin/sql/",
				map[string]string{"query": string(schemaSQL)}, token)
			if resp2.StatusCode != http.StatusOK {
				msg, _ := body2["message"].(string)
				if !strings.Contains(msg, "already exists") {
					t.Fatalf("second schema apply: expected 200 or 'already exists' error, got %d: %s",
						resp2.StatusCode, msg)
				}
			}
		})
	}
}

func TestDemoSmoke_TablesExist(t *testing.T) {
	expectedTables := map[string][]string{
		"kanban":     {"boards", "columns", "cards"},
		"live-polls": {"polls", "poll_options", "votes"},
	}

	for _, name := range allDemos {
		t.Run(name, func(t *testing.T) {
			ts := newDemoServer(t)
			defer ts.Close()

			schemaSQL, err := fs.ReadFile(examples.FS, name+"/schema.sql")
			testutil.NoError(t, err)
			token := adminToken(t, ts.URL)

			resp, _ := httpJSON(t, "POST", ts.URL+"/api/admin/sql/",
				map[string]string{"query": string(schemaSQL)}, token)
			testutil.StatusCode(t, http.StatusOK, resp.StatusCode)

			tables := expectedTables[name]
			for _, table := range tables {
				query := fmt.Sprintf(
					"SELECT COUNT(*) FROM information_schema.tables WHERE table_schema='public' AND table_name='%s'",
					table)
				resp, body := httpJSON(t, "POST", ts.URL+"/api/admin/sql/",
					map[string]string{"query": query}, token)
				testutil.StatusCode(t, http.StatusOK, resp.StatusCode)
				rows := body["rows"].([]any)
				testutil.True(t, len(rows) > 0, "expected rows from information_schema query")
				count := rows[0].([]any)[0]
				testutil.Equal(t, float64(1), count.(float64))
			}
		})
	}
}

func TestDemoSmoke_RLSEnabled(t *testing.T) {
	expectedTables2 := map[string][]string{
		"kanban":     {"boards", "columns", "cards"},
		"live-polls": {"polls", "poll_options", "votes"},
	}

	for _, name := range allDemos {
		t.Run(name, func(t *testing.T) {
			ts := newDemoServer(t)
			defer ts.Close()

			schemaSQL, err := fs.ReadFile(examples.FS, name+"/schema.sql")
			testutil.NoError(t, err)
			token := adminToken(t, ts.URL)

			resp, _ := httpJSON(t, "POST", ts.URL+"/api/admin/sql/",
				map[string]string{"query": string(schemaSQL)}, token)
			testutil.StatusCode(t, http.StatusOK, resp.StatusCode)

			for _, table := range expectedTables2[name] {
				query := fmt.Sprintf(
					"SELECT rowsecurity FROM pg_tables WHERE schemaname='public' AND tablename='%s'",
					table)
				resp, body := httpJSON(t, "POST", ts.URL+"/api/admin/sql/",
					map[string]string{"query": query}, token)
				testutil.StatusCode(t, http.StatusOK, resp.StatusCode)
				rows := body["rows"].([]any)
				testutil.True(t, len(rows) > 0, "expected row for table %s", table)
				testutil.Equal(t, true, rows[0].([]any)[0].(bool))
			}
		})
	}
}

// TestDemoSmoke_RPCFunctionsExist verifies that demos with RPC functions
// have those functions created in the database after schema application.
func TestDemoSmoke_RPCFunctionsExist(t *testing.T) {
	expectedFunctions := map[string][]string{
		"live-polls": {"cast_vote"},
	}

	for _, name := range allDemos {
		funcs, ok := expectedFunctions[name]
		if !ok {
			continue
		}
		t.Run(name, func(t *testing.T) {
			ts := newDemoServer(t)
			defer ts.Close()

			schemaSQL, err := fs.ReadFile(examples.FS, name+"/schema.sql")
			testutil.NoError(t, err)
			token := adminToken(t, ts.URL)

			resp, _ := httpJSON(t, "POST", ts.URL+"/api/admin/sql/",
				map[string]string{"query": string(schemaSQL)}, token)
			testutil.StatusCode(t, http.StatusOK, resp.StatusCode)

			for _, fn := range funcs {
				query := fmt.Sprintf(
					"SELECT COUNT(*) FROM information_schema.routines WHERE routine_schema='public' AND routine_name='%s'",
					fn)
				resp, body := httpJSON(t, "POST", ts.URL+"/api/admin/sql/",
					map[string]string{"query": query}, token)
				testutil.StatusCode(t, http.StatusOK, resp.StatusCode)
				rows := body["rows"].([]any)
				testutil.True(t, len(rows) > 0, "expected result for function %s", fn)
				testutil.Equal(t, float64(1), rows[0].([]any)[0].(float64))
			}
		})
	}
}

func TestDemoSmoke_BasicCRUD(t *testing.T) {
	for _, name := range allDemos {
		t.Run(name, func(t *testing.T) {
			ts := newDemoServer(t)
			defer ts.Close()

			schemaSQL, err := fs.ReadFile(examples.FS, name+"/schema.sql")
			testutil.NoError(t, err)
			token := adminToken(t, ts.URL)

			resp, _ := httpJSON(t, "POST", ts.URL+"/api/admin/sql/",
				map[string]string{"query": string(schemaSQL)}, token)
			testutil.StatusCode(t, http.StatusOK, resp.StatusCode)

			registerUser(t, ts.URL, "demo-test@example.com", "password123")

			resp, body := httpJSON(t, "POST", ts.URL+"/api/admin/sql/",
				map[string]string{"query": "SELECT id FROM _ayb_users LIMIT 1"}, token)
			testutil.StatusCode(t, http.StatusOK, resp.StatusCode)
			userID := body["rows"].([]any)[0].([]any)[0].(string)

			switch name {
			case "kanban":
				smokeTestKanban(t, ts.URL, token, userID)
			case "live-polls":
				smokeTestLivePolls(t, ts.URL, token, userID)
			}
		})
	}
}

func smokeTestKanban(t *testing.T, baseURL, adminToken, userID string) {
	t.Helper()

	resp, body := httpJSON(t, "POST", baseURL+"/api/admin/sql/",
		map[string]string{"query": fmt.Sprintf(
			"INSERT INTO boards (title, user_id) VALUES ('Test Board', '%s') RETURNING id", userID)},
		adminToken)
	testutil.StatusCode(t, http.StatusOK, resp.StatusCode)
	rows := body["rows"].([]any)
	testutil.True(t, len(rows) == 1, "expected 1 row from INSERT RETURNING")
	boardID := rows[0].([]any)[0].(string)

	resp, body = httpJSON(t, "POST", baseURL+"/api/admin/sql/",
		map[string]string{"query": fmt.Sprintf(
			"INSERT INTO columns (board_id, title, position) VALUES ('%s', 'To Do', 0) RETURNING id", boardID)},
		adminToken)
	testutil.StatusCode(t, http.StatusOK, resp.StatusCode)
	rows = body["rows"].([]any)
	colID := rows[0].([]any)[0].(string)

	resp, _ = httpJSON(t, "POST", baseURL+"/api/admin/sql/",
		map[string]string{"query": fmt.Sprintf(
			"INSERT INTO cards (column_id, title, position) VALUES ('%s', 'Test Card', 0)", colID)},
		adminToken)
	testutil.StatusCode(t, http.StatusOK, resp.StatusCode)

	resp, body = httpJSON(t, "POST", baseURL+"/api/admin/sql/",
		map[string]string{"query": "SELECT title FROM boards"},
		adminToken)
	testutil.StatusCode(t, http.StatusOK, resp.StatusCode)
	testutil.Equal(t, 1, len(body["rows"].([]any)))
}

func smokeTestLivePolls(t *testing.T, baseURL, adminToken, userID string) {
	t.Helper()

	resp, body := httpJSON(t, "POST", baseURL+"/api/admin/sql/",
		map[string]string{"query": fmt.Sprintf(
			"INSERT INTO polls (user_id, question) VALUES ('%s', 'Favorite color?') RETURNING id", userID)},
		adminToken)
	testutil.StatusCode(t, http.StatusOK, resp.StatusCode)
	rows := body["rows"].([]any)
	pollID := rows[0].([]any)[0].(string)

	resp, body = httpJSON(t, "POST", baseURL+"/api/admin/sql/",
		map[string]string{"query": fmt.Sprintf(
			"INSERT INTO poll_options (poll_id, label, position) VALUES ('%s', 'Red', 0), ('%s', 'Blue', 1) RETURNING id",
			pollID, pollID)},
		adminToken)
	testutil.StatusCode(t, http.StatusOK, resp.StatusCode)
	rows = body["rows"].([]any)
	testutil.True(t, len(rows) == 2, "expected 2 option rows")
	optionID := rows[0].([]any)[0].(string)

	resp, _ = httpJSON(t, "POST", baseURL+"/api/admin/sql/",
		map[string]string{"query": fmt.Sprintf(
			"INSERT INTO votes (poll_id, option_id, user_id) VALUES ('%s', '%s', '%s')",
			pollID, optionID, userID)},
		adminToken)
	testutil.StatusCode(t, http.StatusOK, resp.StatusCode)

	resp2, body2 := httpJSON(t, "POST", baseURL+"/api/admin/sql/",
		map[string]string{"query": fmt.Sprintf(
			"INSERT INTO votes (poll_id, option_id, user_id) VALUES ('%s', '%s', '%s')",
			pollID, optionID, userID)},
		adminToken)
	testutil.StatusCode(t, http.StatusBadRequest, resp2.StatusCode)
	msg := body2["message"].(string)
	testutil.Contains(t, msg, "duplicate key")

	resp, body = httpJSON(t, "POST", baseURL+"/api/admin/sql/",
		map[string]string{"query": fmt.Sprintf("SELECT COUNT(*) FROM votes WHERE poll_id='%s'", pollID)},
		adminToken)
	testutil.StatusCode(t, http.StatusOK, resp.StatusCode)
	testutil.Equal(t, float64(1), body["rows"].([]any)[0].([]any)[0].(float64))
}
