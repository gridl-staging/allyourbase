//go:build integration

package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

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

// resetAndSeedDB drops the public schema and recreates the test tables with seed data.
func resetAndSeedDB(t *testing.T, ctx context.Context) {
	t.Helper()

	_, err := sharedPG.Pool.Exec(ctx, "DROP SCHEMA public CASCADE; CREATE SCHEMA public")
	if err != nil {
		t.Fatalf("resetting schema: %v", err)
	}

	_, err = sharedPG.Pool.Exec(ctx, `
		CREATE TABLE authors (
			id SERIAL PRIMARY KEY,
			name TEXT NOT NULL
		);
		CREATE TABLE posts (
			id SERIAL PRIMARY KEY,
			title TEXT NOT NULL,
			body TEXT,
			author_id INTEGER REFERENCES authors(id),
			status TEXT DEFAULT 'draft',
			created_at TIMESTAMPTZ DEFAULT now()
		);
		CREATE TABLE tags (
			id SERIAL PRIMARY KEY,
			name TEXT NOT NULL UNIQUE
		);

		INSERT INTO authors (name) VALUES ('Alice'), ('Bob');
		INSERT INTO posts (title, body, author_id, status) VALUES
			('First Post', 'Hello world', 1, 'published'),
			('Second Post', 'Another post', 1, 'draft'),
			('Bob Post', 'By Bob', 2, 'published');
		INSERT INTO tags (name) VALUES ('go'), ('api'), ('test');
	`)
	if err != nil {
		t.Fatalf("creating test schema: %v", err)
	}
}

func setupTestServer(t *testing.T, ctx context.Context) (*server.Server, *testutil.PGContainer) {
	t.Helper()

	resetAndSeedDB(t, ctx)

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	if err := ch.Load(ctx); err != nil {
		t.Fatalf("loading schema cache: %v", err)
	}

	cfg := config.Default()
	srv := server.New(cfg, logger, ch, sharedPG.Pool, nil, nil)

	return srv, sharedPG
}

func doRequest(t *testing.T, srv *server.Server, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reqBody io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		reqBody = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, reqBody)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)
	return w
}

func parseJSON(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var result map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("parsing JSON response: %v\nbody: %s", err, w.Body.String())
	}
	return result
}

// jsonNum extracts a float64 from a JSON-decoded map value.
func jsonNum(t *testing.T, v any) float64 {
	t.Helper()
	f, ok := v.(float64)
	if !ok {
		t.Fatalf("expected float64, got %T: %v", v, v)
	}
	return f
}

func jsonStr(t *testing.T, v any) string {
	t.Helper()
	s, ok := v.(string)
	if !ok {
		t.Fatalf("expected string, got %T: %v", v, v)
	}
	return s
}

func jsonItems(t *testing.T, body map[string]any) []map[string]any {
	t.Helper()
	raw, ok := body["items"].([]any)
	if !ok {
		t.Fatalf("expected items array, got %T", body["items"])
	}
	items := make([]map[string]any, len(raw))
	for i, v := range raw {
		items[i] = v.(map[string]any)
	}
	return items
}

// --- List tests ---

func TestListRecords(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	w := doRequest(t, srv, "GET", "/api/collections/posts/", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	body := parseJSON(t, w)
	testutil.Equal(t, 1.0, jsonNum(t, body["page"]))
	testutil.Equal(t, 20.0, jsonNum(t, body["perPage"]))
	testutil.Equal(t, 3.0, jsonNum(t, body["totalItems"]))

	items := jsonItems(t, body)
	testutil.Equal(t, 3, len(items))
}

func TestListPagination(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	w := doRequest(t, srv, "GET", "/api/collections/posts/?page=1&perPage=2", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	body := parseJSON(t, w)
	testutil.Equal(t, 1.0, jsonNum(t, body["page"]))
	testutil.Equal(t, 2.0, jsonNum(t, body["perPage"]))
	testutil.Equal(t, 3.0, jsonNum(t, body["totalItems"]))
	testutil.Equal(t, 2.0, jsonNum(t, body["totalPages"]))

	items := jsonItems(t, body)
	testutil.Equal(t, 2, len(items))
}

func TestListPaginationPage2(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	w := doRequest(t, srv, "GET", "/api/collections/posts/?page=2&perPage=2&sort=id", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	body := parseJSON(t, w)
	testutil.Equal(t, 2.0, jsonNum(t, body["page"]))
	testutil.Equal(t, 3.0, jsonNum(t, body["totalItems"]))
	items := jsonItems(t, body)
	testutil.Equal(t, 1, len(items))
	// Page 2 with perPage=2 sorted by id should return the 3rd post (Bob Post).
	testutil.Equal(t, "Bob Post", jsonStr(t, items[0]["title"]))
}

func TestListSkipTotal(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	w := doRequest(t, srv, "GET", "/api/collections/posts/?skipTotal=true", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	body := parseJSON(t, w)
	testutil.Equal(t, -1.0, jsonNum(t, body["totalItems"]))
	testutil.Equal(t, -1.0, jsonNum(t, body["totalPages"]))
	// Verify items are still returned even when totals are skipped.
	items := jsonItems(t, body)
	testutil.Equal(t, 3, len(items))
}

func TestListWithSort(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	w := doRequest(t, srv, "GET", "/api/collections/posts/?sort=-id", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	body := parseJSON(t, w)
	items := jsonItems(t, body)
	testutil.Equal(t, 3, len(items))
	testutil.Equal(t, 3.0, jsonNum(t, items[0]["id"])) // highest ID first
	testutil.Equal(t, 2.0, jsonNum(t, items[1]["id"]))
	testutil.Equal(t, 1.0, jsonNum(t, items[2]["id"])) // lowest ID last
}

func TestListWithFields(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	w := doRequest(t, srv, "GET", "/api/collections/posts/?fields=id,title&sort=id", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	body := parseJSON(t, w)
	items := jsonItems(t, body)
	testutil.True(t, len(items) > 0, "expected items")
	first := items[0]
	testutil.Equal(t, 1.0, jsonNum(t, first["id"]))
	testutil.Equal(t, "First Post", jsonStr(t, first["title"]))
	_, hasBody := first["body"]
	testutil.False(t, hasBody, "body field should not be present")
	_, hasStatus := first["status"]
	testutil.False(t, hasStatus, "status field should not be present")
}

func TestListWithFilter(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	w := doRequest(t, srv, "GET", "/api/collections/posts/?filter=status%3D'published'", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	body := parseJSON(t, w)
	items := jsonItems(t, body)
	testutil.Equal(t, 2, len(items))
	// Verify every returned item actually has status=published.
	for _, item := range items {
		testutil.Equal(t, "published", jsonStr(t, item["status"]))
	}
}

func TestListWithFilterAnd(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	w := doRequest(t, srv, "GET", "/api/collections/posts/?filter=status%3D'published'+AND+author_id%3D1", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	body := parseJSON(t, w)
	items := jsonItems(t, body)
	testutil.Equal(t, 1, len(items))
	testutil.Equal(t, "First Post", jsonStr(t, items[0]["title"]))
	testutil.Equal(t, "published", jsonStr(t, items[0]["status"]))
	testutil.Equal(t, 1.0, jsonNum(t, items[0]["author_id"]))
}

func TestListInvalidFilter(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	w := doRequest(t, srv, "GET", "/api/collections/posts/?filter=nonexistent%3D'x'", nil)
	testutil.StatusCode(t, http.StatusBadRequest, w.Code)
}

func TestListCollectionNotFound(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	w := doRequest(t, srv, "GET", "/api/collections/nonexistent/", nil)
	testutil.StatusCode(t, http.StatusNotFound, w.Code)
}

// --- Read single record tests ---

func TestReadRecord(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	w := doRequest(t, srv, "GET", "/api/collections/posts/1", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	body := parseJSON(t, w)
	testutil.Equal(t, 1.0, jsonNum(t, body["id"]))
	testutil.Equal(t, "First Post", jsonStr(t, body["title"]))
}

func TestReadRecordNotFound(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	w := doRequest(t, srv, "GET", "/api/collections/posts/999", nil)
	testutil.StatusCode(t, http.StatusNotFound, w.Code)
}

func TestReadRecordWithFields(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	w := doRequest(t, srv, "GET", "/api/collections/posts/1?fields=id,title", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	body := parseJSON(t, w)
	testutil.Equal(t, 1.0, jsonNum(t, body["id"]))
	testutil.Equal(t, "First Post", jsonStr(t, body["title"]))
	_, hasBody := body["body"]
	testutil.False(t, hasBody, "body should not be present")
	_, hasStatus := body["status"]
	testutil.False(t, hasStatus, "status should not be present")
}

// --- Create tests ---

func TestCreateRecord(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	data := map[string]any{"name": "Charlie"}
	w := doRequest(t, srv, "POST", "/api/collections/authors/", data)
	testutil.StatusCode(t, http.StatusCreated, w.Code)

	body := parseJSON(t, w)
	testutil.Equal(t, "Charlie", jsonStr(t, body["name"]))
	testutil.NotNil(t, body["id"])
}

func TestCreateRecordInvalidJSON(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	req := httptest.NewRequest("POST", "/api/collections/authors/", bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)
	testutil.StatusCode(t, http.StatusBadRequest, w.Code)
}

func TestCreateRecordEmptyBody(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	w := doRequest(t, srv, "POST", "/api/collections/authors/", map[string]any{})
	testutil.StatusCode(t, http.StatusBadRequest, w.Code)
}

func TestCreateRecordNotNullViolation(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	// authors.name is NOT NULL.
	data := map[string]any{"id": 100}
	w := doRequest(t, srv, "POST", "/api/collections/authors/", data)
	testutil.StatusCode(t, http.StatusBadRequest, w.Code)

	body := parseJSON(t, w)
	testutil.Contains(t, jsonStr(t, body["message"]), "missing required")
}

func TestCreateRecordUniqueViolation(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	// tags.name has UNIQUE constraint.
	data := map[string]any{"name": "go"} // already exists
	w := doRequest(t, srv, "POST", "/api/collections/tags/", data)
	testutil.StatusCode(t, http.StatusConflict, w.Code)

	resp := parseJSON(t, w)
	testutil.Contains(t, jsonStr(t, resp["message"]), "unique constraint violation")
}

// --- Update tests ---

func TestUpdateRecord(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	data := map[string]any{"title": "Updated Title"}
	w := doRequest(t, srv, "PATCH", "/api/collections/posts/1", data)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	body := parseJSON(t, w)
	testutil.Equal(t, "Updated Title", jsonStr(t, body["title"]))
	testutil.Equal(t, 1.0, jsonNum(t, body["id"]))
}

func TestUpdateRecordNotFound(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	data := map[string]any{"title": "nope"}
	w := doRequest(t, srv, "PATCH", "/api/collections/posts/999", data)
	testutil.StatusCode(t, http.StatusNotFound, w.Code)
}

func TestUpdateRecordEmptyBody(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	w := doRequest(t, srv, "PATCH", "/api/collections/posts/1", map[string]any{})
	testutil.StatusCode(t, http.StatusBadRequest, w.Code)
}

// --- Delete tests ---

func TestDeleteRecord(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	w := doRequest(t, srv, "DELETE", "/api/collections/tags/3", nil)
	testutil.StatusCode(t, http.StatusNoContent, w.Code)

	// Verify it's gone.
	w = doRequest(t, srv, "GET", "/api/collections/tags/3", nil)
	testutil.StatusCode(t, http.StatusNotFound, w.Code)
}

func TestDeleteRecordNotFound(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	w := doRequest(t, srv, "DELETE", "/api/collections/tags/999", nil)
	testutil.StatusCode(t, http.StatusNotFound, w.Code)
}

// --- Expand tests ---

func TestReadWithExpand(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	// Test expand by FK column name (author_id).
	w := doRequest(t, srv, "GET", "/api/collections/posts/1?expand=author_id", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	body := parseJSON(t, w)
	testutil.Equal(t, "First Post", jsonStr(t, body["title"]))

	expandData, ok := body["expand"]
	if !ok {
		t.Fatal("expand key not present in response")
	}

	expandMap := expandData.(map[string]any)
	author, ok := expandMap["author"].(map[string]any)
	if !ok {
		t.Fatalf("expected expand.author to be a map, got %T", expandMap["author"])
	}
	testutil.Equal(t, "Alice", jsonStr(t, author["name"]))
}

func TestReadWithExpandFriendlyName(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	// Test expand by friendly name (author, derived from author_id).
	w := doRequest(t, srv, "GET", "/api/collections/posts/1?expand=author", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	body := parseJSON(t, w)
	expandData, ok := body["expand"]
	if !ok {
		t.Fatal("expand key not present in response")
	}

	expandMap := expandData.(map[string]any)
	author, ok := expandMap["author"].(map[string]any)
	if !ok {
		t.Fatalf("expected expand.author to be a map, got %T", expandMap["author"])
	}
	testutil.Equal(t, "Alice", jsonStr(t, author["name"]))
}

func TestListWithExpand(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	w := doRequest(t, srv, "GET", "/api/collections/posts/?expand=author", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	body := parseJSON(t, w)
	items := jsonItems(t, body)
	testutil.Equal(t, 3, len(items))

	// Build expected author_id -> name mapping from fixtures.
	wantAuthor := map[float64]string{1: "Alice", 2: "Bob"}

	// Every post with an author_id should have the correct expand.author.
	for _, item := range items {
		if item["author_id"] == nil {
			continue
		}
		authorID := jsonNum(t, item["author_id"])
		expandData, ok := item["expand"]
		if !ok {
			t.Fatalf("expand key not present on post with author_id=%v", authorID)
		}
		expandMap := expandData.(map[string]any)
		author, ok := expandMap["author"].(map[string]any)
		if !ok {
			t.Fatalf("expected expand.author to be a map, got %T", expandMap["author"])
		}
		// Verify the expanded author matches the post's author_id.
		testutil.Equal(t, authorID, jsonNum(t, author["id"]))
		testutil.Equal(t, wantAuthor[authorID], jsonStr(t, author["name"]))
	}
}

// --- One-to-many expand test ---

func TestListWithOneToManyExpand(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	// Expand posts from an author (one-to-many).
	w := doRequest(t, srv, "GET", "/api/collections/authors/1?expand=posts", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	body := parseJSON(t, w)
	testutil.Equal(t, "Alice", jsonStr(t, body["name"]))

	expandData, ok := body["expand"]
	if !ok {
		t.Fatal("expand key not present — one-to-many expand failed")
	}
	expandMap := expandData.(map[string]any)
	posts, ok := expandMap["posts"].([]any)
	if !ok {
		t.Fatalf("expected expand.posts to be an array, got %T", expandMap["posts"])
	}
	testutil.Equal(t, 2, len(posts)) // Alice has 2 posts
	// Verify the expanded posts are actually Alice's posts.
	titles := make(map[string]bool)
	for _, p := range posts {
		post := p.(map[string]any)
		titles[post["title"].(string)] = true
	}
	testutil.True(t, titles["First Post"], "expected 'First Post' in Alice's expanded posts")
	testutil.True(t, titles["Second Post"], "expected 'Second Post' in Alice's expanded posts")
}

// --- Validation tests ---

func TestCreateAllUnknownColumns(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	data := map[string]any{"nonexistent_col": "value", "also_fake": 123}
	w := doRequest(t, srv, "POST", "/api/collections/authors/", data)
	testutil.StatusCode(t, http.StatusBadRequest, w.Code)

	body := parseJSON(t, w)
	testutil.Contains(t, jsonStr(t, body["message"]), "no recognized columns")
}

func TestUpdateAllUnknownColumns(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	data := map[string]any{"nonexistent_col": "value"}
	w := doRequest(t, srv, "PATCH", "/api/collections/posts/1", data)
	testutil.StatusCode(t, http.StatusBadRequest, w.Code)

	body := parseJSON(t, w)
	testutil.Contains(t, jsonStr(t, body["message"]), "no recognized columns")
}

// --- Edge case tests ---

func TestViewReadOnly(t *testing.T) {
	ctx := context.Background()
	srv, pg := setupTestServer(t, ctx)

	// Create a view.
	_, err := pg.Pool.Exec(ctx, `CREATE VIEW active_posts AS SELECT * FROM posts WHERE status = 'published'`)
	if err != nil {
		t.Fatalf("creating view: %v", err)
	}

	// Reload schema to pick up the view.
	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(pg.Pool, logger)
	if err := ch.Load(ctx); err != nil {
		t.Fatalf("reloading schema: %v", err)
	}
	cfg := config.Default()
	srv = server.New(cfg, logger, ch, pg.Pool, nil, nil)

	// GET should work.
	w := doRequest(t, srv, "GET", "/api/collections/active_posts/", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	// POST should be rejected.
	data := map[string]any{"title": "test"}
	w = doRequest(t, srv, "POST", "/api/collections/active_posts/", data)
	testutil.StatusCode(t, http.StatusMethodNotAllowed, w.Code)
}

// --- Error format tests ---

func TestErrorResponseFormat(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	w := doRequest(t, srv, "GET", "/api/collections/nonexistent/", nil)
	testutil.StatusCode(t, http.StatusNotFound, w.Code)

	body := parseJSON(t, w)
	testutil.Equal(t, 404.0, jsonNum(t, body["code"]))
	msg, ok := body["message"].(string)
	testutil.True(t, ok, "expected message to be a string")
	testutil.Contains(t, msg, "not found")
}

// --- Full-text search tests ---

func TestSearchBasic(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	// Search for "Alice" — only appears in authors, not in posts.
	// Search for "Bob" — matches "Bob Post" title and "By Bob" body (1 post).
	w := doRequest(t, srv, "GET", "/api/collections/posts/?search=Bob", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	body := parseJSON(t, w)
	items := jsonItems(t, body)
	testutil.Equal(t, 1, len(items))
	// Verify both title and body to ensure search actually worked on content.
	testutil.Equal(t, "Bob Post", jsonStr(t, items[0]["title"]))
	testutil.Equal(t, "By Bob", jsonStr(t, items[0]["body"]))
	testutil.Equal(t, 2.0, jsonNum(t, items[0]["author_id"]))
}

func TestSearchMatchesContent(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	// "Hello world" appears only in First Post's body.
	w := doRequest(t, srv, "GET", "/api/collections/posts/?search=hello+world", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	body := parseJSON(t, w)
	items := jsonItems(t, body)
	testutil.Equal(t, 1, len(items))
	testutil.Equal(t, "First Post", jsonStr(t, items[0]["title"]))
}

func TestSearchNoResults(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	w := doRequest(t, srv, "GET", "/api/collections/posts/?search=zzz_nonexistent_xyz", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	body := parseJSON(t, w)
	items := jsonItems(t, body)
	testutil.Equal(t, 0, len(items))
	testutil.Equal(t, 0.0, jsonNum(t, body["totalItems"]))
}

func TestSearchWithFilter(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	// Search for "post" (all 3 match) but filter to only published (2 match).
	w := doRequest(t, srv, "GET", "/api/collections/posts/?search=post&filter=status%3D'published'", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	body := parseJSON(t, w)
	items := jsonItems(t, body)
	testutil.Equal(t, 2, len(items))
	testutil.Equal(t, 2.0, jsonNum(t, body["totalItems"]))

	// Verify all returned items are published.
	for _, item := range items {
		testutil.Equal(t, "published", jsonStr(t, item["status"]))
	}
}

func TestSearchWithPagination(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	// Search for "post" (3 results), paginate to 1 per page.
	w := doRequest(t, srv, "GET", "/api/collections/posts/?search=post&perPage=1&page=1", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	body := parseJSON(t, w)
	items := jsonItems(t, body)
	testutil.Equal(t, 1, len(items))
	testutil.Equal(t, 3.0, jsonNum(t, body["totalItems"]))
	testutil.Equal(t, 3.0, jsonNum(t, body["totalPages"]))
}

func TestSearchNoTextColumnsTable(t *testing.T) {
	ctx := context.Background()
	srv, pg := setupTestServer(t, ctx)

	// Create a table with no text columns.
	_, err := pg.Pool.Exec(ctx, `CREATE TABLE counters (id SERIAL PRIMARY KEY, count INTEGER)`)
	if err != nil {
		t.Fatalf("creating counters table: %v", err)
	}

	// Reload schema to pick up the new table.
	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(pg.Pool, logger)
	if err := ch.Load(ctx); err != nil {
		t.Fatalf("reloading schema: %v", err)
	}
	cfg := config.Default()
	srv = server.New(cfg, logger, ch, pg.Pool, nil, nil)

	w := doRequest(t, srv, "GET", "/api/collections/counters/?search=test", nil)
	testutil.StatusCode(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "no text columns")
}

func TestSearchEmptyString(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	// Empty search param should be ignored (return all records).
	w := doRequest(t, srv, "GET", "/api/collections/posts/?search=", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	body := parseJSON(t, w)
	items := jsonItems(t, body)
	testutil.Equal(t, 3, len(items)) // all posts returned
}

func TestSearchWhitespaceOnly(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	// Whitespace-only search should be treated as empty (trimmed by handler).
	w := doRequest(t, srv, "GET", "/api/collections/posts/?search=+++", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	body := parseJSON(t, w)
	items := jsonItems(t, body)
	testutil.Equal(t, 3, len(items)) // all posts returned
}

// --- Combined sort + filter + pagination ---

func TestCombinedFilterSortPagination(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	// Filter published, sort by id desc, page 1 perPage 1.
	w := doRequest(t, srv, "GET", "/api/collections/posts/?filter=status%3D'published'&sort=-id&page=1&perPage=1", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	body := parseJSON(t, w)
	testutil.Equal(t, 2.0, jsonNum(t, body["totalItems"]))
	testutil.Equal(t, 2.0, jsonNum(t, body["totalPages"]))

	items := jsonItems(t, body)
	testutil.Equal(t, 1, len(items))
	testutil.Equal(t, 3.0, jsonNum(t, items[0]["id"])) // Bob Post, highest published ID
}

// --- API hardening: FK expand edge cases ---

func TestExpandCircularReferenceSelfReferential(t *testing.T) {
	ctx := context.Background()
	srv, pg := setupTestServer(t, ctx)

	// Create a table with self-referential FK (e.g., users.manager_id -> users.id).
	_, err := pg.Pool.Exec(ctx, `
		CREATE TABLE users (
			id SERIAL PRIMARY KEY,
			name TEXT NOT NULL,
			manager_id INTEGER REFERENCES users(id)
		);
		INSERT INTO users (name, manager_id) VALUES
			('Alice', NULL),
			('Bob', 1),
			('Charlie', 2);
	`)
	testutil.NoError(t, err)

	// Reload schema.
	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(pg.Pool, logger)
	testutil.NoError(t, ch.Load(ctx))
	cfg := config.Default()
	srv = server.New(cfg, logger, ch, pg.Pool, nil, nil)

	// Expand manager.manager (two levels deep).
	w := doRequest(t, srv, "GET", "/api/collections/users/3?expand=manager.manager", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	body := parseJSON(t, w)
	testutil.Equal(t, "Charlie", jsonStr(t, body["name"]))

	// Verify expand.manager exists.
	expand := body["expand"].(map[string]any)
	manager := expand["manager"].(map[string]any)
	testutil.Equal(t, "Bob", jsonStr(t, manager["name"]))

	// Verify expand.manager.expand.manager exists (Alice, two levels deep).
	managerExpand := manager["expand"].(map[string]any)
	grandManager := managerExpand["manager"].(map[string]any)
	testutil.Equal(t, "Alice", jsonStr(t, grandManager["name"]))
}

func TestExpandMaxDepthEnforced(t *testing.T) {
	ctx := context.Background()
	srv, pg := setupTestServer(t, ctx)

	// Create self-referential table.
	_, err := pg.Pool.Exec(ctx, `
		CREATE TABLE categories (
			id SERIAL PRIMARY KEY,
			name TEXT NOT NULL,
			parent_id INTEGER REFERENCES categories(id)
		);
		INSERT INTO categories (name, parent_id) VALUES
			('Root', NULL),
			('Level1', 1),
			('Level2', 2),
			('Level3', 3);
	`)
	testutil.NoError(t, err)

	// Reload schema.
	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(pg.Pool, logger)
	testutil.NoError(t, ch.Load(ctx))
	cfg := config.Default()
	srv = server.New(cfg, logger, ch, pg.Pool, nil, nil)

	// Try to expand 3 levels (parent.parent.parent), but maxExpandDepth is 2.
	w := doRequest(t, srv, "GET", "/api/collections/categories/4?expand=parent.parent.parent", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	body := parseJSON(t, w)
	testutil.Equal(t, "Level3", jsonStr(t, body["name"]))

	// Should have expand.parent (Level2).
	expand := body["expand"].(map[string]any)
	parent := expand["parent"].(map[string]any)
	testutil.Equal(t, "Level2", jsonStr(t, parent["name"]))

	// Should have expand.parent.expand.parent (Level1).
	parentExpand := parent["expand"].(map[string]any)
	grandParent := parentExpand["parent"].(map[string]any)
	testutil.Equal(t, "Level1", jsonStr(t, grandParent["name"]))

	// Should NOT have a third level (depth limit enforced).
	_, hasThirdLevel := grandParent["expand"]
	testutil.False(t, hasThirdLevel, "max depth should prevent third level expand")
}

func TestExpandMissingRelation(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	// Try to expand a nonexistent relation.
	w := doRequest(t, srv, "GET", "/api/collections/posts/1?expand=nonexistent", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	body := parseJSON(t, w)
	// Verify the post was returned correctly with all expected fields.
	testutil.Equal(t, 1.0, jsonNum(t, body["id"]))
	testutil.Equal(t, "First Post", jsonStr(t, body["title"]))
	testutil.Equal(t, "Hello world", jsonStr(t, body["body"]))
	testutil.Equal(t, 1.0, jsonNum(t, body["author_id"]))
	testutil.Equal(t, "published", jsonStr(t, body["status"]))
	// The expand key should either be absent or not contain the nonexistent relation.
	expand, hasExpand := body["expand"]
	if !hasExpand {
		// expand key absent is valid — nonexistent relation correctly ignored.
		return
	}
	expandMap := expand.(map[string]any)
	_, hasNonexistent := expandMap["nonexistent"]
	testutil.False(t, hasNonexistent, "nonexistent relation should not be in expand")
}

// --- API hardening: Batch operation rollback ---

func TestBatchCreatePartialFailureRollback(t *testing.T) {
	ctx := context.Background()
	srv, pg := setupTestServer(t, ctx)

	// Create a table with a unique constraint.
	_, err := pg.Pool.Exec(ctx, `
		CREATE TABLE emails (
			id SERIAL PRIMARY KEY,
			address TEXT NOT NULL UNIQUE
		);
	`)
	testutil.NoError(t, err)

	// Reload schema.
	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(pg.Pool, logger)
	testutil.NoError(t, ch.Load(ctx))
	cfg := config.Default()
	srv = server.New(cfg, logger, ch, pg.Pool, nil, nil)

	// Batch insert: third record duplicates first, triggering unique constraint violation.
	batch := map[string]any{
		"operations": []map[string]any{
			{"method": "create", "body": map[string]any{"address": "alice@example.com"}},
			{"method": "create", "body": map[string]any{"address": "bob@example.com"}},
			{"method": "create", "body": map[string]any{"address": "alice@example.com"}}, // duplicate
		},
	}
	w := doRequest(t, srv, "POST", "/api/collections/emails/batch", batch)

	// Batch should fail with conflict (duplicate key violation).
	testutil.StatusCode(t, http.StatusConflict, w.Code)

	// Verify NO records were inserted (full rollback).
	var count int
	err = pg.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM emails").Scan(&count)
	testutil.NoError(t, err)
	testutil.Equal(t, 0, count)
}

func TestBatchUpdatePartialFailureRollback(t *testing.T) {
	ctx := context.Background()
	srv, pg := setupTestServer(t, ctx)

	// Seed data: tags table already exists from resetAndSeedDB with unique constraint on name.
	// Insert two additional tags with known IDs for this test.
	_, err := pg.Pool.Exec(ctx, `
		INSERT INTO tags (id, name) VALUES (100, 'original1'), (101, 'original2')
		ON CONFLICT (name) DO NOTHING;
	`)
	testutil.NoError(t, err)

	// Batch update via POST (batch endpoint is POST only) with BatchRequest format.
	// Try to set both to same name (violates unique constraint on name).
	batch := map[string]any{
		"operations": []map[string]any{
			{"method": "update", "id": "100", "body": map[string]any{"name": "updated"}},
			{"method": "update", "id": "101", "body": map[string]any{"name": "updated"}}, // duplicate
		},
	}
	w := doRequest(t, srv, "POST", "/api/collections/tags/batch", batch)

	// Should fail with conflict (unique constraint violation).
	testutil.StatusCode(t, http.StatusConflict, w.Code)

	// Verify BOTH records remain unchanged (full rollback).
	var name1, name2 string
	err = pg.Pool.QueryRow(ctx, "SELECT name FROM tags WHERE id = 100").Scan(&name1)
	testutil.NoError(t, err)
	testutil.Equal(t, "original1", name1)

	err = pg.Pool.QueryRow(ctx, "SELECT name FROM tags WHERE id = 101").Scan(&name2)
	testutil.NoError(t, err)
	testutil.Equal(t, "original2", name2)
}

// --- API hardening: RPC edge cases ---

func TestRPCFunctionWithVARIADICArgs(t *testing.T) {
	ctx := context.Background()
	srv, pg := setupTestServer(t, ctx)

	// Create a function with VARIADIC args.
	_, err := pg.Pool.Exec(ctx, `
		CREATE FUNCTION sum_all(VARIADIC vals INTEGER[]) RETURNS INTEGER AS $$
			SELECT SUM(v) FROM UNNEST(vals) AS v;
		$$ LANGUAGE SQL;
	`)
	testutil.NoError(t, err)

	// Reload schema so the new function is discoverable.
	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(pg.Pool, logger)
	testutil.NoError(t, ch.Load(ctx))
	cfg := config.Default()
	srv = server.New(cfg, logger, ch, pg.Pool, nil, nil)

	// Call with array of values.
	body := map[string]any{
		"vals": []int{1, 2, 3, 4, 5},
	}
	w := doRequest(t, srv, "POST", "/api/rpc/sum_all", body)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	// Single-column result is unwrapped to a scalar by the RPC handler.
	var result float64
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
	testutil.Equal(t, 15.0, result)
}

func TestRPCFunctionWithOUTParameters(t *testing.T) {
	ctx := context.Background()
	srv, pg := setupTestServer(t, ctx)

	// Create a function with OUT parameters.
	_, err := pg.Pool.Exec(ctx, `
		CREATE FUNCTION get_stats(OUT total INTEGER, OUT avg_val NUMERIC) AS $$
		BEGIN
			SELECT COUNT(*), AVG(id) INTO total, avg_val FROM posts;
		END;
		$$ LANGUAGE plpgsql;
	`)
	testutil.NoError(t, err)

	// Reload schema so the new function is discoverable.
	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(pg.Pool, logger)
	testutil.NoError(t, ch.Load(ctx))
	cfg := config.Default()
	srv = server.New(cfg, logger, ch, pg.Pool, nil, nil)

	// Call the function.
	w := doRequest(t, srv, "POST", "/api/rpc/get_stats", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	result := parseJSON(t, w)
	// OUT parameters return a record with named fields.
	testutil.Equal(t, 3.0, jsonNum(t, result["total"]))
	testutil.Equal(t, 2.0, jsonNum(t, result["avg_val"])) // AVG(1,2,3) = 2
}

func TestRPCFunctionReturningSetOf(t *testing.T) {
	ctx := context.Background()
	srv, pg := setupTestServer(t, ctx)

	// Create a function returning SETOF.
	_, err := pg.Pool.Exec(ctx, `
		CREATE FUNCTION get_all_author_names() RETURNS SETOF TEXT AS $$
			SELECT name FROM authors ORDER BY id;
		$$ LANGUAGE SQL;
	`)
	testutil.NoError(t, err)

	// Reload schema so the new function is discoverable.
	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(pg.Pool, logger)
	testutil.NoError(t, ch.Load(ctx))
	cfg := config.Default()
	srv = server.New(cfg, logger, ch, pg.Pool, nil, nil)

	w := doRequest(t, srv, "POST", "/api/rpc/get_all_author_names", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	// SETOF returns an array of records (each record is a map with column name as key).
	// For SETOF TEXT, the column is named after the function.
	var result []map[string]any
	err = json.Unmarshal(w.Body.Bytes(), &result)
	testutil.NoError(t, err)
	testutil.Equal(t, 2, len(result))
	// Extract the single column value from each record.
	names := make([]string, len(result))
	for i, row := range result {
		for _, v := range row {
			names[i] = v.(string)
		}
	}
	testutil.Equal(t, "Alice", names[0])
	testutil.Equal(t, "Bob", names[1])
}

func TestRPCFunctionThatRaisesException(t *testing.T) {
	ctx := context.Background()
	srv, pg := setupTestServer(t, ctx)

	// Create a function that raises an exception.
	_, err := pg.Pool.Exec(ctx, `
		CREATE FUNCTION raise_error() RETURNS VOID AS $$
		BEGIN
			RAISE EXCEPTION 'intentional error';
		END;
		$$ LANGUAGE plpgsql;
	`)
	testutil.NoError(t, err)

	// Reload schema so the new function is discoverable.
	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(pg.Pool, logger)
	testutil.NoError(t, ch.Load(ctx))
	cfg := config.Default()
	srv = server.New(cfg, logger, ch, pg.Pool, nil, nil)

	w := doRequest(t, srv, "POST", "/api/rpc/raise_error", nil)
	// P0001 (RAISE EXCEPTION) is mapped to 400 Bad Request by mapPGError.
	testutil.StatusCode(t, http.StatusBadRequest, w.Code)

	body := parseJSON(t, w)
	testutil.Contains(t, jsonStr(t, body["message"]), "intentional error")
}

func TestRPCFunctionWithNULLHandling(t *testing.T) {
	ctx := context.Background()
	srv, pg := setupTestServer(t, ctx)

	// Create a function that handles NULL.
	_, err := pg.Pool.Exec(ctx, `
		CREATE FUNCTION coalesce_text(val TEXT, fallback TEXT) RETURNS TEXT AS $$
			SELECT COALESCE(val, fallback);
		$$ LANGUAGE SQL;
	`)
	testutil.NoError(t, err)

	// Reload schema so the new function is discoverable.
	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(pg.Pool, logger)
	testutil.NoError(t, ch.Load(ctx))
	cfg := config.Default()
	srv = server.New(cfg, logger, ch, pg.Pool, nil, nil)

	// Call with NULL value.
	body := map[string]any{
		"val":      nil,
		"fallback": "default",
	}
	w := doRequest(t, srv, "POST", "/api/rpc/coalesce_text", body)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	// Single-column result is unwrapped to a scalar by the RPC handler.
	var result string
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
	testutil.Equal(t, "default", result)
}

// --- Error path coverage: constraint violations, type errors, FK violations ---

func TestCheckConstraintViolation(t *testing.T) {
	ctx := context.Background()
	srv, pg := setupTestServer(t, ctx)

	// Create a table with a CHECK constraint.
	_, err := pg.Pool.Exec(ctx, `
		CREATE TABLE products (
			id SERIAL PRIMARY KEY,
			name TEXT NOT NULL,
			price NUMERIC NOT NULL CHECK (price > 0)
		);
	`)
	testutil.NoError(t, err)

	// Reload schema.
	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(pg.Pool, logger)
	testutil.NoError(t, ch.Load(ctx))
	cfg := config.Default()
	srv = server.New(cfg, logger, ch, pg.Pool, nil, nil)

	// Insert with price = -1 to trigger CHECK violation.
	body := map[string]any{"name": "Widget", "price": -1}
	w := doRequest(t, srv, "POST", "/api/collections/products/", body)
	testutil.StatusCode(t, http.StatusBadRequest, w.Code)

	resp := parseJSON(t, w)
	testutil.Contains(t, resp["message"].(string), "check constraint violation")
}

func TestInvalidTypeValue(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	// posts.author_id is INTEGER. Pass a string that can't be parsed as int.
	body := map[string]any{"title": "Test", "author_id": "not-a-number"}
	w := doRequest(t, srv, "POST", "/api/collections/posts/", body)
	testutil.StatusCode(t, http.StatusBadRequest, w.Code)

	resp := parseJSON(t, w)
	testutil.Contains(t, resp["message"].(string), "invalid integer value")
}

func TestDeleteWithForeignKeyViolation(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	// Try to delete author 1 (Alice) — posts reference her via author_id FK.
	w := doRequest(t, srv, "DELETE", "/api/collections/authors/1", nil)
	testutil.StatusCode(t, http.StatusBadRequest, w.Code)

	resp := parseJSON(t, w)
	testutil.Contains(t, resp["message"].(string), "foreign key violation")
}

func TestBatchUpdateNotFoundReturns404(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	// Batch update with a non-existent ID.
	batch := map[string]any{
		"operations": []map[string]any{
			{"method": "update", "id": "99999", "body": map[string]any{"name": "Ghost"}},
		},
	}
	w := doRequest(t, srv, "POST", "/api/collections/authors/batch", batch)
	testutil.StatusCode(t, http.StatusNotFound, w.Code)

	resp := parseJSON(t, w)
	testutil.Contains(t, resp["message"].(string), "record not found")
}

func TestBatchDeleteNotFoundReturns404(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	// Batch delete with a non-existent ID.
	batch := map[string]any{
		"operations": []map[string]any{
			{"method": "delete", "id": "99999"},
		},
	}
	w := doRequest(t, srv, "POST", "/api/collections/authors/batch", batch)
	testutil.StatusCode(t, http.StatusNotFound, w.Code)

	resp := parseJSON(t, w)
	testutil.Contains(t, resp["message"].(string), "record not found")
}

func TestBatchNotFoundRollsBack(t *testing.T) {
	ctx := context.Background()
	srv, pg := setupTestServer(t, ctx)

	// Batch: create a record, then update a non-existent one.
	// The create should be rolled back.
	batch := map[string]any{
		"operations": []map[string]any{
			{"method": "create", "body": map[string]any{"name": "Charlie"}},
			{"method": "update", "id": "99999", "body": map[string]any{"name": "Ghost"}},
		},
	}
	w := doRequest(t, srv, "POST", "/api/collections/authors/batch", batch)
	testutil.StatusCode(t, http.StatusNotFound, w.Code)

	// Verify Charlie was NOT created (transaction rolled back).
	var count int
	err := pg.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM authors WHERE name = 'Charlie'").Scan(&count)
	testutil.NoError(t, err)
	testutil.Equal(t, 0, count)
}

func TestRPCFunctionReturningNULL(t *testing.T) {
	ctx := context.Background()
	srv, pg := setupTestServer(t, ctx)

	// Create a function that returns NULL.
	_, err := pg.Pool.Exec(ctx, `
		CREATE FUNCTION always_null() RETURNS TEXT AS $$
			SELECT NULL::TEXT;
		$$ LANGUAGE SQL;
	`)
	testutil.NoError(t, err)

	// Reload schema.
	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(pg.Pool, logger)
	testutil.NoError(t, ch.Load(ctx))
	cfg := config.Default()
	srv = server.New(cfg, logger, ch, pg.Pool, nil, nil)

	w := doRequest(t, srv, "POST", "/api/rpc/always_null", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	// A function returning NULL should produce a JSON null response.
	testutil.Equal(t, "null\n", w.Body.String())
}
