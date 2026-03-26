package tenant

import (
	"testing"
	"time"
)

func TestTenantBreakerTracker_ClosedToOpen(t *testing.T) {
	t.Parallel()

	tracker := NewTenantBreakerTracker(TenantBreakerConfig{
		FailureThreshold: 3,
		OpenDuration:     10 * time.Second,
	}, nil)

	tenantID := "test-tenant"

	for i := 0; i < 3; i++ {
		tracker.RecordFailure(tenantID)
	}

	state := tracker.State(tenantID)
	if state != BreakerStateOpen {
		t.Errorf("expected state open after 3 failures, got %s", state)
	}
}

func TestTenantBreakerTracker_OpenToHalfOpen(t *testing.T) {
	t.Parallel()

	now := time.Now()
	tracker := NewTenantBreakerTracker(TenantBreakerConfig{
		FailureThreshold: 2,
		OpenDuration:     50 * time.Millisecond,
	}, func() time.Time { return now })

	tenantID := "test-tenant"

	tracker.RecordFailure(tenantID)
	tracker.RecordFailure(tenantID)

	state := tracker.State(tenantID)
	if state != BreakerStateOpen {
		t.Fatalf("expected open, got %s", state)
	}

	now = now.Add(51 * time.Millisecond)

	state = tracker.State(tenantID)
	if state != BreakerStateHalfOpen {
		t.Errorf("expected half_open after duration elapses, got %s", state)
	}
}

func TestTenantBreakerTracker_HalfOpenToClosed(t *testing.T) {
	t.Parallel()

	now := time.Now()
	tracker := NewTenantBreakerTracker(TenantBreakerConfig{
		FailureThreshold:    2,
		OpenDuration:        10 * time.Second,
		HalfOpenMaxRequests: 1,
	}, func() time.Time { return now })

	tenantID := "test-tenant"

	tracker.RecordFailure(tenantID)
	tracker.RecordFailure(tenantID)

	state := tracker.State(tenantID)
	if state != BreakerStateOpen {
		t.Fatalf("expected open, got %s", state)
	}

	now = now.Add(11 * time.Second)

	err := tracker.Allow(tenantID)
	if err != nil {
		t.Fatalf("expected allow in half_open, got error: %v", err)
	}

	tracker.RecordSuccess(tenantID)

	state = tracker.State(tenantID)
	if state != BreakerStateClosed {
		t.Errorf("expected closed after success in half_open, got %s", state)
	}
}

func TestTenantBreakerTracker_HalfOpenToOpen(t *testing.T) {
	t.Parallel()

	now := time.Now()
	tracker := NewTenantBreakerTracker(TenantBreakerConfig{
		FailureThreshold:    2,
		OpenDuration:        10 * time.Second,
		HalfOpenMaxRequests: 1,
	}, func() time.Time { return now })

	tenantID := "test-tenant"

	tracker.RecordFailure(tenantID)
	tracker.RecordFailure(tenantID)

	state := tracker.State(tenantID)
	if state != BreakerStateOpen {
		t.Fatalf("expected open, got %s", state)
	}

	// Advance time past OpenDuration to transition to half_open.
	now = now.Add(11 * time.Second)

	err := tracker.Allow(tenantID)
	if err != nil {
		t.Fatalf("expected allow in half_open, got error: %v", err)
	}

	state = tracker.State(tenantID)
	if state != BreakerStateHalfOpen {
		t.Fatalf("expected half_open after cooldown, got %s", state)
	}

	// Failure in half_open should transition back to open.
	tracker.RecordFailure(tenantID)

	state = tracker.State(tenantID)
	if state != BreakerStateOpen {
		t.Errorf("expected open after failure in half_open, got %s", state)
	}
}

func TestTenantBreakerTracker_ResetBreaker(t *testing.T) {
	t.Parallel()

	tracker := NewTenantBreakerTracker(TenantBreakerConfig{
		FailureThreshold: 2,
		OpenDuration:     10 * time.Second,
	}, nil)

	tenantID := "test-tenant"

	tracker.RecordFailure(tenantID)
	tracker.RecordFailure(tenantID)

	state := tracker.State(tenantID)
	if state != BreakerStateOpen {
		t.Fatalf("expected open, got %s", state)
	}

	tracker.ResetBreaker(tenantID)

	state = tracker.State(tenantID)
	if state != BreakerStateClosed {
		t.Errorf("expected closed after reset, got %s", state)
	}
}

func TestTenantBreakerTracker_AllowInClosed(t *testing.T) {
	t.Parallel()

	tracker := NewTenantBreakerTracker(TenantBreakerConfig{
		FailureThreshold: 3,
		OpenDuration:     10 * time.Second,
	}, nil)

	tenantID := "test-tenant"

	err := tracker.Allow(tenantID)
	if err != nil {
		t.Errorf("expected allow in closed state, got error: %v", err)
	}
}

func TestTenantBreakerTracker_AllowInOpen(t *testing.T) {
	t.Parallel()

	tracker := NewTenantBreakerTracker(TenantBreakerConfig{
		FailureThreshold: 1,
		OpenDuration:     10 * time.Second,
	}, nil)

	tenantID := "test-tenant"

	tracker.RecordFailure(tenantID)

	err := tracker.Allow(tenantID)
	if err == nil {
		t.Error("expected error in open state")
	}
	breakerErr, ok := err.(*TenantBreakerOpenError)
	if !ok {
		t.Fatalf("expected TenantBreakerOpenError, got %T", err)
	}
	if breakerErr.TenantID != tenantID {
		t.Errorf("expected tenantID %s, got %s", tenantID, breakerErr.TenantID)
	}
}

func TestTenantBreakerTracker_RecordSuccessInClosed(t *testing.T) {
	t.Parallel()

	tracker := NewTenantBreakerTracker(TenantBreakerConfig{
		FailureThreshold: 3,
		OpenDuration:     10 * time.Second,
	}, nil)

	tenantID := "test-tenant"

	tracker.RecordSuccess(tenantID)

	state := tracker.State(tenantID)
	if state != BreakerStateClosed {
		t.Errorf("expected closed, got %s", state)
	}
}
