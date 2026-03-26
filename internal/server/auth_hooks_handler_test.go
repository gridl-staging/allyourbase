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

func authHooksServerWithAuth(t *testing.T, hooks config.AuthHooks) (*server.Server, string) {
	t.Helper()
	cfg := config.Default()
	cfg.Admin.Password = "admin-pass"
	cfg.Auth.Enabled = true
	cfg.Auth.JWTSecret = "test-secret-that-is-at-least-32-chars-long"
	cfg.Auth.Hooks = hooks

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(nil, logger)
	authSvc := auth.NewService(nil, cfg.Auth.JWTSecret, 15*time.Minute, 7*24*time.Hour, 8, logger)
	srv := server.New(cfg, logger, ch, nil, authSvc, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/auth", strings.NewReader(`{"password":"admin-pass"}`))
	req.Header.Set("Content-Type", "application/json")
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)

	var loginResp map[string]string
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &loginResp))
	return srv, loginResp["token"]
}

func TestAdminAuthHooksGet_RequiresAdminToken(t *testing.T) {
	t.Parallel()
	srv, _ := authHooksServerWithAuth(t, config.AuthHooks{})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/auth/hooks", nil)
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAdminAuthHooksGet_NoAuth(t *testing.T) {
	t.Parallel()
	cfg := config.Default()
	cfg.Admin.Password = "admin-pass"
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(nil, logger)
	srv := server.New(cfg, logger, ch, nil, nil, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/auth", strings.NewReader(`{"password":"admin-pass"}`))
	req.Header.Set("Content-Type", "application/json")
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)

	var loginResp map[string]string
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &loginResp))

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/admin/auth/hooks", nil)
	req.Header.Set("Authorization", "Bearer "+loginResp["token"])
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusNotFound, w.Code)
}

func TestAdminAuthHooksGet_ReturnsConfiguredHooks(t *testing.T) {
	t.Parallel()
	hooks := config.AuthHooks{
		BeforeSignUp:        "before-signup-fn",
		AfterSignUp:         "after-signup-fn",
		CustomAccessToken:   "custom-token-fn",
		BeforePasswordReset: "before-reset-fn",
		SendEmail:           "send-email-fn",
		SendSMS:             "send-sms-fn",
	}
	srv, token := authHooksServerWithAuth(t, hooks)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/auth/hooks", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)

	var got config.AuthHooks
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	testutil.Equal(t, hooks.BeforeSignUp, got.BeforeSignUp)
	testutil.Equal(t, hooks.AfterSignUp, got.AfterSignUp)
	testutil.Equal(t, hooks.CustomAccessToken, got.CustomAccessToken)
	testutil.Equal(t, hooks.BeforePasswordReset, got.BeforePasswordReset)
	testutil.Equal(t, hooks.SendEmail, got.SendEmail)
	testutil.Equal(t, hooks.SendSMS, got.SendSMS)
}
