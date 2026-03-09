package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/allyourbase/ayb/internal/testutil"
)

func newTestService() *Service {
	return &Service{
		jwtSecret:  []byte(testSecret),
		tokenDur:   time.Hour,
		refreshDur: 7 * 24 * time.Hour,
		minPwLen:   8,
		logger:     testutil.DiscardLogger(),
	}
}

func generateTestToken(t *testing.T, svc *Service, userID, email string) string {
	t.Helper()
	user := &User{ID: userID, Email: email}
	token, err := svc.generateToken(user)
	if err != nil {
		t.Fatalf("generating test token: %v", err)
	}
	return token
}

func TestRequireAuthValidToken(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	token := generateTestToken(t, svc, "user-1", "test@example.com")

	var gotClaims *Claims
	handler := RequireAuth(svc)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotClaims = ClaimsFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)
	testutil.NotNil(t, gotClaims)
	testutil.Equal(t, "user-1", gotClaims.Subject)
	testutil.Equal(t, "test@example.com", gotClaims.Email)
}

func TestRequireAuthMissingHeader(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	called := false
	handler := RequireAuth(svc)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusUnauthorized, w.Code)
	testutil.False(t, called, "next handler should not be called")
}

func TestRequireAuthMalformedHeader(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	handler := RequireAuth(svc)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// No "Bearer " prefix.
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Token abc123")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestRequireAuthExpiredToken(t *testing.T) {
	t.Parallel()
	svc := &Service{
		jwtSecret: []byte(testSecret),
		tokenDur:  -time.Hour,
	}
	token := generateTestToken(t, svc, "user-1", "test@example.com")

	validSvc := newTestService()
	handler := RequireAuth(validSvc)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestOptionalAuthNoHeader(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	var gotClaims *Claims
	handler := OptionalAuth(svc)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotClaims = ClaimsFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)
	testutil.Nil(t, gotClaims)
}

func TestOptionalAuthInvalidToken(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	var gotClaims *Claims
	handler := OptionalAuth(svc)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotClaims = ClaimsFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	// Invalid token should be silently ignored, not rejected.
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer invalid-garbage-token")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)
	testutil.Nil(t, gotClaims)
}

func TestOptionalAuthValidToken(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	token := generateTestToken(t, svc, "user-2", "other@example.com")

	var gotClaims *Claims
	handler := OptionalAuth(svc)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotClaims = ClaimsFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)
	testutil.NotNil(t, gotClaims)
	testutil.Equal(t, "user-2", gotClaims.Subject)
}

func TestOptionalAuth_MFAPendingToken_TreatedAsUnauthenticated(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	user := &User{ID: "550e8400-e29b-41d4-a716-446655440000", Email: "mfa@example.com"}

	token, err := svc.generateMFAPendingToken(user)
	testutil.NoError(t, err)

	var gotClaims *Claims
	handler := OptionalAuth(svc)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotClaims = ClaimsFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)
	testutil.Nil(t, gotClaims)
}

func TestClaimsFromContextNil(t *testing.T) {
	t.Parallel()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	claims := ClaimsFromContext(req.Context())
	testutil.Nil(t, claims)
}

func TestRequireAuthMissingHeaderDocURL(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	handler := RequireAuth(svc)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	var resp httputil.ErrorResponse
	testutil.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	testutil.Equal(t, "https://allyourbase.io/guide/authentication", resp.DocURL)
}

func TestRequireAuthExpiredTokenDocURL(t *testing.T) {
	t.Parallel()
	svc := &Service{
		jwtSecret: []byte(testSecret),
		tokenDur:  -time.Hour,
	}
	token := generateTestToken(t, svc, "user-1", "test@example.com")

	validSvc := newTestService()
	handler := RequireAuth(validSvc)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	var resp httputil.ErrorResponse
	testutil.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	testutil.Equal(t, "https://allyourbase.io/guide/authentication", resp.DocURL)
}

func TestValidateTokenOrAPIKeyOAuthWithNilPoolReturnsError(t *testing.T) {
	t.Parallel()
	svc := newTestService() // pool is nil in unit tests

	claims, err := validateTokenOrAPIKey(context.Background(), svc, "ayb_at_abcdef")
	testutil.Nil(t, claims)
	testutil.True(t, err != nil, "expected error when oauth token validation has no DB pool")
}

func TestValidateTokenOrAPIKeyAPIKeyWithNilPoolReturnsError(t *testing.T) {
	t.Parallel()
	svc := newTestService() // pool is nil in unit tests

	claims, err := validateTokenOrAPIKey(context.Background(), svc, "ayb_deadbeefcafebabe")
	testutil.Nil(t, claims)
	testutil.True(t, err != nil, "expected error when api key validation has no DB pool")
}

func TestRequireAuthAPIKeyWithNilPoolReturnsUnauthorized(t *testing.T) {
	t.Parallel()
	svc := newTestService() // pool is nil in unit tests

	handler := RequireAuth(svc)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer ayb_deadbeefcafebabe")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusUnauthorized, w.Code)
}

// --- OAuth scope enforcement via Claims conversion ---

func TestOAuthReadonlyScopeDeniesWriteViaClaimsConversion(t *testing.T) {
	t.Parallel()

	uid := "user-1"
	claims := oauthTokenInfoToClaims(&OAuthTokenInfo{
		UserID:   &uid,
		ClientID: "ayb_cid_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Scope:    ScopeReadOnly,
	})

	err := CheckWriteScope(claims)
	testutil.True(t, err != nil, "readonly OAuth token should deny write operations")
	testutil.True(t, errors.Is(err, ErrScopeReadOnly), "expected ErrScopeReadOnly")
}

func TestOAuthReadWriteScopeAllowsWriteViaClaimsConversion(t *testing.T) {
	t.Parallel()

	uid := "user-1"
	claims := oauthTokenInfoToClaims(&OAuthTokenInfo{
		UserID:   &uid,
		ClientID: "ayb_cid_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Scope:    ScopeReadWrite,
	})

	testutil.NoError(t, CheckWriteScope(claims))
}

func TestOAuthFullAccessScopeAllowsWriteViaClaimsConversion(t *testing.T) {
	t.Parallel()

	uid := "user-1"
	claims := oauthTokenInfoToClaims(&OAuthTokenInfo{
		UserID:   &uid,
		ClientID: "ayb_cid_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Scope:    ScopeFullAccess,
	})

	testutil.NoError(t, CheckWriteScope(claims))
}

func TestOAuthAllowedTablesDeniesUnauthorizedTableViaClaimsConversion(t *testing.T) {
	t.Parallel()

	uid := "user-1"
	claims := oauthTokenInfoToClaims(&OAuthTokenInfo{
		UserID:        &uid,
		ClientID:      "ayb_cid_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Scope:         ScopeReadWrite,
		AllowedTables: []string{"posts", "comments"},
	})

	testutil.NoError(t, CheckTableScope(claims, "posts"))
	testutil.NoError(t, CheckTableScope(claims, "comments"))

	err := CheckTableScope(claims, "users")
	testutil.True(t, err != nil, "OAuth token should deny access to unauthorized table")
	testutil.True(t, errors.Is(err, ErrScopeTableDenied), "expected ErrScopeTableDenied")
}

func TestOAuthEmptyAllowedTablesAllowsAllTables(t *testing.T) {
	t.Parallel()

	uid := "user-1"
	claims := oauthTokenInfoToClaims(&OAuthTokenInfo{
		UserID:   &uid,
		ClientID: "ayb_cid_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Scope:    ScopeReadWrite,
	})

	testutil.NoError(t, CheckTableScope(claims, "anything"))
	testutil.NoError(t, CheckTableScope(claims, "whatever"))
}

// --- Mixed auth coexistence ---

func TestValidateTokenOrAPIKeyRoutesJWT(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	token := generateTestToken(t, svc, "user-1", "test@example.com")

	claims, err := validateTokenOrAPIKey(context.Background(), svc, token)
	testutil.NoError(t, err)
	testutil.NotNil(t, claims)
	testutil.Equal(t, "user-1", claims.Subject)
	testutil.Equal(t, "", claims.APIKeyScope) // JWT tokens have no scope
}

func TestValidateTokenOrAPIKeyRoutesOAuthToken(t *testing.T) {
	t.Parallel()
	svc := newTestService() // nil pool

	// ayb_at_ prefix should be routed to ValidateOAuthToken, which fails
	// because pool is nil — confirming the routing works.
	claims, err := validateTokenOrAPIKey(context.Background(), svc, "ayb_at_deadbeef1234567890abcdef1234567890abcdef1234567890abcdef12345678")
	testutil.Nil(t, claims)
	testutil.True(t, err != nil, "oauth token with nil pool should error")
}

func TestValidateTokenOrAPIKeyRoutesAPIKey(t *testing.T) {
	t.Parallel()
	svc := newTestService() // nil pool

	// ayb_ prefix (not ayb_at_) should be routed to ValidateAPIKey, which fails
	// because pool is nil — confirming the routing works.
	claims, err := validateTokenOrAPIKey(context.Background(), svc, "ayb_deadbeef1234567890abcdef1234567890abcdef12345678")
	testutil.Nil(t, claims)
	testutil.True(t, err != nil, "api key with nil pool should error")
}

func TestOAuthTokenInfoToClaimsIncludesAppRateLimitFields(t *testing.T) {
	t.Parallel()

	userID := "user-123"
	claims := oauthTokenInfoToClaims(&OAuthTokenInfo{
		UserID:                    &userID,
		ClientID:                  "ayb_cid_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Scope:                     ScopeReadOnly,
		AllowedTables:             []string{"posts"},
		AppID:                     "app-123",
		AppRateLimitRPS:           42,
		AppRateLimitWindowSeconds: 75,
	})

	testutil.NotNil(t, claims)
	testutil.Equal(t, userID, claims.Subject)
	testutil.Equal(t, ScopeReadOnly, claims.APIKeyScope)
	testutil.Equal(t, "app-123", claims.AppID)
	testutil.Equal(t, 42, claims.AppRateLimitRPS)
	testutil.Equal(t, 75, claims.AppRateLimitWindow)
	testutil.Equal(t, 1, len(claims.AllowedTables))
	testutil.Equal(t, "posts", claims.AllowedTables[0])
}
