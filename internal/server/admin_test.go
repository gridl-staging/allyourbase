package server_test

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/server"
	"github.com/allyourbase/ayb/internal/testutil"
)

func newTestServerWithPassword(t *testing.T, password string) *server.Server {
	t.Helper()
	cfg := config.Default()
	cfg.Admin.Password = password
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(nil, logger)
	return server.New(cfg, logger, ch, nil, nil, nil)
}

func TestAdminStatusNoPassword(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, newCacheHolderWithSchema(nil))

	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/admin/status", nil))

	testutil.Equal(t, http.StatusOK, w.Code)
	var body map[string]bool
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	testutil.False(t, body["auth"])
}

func TestAdminStatusWithPassword(t *testing.T) {
	t.Parallel()
	srv := newTestServerWithPassword(t, "secret123")

	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/admin/status", nil))

	testutil.Equal(t, http.StatusOK, w.Code)
	var body map[string]bool
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	testutil.True(t, body["auth"])
}

func TestAdminLoginSuccess(t *testing.T) {
	t.Parallel()
	srv := newTestServerWithPassword(t, "mypassword")

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/auth", strings.NewReader(`{"password":"mypassword"}`))
	req.Header.Set("Content-Type", "application/json")
	srv.Router().ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)
	var body map[string]string
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	testutil.True(t, body["token"] != "", "expected non-empty token")
}

func TestAdminLoginWrongPassword(t *testing.T) {
	t.Parallel()
	srv := newTestServerWithPassword(t, "mypassword")

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/auth", strings.NewReader(`{"password":"wrong"}`))
	req.Header.Set("Content-Type", "application/json")
	srv.Router().ServeHTTP(w, req)

	testutil.Equal(t, http.StatusUnauthorized, w.Code)
	testutil.Contains(t, w.Body.String(), "invalid password")
}

func TestAdminLoginNotConfigured(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, newCacheHolderWithSchema(nil))

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/auth", strings.NewReader(`{"password":"any"}`))
	req.Header.Set("Content-Type", "application/json")
	srv.Router().ServeHTTP(w, req)

	testutil.Equal(t, http.StatusNotFound, w.Code)
}

func TestAdminLoginEmptyJSON(t *testing.T) {
	t.Parallel()
	srv := newTestServerWithPassword(t, "mypassword")

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/auth", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	srv.Router().ServeHTTP(w, req)

	testutil.Equal(t, http.StatusUnauthorized, w.Code)
	testutil.Contains(t, w.Body.String(), "invalid password")
}

func TestAdminSQLRouteNotRegisteredWithoutPool(t *testing.T) {
	// When pool is nil, the admin SQL route should not be registered.
	t.Parallel()

	srv := newTestServerWithPassword(t, "pass")

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/sql/", strings.NewReader(`{"query":"SELECT 1"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer some-token")
	srv.Router().ServeHTTP(w, req)

	testutil.Equal(t, http.StatusNotFound, w.Code)
}

func TestAdminTokenConsistency(t *testing.T) {
	t.Parallel()
	srv := newTestServerWithPassword(t, "pass")

	// Login twice, should get same token (deterministic HMAC).
	login := func() string {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/admin/auth", strings.NewReader(`{"password":"pass"}`))
		req.Header.Set("Content-Type", "application/json")
		srv.Router().ServeHTTP(w, req)
		testutil.Equal(t, http.StatusOK, w.Code)
		var body map[string]string
		testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		token := body["token"]
		testutil.True(t, token != "", "expected non-empty token")
		return token
	}

	t1 := login()
	t2 := login()
	testutil.Equal(t, t1, t2)
	testutil.Equal(t, 64, len(t1))
}
