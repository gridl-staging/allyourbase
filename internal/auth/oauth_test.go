package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/testutil"
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

func TestAuthorizationURLUnsupported(t *testing.T) {
	t.Parallel()
	client := OAuthClientConfig{ClientID: "id", ClientSecret: "secret"}
	_, err := AuthorizationURL("twitter", client, "http://localhost/callback", "state")
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

func TestParseGitHubUser(t *testing.T) {
	t.Parallel()
	body := `{"id":42,"login":"octocat","email":"octocat@github.com","name":"The Octocat"}`
	info, err := parseGitHubUser(context.Background(), []byte(body), "unused-token", oauthHTTPClient)
	testutil.NoError(t, err)
	testutil.Equal(t, "42", info.ProviderUserID)
	testutil.Equal(t, "octocat@github.com", info.Email)
	testutil.Equal(t, "The Octocat", info.Name)
}

func TestParseGitHubUserFallbackLoginAsName(t *testing.T) {
	t.Parallel()
	body := `{"id":42,"login":"octocat","email":"octocat@github.com","name":""}`
	info, err := parseGitHubUser(context.Background(), []byte(body), "unused-token", oauthHTTPClient)
	testutil.NoError(t, err)
	testutil.Equal(t, "octocat", info.Name)
}

func TestParseGitHubUserMissingID(t *testing.T) {
	t.Parallel()
	body := `{"login":"octocat"}`
	_, err := parseGitHubUser(context.Background(), []byte(body), "token", oauthHTTPClient)
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
	testutil.True(t, !containsParam(loc, "state=bogus"),
		"should generate new state when provided state doesn't match an SSE client")
}

// containsParam checks if the URL contains a specific query parameter value.
func containsParam(u, param string) bool {
	return len(u) > 0 && len(param) > 0 && contains(u, param)
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
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

func TestOAuthCallbackProviderErrorWithDescriptionParam(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	h.SetOAuthProvider("google", OAuthClientConfig{ClientID: "id", ClientSecret: "secret"})
	router := h.Routes()

	// Error with error_description
	req := httptest.NewRequest(http.MethodGet, "/oauth/google/callback?error=server_error&error_description=internal+failure", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "denied or failed")
}

func TestOAuthCallbackProviderErrorWithoutDescription(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	h.SetOAuthProvider("google", OAuthClientConfig{ClientID: "id", ClientSecret: "secret"})
	router := h.Routes()

	// Error without error_description
	req := httptest.NewRequest(http.MethodGet, "/oauth/google/callback?error=access_denied", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "denied or failed")
}

// --- OAuth state parameter security tests ---

func TestOAuthStateTampering(t *testing.T) {
	t.Parallel()
	store := NewOAuthStateStore(time.Minute)

	// Generate a valid state.
	validState, err := store.Generate()
	testutil.NoError(t, err)

	// Tamper with the state (modify a character).
	tamperedState := validState[:len(validState)-1] + "X"

	// Tampered state should fail validation.
	testutil.False(t, store.Validate(tamperedState), "tampered state should be rejected")

	// Original state should still work (one-time use).
	testutil.True(t, store.Validate(validState), "original state should validate")
}

func TestOAuthStateReuse(t *testing.T) {
	t.Parallel()
	store := NewOAuthStateStore(time.Minute)

	state, err := store.Generate()
	testutil.NoError(t, err)

	// First use succeeds.
	testutil.True(t, store.Validate(state), "first use should succeed")

	// Second use fails (consumed).
	testutil.False(t, store.Validate(state), "state reuse should be rejected")

	// Third use also fails.
	testutil.False(t, store.Validate(state), "multiple reuse attempts should fail")
}

func TestOAuthCallbackWithTamperedState(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	h.SetOAuthProvider("google", OAuthClientConfig{ClientID: "id", ClientSecret: "secret"})

	// Generate valid state.
	validState, err := h.oauthStateStore.Generate()
	testutil.NoError(t, err)

	// Tamper with state before callback.
	tamperedState := validState[:len(validState)-5] + "XXXXX"

	router := h.Routes()
	req := httptest.NewRequest(http.MethodGet, "/oauth/google/callback?code=abc&state="+tamperedState, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Tampered state should be rejected.
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "invalid or expired OAuth state")
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

func TestOAuthStateMissingParameter(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	h.SetOAuthProvider("google", OAuthClientConfig{ClientID: "id", ClientSecret: "secret"})
	router := h.Routes()

	// Callback without state parameter at all.
	req := httptest.NewRequest(http.MethodGet, "/oauth/google/callback?code=abc123", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "invalid or expired OAuth state")
}

func TestOAuthStateEmptyParameter(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	h.SetOAuthProvider("google", OAuthClientConfig{ClientID: "id", ClientSecret: "secret"})
	router := h.Routes()

	// Callback with empty state parameter.
	req := httptest.NewRequest(http.MethodGet, "/oauth/google/callback?code=abc&state=", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "invalid or expired OAuth state")
}
