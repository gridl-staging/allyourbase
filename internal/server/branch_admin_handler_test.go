package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/branching"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/go-chi/chi/v5"
)

// mockBranchAdmin implements branchAdmin for testing.
type mockBranchAdmin struct {
	branches []branching.BranchRecord
	createFn func(ctx context.Context, name, sourceURL string) (*branching.BranchRecord, error)
	deleteFn func(ctx context.Context, name string) error
	listFn   func(ctx context.Context) ([]branching.BranchRecord, error)
}

func (m *mockBranchAdmin) Create(ctx context.Context, name, sourceURL string) (*branching.BranchRecord, error) {
	if m.createFn != nil {
		return m.createFn(ctx, name, sourceURL)
	}
	rec := &branching.BranchRecord{
		ID:             "test-id",
		Name:           name,
		SourceDatabase: "main_db",
		BranchDatabase: branching.BranchDBName(name),
		Status:         branching.StatusReady,
	}
	return rec, nil
}

func (m *mockBranchAdmin) Delete(ctx context.Context, name string) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, name)
	}
	return nil
}

func (m *mockBranchAdmin) List(ctx context.Context) ([]branching.BranchRecord, error) {
	if m.listFn != nil {
		return m.listFn(ctx)
	}
	return m.branches, nil
}

func branchTestServer(mock *mockBranchAdmin) *Server {
	return &Server{branchService: mock}
}

type branchAdminRouteFixture struct {
	server *Server
	router *chi.Mux
}

func newBranchAdminRouteFixture(mock *mockBranchAdmin) branchAdminRouteFixture {
	s := &Server{branchService: mock, adminAuth: newAdminAuth("secret")}
	r := chi.NewRouter()
	r.Route("/api/admin/branches", func(r chi.Router) {
		r.Use(s.requireAdminToken)
		r.Get("/", s.handleAdminBranchList)
		r.Post("/", s.handleAdminBranchCreate)
		r.Delete("/{name}", s.handleAdminBranchDelete)
	})
	return branchAdminRouteFixture{server: s, router: r}
}

func (f branchAdminRouteFixture) validToken() string {
	return f.server.adminAuth.token()
}

func TestHandleAdminBranchList_success(t *testing.T) {
	t.Parallel()
	mock := &mockBranchAdmin{
		branches: []branching.BranchRecord{
			{ID: "1", Name: "feature-a", Status: branching.StatusReady},
			{ID: "2", Name: "feature-b", Status: branching.StatusCreating},
		},
	}
	s := branchTestServer(mock)
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/admin/branches", nil)
	r.Header.Set("Authorization", "Bearer test-token")

	s.handleAdminBranchList(w, r)

	testutil.StatusCode(t, http.StatusOK, w.Code)
	var body map[string]any
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	branches, ok := body["branches"].([]any)
	testutil.True(t, ok, "expected branches array")
	testutil.Equal(t, 2, len(branches))
}

func TestHandleAdminBranchList_nilService(t *testing.T) {
	t.Parallel()
	s := &Server{}
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/admin/branches", nil)

	s.handleAdminBranchList(w, r)

	testutil.StatusCode(t, http.StatusOK, w.Code)
	var body map[string]any
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	branches, ok := body["branches"].([]any)
	testutil.True(t, ok, "expected branches array")
	testutil.Equal(t, 0, len(branches))
}

func TestHandleAdminBranchList_requiresAuth(t *testing.T) {
	t.Parallel()
	f := newBranchAdminRouteFixture(&mockBranchAdmin{})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/branches", nil)
	f.router.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusUnauthorized, w.Code)
	testutil.Contains(t, w.Body.String(), "admin authentication required")
}

func TestHandleAdminBranchCreate_success(t *testing.T) {
	t.Parallel()
	mock := &mockBranchAdmin{}
	s := branchTestServer(mock)

	w := httptest.NewRecorder()
	body := `{"name":"feature-test"}`
	r := httptest.NewRequest("POST", "/api/admin/branches", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Authorization", "Bearer test-token")

	s.handleAdminBranchCreate(w, r)

	testutil.StatusCode(t, http.StatusCreated, w.Code)
	var resp map[string]any
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	testutil.Equal(t, "feature-test", resp["name"])
}

func TestHandleAdminBranchCreate_requiresAuth(t *testing.T) {
	t.Parallel()
	mock := &mockBranchAdmin{}
	f := newBranchAdminRouteFixture(mock)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/branches", strings.NewReader(`{"name":"feature-auth"}`))
	req.Header.Set("Content-Type", "application/json")
	f.router.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusUnauthorized, w.Code)
	testutil.Contains(t, w.Body.String(), "admin authentication required")
}

func TestHandleAdminBranchCreate_missingName(t *testing.T) {
	t.Parallel()
	mock := &mockBranchAdmin{}
	s := branchTestServer(mock)

	w := httptest.NewRecorder()
	body := `{}`
	r := httptest.NewRequest("POST", "/api/admin/branches", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")

	s.handleAdminBranchCreate(w, r)

	testutil.StatusCode(t, http.StatusBadRequest, w.Code)
}

func TestHandleAdminBranchCreate_invalidName(t *testing.T) {
	t.Parallel()
	mock := &mockBranchAdmin{}
	f := newBranchAdminRouteFixture(mock)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/branches", strings.NewReader(`{"name":"INVALID"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+f.validToken())
	f.router.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusBadRequest, w.Code)
}

func TestHandleAdminBranchCreate_conflict(t *testing.T) {
	t.Parallel()
	mock := &mockBranchAdmin{
		createFn: func(_ context.Context, name, _ string) (*branching.BranchRecord, error) {
			return nil, fmt.Errorf("branch %q already exists (status: ready)", name)
		},
	}
	s := branchTestServer(mock)

	w := httptest.NewRecorder()
	body := `{"name":"existing"}`
	r := httptest.NewRequest("POST", "/api/admin/branches", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")

	s.handleAdminBranchCreate(w, r)

	testutil.StatusCode(t, http.StatusConflict, w.Code)
}

func TestHandleAdminBranchCreate_conflictViaRoute(t *testing.T) {
	t.Parallel()
	mock := &mockBranchAdmin{
		createFn: func(_ context.Context, name, _ string) (*branching.BranchRecord, error) {
			return nil, fmt.Errorf("branch %q already exists (status: ready)", name)
		},
	}
	f := newBranchAdminRouteFixture(mock)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/branches", strings.NewReader(`{"name":"existing"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+f.validToken())
	f.router.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusConflict, w.Code)
}

func TestHandleAdminBranchCreate_nilService(t *testing.T) {
	t.Parallel()
	s := &Server{}

	w := httptest.NewRecorder()
	body := `{"name":"test"}`
	r := httptest.NewRequest("POST", "/api/admin/branches", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")

	s.handleAdminBranchCreate(w, r)

	testutil.StatusCode(t, http.StatusServiceUnavailable, w.Code)
}

func TestHandleAdminBranchDelete_success(t *testing.T) {
	t.Parallel()
	mock := &mockBranchAdmin{}
	s := branchTestServer(mock)

	w := httptest.NewRecorder()
	r := httptest.NewRequest("DELETE", "/api/admin/branches/feature-test", nil)
	r.Header.Set("Authorization", "Bearer test-token")
	r.SetPathValue("name", "feature-test")

	s.handleAdminBranchDelete(w, r)

	testutil.StatusCode(t, http.StatusOK, w.Code)
}

func TestHandleAdminBranchDelete_requiresAuth(t *testing.T) {
	t.Parallel()
	f := newBranchAdminRouteFixture(&mockBranchAdmin{})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/admin/branches/feature-auth", nil)
	f.router.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusUnauthorized, w.Code)
	testutil.Contains(t, w.Body.String(), "admin authentication required")
}

func TestHandleAdminBranchDelete_notFound(t *testing.T) {
	t.Parallel()
	mock := &mockBranchAdmin{
		deleteFn: func(_ context.Context, name string) error {
			return fmt.Errorf("branch %q not found", name)
		},
	}
	s := branchTestServer(mock)

	w := httptest.NewRecorder()
	r := httptest.NewRequest("DELETE", "/api/admin/branches/nonexistent", nil)
	r.SetPathValue("name", "nonexistent")

	s.handleAdminBranchDelete(w, r)

	testutil.StatusCode(t, http.StatusNotFound, w.Code)
}

func TestHandleAdminBranchDelete_notFoundViaRoute(t *testing.T) {
	t.Parallel()
	mock := &mockBranchAdmin{
		deleteFn: func(_ context.Context, name string) error {
			return fmt.Errorf("branch %q not found", name)
		},
	}
	f := newBranchAdminRouteFixture(mock)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/admin/branches/nonexistent", nil)
	req.Header.Set("Authorization", "Bearer "+f.validToken())
	f.router.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusNotFound, w.Code)
}

func TestHandleAdminBranchDelete_nilService(t *testing.T) {
	t.Parallel()
	s := &Server{}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("DELETE", "/api/admin/branches/test", nil)
	r.SetPathValue("name", "test")

	s.handleAdminBranchDelete(w, r)

	testutil.StatusCode(t, http.StatusServiceUnavailable, w.Code)
}
