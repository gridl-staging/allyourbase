package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestRequireAdminTokenMiddleware(t *testing.T) {
	t.Parallel()
	okHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	t.Run("passthrough when admin auth not configured", func(t *testing.T) {
		t.Parallel()
		s := &Server{} // adminAuth is nil
		handler := s.requireAdminToken(okHandler)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		handler.ServeHTTP(w, req)

		testutil.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("valid token passes", func(t *testing.T) {
		t.Parallel()
		s := &Server{adminAuth: newAdminAuth("secret")}
		handler := s.requireAdminToken(okHandler)
		token := s.adminAuth.token()

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		handler.ServeHTTP(w, req)

		testutil.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("invalid token rejected", func(t *testing.T) {
		t.Parallel()
		s := &Server{adminAuth: newAdminAuth("secret")}
		handler := s.requireAdminToken(okHandler)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer bad-token")
		handler.ServeHTTP(w, req)

		testutil.Equal(t, http.StatusUnauthorized, w.Code)
		testutil.Contains(t, w.Body.String(), "admin authentication required")
	})

	t.Run("missing token rejected", func(t *testing.T) {
		t.Parallel()
		s := &Server{adminAuth: newAdminAuth("secret")}
		handler := s.requireAdminToken(okHandler)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		handler.ServeHTTP(w, req)

		testutil.Equal(t, http.StatusUnauthorized, w.Code)
	})
}
