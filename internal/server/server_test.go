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
	"fmt"
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

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
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

func loginAdmin(t *testing.T, srv *server.Server, password string) string {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/api/admin/auth", strings.NewReader(`{"password":"`+password+`"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)

	var body map[string]string
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	token := body["token"]
	testutil.True(t, token != "", "expected non-empty admin token")
	return token
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

func TestOAuthRedirectUsesConfiguredMicrosoftTenant(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.Auth.Enabled = true
	cfg.Auth.JWTSecret = "this-is-a-secret-that-is-at-least-32-characters-long"
	cfg.Auth.OAuth = map[string]config.OAuthProvider{
		"microsoft": {
			Enabled:      true,
			ClientID:     "ms-id",
			ClientSecret: "ms-secret",
			TenantID:     "contoso-tenant",
		},
	}

	ch := newCacheHolderWithSchema(nil)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := server.New(cfg, logger, ch, nil, &auth.Service{}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/auth/oauth/microsoft", nil)
	req.Host = "localhost:8090"
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	testutil.Equal(t, http.StatusTemporaryRedirect, w.Code)
	testutil.Contains(t, w.Header().Get("Location"), "login.microsoftonline.com/contoso-tenant/oauth2/v2.0/authorize")
}

func TestOAuthRedirectUsesConfiguredFacebookAPIVersion(t *testing.T) {
	t.Parallel()

	t.Cleanup(func() {
		auth.ResetProviderURLs("facebook")
	})

	cfg := config.Default()
	cfg.Auth.Enabled = true
	cfg.Auth.JWTSecret = "this-is-a-secret-that-is-at-least-32-characters-long"
	cfg.Auth.OAuth = map[string]config.OAuthProvider{
		"facebook": {
			Enabled:            true,
			ClientID:           "fb-id",
			ClientSecret:       "fb-secret",
			FacebookAPIVersion: "v33.0",
		},
	}

	ch := newCacheHolderWithSchema(nil)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := server.New(cfg, logger, ch, nil, &auth.Service{}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/auth/oauth/facebook", nil)
	req.Host = "localhost:8090"
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	testutil.Equal(t, http.StatusTemporaryRedirect, w.Code)
	testutil.Contains(t, w.Header().Get("Location"), "/v33.0/dialog/oauth")
}

func TestOAuthRedirectUsesConfiguredGitLabBaseURL(t *testing.T) {
	t.Parallel()

	t.Cleanup(func() {
		auth.ResetProviderURLs("gitlab")
	})

	cfg := config.Default()
	cfg.Auth.Enabled = true
	cfg.Auth.JWTSecret = "this-is-a-secret-that-is-at-least-32-characters-long"
	cfg.Auth.OAuth = map[string]config.OAuthProvider{
		"gitlab": {
			Enabled:       true,
			ClientID:      "gl-id",
			ClientSecret:  "gl-secret",
			GitLabBaseURL: "https://gitlab.example.com",
		},
	}

	ch := newCacheHolderWithSchema(nil)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := server.New(cfg, logger, ch, nil, &auth.Service{}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/auth/oauth/gitlab", nil)
	req.Host = "localhost:8090"
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	testutil.Equal(t, http.StatusTemporaryRedirect, w.Code)
	testutil.Contains(t, w.Header().Get("Location"), "https://gitlab.example.com/oauth/authorize")
}

func TestOAuthRedirectUsesConfiguredOIDCProvider(t *testing.T) {
	const providerName = "oidc_server_test"
	t.Cleanup(func() {
		auth.UnregisterOIDCProvider(providerName)
	})

	const issuerURL = "https://idp.example.com"
	prevTransport := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != issuerURL+"/.well-known/openid-configuration" {
			return &http.Response{
				StatusCode: http.StatusNotFound,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`{}`)),
			}, nil
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header: http.Header{
				"Content-Type": []string{"application/json"},
			},
			Body: io.NopCloser(strings.NewReader(`{
				"issuer":"https://idp.example.com",
				"authorization_endpoint":"https://idp.example.com/authorize",
				"token_endpoint":"https://idp.example.com/token",
				"userinfo_endpoint":"https://idp.example.com/userinfo",
				"jwks_uri":"https://idp.example.com/jwks"
			}`)),
		}, nil
	})
	t.Cleanup(func() {
		http.DefaultTransport = prevTransport
	})

	cfg := config.Default()
	cfg.Auth.Enabled = true
	cfg.Auth.JWTSecret = "this-is-a-secret-that-is-at-least-32-characters-long"
	cfg.Auth.OIDC = map[string]config.OIDCProvider{
		providerName: {
			Enabled:      true,
			IssuerURL:    issuerURL,
			ClientID:     "oidc-client-id",
			ClientSecret: "oidc-client-secret",
		},
	}

	ch := newCacheHolderWithSchema(nil)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := server.New(cfg, logger, ch, nil, &auth.Service{}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/auth/oauth/"+providerName, nil)
	req.Host = "localhost:8090"
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	testutil.Equal(t, http.StatusTemporaryRedirect, w.Code)
	testutil.Contains(t, w.Header().Get("Location"), issuerURL+"/authorize")
	testutil.Contains(t, w.Header().Get("Location"), "client_id=oidc-client-id")
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

// TestAuthEmailMFAEndpointHonorsConfigToggle verifies that server wiring passes
// auth.email_mfa_enabled through to the auth handler.
func TestAuthEmailMFAEndpointHonorsConfigToggle(t *testing.T) {
	t.Parallel()

	secret := "test-secret-that-is-at-least-32-chars!!"
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(nil, logger)

	disabled := config.Default()
	disabled.Auth.Enabled = true
	disabled.Auth.JWTSecret = secret
	srvDisabled := server.New(disabled, logger, ch, nil, auth.NewService(nil, secret, time.Hour, 7*24*time.Hour, 8, logger), nil)

	req := httptest.NewRequest(http.MethodPost, "/api/auth/mfa/email/enroll", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srvDisabled.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusNotFound, w.Code)

	enabled := config.Default()
	enabled.Auth.Enabled = true
	enabled.Auth.JWTSecret = secret
	enabled.Auth.EmailMFAEnabled = true
	srvEnabled := server.New(enabled, logger, ch, nil, auth.NewService(nil, secret, time.Hour, 7*24*time.Hour, 8, logger), nil)

	req = httptest.NewRequest(http.MethodPost, "/api/auth/mfa/email/enroll", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	srvEnabled.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusUnauthorized, w.Code)
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

	// 4th request should be rate limited.
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/auth", strings.NewReader(`{"password":"wrong"}`))
	req.Header.Set("Content-Type", "application/json")
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusTooManyRequests, w.Code)
	testutil.Contains(t, w.Body.String(), "too many requests")
	testutil.True(t, w.Header().Get("Retry-After") != "", "should have Retry-After header")
}

func TestServerAllowlistDeniesNonAdminAPIRoute(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.Server.AllowedIPs = []string{"203.0.113.10"}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := newCacheHolderWithSchema(&schema.SchemaCache{
		Tables:    map[string]*schema.Table{},
		Functions: map[string]*schema.Function{},
		Schemas:   []string{},
	})
	srv := server.New(cfg, logger, ch, nil, nil, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/schema", nil)
	req.RemoteAddr = "198.51.100.24:3456"
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusForbidden, w.Code)

	var body map[string]string
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	testutil.Equal(t, "access_denied", body["error"])
	testutil.Equal(t, "Your IP address is not in the allowlist", body["message"])
}

func TestServerAllowlistAllowsNonAdminAPIRoute(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.Server.AllowedIPs = []string{"198.51.100.0/24"}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := newCacheHolderWithSchema(&schema.SchemaCache{
		Tables:    map[string]*schema.Table{},
		Functions: map[string]*schema.Function{},
		Schemas:   []string{},
	})
	srv := server.New(cfg, logger, ch, nil, nil, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/schema", nil)
	req.RemoteAddr = "198.51.100.24:3456"
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)
}

func TestServerAllowlistUsesAdminPolicyForAdminRoutes(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.Server.AllowedIPs = []string{"198.51.100.0/24"}
	cfg.Admin.AllowedIPs = []string{"203.0.113.5"}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := newCacheHolderWithSchema(&schema.SchemaCache{
		Tables:    map[string]*schema.Table{},
		Functions: map[string]*schema.Function{},
		Schemas:   []string{},
	})
	srv := server.New(cfg, logger, ch, nil, nil, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/status", nil)
	req.RemoteAddr = "198.51.100.24:3456"
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusForbidden, w.Code)
	var body map[string]string
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	testutil.Equal(t, "access_denied", body["error"])

	req = httptest.NewRequest(http.MethodGet, "/api/admin/status", nil)
	req.RemoteAddr = "203.0.113.5:3456"
	w = httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)
}

func TestServerAllowlistDoesNotTreatNonAdminPrefixAsAdminRoute(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.Server.AllowedIPs = []string{"198.51.100.0/24"}
	cfg.Admin.AllowedIPs = []string{"203.0.113.5"}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := newCacheHolderWithSchema(&schema.SchemaCache{
		Tables:    map[string]*schema.Table{},
		Functions: map[string]*schema.Function{},
		Schemas:   []string{},
	})
	srv := server.New(cfg, logger, ch, nil, nil, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/administrator/ping", nil)
	req.RemoteAddr = "198.51.100.24:3456"
	srv.Router().ServeHTTP(w, req)

	// This path is not an admin route; it should follow server allowlist and
	// fall through to normal routing (404), not be blocked by admin allowlist.
	testutil.Equal(t, http.StatusNotFound, w.Code)
}

func TestAuthAnonymousRouteRateLimitedSeparately(t *testing.T) {
	t.Parallel()

	secret := "test-secret-that-is-at-least-32-chars!!"
	cfg := config.Default()
	cfg.Auth.Enabled = true
	cfg.Auth.JWTSecret = secret
	cfg.Auth.AnonymousAuthEnabled = true
	cfg.Auth.RateLimit = 100
	cfg.Auth.AnonymousRateLimit = 2

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(nil, logger)
	authSvc := auth.NewService(nil, secret, time.Hour, 7*24*time.Hour, 8, logger)
	srv := server.New(cfg, logger, ch, nil, authSvc, nil)

	for i := 0; i < 2; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/auth/anonymous", nil)
		req.RemoteAddr = "198.51.100.24:34567"
		srv.Router().ServeHTTP(w, req)
		testutil.Equal(t, http.StatusInternalServerError, w.Code)
		testutil.Equal(t, "2", w.Header().Get("X-RateLimit-Limit"))
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/anonymous", nil)
	req.RemoteAddr = "198.51.100.24:34567"
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusTooManyRequests, w.Code)
	testutil.Equal(t, "2", w.Header().Get("X-RateLimit-Limit"))
	testutil.Equal(t, "0", w.Header().Get("X-RateLimit-Remaining"))
	testutil.Contains(t, w.Body.String(), "too many requests")
}

func TestAuthSensitiveEndpointsUseStricterRateLimit(t *testing.T) {
	t.Parallel()

	secret := "test-secret-that-is-at-least-32-chars!!"
	cfg := config.Default()
	cfg.Auth.Enabled = true
	cfg.Auth.JWTSecret = secret
	cfg.Auth.RateLimit = 100
	cfg.Auth.RateLimitAuth = "2/min"

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(nil, logger)
	authSvc := auth.NewService(nil, secret, time.Hour, 7*24*time.Hour, 8, logger)
	srv := server.New(cfg, logger, ch, nil, authSvc, nil)

	for i := 0; i < 2; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"email":"a@b.com","password":"bad"}`))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "198.51.100.24:4567"
		srv.Router().ServeHTTP(w, req)
		testutil.Equal(t, "2", w.Header().Get("X-RateLimit-Limit"))
		testutil.True(t, w.Code != http.StatusTooManyRequests, "request %d should not be limited yet", i+1)
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"email":"a@b.com","password":"bad"}`))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "198.51.100.24:4567"
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusTooManyRequests, w.Code)
	testutil.Equal(t, "2", w.Header().Get("X-RateLimit-Limit"))
	testutil.Equal(t, "0", w.Header().Get("X-RateLimit-Remaining"))
}

func TestAuthSMSConfirmUsesStricterRateLimit(t *testing.T) {
	t.Parallel()

	secret := "test-secret-that-is-at-least-32-chars!!"
	cfg := config.Default()
	cfg.Auth.Enabled = true
	cfg.Auth.JWTSecret = secret
	cfg.Auth.RateLimit = 100
	cfg.Auth.RateLimitAuth = "1/min"

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(nil, logger)
	authSvc := auth.NewService(nil, secret, time.Hour, 7*24*time.Hour, 8, logger)
	srv := server.New(cfg, logger, ch, nil, authSvc, nil)

	// First request should not be rate-limited yet (handler will return validation error).
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/sms/confirm", strings.NewReader(`{"phone":"+15551234567","code":"000000"}`))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "198.51.100.24:4567"
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, "1", w.Header().Get("X-RateLimit-Limit"))
	testutil.True(t, w.Code != http.StatusTooManyRequests, "first request should not be limited yet")

	// Second request should be limited by the auth-sensitive limiter.
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/auth/sms/confirm", strings.NewReader(`{"phone":"+15551234567","code":"000000"}`))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "198.51.100.24:4567"
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusTooManyRequests, w.Code)
	testutil.Equal(t, "1", w.Header().Get("X-RateLimit-Limit"))
	testutil.Equal(t, "0", w.Header().Get("X-RateLimit-Remaining"))
}

func TestAuthMFATOTPVerifyUsesStricterRateLimit(t *testing.T) {
	t.Parallel()

	secret := "test-secret-that-is-at-least-32-chars!!"
	cfg := config.Default()
	cfg.Auth.Enabled = true
	cfg.Auth.JWTSecret = secret
	cfg.Auth.RateLimit = 100
	cfg.Auth.RateLimitAuth = "1/min"

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(nil, logger)
	authSvc := auth.NewService(nil, secret, time.Hour, 7*24*time.Hour, 8, logger)
	srv := server.New(cfg, logger, ch, nil, authSvc, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/mfa/totp/verify", strings.NewReader(`{"code":"000000"}`))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "198.51.100.24:4567"
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, "1", w.Header().Get("X-RateLimit-Limit"))
	testutil.True(t, w.Code != http.StatusTooManyRequests, "first request should not be limited yet")

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/auth/mfa/totp/verify", strings.NewReader(`{"code":"000000"}`))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "198.51.100.24:4567"
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusTooManyRequests, w.Code)
	testutil.Equal(t, "1", w.Header().Get("X-RateLimit-Limit"))
	testutil.Equal(t, "0", w.Header().Get("X-RateLimit-Remaining"))
}

func TestAuthNonSensitiveEndpointsUseGeneralRateLimit(t *testing.T) {
	t.Parallel()

	secret := "test-secret-that-is-at-least-32-chars!!"
	cfg := config.Default()
	cfg.Auth.Enabled = true
	cfg.Auth.JWTSecret = secret
	cfg.Auth.RateLimit = 2
	cfg.Auth.RateLimitAuth = "1/min"

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(nil, logger)
	authSvc := auth.NewService(nil, secret, time.Hour, 7*24*time.Hour, 8, logger)
	srv := server.New(cfg, logger, ch, nil, authSvc, nil)

	for i := 0; i < 2; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/auth/refresh", strings.NewReader(`{"refreshToken":""}`))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "203.0.113.10:7890"
		srv.Router().ServeHTTP(w, req)
		testutil.Equal(t, "2", w.Header().Get("X-RateLimit-Limit"))
		testutil.True(t, w.Code != http.StatusTooManyRequests, "request %d should not be limited yet", i+1)
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/refresh", strings.NewReader(`{"refreshToken":""}`))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "203.0.113.10:7890"
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusTooManyRequests, w.Code)
	testutil.Equal(t, "2", w.Header().Get("X-RateLimit-Limit"))
}

func TestAPIRoutesUseAnonymousRateLimitWhenUnauthenticated(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.RateLimit.APIAnonymous = "1/min"
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := newCacheHolderWithSchema(nil)
	srv := server.New(cfg, logger, ch, nil, nil, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/schema", nil)
	req.RemoteAddr = "198.51.100.24:1111"
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusServiceUnavailable, w.Code)
	testutil.Equal(t, "1", w.Header().Get("X-RateLimit-Limit"))

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/schema", nil)
	req.RemoteAddr = "198.51.100.24:1111"
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusTooManyRequests, w.Code)
	testutil.Equal(t, "1", w.Header().Get("X-RateLimit-Limit"))
	testutil.Equal(t, "0", w.Header().Get("X-RateLimit-Remaining"))
	testutil.Contains(t, w.Body.String(), "too many requests")
}

func TestAPIRoutesWithAuthUseAnonymousRateLimitForUnauthenticatedRequests(t *testing.T) {
	t.Parallel()

	secret := "test-secret-that-is-at-least-32-chars!!"
	cfg := config.Default()
	cfg.Auth.Enabled = true
	cfg.Auth.JWTSecret = secret
	cfg.RateLimit.APIAnonymous = "1/min"

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := newCacheHolderWithSchema(nil)
	authSvc := auth.NewService(nil, secret, time.Hour, 7*24*time.Hour, 8, logger)
	srv := server.New(cfg, logger, ch, nil, authSvc, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/schema", nil)
	req.RemoteAddr = "198.51.100.24:1111"
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusUnauthorized, w.Code)
	testutil.Equal(t, "1", w.Header().Get("X-RateLimit-Limit"))

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/schema", nil)
	req.RemoteAddr = "198.51.100.24:1111"
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusTooManyRequests, w.Code)
	testutil.Equal(t, "1", w.Header().Get("X-RateLimit-Limit"))
	testutil.Equal(t, "0", w.Header().Get("X-RateLimit-Remaining"))
}

func TestAPIRoutesUseAuthenticatedRateLimitByUser(t *testing.T) {
	t.Parallel()

	secret := "test-secret-that-is-at-least-32-chars!!"
	cfg := config.Default()
	cfg.Auth.Enabled = true
	cfg.Auth.JWTSecret = secret
	cfg.RateLimit.API = "2/min"
	cfg.RateLimit.APIAnonymous = "1/min"
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := newCacheHolderWithSchema(&schema.SchemaCache{
		Tables: map[string]*schema.Table{
			"public.users": {Schema: "public", Name: "users", Kind: "table"},
		},
		Schemas: []string{"public"},
		BuiltAt: time.Now(),
	})
	authSvc := auth.NewService(nil, secret, time.Hour, 7*24*time.Hour, 8, logger)
	srv := server.New(cfg, logger, ch, nil, authSvc, nil)

	jwt, err := authSvc.IssueTestToken("user-1", "user1@example.com")
	testutil.NoError(t, err)

	for i := 0; i < 2; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/schema", nil)
		req.Header.Set("Authorization", "Bearer "+jwt)
		req.RemoteAddr = "203.0.113.24:1111"
		srv.Router().ServeHTTP(w, req)
		testutil.Equal(t, http.StatusOK, w.Code)
		testutil.Equal(t, "2", w.Header().Get("X-RateLimit-Limit"))
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/schema", nil)
	req.Header.Set("Authorization", "Bearer "+jwt)
	req.RemoteAddr = "203.0.113.24:1111"
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusTooManyRequests, w.Code)
	testutil.Equal(t, "2", w.Header().Get("X-RateLimit-Limit"))
}

func TestAPIRouteRateLimitIsPerUser(t *testing.T) {
	t.Parallel()

	secret := "test-secret-that-is-at-least-32-chars!!"
	cfg := config.Default()
	cfg.Auth.Enabled = true
	cfg.Auth.JWTSecret = secret
	cfg.RateLimit.API = "1/min"
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := newCacheHolderWithSchema(&schema.SchemaCache{
		Tables: map[string]*schema.Table{
			"public.users": {Schema: "public", Name: "users", Kind: "table"},
		},
		Schemas: []string{"public"},
		BuiltAt: time.Now(),
	})
	authSvc := auth.NewService(nil, secret, time.Hour, 7*24*time.Hour, 8, logger)
	srv := server.New(cfg, logger, ch, nil, authSvc, nil)

	user1, err := authSvc.IssueTestToken("user-1", "one@example.com")
	testutil.NoError(t, err)
	user2, err := authSvc.IssueTestToken("user-2", "two@example.com")
	testutil.NoError(t, err)

	for i := 0; i < 2; i++ {
		token := user1
		expected := http.StatusOK
		if i > 0 {
			expected = http.StatusTooManyRequests
		}
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/schema", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		req.RemoteAddr = "192.0.2.10:1111"
		srv.Router().ServeHTTP(w, req)
		testutil.Equal(t, expected, w.Code)
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/schema", nil)
	req.Header.Set("Authorization", "Bearer "+user2)
	req.RemoteAddr = "192.0.2.10:1111"
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)
}

func TestAPIRouteRateLimitMiddlewareResetsAfterWindow(t *testing.T) {
	rl := auth.NewRateLimiter(1, 20*time.Millisecond)
	defer rl.Stop()

	mw := server.APIRouteRateLimitMiddleware(rl, rl, 1, 1)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/schema", nil)
	req.RemoteAddr = "198.51.100.24:1111"
	handler.ServeHTTP(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/schema", nil)
	req.RemoteAddr = "198.51.100.24:1111"
	handler.ServeHTTP(w, req)
	testutil.Equal(t, http.StatusTooManyRequests, w.Code)

	time.Sleep(50 * time.Millisecond)

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/schema", nil)
	req.RemoteAddr = "198.51.100.24:1111"
	handler.ServeHTTP(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)
}

func TestUserProtectedRoutesApplyAPIRateLimit(t *testing.T) {
	t.Parallel()

	secret := "test-secret-that-is-at-least-32-chars!!"
	cfg := config.Default()
	cfg.Auth.Enabled = true
	cfg.Auth.JWTSecret = secret
	cfg.RateLimit.API = "1/min"

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := newCacheHolderWithSchema(nil)
	authSvc := auth.NewService(nil, secret, time.Hour, 7*24*time.Hour, 8, logger)
	srv := server.New(cfg, logger, ch, nil, authSvc, nil)

	jwt, err := authSvc.IssueTestToken("user-1", "user1@example.com")
	testutil.NoError(t, err)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/push/devices", nil)
	req.Header.Set("Authorization", "Bearer "+jwt)
	req.RemoteAddr = "203.0.113.24:1111"
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, "1", w.Header().Get("X-RateLimit-Limit"))
	testutil.Equal(t, http.StatusServiceUnavailable, w.Code)

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/push/devices", nil)
	req.Header.Set("Authorization", "Bearer "+jwt)
	req.RemoteAddr = "203.0.113.24:1111"
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusTooManyRequests, w.Code)
	testutil.Equal(t, "1", w.Header().Get("X-RateLimit-Limit"))
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
	storageSvc := storage.NewService(nil, localBackend, "sign-key-for-test", logger, 0)

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

func TestAdminUserStorageQuotaRoutesRequireAdminWhenStorageConfigured(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.Storage.Enabled = true
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(nil, logger)

	authSvc := auth.NewService(nil, "test-secret-that-is-at-least-32-chars!!", time.Hour, 7*24*time.Hour, 8, logger)
	localBackend, err := storage.NewLocalBackend(t.TempDir())
	testutil.NoError(t, err)
	storageSvc := storage.NewService(nil, localBackend, "sign-key-for-test", logger, 0)

	srv := server.New(cfg, logger, ch, nil, authSvc, storageSvc)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/users/user-1/storage-quota", nil)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	testutil.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAdminUserStorageQuotaRoutesOmittedWithoutStorageService(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.Admin.Password = "admin-pass"
	cfg.Storage.Enabled = true
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(nil, logger)

	authSvc := auth.NewService(nil, "test-secret-that-is-at-least-32-chars!!", time.Hour, 7*24*time.Hour, 8, logger)
	srv := server.New(cfg, logger, ch, nil, authSvc, nil)
	adminToken := loginAdmin(t, srv, cfg.Admin.Password)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/users/user-1/storage-quota", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	testutil.Equal(t, http.StatusNotFound, w.Code)
}

func TestAdminAnalyticsRequestsRouteRequiresAdminAndReachesHandler(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.Admin.Password = "testpass"
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := newCacheHolderWithSchema(nil)
	srv := server.New(cfg, logger, ch, nil, nil, nil)

	unauthReq := httptest.NewRequest(http.MethodGet, "/api/admin/analytics/requests", nil)
	unauthRes := httptest.NewRecorder()
	srv.Router().ServeHTTP(unauthRes, unauthReq)
	testutil.Equal(t, http.StatusUnauthorized, unauthRes.Code)

	adminToken := loginAdmin(t, srv, cfg.Admin.Password)
	authReq := httptest.NewRequest(http.MethodGet, "/api/admin/analytics/requests", nil)
	authReq.Header.Set("Authorization", "Bearer "+adminToken)
	authRes := httptest.NewRecorder()
	srv.Router().ServeHTTP(authRes, authReq)
	testutil.Equal(t, http.StatusServiceUnavailable, authRes.Code)
	testutil.Contains(t, authRes.Body.String(), "database not configured")
}

func TestAdminAuditRouteRequiresAdminAndReachesHandler(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.Admin.Password = "testpass"
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := newCacheHolderWithSchema(nil)
	srv := server.New(cfg, logger, ch, nil, nil, nil)

	unauthReq := httptest.NewRequest(http.MethodGet, "/api/admin/audit", nil)
	unauthRes := httptest.NewRecorder()
	srv.Router().ServeHTTP(unauthRes, unauthReq)
	testutil.Equal(t, http.StatusUnauthorized, unauthRes.Code)

	adminToken := loginAdmin(t, srv, cfg.Admin.Password)
	authReq := httptest.NewRequest(http.MethodGet, "/api/admin/audit", nil)
	authReq.Header.Set("Authorization", "Bearer "+adminToken)
	authRes := httptest.NewRecorder()
	srv.Router().ServeHTTP(authRes, authReq)
	testutil.Equal(t, http.StatusServiceUnavailable, authRes.Code)
	testutil.Contains(t, authRes.Body.String(), "database not configured")
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

func TestAllSensitiveAuthPathsUseStricterLimit(t *testing.T) {
	t.Parallel()

	secret := "test-secret-that-is-at-least-32-chars!!"
	cfg := config.Default()
	cfg.Auth.Enabled = true
	cfg.Auth.JWTSecret = secret
	cfg.Auth.RateLimit = 100
	cfg.Auth.RateLimitAuth = "1/min"

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(nil, logger)
	authSvc := auth.NewService(nil, secret, time.Hour, 7*24*time.Hour, 8, logger)
	srv := server.New(cfg, logger, ch, nil, authSvc, nil)

	sensitivePaths := []string{
		"/api/auth/login",
		"/api/auth/register",
		"/api/auth/magic-link",
		"/api/auth/sms",
		"/api/auth/sms/confirm",
		"/api/auth/mfa/totp/verify",
		"/api/auth/mfa/sms/verify",
		"/api/auth/mfa/email/verify",
		"/api/auth/mfa/backup/verify",
	}

	for i, path := range sensitivePaths {
		t.Run(path, func(t *testing.T) {
			// Use a unique IP per path so rate limiters don't interfere.
			ip := fmt.Sprintf("198.51.100.%d:4567", i+10)
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{}`))
			req.Header.Set("Content-Type", "application/json")
			req.RemoteAddr = ip
			srv.Router().ServeHTTP(w, req)

			// The sensitive limiter has limit=1, so X-RateLimit-Limit must be "1".
			testutil.Equal(t, "1", w.Header().Get("X-RateLimit-Limit"))
		})
	}
}

func TestHealthEndpointBypassesIPAllowlist(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.Server.AllowedIPs = []string{"203.0.113.5"}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := newCacheHolderWithSchema(nil)
	srv := server.New(cfg, logger, ch, nil, nil, nil)

	// Request /health from an IP NOT in the allowlist.
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.RemoteAddr = "198.51.100.99:4567"
	srv.Router().ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)
}

func TestServerAllowlistDoesNotRestrictAdminWhenAdminAllowlistEmpty(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.Server.AllowedIPs = []string{"198.51.100.0/24"}
	// Admin.AllowedIPs is empty (default) — admin should allow all.

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := newCacheHolderWithSchema(&schema.SchemaCache{
		Tables:    map[string]*schema.Table{},
		Functions: map[string]*schema.Function{},
		Schemas:   []string{},
	})
	srv := server.New(cfg, logger, ch, nil, nil, nil)

	// Admin route from an IP outside the server allowlist should still be accessible
	// when no admin allowlist is configured.
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/status", nil)
	req.RemoteAddr = "10.0.0.1:3456"
	srv.Router().ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)
}
