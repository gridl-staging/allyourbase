package tenant

import (
	"testing"
	"time"
)

func TestTenantRateLimiter_AllowUnderLimit(t *testing.T) {
	t.Parallel()

	hard := 1

	rl := NewTenantRateLimiter(time.Minute)
	t.Cleanup(func() { rl.Stop() })

	tenantID := "tenant-1"

	for i := 0; i < 59; i++ {
		allowed, _, remaining, _ := rl.Allow(tenantID, &hard, nil)
		if !allowed {
			t.Fatalf("request %d should be allowed", i+1)
		}
		expectedRemaining := 60 - (i + 1)
		if remaining != expectedRemaining {
			t.Fatalf("request %d: remaining=%d, want %d", i+1, remaining, expectedRemaining)
		}
	}
}

func TestTenantRateLimiter_DenyAtHardLimitReturnsRetryAfter(t *testing.T) {
	t.Parallel()

	hard := 1
	soft := 1

	rl := NewTenantRateLimiter(time.Minute)
	t.Cleanup(func() { rl.Stop() })

	tenantID := "tenant-2"
	for i := 0; i < 60; i++ {
		allowed, _, _, _ := rl.Allow(tenantID, &hard, &soft)
		if !allowed {
			t.Fatalf("request %d should be allowed before hard limit", i+1)
		}
	}

	allowed, softWarn, remaining, retryAfter := rl.Allow(tenantID, &hard, &soft)
	if allowed {
		t.Fatal("61st request should be denied by hard limit")
	}
	if softWarn {
		t.Fatal("soft warning should not be set when hard limit is hit")
	}
	if remaining != 0 {
		t.Fatalf("remaining=%d, want 0", remaining)
	}
	if retryAfter <= 0 {
		t.Fatalf("retryAfter=%v, expected positive duration", retryAfter)
	}
}

func TestTenantRateLimiter_SoftWarningBetweenLimits(t *testing.T) {
	t.Parallel()

	hard := 2
	soft := 1

	rl := NewTenantRateLimiter(time.Minute)
	t.Cleanup(func() { rl.Stop() })

	tenantID := "tenant-3"
	for i := 0; i < 59; i++ {
		allowed, softWarn, _, _ := rl.Allow(tenantID, &hard, &soft)
		if !allowed {
			t.Fatalf("request %d should be allowed", i+1)
		}
		if softWarn {
			t.Fatalf("request %d should not be soft warning", i+1)
		}
	}

	allowed, softWarn, remaining, _ := rl.Allow(tenantID, &hard, &soft)
	if !allowed {
		t.Fatal("60th request should be allowed")
	}
	if !softWarn {
		t.Fatal("60th request should be a soft warning")
	}
	if remaining != 60 {
		t.Fatalf("remaining=%d, want 60", remaining)
	}
}

func TestTenantRateLimiter_NilLimitsAlwaysAllow(t *testing.T) {
	t.Parallel()

	rl := NewTenantRateLimiter(time.Minute)
	t.Cleanup(func() { rl.Stop() })

	allowed, softWarn, remaining, retryAfter := rl.Allow("tenant-4", nil, nil)
	if !allowed {
		t.Fatal("nil limits should allow")
	}
	if softWarn {
		t.Fatal("nil limits should not soft warn")
	}
	if remaining != 0 {
		t.Fatalf("remaining=%d, want 0", remaining)
	}
	if retryAfter != 0 {
		t.Fatalf("retryAfter=%v, want 0", retryAfter)
	}
}

func TestTenantRateLimiter_ConcurrentAccessIsSafe(t *testing.T) {
	t.Parallel()

	rl := NewTenantRateLimiter(time.Minute)
	t.Cleanup(func() { rl.Stop() })

	done := make(chan struct{})
	tenantID := "tenant-5"
	for i := 0; i < 8; i++ {
		go func() {
			for j := 0; j < 200; j++ {
				allowed, _, _, _ := rl.Allow(tenantID, nil, nil)
				if !allowed {
					t.Error("nil limits should always allow")
					return
				}
			}
			done <- struct{}{}
		}()
	}

	for i := 0; i < 8; i++ {
		<-done
	}
}

func TestTenantRateLimiter_NonPositiveLimitsAreIgnored(t *testing.T) {
	t.Parallel()

	rl := NewTenantRateLimiter(time.Minute)
	t.Cleanup(func() { rl.Stop() })

	hardZero := 0
	softNegative := -1

	for i := 0; i < 100; i++ {
		allowed, softWarn, remaining, retryAfter := rl.Allow("tenant-nonpositive", &hardZero, &softNegative)
		if !allowed {
			t.Fatalf("request %d should be allowed when limits are non-positive", i+1)
		}
		if softWarn {
			t.Fatalf("request %d should not emit soft warning when limits are non-positive", i+1)
		}
		if remaining != 0 {
			t.Fatalf("request %d: remaining=%d, want 0", i+1, remaining)
		}
		if retryAfter != 0 {
			t.Fatalf("request %d: retryAfter=%v, want 0", i+1, retryAfter)
		}
	}

	rl.mu.Lock()
	if _, ok := rl.windows["tenant-nonpositive"]; ok {
		rl.mu.Unlock()
		t.Fatal("non-positive limits should not allocate tenant window state")
	}
	rl.mu.Unlock()
}

func TestTenantRateLimiter_StopIsIdempotent(t *testing.T) {
	t.Parallel()

	rl := NewTenantRateLimiter(time.Minute)
	rl.Stop()
	rl.Stop()
}
