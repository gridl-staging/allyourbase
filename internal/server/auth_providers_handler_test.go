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

// authProvidersServerWithAuth creates a test server with auth + admin configured,
// and returns the server and a valid admin token.
func authProvidersServerWithAuth(t *testing.T, oauthProviders map[string]config.OAuthProvider) (*server.Server, string) {
	t.Helper()
	cfg := config.Default()
	cfg.Admin.Password = "admin-pass"
	cfg.Auth.Enabled = true
	cfg.Auth.JWTSecret = "test-secret-that-is-at-least-32-chars-long"
	if oauthProviders != nil {
		cfg.Auth.OAuth = oauthProviders
	}

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

func TestAdminAuthProvidersGet_RequiresAdminToken(t *testing.T) {
	t.Parallel()
	srv, _ := authProvidersServerWithAuth(t, nil)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/auth/providers", nil)
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAdminAuthProvidersGet_NoAuth(t *testing.T) {
	t.Parallel()
	// Server without auth enabled — should return 404.
	cfg := config.Default()
	cfg.Admin.Password = "admin-pass"
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(nil, logger)
	srv := server.New(cfg, logger, ch, nil, nil, nil)

	// Get admin token.
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/auth",
		strings.NewReader(`{"password":"admin-pass"}`))
	req.Header.Set("Content-Type", "application/json")
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)
	var loginResp map[string]string
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &loginResp))

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/admin/auth/providers", nil)
	req.Header.Set("Authorization", "Bearer "+loginResp["token"])
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusNotFound, w.Code)
}

func TestAdminAuthProvidersGet_ListsConfiguredProviders(t *testing.T) {
	t.Parallel()
	oauthCfg := map[string]config.OAuthProvider{
		"google": {
			Enabled:  true,
			ClientID: "google-client-id",
		},
		"github": {
			Enabled:  true,
			ClientID: "github-client-id",
		},
		"discord": {
			Enabled:  false,
			ClientID: "discord-client-id",
		},
	}
	srv, token := authProvidersServerWithAuth(t, oauthCfg)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/auth/providers", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Providers []auth.OAuthProviderInfo `json:"providers"`
	}
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	// Should have at least the two enabled providers.
	providerMap := make(map[string]auth.OAuthProviderInfo)
	for _, p := range resp.Providers {
		providerMap[p.Name] = p
	}

	// Google should be present and enabled.
	google, ok := providerMap["google"]
	testutil.True(t, ok, "google should be in the response")
	testutil.True(t, google.Enabled)
	testutil.Equal(t, "builtin", google.Type)
	testutil.True(t, google.ClientIDConfigured)

	// GitHub should be present and enabled.
	github, ok := providerMap["github"]
	testutil.True(t, ok, "github should be in the response")
	testutil.True(t, github.Enabled)

	// Discord was not enabled in config — should not appear as enabled.
	discord, ok := providerMap["discord"]
	if ok {
		testutil.False(t, discord.Enabled)
		testutil.True(t, discord.ClientIDConfigured, "disabled provider with client_id should report client_id_configured=true")
	}
}

func TestAdminAuthProvidersGet_NoProvidersConfigured(t *testing.T) {
	t.Parallel()
	srv, token := authProvidersServerWithAuth(t, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/auth/providers", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Providers []auth.OAuthProviderInfo `json:"providers"`
	}
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	// No providers configured — should return empty or only unconfigured builtins.
	for _, p := range resp.Providers {
		testutil.False(t, p.Enabled, "no providers should be enabled when none are configured")
	}
}

func TestAdminAuthProvidersPut_RequiresAdminToken(t *testing.T) {
	t.Parallel()
	srv, _ := authProvidersServerWithAuth(t, nil)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/admin/auth/providers/google",
		strings.NewReader(`{"enabled":true,"client_id":"google-id","client_secret":"google-secret"}`))
	req.Header.Set("Content-Type", "application/json")
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAdminAuthProvidersPut_UpdatesBuiltinProvider(t *testing.T) {
	t.Parallel()
	srv, token := authProvidersServerWithAuth(t, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/admin/auth/providers/google",
		strings.NewReader(`{"enabled":true,"client_id":"google-id","client_secret":"google-secret"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)

	providers := getAuthProviderMap(t, srv, token)
	google, ok := providers["google"]
	testutil.True(t, ok, "google should be present")
	testutil.True(t, google.Enabled)
	testutil.True(t, google.ClientIDConfigured)

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/auth/oauth/google", nil)
	req.Host = "localhost:8090"
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusTemporaryRedirect, w.Code)

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPut, "/api/admin/auth/providers/google",
		strings.NewReader(`{"enabled":false}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)

	providers = getAuthProviderMap(t, srv, token)
	google = providers["google"]
	testutil.False(t, google.Enabled)

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/auth/oauth/google", nil)
	req.Host = "localhost:8090"
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusNotFound, w.Code)
}

func TestAdminAuthProvidersPut_ValidatesBuiltinProviderFields(t *testing.T) {
	t.Parallel()
	srv, token := authProvidersServerWithAuth(t, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/admin/auth/providers/google",
		strings.NewReader(`{"enabled":true,"client_id":"google-id"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	srv.Router().ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "client_secret is required when enabled")
}

func TestAdminAuthProvidersPut_UpdatesOIDCProvider(t *testing.T) {
	providerName := "oidc_admin_put_test"
	auth.UnregisterOIDCProvider(providerName)
	t.Cleanup(func() {
		auth.UnregisterOIDCProvider(providerName)
	})
	discoveryServer := newMockOIDCDiscoveryServer(t)
	issuerURL := discoveryServer.URL

	srv, token := authProvidersServerWithAuth(t, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/admin/auth/providers/"+providerName,
		strings.NewReader(`{
			"enabled":true,
			"issuer_url":"`+issuerURL+`",
			"client_id":"oidc-client-id",
			"client_secret":"oidc-client-secret",
			"scopes":["openid","profile","email"],
			"display_name":"OIDC Provider"
		}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)

	providers := getAuthProviderMap(t, srv, token)
	oidc, ok := providers[providerName]
	testutil.True(t, ok, "oidc provider should be present")
	testutil.Equal(t, "oidc", oidc.Type)
	testutil.True(t, oidc.Enabled)
	testutil.True(t, oidc.ClientIDConfigured)

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/auth/oauth/"+providerName, nil)
	req.Host = "localhost:8090"
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusTemporaryRedirect, w.Code)
	testutil.Contains(t, w.Header().Get("Location"), issuerURL+"/authorize")
}

func TestAdminAuthProvidersPut_MicrosoftTenantAffectsRedirectURL(t *testing.T) {
	t.Parallel()
	srv, token := authProvidersServerWithAuth(t, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/admin/auth/providers/microsoft",
		strings.NewReader(`{
			"enabled":true,
			"client_id":"ms-client-id",
			"client_secret":"ms-client-secret",
			"tenant_id":"contoso-tenant"
		}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/auth/oauth/microsoft", nil)
	req.Host = "localhost:8090"
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusTemporaryRedirect, w.Code)
	testutil.Contains(t, w.Header().Get("Location"), "https://login.microsoftonline.com/contoso-tenant/oauth2/v2.0/authorize")
}

func TestAdminAuthProvidersDelete_RequiresAdminToken(t *testing.T) {
	t.Parallel()
	srv, _ := authProvidersServerWithAuth(t, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/admin/auth/providers/google", nil)
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAdminAuthProvidersDelete_RemovesBuiltInProviderConfig(t *testing.T) {
	t.Parallel()
	srv, token := authProvidersServerWithAuth(t, map[string]config.OAuthProvider{
		"google": {
			Enabled:      true,
			ClientID:     "google-id",
			ClientSecret: "google-secret",
		},
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/auth/oauth/google", nil)
	req.Host = "localhost:8090"
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusTemporaryRedirect, w.Code)

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodDelete, "/api/admin/auth/providers/google", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusNoContent, w.Code)

	providers := getAuthProviderMap(t, srv, token)
	google, ok := providers["google"]
	testutil.True(t, ok, "google should still be listed as a builtin provider")
	testutil.False(t, google.Enabled)
	testutil.False(t, google.ClientIDConfigured)

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/auth/oauth/google", nil)
	req.Host = "localhost:8090"
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusNotFound, w.Code)
}

func TestAdminAuthProvidersDelete_RemovesOIDCProvider(t *testing.T) {
	providerName := "oidc_admin_delete_test"
	auth.UnregisterOIDCProvider(providerName)
	t.Cleanup(func() {
		auth.UnregisterOIDCProvider(providerName)
	})
	discoveryServer := newMockOIDCDiscoveryServer(t)
	issuerURL := discoveryServer.URL

	srv, token := authProvidersServerWithAuth(t, nil)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/admin/auth/providers/"+providerName,
		strings.NewReader(`{
			"enabled":true,
			"issuer_url":"`+issuerURL+`",
			"client_id":"oidc-client-id",
			"client_secret":"oidc-client-secret"
		}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)

	providers := getAuthProviderMap(t, srv, token)
	_, ok := providers[providerName]
	testutil.True(t, ok, "oidc provider should be present before delete")

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodDelete, "/api/admin/auth/providers/"+providerName, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusNoContent, w.Code)

	providers = getAuthProviderMap(t, srv, token)
	_, ok = providers[providerName]
	testutil.False(t, ok, "oidc provider should be removed from provider list")

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/auth/oauth/"+providerName, nil)
	req.Host = "localhost:8090"
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusNotFound, w.Code)
}

func TestAdminAuthProvidersDelete_UnknownOIDCProviderReturnsNotFound(t *testing.T) {
	t.Parallel()
	providerName := "oidc_does_not_exist"
	auth.UnregisterOIDCProvider(providerName)

	srv, token := authProvidersServerWithAuth(t, nil)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/admin/auth/providers/"+providerName, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusNotFound, w.Code)
}

func newMockOIDCDiscoveryServer(t *testing.T) *httptest.Server {
	t.Helper()
	var issuerURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/.well-known/openid-configuration" {
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"issuer":"`+issuerURL+`",
			"authorization_endpoint":"`+issuerURL+`/authorize",
			"token_endpoint":"`+issuerURL+`/token",
			"userinfo_endpoint":"`+issuerURL+`/userinfo",
			"jwks_uri":"`+issuerURL+`/jwks"
		}`)
	}))
	issuerURL = server.URL
	t.Cleanup(server.Close)
	return server
}

// --- Test Connection Endpoint Tests ---

func TestAdminAuthProvidersTest_RequiresAdminToken(t *testing.T) {
	t.Parallel()
	srv, _ := authProvidersServerWithAuth(t, nil)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/auth/providers/google/test", nil)
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAdminAuthProvidersTest_UnconfiguredProviderFails(t *testing.T) {
	t.Parallel()
	// Google is a builtin but not configured (no client_id/secret).
	srv, token := authProvidersServerWithAuth(t, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/auth/providers/google/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)

	var resp testProviderResponse
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	testutil.False(t, resp.Success, "unconfigured provider should fail test")
	testutil.Equal(t, "google", resp.Provider)
	testutil.Contains(t, resp.Error, "not configured")
}

func TestAdminAuthProvidersTest_BuiltInProviderReachable(t *testing.T) {
	providerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.Equal(t, "/authorize", r.URL.Path)
		_, _ = io.WriteString(w, "ok")
	}))
	defer providerServer.Close()

	// Override the provider URLs before constructing the server so the
	// handler captures these URLs in its per-instance provider map.
	auth.SetProviderURLs("google", auth.OAuthProviderConfig{
		AuthURL:     providerServer.URL + "/authorize",
		TokenURL:    providerServer.URL + "/token",
		UserInfoURL: providerServer.URL + "/userinfo",
		Scopes:      []string{"openid", "email"},
	})
	t.Cleanup(func() { auth.ResetProviderURLs("google") })

	srv, token := authProvidersServerWithAuth(t, map[string]config.OAuthProvider{
		"google": {Enabled: true, ClientID: "test-id", ClientSecret: "test-secret"},
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/auth/providers/google/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)

	var resp testProviderResponse
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	testutil.True(t, resp.Success, "configured+reachable provider should pass test")
	testutil.Equal(t, "google", resp.Provider)
}

func TestAdminAuthProvidersTest_BuiltInProviderUnreachable(t *testing.T) {
	// Point the auth URL at a port that won't be listening.
	auth.SetProviderURLs("google", auth.OAuthProviderConfig{
		AuthURL:     "http://127.0.0.1:1/authorize",
		TokenURL:    "http://127.0.0.1:1/token",
		UserInfoURL: "http://127.0.0.1:1/userinfo",
		Scopes:      []string{"openid"},
	})
	t.Cleanup(func() { auth.ResetProviderURLs("google") })

	srv, token := authProvidersServerWithAuth(t, map[string]config.OAuthProvider{
		"google": {Enabled: true, ClientID: "test-id", ClientSecret: "test-secret"},
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/auth/providers/google/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)

	var resp testProviderResponse
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	testutil.False(t, resp.Success, "unreachable provider should fail test")
	testutil.Contains(t, resp.Error, "authorization endpoint unreachable")
}

func TestAdminAuthProvidersTest_BuiltInProviderHTTP404Fails(t *testing.T) {
	providerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.Equal(t, "/authorize", r.URL.Path)
		http.NotFound(w, r)
	}))
	defer providerServer.Close()

	auth.SetProviderURLs("google", auth.OAuthProviderConfig{
		AuthURL:     providerServer.URL + "/authorize",
		TokenURL:    providerServer.URL + "/token",
		UserInfoURL: providerServer.URL + "/userinfo",
		Scopes:      []string{"openid", "email"},
	})
	t.Cleanup(func() { auth.ResetProviderURLs("google") })

	srv, token := authProvidersServerWithAuth(t, map[string]config.OAuthProvider{
		"google": {Enabled: true, ClientID: "test-id", ClientSecret: "test-secret"},
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/auth/providers/google/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)

	var resp testProviderResponse
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	testutil.False(t, resp.Success, "404 response must fail connectivity test")
	testutil.Contains(t, resp.Error, "returned 404")
}

func TestAdminAuthProvidersTest_BuiltInProviderRedirectWithoutFollowSucceeds(t *testing.T) {
	providerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.Equal(t, "/authorize", r.URL.Path)
		w.Header().Set("Location", "http://127.0.0.1:1/will-never-connect")
		w.WriteHeader(http.StatusFound)
	}))
	defer providerServer.Close()

	auth.SetProviderURLs("google", auth.OAuthProviderConfig{
		AuthURL:     providerServer.URL + "/authorize",
		TokenURL:    providerServer.URL + "/token",
		UserInfoURL: providerServer.URL + "/userinfo",
		Scopes:      []string{"openid", "email"},
	})
	t.Cleanup(func() { auth.ResetProviderURLs("google") })

	srv, token := authProvidersServerWithAuth(t, map[string]config.OAuthProvider{
		"google": {Enabled: true, ClientID: "test-id", ClientSecret: "test-secret"},
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/auth/providers/google/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)

	var resp testProviderResponse
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	testutil.True(t, resp.Success, "redirecting auth endpoint should be considered reachable")
}

func TestAdminAuthProvidersTest_BuiltInProviderUsesHandlerTenantURL(t *testing.T) {
	var requestedURL string
	var providerURL string
	providerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedURL = providerURL + r.URL.Path
		_, _ = io.WriteString(w, "ok")
	}))
	defer providerServer.Close()
	providerURL = providerServer.URL

	auth.SetProviderURLs("microsoft", auth.OAuthProviderConfig{
		AuthURL:     providerURL + "/{tenant}/authorize",
		TokenURL:    providerURL + "/{tenant}/token",
		UserInfoURL: providerURL + "/userinfo",
		Scopes:      []string{"openid", "profile", "email", "User.Read"},
		TenantID:    "common",
	})
	t.Cleanup(func() { auth.ResetProviderURLs("microsoft") })

	srv, token := authProvidersServerWithAuth(t, map[string]config.OAuthProvider{
		"microsoft": {
			Enabled:      true,
			ClientID:     "ms-id",
			ClientSecret: "ms-secret",
			TenantID:     "contoso-tenant",
		},
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/auth/providers/microsoft/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)

	var resp testProviderResponse
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	testutil.True(t, resp.Success)
	testutil.Equal(t, providerURL+"/contoso-tenant/authorize", requestedURL)
}

func TestAdminAuthProvidersTest_OIDCProviderDiscoverySucceeds(t *testing.T) {
	providerName := "oidc_admin_test_connectivity"
	auth.UnregisterOIDCProvider(providerName)
	t.Cleanup(func() { auth.UnregisterOIDCProvider(providerName) })
	discoveryServer := newMockOIDCDiscoveryServer(t)
	issuerURL := discoveryServer.URL

	srv, token := authProvidersServerWithAuth(t, nil)

	// First create the OIDC provider via PUT.
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/admin/auth/providers/"+providerName,
		strings.NewReader(`{
			"enabled":true,
			"issuer_url":"`+issuerURL+`",
			"client_id":"oidc-test-id",
			"client_secret":"oidc-test-secret"
		}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)

	// Now test connectivity.
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/admin/auth/providers/"+providerName+"/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)

	var resp testProviderResponse
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	testutil.True(t, resp.Success, "OIDC provider with valid discovery should pass")
	testutil.Equal(t, providerName, resp.Provider)
}

func TestAdminAuthProvidersTest_OIDCProviderDiscoveryFailureIncludesError(t *testing.T) {
	providerName := "oidc_admin_test_connectivity_fail"
	auth.UnregisterOIDCProvider(providerName)
	t.Cleanup(func() { auth.UnregisterOIDCProvider(providerName) })
	discoveryServer := newMockOIDCDiscoveryServer(t)
	issuerURL := discoveryServer.URL

	srv, token := authProvidersServerWithAuth(t, nil)

	// First create the OIDC provider while discovery is reachable.
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/admin/auth/providers/"+providerName,
		strings.NewReader(`{
			"enabled":true,
			"issuer_url":"`+issuerURL+`",
			"client_id":"oidc-test-id",
			"client_secret":"oidc-test-secret"
		}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)

	// Now force discovery failure during provider test.
	discoveryServer.Close()

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/admin/auth/providers/"+providerName+"/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)

	var resp testProviderResponse
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	testutil.False(t, resp.Success, "OIDC provider with failed discovery should fail")
	testutil.Equal(t, providerName, resp.Provider)
	testutil.Contains(t, resp.Error, "OIDC discovery failed")
	testutil.Contains(t, resp.Error, "connect")
}

func TestRegisterAuthRoutes_OIDCRegistrationFailureClearsStaleProviderState(t *testing.T) {
	providerName := "oidc_startup_register_failure"

	auth.UnregisterOIDCProvider(providerName)
	t.Cleanup(func() { auth.UnregisterOIDCProvider(providerName) })

	// Seed stale global provider state that a new auth handler would otherwise
	// copy at construction time.
	auth.SetProviderURLs(providerName, auth.OAuthProviderConfig{
		AuthURL:     "https://stale.example.com/authorize",
		TokenURL:    "https://stale.example.com/token",
		UserInfoURL: "https://stale.example.com/userinfo",
		Scopes:      []string{"openid", "email"},
	})
	failedDiscoveryServer := newMockOIDCDiscoveryServer(t)
	issuerURL := failedDiscoveryServer.URL
	failedDiscoveryServer.Close()

	cfg := config.Default()
	cfg.Admin.Password = "admin-pass"
	cfg.Auth.Enabled = true
	cfg.Auth.JWTSecret = "test-secret-that-is-at-least-32-chars-long"
	cfg.Auth.OIDC = map[string]config.OIDCProvider{
		providerName: {
			Enabled:      true,
			IssuerURL:    issuerURL,
			ClientID:     "oidc-test-id",
			ClientSecret: "oidc-test-secret",
		},
	}

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

	providers := getAuthProviderMap(t, srv, loginResp["token"])
	_, exists := providers[providerName]
	testutil.False(t, exists, "failed OIDC startup registration must not keep stale provider config in auth handler")
}

func TestAdminAuthProvidersTest_UnknownProviderNotFound(t *testing.T) {
	t.Parallel()
	srv, token := authProvidersServerWithAuth(t, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/auth/providers/nonexistent_xyz/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusNotFound, w.Code)
}

type testProviderResponse struct {
	Success  bool   `json:"success"`
	Provider string `json:"provider"`
	Message  string `json:"message,omitempty"`
	Error    string `json:"error,omitempty"`
}

func getAuthProviderMap(t *testing.T, srv *server.Server, token string) map[string]auth.OAuthProviderInfo {
	t.Helper()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/auth/providers", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Providers []auth.OAuthProviderInfo `json:"providers"`
	}
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	out := make(map[string]auth.OAuthProviderInfo, len(resp.Providers))
	for _, p := range resp.Providers {
		out[p.Name] = p
	}
	return out
}
