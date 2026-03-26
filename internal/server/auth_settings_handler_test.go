package server_test

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/server"
	"github.com/allyourbase/ayb/internal/testutil"
)

// authSettingsAdminToken creates a test server with admin password and returns it with a valid admin token.
func authSettingsAdminToken(t *testing.T) (*server.Server, string) {
	t.Helper()
	srv := newTestServerWithPassword(t, "admin-pass")
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/auth",
		strings.NewReader(`{"password":"admin-pass"}`))
	req.Header.Set("Content-Type", "application/json")
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)
	var body map[string]string
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	return srv, body["token"]
}

// authSettingsServerWithAuth creates a test server with auth enabled and returns it with a valid admin token.
func authSettingsServerWithAuth(t *testing.T) (*server.Server, string) {
	t.Helper()
	cfg := config.Default()
	cfg.Admin.Password = "admin-pass"
	cfg.Auth.Enabled = true
	cfg.Auth.JWTSecret = "test-secret-that-is-at-least-32-chars-long"
	cfg.Auth.TOTPEnabled = false
	cfg.Auth.AnonymousAuthEnabled = false
	cfg.Auth.EmailMFAEnabled = false

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(nil, logger)
	authSvc := auth.NewService(nil, cfg.Auth.JWTSecret, 15*time.Minute, 7*24*time.Hour, 8, logger)
	srv := server.New(cfg, logger, ch, nil, authSvc, nil)

	// Get admin token.
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/auth",
		strings.NewReader(`{"password":"admin-pass"}`))
	req.Header.Set("Content-Type", "application/json")
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)
	var loginResp map[string]string
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &loginResp))
	return srv, loginResp["token"]
}

func TestAuthSettingsGet_NoAuth(t *testing.T) {
	t.Parallel()
	// Server without auth service — should return 404.
	srv, token := authSettingsAdminToken(t)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/auth-settings", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusNotFound, w.Code)
}

func TestAuthSettingsGet_RequiresAdminToken(t *testing.T) {
	t.Parallel()
	srv, _ := authSettingsAdminToken(t)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/auth-settings", nil)
	// No auth header.
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAuthSettingsGet_ReturnsDefaults(t *testing.T) {
	t.Parallel()
	// Server WITH auth enabled — all toggles default to false.
	srv, token := authSettingsServerWithAuth(t)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/auth-settings", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)

	var settings auth.AuthSettings
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &settings))
	testutil.False(t, settings.MagicLinkEnabled)
	testutil.False(t, settings.SMSEnabled)
	testutil.False(t, settings.EmailMFAEnabled)
	testutil.False(t, settings.AnonymousAuthEnabled)
	testutil.False(t, settings.TOTPEnabled)
}

func TestAuthSettingsUpdate_RequiresAdminToken(t *testing.T) {
	t.Parallel()
	srv, _ := authSettingsAdminToken(t)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/admin/auth-settings",
		strings.NewReader(`{"totp_enabled":true}`))
	req.Header.Set("Content-Type", "application/json")
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAuthSettingsGetAndUpdate_WithAuthService(t *testing.T) {
	t.Parallel()
	srv, token := authSettingsServerWithAuth(t)

	// GET auth settings — all should be false by default.
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/auth-settings", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)

	var settings auth.AuthSettings
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &settings))
	testutil.False(t, settings.TOTPEnabled)
	testutil.False(t, settings.AnonymousAuthEnabled)
	testutil.False(t, settings.EmailMFAEnabled)

	// PUT to enable TOTP and anonymous auth.
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPut, "/api/admin/auth-settings",
		strings.NewReader(`{"totp_enabled":true,"anonymous_auth_enabled":true,"email_mfa_enabled":false,"sms_enabled":false,"magic_link_enabled":false}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)

	var updated auth.AuthSettings
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &updated))
	testutil.True(t, updated.TOTPEnabled)
	testutil.True(t, updated.AnonymousAuthEnabled)
	testutil.False(t, updated.EmailMFAEnabled)

	// GET again to confirm persistence.
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/admin/auth-settings", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)

	var confirmed auth.AuthSettings
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &confirmed))
	testutil.True(t, confirmed.TOTPEnabled)
	testutil.True(t, confirmed.AnonymousAuthEnabled)
}
