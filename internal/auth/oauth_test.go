package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/golang-jwt/jwt/v5"
)

func TestOAuthStateStoreGenerateAndValidate(t *testing.T) {
	t.Parallel()
	store := NewOAuthStateStore(time.Minute)

	token, err := store.Generate()
	testutil.NoError(t, err)
	testutil.True(t, len(token) > 0, "token should not be empty")

	// First validation succeeds.
	testutil.True(t, store.Validate(token), "first validate should succeed")

	// Second validation fails (one-time use).
	testutil.False(t, store.Validate(token), "second validate should fail (consumed)")
}

func TestOAuthStateStoreExpiry(t *testing.T) {
	t.Parallel()
	store := NewOAuthStateStore(1 * time.Millisecond)

	token, err := store.Generate()
	testutil.NoError(t, err)

	time.Sleep(5 * time.Millisecond)
	testutil.False(t, store.Validate(token), "expired token should fail")
}

func TestOAuthStateStoreInvalid(t *testing.T) {
	t.Parallel()
	store := NewOAuthStateStore(time.Minute)
	testutil.False(t, store.Validate("nonexistent"), "unknown token should fail")
}

func TestAuthorizationURLGoogle(t *testing.T) {
	t.Parallel()
	client := OAuthClientConfig{ClientID: "my-id", ClientSecret: "my-secret"}
	u, err := AuthorizationURL("google", client, "http://localhost/callback", "test-state")
	testutil.NoError(t, err)
	testutil.Contains(t, u, "accounts.google.com")
	testutil.Contains(t, u, "client_id=my-id")
	testutil.Contains(t, u, "state=test-state")
	testutil.Contains(t, u, "redirect_uri=")
	testutil.Contains(t, u, "scope=")
	testutil.Contains(t, u, "access_type=offline")
}

func TestAuthorizationURLGitHub(t *testing.T) {
	t.Parallel()
	client := OAuthClientConfig{ClientID: "gh-id", ClientSecret: "gh-secret"}
	u, err := AuthorizationURL("github", client, "http://localhost/callback", "test-state")
	testutil.NoError(t, err)
	testutil.Contains(t, u, "github.com/login/oauth/authorize")
	testutil.Contains(t, u, "client_id=gh-id")
	testutil.Contains(t, u, "scope=user")
}

func TestAuthorizationURLMicrosoftDefaultTenant(t *testing.T) {
	client := OAuthClientConfig{ClientID: "ms-id", ClientSecret: "ms-secret"}
	u, err := AuthorizationURL("microsoft", client, "http://localhost/callback", "test-state")
	testutil.NoError(t, err)
	testutil.Contains(t, u, "login.microsoftonline.com/common/oauth2/v2.0/authorize")
	testutil.Contains(t, u, "client_id=ms-id")
	testutil.Contains(t, u, "scope=openid+profile+email+User.Read")
}

func TestAuthorizationURLMicrosoftCustomTenant(t *testing.T) {
	SetProviderURLs("microsoft", OAuthProviderConfig{
		AuthURL:     "https://login.microsoftonline.com/{tenant}/oauth2/v2.0/authorize",
		TokenURL:    "https://login.microsoftonline.com/{tenant}/oauth2/v2.0/token",
		UserInfoURL: "https://graph.microsoft.com/v1.0/me",
		Scopes:      []string{"openid", "profile", "email", "User.Read"},
		TenantID:    "contoso-tenant",
	})
	t.Cleanup(func() {
		ResetProviderURLs("microsoft")
	})

	client := OAuthClientConfig{ClientID: "ms-id", ClientSecret: "ms-secret"}
	u, err := AuthorizationURL("microsoft", client, "http://localhost/callback", "test-state")
	testutil.NoError(t, err)
	testutil.Contains(t, u, "login.microsoftonline.com/contoso-tenant/oauth2/v2.0/authorize")
}

func TestHandleOAuthRedirectMicrosoftUsesHandlerTenantOverride(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	h.SetOAuthProvider("microsoft", OAuthClientConfig{ClientID: "ms-id", ClientSecret: "ms-secret"})
	h.SetProviderURLs("microsoft", OAuthProviderConfig{
		AuthURL:     "https://login.microsoftonline.com/{tenant}/oauth2/v2.0/authorize",
		TokenURL:    "https://login.microsoftonline.com/{tenant}/oauth2/v2.0/token",
		UserInfoURL: "https://graph.microsoft.com/v1.0/me",
		Scopes:      []string{"openid", "profile", "email", "User.Read"},
		TenantID:    "contoso-tenant",
	})
	router := h.Routes()

	req := httptest.NewRequest(http.MethodGet, "/oauth/microsoft", nil)
	req.Host = "localhost:8090"
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusTemporaryRedirect, w.Code)
	testutil.Contains(t, w.Header().Get("Location"), "login.microsoftonline.com/contoso-tenant/oauth2/v2.0/authorize")
}

func TestAuthorizationURLUnsupported(t *testing.T) {
	t.Parallel()
	client := OAuthClientConfig{ClientID: "id", ClientSecret: "secret"}
	_, err := AuthorizationURL("nonexistent", client, "http://localhost/callback", "state")
	testutil.ErrorContains(t, err, "not configured")
}

func TestParseGoogleUser(t *testing.T) {
	t.Parallel()
	body := `{"id":"12345","email":"user@gmail.com","name":"Test User"}`
	info, err := parseGoogleUser([]byte(body))
	testutil.NoError(t, err)
	testutil.Equal(t, "12345", info.ProviderUserID)
	testutil.Equal(t, "user@gmail.com", info.Email)
	testutil.Equal(t, "Test User", info.Name)
}

func TestParseGoogleUserMissingID(t *testing.T) {
	t.Parallel()
	body := `{"email":"user@gmail.com"}`
	_, err := parseGoogleUser([]byte(body))
	testutil.ErrorContains(t, err, "missing user ID")
}

func TestParseGitHubUserPayload(t *testing.T) {
	t.Parallel()
	body := `{"id":42,"login":"octocat","email":"octocat@github.com","name":"The Octocat"}`
	info, err := parseGitHubUserPayload([]byte(body))
	testutil.NoError(t, err)
	testutil.Equal(t, "42", info.ProviderUserID)
	testutil.Equal(t, "octocat@github.com", info.Email)
	testutil.Equal(t, "The Octocat", info.Name)
}

func TestParseGitHubUserPayloadFallbackLoginAsName(t *testing.T) {
	t.Parallel()
	body := `{"id":42,"login":"octocat","email":"octocat@github.com","name":""}`
	info, err := parseGitHubUserPayload([]byte(body))
	testutil.NoError(t, err)
	testutil.Equal(t, "octocat", info.Name)
}

func TestParseGitHubUserPayloadMissingID(t *testing.T) {
	t.Parallel()
	body := `{"login":"octocat"}`
	_, err := parseGitHubUserPayload([]byte(body))
	testutil.ErrorContains(t, err, "missing user ID")
}

func TestParseMicrosoftUser(t *testing.T) {
	t.Parallel()
	body := `{"id":"ms-user-1","mail":"ms@example.com","userPrincipalName":"ignored@example.com","displayName":"MS User"}`
	info, err := parseUserInfo("microsoft", []byte(body))
	testutil.NoError(t, err)
	testutil.Equal(t, "ms-user-1", info.ProviderUserID)
	testutil.Equal(t, "ms@example.com", info.Email)
	testutil.Equal(t, "MS User", info.Name)
}

func TestParseMicrosoftUserFallbackToUserPrincipalName(t *testing.T) {
	t.Parallel()
	body := `{"id":"ms-user-2","mail":"","userPrincipalName":"upn@example.com","displayName":"UPN User"}`
	info, err := parseUserInfo("microsoft", []byte(body))
	testutil.NoError(t, err)
	testutil.Equal(t, "ms-user-2", info.ProviderUserID)
	testutil.Equal(t, "upn@example.com", info.Email)
	testutil.Equal(t, "UPN User", info.Name)
}

func TestParseMicrosoftUserMissingID(t *testing.T) {
	t.Parallel()
	body := `{"mail":"ms@example.com","displayName":"MS User"}`
	_, err := parseUserInfo("microsoft", []byte(body))
	testutil.ErrorContains(t, err, "missing user ID")
}

func TestHandleOAuthRedirectUnknownProvider(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	router := h.Routes()

	req := httptest.NewRequest(http.MethodGet, "/oauth/twitter", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusNotFound, w.Code)
	testutil.Contains(t, w.Body.String(), "not configured")
}

func TestHandleOAuthRedirectConfiguredProvider(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	h.SetOAuthProvider("google", OAuthClientConfig{ClientID: "test-id", ClientSecret: "test-secret"})
	router := h.Routes()

	req := httptest.NewRequest(http.MethodGet, "/oauth/google", nil)
	req.Host = "localhost:8090"
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusTemporaryRedirect, w.Code)
	loc := w.Header().Get("Location")
	testutil.Contains(t, loc, "accounts.google.com")
	testutil.Contains(t, loc, "client_id=test-id")
	testutil.Contains(t, loc, "state=")
}

func TestHandleOAuthCallbackMissingState(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	h.SetOAuthProvider("google", OAuthClientConfig{ClientID: "id", ClientSecret: "secret"})
	router := h.Routes()

	req := httptest.NewRequest(http.MethodGet, "/oauth/google/callback?code=abc", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "invalid or expired OAuth state")
}

func TestHandleOAuthCallbackMissingCode(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	h.SetOAuthProvider("google", OAuthClientConfig{ClientID: "id", ClientSecret: "secret"})

	// Generate a valid state.
	state, err := h.oauthStateStore.Generate()
	testutil.NoError(t, err)

	router := h.Routes()
	req := httptest.NewRequest(http.MethodGet, "/oauth/google/callback?state="+state, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "missing authorization code")
}

func TestHandleOAuthCallbackPassesStateAsNonceToIDTokenParser(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	h.SetOAuthProvider("oidc_nonce_test", OAuthClientConfig{ClientID: "cid", ClientSecret: "secret"})

	var seenNonce string
	h.SetProviderURLs("oidc_nonce_test", OAuthProviderConfig{
		TokenURL:       "https://provider.test/token",
		UserInfoSource: OAuthUserInfoSourceIDToken,
		DiscoveryURL:   "https://idp.example.com",
		IDTokenUserInfoParser: func(ctx context.Context, idToken string) (*OAuthUserInfo, error) {
			seenNonce = oauthExpectedNonceFromContext(ctx)
			return nil, fmt.Errorf("forced parser failure")
		},
	})

	h.oauthHTTPClient = &http.Client{
		Transport: oauthRoundTripFunc(func(r *http.Request) (*http.Response, error) {
			switch r.URL.Path {
			case "/token":
				return oauthJSONResponse(http.StatusOK, `{"access_token":"tok","id_token":"header.payload.sig"}`), nil
			default:
				return oauthJSONResponse(http.StatusNotFound, `{"error":"not found"}`), nil
			}
		}),
	}

	state, err := h.oauthStateStore.Generate()
	testutil.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/oauth/oidc_nonce_test/callback?code=test-code&state="+url.QueryEscape(state), nil)
	w := httptest.NewRecorder()
	h.Routes().ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadGateway, w.Code)
	testutil.Equal(t, state, seenNonce)
}

func TestHandleOAuthCallbackProviderError(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	h.SetOAuthProvider("google", OAuthClientConfig{ClientID: "id", ClientSecret: "secret"})
	router := h.Routes()

	req := httptest.NewRequest(http.MethodGet, "/oauth/google/callback?error=access_denied&error_description=user+denied", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "denied or failed")
}

func TestOAuthCallbackURLDerivation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		host     string
		proto    string
		tls      bool
		provider string
		want     string
	}{
		{
			name:     "http",
			host:     "localhost:8090",
			provider: "google",
			want:     "http://localhost:8090/api/auth/oauth/google/callback",
		},
		{
			name:     "forwarded https",
			host:     "myapp.com",
			proto:    "https",
			provider: "github",
			want:     "https://myapp.com/api/auth/oauth/github/callback",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Host = tt.host
			if tt.proto != "" {
				req.Header.Set("X-Forwarded-Proto", tt.proto)
			}
			got := oauthCallbackURL(req, tt.provider)
			testutil.Equal(t, tt.want, got)
		})
	}
}

func TestOAuthHTTPClientTimeout(t *testing.T) {
	t.Parallel()
	testutil.True(t, oauthHTTPClient.Timeout > 0, "oauthHTTPClient should have a timeout")
	testutil.Equal(t, 10*time.Second, oauthHTTPClient.Timeout)
}

func TestExchangeCodeTimesOut(t *testing.T) {
	t.Parallel()
	// Server that delays until unblocked — allows clean shutdown after
	// the HTTP client timeout fires.
	done := make(chan struct{})
	slowServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-done:
		case <-r.Context().Done():
		}
	}))
	defer func() {
		close(done)
		slowServer.Close()
	}()

	// Use explicit deps — no global mutation, safe to run in parallel.
	fastClient := &http.Client{Timeout: 50 * time.Millisecond}
	pc := OAuthProviderConfig{TokenURL: slowServer.URL}
	client := OAuthClientConfig{ClientID: "id", ClientSecret: "secret"}
	_, err := exchangeCode(context.Background(), "google", client, "code", "http://localhost/callback", pc, fastClient)
	testutil.NotNil(t, err)
	testutil.ErrorContains(t, err, "code exchange failed")
}

func TestOAuthCallbackWithCodeExchangeFailure(t *testing.T) {
	t.Parallel()
	// Start a fake token endpoint that returns an error.
	fakeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "bad_code"})
	}))
	defer fakeServer.Close()

	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	h.SetOAuthProvider("google", OAuthClientConfig{ClientID: "id", ClientSecret: "secret"})
	// Override just the token URL on this handler instance — no global mutation.
	h.SetProviderURLs("google", OAuthProviderConfig{TokenURL: fakeServer.URL})

	state, err := h.oauthStateStore.Generate()
	testutil.NoError(t, err)

	router := h.Routes()
	req := httptest.NewRequest(http.MethodGet, "/oauth/google/callback?code=bad&state="+state, nil)
	req.Host = "localhost:8090"
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadGateway, w.Code)
	testutil.Contains(t, w.Body.String(), "failed to authenticate")
}

func TestHandleOAuthCallbackStoresProviderTokensWhenEnabled(t *testing.T) {
	t.Parallel()

	svc := newTestService()
	store := &fakeProviderTokenStore{}
	svc.SetProviderTokenStore(store)

	h := NewHandler(svc, testutil.DiscardLogger())
	h.SetOAuthProvider("google", OAuthClientConfig{ClientID: "id", ClientSecret: "secret"})
	h.SetOAuthProviderTokenStorage("google", true)
	h.SetProviderURLs("google", OAuthProviderConfig{
		TokenURL:    "https://provider.test/token",
		UserInfoURL: "https://provider.test/userinfo",
	})
	h.oauthLoginFn = func(_ context.Context, _ string, info *OAuthUserInfo) (*User, string, string, error) {
		return &User{ID: "user-1", Email: info.Email}, "ayb-access", "ayb-refresh", nil
	}
	h.oauthHTTPClient = &http.Client{
		Transport: oauthRoundTripFunc(func(r *http.Request) (*http.Response, error) {
			switch r.URL.Path {
			case "/token":
				return oauthJSONResponse(http.StatusOK, `{
					"access_token":"provider-access",
					"refresh_token":"provider-refresh",
					"token_type":"Bearer",
					"scope":"openid email profile",
					"expires_in":3600
				}`), nil
			case "/userinfo":
				return oauthJSONResponse(http.StatusOK, `{
					"id":"provider-user-1",
					"email":"tokenstore@example.com",
					"name":"Token Store User"
				}`), nil
			default:
				return oauthJSONResponse(http.StatusNotFound, `{"error":"not found"}`), nil
			}
		}),
	}

	state, err := h.oauthStateStore.Generate()
	testutil.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/oauth/google/callback?code=test-code&state="+state, nil)
	req.Host = "localhost:8090"
	w := httptest.NewRecorder()
	h.Routes().ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)
	testutil.Equal(t, 1, store.calls)
	testutil.Equal(t, "user-1", store.lastUserID)
	testutil.Equal(t, "google", store.lastProvider)
	testutil.Equal(t, "provider-access", store.lastAccessToken)
	testutil.Equal(t, "provider-refresh", store.lastRefreshToken)
	testutil.Equal(t, "Bearer", store.lastTokenType)
	testutil.Equal(t, "openid email profile", store.lastScopes)
	testutil.True(t, store.lastExpiresAtSet, "expected expires_at to be stored")
}

func TestHandleOAuthCallbackProviderTokenPersistenceFailureReturnsInternalError(t *testing.T) {
	t.Parallel()

	svc := newTestService()
	store := &failingProviderTokenStore{
		err: fmt.Errorf("store failed"),
	}
	svc.SetProviderTokenStore(store)

	h := NewHandler(svc, testutil.DiscardLogger())
	h.SetOAuthProvider("google", OAuthClientConfig{ClientID: "id", ClientSecret: "secret"})
	h.SetOAuthProviderTokenStorage("google", true)
	h.SetProviderURLs("google", OAuthProviderConfig{
		TokenURL:    "https://provider.test/token",
		UserInfoURL: "https://provider.test/userinfo",
	})
	h.oauthLoginFn = func(_ context.Context, _ string, info *OAuthUserInfo) (*User, string, string, error) {
		return &User{ID: "user-1", Email: info.Email}, "ayb-access", "ayb-refresh", nil
	}
	h.oauthHTTPClient = &http.Client{
		Transport: oauthRoundTripFunc(func(r *http.Request) (*http.Response, error) {
			switch r.URL.Path {
			case "/token":
				return oauthJSONResponse(http.StatusOK, `{
					"access_token":"provider-access",
					"refresh_token":"provider-refresh",
					"token_type":"Bearer",
					"scope":"openid email profile",
					"expires_in":3600
				}`), nil
			case "/userinfo":
				return oauthJSONResponse(http.StatusOK, `{
					"id":"provider-user-1",
					"email":"tokenstore@example.com",
					"name":"Token Store User"
				}`), nil
			default:
				return oauthJSONResponse(http.StatusNotFound, `{"error":"not found"}`), nil
			}
		}),
	}

	state, err := h.oauthStateStore.Generate()
	testutil.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/oauth/google/callback?code=test-code&state="+state, nil)
	req.Host = "localhost:8090"
	w := httptest.NewRecorder()
	h.Routes().ServeHTTP(w, req)

	testutil.Equal(t, http.StatusInternalServerError, w.Code)
	testutil.Contains(t, w.Body.String(), "internal error")
	testutil.Equal(t, 1, store.calls)
	testutil.Equal(t, "user-1", store.lastUserID)
	testutil.Equal(t, "google", store.lastProvider)
}

func TestHandleOAuthCallbackProviderTokenPersistenceFailurePublishesViaSSE(t *testing.T) {
	t.Parallel()

	svc := newTestService()
	store := &failingProviderTokenStore{
		err: fmt.Errorf("store failed"),
	}
	svc.SetProviderTokenStore(store)

	h := NewHandler(svc, testutil.DiscardLogger())
	h.SetOAuthProvider("google", OAuthClientConfig{ClientID: "id", ClientSecret: "secret"})
	h.SetOAuthProviderTokenStorage("google", true)
	h.SetProviderURLs("google", OAuthProviderConfig{
		TokenURL:    "https://provider.test/token",
		UserInfoURL: "https://provider.test/userinfo",
	})
	h.oauthLoginFn = func(_ context.Context, _ string, info *OAuthUserInfo) (*User, string, string, error) {
		return &User{ID: "user-1", Email: info.Email}, "ayb-access", "ayb-refresh", nil
	}
	h.oauthHTTPClient = &http.Client{
		Transport: oauthRoundTripFunc(func(r *http.Request) (*http.Response, error) {
			switch r.URL.Path {
			case "/token":
				return oauthJSONResponse(http.StatusOK, `{
					"access_token":"provider-access",
					"refresh_token":"provider-refresh",
					"token_type":"Bearer",
					"scope":"openid email profile",
					"expires_in":3600
				}`), nil
			case "/userinfo":
				return oauthJSONResponse(http.StatusOK, `{
					"id":"provider-user-1",
					"email":"tokenstore@example.com",
					"name":"Token Store User"
				}`), nil
			default:
				return oauthJSONResponse(http.StatusNotFound, `{"error":"not found"}`), nil
			}
		}),
	}

	pub := newFakeOAuthPublisher()
	pub.clients["sse-token-store-failure"] = true
	h.SetOAuthPublisher(pub)
	h.oauthStateStore.RegisterExternalState("sse-token-store-failure")

	req := httptest.NewRequest(http.MethodGet, "/oauth/google/callback?code=test-code&state=sse-token-store-failure", nil)
	req.Host = "localhost:8090"
	w := httptest.NewRecorder()
	h.Routes().ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)
	testutil.Contains(t, w.Body.String(), "Authentication complete")
	testutil.Contains(t, w.Body.String(), "window.close()")
	testutil.SliceLen(t, pub.published, 1)
	testutil.Equal(t, "sse-token-store-failure", pub.lastTarget)
	testutil.Equal(t, "internal error", pub.published[0].Error)
	testutil.Equal(t, 1, store.calls)
}

// --- OAuth SSE / Popup flow tests ---

// fakeOAuthPublisher implements auth.OAuthPublisher for tests.
type fakeOAuthPublisher struct {
	clients    map[string]bool
	published  []*OAuthEvent
	lastTarget string
}

func newFakeOAuthPublisher() *fakeOAuthPublisher {
	return &fakeOAuthPublisher{clients: make(map[string]bool)}
}

func (f *fakeOAuthPublisher) HasClient(id string) bool {
	return f.clients[id]
}

func (f *fakeOAuthPublisher) PublishOAuth(clientID string, event *OAuthEvent) {
	f.lastTarget = clientID
	f.published = append(f.published, event)
}

type failingProviderTokenStore struct {
	fakeProviderTokenStore
	err error
}

func (f *failingProviderTokenStore) StoreTokens(ctx context.Context, userID, provider, accessToken, refreshToken, tokenType, scopes string, expiresAt *time.Time) error {
	_ = f.fakeProviderTokenStore.StoreTokens(ctx, userID, provider, accessToken, refreshToken, tokenType, scopes, expiresAt)
	return f.err
}

func TestRegisterExternalState(t *testing.T) {
	t.Parallel()
	store := NewOAuthStateStore(time.Minute)

	store.RegisterExternalState("sse-client-1")

	// Should be valid and consumable.
	testutil.True(t, store.Validate("sse-client-1"), "registered external state should be valid")

	// One-time use: second validation fails.
	testutil.False(t, store.Validate("sse-client-1"), "external state should be consumed after first validate")
}

func TestRegisterExternalStateExpires(t *testing.T) {
	t.Parallel()
	store := NewOAuthStateStore(1 * time.Millisecond)

	store.RegisterExternalState("sse-client-1")
	time.Sleep(5 * time.Millisecond)

	testutil.False(t, store.Validate("sse-client-1"), "expired external state should fail")
}

func TestHandleOAuthRedirectWithSSEState(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	h.SetOAuthProvider("google", OAuthClientConfig{ClientID: "test-id", ClientSecret: "test-secret"})

	pub := newFakeOAuthPublisher()
	pub.clients["sse-client-42"] = true
	h.SetOAuthPublisher(pub)

	router := h.Routes()

	// Provide state that matches an active SSE client.
	req := httptest.NewRequest(http.MethodGet, "/oauth/google?state=sse-client-42", nil)
	req.Host = "localhost:8090"
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusTemporaryRedirect, w.Code)
	loc := w.Header().Get("Location")
	testutil.Contains(t, loc, "accounts.google.com")
	// The state should be the SSE client ID, not a newly generated one.
	testutil.Contains(t, loc, "state=sse-client-42")
}

func TestHandleOAuthRedirectIgnoresUnknownState(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	h.SetOAuthProvider("google", OAuthClientConfig{ClientID: "test-id", ClientSecret: "test-secret"})

	pub := newFakeOAuthPublisher()
	h.SetOAuthPublisher(pub)

	router := h.Routes()

	// Provide a state that doesn't match any SSE client.
	req := httptest.NewRequest(http.MethodGet, "/oauth/google?state=bogus", nil)
	req.Host = "localhost:8090"
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusTemporaryRedirect, w.Code)
	loc := w.Header().Get("Location")
	// State should be a newly generated one, not "bogus".
	testutil.True(t, !strings.Contains(loc, "state=bogus"),
		"should generate new state when provided state doesn't match an SSE client")
}

func TestOAuthCallbackPublishesErrorViaSSEOnExchangeFailure(t *testing.T) {
	t.Parallel()
	// Token endpoint returns an error — code exchange fails.
	fakeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "bad_code"})
	}))
	defer fakeServer.Close()

	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	h.SetOAuthProvider("google", OAuthClientConfig{ClientID: "id", ClientSecret: "secret"})
	// Override token and userinfo URLs on this handler instance — no global mutation.
	h.SetProviderURLs("google", OAuthProviderConfig{TokenURL: fakeServer.URL, UserInfoURL: fakeServer.URL})

	pub := newFakeOAuthPublisher()
	pub.clients["sse-client-99"] = true
	h.SetOAuthPublisher(pub)

	// Register the SSE clientId as valid state.
	h.oauthStateStore.RegisterExternalState("sse-client-99")

	router := h.Routes()
	req := httptest.NewRequest(http.MethodGet, "/oauth/google/callback?code=bad&state=sse-client-99", nil)
	req.Host = "localhost:8090"
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Code exchange failed — handler should publish error via SSE and show close page.
	testutil.Equal(t, http.StatusOK, w.Code)
	testutil.Contains(t, w.Body.String(), "Authentication complete")
	testutil.Contains(t, w.Body.String(), "window.close()")

	// Verify the publisher received an error event.
	testutil.SliceLen(t, pub.published, 1)
	testutil.Equal(t, "sse-client-99", pub.lastTarget)
	testutil.Contains(t, pub.published[0].Error, "failed to authenticate")
}

func TestOAuthCallbackFallsBackToJSONWithoutSSE(t *testing.T) {
	t.Parallel()
	// When the state doesn't match an SSE client, callback should behave
	// as before (JSON or redirect). We test the error path here.
	fakeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "bad"})
	}))
	defer fakeServer.Close()

	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	h.SetOAuthProvider("google", OAuthClientConfig{ClientID: "id", ClientSecret: "secret"})
	// Override token and userinfo URLs on this handler instance — no global mutation.
	h.SetProviderURLs("google", OAuthProviderConfig{TokenURL: fakeServer.URL, UserInfoURL: fakeServer.URL})

	pub := newFakeOAuthPublisher()
	h.SetOAuthPublisher(pub)

	state, err := h.oauthStateStore.Generate()
	testutil.NoError(t, err)

	router := h.Routes()
	req := httptest.NewRequest(http.MethodGet, "/oauth/google/callback?code=bad&state="+state, nil)
	req.Host = "localhost:8090"
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Should return JSON error, not the close page.
	testutil.Equal(t, http.StatusBadGateway, w.Code)
	testutil.Contains(t, w.Body.String(), "failed to authenticate")

	// Publisher should not have been called.
	testutil.SliceLen(t, pub.published, 0)
}

func TestOAuthProviderErrorPublishesViaSSEWhenPopup(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	h.SetOAuthProvider("google", OAuthClientConfig{ClientID: "id", ClientSecret: "secret"})

	pub := newFakeOAuthPublisher()
	pub.clients["sse-popup"] = true
	h.SetOAuthPublisher(pub)

	router := h.Routes()
	req := httptest.NewRequest(http.MethodGet,
		"/oauth/google/callback?error=access_denied&error_description=user+denied&state=sse-popup", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Should serve the close page.
	testutil.Equal(t, http.StatusOK, w.Code)
	testutil.Contains(t, w.Body.String(), "window.close()")

	// Should publish error via SSE.
	testutil.SliceLen(t, pub.published, 1)
	testutil.Contains(t, pub.published[0].Error, "denied or failed")
}

func TestOAuthCallbackPublishesSuccessViaSSEInsteadOfJSON(t *testing.T) {
	t.Parallel()

	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	h.SetOAuthProvider("google", OAuthClientConfig{ClientID: "id", ClientSecret: "secret"})
	h.SetProviderURLs("google", OAuthProviderConfig{
		TokenURL:    "https://provider.test/token",
		UserInfoURL: "https://provider.test/userinfo",
	})
	h.oauthLoginFn = func(_ context.Context, _ string, info *OAuthUserInfo) (*User, string, string, error) {
		return &User{ID: "user-1", Email: info.Email}, "ayb-access", "ayb-refresh", nil
	}
	h.oauthHTTPClient = &http.Client{
		Transport: oauthRoundTripFunc(func(r *http.Request) (*http.Response, error) {
			switch r.URL.Path {
			case "/token":
				return oauthJSONResponse(http.StatusOK, `{
					"access_token":"provider-access",
					"refresh_token":"provider-refresh",
					"token_type":"Bearer"
				}`), nil
			case "/userinfo":
				return oauthJSONResponse(http.StatusOK, `{
					"id":"provider-user-1",
					"email":"sseuser@example.com",
					"name":"SSE User"
				}`), nil
			default:
				return oauthJSONResponse(http.StatusNotFound, `{"error":"not found"}`), nil
			}
		}),
	}

	pub := newFakeOAuthPublisher()
	pub.clients["sse-success"] = true
	h.SetOAuthPublisher(pub)
	h.oauthStateStore.RegisterExternalState("sse-success")

	req := httptest.NewRequest(http.MethodGet, "/oauth/google/callback?code=test-code&state=sse-success", nil)
	req.Host = "localhost:8090"
	w := httptest.NewRecorder()
	h.Routes().ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)
	testutil.Contains(t, w.Body.String(), "Authentication complete")
	testutil.Contains(t, w.Body.String(), "window.close()")

	testutil.SliceLen(t, pub.published, 1)
	testutil.Equal(t, "sse-success", pub.lastTarget)
	testutil.Equal(t, "", pub.published[0].Error)
	testutil.Equal(t, "ayb-access", pub.published[0].Token)
	testutil.Equal(t, "ayb-refresh", pub.published[0].RefreshToken)
	user, ok := pub.published[0].User.(*User)
	testutil.True(t, ok, "expected OAuth event user payload")
	testutil.Equal(t, "sseuser@example.com", user.Email)
}

func TestOAuthCompletePage(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())

	w := httptest.NewRecorder()
	h.writeOAuthCompletePage(w)

	testutil.Equal(t, http.StatusOK, w.Code)
	testutil.Equal(t, "text/html; charset=utf-8", w.Header().Get("Content-Type"))
	testutil.Contains(t, w.Body.String(), "<!DOCTYPE html>")
	testutil.Contains(t, w.Body.String(), "Authentication complete")
	testutil.Contains(t, w.Body.String(), "window.close()")
}

func TestHandleOAuthCallbackEmptyCodeParam(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	h.SetOAuthProvider("google", OAuthClientConfig{ClientID: "id", ClientSecret: "secret"})

	state, err := h.oauthStateStore.Generate()
	testutil.NoError(t, err)

	router := h.Routes()
	// Empty code= parameter (e.g. "?code=&state=...")
	req := httptest.NewRequest(http.MethodGet, "/oauth/google/callback?code=&state="+state, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "missing authorization code")
}

func TestOAuthStateStoreConcurrentAccess(t *testing.T) {
	t.Parallel()
	store := NewOAuthStateStore(time.Minute)

	// Generate and validate tokens concurrently.
	const n = 50
	tokens := make([]string, n)
	for i := 0; i < n; i++ {
		tok, err := store.Generate()
		testutil.NoError(t, err)
		tokens[i] = tok
	}

	// Validate all tokens concurrently.
	results := make([]bool, n)
	done := make(chan int, n)
	for i := 0; i < n; i++ {
		go func(idx int) {
			results[idx] = store.Validate(tokens[idx])
			done <- idx
		}(i)
	}
	for i := 0; i < n; i++ {
		<-done
	}

	// All should have succeeded (one-time use).
	for i, ok := range results {
		testutil.True(t, ok, "token %d should validate successfully", i)
	}

	// Second validation should all fail.
	for _, tok := range tokens {
		testutil.False(t, store.Validate(tok), "token should be consumed")
	}
}

func TestOAuthCallbackStateReuseAfterSuccess(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	h.SetOAuthProvider("google", OAuthClientConfig{ClientID: "id", ClientSecret: "secret"})

	// Generate state.
	state, err := h.oauthStateStore.Generate()
	testutil.NoError(t, err)

	router := h.Routes()

	// First callback (even if it fails due to invalid code, state is consumed).
	req := httptest.NewRequest(http.MethodGet, "/oauth/google/callback?code=invalid&state="+state, nil)
	req.Host = "localhost:8090"
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Second callback with same state should fail.
	req2 := httptest.NewRequest(http.MethodGet, "/oauth/google/callback?code=another&state="+state, nil)
	req2.Host = "localhost:8090"
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)

	testutil.Equal(t, http.StatusBadRequest, w2.Code)
	testutil.Contains(t, w2.Body.String(), "invalid or expired OAuth state")
}

type oauthRoundTripFunc func(*http.Request) (*http.Response, error)

func (f oauthRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func oauthJSONResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header: http.Header{
			"Content-Type": []string{"application/json"},
		},
		Body: io.NopCloser(strings.NewReader(body)),
	}
}

func TestExchangeCode_UsesBasicAuthTokenMethod(t *testing.T) {
	t.Parallel()

	const provider = "test_basic_auth"
	SetOAuthUserInfoParser(provider, parseGoogleUser)
	t.Cleanup(func() {
		ResetOAuthUserInfoParser(provider)
	})

	var sawBasicAuth bool
	httpClient := &http.Client{
		Transport: oauthRoundTripFunc(func(r *http.Request) (*http.Response, error) {
			switch r.URL.Path {
			case "/token":
				wantAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte("cid:secret"))
				if r.Header.Get("Authorization") != wantAuth {
					return oauthJSONResponse(http.StatusUnauthorized, `{"error":"missing basic auth"}`), nil
				}
				sawBasicAuth = true
				payload, err := io.ReadAll(r.Body)
				if err != nil {
					return nil, err
				}
				form, err := url.ParseQuery(string(payload))
				if err != nil {
					return nil, err
				}
				if got := form.Get("client_secret"); got != "" {
					return oauthJSONResponse(http.StatusBadRequest, `{"error":"client_secret must not be in body"}`), nil
				}
				if got := form.Get("client_id"); got != "" {
					return oauthJSONResponse(http.StatusBadRequest, `{"error":"client_id must not be in body for Basic Auth (RFC 6749 §2.3.1)"}`), nil
				}
				return oauthJSONResponse(http.StatusOK, `{"access_token":"token-basic"}`), nil
			case "/userinfo":
				if r.Header.Get("Authorization") != "Bearer token-basic" {
					return oauthJSONResponse(http.StatusUnauthorized, `{"error":"missing bearer token"}`), nil
				}
				return oauthJSONResponse(http.StatusOK, `{"id":"u-basic","email":"basic@example.com","name":"Basic User"}`), nil
			default:
				return oauthJSONResponse(http.StatusNotFound, `{"error":"not found"}`), nil
			}
		}),
	}

	pc := OAuthProviderConfig{
		TokenURL:        "https://provider.test/token",
		UserInfoURL:     "https://provider.test/userinfo",
		TokenAuthMethod: OAuthTokenAuthMethodClientSecretBasic,
	}
	client := OAuthClientConfig{ClientID: "cid", ClientSecret: "secret"}

	info, err := exchangeCode(context.Background(), provider, client, "code123", "http://localhost/callback", pc, httpClient)
	testutil.NoError(t, err)
	testutil.True(t, sawBasicAuth, "expected token request to use basic auth")
	testutil.Equal(t, "u-basic", info.ProviderUserID)
	testutil.Equal(t, "basic@example.com", info.Email)
	testutil.Equal(t, "Basic User", info.Name)
}

func TestExchangeCode_BitbucketFetchesPrimaryEmailWhenMissing(t *testing.T) {
	t.Parallel()

	var sawEmailsEndpoint bool
	httpClient := &http.Client{
		Transport: oauthRoundTripFunc(func(r *http.Request) (*http.Response, error) {
			switch {
			case r.URL.Host == "provider.test" && r.URL.Path == "/token":
				return oauthJSONResponse(http.StatusOK, `{"access_token":"token-bitbucket"}`), nil
			case r.URL.Host == "provider.test" && r.URL.Path == "/userinfo":
				if r.Header.Get("Authorization") != "Bearer token-bitbucket" {
					return oauthJSONResponse(http.StatusUnauthorized, `{"error":"missing bearer token"}`), nil
				}
				return oauthJSONResponse(http.StatusOK, `{"uuid":"{bb-uid-1}","display_name":"Bitbucket User"}`), nil
			case r.URL.Host == "api.bitbucket.org" && r.URL.Path == "/2.0/user/emails":
				sawEmailsEndpoint = true
				if r.Header.Get("Authorization") != "Bearer token-bitbucket" {
					return oauthJSONResponse(http.StatusUnauthorized, `{"error":"missing bearer token on emails endpoint"}`), nil
				}
				return oauthJSONResponse(http.StatusOK, `{
					"values":[
						{"email":"secondary@example.com","is_primary":false,"is_confirmed":true},
						{"email":"primary@example.com","is_primary":true,"is_confirmed":true}
					]
				}`), nil
			default:
				return oauthJSONResponse(http.StatusNotFound, `{"error":"not found"}`), nil
			}
		}),
	}

	pc := OAuthProviderConfig{
		TokenURL:    "https://provider.test/token",
		UserInfoURL: "https://provider.test/userinfo",
	}
	client := OAuthClientConfig{ClientID: "cid", ClientSecret: "secret"}

	info, err := exchangeCode(context.Background(), "bitbucket", client, "code123", "http://localhost/callback", pc, httpClient)
	testutil.NoError(t, err)
	testutil.True(t, sawEmailsEndpoint, "expected bitbucket emails endpoint to be called when main userinfo has no email")
	testutil.Equal(t, "{bb-uid-1}", info.ProviderUserID)
	testutil.Equal(t, "Bitbucket User", info.Name)
	testutil.Equal(t, "primary@example.com", info.Email)
}

func TestExchangeCode_ExtractsUserInfoFromTokenResponse(t *testing.T) {
	t.Parallel()

	sawUserInfoEndpoint := false
	httpClient := &http.Client{
		Transport: oauthRoundTripFunc(func(r *http.Request) (*http.Response, error) {
			switch r.URL.Path {
			case "/token":
				return oauthJSONResponse(http.StatusOK, `{
					"access_token":"token-notion",
					"owner":{"user":{"id":"owner-1","email":"owner@example.com","name":"Owner User"}}
				}`), nil
			case "/userinfo":
				sawUserInfoEndpoint = true
				return oauthJSONResponse(http.StatusInternalServerError, `{"error":"should not call userinfo endpoint"}`), nil
			default:
				return oauthJSONResponse(http.StatusNotFound, `{"error":"not found"}`), nil
			}
		}),
	}

	pc := OAuthProviderConfig{
		TokenURL:       "https://provider.test/token",
		UserInfoURL:    "https://provider.test/userinfo",
		UserInfoSource: OAuthUserInfoSourceTokenResponse,
		TokenResponseUserInfoExtractor: func(tokenBody []byte) (*OAuthUserInfo, error) {
			var payload struct {
				Owner struct {
					User struct {
						ID    string `json:"id"`
						Email string `json:"email"`
						Name  string `json:"name"`
					} `json:"user"`
				} `json:"owner"`
			}
			if err := json.Unmarshal(tokenBody, &payload); err != nil {
				return nil, err
			}
			return &OAuthUserInfo{
				ProviderUserID: payload.Owner.User.ID,
				Email:          payload.Owner.User.Email,
				Name:           payload.Owner.User.Name,
			}, nil
		},
	}
	client := OAuthClientConfig{ClientID: "cid", ClientSecret: "secret"}

	info, err := exchangeCode(context.Background(), "notion", client, "code123", "http://localhost/callback", pc, httpClient)
	testutil.NoError(t, err)
	testutil.False(t, sawUserInfoEndpoint, "userinfo endpoint should be skipped")
	testutil.Equal(t, "owner-1", info.ProviderUserID)
	testutil.Equal(t, "owner@example.com", info.Email)
	testutil.Equal(t, "Owner User", info.Name)
}

func TestExchangeCode_UsesIDTokenUserInfoSource(t *testing.T) {
	t.Parallel()

	sawUserInfoEndpoint := false
	httpClient := &http.Client{
		Transport: oauthRoundTripFunc(func(r *http.Request) (*http.Response, error) {
			switch r.URL.Path {
			case "/token":
				return oauthJSONResponse(http.StatusOK, `{"access_token":"token-oidc","id_token":"header.payload.sig"}`), nil
			case "/userinfo":
				sawUserInfoEndpoint = true
				return oauthJSONResponse(http.StatusInternalServerError, `{"error":"should not call userinfo endpoint"}`), nil
			default:
				return oauthJSONResponse(http.StatusNotFound, `{"error":"not found"}`), nil
			}
		}),
	}

	pc := OAuthProviderConfig{
		TokenURL:       "https://provider.test/token",
		UserInfoURL:    "https://provider.test/userinfo",
		UserInfoSource: OAuthUserInfoSourceIDToken,
		IDTokenUserInfoParser: func(_ context.Context, idToken string) (*OAuthUserInfo, error) {
			testutil.Equal(t, "header.payload.sig", idToken)
			return &OAuthUserInfo{
				ProviderUserID: "oidc-sub-123",
				Email:          "oidc@example.com",
				Name:           "OIDC User",
			}, nil
		},
	}
	client := OAuthClientConfig{ClientID: "cid", ClientSecret: "secret"}

	info, err := exchangeCode(context.Background(), "oidc", client, "code123", "http://localhost/callback", pc, httpClient)
	testutil.NoError(t, err)
	testutil.False(t, sawUserInfoEndpoint, "userinfo endpoint should be skipped")
	testutil.Equal(t, "oidc-sub-123", info.ProviderUserID)
	testutil.Equal(t, "oidc@example.com", info.Email)
	testutil.Equal(t, "OIDC User", info.Name)
}

func TestExchangeCode_IDTokenParserReceivesExpectedNonceFromContext(t *testing.T) {
	t.Parallel()

	httpClient := &http.Client{
		Transport: oauthRoundTripFunc(func(r *http.Request) (*http.Response, error) {
			switch r.URL.Path {
			case "/token":
				return oauthJSONResponse(http.StatusOK, `{"access_token":"token-oidc","id_token":"header.payload.sig"}`), nil
			default:
				return oauthJSONResponse(http.StatusNotFound, `{"error":"not found"}`), nil
			}
		}),
	}

	pc := OAuthProviderConfig{
		TokenURL:       "https://provider.test/token",
		UserInfoSource: OAuthUserInfoSourceIDToken,
		IDTokenUserInfoParser: func(ctx context.Context, idToken string) (*OAuthUserInfo, error) {
			testutil.Equal(t, "nonce-state-42", oauthExpectedNonceFromContext(ctx))
			testutil.Equal(t, "header.payload.sig", idToken)
			return &OAuthUserInfo{
				ProviderUserID: "oidc-sub-123",
				Email:          "oidc@example.com",
				Name:           "OIDC User",
			}, nil
		},
	}
	client := OAuthClientConfig{ClientID: "cid", ClientSecret: "secret"}

	ctx := withOAuthExpectedNonce(context.Background(), "nonce-state-42")
	info, err := exchangeCode(ctx, "oidc", client, "code123", "http://localhost/callback", pc, httpClient)
	testutil.NoError(t, err)
	testutil.Equal(t, "oidc-sub-123", info.ProviderUserID)
}

func TestExchangeCode_AddsConfiguredUserInfoHeaders(t *testing.T) {
	t.Parallel()

	const provider = "test_custom_headers"
	SetOAuthUserInfoParser(provider, parseGoogleUser)
	t.Cleanup(func() {
		ResetOAuthUserInfoParser(provider)
	})

	var sawClientHeader bool
	httpClient := &http.Client{
		Transport: oauthRoundTripFunc(func(r *http.Request) (*http.Response, error) {
			switch r.URL.Path {
			case "/token":
				return oauthJSONResponse(http.StatusOK, `{"access_token":"token-twitch"}`), nil
			case "/userinfo":
				if r.Header.Get("Client-Id") == "twitch-client-id" {
					sawClientHeader = true
				}
				return oauthJSONResponse(http.StatusOK, `{"id":"tw-1","email":"tw@example.com","name":"Twitch User"}`), nil
			default:
				return oauthJSONResponse(http.StatusNotFound, `{"error":"not found"}`), nil
			}
		}),
	}

	pc := OAuthProviderConfig{
		TokenURL:       "https://provider.test/token",
		UserInfoURL:    "https://provider.test/userinfo",
		UserInfoSource: OAuthUserInfoSourceEndpoint,
		UserInfoHeaders: map[string]string{
			"Client-Id": "{client_id}",
		},
	}
	client := OAuthClientConfig{ClientID: "twitch-client-id", ClientSecret: "secret"}

	info, err := exchangeCode(context.Background(), provider, client, "code123", "http://localhost/callback", pc, httpClient)
	testutil.NoError(t, err)
	testutil.True(t, sawClientHeader, "expected userinfo request to include Client-Id header")
	testutil.Equal(t, "tw-1", info.ProviderUserID)
}

// --- Microsoft integration tests (full flow with mocked endpoints) ---

func TestMicrosoftOAuthIntegration_FullFlow(t *testing.T) {
	t.Parallel()

	// Mock Microsoft token + Graph API endpoints.
	fakeMicrosoft := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/contoso/oauth2/v2.0/token":
			// Verify the token request body.
			if err := r.ParseForm(); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if r.Form.Get("grant_type") != "authorization_code" {
				json.NewEncoder(w).Encode(map[string]string{"error": "invalid_grant"})
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if r.Form.Get("code") != "ms-auth-code" {
				w.WriteHeader(http.StatusBadRequest)
				json.NewEncoder(w).Encode(map[string]string{"error": "bad code"})
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{
				"access_token": "ms-access-token-123",
				"token_type":   "Bearer",
			})
		case "/v1.0/me":
			// Verify bearer token.
			if r.Header.Get("Authorization") != "Bearer ms-access-token-123" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":                "ms-user-abc-123",
				"mail":              "alice@contoso.com",
				"userPrincipalName": "alice@contoso.onmicrosoft.com",
				"displayName":       "Alice Contoso",
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer fakeMicrosoft.Close()

	// Test via exchangeCode directly with custom tenant URL templates resolved.
	pc := OAuthProviderConfig{
		TokenURL:    fakeMicrosoft.URL + "/{tenant}/oauth2/v2.0/token",
		UserInfoURL: fakeMicrosoft.URL + "/v1.0/me",
		Scopes:      []string{"openid", "profile", "email", "User.Read"},
		TenantID:    "contoso",
	}
	client := OAuthClientConfig{ClientID: "ms-client-id", ClientSecret: "ms-client-secret"}

	info, err := exchangeCode(context.Background(), "microsoft", client, "ms-auth-code", "http://localhost/callback", pc, fakeMicrosoft.Client())
	testutil.NoError(t, err)
	testutil.Equal(t, "ms-user-abc-123", info.ProviderUserID)
	testutil.Equal(t, "alice@contoso.com", info.Email)
	testutil.Equal(t, "Alice Contoso", info.Name)
}

func TestMicrosoftOAuthIntegration_FallbackToUPN(t *testing.T) {
	t.Parallel()

	// Graph API returns user with empty mail (org account without mailbox).
	fakeMicrosoft := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/common/oauth2/v2.0/token":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{
				"access_token": "ms-tok",
			})
		case "/v1.0/me":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":                "ms-no-mail-user",
				"mail":              nil,
				"userPrincipalName": "bob@contoso.onmicrosoft.com",
				"displayName":       "Bob NoMail",
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer fakeMicrosoft.Close()

	pc := OAuthProviderConfig{
		TokenURL:    fakeMicrosoft.URL + "/{tenant}/oauth2/v2.0/token",
		UserInfoURL: fakeMicrosoft.URL + "/v1.0/me",
		Scopes:      []string{"openid", "profile", "email", "User.Read"},
		TenantID:    "common",
	}
	client := OAuthClientConfig{ClientID: "ms-client-id", ClientSecret: "ms-secret"}

	info, err := exchangeCode(context.Background(), "microsoft", client, "code", "http://localhost/callback", pc, fakeMicrosoft.Client())
	testutil.NoError(t, err)
	testutil.Equal(t, "ms-no-mail-user", info.ProviderUserID)
	testutil.Equal(t, "bob@contoso.onmicrosoft.com", info.Email)
	testutil.Equal(t, "Bob NoMail", info.Name)
}

func TestMicrosoftOAuthIntegration_TokenExchangeFailure(t *testing.T) {
	t.Parallel()

	fakeMicrosoft := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error":             "invalid_grant",
			"error_description": "AADSTS70000: The provided authorization code has expired",
		})
	}))
	defer fakeMicrosoft.Close()

	pc := OAuthProviderConfig{
		TokenURL:    fakeMicrosoft.URL + "/{tenant}/oauth2/v2.0/token",
		UserInfoURL: fakeMicrosoft.URL + "/v1.0/me",
		TenantID:    "common",
	}
	client := OAuthClientConfig{ClientID: "ms-id", ClientSecret: "ms-secret"}

	_, err := exchangeCode(context.Background(), "microsoft", client, "expired-code", "http://localhost/callback", pc, fakeMicrosoft.Client())
	testutil.NotNil(t, err)
	testutil.ErrorContains(t, err, "code exchange failed")
}

func TestMicrosoftOAuthIntegration_HandlerRedirect(t *testing.T) {
	t.Parallel()

	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	h.SetOAuthProvider("microsoft", OAuthClientConfig{ClientID: "ms-handler-id", ClientSecret: "ms-handler-secret"})
	h.SetProviderURLs("microsoft", OAuthProviderConfig{
		AuthURL:     "https://login.microsoftonline.com/{tenant}/oauth2/v2.0/authorize",
		TokenURL:    "https://login.microsoftonline.com/{tenant}/oauth2/v2.0/token",
		UserInfoURL: "https://graph.microsoft.com/v1.0/me",
		Scopes:      []string{"openid", "profile", "email", "User.Read"},
		TenantID:    "mytenant",
	})

	router := h.Routes()

	// Test redirect uses correct tenant in auth URL.
	req := httptest.NewRequest(http.MethodGet, "/oauth/microsoft", nil)
	req.Host = "localhost:8090"
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusTemporaryRedirect, w.Code)
	loc := w.Header().Get("Location")
	testutil.Contains(t, loc, "login.microsoftonline.com/mytenant/oauth2/v2.0/authorize")
	testutil.Contains(t, loc, "client_id=ms-handler-id")
	testutil.Contains(t, loc, "scope=openid+profile+email+User.Read")

	// Verify state is present in the redirect.
	redirectURL, err := url.Parse(loc)
	testutil.NoError(t, err)
	state := redirectURL.Query().Get("state")
	testutil.True(t, state != "", "state should be non-empty")

	// Verify callback URL uses handler's host.
	testutil.Contains(t, loc, url.QueryEscape("http://localhost:8090/api/auth/oauth/microsoft/callback"))
}

// --- Apple integration tests (full flow with mocked endpoints) ---

func TestAppleOAuthIntegration_FullFlow(t *testing.T) {
	t.Parallel()

	const (
		appleTeamID      = "TEAM123456"
		appleClientID    = "com.allyourbase.test"
		appleClientKeyID = "APPLE_CLIENT_KID"
		appleJWKSKeyID   = "APPLE_JWKS_KID"
	)

	clientSecretSigner, clientSecretPEM := generateTestES256Key(t)
	appleSigner, _ := generateTestES256Key(t)
	jwks := buildTestJWKS(t, &appleSigner.PublicKey, appleJWKSKeyID)
	jwksBody, err := json.Marshal(jwks)
	testutil.NoError(t, err)

	sawUserInfoEndpoint := false
	httpClient := &http.Client{
		Transport: oauthRoundTripFunc(func(r *http.Request) (*http.Response, error) {
			switch r.URL.Path {
			case "/token":
				if err := r.ParseForm(); err != nil {
					return oauthJSONResponse(http.StatusBadRequest, `{"error":"invalid form body"}`), nil
				}
				if r.Form.Get("grant_type") != "authorization_code" {
					return oauthJSONResponse(http.StatusBadRequest, `{"error":"invalid grant_type"}`), nil
				}
				if r.Form.Get("code") != "apple-auth-code" {
					return oauthJSONResponse(http.StatusBadRequest, `{"error":"invalid code"}`), nil
				}
				if r.Form.Get("client_id") != appleClientID {
					return oauthJSONResponse(http.StatusBadRequest, `{"error":"invalid client_id"}`), nil
				}
				if r.Form.Get("redirect_uri") != "http://localhost/callback" {
					return oauthJSONResponse(http.StatusBadRequest, `{"error":"invalid redirect_uri"}`), nil
				}

				clientSecret := r.Form.Get("client_secret")
				parser := jwt.NewParser(
					jwt.WithValidMethods([]string{"ES256"}),
					jwt.WithIssuer(appleTeamID),
					jwt.WithAudience(appleIssuer),
					jwt.WithExpirationRequired(),
				)
				verified, err := parser.Parse(clientSecret, func(token *jwt.Token) (interface{}, error) {
					return &clientSecretSigner.PublicKey, nil
				})
				if err != nil {
					return oauthJSONResponse(http.StatusBadRequest, `{"error":"invalid client_secret"}`), nil
				}
				if verified.Header["kid"] != appleClientKeyID {
					return oauthJSONResponse(http.StatusBadRequest, `{"error":"invalid client_secret kid"}`), nil
				}
				claims, ok := verified.Claims.(jwt.MapClaims)
				if !ok {
					return oauthJSONResponse(http.StatusBadRequest, `{"error":"invalid client_secret claims"}`), nil
				}
				if claims["sub"] != appleClientID {
					return oauthJSONResponse(http.StatusBadRequest, `{"error":"invalid client_secret sub"}`), nil
				}

				now := time.Now()
				idTokenClaims := jwt.MapClaims{
					"iss":   appleIssuer,
					"aud":   appleClientID,
					"exp":   now.Add(time.Hour).Unix(),
					"iat":   now.Unix(),
					"sub":   "apple-user-123",
					"email": "appleuser@example.com",
				}
				idToken := jwt.NewWithClaims(jwt.SigningMethodES256, idTokenClaims)
				idToken.Header["kid"] = appleJWKSKeyID
				signedIDToken, err := idToken.SignedString(appleSigner)
				if err != nil {
					return oauthJSONResponse(http.StatusInternalServerError, `{"error":"failed to sign id_token"}`), nil
				}

				payload, err := json.Marshal(map[string]string{
					"access_token": "apple-access-token",
					"id_token":     signedIDToken,
					"token_type":   "Bearer",
				})
				if err != nil {
					return nil, err
				}
				return oauthJSONResponse(http.StatusOK, string(payload)), nil
			case "/auth/keys":
				return oauthJSONResponse(http.StatusOK, string(jwksBody)), nil
			case "/userinfo":
				sawUserInfoEndpoint = true
				return oauthJSONResponse(http.StatusInternalServerError, `{"error":"should not call userinfo endpoint"}`), nil
			default:
				return oauthJSONResponse(http.StatusNotFound, `{"error":"not found"}`), nil
			}
		}),
	}

	fetcher := NewAppleJWKSFetcher("https://apple.test/auth/keys", 24*time.Hour)
	fetcher.httpClient = httpClient
	pc := OAuthProviderConfig{
		TokenURL:       "https://apple.test/token",
		UserInfoURL:    "https://apple.test/userinfo",
		UserInfoSource: OAuthUserInfoSourceIDToken,
		TokenRequestMutator: func(ctx context.Context, provider string, client OAuthClientConfig, form url.Values, headers http.Header) error {
			secret, err := GenerateAppleClientSecret(AppleClientSecretParams{
				TeamID:     appleTeamID,
				ClientID:   client.ClientID,
				KeyID:      appleClientKeyID,
				PrivateKey: clientSecretPEM,
			})
			if err != nil {
				return err
			}
			form.Set("client_secret", secret)
			return nil
		},
		IDTokenUserInfoParser: func(_ context.Context, idToken string) (*OAuthUserInfo, error) {
			return VerifyAppleIDToken(idToken, appleClientID, fetcher)
		},
	}
	client := OAuthClientConfig{ClientID: appleClientID}

	info, err := exchangeCode(context.Background(), "apple", client, "apple-auth-code", "http://localhost/callback", pc, httpClient)
	testutil.NoError(t, err)
	testutil.False(t, sawUserInfoEndpoint, "userinfo endpoint should be skipped for Apple id_token flow")
	testutil.Equal(t, "apple-user-123", info.ProviderUserID)
	testutil.Equal(t, "appleuser@example.com", info.Email)
}

func TestAppleOAuthIntegration_HandlerRedirectUsesFormPost(t *testing.T) {
	t.Parallel()

	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	h.SetOAuthProvider("apple", OAuthClientConfig{ClientID: "apple-client-id", ClientSecret: "ignored"})
	h.SetProviderURLs("apple", OAuthProviderConfig{
		AuthURL:        "https://appleid.apple.com/auth/authorize",
		TokenURL:       "https://appleid.apple.com/auth/token",
		Scopes:         []string{"name", "email"},
		ResponseMode:   "form_post",
		UserInfoSource: OAuthUserInfoSourceIDToken,
	})

	router := h.Routes()
	req := httptest.NewRequest(http.MethodGet, "/oauth/apple", nil)
	req.Host = "localhost:8090"
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusTemporaryRedirect, w.Code)
	loc := w.Header().Get("Location")
	testutil.Contains(t, loc, "appleid.apple.com/auth/authorize")
	testutil.Contains(t, loc, "response_mode=form_post")
	testutil.Contains(t, loc, "scope=name+email")
}

func TestAppleOAuthIntegration_CallbackFormPostUsesFormCode(t *testing.T) {
	t.Parallel()

	sawCode := false
	httpClient := &http.Client{
		Transport: oauthRoundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.URL.Path != "/token" {
				return oauthJSONResponse(http.StatusNotFound, `{"error":"not found"}`), nil
			}
			body, err := io.ReadAll(r.Body)
			if err != nil {
				return nil, err
			}
			form, err := url.ParseQuery(string(body))
			if err != nil {
				return nil, err
			}
			if form.Get("code") == "apple-form-code" {
				sawCode = true
			}
			return oauthJSONResponse(http.StatusBadRequest, `{"error":"invalid_grant"}`), nil
		}),
	}

	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	h.oauthHTTPClient = httpClient
	h.SetOAuthProvider("apple", OAuthClientConfig{ClientID: "apple-id", ClientSecret: "apple-secret"})
	h.SetProviderURLs("apple", OAuthProviderConfig{
		TokenURL:       "https://apple.test/token",
		UserInfoSource: OAuthUserInfoSourceIDToken,
		TokenRequestMutator: func(ctx context.Context, provider string, client OAuthClientConfig, form url.Values, headers http.Header) error {
			form.Set("client_secret", "test-client-secret")
			return nil
		},
		IDTokenUserInfoParser: func(_ context.Context, idToken string) (*OAuthUserInfo, error) {
			return &OAuthUserInfo{ProviderUserID: "unused"}, nil
		},
	})

	state, err := h.oauthStateStore.Generate()
	testutil.NoError(t, err)

	form := url.Values{
		"code":  {"apple-form-code"},
		"state": {state},
	}
	req := httptest.NewRequest(http.MethodPost, "/oauth/apple/callback", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Host = "localhost:8090"

	w := httptest.NewRecorder()
	h.Routes().ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadGateway, w.Code)
	testutil.True(t, sawCode, "token exchange should use form_post code value")
}

func TestParseUserInfo_NilRegisteredParserReturnsError(t *testing.T) {
	t.Parallel()

	const provider = "test_nil_parser"
	SetOAuthUserInfoParser(provider, nil)
	t.Cleanup(func() {
		ResetOAuthUserInfoParser(provider)
	})

	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("parseUserInfo should not panic when parser is nil: %v", recovered)
		}
	}()

	_, err := parseUserInfo(provider, []byte(`{"id":"1"}`))
	testutil.ErrorContains(t, err, "not configured")
}

// --- Error-path coverage for exchangeCode / fetchUserInfoWithConfig ---

func TestExchangeCode_TokenRequestMutatorError(t *testing.T) {
	t.Parallel()

	httpClient := &http.Client{
		Transport: oauthRoundTripFunc(func(r *http.Request) (*http.Response, error) {
			t.Fatal("HTTP client should not be called when mutator fails")
			return nil, nil
		}),
	}

	pc := OAuthProviderConfig{
		TokenURL: "https://provider.test/token",
		TokenRequestMutator: func(ctx context.Context, provider string, client OAuthClientConfig, form url.Values, headers http.Header) error {
			return fmt.Errorf("key rotation in progress")
		},
	}
	client := OAuthClientConfig{ClientID: "cid", ClientSecret: "secret"}

	_, err := exchangeCode(context.Background(), "test", client, "code", "http://localhost/callback", pc, httpClient)
	testutil.NotNil(t, err)
	testutil.ErrorContains(t, err, "building token request")
	testutil.ErrorContains(t, err, "key rotation in progress")
}

func TestExchangeCode_TokenResponseSourceMissingExtractor(t *testing.T) {
	t.Parallel()

	httpClient := &http.Client{
		Transport: oauthRoundTripFunc(func(r *http.Request) (*http.Response, error) {
			return oauthJSONResponse(http.StatusOK, `{"access_token":"tok"}`), nil
		}),
	}

	pc := OAuthProviderConfig{
		TokenURL:                       "https://provider.test/token",
		UserInfoSource:                 OAuthUserInfoSourceTokenResponse,
		TokenResponseUserInfoExtractor: nil, // misconfigured
	}
	client := OAuthClientConfig{ClientID: "cid", ClientSecret: "secret"}

	_, err := exchangeCode(context.Background(), "notion", client, "code", "http://localhost/callback", pc, httpClient)
	testutil.NotNil(t, err)
	testutil.ErrorContains(t, err, "not configured")
	testutil.ErrorContains(t, err, "notion")
}

func TestExchangeCode_IDTokenSourceEmptyIDToken(t *testing.T) {
	t.Parallel()

	httpClient := &http.Client{
		Transport: oauthRoundTripFunc(func(r *http.Request) (*http.Response, error) {
			// Token response with access_token but no id_token.
			return oauthJSONResponse(http.StatusOK, `{"access_token":"tok"}`), nil
		}),
	}

	pc := OAuthProviderConfig{
		TokenURL:       "https://provider.test/token",
		UserInfoSource: OAuthUserInfoSourceIDToken,
		IDTokenUserInfoParser: func(_ context.Context, idToken string) (*OAuthUserInfo, error) {
			t.Fatal("parser should not be called with empty id_token")
			return nil, nil
		},
	}
	client := OAuthClientConfig{ClientID: "cid", ClientSecret: "secret"}

	_, err := exchangeCode(context.Background(), "apple", client, "code", "http://localhost/callback", pc, httpClient)
	testutil.NotNil(t, err)
	testutil.ErrorContains(t, err, "empty id_token")
}

func TestExchangeCode_IDTokenSourceMissingParser(t *testing.T) {
	t.Parallel()

	httpClient := &http.Client{
		Transport: oauthRoundTripFunc(func(r *http.Request) (*http.Response, error) {
			return oauthJSONResponse(http.StatusOK, `{"access_token":"tok","id_token":"header.payload.sig"}`), nil
		}),
	}

	pc := OAuthProviderConfig{
		TokenURL:              "https://provider.test/token",
		UserInfoSource:        OAuthUserInfoSourceIDToken,
		IDTokenUserInfoParser: nil, // misconfigured
	}
	client := OAuthClientConfig{ClientID: "cid", ClientSecret: "secret"}

	_, err := exchangeCode(context.Background(), "oidc", client, "code", "http://localhost/callback", pc, httpClient)
	testutil.NotNil(t, err)
	testutil.ErrorContains(t, err, "not configured")
	testutil.ErrorContains(t, err, "oidc")
}

// Verify that IDTokenWithEndpointFallback propagates parser errors rather than
// silently falling through to the userinfo endpoint.
func TestExchangeCode_IDTokenWithEndpointFallbackPropagatesParserError(t *testing.T) {
	t.Parallel()

	var endpointCalled bool
	httpClient := &http.Client{
		Transport: oauthRoundTripFunc(func(r *http.Request) (*http.Response, error) {
			switch r.URL.Path {
			case "/token":
				return oauthJSONResponse(http.StatusOK, `{"access_token":"tok","id_token":"header.payload.sig"}`), nil
			case "/userinfo":
				endpointCalled = true
				return oauthJSONResponse(http.StatusOK, `{"sub":"u1","email":"e@x.com"}`), nil
			default:
				return oauthJSONResponse(http.StatusNotFound, `{"error":"not found"}`), nil
			}
		}),
	}

	pc := OAuthProviderConfig{
		TokenURL:       "https://provider.test/token",
		UserInfoURL:    "https://provider.test/userinfo",
		UserInfoSource: OAuthUserInfoSourceIDTokenWithEndpointFallback,
		IDTokenUserInfoParser: func(_ context.Context, _ string) (*OAuthUserInfo, error) {
			return nil, fmt.Errorf("id_token verification failed: nonce mismatch")
		},
	}
	client := OAuthClientConfig{ClientID: "cid", ClientSecret: "secret"}

	_, err := exchangeCode(context.Background(), "oidc-fb", client, "code", "http://localhost/callback", pc, httpClient)
	testutil.NotNil(t, err)
	testutil.ErrorContains(t, err, "nonce mismatch")
	if endpointCalled {
		t.Fatal("expected userinfo endpoint NOT to be called when id_token parser returns an error")
	}
}

func TestExchangeCode_GitHubFetchesPrimaryEmailWhenMissing(t *testing.T) {
	t.Parallel()

	var sawEmailsEndpoint bool
	httpClient := &http.Client{
		Transport: oauthRoundTripFunc(func(r *http.Request) (*http.Response, error) {
			switch {
			case r.URL.Host == "provider.test" && r.URL.Path == "/token":
				return oauthJSONResponse(http.StatusOK, `{"access_token":"gh-tok"}`), nil
			case r.URL.Host == "provider.test" && r.URL.Path == "/userinfo":
				if r.Header.Get("Authorization") != "Bearer gh-tok" {
					return oauthJSONResponse(http.StatusUnauthorized, `{"error":"bad token"}`), nil
				}
				// GitHub returns user with no email — triggers email fetch.
				return oauthJSONResponse(http.StatusOK, `{"id":42,"login":"octocat","name":"Octocat"}`), nil
			case r.URL.Host == "api.github.com" && r.URL.Path == "/user/emails":
				sawEmailsEndpoint = true
				if r.Header.Get("Authorization") != "Bearer gh-tok" {
					return oauthJSONResponse(http.StatusUnauthorized, `{"error":"bad token"}`), nil
				}
				return oauthJSONResponse(http.StatusOK, `[
					{"email":"secondary@example.com","primary":false,"verified":true},
					{"email":"primary@example.com","primary":true,"verified":true}
				]`), nil
			default:
				return oauthJSONResponse(http.StatusNotFound, `{"error":"not found"}`), nil
			}
		}),
	}

	pc := OAuthProviderConfig{
		TokenURL:    "https://provider.test/token",
		UserInfoURL: "https://provider.test/userinfo",
	}
	client := OAuthClientConfig{ClientID: "cid", ClientSecret: "secret"}

	info, err := exchangeCode(context.Background(), "github", client, "code123", "http://localhost/callback", pc, httpClient)
	testutil.NoError(t, err)
	testutil.True(t, sawEmailsEndpoint, "expected github emails endpoint to be called when main userinfo has no email")
	testutil.Equal(t, "42", info.ProviderUserID)
	testutil.Equal(t, "Octocat", info.Name)
	testutil.Equal(t, "primary@example.com", info.Email)
}
