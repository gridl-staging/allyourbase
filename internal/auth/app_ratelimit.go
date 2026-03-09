package auth

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/allyourbase/ayb/internal/httputil"
)

// AppRateLimiter enforces per-app rate limits based on the app's configured
// RateLimitRPS and RateLimitWindowSeconds. Each app gets its own sliding window.
// Apps with zero rate limits (unconfigured) are not rate-limited.
type AppRateLimiter struct {
	mu   sync.Mutex
	apps map[string]*appBucket
	stop chan struct{}
}

type appBucket struct {
	timestamps []time.Time
	limit      int
	window     time.Duration
}

// NewAppRateLimiter creates a per-app rate limiter.
func NewAppRateLimiter() *AppRateLimiter {
	arl := &AppRateLimiter{
		apps: make(map[string]*appBucket),
		stop: make(chan struct{}),
	}
	go arl.cleanup()
	return arl
}

// Stop terminates the background cleanup goroutine.
func (arl *AppRateLimiter) Stop() {
	close(arl.stop)
}

// allow checks whether the given app is within its rate limit.
func (arl *AppRateLimiter) allow(appID string, limit int, window time.Duration) (allowed bool, remaining int, resetTime time.Time) {
	arl.mu.Lock()
	defer arl.mu.Unlock()

	now := time.Now()

	b, ok := arl.apps[appID]
	if !ok {
		b = &appBucket{limit: limit, window: window}
		arl.apps[appID] = b
	}
	// Update limit/window in case app config changed.
	b.limit = limit
	b.window = window

	cutoff := now.Add(-b.window)
	pruneAppTimestamps(b, cutoff)

	if len(b.timestamps) >= b.limit {
		oldestExpiry := b.timestamps[0].Add(b.window)
		return false, 0, oldestExpiry
	}

	b.timestamps = append(b.timestamps, now)
	remaining = b.limit - len(b.timestamps)
	resetTime = now.Add(b.window)
	return true, remaining, resetTime
}

// Middleware returns HTTP middleware that enforces per-app rate limits.
// It reads Claims from request context. If the request has no app scope
// or the app has no rate limits configured (RPS=0), it passes through.
func (arl *AppRateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims := ClaimsFromContext(r.Context())
		if claims == nil || claims.AppID == "" || claims.AppRateLimitRPS <= 0 {
			next.ServeHTTP(w, r)
			return
		}

		window := time.Duration(claims.AppRateLimitWindow) * time.Second
		if window <= 0 {
			window = time.Minute // default window
		}

		allowed, remaining, resetTime := arl.allow(claims.AppID, claims.AppRateLimitRPS, window)

		w.Header().Set("X-App-RateLimit-Limit", strconv.Itoa(claims.AppRateLimitRPS))
		w.Header().Set("X-App-RateLimit-Remaining", strconv.Itoa(remaining))
		w.Header().Set("X-App-RateLimit-Reset", strconv.FormatInt(resetTime.Unix(), 10))

		if !allowed {
			retryAfter := int(time.Until(resetTime).Seconds()) + 1
			if retryAfter < 1 {
				retryAfter = 1
			}
			w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
			httputil.WriteError(w, http.StatusTooManyRequests, "app rate limit exceeded")
			return
		}

		next.ServeHTTP(w, r)
	})
}

func pruneAppTimestamps(b *appBucket, cutoff time.Time) {
	valid := b.timestamps[:0]
	for _, ts := range b.timestamps {
		if ts.After(cutoff) {
			valid = append(valid, ts)
		}
	}
	b.timestamps = valid
}

func (arl *AppRateLimiter) cleanup() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			arl.mu.Lock()
			now := time.Now()
			for id, b := range arl.apps {
				cutoff := now.Add(-b.window)
				pruneAppTimestamps(b, cutoff)
				if len(b.timestamps) == 0 {
					delete(arl.apps, id)
				}
			}
			arl.mu.Unlock()
		case <-arl.stop:
			return
		}
	}
}
