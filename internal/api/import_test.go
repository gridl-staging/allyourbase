package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/tenant"
	"github.com/allyourbase/ayb/internal/testutil"
)

// --- Parameter validation tests ---

func TestImport_InvalidMode(t *testing.T) {
	t.Parallel()
	h := testHandler(testSchema())
	w := doRequest(h, "POST", "/collections/users/import?mode=bogus", "")
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, resp.Message, "mode")
}

func TestImport_InvalidOnConflict(t *testing.T) {
	t.Parallel()
	h := testHandler(testSchema())
	w := doRequest(h, "POST", "/collections/users/import?on_conflict=bogus", "")
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, resp.Message, "on_conflict")
}

func TestImport_UnsupportedContentType(t *testing.T) {
	t.Parallel()
	h := testHandler(testSchema())
	req := httptest.NewRequest("POST", "/collections/users/import", strings.NewReader("data"))
	req.Header.Set("Content-Type", "application/xml")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	testutil.Equal(t, http.StatusUnsupportedMediaType, w.Code)
}

func TestImport_ViewNotWritable(t *testing.T) {
	t.Parallel()
	h := testHandler(testSchema())
	req := httptest.NewRequest("POST", "/collections/logs/import", strings.NewReader("[]"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	testutil.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func TestImport_NoPrimaryKey(t *testing.T) {
	t.Parallel()
	h := testHandler(testSchema())
	req := httptest.NewRequest("POST", "/collections/nopk/import", strings.NewReader("[]"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	testutil.Equal(t, http.StatusBadRequest, w.Code)
}

func TestImport_UnknownTable(t *testing.T) {
	t.Parallel()
	h := testHandler(testSchema())
	req := httptest.NewRequest("POST", "/collections/nonexistent/import", strings.NewReader("[]"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	testutil.Equal(t, http.StatusNotFound, w.Code)
}

// --- SQL builder tests ---

func TestBuildImportInsertSQL(t *testing.T) {
	t.Parallel()
	tbl := testSchema().Tables["public.users"]
	cols := []string{"id", "email", "name"}
	sql := buildImportInsertSQL(tbl, cols)
	testutil.Contains(t, sql, `INSERT INTO "public"."users"`)
	testutil.Contains(t, sql, `"id"`)
	testutil.Contains(t, sql, `"email"`)
	testutil.Contains(t, sql, `"name"`)
	testutil.Contains(t, sql, "$1")
	testutil.Contains(t, sql, "$2")
	testutil.Contains(t, sql, "$3")
	// No ON CONFLICT
	testutil.True(t, !strings.Contains(sql, "ON CONFLICT"))
}

func TestBuildImportInsertSQL_Skip(t *testing.T) {
	t.Parallel()
	tbl := testSchema().Tables["public.users"]
	cols := []string{"id", "email", "name"}
	sql := buildImportSkipSQL(tbl, cols)
	testutil.Contains(t, sql, `INSERT INTO "public"."users"`)
	testutil.Contains(t, sql, "ON CONFLICT")
	testutil.Contains(t, sql, "DO NOTHING")
}

func TestBuildImportInsertSQL_Update(t *testing.T) {
	t.Parallel()
	tbl := testSchema().Tables["public.users"]
	cols := []string{"id", "email", "name"}
	sql := buildImportUpdateSQL(tbl, cols)
	testutil.Contains(t, sql, `INSERT INTO "public"."users"`)
	testutil.Contains(t, sql, "ON CONFLICT")
	testutil.Contains(t, sql, "DO UPDATE SET")
	// PK columns should NOT be in the SET clause
	testutil.True(t, !strings.Contains(sql, `SET "id"`), "PK should be excluded from update SET")
	// Non-PK columns should be in the SET clause
	testutil.Contains(t, sql, `"email" = EXCLUDED."email"`)
	testutil.Contains(t, sql, `"name" = EXCLUDED."name"`)
}

func TestBuildImportSQL_IdentifierSafety(t *testing.T) {
	t.Parallel()
	tbl := &schema.Table{
		Schema: "public",
		Name:   "test",
		Kind:   "table",
		Columns: []*schema.Column{
			{Name: `col"inject`, TypeName: "text"},
		},
		PrimaryKey: []string{`col"inject`},
	}
	cols := []string{`col"inject`}
	sql := buildImportInsertSQL(tbl, cols)
	// quoteIdent escapes double quotes by doubling them
	testutil.Contains(t, sql, `"col""inject"`)
}

func TestBuildImportSQL_PlaceholderOrdering(t *testing.T) {
	t.Parallel()
	tbl := testSchema().Tables["public.users"]
	cols := []string{"id", "email", "name"}
	sql := buildImportInsertSQL(tbl, cols)
	testutil.Contains(t, sql, "($1, $2, $3)")
}

// --- CSV parser tests ---

func TestParseCSVHeaders_Valid(t *testing.T) {
	t.Parallel()
	tbl := testSchema().Tables["public.users"]
	reader := strings.NewReader("id,email,name\n1,a@b.com,Alice\n")
	headers, validCols, csvR, err := parseCSVHeaders(tbl, reader)
	testutil.NoError(t, err)
	testutil.SliceLen(t, headers, 3)
	testutil.Equal(t, 3, len(validCols))
	testutil.NotNil(t, csvR)
}

func TestParseCSVHeaders_BOM(t *testing.T) {
	t.Parallel()
	tbl := testSchema().Tables["public.users"]
	// UTF-8 BOM: \xEF\xBB\xBF
	reader := strings.NewReader("\xEF\xBB\xBFid,email,name\n1,a@b.com,Alice\n")
	headers, _, _, err := parseCSVHeaders(tbl, reader)
	testutil.NoError(t, err)
	testutil.Equal(t, "id", headers[0])
}

func TestParseCSVHeaders_MissingHeaders(t *testing.T) {
	t.Parallel()
	tbl := testSchema().Tables["public.users"]
	reader := strings.NewReader("")
	_, _, _, err := parseCSVHeaders(tbl, reader)
	testutil.Error(t, err)
	testutil.Contains(t, err.Error(), "header")
}

func TestParseCSVHeaders_DuplicateHeaders(t *testing.T) {
	t.Parallel()
	tbl := testSchema().Tables["public.users"]
	reader := strings.NewReader("id,email,email\n1,a@b.com,x@y.com\n")
	_, _, _, err := parseCSVHeaders(tbl, reader)
	testutil.Error(t, err)
	testutil.Contains(t, err.Error(), "duplicate")
}

func TestParseCSVHeaders_AllUnknownColumns(t *testing.T) {
	t.Parallel()
	tbl := testSchema().Tables["public.users"]
	reader := strings.NewReader("bogus,fake\n1,2\n")
	_, _, _, err := parseCSVHeaders(tbl, reader)
	testutil.Error(t, err)
	testutil.Contains(t, err.Error(), "no recognized columns")
}

func TestParseCSVHeaders_QuotedNewline(t *testing.T) {
	t.Parallel()
	tbl := testSchema().Tables["public.users"]
	reader := strings.NewReader("id,email,name\n1,a@b.com,\"Alice\nBob\"\n")
	headers, validCols, _, err := parseCSVHeaders(tbl, reader)
	testutil.NoError(t, err)
	testutil.SliceLen(t, headers, 3)
	testutil.Equal(t, 3, len(validCols))
}

func TestParseCSVHeaders_EmbeddedCommaAndQuotes(t *testing.T) {
	t.Parallel()
	tbl := testSchema().Tables["public.users"]
	reader := strings.NewReader("id,email,name\n1,a@b.com,\"Smith, Jr.\"\n")
	headers, _, _, err := parseCSVHeaders(tbl, reader)
	testutil.NoError(t, err)
	testutil.SliceLen(t, headers, 3)
}

// --- JSON streaming parser tests ---

func TestParseJSONStream_ValidArray(t *testing.T) {
	t.Parallel()
	tbl := testSchema().Tables["public.users"]
	body := `[{"id":"1","email":"a@b.com","name":"Alice"},{"id":"2","email":"c@d.com","name":"Bob"}]`
	reader := strings.NewReader(body)
	colMap := buildColumnMap(tbl)
	var rows []map[string]any
	var errs []ImportRowError
	err := streamJSONRows(reader, colMap, 100, func(row int, record map[string]any) error {
		rows = append(rows, record)
		return nil
	}, &errs)
	testutil.NoError(t, err)
	testutil.SliceLen(t, rows, 2)
	testutil.SliceLen(t, errs, 0)
}

func TestParseJSONStream_MalformedJSON(t *testing.T) {
	t.Parallel()
	tbl := testSchema().Tables["public.users"]
	body := `[{"id":"1", broken`
	reader := strings.NewReader(body)
	colMap := buildColumnMap(tbl)
	var errs []ImportRowError
	err := streamJSONRows(reader, colMap, 100, func(row int, record map[string]any) error {
		return nil
	}, &errs)
	testutil.Error(t, err)
}

func TestParseJSONStream_NonArrayTopLevel(t *testing.T) {
	t.Parallel()
	tbl := testSchema().Tables["public.users"]
	body := `{"id":"1","email":"a@b.com"}`
	reader := strings.NewReader(body)
	colMap := buildColumnMap(tbl)
	var errs []ImportRowError
	err := streamJSONRows(reader, colMap, 100, func(row int, record map[string]any) error {
		return nil
	}, &errs)
	testutil.Error(t, err)
	testutil.Contains(t, err.Error(), "array")
}

func TestParseJSONStream_NonObjectItem(t *testing.T) {
	t.Parallel()
	tbl := testSchema().Tables["public.users"]
	body := `["not-an-object", {"id":"1","email":"a@b.com"}]`
	reader := strings.NewReader(body)
	colMap := buildColumnMap(tbl)
	var rows []map[string]any
	var errs []ImportRowError
	err := streamJSONRows(reader, colMap, 100, func(row int, record map[string]any) error {
		rows = append(rows, record)
		return nil
	}, &errs)
	testutil.NoError(t, err) // continues past bad rows
	testutil.SliceLen(t, errs, 1)
	testutil.Equal(t, 1, errs[0].Row)
	testutil.Contains(t, errs[0].Message, "object")
}

func TestParseJSONStream_EmptyArray(t *testing.T) {
	t.Parallel()
	tbl := testSchema().Tables["public.users"]
	body := `[]`
	reader := strings.NewReader(body)
	colMap := buildColumnMap(tbl)
	var rows []map[string]any
	var errs []ImportRowError
	err := streamJSONRows(reader, colMap, 100, func(row int, record map[string]any) error {
		rows = append(rows, record)
		return nil
	}, &errs)
	testutil.NoError(t, err)
	testutil.SliceLen(t, rows, 0)
}

func TestParseJSONStream_RowLimitIncludesNonObjectItems(t *testing.T) {
	t.Parallel()
	tbl := testSchema().Tables["public.users"]
	body := `["not-an-object", "also-not-an-object"]`
	reader := strings.NewReader(body)
	colMap := buildColumnMap(tbl)
	var errs []ImportRowError
	err := streamJSONRows(reader, colMap, 1, func(row int, record map[string]any) error {
		return nil
	}, &errs)
	testutil.Error(t, err)
	testutil.Contains(t, err.Error(), "import row limit exceeded")
	testutil.SliceLen(t, errs, 1)
}

// --- buildColumnMap tests ---

func TestBuildColumnMap(t *testing.T) {
	t.Parallel()
	tbl := testSchema().Tables["public.users"]
	m := buildColumnMap(tbl)
	testutil.Equal(t, 3, len(m))
	testutil.True(t, m["id"])
	testutil.True(t, m["email"])
	testutil.True(t, m["name"])
	testutil.False(t, m["nonexistent"])
}

// --- Route wiring test ---

func TestImport_RouteWired(t *testing.T) {
	t.Parallel()
	h := testHandler(testSchema())
	req := httptest.NewRequest("POST", "/collections/users/import", strings.NewReader("[]"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	// Should not be 404 — it should hit the handler and fail at DB level (500)
	testutil.True(t, w.Code != http.StatusNotFound, "expected non-404, got %d", w.Code)
	testutil.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestImport_RouteWiredCSV(t *testing.T) {
	t.Parallel()
	h := testHandler(testSchema())
	req := httptest.NewRequest("POST", "/collections/users/import", strings.NewReader("id,email,name\n1,a@b.com,Alice\n"))
	req.Header.Set("Content-Type", "text/csv")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	testutil.True(t, w.Code != http.StatusNotFound, "expected non-404, got %d", w.Code)
	testutil.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestImport_OversizeBodyRejected(t *testing.T) {
	t.Parallel()
	h := testHandlerWithOptions(testSchema(), WithAPILimits(config.APIConfig{ImportMaxSizeMB: 1}))

	largeBody := strings.Repeat("a", (1<<20)+1)
	req := httptest.NewRequest("POST", "/collections/users/import", strings.NewReader(largeBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusRequestEntityTooLarge && w.Code != http.StatusBadRequest {
		t.Fatalf("expected 413 or 400 for oversized import body, got %d", w.Code)
	}
}

func TestImport_RowLimitExceededRejected(t *testing.T) {
	t.Parallel()
	h := testHandlerWithOptions(
		testSchema(),
		WithAPILimits(config.APIConfig{ImportMaxRows: 1}),
	)

	req := httptest.NewRequest("POST", "/collections/users/import", strings.NewReader(`[{"id":"1","email":"a@b.com"},{"id":"2","email":"c@d.com"}]`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, resp.Message, "import row limit exceeded")
}

func TestImport_CSVParseErrorsStillCountTowardRowLimit(t *testing.T) {
	t.Parallel()
	h := testHandlerWithOptions(
		testSchema(),
		WithAPILimits(config.APIConfig{ImportMaxRows: 1}),
	)

	body := "id,email,name\n1,a@b.com\n2,c@d.com\n"
	req := httptest.NewRequest("POST", "/collections/users/import?mode=partial", strings.NewReader(body))
	req.Header.Set("Content-Type", "text/csv")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, resp.Message, "import row limit exceeded")
}

func TestImportQuerier_RLSEnabledTableInjectsClaimsWithoutCallerClaims(t *testing.T) {
	t.Parallel()

	for _, mode := range []string{"full", "partial"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()

			tbl := testSchema().Tables["public.users"]
			tbl.RLSEnabled = true

			requestTx := &fakeRequestTx{}
			requestConn := &fakeRequestConn{beginTx: requestTx}
			h := &Handler{logger: slog.Default()}

			req := httptest.NewRequest(http.MethodPost, "/collections/users/import", nil)
			req = req.WithContext(tenant.ContextWithRequestConn(req.Context(), requestConn))

			q, done, err := h.importQuerier(req, tbl, mode)
			testutil.NoError(t, err)
			testutil.True(t, q == requestTx, "expected request-scoped transaction for RLS-enabled import")
			testutil.Equal(t, 1, requestConn.beginCalls)
			testutil.True(t, len(requestTx.execSQLs) > 0, "expected RLS session SQL to be applied")
			testutil.Contains(t, requestTx.execSQLs[0], "SET LOCAL ROLE")
			testutil.NoError(t, done(nil))
		})
	}
}

func TestImportQuerier_NonRLSTablePreservesExistingClaimlessBehavior(t *testing.T) {
	t.Parallel()

	tbl := testSchema().Tables["public.users"]
	requestTx := &fakeRequestTx{}
	requestConn := &fakeRequestConn{beginTx: requestTx}
	h := &Handler{logger: slog.Default()}

	fullReq := httptest.NewRequest(http.MethodPost, "/collections/users/import", nil)
	fullReq = fullReq.WithContext(tenant.ContextWithRequestConn(fullReq.Context(), requestConn))

	q, done, err := h.importQuerier(fullReq, tbl, "full")
	testutil.NoError(t, err)
	testutil.True(t, q == requestTx, "expected full-mode import to use explicit transaction")
	testutil.Equal(t, 1, requestConn.beginCalls)
	testutil.Equal(t, 0, len(requestTx.execSQLs))
	testutil.NoError(t, done(nil))

	partialReq := httptest.NewRequest(http.MethodPost, "/collections/users/import", nil)
	partialReq = partialReq.WithContext(tenant.ContextWithRequestConn(partialReq.Context(), requestConn))

	q, done, err = h.importQuerier(partialReq, tbl, "partial")
	testutil.NoError(t, err)
	testutil.True(t, q == requestConn, "expected partial-mode import to use request connection directly")
	testutil.Equal(t, 1, requestConn.beginCalls)
	testutil.NoError(t, done(nil))
}

// --- ImportResponse JSON shape ---

func TestImportResponse_JSONShape(t *testing.T) {
	t.Parallel()
	resp := ImportResponse{
		Processed: 10,
		Inserted:  8,
		Updated:   0,
		Skipped:   1,
		Failed:    1,
		Errors: []ImportRowError{
			{Row: 3, Message: "duplicate key"},
		},
	}
	b, err := json.Marshal(resp)
	testutil.NoError(t, err)
	var m map[string]any
	testutil.NoError(t, json.Unmarshal(b, &m))
	testutil.Equal[float64](t, 10, m["processed"].(float64))
	testutil.Equal[float64](t, 8, m["inserted"].(float64))
	testutil.Equal[float64](t, 0, m["updated"].(float64))
	testutil.Equal[float64](t, 1, m["skipped"].(float64))
	testutil.Equal[float64](t, 1, m["failed"].(float64))
	errs := m["errors"].([]any)
	testutil.Equal(t, 1, len(errs))
}

func TestImportResponse_NoErrors_OmitsField(t *testing.T) {
	t.Parallel()
	resp := ImportResponse{
		Processed: 5,
		Inserted:  5,
	}
	b, err := json.Marshal(resp)
	testutil.NoError(t, err)
	var m map[string]any
	testutil.NoError(t, json.Unmarshal(b, &m))
	_, hasErrors := m["errors"]
	testutil.False(t, hasErrors, "errors field should be omitted when empty")
}

// --- filterRecordColumns tests ---

func TestFilterRecordColumns_RemovesUnknown(t *testing.T) {
	t.Parallel()
	colMap := map[string]bool{"id": true, "email": true}
	record := map[string]any{"id": "1", "email": "a@b.com", "bogus": "x"}
	filtered := filterRecordColumns(record, colMap)
	testutil.Equal(t, 2, len(filtered))
	testutil.Equal(t, "1", filtered["id"])
	testutil.Equal(t, "a@b.com", filtered["email"])
}

func TestFilterRecordColumns_AllValid(t *testing.T) {
	t.Parallel()
	colMap := map[string]bool{"id": true, "email": true}
	record := map[string]any{"id": "1", "email": "a@b.com"}
	filtered := filterRecordColumns(record, colMap)
	testutil.Equal(t, 2, len(filtered))
}
