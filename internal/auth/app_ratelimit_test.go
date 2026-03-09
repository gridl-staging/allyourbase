package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestAppRateLimiterAllow(t *testing.T) {
	t.Parallel()
	arl := NewAppRateLimiter()
	defer arl.Stop()

	// App with limit of 3 per minute.
	allowed, remaining, _ := arl.allow("app-1", 3, time.Minute)
	testutil.True(t, allowed, "first request should be allowed")
	testutil.Equal(t, 2, remaining)

	allowed, remaining, _ = arl.allow("app-1", 3, time.Minute)
	testutil.True(t, allowed, "second request")
	testutil.Equal(t, 1, remaining)

	allowed, remaining, _ = arl.allow("app-1", 3, time.Minute)
	testutil.True(t, allowed, "third request")
	testutil.Equal(t, 0, remaining)

	allowed, remaining, _ = arl.allow("app-1", 3, time.Minute)
	testutil.False(t, allowed, "fourth request should be denied")
	testutil.Equal(t, 0, remaining)

	// Different app should have its own bucket.
	allowed, remaining, _ = arl.allow("app-2", 3, time.Minute)
	testutil.True(t, allowed, "different app should be allowed")
	testutil.Equal(t, 2, remaining)
}

func TestAppRateLimiterWindowExpiry(t *testing.T) {
	t.Parallel()
	arl := NewAppRateLimiter()
	defer arl.Stop()

	allowed, _, _ := arl.allow("app-1", 2, 20*time.Millisecond)
	testutil.True(t, allowed, "first request")

	allowed, _, _ = arl.allow("app-1", 2, 20*time.Millisecond)
	testutil.True(t, allowed, "second request")

	allowed, _, _ = arl.allow("app-1", 2, 20*time.Millisecond)
	testutil.False(t, allowed, "third request rejected")

	time.Sleep(50 * time.Millisecond)

	allowed, _, _ = arl.allow("app-1", 2, 20*time.Millisecond)
	testutil.True(t, allowed, "should be allowed after window expires")
}

func TestAppRateLimiterMiddlewareNoAppID(t *testing.T) {
	t.Parallel()
	arl := NewAppRateLimiter()
	defer arl.Stop()

	handler := arl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// No claims at all — should pass through.
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)
}

func TestAppRateLimiterMiddlewareNoRateLimit(t *testing.T) {
	t.Parallel()
	arl := NewAppRateLimiter()
	defer arl.Stop()

	handler := arl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Claims with AppID but no rate limits (RPS=0) — should pass through.
	claims := &Claims{AppID: "app-1", AppRateLimitRPS: 0}
	ctx := context.WithValue(context.Background(), ctxKey{}, claims)
	req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)

	// No app rate limit headers when not rate-limited.
	testutil.Equal(t, "", w.Header().Get("X-App-RateLimit-Limit"))
}

func TestAppRateLimiterMiddleware429(t *testing.T) {
	t.Parallel()
	arl := NewAppRateLimiter()
	defer arl.Stop()

	handler := arl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	claims := &Claims{
		AppID:            "app-limited",
		AppRateLimitRPS:  2,
		AppRateLimitWindow: 60,
	}
	ctx := context.WithValue(context.Background(), ctxKey{}, claims)

	// First two requests pass.
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		testutil.Equal(t, http.StatusOK, w.Code)
		testutil.Equal(t, "2", w.Header().Get("X-App-RateLimit-Limit"))
	}

	// Third request gets 429.
	req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	testutil.Equal(t, http.StatusTooManyRequests, w.Code)
	testutil.Equal(t, "2", w.Header().Get("X-App-RateLimit-Limit"))
	testutil.Equal(t, "0", w.Header().Get("X-App-RateLimit-Remaining"))
	testutil.True(t, w.Header().Get("Retry-After") != "", "429 should set Retry-After header")
}

func TestAppRateLimiterMiddlewareHeaders(t *testing.T) {
	t.Parallel()
	arl := NewAppRateLimiter()
	defer arl.Stop()

	handler := arl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	claims := &Claims{
		AppID:            "app-headers",
		AppRateLimitRPS:  5,
		AppRateLimitWindow: 60,
	}
	ctx := context.WithValue(context.Background(), ctxKey{}, claims)

	req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)
	testutil.Equal(t, "5", w.Header().Get("X-App-RateLimit-Limit"))
	testutil.Equal(t, "4", w.Header().Get("X-App-RateLimit-Remaining"))
	testutil.True(t, w.Header().Get("X-App-RateLimit-Reset") != "", "should set reset header")
}

func TestAppRateLimiterIsolation(t *testing.T) {
	t.Parallel()
	arl := NewAppRateLimiter()
	defer arl.Stop()

	handler := arl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Exhaust app-A's limit.
	claimsA := &Claims{AppID: "app-A", AppRateLimitRPS: 1, AppRateLimitWindow: 60}
	ctxA := context.WithValue(context.Background(), ctxKey{}, claimsA)
	req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctxA)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)

	// App-A is now exhausted.
	req = httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctxA)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	testutil.Equal(t, http.StatusTooManyRequests, w.Code)

	// App-B should still be fine.
	claimsB := &Claims{AppID: "app-B", AppRateLimitRPS: 1, AppRateLimitWindow: 60}
	ctxB := context.WithValue(context.Background(), ctxKey{}, claimsB)
	req = httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctxB)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)
}

func TestAppRateLimiterMiddlewareDefaultWindow(t *testing.T) {
	// When AppRateLimitWindow is 0 (unconfigured), the middleware defaults to 1 minute.
	// The rate limiter should still enforce the RPS limit, not bypass it.
	t.Parallel()
	arl := NewAppRateLimiter()
	defer arl.Stop()

	handler := arl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	claims := &Claims{
		AppID:              "app-default-window",
		AppRateLimitRPS:    1,
		AppRateLimitWindow: 0, // triggers default 1-minute window
	}
	ctx := context.WithValue(context.Background(), ctxKey{}, claims)

	// First request should pass.
	req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)
	testutil.Equal(t, "1", w.Header().Get("X-App-RateLimit-Limit"))

	// Second request should be rate-limited.
	req = httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	testutil.Equal(t, http.StatusTooManyRequests, w.Code)
}
