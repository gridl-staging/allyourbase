package server_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"io"
	"log/slog"
	"math/big"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/server"
	"github.com/allyourbase/ayb/internal/storage"
	"github.com/allyourbase/ayb/internal/testutil"
)

// generateTestTLSConfig creates a self-signed cert for use in tests.
// No network required — purely in-process.
func generateTestTLSConfig(t *testing.T) *tls.Config {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	testutil.NoError(t, err)

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "localhost"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &priv.PublicKey, priv)
	testutil.NoError(t, err)

	privDER, err := x509.MarshalECPrivateKey(priv)
	testutil.NoError(t, err)

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: privDER})

	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	testutil.NoError(t, err)

	return &tls.Config{Certificates: []tls.Certificate{cert}}
}

func newTestServer(t *testing.T, schemaCache *schema.CacheHolder) *server.Server {
	t.Helper()
	cfg := config.Default()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return server.New(cfg, logger, schemaCache, nil, nil, nil)
}

// newCacheHolderWithSchema creates a CacheHolder with an optional pre-loaded schema for tests.
func newCacheHolderWithSchema(sc *schema.SchemaCache) *schema.CacheHolder {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(nil, logger)
	if sc != nil {
		ch.SetForTesting(sc)
	}
	return ch
}

func TestHealthEndpoint(t *testing.T) {
	t.Parallel()
	ch := newCacheHolderWithSchema(nil)
	srv := newTestServer(t, ch)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)
	testutil.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var body map[string]string
	err := json.Unmarshal(w.Body.Bytes(), &body)
	testutil.NoError(t, err)
	testutil.Equal(t, "ok", body["status"])
}

func TestSchemaEndpointNotReady(t *testing.T) {
	t.Parallel()
	ch := newCacheHolderWithSchema(nil)
	srv := newTestServer(t, ch)

	req := httptest.NewRequest(http.MethodGet, "/api/schema", nil)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	testutil.Equal(t, http.StatusServiceUnavailable, w.Code)

	var body struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	err := json.Unmarshal(w.Body.Bytes(), &body)
	testutil.NoError(t, err)
	testutil.Equal(t, 503, body.Code)
	testutil.Contains(t, body.Message, "schema cache not ready")
}

func TestSchemaEndpointReady(t *testing.T) {
	t.Parallel()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(nil, logger)
	ch.SetForTesting(&schema.SchemaCache{
		Tables: map[string]*schema.Table{
			"public.users": {Schema: "public", Name: "users", Kind: "table"},
		},
		Schemas: []string{"public"},
		BuiltAt: time.Now(),
	})

	srv := newTestServer(t, ch)

	req := httptest.NewRequest(http.MethodGet, "/api/schema", nil)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)
	testutil.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var body map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &body)
	testutil.NoError(t, err)
	tables, ok := body["tables"].(map[string]any)
	testutil.True(t, ok, "tables should be a map")
	testutil.Equal(t, 1, len(tables))
	usersRaw, ok := tables["public.users"].(map[string]any)
	testutil.True(t, ok, "public.users should be a map")
	testutil.Equal(t, "users", usersRaw["name"])
	testutil.Equal(t, "public", usersRaw["schema"])
	testutil.Equal(t, "table", usersRaw["kind"])
}

func TestRouterSetup(t *testing.T) {
	t.Parallel()
	ch := newCacheHolderWithSchema(nil)
	srv := newTestServer(t, ch)

	// Health endpoint exists.
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)

	// Schema endpoint exists (returns 503 since cache not loaded).
	req = httptest.NewRequest(http.MethodGet, "/api/schema", nil)
	w = httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusServiceUnavailable, w.Code)

	// Unknown API route returns 404.
	req = httptest.NewRequest(http.MethodGet, "/api/nonexistent", nil)
	w = httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusNotFound, w.Code)
}

// TestCacheHolderGetBeforeLoad verifies that Get() returns nil before Load().
func TestCacheHolderGetBeforeLoad(t *testing.T) {
	t.Parallel()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(nil, logger)

	got := ch.Get()
	testutil.Nil(t, got)
}

func TestOpenAPISpecEndpoint(t *testing.T) {
	t.Parallel()
	ch := newCacheHolderWithSchema(nil)
	srv := newTestServer(t, ch)

	req := httptest.NewRequest(http.MethodGet, "/api/openapi.yaml", nil)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)
	testutil.Equal(t, "application/yaml", w.Header().Get("Content-Type"))
	testutil.Contains(t, w.Body.String(), "openapi: 3.0.3")
	testutil.Contains(t, w.Body.String(), "Allyourbase API")
}

// TestCacheHolderReadyChannel verifies the ready channel is open before Load().
func TestCacheHolderReadyChannel(t *testing.T) {
	t.Parallel()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(nil, logger)

	// Ready channel should not be closed yet.
	select {
	case <-ch.Ready():
		t.Fatal("ready channel should not be closed before Load()")
	default:
		// Expected.
	}
}

// TestStartTLSWithReady verifies TLS listener binds, ready fires, and /health
// responds over HTTPS. Uses a self-signed cert — no network or certmagic needed.
func TestStartTLSWithReady(t *testing.T) {
	tlsCfg := generateTestTLSConfig(t)

	ln, err := tls.Listen("tcp", "127.0.0.1:0", tlsCfg)
	testutil.NoError(t, err)
	addr := ln.Addr().String()

	ch := newCacheHolderWithSchema(nil)
	srv := newTestServer(t, ch)

	ready := make(chan struct{})
	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.StartTLSWithReady(ln, ready)
	}()

	select {
	case <-ready:
		// Listener bound.
	case err := <-errCh:
		t.Fatalf("server error before ready: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for TLS server to become ready")
	}

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec — test only
		},
	}
	resp, err := client.Get("https://" + addr + "/health")
	testutil.NoError(t, err)
	defer resp.Body.Close()
	testutil.Equal(t, http.StatusOK, resp.StatusCode)

	// Check that the TLS server uses the right cert (our test cert, not a real one).
	connState := resp.TLS
	testutil.NotNil(t, connState)

	// Shut down cleanly.
	if err := srv.Shutdown(t.Context()); err != nil {
		t.Logf("shutdown: %v", err)
	}
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("unexpected server error after shutdown: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for server goroutine to exit")
	}
}

// TestStartTLSWithReadyClosesReadyBeforeServing verifies ready fires right after
// bind, not after the first request.
func TestStartTLSWithReadyClosesReadyBeforeServing(t *testing.T) {
	tlsCfg := generateTestTLSConfig(t)
	ln, err := tls.Listen("tcp", "127.0.0.1:0", tlsCfg)
	testutil.NoError(t, err)

	ch := newCacheHolderWithSchema(nil)
	srv := newTestServer(t, ch)

	ready := make(chan struct{})
	go func() { srv.StartTLSWithReady(ln, ready) }() //nolint:errcheck

	select {
	case <-ready:
		// Good — ready fired before any request.
	case <-time.After(5 * time.Second):
		t.Fatal("timeout: ready channel never closed")
	}
	srv.Shutdown(t.Context()) //nolint:errcheck
}

// --- Security wiring tests ---

// TestSchemaEndpointRequiresAuthWhenConfigured verifies that /api/schema returns
// 401 when authSvc is configured and no bearer token is provided.
func TestSchemaEndpointRequiresAuthWhenConfigured(t *testing.T) {
	t.Parallel()
	cfg := config.Default()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(nil, logger)
	ch.SetForTesting(&schema.SchemaCache{
		Tables:  map[string]*schema.Table{"public.t": {Schema: "public", Name: "t", Kind: "table"}},
		Schemas: []string{"public"},
		BuiltAt: time.Now(),
	})
	authSvc := auth.NewService(nil, "test-secret-that-is-at-least-32-chars!!", time.Hour, 7*24*time.Hour, 8, logger)
	srv := server.New(cfg, logger, ch, nil, authSvc, nil)

	// Without auth header → 401.
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/schema", nil)
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusUnauthorized, w.Code)

	// With valid JWT → 200.
	jwt, err := authSvc.IssueTestToken("user-1", "test@example.com")
	testutil.NoError(t, err)

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/schema", nil)
	req.Header.Set("Authorization", "Bearer "+jwt)
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)
}

// TestAdminAuthRateLimited verifies that the rate limiter middleware is wired
// to the /api/admin/auth endpoint by exhausting the limit and getting 429.
func TestAdminAuthRateLimited(t *testing.T) {
	t.Parallel()
	// Use a low rate limit (3/min) so the test doesn't need many requests.
	cfg := config.Default()
	cfg.Admin.Password = "testpass"
	cfg.Admin.LoginRateLimit = 3
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(nil, logger)
	srv := server.New(cfg, logger, ch, nil, nil, nil)

	// Send requests to exhaust the limit.
	for i := 0; i < 3; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/admin/auth", strings.NewReader(`{"password":"wrong"}`))
		req.Header.Set("Content-Type", "application/json")
		srv.Router().ServeHTTP(w, req)
		// These should be 401 (wrong password), not 429 yet.
		testutil.Equal(t, http.StatusUnauthorized, w.Code)
	}

	// 6th request should be rate limited.
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/auth", strings.NewReader(`{"password":"wrong"}`))
	req.Header.Set("Content-Type", "application/json")
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusTooManyRequests, w.Code)
	testutil.Contains(t, w.Body.String(), "too many requests")
	testutil.True(t, w.Header().Get("Retry-After") != "", "should have Retry-After header")
}

// TestStorageWriteRoutesRequireAuth verifies that storage upload and delete
// routes return 401 when authSvc is configured but no token is provided.
func TestStorageWriteRoutesRequireAuth(t *testing.T) {
	t.Parallel()
	cfg := config.Default()
	cfg.Storage.Enabled = true
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(nil, logger)

	authSvc := auth.NewService(nil, "test-secret-that-is-at-least-32-chars!!", time.Hour, 7*24*time.Hour, 8, logger)
	localBackend, err := storage.NewLocalBackend(t.TempDir())
	testutil.NoError(t, err)
	storageSvc := storage.NewService(nil, localBackend, "sign-key-for-test", logger)

	srv := server.New(cfg, logger, ch, nil, authSvc, storageSvc)

	// POST (upload) without auth → 401.
	var body strings.Builder
	mpw := multipart.NewWriter(&body)
	fw, err := mpw.CreateFormFile("file", "test.txt")
	testutil.NoError(t, err)
	fw.Write([]byte("hello"))
	mpw.Close()

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/storage/mybucket", strings.NewReader(body.String()))
	req.Header.Set("Content-Type", mpw.FormDataContentType())
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusUnauthorized, w.Code)

	// DELETE without auth → 401.
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodDelete, "/api/storage/mybucket/test.txt", nil)
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusUnauthorized, w.Code)

	// Sign endpoint without auth → 401.
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/storage/mybucket/test.txt/sign", nil)
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAuthTokenEndpointAcceptsFormContentType(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(nil, logger)
	authSvc := auth.NewService(nil, "test-secret-that-is-at-least-32-chars!!", time.Hour, 7*24*time.Hour, 8, logger)
	srv := server.New(cfg, logger, ch, nil, authSvc, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/auth/token", strings.NewReader("grant_type=password"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	// The request should reach the auth token handler (400 unsupported grant),
	// not fail outer middleware content-type checks (415).
	testutil.Equal(t, http.StatusBadRequest, w.Code)

	var body map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &body)
	testutil.NoError(t, err)
	testutil.Equal(t, "unsupported_grant_type", body["error"])
}

func TestAuthRevokeEndpointAcceptsFormContentType(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(nil, logger)
	authSvc := auth.NewService(nil, "test-secret-that-is-at-least-32-chars!!", time.Hour, 7*24*time.Hour, 8, logger)
	srv := server.New(cfg, logger, ch, nil, authSvc, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/auth/revoke", strings.NewReader("token=ayb_at_test123"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	// Per RFC 7009: revocation always returns 200.
	testutil.Equal(t, http.StatusOK, w.Code)
}

func TestCORSPreflightOnOAuthTokenEndpoint(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.Server.CORSAllowedOrigins = []string{"https://spa.example.com"}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(nil, logger)
	authSvc := auth.NewService(nil, "test-secret-that-is-at-least-32-chars!!", time.Hour, 7*24*time.Hour, 8, logger)
	srv := server.New(cfg, logger, ch, nil, authSvc, nil)

	// OPTIONS preflight to /api/auth/token.
	req := httptest.NewRequest(http.MethodOptions, "/api/auth/token", nil)
	req.Header.Set("Origin", "https://spa.example.com")
	req.Header.Set("Access-Control-Request-Method", "POST")
	req.Header.Set("Access-Control-Request-Headers", "Content-Type, Authorization")
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	testutil.Equal(t, http.StatusNoContent, w.Code)
	testutil.Equal(t, "https://spa.example.com", w.Header().Get("Access-Control-Allow-Origin"))
	testutil.Contains(t, w.Header().Get("Access-Control-Allow-Methods"), "POST")
	testutil.Contains(t, w.Header().Get("Access-Control-Allow-Headers"), "Content-Type")
	testutil.Contains(t, w.Header().Get("Access-Control-Allow-Headers"), "Authorization")
}

func TestCORSPreflightOnOAuthRevokeEndpoint(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.Server.CORSAllowedOrigins = []string{"https://spa.example.com"}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(nil, logger)
	authSvc := auth.NewService(nil, "test-secret-that-is-at-least-32-chars!!", time.Hour, 7*24*time.Hour, 8, logger)
	srv := server.New(cfg, logger, ch, nil, authSvc, nil)

	// OPTIONS preflight to /api/auth/revoke.
	req := httptest.NewRequest(http.MethodOptions, "/api/auth/revoke", nil)
	req.Header.Set("Origin", "https://spa.example.com")
	req.Header.Set("Access-Control-Request-Method", "POST")
	req.Header.Set("Access-Control-Request-Headers", "Content-Type")
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	testutil.Equal(t, http.StatusNoContent, w.Code)
	testutil.Equal(t, "https://spa.example.com", w.Header().Get("Access-Control-Allow-Origin"))
	testutil.Contains(t, w.Header().Get("Access-Control-Allow-Methods"), "POST")
}
