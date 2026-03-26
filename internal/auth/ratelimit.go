// Package auth provides rate limiting functionality. This file implements RateLimiter, an in-memory per-IP sliding window rate limiter with periodic cleanup of stale entries.
package auth

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/allyourbase/ayb/internal/httputil"
)

// RateLimiter is a simple in-memory per-IP sliding window rate limiter.
type RateLimiter struct {
	mu       sync.Mutex
	visitors map[string]*visitor
	limit    int
	window   time.Duration
	stop     chan struct{}
}

type visitor struct {
	timestamps []time.Time
}

// NewRateLimiter creates a rate limiter that allows limit requests per window per IP.
// It starts a background goroutine to clean up stale entries.
func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	rl := &RateLimiter{
		visitors: make(map[string]*visitor),
		limit:    limit,
		window:   window,
		stop:     make(chan struct{}),
	}
	go rl.cleanup()
	return rl
}

// Stop terminates the background cleanup goroutine.
func (rl *RateLimiter) Stop() {
	close(rl.stop)
}

// Allow checks whether the given IP is within the rate limit.
// Returns allowed (bool), remaining (int), resetTime (time.Time).
func (rl *RateLimiter) Allow(ip string) (allowed bool, remaining int, resetTime time.Time) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-rl.window)

	v, ok := rl.visitors[ip]
	if !ok {
		v = &visitor{}
		rl.visitors[ip] = v
	}

	pruneTimestamps(v, cutoff)

	if len(v.timestamps) >= rl.limit {
		// Denied: return remaining=0 and reset time (when oldest timestamp expires)
		oldestExpiry := v.timestamps[0].Add(rl.window)
		return false, 0, oldestExpiry
	}

	v.timestamps = append(v.timestamps, now)
	remaining = rl.limit - len(v.timestamps)
	resetTime = now.Add(rl.window)
	return true, remaining, resetTime
}

// Middleware returns HTTP middleware that rate-limits by client IP.
func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := clientIP(r)
		allowed, remaining, resetTime := rl.Allow(ip)

		// Always set rate limit headers (even on success)
		w.Header().Set("X-RateLimit-Limit", strconv.Itoa(rl.limit))
		w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))
		w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(resetTime.Unix(), 10))

		if !allowed {
			retryAfter := int(time.Until(resetTime).Seconds()) + 1 // round up
			if retryAfter < 1 {
				retryAfter = 1
			}
			w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
			httputil.WriteErrorWithDocURL(w, http.StatusTooManyRequests, "too many requests",
				"https://allyourbase.io/guide/authentication")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// pruneTimestamps removes timestamps older than cutoff from a visitor in place.
func pruneTimestamps(v *visitor, cutoff time.Time) {
	valid := v.timestamps[:0]
	for _, ts := range v.timestamps {
		if ts.After(cutoff) {
			valid = append(valid, ts)
		}
	}
	v.timestamps = valid
}

// cleanup is a background goroutine that periodically removes stale visitor entries. It runs once per rate limit window, pruning timestamps older than the window duration and deleting visitor entries with no remaining timestamps. It terminates when Stop is called.
func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(rl.window)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			rl.mu.Lock()
			cutoff := time.Now().Add(-rl.window)
			for ip, v := range rl.visitors {
				pruneTimestamps(v, cutoff)
				if len(v.timestamps) == 0 {
					delete(rl.visitors, ip)
				}
			}
			rl.mu.Unlock()
		case <-rl.stop:
			return
		}
	}
}

func clientIP(r *http.Request) string {
	return httputil.ClientIP(r)
}
