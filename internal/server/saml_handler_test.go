package server_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestAdminSAMLRoutesRequireAdminToken(t *testing.T) {
	t.Parallel()
	srv, _ := authProvidersServerWithAuth(t, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/auth/saml", nil)
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAdminSAMLListWithoutPoolReturnsServiceUnavailable(t *testing.T) {
	t.Parallel()
	srv, token := authProvidersServerWithAuth(t, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/auth/saml", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestAdminSAMLCreateWithoutPoolReturnsServiceUnavailable(t *testing.T) {
	t.Parallel()
	srv, token := authProvidersServerWithAuth(t, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/auth/saml", strings.NewReader(`{"name":"okta","entity_id":"https://sp.example.com/okta","idp_metadata_xml":"<EntityDescriptor></EntityDescriptor>"}`))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusServiceUnavailable, w.Code)
}
