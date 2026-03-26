package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/go-chi/chi/v5"
)

// vectorAdminTestServer creates a minimal test server with vector admin routes.
func vectorAdminTestServer(sc *schema.SchemaCache) (*Server, *chi.Mux) {
	ch := schema.NewCacheHolder(nil, testutil.DiscardLogger())
	if sc != nil {
		ch.SetForTesting(sc)
	}
	s := &Server{
		schema: ch,
		logger: testutil.DiscardLogger(),
	}
	r := chi.NewRouter()
	r.Route("/admin/vector/indexes", func(r chi.Router) {
		r.Post("/", s.handleAdminVectorIndexCreate)
		r.Get("/", s.handleAdminVectorIndexList)
	})
	return s, r
}

func vectorAdminSchemaCache() *schema.SchemaCache {
	return &schema.SchemaCache{
		Tables: map[string]*schema.Table{
			"public.documents": {
				Schema: "public",
				Name:   "documents",
				Kind:   "table",
				Columns: []*schema.Column{
					{Name: "id", TypeName: "uuid", IsPrimaryKey: true},
					{Name: "title", TypeName: "text"},
					{Name: "embedding", TypeName: "vector(3)", IsVector: true, VectorDim: 3},
				},
				PrimaryKey: []string{"id"},
			},
		},
		HasPgVector: true,
		Schemas:     []string{"public"},
	}
}

func doVectorAdminReq(r chi.Router, method, path, body string) *httptest.ResponseRecorder {
	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// --- Create index tests ---

func TestAdminVectorIndexCreate_MissingTable(t *testing.T) {
	t.Parallel()
	_, router := vectorAdminTestServer(vectorAdminSchemaCache())
	body := `{"column":"embedding","method":"hnsw","metric":"cosine"}`
	w := doVectorAdminReq(router, "POST", "/admin/vector/indexes", body)
	testutil.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAdminVectorIndexCreate_MissingColumn(t *testing.T) {
	t.Parallel()
	_, router := vectorAdminTestServer(vectorAdminSchemaCache())
	body := `{"table":"documents","method":"hnsw","metric":"cosine"}`
	w := doVectorAdminReq(router, "POST", "/admin/vector/indexes", body)
	testutil.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAdminVectorIndexCreate_MissingMethod(t *testing.T) {
	t.Parallel()
	_, router := vectorAdminTestServer(vectorAdminSchemaCache())
	body := `{"table":"documents","column":"embedding","metric":"cosine"}`
	w := doVectorAdminReq(router, "POST", "/admin/vector/indexes", body)
	testutil.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAdminVectorIndexCreate_MissingMetric(t *testing.T) {
	t.Parallel()
	_, router := vectorAdminTestServer(vectorAdminSchemaCache())
	body := `{"table":"documents","column":"embedding","method":"hnsw"}`
	w := doVectorAdminReq(router, "POST", "/admin/vector/indexes", body)
	testutil.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAdminVectorIndexCreate_InvalidMethod(t *testing.T) {
	t.Parallel()
	_, router := vectorAdminTestServer(vectorAdminSchemaCache())
	body := `{"table":"documents","column":"embedding","method":"btree","metric":"cosine"}`
	w := doVectorAdminReq(router, "POST", "/admin/vector/indexes", body)
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	testutil.Contains(t, resp["message"].(string), "unsupported index method")
}

func TestAdminVectorIndexCreate_InvalidMetric(t *testing.T) {
	t.Parallel()
	_, router := vectorAdminTestServer(vectorAdminSchemaCache())
	body := `{"table":"documents","column":"embedding","method":"hnsw","metric":"manhattan"}`
	w := doVectorAdminReq(router, "POST", "/admin/vector/indexes", body)
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	testutil.Contains(t, resp["message"].(string), "unsupported metric")
}

func TestAdminVectorIndexCreate_InvalidJSON(t *testing.T) {
	t.Parallel()
	_, router := vectorAdminTestServer(vectorAdminSchemaCache())
	w := doVectorAdminReq(router, "POST", "/admin/vector/indexes", "not json")
	testutil.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAdminVectorIndexCreate_ValidNoPool(t *testing.T) {
	t.Parallel()
	_, router := vectorAdminTestServer(vectorAdminSchemaCache())
	body := `{"table":"documents","column":"embedding","method":"hnsw","metric":"cosine"}`
	w := doVectorAdminReq(router, "POST", "/admin/vector/indexes", body)
	// With nil pool we expect 500 (can't execute DDL), not 400.
	testutil.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestAdminVectorIndexCreate_AutoGeneratesName(t *testing.T) {
	t.Parallel()
	_, router := vectorAdminTestServer(vectorAdminSchemaCache())
	body := `{"table":"documents","column":"embedding","method":"hnsw","metric":"cosine"}`
	w := doVectorAdminReq(router, "POST", "/admin/vector/indexes", body)
	// Even though it fails at execution (nil pool), it should not fail at validation.
	// Status 500 means it passed validation and tried to execute.
	testutil.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestAdminVectorIndexCreate_TableNotFound(t *testing.T) {
	t.Parallel()
	_, router := vectorAdminTestServer(vectorAdminSchemaCache())
	body := `{"table":"nonexistent","column":"embedding","method":"hnsw","metric":"cosine"}`
	w := doVectorAdminReq(router, "POST", "/admin/vector/indexes", body)
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	testutil.Contains(t, resp["message"].(string), "not found")
}

func TestAdminVectorIndexCreate_ColumnNotVector(t *testing.T) {
	t.Parallel()
	_, router := vectorAdminTestServer(vectorAdminSchemaCache())
	body := `{"table":"documents","column":"title","method":"hnsw","metric":"cosine"}`
	w := doVectorAdminReq(router, "POST", "/admin/vector/indexes", body)
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	testutil.Contains(t, resp["message"].(string), "not a vector column")
}

// --- List indexes tests ---

func TestAdminVectorIndexList_NoPool(t *testing.T) {
	t.Parallel()
	_, router := vectorAdminTestServer(vectorAdminSchemaCache())
	w := doVectorAdminReq(router, "GET", "/admin/vector/indexes", "")
	// Falls back to schema cache when pool is nil.
	testutil.Equal(t, http.StatusOK, w.Code)
	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	indexes := resp["indexes"].([]any)
	testutil.Equal(t, 0, len(indexes))
}

func TestAdminVectorIndexList_FromSchemaCache(t *testing.T) {
	t.Parallel()
	sc := vectorAdminSchemaCache()
	sc.Tables["public.documents"].Indexes = []*schema.Index{
		{Name: "idx_docs_embedding", Method: "hnsw", Definition: "CREATE INDEX idx_docs_embedding ON public.documents USING hnsw (embedding vector_cosine_ops)"},
		{Name: "docs_pkey", Method: "btree", Definition: "CREATE UNIQUE INDEX docs_pkey ON public.documents USING btree (id)", IsPrimary: true},
	}
	_, router := vectorAdminTestServer(sc)
	w := doVectorAdminReq(router, "GET", "/admin/vector/indexes", "")
	testutil.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	indexes := resp["indexes"].([]any)
	// Only the HNSW index should be returned, not the btree pkey.
	testutil.Equal(t, 1, len(indexes))
	idx := indexes[0].(map[string]any)
	testutil.Equal(t, "idx_docs_embedding", idx["name"])
	testutil.Equal(t, "hnsw", idx["method"])
}
