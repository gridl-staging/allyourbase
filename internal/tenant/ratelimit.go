// Package tenant TenantRateLimiter enforces per-tenant request rates in a sliding time window, managing request timestamps and cleaning up idle entries.
package tenant

import (
	"sync"
	"time"
)

const (
	defaultTenantRateWindow  = time.Minute
	tenantRateIdleWindow     = 5 * time.Minute
	tenantRateCleanupTimeout = time.Minute
)

type tenantWindowState struct {
	timestamps []time.Time
	lastSeen   time.Time
}

// TenantRateLimiter enforces per-tenant request rates in a sliding time window.
type TenantRateLimiter struct {
	mu       sync.Mutex
	windows  map[string]*tenantWindowState
	window   time.Duration
	idle     time.Duration
	stop     chan struct{}
	stopOnce sync.Once
}

// NewTenantRateLimiter creates a new tenant request rate limiter.
func NewTenantRateLimiter(window time.Duration) *TenantRateLimiter {
	if window <= 0 {
		window = defaultTenantRateWindow
	}

	rl := &TenantRateLimiter{
		windows: make(map[string]*tenantWindowState),
		window:  window,
		idle:    tenantRateIdleWindow,
		stop:    make(chan struct{}),
	}
	go rl.cleanup()
	return rl
}

// Stop terminates the cleanup goroutine.
func (rl *TenantRateLimiter) Stop() {
	rl.stopOnce.Do(func() {
		close(rl.stop)
	})
}

// Allow checks whether a request should be admitted.
func (rl *TenantRateLimiter) Allow(tenantID string, rpsHard, rpsSoft *int) (allowed bool, softWarning bool, remaining int, retryAfter time.Duration) {
	if tenantID == "" || (rpsHard == nil && rpsSoft == nil) {
		return true, false, 0, 0
	}

	rl.mu.Lock()
	defer rl.mu.Unlock()

	hardLimit, hasHard := normalizeRPSLimit(rpsHard)
	softLimit, hasSoft := normalizeRPSLimit(rpsSoft)
	if !hasHard && !hasSoft {
		return true, false, 0, 0
	}

	state, ok := rl.windows[tenantID]
	if !ok {
		state = &tenantWindowState{
			timestamps: make([]time.Time, 0),
		}
		rl.windows[tenantID] = state
	}

	now := time.Now().UTC()
	state.lastSeen = now
	cutoff := now.Add(-rl.window)
	state.timestamps = pruneTimestamps(state.timestamps, cutoff)

	requestLimit := int64(0)
	if hasHard {
		requestLimit = hardLimit
	} else if hasSoft {
		requestLimit = softLimit
	}

	if hasHard && int64(len(state.timestamps)) >= hardLimit {
		retryAfter = time.Until(state.timestamps[0].Add(rl.window))
		if retryAfter < time.Second {
			retryAfter = time.Second
		}
		return false, false, 0, retryAfter
	}

	state.timestamps = append(state.timestamps, now)

	remaining = computeRemaining(requestLimit, len(state.timestamps))
	if hasSoft && int64(len(state.timestamps)) >= softLimit && (!hasHard || int64(len(state.timestamps)) < hardLimit) {
		softWarning = true
	}

	return true, softWarning, remaining, rl.window
}

func computeRemaining(limit int64, current int) int {
	if limit <= 0 {
		return 0
	}
	remaining := limit - int64(current)
	if remaining < 0 {
		return 0
	}
	return int(remaining)
}

func normalizeRPSLimit(rps *int) (int64, bool) {
	val, ok := normalizeIntLimit(rps)
	if !ok {
		return 0, false
	}
	return val * int64(time.Minute.Seconds()), true
}

func pruneTimestamps(timestamps []time.Time, cutoff time.Time) []time.Time {
	valid := timestamps[:0]
	for _, ts := range timestamps {
		if ts.After(cutoff) {
			valid = append(valid, ts)
		}
	}
	return valid
}

// cleanup periodically removes expired request timestamps and deletes idle tenant windows until the limiter is stopped.
func (rl *TenantRateLimiter) cleanup() {
	ticker := time.NewTicker(tenantRateCleanupTimeout)
	defer ticker.Stop()

	for {
		select {
		case now := <-ticker.C:
			cutoff := now.Add(-rl.window)
			rl.mu.Lock()
			for tenantID, state := range rl.windows {
				state.timestamps = pruneTimestamps(state.timestamps, cutoff)
				if len(state.timestamps) == 0 && now.Sub(state.lastSeen) >= rl.idle {
					delete(rl.windows, tenantID)
				}
			}
			rl.mu.Unlock()
		case <-rl.stop:
			return
		}
	}
}
