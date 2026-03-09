package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestHandleCreateAPIKeyNoAuth(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	router := h.Routes()

	req := httptest.NewRequest(http.MethodPost, "/api-keys/",
		strings.NewReader(`{"name":"test"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// RequireAuth middleware rejects before handler runs.
	testutil.Equal(t, http.StatusUnauthorized, w.Code)
	testutil.Contains(t, w.Body.String(), "missing or invalid authorization")
}

func TestHandleCreateAPIKeyMissingName(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	router := h.Routes()
	token := generateTestToken(t, svc, "user-1", "test@example.com")

	req := httptest.NewRequest(http.MethodPost, "/api-keys/",
		strings.NewReader(`{"name":""}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "name is required")
}

func TestHandleCreateAPIKeyEmptyBody(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	router := h.Routes()
	token := generateTestToken(t, svc, "user-1", "test@example.com")

	req := httptest.NewRequest(http.MethodPost, "/api-keys/",
		strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "name is required")
}

func TestHandleCreateAPIKeyMalformedJSON(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	router := h.Routes()
	token := generateTestToken(t, svc, "user-1", "test@example.com")

	req := httptest.NewRequest(http.MethodPost, "/api-keys/",
		strings.NewReader("{bad json"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "invalid JSON body")
}

func TestHandleListAPIKeysNoAuth(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	router := h.Routes()

	req := httptest.NewRequest(http.MethodGet, "/api-keys/", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusUnauthorized, w.Code)
	testutil.Contains(t, w.Body.String(), "missing or invalid authorization")
}

func TestHandleRevokeAPIKeyNoAuth(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	router := h.Routes()

	req := httptest.NewRequest(http.MethodDelete, "/api-keys/some-id", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusUnauthorized, w.Code)
	testutil.Contains(t, w.Body.String(), "missing or invalid authorization")
}

func TestHandleRevokeAPIKeyInvalidUUID(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	router := h.Routes()
	token := generateTestToken(t, svc, "user-1", "test@example.com")

	req := httptest.NewRequest(http.MethodDelete, "/api-keys/not-a-uuid", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// UUID validation should return 400, not reach the service (which would 500 on a bad UUID).
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "invalid api key id format")
}

func TestHandleCreateAPIKeyInvalidScope(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	router := h.Routes()
	token := generateTestToken(t, svc, "user-1", "test@example.com")

	req := httptest.NewRequest(http.MethodPost, "/api-keys/",
		strings.NewReader(`{"name":"my-key","scope":"admin"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "invalid scope")
	testutil.Contains(t, w.Body.String(), "doc_url")
}

func TestHandleAPIKeyRoutesRegistered(t *testing.T) {
	// Verify all three API key routes are registered and accessible.
	// Wrong methods should return 405 (Method Not Allowed), not 404.
	t.Parallel()

	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	router := h.Routes()

	tests := []struct {
		name   string
		method string
		path   string
	}{
		{"GET on POST-only create", http.MethodGet, "/api-keys/"},
		{"DELETE on collection", http.MethodDelete, "/api-keys/"},
		{"POST on delete endpoint", http.MethodPost, "/api-keys/some-id"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest(tt.method, tt.path, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			// Chi returns 405 for wrong method on a registered route.
			// 401 from RequireAuth is also acceptable — it means the route
			// matched and the middleware ran (which proves registration).
			if w.Code == http.StatusNotFound {
				t.Errorf("route %s %s returned 404; expected route to be registered", tt.method, tt.path)
			}
		})
	}
}
