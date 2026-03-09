package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/golang-jwt/jwt/v5"
)

func signedAuthToken(t *testing.T, secret string, claims *auth.Claims) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	s, err := tok.SignedString([]byte(secret))
	testutil.NoError(t, err)
	return s
}

func TestRequireAdminOrUserAuthAppRateLimitEnforced(t *testing.T) {
	t.Parallel()

	secret := "middleware-test-secret"
	authSvc := auth.NewService(nil, secret, time.Hour, 24*time.Hour, 8, testutil.DiscardLogger())

	s := &Server{appRL: auth.NewAppRateLimiter()}
	defer s.appRL.Stop()

	h := s.requireAdminOrUserAuth(authSvc)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	token := signedAuthToken(t, secret, &auth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "user-1",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(5 * time.Minute)),
		},
		Email:              "test@example.com",
		AppID:              "app-1",
		AppRateLimitRPS:    1,
		AppRateLimitWindow: 60,
	})

	req1 := httptest.NewRequest(http.MethodGet, "/api/posts", nil)
	req1.Header.Set("Authorization", "Bearer "+token)
	w1 := httptest.NewRecorder()
	h.ServeHTTP(w1, req1)
	testutil.Equal(t, http.StatusOK, w1.Code)

	req2 := httptest.NewRequest(http.MethodGet, "/api/posts", nil)
	req2.Header.Set("Authorization", "Bearer "+token)
	w2 := httptest.NewRecorder()
	h.ServeHTTP(w2, req2)
	testutil.Equal(t, http.StatusTooManyRequests, w2.Code)
	testutil.Equal(t, "1", w2.Header().Get("X-App-RateLimit-Limit"))
	testutil.Equal(t, "0", w2.Header().Get("X-App-RateLimit-Remaining"))
}

func TestRequireAdminOrUserAuthAdminBypassesRateLimit(t *testing.T) {
	// Admin tokens must bypass app rate limits entirely.
	// The admin fast-path goes to `next` directly, not through the rate limiter.
	t.Parallel()

	secret := "middleware-test-secret"
	authSvc := auth.NewService(nil, secret, time.Hour, 24*time.Hour, 8, testutil.DiscardLogger())

	adminAuth := newAdminAuth("test-password")
	adminToken := adminAuth.token()

	s := &Server{
		appRL:     auth.NewAppRateLimiter(),
		adminAuth: adminAuth,
	}
	defer s.appRL.Stop()

	h := s.requireAdminOrUserAuth(authSvc)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Make many requests with admin token — none should be rate-limited.
	for i := 0; i < 10; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/posts", nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		testutil.Equal(t, http.StatusOK, w.Code)
		// Admin requests should NOT have rate limit headers.
		testutil.Equal(t, "", w.Header().Get("X-App-RateLimit-Limit"))
	}
}
