package server

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/schema"
	"github.com/go-chi/chi/v5"
)

// openapiTestServer builds a minimal server with the openapi routes wired.
func openapiTestServer(sc *schema.SchemaCache) *Server {
	holder := schema.NewCacheHolder(nil, nil)
	holder.SetForTesting(sc)

	s := &Server{
		schema: holder,
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	r := chi.NewRouter()
	r.Get("/api/openapi.json", s.handleOpenAPIJSON)
	r.Get("/api/docs", handleDocs)
	s.router = r

	return s
}

func minimalSchemaCache() *schema.SchemaCache {
	return &schema.SchemaCache{
		Tables: map[string]*schema.Table{
			"public.users": {
				Schema: "public",
				Name:   "users",
				Kind:   "table",
				Columns: []*schema.Column{
					{Name: "id", TypeName: "integer", IsPrimaryKey: true},
					{Name: "email", TypeName: "text"},
				},
			},
		},
		Functions: map[string]*schema.Function{},
		Schemas:   []string{"public"},
	}
}

func TestHandleOpenAPIJSON_returnsJSON(t *testing.T) {
	t.Parallel()
	s := openapiTestServer(minimalSchemaCache())
	req := httptest.NewRequest(http.MethodGet, "/api/openapi.json", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	if !json.Valid(w.Body.Bytes()) {
		t.Error("response body is not valid JSON")
	}
}

func TestHandleOpenAPIJSON_etagPresent(t *testing.T) {
	t.Parallel()
	s := openapiTestServer(minimalSchemaCache())
	req := httptest.NewRequest(http.MethodGet, "/api/openapi.json", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	etag := w.Header().Get("ETag")
	if etag == "" {
		t.Error("ETag header must be present")
	}
	if !strings.HasPrefix(etag, `"`) || !strings.HasSuffix(etag, `"`) {
		t.Errorf("ETag %q should be a quoted string", etag)
	}
}

func TestHandleOpenAPIJSON_conditionalGet304(t *testing.T) {
	t.Parallel()
	s := openapiTestServer(minimalSchemaCache())

	// First request to get the ETag.
	req1 := httptest.NewRequest(http.MethodGet, "/api/openapi.json", nil)
	w1 := httptest.NewRecorder()
	s.router.ServeHTTP(w1, req1)
	etag := w1.Header().Get("ETag")
	if etag == "" {
		t.Fatal("first request must return ETag")
	}

	// Second request with matching If-None-Match.
	req2 := httptest.NewRequest(http.MethodGet, "/api/openapi.json", nil)
	req2.Header.Set("If-None-Match", etag)
	w2 := httptest.NewRecorder()
	s.router.ServeHTTP(w2, req2)

	if w2.Code != http.StatusNotModified {
		t.Errorf("status = %d, want 304 Not Modified", w2.Code)
	}
}

func TestHandleOpenAPIJSON_mismatchedEtagStillServes(t *testing.T) {
	t.Parallel()
	s := openapiTestServer(minimalSchemaCache())
	req := httptest.NewRequest(http.MethodGet, "/api/openapi.json", nil)
	req.Header.Set("If-None-Match", `"stale-etag-value"`)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 for mismatched ETag", w.Code)
	}
}

func TestHandleOpenAPIJSON_schemaNotReady(t *testing.T) {
	t.Parallel()
	// No schema cache set — Get() returns nil.
	holder := schema.NewCacheHolder(nil, nil)
	s := &Server{
		schema: holder,
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	r := chi.NewRouter()
	r.Get("/api/openapi.json", s.handleOpenAPIJSON)
	s.router = r

	req := httptest.NewRequest(http.MethodGet, "/api/openapi.json", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503 when schema not ready", w.Code)
	}
}

func TestHandleOpenAPIJSON_nilSchemaHolder(t *testing.T) {
	t.Parallel()
	s := &Server{
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	r := chi.NewRouter()
	r.Get("/api/openapi.json", s.handleOpenAPIJSON)
	s.router = r

	req := httptest.NewRequest(http.MethodGet, "/api/openapi.json", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503 when schema holder is nil", w.Code)
	}
}

func TestHandleOpenAPIJSON_responseIsValidOpenAPI(t *testing.T) {
	t.Parallel()
	s := openapiTestServer(minimalSchemaCache())
	req := httptest.NewRequest(http.MethodGet, "/api/openapi.json", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	var doc map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if doc["openapi"] != "3.1.0" {
		t.Errorf("openapi = %v, want 3.1.0", doc["openapi"])
	}
	if _, ok := doc["paths"]; !ok {
		t.Error("response must have paths key")
	}
	if _, ok := doc["info"]; !ok {
		t.Error("response must have info key")
	}
}

func TestHandleDocs_returnsHTML(t *testing.T) {
	t.Parallel()
	s := openapiTestServer(minimalSchemaCache())
	req := httptest.NewRequest(http.MethodGet, "/api/docs", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/html") {
		t.Errorf("Content-Type = %q, want text/html", ct)
	}
}

func TestHandleDocs_containsSwaggerUI(t *testing.T) {
	t.Parallel()
	s := openapiTestServer(minimalSchemaCache())
	req := httptest.NewRequest(http.MethodGet, "/api/docs", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	body := w.Body.String()
	if !strings.Contains(body, "swagger-ui") {
		t.Error("docs page must reference swagger-ui")
	}
	if !strings.Contains(body, "/api/openapi.json") {
		t.Error("docs page must reference /api/openapi.json")
	}
}

func TestHandleOpenAPIJSON_includesStage2Endpoints(t *testing.T) {
	t.Parallel()
	s := openapiTestServer(minimalSchemaCache())
	req := httptest.NewRequest(http.MethodGet, "/api/openapi.json", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	var doc map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	paths := doc["paths"].(map[string]any)

	stage2Paths := []string{
		"/api/collections/users/aggregate",
		"/api/collections/users/import",
		"/api/collections/users/export.csv",
		"/api/collections/users/export.json",
		"/api/collections/users/batch",
	}

	for _, p := range stage2Paths {
		pathItem, ok := paths[p].(map[string]any)
		if !ok {
			t.Errorf("missing Stage 2 path: %s", p)
			continue
		}
		hasMethod := false
		for _, method := range []string{"get", "post"} {
			if pathItem[method] != nil {
				hasMethod = true
				break
			}
		}
		if !hasMethod {
			t.Errorf("Stage 2 path %s has no HTTP operation", p)
		}
	}
}

func TestHandleOpenAPIJSON_includesComponentSchemas(t *testing.T) {
	t.Parallel()
	s := openapiTestServer(minimalSchemaCache())
	req := httptest.NewRequest(http.MethodGet, "/api/openapi.json", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	var doc map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	comps, ok := doc["components"].(map[string]any)
	if !ok {
		t.Fatal("components is missing")
	}

	schemas, ok := comps["schemas"].(map[string]any)
	if !ok {
		t.Fatal("components.schemas is missing or empty")
	}

	if len(schemas) == 0 {
		t.Error("components.schemas should not be empty")
	}

	expectedKeys := []string{"Users", "UsersCreate", "UsersWrite"}
	for _, key := range expectedKeys {
		if _, ok := schemas[key]; !ok {
			t.Errorf("components.schemas missing key %q", key)
		}
	}
}

func TestHandleOpenAPIJSON_operationsUseComponentRefs(t *testing.T) {
	t.Parallel()
	s := openapiTestServer(minimalSchemaCache())
	req := httptest.NewRequest(http.MethodGet, "/api/openapi.json", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	var doc map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	paths := doc["paths"].(map[string]any)
	usersPath := paths["/api/collections/users"].(map[string]any)
	listOp := usersPath["get"].(map[string]any)
	listResp := listOp["responses"].(map[string]any)["200"].(map[string]any)
	listContent := listResp["content"].(map[string]any)["application/json"].(map[string]any)
	listSchema := listContent["schema"].(map[string]any)
	listItems := listSchema["items"].(map[string]any)

	if ref := listItems["$ref"]; ref == "" {
		t.Error("list response items should use $ref to component schema")
	} else if !strings.HasPrefix(ref.(string), "#/components/schemas/") {
		t.Errorf("list response items $ref = %v, want #/components/schemas/...", ref)
	}
}
