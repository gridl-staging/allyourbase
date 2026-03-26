//go:build integration

package api_test

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/server"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/golang-jwt/jwt/v5"
)

// --- RLS Enforcement Tests for Export, Aggregate, and Import ---

// setupRLSTestServer creates a test server with RLS-enabled tables and policies.
// It creates the ayb_authenticated role (matching auth.AuthenticatedRole) and
// grants it access so that SET LOCAL ROLE succeeds during withRLS.
func setupRLSTestServer(t *testing.T, ctx context.Context) *server.Server {
	t.Helper()

	resetAndSeedDB(t, ctx)

	// Create the authenticated role (idempotent) and grant schema/table access.
	_, err := sharedPG.Pool.Exec(ctx, `
		DO $$ BEGIN
			CREATE ROLE ayb_authenticated NOLOGIN;
		EXCEPTION WHEN duplicate_object THEN NULL;
		END $$;

		GRANT USAGE ON SCHEMA public TO ayb_authenticated;
		GRANT ALL ON ALL TABLES IN SCHEMA public TO ayb_authenticated;
		GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO ayb_authenticated;
	`)
	if err != nil {
		t.Fatalf("creating ayb_authenticated role: %v", err)
	}

	// Create a table with RLS enabled and a policy that restricts access.
	_, err = sharedPG.Pool.Exec(ctx, `
		CREATE TABLE rls_test_docs (
			id SERIAL PRIMARY KEY,
			owner_id TEXT NOT NULL,
			content TEXT NOT NULL
		);
		ALTER TABLE rls_test_docs ENABLE ROW LEVEL SECURITY;
		ALTER TABLE rls_test_docs FORCE ROW LEVEL SECURITY;

		-- Grant the authenticated role access to the new table and its sequence.
		GRANT ALL ON rls_test_docs TO ayb_authenticated;
		GRANT USAGE, SELECT ON SEQUENCE rls_test_docs_id_seq TO ayb_authenticated;

		-- Policy: users can only see/modify their own rows.
		CREATE POLICY owner_isolation ON rls_test_docs
			FOR ALL
			TO ayb_authenticated
			USING (owner_id = current_setting('ayb.user_id', true))
			WITH CHECK (owner_id = current_setting('ayb.user_id', true));

		-- Insert test data for two different "users"
		INSERT INTO rls_test_docs (owner_id, content) VALUES
			('user-alice', 'Alice secret doc'),
			('user-alice', 'Alice second doc'),
			('user-bob', 'Bob private doc');
	`)
	if err != nil {
		t.Fatalf("creating RLS test table: %v", err)
	}

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	if err := ch.Load(ctx); err != nil {
		t.Fatalf("loading schema cache: %v", err)
	}

	cfg := config.Default()
	srv := server.New(cfg, logger, ch, sharedPG.Pool, nil, nil)
	return srv
}

// TestExportCSV_EnforcesRLS verifies that CSV export respects RLS policies.
func TestExportCSV_EnforcesRLS(t *testing.T) {
	ctx := context.Background()
	srv := setupRLSTestServer(t, ctx)

	// Request as Alice - should only see Alice's 2 rows.
	claims := &auth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: "user-alice",
		},
	}

	w := doRequestWithClaims(t, srv, "GET", "/api/collections/rls_test_docs/export.csv", nil, claims)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	// Parse CSV and verify only Alice's rows are present.
	reader := csv.NewReader(strings.NewReader(w.Body.String()))
	records, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("parsing CSV: %v", err)
	}

	// Header + 2 data rows (Alice's docs only).
	if len(records) != 3 {
		t.Fatalf("expected 3 CSV rows (1 header + 2 Alice's data), got %d", len(records))
	}

	// Verify Bob's content is NOT present.
	bodyStr := w.Body.String()
	if strings.Contains(bodyStr, "Bob private doc") {
		t.Fatal("RLS leak: Bob's data visible to Alice in CSV export")
	}
	if !strings.Contains(bodyStr, "Alice secret doc") {
		t.Fatal("Alice's data should be visible to Alice in CSV export")
	}
}

// TestExportJSON_EnforcesRLS verifies that JSON export respects RLS policies.
func TestExportJSON_EnforcesRLS(t *testing.T) {
	ctx := context.Background()
	srv := setupRLSTestServer(t, ctx)

	// Request as Bob - should only see Bob's 1 row.
	claims := &auth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: "user-bob",
		},
	}

	w := doRequestWithClaims(t, srv, "GET", "/api/collections/rls_test_docs/export.json", nil, claims)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	// Parse JSON and verify only Bob's row is present.
	var items []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &items); err != nil {
		t.Fatalf("parsing JSON: %v\nbody: %s", err, w.Body.String())
	}

	if len(items) != 1 {
		t.Fatalf("expected 1 JSON item (Bob's data), got %d", len(items))
	}

	// Verify it's Bob's content.
	content, ok := items[0]["content"].(string)
	if !ok || content != "Bob private doc" {
		t.Fatalf("expected Bob's content, got: %v", items[0])
	}
}

// TestAggregate_EnforcesRLS verifies that aggregate queries respect RLS policies.
func TestAggregate_EnforcesRLS(t *testing.T) {
	ctx := context.Background()
	srv := setupRLSTestServer(t, ctx)

	// Request as Alice - should count only Alice's 2 rows.
	claims := &auth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: "user-alice",
		},
	}

	w := doRequestWithClaims(t, srv, "GET", "/api/collections/rls_test_docs/?aggregate=count", nil, claims)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	body := parseJSON(t, w)
	results := jsonResults(t, body)

	if len(results) != 1 {
		t.Fatalf("expected 1 aggregate result, got %d", len(results))
	}

	// Should be 2 (Alice's rows), not 3 (total rows).
	count := jsonNum(t, results[0]["count"])
	if count != 2.0 {
		t.Fatalf("expected count=2 for Alice's rows, got %v", count)
	}
}

// TestAggregateGroupBy_EnforcesRLS verifies that grouped aggregates respect RLS.
func TestAggregateGroupBy_EnforcesRLS(t *testing.T) {
	ctx := context.Background()
	srv := setupRLSTestServer(t, ctx)

	// Request as Alice with group by.
	claims := &auth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: "user-alice",
		},
	}

	w := doRequestWithClaims(t, srv, "GET", "/api/collections/rls_test_docs/?aggregate=count&group=owner_id", nil, claims)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	body := parseJSON(t, w)
	results := jsonResults(t, body)

	// Should only see Alice's group.
	if len(results) != 1 {
		t.Fatalf("expected 1 group result (Alice only), got %d", len(results))
	}

	ownerID := jsonStr(t, results[0]["owner_id"])
	if ownerID != "user-alice" {
		t.Fatalf("expected only Alice's group, got owner_id: %s", ownerID)
	}
}

// TestImport_EnforcesRLS verifies that import operations respect RLS policies.
func TestImport_EnforcesRLS(t *testing.T) {
	ctx := context.Background()
	srv := setupRLSTestServer(t, ctx)

	// Bob tries to insert a row with Alice as owner - should be blocked by RLS.
	claims := &auth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: "user-bob",
		},
	}

	importBody := []map[string]string{{"owner_id": "user-alice", "content": "Bob pretending to be Alice"}}
	w := doRequestWithClaims(t, srv, "POST", "/api/collections/rls_test_docs/import", importBody, claims)

	// Should fail due to RLS policy violation.
	if w.Code < 400 {
		t.Fatalf("expected error due to RLS policy violation, got status %d", w.Code)
	}

	// Verify the row was NOT inserted.
	var count int
	err := sharedPG.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM rls_test_docs WHERE content = 'Bob pretending to be Alice'").Scan(&count)
	if err != nil {
		t.Fatalf("querying for inserted row: %v", err)
	}
	if count != 0 {
		t.Fatal("RLS policy violation: Bob was able to insert row with Alice as owner")
	}
}

// TestExport_NoClaimsNoRLSContext verifies export without claims still works (no RLS context).
func TestExport_NoClaimsNoRLSContext(t *testing.T) {
	ctx := context.Background()
	srv := setupRLSTestServer(t, ctx)

	// Without claims, withRLS returns the pool (no SET LOCAL ROLE).
	// FORCE ROW LEVEL SECURITY is on and the policy targets ayb_authenticated,
	// so the pool owner has no matching policy → default deny → 0 rows.
	w := doRequest(t, srv, "GET", "/api/collections/rls_test_docs/export.json", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	var items []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &items); err != nil {
		t.Fatalf("parsing JSON: %v\nbody: %s", err, w.Body.String())
	}
	if len(items) != 0 {
		t.Fatalf("expected 0 items without RLS claims context, got %d", len(items))
	}
}

// --- Cross-Stage Regression Protection Tests ---

// TestAggregate_DisabledReturns403_Regression verifies Stage 4 aggregate_enabled gate.
func TestAggregate_DisabledReturns403_Regression(t *testing.T) {
	ctx := context.Background()

	resetAndSeedDB(t, ctx)

	// Add products table for aggregate testing.
	_, err := sharedPG.Pool.Exec(ctx, `
		CREATE TABLE products (
			id SERIAL PRIMARY KEY,
			name TEXT NOT NULL,
			price NUMERIC NOT NULL,
			category TEXT NOT NULL
		);
		INSERT INTO products (name, price, category) VALUES
			('Widget', 10.00, 'toys'),
			('Gadget', 20.00, 'toys');
	`)
	if err != nil {
		t.Fatalf("creating products table: %v", err)
	}

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	if err := ch.Load(ctx); err != nil {
		t.Fatalf("loading schema cache: %v", err)
	}

	// Create server with aggregate disabled.
	cfg := config.Default()
	cfg.API.AggregateEnabled = false
	srv := server.New(cfg, logger, ch, sharedPG.Pool, nil, nil)

	w := doRequest(t, srv, "GET", "/api/collections/products/?aggregate=count", nil)
	testutil.StatusCode(t, http.StatusForbidden, w.Code)

	body := parseJSON(t, w)
	if !strings.Contains(jsonStr(t, body["message"]), "aggregate queries are disabled") {
		t.Fatalf("expected 'aggregate queries are disabled' message, got: %s", jsonStr(t, body["message"]))
	}
}

// TestExport_RespectsMaxRowsLimit_Regression verifies Stage 4 ExportMaxRows limit.
func TestExport_RespectsMaxRowsLimit_Regression(t *testing.T) {
	ctx := context.Background()

	resetAndSeedDB(t, ctx)

	// Create table with 10 rows.
	_, err := sharedPG.Pool.Exec(ctx, `
		CREATE TABLE many_items (
			id SERIAL PRIMARY KEY,
			name TEXT NOT NULL
		);
		INSERT INTO many_items (name) 
		SELECT 'item_' || generate_series FROM generate_series(1, 10);
	`)
	if err != nil {
		t.Fatalf("creating many_items table: %v", err)
	}

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	if err := ch.Load(ctx); err != nil {
		t.Fatalf("loading schema cache: %v", err)
	}

	// Create server with export limit of 3.
	cfg := config.Default()
	cfg.API.ExportMaxRows = 3
	srv := server.New(cfg, logger, ch, sharedPG.Pool, nil, nil)

	w := doRequest(t, srv, "GET", "/api/collections/many_items/export.csv", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	// Parse CSV and verify only 3 data rows + header.
	reader := csv.NewReader(strings.NewReader(w.Body.String()))
	records, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("parsing CSV: %v", err)
	}

	// Header + 3 data rows (limited by ExportMaxRows).
	if len(records) != 4 {
		t.Fatalf("expected 4 CSV rows (1 header + 3 data due to ExportMaxRows=3), got %d", len(records))
	}
}

// TestImport_RespectsMaxRowsLimit_Regression verifies Stage 4 ImportMaxRows limit.
func TestImport_RespectsMaxRowsLimit_Regression(t *testing.T) {
	ctx := context.Background()

	resetAndSeedDB(t, ctx)

	// Create table for import.
	_, err := sharedPG.Pool.Exec(ctx, `
		CREATE TABLE limited_items (
			id INTEGER PRIMARY KEY,
			name TEXT NOT NULL
		);
	`)
	if err != nil {
		t.Fatalf("creating limited_items table: %v", err)
	}

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	if err := ch.Load(ctx); err != nil {
		t.Fatalf("loading schema cache: %v", err)
	}

	// Create server with import limit of 2.
	cfg := config.Default()
	cfg.API.ImportMaxRows = 2
	srv := server.New(cfg, logger, ch, sharedPG.Pool, nil, nil)

	// Try to import 3 rows (should fail).
	jsonBody := `[{"id":1,"name":"Item 1"},{"id":2,"name":"Item 2"},{"id":3,"name":"Item 3"}]`
	w := doImportRequest(t, srv, "/api/collections/limited_items/import", "application/json", jsonBody)

	// Should get 400 Bad Request due to row limit exceeded.
	testutil.StatusCode(t, http.StatusBadRequest, w.Code)

	body := parseJSON(t, w)
	if !strings.Contains(jsonStr(t, body["message"]), "import row limit exceeded") {
		t.Fatalf("expected 'import row limit exceeded' message, got: %s", jsonStr(t, body["message"]))
	}
}

// TestAggregate_HasQueryDurationHeader_Regression verifies Stage 4 X-Query-Duration-Ms header.
func TestAggregate_HasQueryDurationHeader_Regression(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupAggregateTestServer(t, ctx)

	w := doRequest(t, srv, "GET", "/api/collections/products/?aggregate=count", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	header := w.Header().Get("X-Query-Duration-Ms")
	if header == "" {
		t.Fatal("expected X-Query-Duration-Ms header in aggregate response")
	}

	// Verify it's a valid non-negative integer.
	ms, err := strconv.ParseInt(header, 10, 64)
	if err != nil {
		t.Fatalf("X-Query-Duration-Ms is not a valid integer: %s", header)
	}
	if ms < 0 {
		t.Fatalf("X-Query-Duration-Ms should be non-negative, got: %d", ms)
	}
}

// TestExport_FilterPassthrough_Regression verifies Stage 2 filter passthrough in export.
func TestExport_FilterPassthrough_Regression(t *testing.T) {
	ctx := context.Background()
	srv := setupExportTestServer(t, ctx)

	// Filter for published posts only.
	w := doRequest(t, srv, "GET", "/api/collections/posts/export.json?filter=status%3D'published'", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	var items []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &items); err != nil {
		t.Fatalf("parsing JSON: %v", err)
	}

	// Should have 3 published posts.
	if len(items) != 3 {
		t.Fatalf("expected 3 published posts, got %d", len(items))
	}

	// Verify all returned items have status=published.
	for _, item := range items {
		status, ok := item["status"].(string)
		if !ok || status != "published" {
			t.Fatalf("expected all items to have status=published, got: %v", item)
		}
	}
}

// TestExport_SortPassthrough_Regression verifies Stage 2 sort passthrough in export.
func TestExport_SortPassthrough_Regression(t *testing.T) {
	ctx := context.Background()
	srv := setupExportTestServer(t, ctx)

	// Sort by id descending.
	w := doRequest(t, srv, "GET", "/api/collections/posts/export.json?sort=-id", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	var items []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &items); err != nil {
		t.Fatalf("parsing JSON: %v", err)
	}

	if len(items) < 2 {
		t.Fatal("expected at least 2 items for sort test")
	}

	// Verify descending order by id.
	prevID := jsonNum(t, items[0]["id"])
	for i := 1; i < len(items); i++ {
		currID := jsonNum(t, items[i]["id"])
		if currID >= prevID {
			t.Fatalf("items not in descending order at index %d: %.0f >= %.0f", i, currID, prevID)
		}
		prevID = currID
	}
}

// TestExport_SearchPassthrough_Regression verifies Stage 2 search passthrough
// and preserves ranking/order semantics in export results.
func TestExport_SearchPassthrough_Regression(t *testing.T) {
	ctx := context.Background()
	srv := setupExportTestServer(t, ctx)

	// Use list endpoint as source-of-truth for search ranking/order semantics.
	listW := doRequest(t, srv, "GET", "/api/collections/posts/?search=post", nil)
	testutil.StatusCode(t, http.StatusOK, listW.Code)
	listBody := parseJSON(t, listW)
	listItems := jsonItems(t, listBody)

	exportW := doRequest(t, srv, "GET", "/api/collections/posts/export.json?search=post", nil)
	testutil.StatusCode(t, http.StatusOK, exportW.Code)

	var exportItems []map[string]any
	if err := json.Unmarshal(exportW.Body.Bytes(), &exportItems); err != nil {
		t.Fatalf("parsing JSON: %v\nbody: %s", err, exportW.Body.String())
	}

	if len(exportItems) != len(listItems) {
		t.Fatalf("expected export and list search results to have equal length, got export=%d list=%d", len(exportItems), len(listItems))
	}

	for i := range exportItems {
		exportID := jsonNum(t, exportItems[i]["id"])
		listID := jsonNum(t, listItems[i]["id"])
		if exportID != listID {
			t.Fatalf("search order mismatch at index %d: export id %.0f != list id %.0f", i, exportID, listID)
		}
	}
}

// TestExport_SearchTooLongReturns400_Regression verifies invalid search input
// in export endpoints still returns the expected 4xx contract.
func TestExport_SearchTooLongReturns400_Regression(t *testing.T) {
	ctx := context.Background()
	srv := setupExportTestServer(t, ctx)

	longSearch := strings.Repeat("a", 1001) // maxSearchLen is 1000
	w := doRequest(t, srv, "GET", "/api/collections/posts/export.json?search="+longSearch, nil)
	testutil.StatusCode(t, http.StatusBadRequest, w.Code)

	body := parseJSON(t, w)
	if !strings.Contains(jsonStr(t, body["message"]), "search term too long") {
		t.Fatalf("expected 'search term too long' message, got: %s", jsonStr(t, body["message"]))
	}
}

// TestImport_FullModeAtomicRollback_Regression verifies Stage 4 import full mode atomicity.
func TestImport_FullModeAtomicRollback_Regression(t *testing.T) {
	ctx := context.Background()
	srv := setupImportTestServer(t, ctx)

	// First row valid, second row duplicate (id=100 exists).
	jsonBody := `[{"id":1000,"name":"Should Rollback","category":"test"},{"id":100,"name":"Dup","category":"dup"}]`
	w := doImportRequest(t, srv, "/api/collections/import_items/import", "application/json", jsonBody)

	// Should fail due to duplicate.
	testutil.StatusCode(t, http.StatusConflict, w.Code)
	resp := parseJSON(t, w)
	testutil.Equal(t, float64(2), jsonNum(t, resp["processed"]))
	testutil.Equal(t, float64(1), jsonNum(t, resp["failed"]))
	errorsList, ok := resp["errors"].([]any)
	if !ok {
		t.Fatalf("expected errors array, got %T", resp["errors"])
	}
	testutil.Equal(t, 1, len(errorsList))
	firstErr, ok := errorsList[0].(map[string]any)
	if !ok {
		t.Fatalf("expected errors[0] object, got %T", errorsList[0])
	}
	testutil.Equal(t, float64(2), jsonNum(t, firstErr["row"]))

	// Verify the first row was NOT committed (rollback).
	var count int
	err := sharedPG.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM import_items WHERE id = 1000").Scan(&count)
	if err != nil {
		t.Fatalf("querying for rolled back row: %v", err)
	}
	if count != 0 {
		t.Fatal("full mode atomicity violation: first row was committed despite second row failing")
	}
	err = sharedPG.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM import_items WHERE id = 100").Scan(&count)
	if err != nil {
		t.Fatalf("querying for existing duplicate row: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected seeded duplicate row to remain unchanged, got count %d", count)
	}
}

// TestImport_PartialModeContinuesOnError_Regression verifies Stage 4 import partial mode.
func TestImport_PartialModeContinuesOnError_Regression(t *testing.T) {
	ctx := context.Background()
	srv := setupImportTestServer(t, ctx)

	// First row valid, second row duplicate, third row valid.
	jsonBody := `[{"id":2000,"name":"Good 1","category":"ok"},{"id":100,"name":"Dup","category":"dup"},{"id":2001,"name":"Good 2","category":"ok"}]`
	w := doImportRequest(t, srv, "/api/collections/import_items/import?mode=partial", "application/json", jsonBody)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	result := parseJSON(t, w)
	testutil.Equal(t, float64(3), jsonNum(t, result["processed"]))
	testutil.Equal(t, float64(2), jsonNum(t, result["inserted"]))
	testutil.Equal(t, float64(1), jsonNum(t, result["failed"]))

	// Verify both valid rows were committed.
	var count int
	err := sharedPG.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM import_items WHERE id IN (2000, 2001)").Scan(&count)
	if err != nil {
		t.Fatalf("querying for committed rows: %v", err)
	}
	if count != 2 {
		t.Fatalf("partial mode should commit valid rows, expected 2, got %d", count)
	}
}
