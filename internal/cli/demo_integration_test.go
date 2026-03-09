//go:build integration

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/examples"
	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/migrations"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/server"
	"github.com/allyourbase/ayb/internal/testutil"
)

var sharedPG *testutil.PGContainer

const (
	testJWTSecret = "demo-integration-test-secret-that-is-at-least-32-chars!!"
	testAdminPass = "demo-test-admin-password"
)

func TestMain(m *testing.M) {
	ctx := context.Background()
	pg, cleanup := testutil.StartPostgresForTestMain(ctx)
	sharedPG = pg
	code := m.Run()
	cleanup()
	os.Exit(code)
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func resetDB(t *testing.T, ctx context.Context) {
	t.Helper()
	_, err := sharedPG.Pool.Exec(ctx, "DROP SCHEMA public CASCADE; CREATE SCHEMA public")
	testutil.NoError(t, err)
}

func runMigrations(t *testing.T, ctx context.Context) {
	t.Helper()
	logger := testutil.DiscardLogger()
	runner := migrations.NewRunner(sharedPG.Pool, logger)
	testutil.NoError(t, runner.Bootstrap(ctx))
	_, err := runner.Run(ctx)
	testutil.NoError(t, err)
}

// newDemoServer creates a server with auth enabled and migrations applied,
// matching the state a demo command expects.
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

func getAdminToken(t *testing.T, baseURL string) string {
	t.Helper()
	body, err := json.Marshal(map[string]string{"password": testAdminPass})
	testutil.NoError(t, err)
	resp, err := http.Post(baseURL+"/api/admin/auth", "application/json", bytes.NewReader(body))
	testutil.NoError(t, err)
	defer resp.Body.Close()
	testutil.Equal(t, http.StatusOK, resp.StatusCode)
	var result map[string]any
	testutil.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
	token, ok := result["token"].(string)
	testutil.True(t, ok, "admin token should be a string")
	return token
}

func postAdminSQL(t *testing.T, baseURL, token, query string) (*http.Response, map[string]any) {
	t.Helper()
	body, err := json.Marshal(map[string]string{"query": query})
	testutil.NoError(t, err)
	req, err := http.NewRequest("POST", baseURL+"/api/admin/sql/", bytes.NewReader(body))
	testutil.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	testutil.NoError(t, err)
	raw, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	testutil.NoError(t, err)
	var result map[string]any
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &result); err != nil {
			t.Fatalf("postAdminSQL: failed to decode response JSON: %v\nbody: %s", err, raw)
		}
	}
	return resp, result
}

// sqlQueryValue runs a single-value SQL query via the admin endpoint and
// returns the scalar result as a float64 (json numbers unmarshal as float64).
func sqlQueryScalar(t *testing.T, baseURL, token, query string) float64 {
	t.Helper()
	resp, body := postAdminSQL(t, baseURL, token, query)
	testutil.Equal(t, http.StatusOK, resp.StatusCode)
	rows, ok := body["rows"].([]any)
	if !ok || len(rows) == 0 {
		t.Fatalf("expected rows for query: %s", query)
	}
	row := rows[0].([]any)
	return row[0].(float64)
}

// ---------------------------------------------------------------------------
// RLS policies (checks pg_policies count — complements e2e's rowsecurity flag check)
// ---------------------------------------------------------------------------

func TestDemoRLSPolicies_Kanban(t *testing.T) {
	ts := newDemoServer(t)
	defer ts.Close()

	token := getAdminToken(t, ts.URL)
	schemaSQL, err := fs.ReadFile(examples.FS, "kanban/schema.sql")
	testutil.NoError(t, err)
	resp, body := postAdminSQL(t, ts.URL, token, string(schemaSQL))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("schema apply failed: %v", body)
	}

	for _, table := range []string{"boards", "columns", "cards"} {
		count := sqlQueryScalar(t, ts.URL, token, fmt.Sprintf(
			"SELECT COUNT(*) FROM pg_policies WHERE tablename='%s'", table))
		if count == 0 {
			t.Errorf("table %s has no RLS policies", table)
		}
	}
}

func TestDemoRLSPolicies_LivePolls(t *testing.T) {
	ts := newDemoServer(t)
	defer ts.Close()

	token := getAdminToken(t, ts.URL)
	schemaSQL, err := fs.ReadFile(examples.FS, "live-polls/schema.sql")
	testutil.NoError(t, err)
	resp, body := postAdminSQL(t, ts.URL, token, string(schemaSQL))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("schema apply failed: %v", body)
	}

	for _, table := range []string{"polls", "poll_options", "votes"} {
		count := sqlQueryScalar(t, ts.URL, token, fmt.Sprintf(
			"SELECT COUNT(*) FROM pg_policies WHERE tablename='%s'", table))
		if count == 0 {
			t.Errorf("table %s has no RLS policies", table)
		}
	}
}

// ---------------------------------------------------------------------------
// Indexes (unique to cli — not covered in e2e)
// ---------------------------------------------------------------------------

func TestDemoIndexes_LivePolls(t *testing.T) {
	ts := newDemoServer(t)
	defer ts.Close()

	token := getAdminToken(t, ts.URL)
	schemaSQL, err := fs.ReadFile(examples.FS, "live-polls/schema.sql")
	testutil.NoError(t, err)
	resp, body := postAdminSQL(t, ts.URL, token, string(schemaSQL))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("schema apply failed: %v", body)
	}

	for _, idx := range []string{"idx_poll_options_poll_id", "idx_votes_poll_id", "idx_votes_option_id"} {
		count := sqlQueryScalar(t, ts.URL, token, fmt.Sprintf(
			"SELECT COUNT(*) FROM pg_indexes WHERE indexname='%s'", idx))
		if count == 0 {
			t.Errorf("index %s not found after schema apply", idx)
		}
	}
}

// ---------------------------------------------------------------------------
// Demo seed users: verify demo accounts can be created and log in.
// This tests the seedDemoUsers + login path that ayb demo uses.
// ---------------------------------------------------------------------------

func TestDemoSeedAndLogin_Kanban(t *testing.T) {
	testDemoSeedAndLogin(t, "kanban")
}

func TestDemoSeedAndLogin_LivePolls(t *testing.T) {
	testDemoSeedAndLogin(t, "live-polls")
}

func testDemoSeedAndLogin(t *testing.T, demoName string) {
	ts := newDemoServer(t)
	defer ts.Close()

	// Apply schema first.
	token := getAdminToken(t, ts.URL)
	schemaSQL, err := fs.ReadFile(examples.FS, demoName+"/schema.sql")
	testutil.NoError(t, err)
	resp, body := postAdminSQL(t, ts.URL, token, string(schemaSQL))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("schema apply failed: %v", body)
	}

	// Seed demo users (same as seedDemoUsers in demo.go).
	for _, u := range demoSeedUsers {
		regBody, err := json.Marshal(map[string]string{"email": u.Email, "password": u.Password})
		testutil.NoError(t, err)
		regResp, err := http.Post(ts.URL+"/api/auth/register", "application/json", bytes.NewReader(regBody))
		testutil.NoError(t, err)
		regResp.Body.Close()
		if regResp.StatusCode != http.StatusCreated && regResp.StatusCode != http.StatusConflict {
			t.Fatalf("registering %s: unexpected status %d", u.Email, regResp.StatusCode)
		}
	}

	// Verify all seed users can log in.
	for _, u := range demoSeedUsers {
		loginBody, err := json.Marshal(map[string]string{"email": u.Email, "password": u.Password})
		testutil.NoError(t, err)
		loginResp, err := http.Post(ts.URL+"/api/auth/login", "application/json", bytes.NewReader(loginBody))
		testutil.NoError(t, err)
		if loginResp.StatusCode != http.StatusOK {
			loginResp.Body.Close()
			t.Errorf("login %s: expected 200, got %d", u.Email, loginResp.StatusCode)
			continue
		}
		var result map[string]any
		testutil.NoError(t, json.NewDecoder(loginResp.Body).Decode(&result))
		loginResp.Body.Close()
		if _, ok := result["token"].(string); !ok {
			t.Errorf("login %s: response missing token", u.Email)
		}
	}
}

// ---------------------------------------------------------------------------
// Demo HTTP handler end-to-end: schema apply → seed users → serve demo app
// with API proxy. This is the closest we can get to testing `ayb demo <name>`
// without spawning a real process.
// ---------------------------------------------------------------------------

func TestDemoHTTPEndToEnd_Kanban(t *testing.T) {
	testDemoHTTPEndToEnd(t, "kanban")
}

func TestDemoHTTPEndToEnd_LivePolls(t *testing.T) {
	testDemoHTTPEndToEnd(t, "live-polls")
}

func testDemoHTTPEndToEnd(t *testing.T, demoName string) {
	ts := newDemoServer(t)
	defer ts.Close()

	// Apply schema.
	token := getAdminToken(t, ts.URL)
	schemaSQL, err := fs.ReadFile(examples.FS, demoName+"/schema.sql")
	testutil.NoError(t, err)
	resp, body := postAdminSQL(t, ts.URL, token, string(schemaSQL))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("schema apply failed: %v", body)
	}

	// Seed users.
	for _, u := range demoSeedUsers {
		regBody, err := json.Marshal(map[string]string{"email": u.Email, "password": u.Password})
		testutil.NoError(t, err)
		regResp, err := http.Post(ts.URL+"/api/auth/register", "application/json", bytes.NewReader(regBody))
		testutil.NoError(t, err)
		regResp.Body.Close()
	}

	// Create a demo app HTTP handler (same as serveDemoApp uses).
	distFS, err := examples.DemoDist(demoName)
	testutil.NoError(t, err)
	handler := demoFileHandler(distFS)
	demoTS := httptest.NewServer(handler)
	defer demoTS.Close()

	// Verify the demo app serves HTML.
	pageResp, err := http.Get(demoTS.URL + "/")
	testutil.NoError(t, err)
	defer pageResp.Body.Close()
	testutil.Equal(t, http.StatusOK, pageResp.StatusCode)
	pageBody, err := io.ReadAll(pageResp.Body)
	testutil.NoError(t, err)
	pageStr := strings.ToLower(string(pageBody))
	if !strings.Contains(pageStr, "<html") && !strings.Contains(pageStr, "<!doctype") {
		t.Error("demo app root does not serve HTML")
	}

	// Verify SPA fallback works for deep routes.
	spaResp, err := http.Get(demoTS.URL + "/boards/123")
	testutil.NoError(t, err)
	defer spaResp.Body.Close()
	testutil.Equal(t, http.StatusOK, spaResp.StatusCode)
}
