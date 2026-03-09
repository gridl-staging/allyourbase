package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/testutil"
)

// testSchema creates a minimal schema cache with a "users" table for testing.
func testSchema() *schema.SchemaCache {
	return &schema.SchemaCache{
		Tables: map[string]*schema.Table{
			"public.users": {
				Schema: "public",
				Name:   "users",
				Kind:   "table",
				Columns: []*schema.Column{
					{Name: "id", TypeName: "uuid"},
					{Name: "email", TypeName: "text"},
					{Name: "name", TypeName: "text", IsNullable: true},
				},
				PrimaryKey: []string{"id"},
			},
			"public.logs": {
				Schema:  "public",
				Name:    "logs",
				Kind:    "view",
				Columns: []*schema.Column{{Name: "id", TypeName: "integer"}, {Name: "message", TypeName: "text"}},
			},
			"public.nopk": {
				Schema:  "public",
				Name:    "nopk",
				Kind:    "table",
				Columns: []*schema.Column{{Name: "data", TypeName: "text"}},
			},
		},
		Schemas: []string{"public"},
	}
}

func testCacheHolder(sc *schema.SchemaCache) *schema.CacheHolder {
	ch := schema.NewCacheHolder(nil, slog.Default())
	if sc != nil {
		ch.SetForTesting(sc)
	}
	return ch
}

func testHandler(sc *schema.SchemaCache) http.Handler {
	ch := testCacheHolder(sc)
	h := NewHandler(nil, ch, slog.Default(), nil, nil)
	return h.Routes()
}

func doRequest(handler http.Handler, method, path string, body string) *httptest.ResponseRecorder {
	var r *http.Request
	if body != "" {
		r = httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	} else {
		r = httptest.NewRequest(method, path, nil)
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	return w
}

func decodeError(t *testing.T, w *httptest.ResponseRecorder) httputil.ErrorResponse {
	t.Helper()
	var resp httputil.ErrorResponse
	testutil.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	return resp
}

// --- Schema not ready ---

func TestListSchemaCacheNotReady(t *testing.T) {
	t.Parallel()
	h := testHandler(nil)
	w := doRequest(h, "GET", "/collections/users", "")
	testutil.Equal(t, http.StatusServiceUnavailable, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, resp.Message, "schema cache not ready")
}

func TestReadSchemaCacheNotReady(t *testing.T) {
	t.Parallel()
	h := testHandler(nil)
	w := doRequest(h, "GET", "/collections/users/123", "")
	testutil.Equal(t, http.StatusServiceUnavailable, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, resp.Message, "schema cache not ready")
}

// --- Collection not found ---

func TestListCollectionNotFound(t *testing.T) {
	t.Parallel()
	h := testHandler(testSchema())
	w := doRequest(h, "GET", "/collections/nonexistent", "")
	testutil.Equal(t, http.StatusNotFound, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, resp.Message, "collection not found")
}

func TestReadCollectionNotFound(t *testing.T) {
	t.Parallel()
	h := testHandler(testSchema())
	w := doRequest(h, "GET", "/collections/nonexistent/123", "")
	testutil.Equal(t, http.StatusNotFound, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, resp.Message, "collection not found")
}

func TestCreateCollectionNotFound(t *testing.T) {
	t.Parallel()
	h := testHandler(testSchema())
	w := doRequest(h, "POST", "/collections/nonexistent", `{"name":"test"}`)
	testutil.Equal(t, http.StatusNotFound, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, resp.Message, "collection not found")
}

// --- Write on view ---

func TestCreateOnViewNotAllowed(t *testing.T) {
	t.Parallel()
	h := testHandler(testSchema())
	w := doRequest(h, "POST", "/collections/logs", `{"message":"test"}`)
	testutil.Equal(t, http.StatusMethodNotAllowed, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, resp.Message, "write operations not allowed")
}

func TestUpdateOnViewNotAllowed(t *testing.T) {
	t.Parallel()
	h := testHandler(testSchema())
	w := doRequest(h, "PATCH", "/collections/logs/1", `{"message":"test"}`)
	testutil.Equal(t, http.StatusMethodNotAllowed, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, resp.Message, "write operations not allowed")
}

func TestDeleteOnViewNotAllowed(t *testing.T) {
	t.Parallel()
	h := testHandler(testSchema())
	w := doRequest(h, "DELETE", "/collections/logs/1", "")
	testutil.Equal(t, http.StatusMethodNotAllowed, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, resp.Message, "write operations not allowed")
}

// --- No primary key ---

func TestReadNoPrimaryKey(t *testing.T) {
	t.Parallel()
	h := testHandler(testSchema())
	w := doRequest(h, "GET", "/collections/nopk/1", "")
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, resp.Message, "no primary key")
}

func TestUpdateNoPrimaryKey(t *testing.T) {
	t.Parallel()
	h := testHandler(testSchema())
	w := doRequest(h, "PATCH", "/collections/nopk/1", `{"data":"test"}`)
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, resp.Message, "no primary key")
}

func TestDeleteNoPrimaryKey(t *testing.T) {
	t.Parallel()
	h := testHandler(testSchema())
	w := doRequest(h, "DELETE", "/collections/nopk/1", "")
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, resp.Message, "no primary key")
}

// --- Invalid body ---

func TestCreateEmptyBody(t *testing.T) {
	t.Parallel()
	h := testHandler(testSchema())
	w := doRequest(h, "POST", "/collections/users", `{}`)
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, resp.Message, "empty request body")
}

func TestCreateInvalidJSON(t *testing.T) {
	t.Parallel()
	h := testHandler(testSchema())
	w := doRequest(h, "POST", "/collections/users", `{invalid`)
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, resp.Message, "invalid JSON body")
}

func TestCreateNoRecognizedColumns(t *testing.T) {
	t.Parallel()
	h := testHandler(testSchema())
	w := doRequest(h, "POST", "/collections/users", `{"unknown_field":"value"}`)
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, resp.Message, "no recognized columns")
}

func TestUpdateEmptyBody(t *testing.T) {
	t.Parallel()
	h := testHandler(testSchema())
	w := doRequest(h, "PATCH", "/collections/users/123", `{}`)
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, resp.Message, "empty request body")
}

func TestUpdateInvalidJSON(t *testing.T) {
	t.Parallel()
	h := testHandler(testSchema())
	w := doRequest(h, "PATCH", "/collections/users/123", `not-json`)
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, resp.Message, "invalid JSON body")
}

// --- Invalid filter ---

func TestListInvalidFilter(t *testing.T) {
	t.Parallel()
	h := testHandler(testSchema())
	w := doRequest(h, "GET", "/collections/users?filter=((broken", "")
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, resp.Message, "invalid filter")
	testutil.Contains(t, resp.DocURL, "/guide/api-reference#filter-syntax")
}

// --- parseFields ---

func TestParseFieldsEmpty(t *testing.T) {
	t.Parallel()
	r := httptest.NewRequest("GET", "/?fields=", nil)
	fields := parseFields(r)
	testutil.Nil(t, fields)
}

func TestParseFieldsMultiple(t *testing.T) {
	t.Parallel()
	r := httptest.NewRequest("GET", "/?fields=id,email,name", nil)
	fields := parseFields(r)
	testutil.Equal(t, 3, len(fields))
	testutil.Equal(t, "id", fields[0])
	testutil.Equal(t, "email", fields[1])
	testutil.Equal(t, "name", fields[2])
}

func TestParseFieldsTrimsSpaces(t *testing.T) {
	t.Parallel()
	r := httptest.NewRequest("GET", "/?fields=+id+,+name+", nil)
	fields := parseFields(r)
	testutil.Equal(t, 2, len(fields))
	testutil.Equal(t, "id", fields[0])
	testutil.Equal(t, "name", fields[1])
}

// --- parseSortSQL ---

func TestParseSortSQLAscending(t *testing.T) {
	t.Parallel()
	sc := testSchema()
	tbl := sc.TableByName("users")
	result := parseSortSQL(tbl, "email")
	testutil.Equal(t, `"email" ASC`, result)
}

func TestParseSortSQLDescending(t *testing.T) {
	t.Parallel()
	sc := testSchema()
	tbl := sc.TableByName("users")
	result := parseSortSQL(tbl, "-email")
	testutil.Equal(t, `"email" DESC`, result)
}

func TestParseSortSQLSkipsUnknownColumns(t *testing.T) {
	t.Parallel()
	sc := testSchema()
	tbl := sc.TableByName("users")
	result := parseSortSQL(tbl, "nonexistent")
	testutil.Equal(t, "", result)
}

// --- Content-Type on responses ---

func TestErrorResponseIsJSON(t *testing.T) {
	t.Parallel()
	h := testHandler(testSchema())
	w := doRequest(h, "GET", "/collections/nonexistent", "")
	testutil.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var resp httputil.ErrorResponse
	err := json.NewDecoder(w.Body).Decode(&resp)
	testutil.NoError(t, err)
	testutil.Equal(t, http.StatusNotFound, resp.Code)
	testutil.Contains(t, resp.Message, "not found")
}

// --- API key scope enforcement ---

func doRequestWithClaims(handler http.Handler, method, path string, body string, claims *auth.Claims) *httptest.ResponseRecorder {
	var r *http.Request
	if body != "" {
		r = httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	} else {
		r = httptest.NewRequest(method, path, nil)
	}
	if claims != nil {
		ctx := auth.ContextWithClaims(r.Context(), claims)
		r = r.WithContext(ctx)
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	return w
}

func TestReadonlyScopeDeniesCreate(t *testing.T) {
	t.Parallel()
	h := testHandler(testSchema())
	claims := &auth.Claims{APIKeyScope: "readonly"}
	w := doRequestWithClaims(h, "POST", "/collections/users", `{"email":"a@b.com"}`, claims)
	testutil.Equal(t, http.StatusForbidden, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, resp.Message, "write operations")
}

func TestReadonlyScopeDeniesUpdate(t *testing.T) {
	t.Parallel()
	h := testHandler(testSchema())
	claims := &auth.Claims{APIKeyScope: "readonly"}
	w := doRequestWithClaims(h, "PATCH", "/collections/users/123", `{"email":"a@b.com"}`, claims)
	testutil.Equal(t, http.StatusForbidden, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, resp.Message, "write operations")
}

func TestReadonlyScopeDeniesDelete(t *testing.T) {
	t.Parallel()
	h := testHandler(testSchema())
	claims := &auth.Claims{APIKeyScope: "readonly"}
	w := doRequestWithClaims(h, "DELETE", "/collections/users/123", "", claims)
	testutil.Equal(t, http.StatusForbidden, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, resp.Message, "write operations")
}

// Removed: TestReadonlyScopeIsReadAllowed — tested auth.Claims directly without
// going through the handler. Covered by TestClaimsIsReadAllowed in auth/apikeys_test.go.

func TestTableScopeDeniesUnauthorizedTable(t *testing.T) {
	t.Parallel()
	h := testHandler(testSchema())
	claims := &auth.Claims{AllowedTables: []string{"logs"}}
	w := doRequestWithClaims(h, "GET", "/collections/users", "", claims)
	testutil.Equal(t, http.StatusForbidden, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, resp.Message, "does not have access to table")
}

// Removed: TestCheckTableScopeAllowsAuthorizedTable, TestCheckWriteScopeAllowsFullAccess,
// TestCheckWriteScopeAllowsReadwrite — tested auth package functions directly without
// going through the handler. Covered in auth/apikeys_test.go.

// --- App-scoped key negative tests (completion gate) ---

func TestAppScopedKeyDeniedOutOfScopeTable(t *testing.T) {
	// An app-scoped key restricted to "logs" must be denied access to "users".
	t.Parallel()
	h := testHandler(testSchema())
	claims := &auth.Claims{
		AppID:         "app-1",
		AllowedTables: []string{"logs"},
	}
	w := doRequestWithClaims(h, "GET", "/collections/users", "", claims)
	testutil.Equal(t, http.StatusForbidden, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, resp.Message, "does not have access to table")
}

func TestAppScopedReadonlyKeyDeniedWrite(t *testing.T) {
	// An app-scoped key with readonly scope must be denied write operations.
	t.Parallel()
	h := testHandler(testSchema())
	claims := &auth.Claims{
		AppID:       "app-1",
		APIKeyScope: "readonly",
	}
	w := doRequestWithClaims(h, "POST", "/collections/users", `{"name":"test"}`, claims)
	testutil.Equal(t, http.StatusForbidden, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, resp.Message, "does not permit write operations")
}

// --- Edge cases: primary key parsing ---

// --- API hardening limits ---

func TestListFilterTooLong(t *testing.T) {
	t.Parallel()
	h := testHandler(testSchema())
	longFilter := "name='" + strings.Repeat("a", maxFilterLen+1) + "'"
	w := doRequest(h, "GET", "/collections/users?filter="+longFilter, "")
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, resp.Message, "filter expression too long")
	testutil.Contains(t, resp.DocURL, "/guide/api-reference#filter-syntax")
}

func TestListSearchTooLong(t *testing.T) {
	t.Parallel()
	h := testHandler(testSchema())
	longSearch := strings.Repeat("a", maxSearchLen+1)
	w := doRequest(h, "GET", "/collections/users?search="+longSearch, "")
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, resp.Message, "search term too long")
	testutil.Contains(t, resp.DocURL, "/guide/api-reference#full-text-search")
}

func TestParseSortSQLMaxFieldsLimit(t *testing.T) {
	t.Parallel()
	sc := testSchema()
	tbl := sc.TableByName("users")
	// Build a sort string with more fields than the limit — only valid columns count.
	// "users" table has id, email, name — so we repeat them to exceed maxSortFields.
	parts := make([]string, 0, maxSortFields+5)
	cols := []string{"id", "email", "name"}
	for i := 0; i < maxSortFields+5; i++ {
		parts = append(parts, cols[i%len(cols)])
	}
	result := parseSortSQL(tbl, strings.Join(parts, ","))
	// Count commas to determine number of clauses.
	clauseCount := strings.Count(result, ",") + 1
	testutil.Equal(t, maxSortFields, clauseCount)
}

// maxPage clamping is exercised via integration tests.
// The constant-arithmetic assertions that were here tested nothing behavioral.

// --- Edge cases: primary key parsing ---

func TestReadCompositePKMissingValue(t *testing.T) {
	t.Parallel()
	sc := &schema.SchemaCache{
		Tables: map[string]*schema.Table{
			"public.composite": {
				Schema:     "public",
				Name:       "composite",
				Kind:       "table",
				PrimaryKey: []string{"user_id", "post_id"},
				Columns: []*schema.Column{
					{Name: "user_id", TypeName: "integer"},
					{Name: "post_id", TypeName: "integer"},
				},
			},
		},
	}
	h := testHandler(sc)
	// Composite PK requires both values: /collections/composite/123,456
	w := doRequest(h, "GET", "/collections/composite/123", "")
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, resp.Message, "invalid primary key: expected 2 values")
}
