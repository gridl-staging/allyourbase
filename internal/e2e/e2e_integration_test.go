//go:build integration

// Package e2e tests AYB end-to-end via real HTTP against a live server.
// Every test starts a real httptest.Server backed by a live Postgres database.
// This validates the full user experience: HTTP routing, middleware, auth,
// realtime SSE, storage, batch operations, RPC, admin, and webhooks.
package e2e

import (
	"bufio"
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/migrations"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/server"
	"github.com/allyourbase/ayb/internal/storage"
	"github.com/allyourbase/ayb/internal/testutil"
)

var sharedPG *testutil.PGContainer

const (
	testJWTSecret = "e2e-integration-test-secret-that-is-at-least-32-chars!!"
	testSignKey   = "e2e-storage-sign-key-at-least-32-characters!!"
	testAdminPass = "test-admin-password-e2e"
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

func seedCRUDSchema(t *testing.T, ctx context.Context) {
	t.Helper()
	_, err := sharedPG.Pool.Exec(ctx, `
		CREATE TABLE authors (
			id   SERIAL PRIMARY KEY,
			name TEXT NOT NULL,
			bio  TEXT
		);
		CREATE TABLE posts (
			id         SERIAL PRIMARY KEY,
			title      TEXT NOT NULL,
			body       TEXT,
			published  BOOLEAN NOT NULL DEFAULT false,
			author_id  INTEGER REFERENCES authors(id),
			created_at TIMESTAMPTZ NOT NULL DEFAULT now()
		);
		CREATE TABLE tags (
			id   SERIAL PRIMARY KEY,
			name TEXT NOT NULL UNIQUE
		);
		INSERT INTO authors (name, bio) VALUES
			('Alice', 'Writer and thinker'),
			('Bob', 'Developer and blogger'),
			('Charlie', NULL);
		INSERT INTO posts (title, body, published, author_id) VALUES
			('Hello World', 'First post content', true, 1),
			('Draft Post', 'Not published yet', false, 1),
			('Go Tips', 'Concurrency patterns', true, 2),
			('Rust vs Go', 'A comparison', true, 2),
			('Solo Post', 'No author bio needed', true, 3);
		INSERT INTO tags (name) VALUES ('go'), ('rust'), ('web');
	`)
	testutil.NoError(t, err)
}

func newCRUDServer(t *testing.T) *httptest.Server {
	t.Helper()
	ctx := context.Background()
	resetDB(t, ctx)
	seedCRUDSchema(t, ctx)

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	testutil.NoError(t, ch.Load(ctx))

	cfg := config.Default()
	cfg.Admin.Password = testAdminPass
	srv := server.New(cfg, logger, ch, sharedPG.Pool, nil, nil)
	return httptest.NewServer(srv.Router())
}

func newFullServer(t *testing.T) *httptest.Server {
	t.Helper()
	ctx := context.Background()
	resetDB(t, ctx)
	runMigrations(t, ctx)
	seedCRUDSchema(t, ctx)

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	testutil.NoError(t, ch.Load(ctx))

	cfg := config.Default()
	cfg.Auth.Enabled = true
	cfg.Auth.JWTSecret = testJWTSecret
	cfg.Storage.Enabled = true
	cfg.Admin.Password = testAdminPass

	authSvc := auth.NewService(sharedPG.Pool, testJWTSecret, 15*time.Minute, 7*24*time.Hour, 8, logger)
	dir := t.TempDir()
	backend, err := storage.NewLocalBackend(dir)
	testutil.NoError(t, err)
	storageSvc := storage.NewService(sharedPG.Pool, backend, testSignKey, logger)

	srv := server.New(cfg, logger, ch, sharedPG.Pool, authSvc, storageSvc)
	return httptest.NewServer(srv.Router())
}

func httpJSON(t *testing.T, method, url string, body any, token string) (*http.Response, map[string]any) {
	t.Helper()
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		testutil.NoError(t, err)
		reqBody = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, url, reqBody)
	testutil.NoError(t, err)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	testutil.NoError(t, err)
	raw, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	testutil.NoError(t, err)
	if len(raw) == 0 {
		return resp, nil
	}
	var result map[string]any
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatalf("httpJSON: failed to parse response body as JSON object: %v\nbody: %s", err, string(raw))
	}
	return resp, result
}

func httpJSONArray(t *testing.T, method, url string, body any, token string) (*http.Response, []any) {
	t.Helper()
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		testutil.NoError(t, err)
		reqBody = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, url, reqBody)
	testutil.NoError(t, err)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	testutil.NoError(t, err)
	raw, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	testutil.NoError(t, err)
	var result []any
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatalf("httpJSONArray: unmarshal response: %v (raw: %q)", err, raw)
	}
	return resp, result
}

func httpRaw(t *testing.T, method, url string, body any, token string) (*http.Response, []byte) {
	t.Helper()
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		testutil.NoError(t, err)
		reqBody = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, url, reqBody)
	testutil.NoError(t, err)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	testutil.NoError(t, err)
	raw, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	testutil.NoError(t, err)
	return resp, raw
}

func adminToken(t *testing.T, baseURL string) string {
	t.Helper()
	resp, body := httpJSON(t, "POST", baseURL+"/api/admin/auth",
		map[string]string{"password": testAdminPass}, "")
	testutil.StatusCode(t, http.StatusOK, resp.StatusCode)
	token, ok := body["token"].(string)
	testutil.True(t, ok, "admin token should be a string")
	return token
}

func registerUser(t *testing.T, baseURL, email, password string) (string, string) {
	t.Helper()
	resp, body := httpJSON(t, "POST", baseURL+"/api/auth/register",
		map[string]string{"email": email, "password": password}, "")
	testutil.StatusCode(t, http.StatusCreated, resp.StatusCode)
	return body["token"].(string), body["refreshToken"].(string)
}

func loginUser(t *testing.T, baseURL, email, password string) (string, string) {
	t.Helper()
	resp, body := httpJSON(t, "POST", baseURL+"/api/auth/login",
		map[string]string{"email": email, "password": password}, "")
	testutil.StatusCode(t, http.StatusOK, resp.StatusCode)
	return body["token"].(string), body["refreshToken"].(string)
}

// ---------------------------------------------------------------------------
// 1. HEALTH CHECK
// ---------------------------------------------------------------------------

func TestE2E_HealthCheck(t *testing.T) {
	ts := newCRUDServer(t)
	defer ts.Close()

	resp, body := httpJSON(t, "GET", ts.URL+"/health", nil, "")
	testutil.StatusCode(t, http.StatusOK, resp.StatusCode)
	testutil.Equal(t, "ok", body["status"].(string))
}

// ---------------------------------------------------------------------------
// 2. SCHEMA ENDPOINT
// ---------------------------------------------------------------------------

func TestE2E_SchemaEndpoint(t *testing.T) {
	ts := newCRUDServer(t)
	defer ts.Close()

	resp, body := httpJSON(t, "GET", ts.URL+"/api/schema", nil, "")
	testutil.StatusCode(t, http.StatusOK, resp.StatusCode)

	// tables is a map[string]*Table keyed by "schema.table".
	tables, ok := body["tables"].(map[string]any)
	testutil.True(t, ok, "schema should have tables map")
	testutil.True(t, len(tables) >= 3, "should have at least authors, posts, tags")

	// Keys are "public.authors", "public.posts", etc.
	testutil.NotNil(t, tables["public.authors"])
	testutil.NotNil(t, tables["public.posts"])
	testutil.NotNil(t, tables["public.tags"])
}

// ---------------------------------------------------------------------------
// 3. ADMIN AUTH + SQL
// ---------------------------------------------------------------------------

func TestE2E_AdminAuth(t *testing.T) {
	ts := newCRUDServer(t)
	defer ts.Close()

	t.Run("status shows auth required", func(t *testing.T) {
		resp, body := httpJSON(t, "GET", ts.URL+"/api/admin/status", nil, "")
		testutil.StatusCode(t, http.StatusOK, resp.StatusCode)
		testutil.Equal(t, true, body["auth"].(bool))
	})

	t.Run("wrong password rejected", func(t *testing.T) {
		resp, _ := httpJSON(t, "POST", ts.URL+"/api/admin/auth",
			map[string]string{"password": "wrong"}, "")
		testutil.StatusCode(t, http.StatusUnauthorized, resp.StatusCode)
	})

	t.Run("correct password returns token", func(t *testing.T) {
		token := adminToken(t, ts.URL)
		testutil.True(t, len(token) > 10, "admin token should be substantial")
	})

	t.Run("SQL requires admin token", func(t *testing.T) {
		resp, _ := httpJSON(t, "POST", ts.URL+"/api/admin/sql/",
			map[string]string{"query": "SELECT 1"}, "")
		testutil.StatusCode(t, http.StatusUnauthorized, resp.StatusCode)
	})

	t.Run("SQL executes query", func(t *testing.T) {
		token := adminToken(t, ts.URL)
		resp, body := httpJSON(t, "POST", ts.URL+"/api/admin/sql/",
			map[string]string{"query": "SELECT name FROM authors ORDER BY id"},
			token)
		testutil.StatusCode(t, http.StatusOK, resp.StatusCode)
		columns := body["columns"].([]any)
		testutil.Equal(t, 1, len(columns))
		testutil.Equal(t, "name", columns[0].(string))
		rows := body["rows"].([]any)
		testutil.Equal(t, 3, len(rows))
		testutil.Equal(t, "Alice", rows[0].([]any)[0].(string))
	})

	t.Run("SQL error for bad query", func(t *testing.T) {
		token := adminToken(t, ts.URL)
		resp, body := httpJSON(t, "POST", ts.URL+"/api/admin/sql/",
			map[string]string{"query": "SELECT * FROM nonexistent_table"}, token)
		testutil.StatusCode(t, http.StatusBadRequest, resp.StatusCode)
		testutil.True(t, len(body["message"].(string)) > 0, "should have error message")
	})
}

// ---------------------------------------------------------------------------
// 4. CRUD LIFECYCLE (no auth)
// ---------------------------------------------------------------------------

func TestE2E_CRUDLifecycle(t *testing.T) {
	ts := newCRUDServer(t)
	defer ts.Close()

	t.Run("list authors", func(t *testing.T) {
		resp, body := httpJSON(t, "GET", ts.URL+"/api/collections/authors", nil, "")
		testutil.StatusCode(t, http.StatusOK, resp.StatusCode)
		items := body["items"].([]any)
		testutil.Equal(t, 3, len(items))
		testutil.Equal(t, float64(3), body["totalItems"].(float64))
	})

	t.Run("create author", func(t *testing.T) {
		resp, body := httpJSON(t, "POST", ts.URL+"/api/collections/authors",
			map[string]any{"name": "Diana", "bio": "New author"}, "")
		testutil.StatusCode(t, http.StatusCreated, resp.StatusCode)
		testutil.Equal(t, "Diana", body["name"].(string))
		testutil.NotNil(t, body["id"])
	})

	t.Run("get single author", func(t *testing.T) {
		resp, body := httpJSON(t, "GET", ts.URL+"/api/collections/authors/1", nil, "")
		testutil.StatusCode(t, http.StatusOK, resp.StatusCode)
		testutil.Equal(t, "Alice", body["name"].(string))
	})

	t.Run("update author", func(t *testing.T) {
		resp, body := httpJSON(t, "PATCH", ts.URL+"/api/collections/authors/1",
			map[string]any{"bio": "Updated bio"}, "")
		testutil.StatusCode(t, http.StatusOK, resp.StatusCode)
		testutil.Equal(t, "Updated bio", body["bio"].(string))
		testutil.Equal(t, "Alice", body["name"].(string))
	})

	t.Run("delete author", func(t *testing.T) {
		_, createBody := httpJSON(t, "POST", ts.URL+"/api/collections/authors",
			map[string]any{"name": "Ephemeral"}, "")
		id := fmt.Sprintf("%.0f", createBody["id"].(float64))

		resp, _ := httpJSON(t, "DELETE", ts.URL+"/api/collections/authors/"+id, nil, "")
		testutil.StatusCode(t, http.StatusNoContent, resp.StatusCode)

		resp2, _ := httpJSON(t, "GET", ts.URL+"/api/collections/authors/"+id, nil, "")
		testutil.StatusCode(t, http.StatusNotFound, resp2.StatusCode)
	})

	t.Run("404 nonexistent record", func(t *testing.T) {
		resp, body := httpJSON(t, "GET", ts.URL+"/api/collections/authors/99999", nil, "")
		testutil.StatusCode(t, http.StatusNotFound, resp.StatusCode)
		testutil.Equal(t, float64(404), body["code"].(float64))
	})

	t.Run("404 nonexistent table", func(t *testing.T) {
		resp, _ := httpJSON(t, "GET", ts.URL+"/api/collections/nonexistent", nil, "")
		testutil.StatusCode(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Run("NOT NULL violation", func(t *testing.T) {
		resp, body := httpJSON(t, "POST", ts.URL+"/api/collections/authors",
			map[string]any{"bio": "no name"}, "")
		// Missing NOT NULL column returns 400 (bad request) or 422 depending on driver.
		testutil.True(t, resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusUnprocessableEntity,
			"expected 400 or 422 for NOT NULL violation")
		testutil.NotNil(t, body["code"])
	})

	t.Run("UNIQUE violation 409", func(t *testing.T) {
		resp, body := httpJSON(t, "POST", ts.URL+"/api/collections/tags",
			map[string]any{"name": "go"}, "")
		testutil.StatusCode(t, http.StatusConflict, resp.StatusCode)
		testutil.Equal(t, float64(409), body["code"].(float64))
	})
}

// ---------------------------------------------------------------------------
// 5. QUERY FEATURES
// ---------------------------------------------------------------------------

func TestE2E_QueryFeatures(t *testing.T) {
	ts := newCRUDServer(t)
	defer ts.Close()

	t.Run("filter", func(t *testing.T) {
		resp, body := httpJSON(t, "GET",
			ts.URL+"/api/collections/posts?filter=published%3Dtrue", nil, "")
		testutil.StatusCode(t, http.StatusOK, resp.StatusCode)
		items := body["items"].([]any)
		testutil.Equal(t, 4, len(items)) // Hello World, Go Tips, Rust vs Go, Solo Post
	})

	t.Run("sort ascending", func(t *testing.T) {
		resp, body := httpJSON(t, "GET",
			ts.URL+"/api/collections/authors?sort=%2Bname", nil, "")
		testutil.StatusCode(t, http.StatusOK, resp.StatusCode)
		items := body["items"].([]any)
		testutil.Equal(t, "Alice", items[0].(map[string]any)["name"].(string))
		testutil.Equal(t, "Charlie", items[2].(map[string]any)["name"].(string))
	})

	t.Run("sort descending", func(t *testing.T) {
		resp, body := httpJSON(t, "GET",
			ts.URL+"/api/collections/authors?sort=-name", nil, "")
		testutil.StatusCode(t, http.StatusOK, resp.StatusCode)
		testutil.Equal(t, "Charlie", body["items"].([]any)[0].(map[string]any)["name"].(string))
	})

	t.Run("pagination", func(t *testing.T) {
		resp, body := httpJSON(t, "GET",
			ts.URL+"/api/collections/posts?perPage=2&page=1&sort=%2Bid", nil, "")
		testutil.StatusCode(t, http.StatusOK, resp.StatusCode)
		testutil.Equal(t, 2, len(body["items"].([]any)))
		testutil.Equal(t, float64(5), body["totalItems"].(float64))
		testutil.Equal(t, float64(3), body["totalPages"].(float64))

		resp2, body2 := httpJSON(t, "GET",
			ts.URL+"/api/collections/posts?perPage=2&page=3&sort=%2Bid", nil, "")
		testutil.StatusCode(t, http.StatusOK, resp2.StatusCode)
		testutil.Equal(t, 1, len(body2["items"].([]any)))
	})

	t.Run("fields projection", func(t *testing.T) {
		resp, body := httpJSON(t, "GET",
			ts.URL+"/api/collections/authors?fields=id,name", nil, "")
		testutil.StatusCode(t, http.StatusOK, resp.StatusCode)
		first := body["items"].([]any)[0].(map[string]any)
		testutil.NotNil(t, first["id"])
		testutil.NotNil(t, first["name"])
		_, hasBio := first["bio"]
		testutil.False(t, hasBio)
	})

	t.Run("expand FK", func(t *testing.T) {
		resp, body := httpJSON(t, "GET",
			ts.URL+"/api/collections/posts/1?expand=author_id", nil, "")
		testutil.StatusCode(t, http.StatusOK, resp.StatusCode)
		expand := body["expand"].(map[string]any)
		// Expand key uses friendly name (strips _id suffix).
		author := expand["author"].(map[string]any)
		testutil.Equal(t, "Alice", author["name"].(string))
	})

	t.Run("skipTotal", func(t *testing.T) {
		resp, body := httpJSON(t, "GET",
			ts.URL+"/api/collections/posts?skipTotal=true", nil, "")
		testutil.StatusCode(t, http.StatusOK, resp.StatusCode)
		testutil.Equal(t, float64(-1), body["totalItems"].(float64))
	})

	t.Run("invalid filter 400", func(t *testing.T) {
		resp, body := httpJSON(t, "GET",
			ts.URL+"/api/collections/posts?filter="+url.QueryEscape("((broken"), nil, "")
		testutil.StatusCode(t, http.StatusBadRequest, resp.StatusCode)
		code, ok := body["code"].(float64)
		testutil.True(t, ok, "response should have numeric code field")
		testutil.Equal(t, float64(400), code)
	})
}

// ---------------------------------------------------------------------------
// 6. BATCH OPERATIONS
// ---------------------------------------------------------------------------

func TestE2E_BatchOperations(t *testing.T) {
	ts := newCRUDServer(t)
	defer ts.Close()

	t.Run("mixed create/update/delete", func(t *testing.T) {
		_, createBody := httpJSON(t, "POST", ts.URL+"/api/collections/tags",
			map[string]any{"name": "batch-test"}, "")
		tagID := fmt.Sprintf("%.0f", createBody["id"].(float64))

		ops := map[string]any{
			"operations": []map[string]any{
				{"method": "create", "body": map[string]any{"name": "batch-new"}},
				{"method": "update", "id": tagID, "body": map[string]any{"name": "batch-updated"}},
				{"method": "delete", "id": tagID},
			},
		}
		resp, results := httpJSONArray(t, "POST", ts.URL+"/api/collections/tags/batch", ops, "")
		testutil.StatusCode(t, http.StatusOK, resp.StatusCode)
		testutil.Equal(t, 3, len(results))

		testutil.Equal(t, float64(201), results[0].(map[string]any)["status"].(float64))
		testutil.Equal(t, float64(200), results[1].(map[string]any)["status"].(float64))
		testutil.Equal(t, float64(204), results[2].(map[string]any)["status"].(float64))
	})

	t.Run("constraint error rolls back all", func(t *testing.T) {
		_, before := httpJSON(t, "GET", ts.URL+"/api/collections/tags", nil, "")
		beforeCount := before["totalItems"].(float64)

		ops := map[string]any{
			"operations": []map[string]any{
				{"method": "create", "body": map[string]any{"name": "batch-ok-1"}},
				{"method": "create", "body": map[string]any{"name": "go"}}, // duplicate
			},
		}
		resp, _ := httpRaw(t, "POST", ts.URL+"/api/collections/tags/batch", ops, "")
		testutil.True(t, resp.StatusCode >= 400, "batch should fail on constraint violation")

		_, after := httpJSON(t, "GET", ts.URL+"/api/collections/tags", nil, "")
		testutil.Equal(t, beforeCount, after["totalItems"].(float64))
	})
}

// ---------------------------------------------------------------------------
// 7. RPC
// ---------------------------------------------------------------------------

func TestE2E_RPC(t *testing.T) {
	ctx := context.Background()

	t.Run("void function 204", func(t *testing.T) {
		resetDB(t, ctx)
		seedCRUDSchema(t, ctx)
		_, err := sharedPG.Pool.Exec(ctx, `
			CREATE OR REPLACE FUNCTION do_nothing() RETURNS void LANGUAGE sql AS $$ SELECT $$;
		`)
		testutil.NoError(t, err)

		logger := testutil.DiscardLogger()
		ch := schema.NewCacheHolder(sharedPG.Pool, logger)
		testutil.NoError(t, ch.Load(ctx))
		cfg := config.Default()
		cfg.Admin.Password = testAdminPass
		srv := server.New(cfg, logger, ch, sharedPG.Pool, nil, nil)
		ts := httptest.NewServer(srv.Router())
		defer ts.Close()

		resp, _ := httpRaw(t, "POST", ts.URL+"/api/rpc/do_nothing", map[string]any{}, "")
		testutil.StatusCode(t, http.StatusNoContent, resp.StatusCode)
	})

	t.Run("scalar function returns value", func(t *testing.T) {
		resetDB(t, ctx)
		seedCRUDSchema(t, ctx)
		_, err := sharedPG.Pool.Exec(ctx, `
			CREATE OR REPLACE FUNCTION add_numbers(a integer, b integer) RETURNS integer
			LANGUAGE sql AS $$ SELECT a + b $$;
		`)
		testutil.NoError(t, err)

		logger := testutil.DiscardLogger()
		ch := schema.NewCacheHolder(sharedPG.Pool, logger)
		testutil.NoError(t, ch.Load(ctx))
		cfg := config.Default()
		cfg.Admin.Password = testAdminPass
		srv := server.New(cfg, logger, ch, sharedPG.Pool, nil, nil)
		ts := httptest.NewServer(srv.Router())
		defer ts.Close()

		resp, raw := httpRaw(t, "POST", ts.URL+"/api/rpc/add_numbers",
			map[string]any{"a": 3, "b": 7}, "")
		testutil.StatusCode(t, http.StatusOK, resp.StatusCode)
		testutil.Equal(t, "10", strings.TrimSpace(string(raw)))
	})

	t.Run("set-returning function returns array", func(t *testing.T) {
		resetDB(t, ctx)
		seedCRUDSchema(t, ctx)
		_, err := sharedPG.Pool.Exec(ctx, `
			CREATE OR REPLACE FUNCTION get_author_names()
			RETURNS TABLE(name text) LANGUAGE sql AS $$
				SELECT name FROM authors ORDER BY id
			$$;
		`)
		testutil.NoError(t, err)

		logger := testutil.DiscardLogger()
		ch := schema.NewCacheHolder(sharedPG.Pool, logger)
		testutil.NoError(t, ch.Load(ctx))
		cfg := config.Default()
		cfg.Admin.Password = testAdminPass
		srv := server.New(cfg, logger, ch, sharedPG.Pool, nil, nil)
		ts := httptest.NewServer(srv.Router())
		defer ts.Close()

		resp, results := httpJSONArray(t, "POST", ts.URL+"/api/rpc/get_author_names",
			map[string]any{}, "")
		testutil.StatusCode(t, http.StatusOK, resp.StatusCode)
		testutil.Equal(t, 3, len(results))
		testutil.Equal(t, "Alice", results[0].(map[string]any)["name"].(string))
	})
}

// ---------------------------------------------------------------------------
// 8. AUTH LIFECYCLE
// ---------------------------------------------------------------------------

func TestE2E_AuthLifecycle(t *testing.T) {
	ts := newFullServer(t)
	defer ts.Close()

	var token, refreshToken string

	t.Run("register", func(t *testing.T) {
		token, refreshToken = registerUser(t, ts.URL, "test@example.com", "securepass123")
	})

	t.Run("duplicate registration 409", func(t *testing.T) {
		resp, body := httpJSON(t, "POST", ts.URL+"/api/auth/register",
			map[string]string{"email": "test@example.com", "password": "securepass123"}, "")
		testutil.StatusCode(t, http.StatusConflict, resp.StatusCode)
		testutil.Equal(t, float64(409), body["code"].(float64))
	})

	t.Run("get me", func(t *testing.T) {
		resp, body := httpJSON(t, "GET", ts.URL+"/api/auth/me", nil, token)
		testutil.StatusCode(t, http.StatusOK, resp.StatusCode)
		testutil.Equal(t, "test@example.com", body["email"].(string))
		testutil.NotNil(t, body["id"])
	})

	t.Run("me without token 401", func(t *testing.T) {
		resp, _ := httpJSON(t, "GET", ts.URL+"/api/auth/me", nil, "")
		testutil.StatusCode(t, http.StatusUnauthorized, resp.StatusCode)
	})

	t.Run("me with bad token 401", func(t *testing.T) {
		resp, _ := httpJSON(t, "GET", ts.URL+"/api/auth/me", nil, "invalid.token")
		testutil.StatusCode(t, http.StatusUnauthorized, resp.StatusCode)
	})

	t.Run("login", func(t *testing.T) {
		newToken, newRefresh := loginUser(t, ts.URL, "test@example.com", "securepass123")
		testutil.True(t, len(newToken) > 0, "should get token")
		token = newToken
		refreshToken = newRefresh
	})

	t.Run("login wrong password", func(t *testing.T) {
		resp, _ := httpJSON(t, "POST", ts.URL+"/api/auth/login",
			map[string]string{"email": "test@example.com", "password": "wrong"}, "")
		testutil.StatusCode(t, http.StatusUnauthorized, resp.StatusCode)
	})

	t.Run("refresh token", func(t *testing.T) {
		resp, body := httpJSON(t, "POST", ts.URL+"/api/auth/refresh",
			map[string]string{"refreshToken": refreshToken}, "")
		testutil.StatusCode(t, http.StatusOK, resp.StatusCode)
		token = body["token"].(string)
		refreshToken = body["refreshToken"].(string)
		testutil.True(t, len(token) > 0, "new token")
	})

	t.Run("logout", func(t *testing.T) {
		resp, _ := httpJSON(t, "POST", ts.URL+"/api/auth/logout",
			map[string]string{"refreshToken": refreshToken}, "")
		testutil.StatusCode(t, http.StatusNoContent, resp.StatusCode)

		resp2, _ := httpJSON(t, "POST", ts.URL+"/api/auth/refresh",
			map[string]string{"refreshToken": refreshToken}, "")
		testutil.True(t, resp2.StatusCode >= 400, "refresh after logout should fail")
	})
}

// ---------------------------------------------------------------------------
// 9. AUTH-GATED CRUD
// ---------------------------------------------------------------------------

func TestE2E_AuthProtectedCRUD(t *testing.T) {
	ts := newFullServer(t)
	defer ts.Close()

	t.Run("requires auth", func(t *testing.T) {
		resp, body := httpJSON(t, "GET", ts.URL+"/api/collections/authors", nil, "")
		testutil.StatusCode(t, http.StatusUnauthorized, resp.StatusCode)
		testutil.Equal(t, float64(401), body["code"].(float64))
	})

	t.Run("works with token", func(t *testing.T) {
		token, _ := registerUser(t, ts.URL, "crud@test.com", "password123")
		resp, body := httpJSON(t, "GET", ts.URL+"/api/collections/authors", nil, token)
		testutil.StatusCode(t, http.StatusOK, resp.StatusCode)
		testutil.NotNil(t, body["items"])
	})
}

// ---------------------------------------------------------------------------
// 10. STORAGE
// ---------------------------------------------------------------------------

func TestE2E_Storage(t *testing.T) {
	ts := newFullServer(t)
	defer ts.Close()

	token, _ := registerUser(t, ts.URL, "storage@test.com", "password123")
	var uploadedName string

	t.Run("upload", func(t *testing.T) {
		body := &bytes.Buffer{}
		w := multipart.NewWriter(body)
		fw, err := w.CreateFormFile("file", "test-doc.txt")
		testutil.NoError(t, err)
		fw.Write([]byte("Hello from E2E test!"))
		w.Close()

		req, _ := http.NewRequest("POST", ts.URL+"/api/storage/testbucket", body)
		req.Header.Set("Content-Type", w.FormDataContentType())
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := http.DefaultClient.Do(req)
		testutil.NoError(t, err)
		testutil.StatusCode(t, http.StatusCreated, resp.StatusCode)
		var result map[string]any
		json.NewDecoder(resp.Body).Decode(&result)
		resp.Body.Close()
		uploadedName = result["name"].(string)
		testutil.True(t, len(uploadedName) > 0, "should have name")
	})

	t.Run("list files", func(t *testing.T) {
		resp, body := httpJSON(t, "GET", ts.URL+"/api/storage/testbucket", nil, token)
		testutil.StatusCode(t, http.StatusOK, resp.StatusCode)
		testutil.Equal(t, 1, len(body["items"].([]any)))
	})

	t.Run("download", func(t *testing.T) {
		resp, err := http.Get(ts.URL + "/api/storage/testbucket/" + uploadedName)
		testutil.NoError(t, err)
		data, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		testutil.StatusCode(t, http.StatusOK, resp.StatusCode)
		testutil.Equal(t, "Hello from E2E test!", string(data))
	})

	t.Run("signed URL", func(t *testing.T) {
		resp, body := httpJSON(t, "POST",
			ts.URL+"/api/storage/testbucket/"+uploadedName+"/sign",
			map[string]any{"expiresIn": 3600}, token)
		testutil.StatusCode(t, http.StatusOK, resp.StatusCode)
		testutil.Contains(t, body["url"].(string), "sig=")
	})

	t.Run("delete", func(t *testing.T) {
		req, _ := http.NewRequest("DELETE", ts.URL+"/api/storage/testbucket/"+uploadedName, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := http.DefaultClient.Do(req)
		testutil.NoError(t, err)
		resp.Body.Close()
		testutil.StatusCode(t, http.StatusNoContent, resp.StatusCode)
	})
}

// ---------------------------------------------------------------------------
// 11. REALTIME SSE
// ---------------------------------------------------------------------------

func TestE2E_RealtimeSSE(t *testing.T) {
	ts := newCRUDServer(t)
	defer ts.Close()

	t.Run("receives create event", func(t *testing.T) {
		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Get(ts.URL + "/api/realtime?tables=authors")
		testutil.NoError(t, err)
		defer resp.Body.Close()
		testutil.StatusCode(t, http.StatusOK, resp.StatusCode)
		testutil.Equal(t, "text/event-stream", resp.Header.Get("Content-Type"))

		scanner := bufio.NewScanner(resp.Body)
		var connected []string
		for scanner.Scan() {
			line := scanner.Text()
			connected = append(connected, line)
			if line == "" && len(connected) > 1 {
				break
			}
		}
		testutil.Equal(t, "event: connected", connected[0])

		if resp, err := http.Post(ts.URL+"/api/collections/authors", "application/json",
			bytes.NewReader([]byte(`{"name":"SSE Author"}`))); err == nil {
			resp.Body.Close()
		}

		eventCh := make(chan string, 1)
		go func() {
			var lines []string
			for scanner.Scan() {
				line := scanner.Text()
				lines = append(lines, line)
				if line == "" && len(lines) > 1 {
					break
				}
			}
			eventCh <- strings.Join(lines, "\n")
		}()

		select {
		case event := <-eventCh:
			testutil.Contains(t, event, `"action":"create"`)
			testutil.Contains(t, event, `"table":"authors"`)
			testutil.Contains(t, event, `SSE Author`)
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for SSE event")
		}
	})

	t.Run("table filtering", func(t *testing.T) {
		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Get(ts.URL + "/api/realtime?tables=tags")
		testutil.NoError(t, err)
		defer resp.Body.Close()

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			if scanner.Text() == "" {
				break
			}
		}

		// Create in authors (not subscribed).
		if resp, err := http.Post(ts.URL+"/api/collections/authors", "application/json",
			bytes.NewReader([]byte(`{"name":"Ignored"}`))); err == nil {
			resp.Body.Close()
		}
		// Create in tags (subscribed).
		if resp, err := http.Post(ts.URL+"/api/collections/tags", "application/json",
			bytes.NewReader([]byte(`{"name":"sse-filter"}`))); err == nil {
			resp.Body.Close()
		}

		eventCh := make(chan string, 1)
		go func() {
			var lines []string
			for scanner.Scan() {
				line := scanner.Text()
				lines = append(lines, line)
				if line == "" && len(lines) > 1 {
					break
				}
			}
			eventCh <- strings.Join(lines, "\n")
		}()

		select {
		case event := <-eventCh:
			testutil.Contains(t, event, `"table":"tags"`)
			testutil.Contains(t, event, `sse-filter`)
		case <-time.After(5 * time.Second):
			t.Fatal("timed out")
		}
	})
}

// ---------------------------------------------------------------------------
// 12. WEBHOOK DELIVERY + HMAC
// ---------------------------------------------------------------------------

func TestE2E_WebhookDelivery(t *testing.T) {
	ctx := context.Background()
	resetDB(t, ctx)
	runMigrations(t, ctx)
	seedCRUDSchema(t, ctx)

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	testutil.NoError(t, ch.Load(ctx))

	cfg := config.Default()
	cfg.Admin.Password = testAdminPass
	srv := server.New(cfg, logger, ch, sharedPG.Pool, nil, nil)
	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	webhookSecret := "test-webhook-secret-for-hmac"

	var (
		mu      sync.Mutex
		payload []byte
		sig     string
	)
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		payload, _ = io.ReadAll(r.Body)
		sig = r.Header.Get("X-AYB-Signature")
		w.WriteHeader(http.StatusOK)
	}))
	defer receiver.Close()

	adminTok := adminToken(t, ts.URL)
	resp, body := httpJSON(t, "POST", ts.URL+"/api/webhooks",
		map[string]any{
			"url":     receiver.URL,
			"secret":  webhookSecret,
			"events":  []string{"create"},
			"tables":  []string{"authors"},
			"enabled": true,
		}, adminTok)
	testutil.StatusCode(t, http.StatusCreated, resp.StatusCode)
	testutil.NotNil(t, body["id"])

	// Trigger.
	createResp, _ := httpJSON(t, "POST", ts.URL+"/api/collections/authors",
		map[string]any{"name": "Webhook Test"}, "")
	testutil.StatusCode(t, http.StatusCreated, createResp.StatusCode)

	// Wait for async delivery.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		p := payload
		mu.Unlock()
		if p != nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	mu.Lock()
	testutil.NotNil(t, payload)
	var webhookBody map[string]any
	testutil.NoError(t, json.Unmarshal(payload, &webhookBody))
	testutil.Equal(t, "create", webhookBody["action"].(string))
	testutil.Equal(t, "authors", webhookBody["table"].(string))
	testutil.Equal(t, "Webhook Test", webhookBody["record"].(map[string]any)["name"].(string))

	// Verify HMAC.
	mac := hmac.New(sha256.New, []byte(webhookSecret))
	mac.Write(payload)
	expectedSig := hex.EncodeToString(mac.Sum(nil))
	testutil.Equal(t, expectedSig, sig)
	mu.Unlock()
}

// ---------------------------------------------------------------------------
// 13. CONCURRENT SSE CLIENTS
// ---------------------------------------------------------------------------

func TestE2E_ConcurrentSSEClients(t *testing.T) {
	ts := newCRUDServer(t)
	defer ts.Close()

	const numClients = 3
	events := make([]chan string, numClients)

	for i := range events {
		events[i] = make(chan string, 1)
		go func(ch chan string) {
			client := &http.Client{Timeout: 10 * time.Second}
			resp, err := client.Get(ts.URL + "/api/realtime?tables=authors")
			if err != nil {
				ch <- "error: " + err.Error()
				return
			}
			defer resp.Body.Close()
			scanner := bufio.NewScanner(resp.Body)
			for scanner.Scan() {
				if scanner.Text() == "" {
					break
				}
			}
			var lines []string
			for scanner.Scan() {
				line := scanner.Text()
				lines = append(lines, line)
				if line == "" && len(lines) > 1 {
					break
				}
			}
			ch <- strings.Join(lines, "\n")
		}(events[i])
	}

	time.Sleep(300 * time.Millisecond)

	if resp, err := http.Post(ts.URL+"/api/collections/authors", "application/json",
		bytes.NewReader([]byte(`{"name":"Broadcast"}`))); err == nil {
		resp.Body.Close()
	}

	for i := range events {
		select {
		case event := <-events[i]:
			testutil.Contains(t, event, `Broadcast`)
		case <-time.After(5 * time.Second):
			t.Fatalf("client %d timed out", i)
		}
	}
}

// ---------------------------------------------------------------------------
// 14. FULL USER JOURNEY
// ---------------------------------------------------------------------------

func TestE2E_FullUserJourney(t *testing.T) {
	ts := newFullServer(t)
	defer ts.Close()

	// Health.
	resp, body := httpJSON(t, "GET", ts.URL+"/health", nil, "")
	testutil.StatusCode(t, http.StatusOK, resp.StatusCode)
	testutil.Equal(t, "ok", body["status"].(string))

	// Schema without auth should be 401.
	resp, _ = httpJSON(t, "GET", ts.URL+"/api/schema", nil, "")
	testutil.StatusCode(t, http.StatusUnauthorized, resp.StatusCode)

	// Register.
	token, _ := registerUser(t, ts.URL, "journey@test.com", "password123")

	// Schema with auth should succeed.
	resp, body = httpJSON(t, "GET", ts.URL+"/api/schema", nil, token)
	testutil.StatusCode(t, http.StatusOK, resp.StatusCode)
	testutil.NotNil(t, body["tables"])

	// Me.
	resp, body = httpJSON(t, "GET", ts.URL+"/api/auth/me", nil, token)
	testutil.StatusCode(t, http.StatusOK, resp.StatusCode)
	testutil.Equal(t, "journey@test.com", body["email"].(string))

	// Create author.
	resp, body = httpJSON(t, "POST", ts.URL+"/api/collections/authors",
		map[string]any{"name": "Journey Author", "bio": "E2E"}, token)
	testutil.StatusCode(t, http.StatusCreated, resp.StatusCode)
	authorID := fmt.Sprintf("%.0f", body["id"].(float64))

	// Create post linked to author.
	resp, body = httpJSON(t, "POST", ts.URL+"/api/collections/posts",
		map[string]any{
			"title": "Journey Post", "body": "E2E",
			"published": true, "author_id": body["id"],
		}, token)
	testutil.StatusCode(t, http.StatusCreated, resp.StatusCode)
	postID := fmt.Sprintf("%.0f", body["id"].(float64))

	// List with filter + expand.
	resp, body = httpJSON(t, "GET",
		ts.URL+"/api/collections/posts?filter=published%3Dtrue&expand=author_id&sort=-id",
		nil, token)
	testutil.StatusCode(t, http.StatusOK, resp.StatusCode)
	found := false
	for _, item := range body["items"].([]any) {
		m := item.(map[string]any)
		if m["title"] == "Journey Post" {
			found = true
			testutil.Equal(t, "Journey Author", m["expand"].(map[string]any)["author"].(map[string]any)["name"].(string))
		}
	}
	testutil.True(t, found, "should find Journey Post")

	// Update.
	resp, body = httpJSON(t, "PATCH", ts.URL+"/api/collections/posts/"+postID,
		map[string]any{"title": "Updated Journey"}, token)
	testutil.StatusCode(t, http.StatusOK, resp.StatusCode)
	testutil.Equal(t, "Updated Journey", body["title"].(string))

	// Upload file.
	fileBody := &bytes.Buffer{}
	w := multipart.NewWriter(fileBody)
	fw, _ := w.CreateFormFile("file", "journey.txt")
	fw.Write([]byte("journey content"))
	w.Close()
	req, _ := http.NewRequest("POST", ts.URL+"/api/storage/journey", fileBody)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)
	fileResp, err := http.DefaultClient.Do(req)
	testutil.NoError(t, err)
	fileResp.Body.Close()
	testutil.StatusCode(t, http.StatusCreated, fileResp.StatusCode)

	// Delete post.
	req, _ = http.NewRequest("DELETE", ts.URL+"/api/collections/posts/"+postID, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	delResp, err := http.DefaultClient.Do(req)
	testutil.NoError(t, err)
	delResp.Body.Close()
	testutil.StatusCode(t, http.StatusNoContent, delResp.StatusCode)

	// Confirm gone.
	resp, _ = httpJSON(t, "GET", ts.URL+"/api/collections/posts/"+postID, nil, token)
	testutil.StatusCode(t, http.StatusNotFound, resp.StatusCode)

	// Delete author.
	req, _ = http.NewRequest("DELETE", ts.URL+"/api/collections/authors/"+authorID, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	delResp, err = http.DefaultClient.Do(req)
	testutil.NoError(t, err)
	delResp.Body.Close()
	testutil.StatusCode(t, http.StatusNoContent, delResp.StatusCode)
}

// ---------------------------------------------------------------------------
// 15. WEBHOOK CRUD
// ---------------------------------------------------------------------------

func TestE2E_WebhookCRUD(t *testing.T) {
	ts := newFullServer(t)
	defer ts.Close()

	adminTok := adminToken(t, ts.URL)

	t.Run("create", func(t *testing.T) {
		resp, body := httpJSON(t, "POST", ts.URL+"/api/webhooks",
			map[string]any{
				"url": "https://example.com/hook", "events": []string{"create"},
				"tables": []string{"authors"}, "enabled": true,
			}, adminTok)
		testutil.StatusCode(t, http.StatusCreated, resp.StatusCode)
		testutil.NotNil(t, body["id"])
	})

	t.Run("list", func(t *testing.T) {
		resp, body := httpJSON(t, "GET", ts.URL+"/api/webhooks", nil, adminTok)
		testutil.StatusCode(t, http.StatusOK, resp.StatusCode)
		testutil.True(t, len(body["items"].([]any)) >= 1, "should have webhook")
	})

	t.Run("requires admin", func(t *testing.T) {
		resp, _ := httpJSON(t, "GET", ts.URL+"/api/webhooks", nil, "")
		testutil.StatusCode(t, http.StatusUnauthorized, resp.StatusCode)
	})
}

// ---------------------------------------------------------------------------
// 16. ERROR EDGE CASES
// ---------------------------------------------------------------------------

func TestE2E_ErrorHandling(t *testing.T) {
	ts := newCRUDServer(t)
	defer ts.Close()

	t.Run("PATCH nonexistent", func(t *testing.T) {
		resp, body := httpJSON(t, "PATCH", ts.URL+"/api/collections/authors/99999",
			map[string]any{"name": "ghost"}, "")
		testutil.StatusCode(t, http.StatusNotFound, resp.StatusCode)
		testutil.Equal(t, float64(404), body["code"].(float64))
	})

	t.Run("DELETE nonexistent", func(t *testing.T) {
		resp, _ := httpJSON(t, "DELETE", ts.URL+"/api/collections/authors/99999", nil, "")
		testutil.StatusCode(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Run("invalid JSON", func(t *testing.T) {
		req, _ := http.NewRequest("POST", ts.URL+"/api/collections/authors",
			bytes.NewReader([]byte(`{bad json`)))
		req.Header.Set("Content-Type", "application/json")
		resp, _ := http.DefaultClient.Do(req)
		resp.Body.Close()
		testutil.StatusCode(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("empty body NOT NULL", func(t *testing.T) {
		req, _ := http.NewRequest("POST", ts.URL+"/api/collections/authors",
			bytes.NewReader([]byte(`{}`)))
		req.Header.Set("Content-Type", "application/json")
		resp, _ := http.DefaultClient.Do(req)
		resp.Body.Close()
		// Empty body with missing NOT NULL fields returns 400 or 422.
		testutil.True(t, resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusUnprocessableEntity,
			"expected 400 or 422")
	})

	t.Run("wrong content type", func(t *testing.T) {
		req, _ := http.NewRequest("POST", ts.URL+"/api/collections/authors",
			bytes.NewReader([]byte(`name=test`)))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		resp, _ := http.DefaultClient.Do(req)
		resp.Body.Close()
		testutil.StatusCode(t, http.StatusUnsupportedMediaType, resp.StatusCode)
	})
}

// ---------------------------------------------------------------------------
// 17. REALTIME SSE WITH RLS FILTERING
// ---------------------------------------------------------------------------

func TestE2E_RealtimeSSEWithRLS(t *testing.T) {
	ctx := context.Background()
	resetDB(t, ctx)
	runMigrations(t, ctx)

	// Create a table with RLS policies.
	_, err := sharedPG.Pool.Exec(ctx, `
		CREATE TABLE messages (
			id SERIAL PRIMARY KEY,
			user_id TEXT NOT NULL,
			content TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now()
		);
		ALTER TABLE messages ENABLE ROW LEVEL SECURITY;
		ALTER TABLE messages FORCE ROW LEVEL SECURITY;
		CREATE POLICY messages_owner ON messages
			USING (user_id = current_setting('ayb.user_id', true));
	`)
	testutil.NoError(t, err)

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	testutil.NoError(t, ch.Load(ctx))

	cfg := config.Default()
	cfg.Auth.Enabled = true
	cfg.Auth.JWTSecret = testJWTSecret
	cfg.Admin.Password = testAdminPass

	authSvc := auth.NewService(sharedPG.Pool, testJWTSecret, 15*time.Minute, 7*24*time.Hour, 8, logger)
	srv := server.New(cfg, logger, ch, sharedPG.Pool, authSvc, nil)
	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	// Register two users.
	user1Token, _ := registerUser(t, ts.URL, "user1@example.com", "password123")
	user2Token, _ := registerUser(t, ts.URL, "user2@example.com", "password123")

	// Get user IDs from tokens.
	resp1, body1 := httpJSON(t, "GET", ts.URL+"/api/auth/me", nil, user1Token)
	testutil.StatusCode(t, http.StatusOK, resp1.StatusCode)
	user1ID := body1["id"].(string)

	resp2, body2 := httpJSON(t, "GET", ts.URL+"/api/auth/me", nil, user2Token)
	testutil.StatusCode(t, http.StatusOK, resp2.StatusCode)
	user2ID := body2["id"].(string)

	// User 1 subscribes to realtime with their auth token.
	client := &http.Client{Timeout: 10 * time.Second}
	req, _ := http.NewRequest("GET", ts.URL+"/api/realtime?tables=messages", nil)
	req.Header.Set("Authorization", "Bearer "+user1Token)
	sseResp, err := client.Do(req)
	testutil.NoError(t, err)
	defer sseResp.Body.Close()
	testutil.StatusCode(t, http.StatusOK, sseResp.StatusCode)

	// Skip "connected" event.
	scanner := bufio.NewScanner(sseResp.Body)
	for scanner.Scan() {
		if scanner.Text() == "" {
			break
		}
	}

	// User2 inserts via the API — this publishes a realtime event to the hub.
	// The RLS canSeeRecord check should block user1 from receiving this event.
	createResp, _ := httpJSON(t, "POST", ts.URL+"/api/collections/messages",
		map[string]any{"user_id": user2ID, "content": "user2 secret"}, user2Token)
	testutil.StatusCode(t, http.StatusCreated, createResp.StatusCode)

	// Small delay to let the hub broadcast + RLS filter run before user1's insert.
	time.Sleep(200 * time.Millisecond)

	// User1 inserts via the API — user1 should receive this event.
	createResp, _ = httpJSON(t, "POST", ts.URL+"/api/collections/messages",
		map[string]any{"user_id": user1ID, "content": "user1 visible"}, user1Token)
	testutil.StatusCode(t, http.StatusCreated, createResp.StatusCode)

	// Collect SSE events — we expect exactly user1's event, not user2's.
	eventCh := make(chan string, 1)
	go func() {
		var lines []string
		for scanner.Scan() {
			line := scanner.Text()
			lines = append(lines, line)
			if line == "" && len(lines) > 1 {
				break
			}
		}
		eventCh <- strings.Join(lines, "\n")
	}()

	// The first event user1 receives should be their own message (user2's was filtered by RLS).
	select {
	case event := <-eventCh:
		testutil.Contains(t, event, `"action":"create"`)
		testutil.Contains(t, event, `"table":"messages"`)
		testutil.Contains(t, event, `user1 visible`)
		// Must NOT contain user2's message — RLS canSeeRecord should have filtered it.
		testutil.True(t, !strings.Contains(event, "user2 secret"),
			"user1 must not receive user2's message — RLS filtering failed")
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for SSE event")
	}
}
