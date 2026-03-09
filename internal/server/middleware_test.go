package server_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/server"
	"github.com/allyourbase/ayb/internal/testutil"
)

// --- CORS tests ---

func TestCORSHeaders(t *testing.T) {
	t.Parallel()
	cfg := config.Default()
	cfg.Server.CORSAllowedOrigins = []string{"http://example.com", "http://other.com"}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(nil, logger)
	srv := server.New(cfg, logger, ch, nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("Origin", "http://example.com")
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	testutil.Equal(t, "http://example.com", w.Header().Get("Access-Control-Allow-Origin"))
	testutil.Contains(t, w.Header().Get("Vary"), "Origin")
	testutil.Contains(t, w.Header().Get("Access-Control-Allow-Methods"), "GET")
	testutil.Contains(t, w.Header().Get("Access-Control-Allow-Methods"), "POST")
	testutil.Contains(t, w.Header().Get("Access-Control-Allow-Methods"), "PUT")
	testutil.Contains(t, w.Header().Get("Access-Control-Allow-Methods"), "DELETE")
	testutil.Contains(t, w.Header().Get("Access-Control-Allow-Headers"), "Content-Type")
	testutil.Contains(t, w.Header().Get("Access-Control-Allow-Headers"), "Authorization")
}

func TestCORSMultiOriginSecondMatch(t *testing.T) {
	t.Parallel()
	cfg := config.Default()
	cfg.Server.CORSAllowedOrigins = []string{"http://example.com", "http://other.com"}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(nil, logger)
	srv := server.New(cfg, logger, ch, nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("Origin", "http://other.com")
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	testutil.Equal(t, "http://other.com", w.Header().Get("Access-Control-Allow-Origin"))
	testutil.Contains(t, w.Header().Get("Vary"), "Origin")
}

func TestCORSNonMatchingOrigin(t *testing.T) {
	t.Parallel()
	cfg := config.Default()
	cfg.Server.CORSAllowedOrigins = []string{"http://example.com"}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(nil, logger)
	srv := server.New(cfg, logger, ch, nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("Origin", "http://evil.com")
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	testutil.Equal(t, "", w.Header().Get("Access-Control-Allow-Origin"))
}

func TestCORSNoOriginHeader(t *testing.T) {
	t.Parallel()
	cfg := config.Default()
	cfg.Server.CORSAllowedOrigins = []string{"http://example.com"}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(nil, logger)
	srv := server.New(cfg, logger, ch, nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	testutil.Equal(t, "", w.Header().Get("Access-Control-Allow-Origin"))
	testutil.Contains(t, w.Header().Get("Access-Control-Allow-Methods"), "GET")
}

func TestCORSPreflight(t *testing.T) {
	t.Parallel()
	cfg := config.Default() // defaults to ["*"]
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(nil, logger)
	srv := server.New(cfg, logger, ch, nil, nil, nil)

	req := httptest.NewRequest(http.MethodOptions, "/api/schema", nil)
	req.Header.Set("Origin", "http://any-origin.com")
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	testutil.Equal(t, http.StatusNoContent, w.Code)
	testutil.Equal(t, "*", w.Header().Get("Access-Control-Allow-Origin"))
	testutil.Equal(t, "86400", w.Header().Get("Access-Control-Max-Age"))
}

func TestCORSPreflightSpecificOrigin(t *testing.T) {
	t.Parallel()
	cfg := config.Default()
	cfg.Server.CORSAllowedOrigins = []string{"http://example.com"}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(nil, logger)
	srv := server.New(cfg, logger, ch, nil, nil, nil)

	req := httptest.NewRequest(http.MethodOptions, "/api/schema", nil)
	req.Header.Set("Origin", "http://example.com")
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	testutil.Equal(t, http.StatusNoContent, w.Code)
	testutil.Equal(t, "http://example.com", w.Header().Get("Access-Control-Allow-Origin"))
	testutil.Contains(t, w.Header().Get("Vary"), "Origin")
}

func TestCORSWildcard(t *testing.T) {
	t.Parallel()
	cfg := config.Default() // defaults to ["*"]
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(nil, logger)
	srv := server.New(cfg, logger, ch, nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("Origin", "http://any-origin.com")
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	testutil.Equal(t, "*", w.Header().Get("Access-Control-Allow-Origin"))
	testutil.Equal(t, "", w.Header().Get("Vary"))
}

// --- CORS preflight on OAuth provider endpoints ---

func newServerWithAuth(t *testing.T) *server.Server {
	t.Helper()
	cfg := config.Default()
	cfg.Auth.Enabled = true
	cfg.Auth.JWTSecret = "test-secret-for-cors-tests"
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(nil, logger)
	authSvc := auth.NewService(nil, cfg.Auth.JWTSecret, time.Hour, 7*24*time.Hour, 8, logger)
	return server.New(cfg, logger, ch, nil, authSvc, nil)
}

func TestCORSPreflightOAuthTokenEndpoint(t *testing.T) {
	t.Parallel()
	srv := newServerWithAuth(t)

	req := httptest.NewRequest(http.MethodOptions, "/api/auth/token", nil)
	req.Header.Set("Origin", "http://spa-client.example.com")
	req.Header.Set("Access-Control-Request-Method", "POST")
	req.Header.Set("Access-Control-Request-Headers", "Content-Type")
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	testutil.Equal(t, http.StatusNoContent, w.Code)
	testutil.Equal(t, "*", w.Header().Get("Access-Control-Allow-Origin"))
	testutil.Contains(t, w.Header().Get("Access-Control-Allow-Methods"), "POST")
	testutil.Contains(t, w.Header().Get("Access-Control-Allow-Headers"), "Content-Type")
	testutil.Equal(t, "86400", w.Header().Get("Access-Control-Max-Age"))
}

func TestCORSPreflightOAuthRevokeEndpoint(t *testing.T) {
	t.Parallel()
	srv := newServerWithAuth(t)

	req := httptest.NewRequest(http.MethodOptions, "/api/auth/revoke", nil)
	req.Header.Set("Origin", "http://spa-client.example.com")
	req.Header.Set("Access-Control-Request-Method", "POST")
	req.Header.Set("Access-Control-Request-Headers", "Content-Type, Authorization")
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	testutil.Equal(t, http.StatusNoContent, w.Code)
	testutil.Equal(t, "*", w.Header().Get("Access-Control-Allow-Origin"))
	testutil.Contains(t, w.Header().Get("Access-Control-Allow-Methods"), "POST")
	testutil.Contains(t, w.Header().Get("Access-Control-Allow-Headers"), "Authorization")
}

func TestCORSPostOAuthTokenEndpointWithOrigin(t *testing.T) {
	t.Parallel()
	cfg := config.Default()
	cfg.Server.CORSAllowedOrigins = []string{"http://spa-client.example.com"}
	cfg.Auth.Enabled = true
	cfg.Auth.JWTSecret = "test-secret-for-cors"
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(nil, logger)
	authSvc := auth.NewService(nil, cfg.Auth.JWTSecret, time.Hour, 7*24*time.Hour, 8, logger)
	srv := server.New(cfg, logger, ch, nil, authSvc, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/auth/token", nil)
	req.Header.Set("Origin", "http://spa-client.example.com")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	// The actual response may be an error (missing params), but the CORS headers
	// must be present so the browser allows the SPA to read the response.
	testutil.Equal(t, "http://spa-client.example.com", w.Header().Get("Access-Control-Allow-Origin"))
	testutil.Contains(t, w.Header().Get("Vary"), "Origin")
}

// TestRequestIDHeader removed — never tested request IDs (no X-Request-Id middleware
// exists). Was just a duplicate of TestHealthEndpoint in server_test.go.

// --- Admin SPA ---

func TestAdminPathServesHTML(t *testing.T) {
	t.Parallel()
	cfg := config.Default()
	cfg.Admin.Enabled = true
	cfg.Admin.Path = "/admin"
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(nil, logger)
	srv := server.New(cfg, logger, ch, nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/admin/", nil)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)
	testutil.Contains(t, w.Header().Get("Content-Type"), "text/html")
}

func TestAdminSPAFallback(t *testing.T) {
	t.Parallel()
	cfg := config.Default()
	cfg.Admin.Enabled = true
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(nil, logger)
	srv := server.New(cfg, logger, ch, nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/admin/some/deep/route", nil)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)
	testutil.Contains(t, w.Header().Get("Content-Type"), "text/html")
	testutil.Contains(t, w.Body.String(), "<!DOCTYPE html>")
}

func TestAdminStaticAssetCacheHeaders(t *testing.T) {
	t.Parallel()
	cfg := config.Default()
	cfg.Admin.Enabled = true
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(nil, logger)
	srv := server.New(cfg, logger, ch, nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/admin/", nil)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)
	testutil.Equal(t, "", w.Header().Get("Cache-Control"))
}

func TestAdminDisabled(t *testing.T) {
	t.Parallel()
	cfg := config.Default()
	cfg.Admin.Enabled = false
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(nil, logger)
	srv := server.New(cfg, logger, ch, nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/admin/", nil)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	testutil.Equal(t, http.StatusNotFound, w.Code)
}

// --- StartWithReady ---

func TestStartWithReadySignalsReady(t *testing.T) {
	t.Parallel()
	cfg := config.Default()
	cfg.Server.Host = "127.0.0.1"
	cfg.Server.Port = 19876
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(nil, logger)
	srv := server.New(cfg, logger, ch, nil, nil, nil)

	ready := make(chan struct{})
	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.StartWithReady(ready)
	}()

	select {
	case <-ready:
		// Verify the server is actually serving HTTP after the ready signal.
		resp, err := http.Get("http://127.0.0.1:19876/health")
		testutil.NoError(t, err)
		resp.Body.Close()
		testutil.Equal(t, http.StatusOK, resp.StatusCode)

		err = srv.Shutdown(context.Background())
		testutil.NoError(t, err)
	case err := <-errCh:
		t.Fatalf("server failed to start: %v", err)
	}
}
