package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/testutil"
)

// --- Anonymous Auth Claims Tests ---

func TestGenerateToken_AnonymousUser(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	user := &User{ID: "550e8400-e29b-41d4-a716-446655440001", Email: "", IsAnonymous: true}

	token, err := svc.generateToken(context.Background(), user)
	testutil.NoError(t, err)
	testutil.True(t, token != "", "token should not be empty")

	claims, err := svc.ValidateToken(token)
	testutil.NoError(t, err)
	testutil.Equal(t, user.ID, claims.Subject)
	testutil.Equal(t, "", claims.Email)
	testutil.True(t, claims.IsAnonymous, "IsAnonymous should be true in claims")
	testutil.Equal(t, "aal1", claims.AAL)
}

func TestGenerateToken_RegularUser_HasAAL1(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	user := &User{ID: "550e8400-e29b-41d4-a716-446655440002", Email: "test@example.com"}

	token, err := svc.generateToken(context.Background(), user)
	testutil.NoError(t, err)

	claims, err := svc.ValidateToken(token)
	testutil.NoError(t, err)
	testutil.Equal(t, "aal1", claims.AAL)
	testutil.True(t, !claims.IsAnonymous, "IsAnonymous should be false for regular user")
}

func TestGenerateTokenWithOpts_AAL2(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	user := &User{ID: "550e8400-e29b-41d4-a716-446655440003", Email: "mfa@example.com"}

	opts := &tokenOptions{
		AAL: "aal2",
		AMR: []string{"password", "totp"},
	}
	token, err := svc.generateTokenWithOpts(context.Background(), user, opts)
	testutil.NoError(t, err)

	claims, err := svc.ValidateToken(token)
	testutil.NoError(t, err)
	testutil.Equal(t, "aal2", claims.AAL)
	testutil.Equal(t, 2, len(claims.AMR))
	testutil.Equal(t, "password", claims.AMR[0])
	testutil.Equal(t, "totp", claims.AMR[1])
}

// --- Anonymous Auth Handler Tests ---

func TestAnonymousRoute_Disabled(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	router := h.Routes()
	// anonymousAuthEnabled defaults to false

	req := httptest.NewRequest(http.MethodPost, "/anonymous", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusNotFound, w.Code)
}

func TestAnonymousRoute_Enabled_NoPool(t *testing.T) {
	// Without a DB pool, CreateAnonymousUser will fail — test that the handler
	// returns 500, confirming it reaches the service layer.
	t.Parallel()
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	h.SetAnonymousAuthEnabled(true)
	router := h.Routes()

	req := httptest.NewRequest(http.MethodPost, "/anonymous", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestAnonymousRoute_Enabled_DefaultRateLimit(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	h.SetAnonymousAuthEnabled(true)
	router := h.Routes()

	for i := 0; i < 30; i++ {
		req := httptest.NewRequest(http.MethodPost, "/anonymous", nil)
		req.RemoteAddr = "198.51.100.77:45678"
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		testutil.Equal(t, http.StatusInternalServerError, w.Code)
		testutil.Equal(t, "30", w.Header().Get("X-RateLimit-Limit"))
	}

	req := httptest.NewRequest(http.MethodPost, "/anonymous", nil)
	req.RemoteAddr = "198.51.100.77:45678"
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusTooManyRequests, w.Code)
	testutil.Equal(t, "30", w.Header().Get("X-RateLimit-Limit"))
	testutil.Equal(t, "0", w.Header().Get("X-RateLimit-Remaining"))
	testutil.Contains(t, strings.ToLower(w.Body.String()), "too many requests")
}

func TestAnonymousRoute_EnableAfterRoutesBuilt_UsesRateLimit(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	router := h.Routes()
	h.SetAnonymousAuthEnabled(true)

	for i := 0; i < 30; i++ {
		req := httptest.NewRequest(http.MethodPost, "/anonymous", nil)
		req.RemoteAddr = "198.51.100.88:56789"
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		testutil.Equal(t, http.StatusInternalServerError, w.Code)
		testutil.Equal(t, "30", w.Header().Get("X-RateLimit-Limit"))
	}

	req := httptest.NewRequest(http.MethodPost, "/anonymous", nil)
	req.RemoteAddr = "198.51.100.88:56789"
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusTooManyRequests, w.Code)
	testutil.Equal(t, "30", w.Header().Get("X-RateLimit-Limit"))
	testutil.Equal(t, "0", w.Header().Get("X-RateLimit-Remaining"))
}

// --- Link OAuth Service Tests ---

func TestLinkOAuth_NilPool(t *testing.T) {
	t.Parallel()
	svc := newTestService() // no pool
	info := &OAuthUserInfo{ProviderUserID: "prov-1", Email: "user@example.com"}

	_, _, _, err := svc.LinkOAuth(context.Background(), "user-123", "google", info)
	testutil.True(t, err != nil, "expected error for nil pool")
	testutil.Contains(t, err.Error(), "database pool is not configured")
}

func TestUnlinkOAuth_CascadesProviderTokenDeletion(t *testing.T) {
	t.Parallel()

	svc := newTestService()
	identityStore := &fakeOAuthIdentityStore{deleted: true}
	tokenStore := &fakeUnlinkProviderTokenStore{}
	svc.SetOAuthIdentityStore(identityStore)
	svc.SetProviderTokenStore(tokenStore)

	err := svc.UnlinkOAuth(context.Background(), "user-123", "google")
	testutil.NoError(t, err)
	testutil.True(t, identityStore.called, "identity store should be called")
	testutil.Equal(t, "user-123", identityStore.userID)
	testutil.Equal(t, "google", identityStore.provider)
	testutil.Equal(t, 1, tokenStore.deleteCalls)
	testutil.Equal(t, "user-123", tokenStore.lastDeleteUserID)
	testutil.Equal(t, "google", tokenStore.lastDeleteProvider)
}

func TestUnlinkOAuth_OAuthIdentityNotFound(t *testing.T) {
	t.Parallel()

	svc := newTestService()
	identityStore := &fakeOAuthIdentityStore{deleted: false}
	tokenStore := &fakeUnlinkProviderTokenStore{}
	svc.SetOAuthIdentityStore(identityStore)
	svc.SetProviderTokenStore(tokenStore)

	err := svc.UnlinkOAuth(context.Background(), "user-123", "google")
	testutil.True(t, errors.Is(err, ErrOAuthLinkNotFound), "should return ErrOAuthLinkNotFound")
	testutil.Equal(t, 0, tokenStore.deleteCalls)
}

func TestUnlinkOAuth_IgnoresMissingProviderTokenRecord(t *testing.T) {
	t.Parallel()

	svc := newTestService()
	identityStore := &fakeOAuthIdentityStore{deleted: true}
	tokenStore := &fakeUnlinkProviderTokenStore{deleteErr: ErrProviderTokenNotFound}
	svc.SetOAuthIdentityStore(identityStore)
	svc.SetProviderTokenStore(tokenStore)

	err := svc.UnlinkOAuth(context.Background(), "user-123", "google")
	testutil.NoError(t, err)
	testutil.Equal(t, 1, tokenStore.deleteCalls)
}

// --- Anonymous Cleanup Tests ---

func TestCleanupAnonymousUsers_NilPool(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	_, err := svc.CleanupAnonymousUsers(context.Background(), 30*24*time.Hour)
	testutil.True(t, err != nil, "expected error for nil pool")
	testutil.Contains(t, err.Error(), "database pool is not configured")
}

func TestCleanupAnonymousUsers_DefaultTTL(t *testing.T) {
	t.Parallel()
	// Verify the default TTL constant is 30 days.
	testutil.Equal(t, 30*24*time.Hour, DefaultAnonymousTTL)
}

// --- Link Email Handler Tests ---

func TestLinkEmailRoute_NotAuthenticated(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	router := h.Routes()

	body := `{"email":"test@example.com","password":"password123"}`
	req := httptest.NewRequest(http.MethodPost, "/link/email", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	// No auth context — should return 401.
	router.ServeHTTP(w, req)
	testutil.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestLinkEmailHandler_NotAnonymous(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())

	// Set up auth context with a non-anonymous user.
	claims := &Claims{
		Email:       "existing@example.com",
		IsAnonymous: false,
	}
	claims.Subject = "user-123"

	body := `{"email":"test@example.com","password":"password123"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/link/email", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(ContextWithClaims(req.Context(), claims))
	w := httptest.NewRecorder()

	h.handleLinkEmail(w, req)
	testutil.Equal(t, http.StatusForbidden, w.Code)
	testutil.Contains(t, strings.ToLower(w.Body.String()), "anonymous")
}

func TestLinkEmailHandler_MissingFields(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())

	claims := &Claims{IsAnonymous: true}
	claims.Subject = "anon-user-123"

	cases := []struct {
		name string
		body string
	}{
		{"missing email", `{"password":"password123"}`},
		{"missing password", `{"email":"test@example.com"}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/auth/link/email", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			req = req.WithContext(ContextWithClaims(req.Context(), claims))
			w := httptest.NewRecorder()
			h.handleLinkEmail(w, req)
			testutil.Equal(t, http.StatusBadRequest, w.Code)
		})
	}
}

// --- Link OAuth Handler Tests ---

func TestLinkOAuthRoute_NotAuthenticated(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	router := h.Routes()

	body := `{"provider":"google","access_token":"fake-token"}`
	req := httptest.NewRequest(http.MethodPost, "/link/oauth", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)
	testutil.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestLinkOAuthHandler_NotAnonymous(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())

	claims := &Claims{
		Email:       "existing@example.com",
		IsAnonymous: false,
	}
	claims.Subject = "user-123"

	body := `{"provider":"google","access_token":"fake-token"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/link/oauth", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(ContextWithClaims(req.Context(), claims))
	w := httptest.NewRecorder()

	h.handleLinkOAuth(w, req)
	testutil.Equal(t, http.StatusForbidden, w.Code)
	testutil.Contains(t, strings.ToLower(w.Body.String()), "anonymous")
}

func TestLinkOAuthHandler_MissingFields(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())

	claims := &Claims{IsAnonymous: true}
	claims.Subject = "anon-user-123"

	cases := []struct {
		name string
		body string
		want string
	}{
		{"missing provider", `{"access_token":"fake-token"}`, "provider is required"},
		{"missing access_token", `{"provider":"google"}`, "access_token is required"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/auth/link/oauth", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			req = req.WithContext(ContextWithClaims(req.Context(), claims))
			w := httptest.NewRecorder()
			h.handleLinkOAuth(w, req)
			testutil.Equal(t, http.StatusBadRequest, w.Code)
			testutil.Contains(t, w.Body.String(), tc.want)
		})
	}
}

func TestLinkOAuthHandler_UnknownProvider(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())

	claims := &Claims{IsAnonymous: true}
	claims.Subject = "anon-user-123"

	body := `{"provider":"unknown-provider","access_token":"fake-token"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/link/oauth", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(ContextWithClaims(req.Context(), claims))
	w := httptest.NewRecorder()

	h.handleLinkOAuth(w, req)
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "not configured")
}

func TestLinkOAuthHandler_FetchesUserInfoAndCallsService(t *testing.T) {
	t.Parallel()

	// Set up a mock OAuth provider that returns user info.
	providerSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify the bearer token is forwarded.
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-access-token" {
			http.Error(w, "bad token", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"google-user-42","email":"linked@example.com","name":"Test User"}`))
	}))
	defer providerSrv.Close()

	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	// Override provider URL to point at our mock.
	h.SetProviderURLs("google", OAuthProviderConfig{
		UserInfoURL: providerSrv.URL,
	})

	claims := &Claims{IsAnonymous: true}
	claims.Subject = "anon-user-for-oauth"

	body := `{"provider":"google","access_token":"test-access-token"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/link/oauth", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(ContextWithClaims(req.Context(), claims))
	w := httptest.NewRecorder()

	h.handleLinkOAuth(w, req)

	// Without a DB pool, LinkOAuth will fail — but reaching the service layer
	// confirms the handler correctly fetched user info and called the service.
	testutil.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestLinkOAuthHandler_ProviderRejectsToken(t *testing.T) {
	t.Parallel()

	// Mock OAuth provider that rejects the token.
	providerSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "invalid token", http.StatusUnauthorized)
	}))
	defer providerSrv.Close()

	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	h.SetProviderURLs("google", OAuthProviderConfig{
		UserInfoURL: providerSrv.URL,
	})

	claims := &Claims{IsAnonymous: true}
	claims.Subject = "anon-user-bad-token"

	body := `{"provider":"google","access_token":"bad-token"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/link/oauth", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(ContextWithClaims(req.Context(), claims))
	w := httptest.NewRecorder()

	h.handleLinkOAuth(w, req)
	testutil.Equal(t, http.StatusBadGateway, w.Code)
	testutil.Contains(t, w.Body.String(), "failed to fetch user info")
}

func TestUnlinkOAuthRoute_NotAuthenticated(t *testing.T) {
	t.Parallel()

	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	router := h.Routes()

	body := `{"provider":"google"}`
	req := httptest.NewRequest(http.MethodDelete, "/link/oauth", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)
	testutil.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestUnlinkOAuthHandler_MissingProvider(t *testing.T) {
	t.Parallel()

	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	claims := &Claims{Email: "existing@example.com"}
	claims.Subject = "user-123"

	req := httptest.NewRequest(http.MethodDelete, "/api/auth/link/oauth", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(ContextWithClaims(req.Context(), claims))
	w := httptest.NewRecorder()

	h.handleUnlinkOAuth(w, req)
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "provider is required")
}

func TestUnlinkOAuthHandler_Success(t *testing.T) {
	t.Parallel()

	svc := newTestService()
	identityStore := &fakeOAuthIdentityStore{deleted: true}
	tokenStore := &fakeUnlinkProviderTokenStore{}
	svc.SetOAuthIdentityStore(identityStore)
	svc.SetProviderTokenStore(tokenStore)
	h := NewHandler(svc, testutil.DiscardLogger())

	claims := &Claims{Email: "existing@example.com"}
	claims.Subject = "user-123"
	req := httptest.NewRequest(http.MethodDelete, "/api/auth/link/oauth", strings.NewReader(`{"provider":"google"}`))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(ContextWithClaims(req.Context(), claims))
	w := httptest.NewRecorder()

	h.handleUnlinkOAuth(w, req)
	testutil.Equal(t, http.StatusNoContent, w.Code)
	testutil.True(t, identityStore.called, "identity store should be called")
	testutil.Equal(t, 1, tokenStore.deleteCalls)
}

type fakeOAuthIdentityStore struct {
	called   bool
	userID   string
	provider string
	deleted  bool
	err      error
}

func (f *fakeOAuthIdentityStore) DeleteOAuthIdentity(_ context.Context, userID, provider string) (bool, error) {
	f.called = true
	f.userID = userID
	f.provider = provider
	if f.err != nil {
		return false, f.err
	}
	return f.deleted, nil
}

type fakeUnlinkProviderTokenStore struct {
	deleteCalls        int
	lastDeleteUserID   string
	lastDeleteProvider string
	deleteErr          error
}

func (f *fakeUnlinkProviderTokenStore) StoreTokens(context.Context, string, string, string, string, string, string, *time.Time) error {
	return nil
}

func (f *fakeUnlinkProviderTokenStore) GetProviderToken(context.Context, string, string) (string, error) {
	return "", nil
}

func (f *fakeUnlinkProviderTokenStore) ListProviderTokens(context.Context, string) ([]ProviderTokenInfo, error) {
	return nil, nil
}

func (f *fakeUnlinkProviderTokenStore) DeleteProviderToken(_ context.Context, userID, provider string) error {
	f.deleteCalls++
	f.lastDeleteUserID = userID
	f.lastDeleteProvider = provider
	return f.deleteErr
}

func (f *fakeUnlinkProviderTokenStore) RefreshExpiringProviderTokens(context.Context, time.Duration) error {
	return nil
}

// --- MFA Enrollment Block for Anonymous Users ---

func TestMFAEnrollHandler_BlocksAnonymousUser(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	h.SetSMSEnabled(true)
	router := h.Routes()

	user := &User{ID: "anon-user-456", IsAnonymous: true}
	token, err := svc.generateToken(context.Background(), user)
	testutil.NoError(t, err)

	body := `{"phone":"+15551234567"}`
	req := httptest.NewRequest(http.MethodPost, "/mfa/sms/enroll", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)
	testutil.Equal(t, http.StatusForbidden, w.Code)
	testutil.Contains(t, w.Body.String(), "link your account")
}
