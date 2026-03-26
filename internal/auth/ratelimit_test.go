package auth

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestRateLimiterAllow(t *testing.T) {
	t.Parallel()
	rl := NewRateLimiter(3, time.Minute)
	defer rl.Stop()

	allowed, remaining, _ := rl.Allow("1.2.3.4")
	testutil.True(t, allowed, "first request should be allowed")
	testutil.Equal(t, 2, remaining)

	allowed, remaining, _ = rl.Allow("1.2.3.4")
	testutil.True(t, allowed, "second request should be allowed")
	testutil.Equal(t, 1, remaining)

	allowed, remaining, _ = rl.Allow("1.2.3.4")
	testutil.True(t, allowed, "third request should be allowed")
	testutil.Equal(t, 0, remaining)

	allowed, remaining, _ = rl.Allow("1.2.3.4")
	testutil.False(t, allowed, "fourth request should be rejected")
	testutil.Equal(t, 0, remaining)

	// Different IP should still be allowed.
	allowed, remaining, _ = rl.Allow("5.6.7.8")
	testutil.True(t, allowed, "different IP should be allowed")
	testutil.Equal(t, 2, remaining)
}

func TestRateLimiterWindowExpiry(t *testing.T) {
	t.Parallel()
	rl := NewRateLimiter(2, 20*time.Millisecond)
	defer rl.Stop()

	allowed, _, _ := rl.Allow("1.2.3.4")
	testutil.True(t, allowed, "first request")

	allowed, _, _ = rl.Allow("1.2.3.4")
	testutil.True(t, allowed, "second request")

	allowed, _, _ = rl.Allow("1.2.3.4")
	testutil.False(t, allowed, "third request rejected")

	// Sleep well past the window to avoid CI flakes.
	time.Sleep(50 * time.Millisecond)

	allowed, _, _ = rl.Allow("1.2.3.4")
	testutil.True(t, allowed, "should be allowed after window expires")
}

func TestRateLimiterMiddleware(t *testing.T) {
	t.Parallel()
	rl := NewRateLimiter(2, time.Minute)
	defer rl.Stop()

	handler := rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// First two requests succeed.
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req.RemoteAddr = "1.2.3.4:12345"
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		testutil.Equal(t, http.StatusOK, w.Code)
	}

	// Third request is rate limited.
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.RemoteAddr = "1.2.3.4:12345"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	testutil.Equal(t, http.StatusTooManyRequests, w.Code)
	retryAfter, err := strconv.Atoi(w.Header().Get("Retry-After"))
	testutil.NoError(t, err)
	testutil.True(t, retryAfter > 0 && retryAfter <= 61, "Retry-After should be 1-61, got %d", retryAfter)
}

func TestRateLimiterMiddlewareHeaders(t *testing.T) {
	t.Parallel()
	rl := NewRateLimiter(3, time.Minute)
	defer rl.Stop()

	handler := rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	tests := []struct {
		name              string
		expectedRemaining string
	}{
		{"first request", "2"},
		{"second request", "1"},
		{"third request", "0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Not parallel: subtests share a rate limiter and test sequential ordering.
			req := httptest.NewRequest(http.MethodPost, "/", nil)
			req.RemoteAddr = "10.0.0.1:12345"
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)

			testutil.Equal(t, http.StatusOK, w.Code)
			testutil.Equal(t, "3", w.Header().Get("X-RateLimit-Limit"))
			testutil.Equal(t, tt.expectedRemaining, w.Header().Get("X-RateLimit-Remaining"))
			resetEpoch, err := strconv.ParseInt(w.Header().Get("X-RateLimit-Reset"), 10, 64)
			testutil.NoError(t, err)
			testutil.True(t, resetEpoch > time.Now().Unix()-1, "X-RateLimit-Reset should be in the near future, got %d", resetEpoch)
		})
	}

	// Fourth request should be rejected with headers
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusTooManyRequests, w.Code)
	testutil.Equal(t, "3", w.Header().Get("X-RateLimit-Limit"))
	testutil.Equal(t, "0", w.Header().Get("X-RateLimit-Remaining"))
	resetEpoch429, err := strconv.ParseInt(w.Header().Get("X-RateLimit-Reset"), 10, 64)
	testutil.NoError(t, err)
	testutil.True(t, resetEpoch429 > time.Now().Unix()-1, "X-RateLimit-Reset should be in the near future, got %d", resetEpoch429)
	retryAfter, err := strconv.Atoi(w.Header().Get("Retry-After"))
	testutil.NoError(t, err)
	testutil.True(t, retryAfter > 0 && retryAfter <= 61, "Retry-After should be 1-61, got %d", retryAfter)
}
