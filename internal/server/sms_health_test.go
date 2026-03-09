package server_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestAdminSMSHealth_RequiresAdmin(t *testing.T) {
	t.Parallel()
	srv := newTestServerWithPassword(t, "testpass")

	// No auth header → 401.
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/sms/health", nil)
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusUnauthorized, w.Code)
	testutil.Contains(t, w.Body.String(), "admin authentication required")
}

func TestAdminSMSHealth_NoPool_Returns404(t *testing.T) {
	// When pool is nil (no DB) the handler returns 404.
	t.Parallel()
	srv := newTestServerWithPassword(t, "testpass")
	token := adminLogin(t, srv)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/sms/health", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusNotFound, w.Code)
}
