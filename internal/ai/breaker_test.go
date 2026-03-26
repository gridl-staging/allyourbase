package ai

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestProviderHealthTracker_ClosedToOpenOnThreshold(t *testing.T) {
	now := time.Date(2026, 2, 28, 12, 0, 0, 0, time.UTC)
	tracker := NewProviderHealthTracker(BreakerConfig{
		FailureThreshold:    2,
		OpenDuration:        10 * time.Second,
		HalfOpenMaxRequests: 1,
	}, func() time.Time { return now })

	tracker.RecordFailure("openai")
	if st := tracker.State("openai"); st != BreakerStateClosed {
		t.Fatalf("state after 1st failure = %s; want %s", st, BreakerStateClosed)
	}

	tracker.RecordFailure("openai")
	if st := tracker.State("openai"); st != BreakerStateOpen {
		t.Fatalf("state after threshold = %s; want %s", st, BreakerStateOpen)
	}
}

func TestProviderHealthTracker_OpenShortCircuitAndHalfOpenRecovery(t *testing.T) {
	now := time.Date(2026, 2, 28, 12, 0, 0, 0, time.UTC)
	tracker := NewProviderHealthTracker(BreakerConfig{
		FailureThreshold:    1,
		OpenDuration:        5 * time.Second,
		HalfOpenMaxRequests: 2,
	}, func() time.Time { return now })

	tracker.RecordFailure("openai")
	if err := tracker.Allow("openai"); err == nil {
		t.Fatal("expected open breaker short-circuit")
	} else {
		var boe *BreakerOpenError
		if !errors.As(err, &boe) {
			t.Fatalf("expected BreakerOpenError, got %T", err)
		}
	}

	// Move time forward into half-open window and allow probes.
	now = now.Add(6 * time.Second)
	if err := tracker.Allow("openai"); err != nil {
		t.Fatalf("first half-open probe should be allowed: %v", err)
	}
	if st := tracker.State("openai"); st != BreakerStateHalfOpen {
		t.Fatalf("state = %s; want %s", st, BreakerStateHalfOpen)
	}

	if err := tracker.Allow("openai"); err != nil {
		t.Fatalf("second half-open probe should be allowed: %v", err)
	}

	if err := tracker.Allow("openai"); err == nil {
		t.Fatal("third half-open probe should be blocked")
	}

	// Probe success should close the breaker and reset counts.
	tracker.RecordSuccess("openai")
	if st := tracker.State("openai"); st != BreakerStateClosed {
		t.Fatalf("state after success = %s; want %s", st, BreakerStateClosed)
	}
	if err := tracker.Allow("openai"); err != nil {
		t.Fatalf("closed breaker should allow: %v", err)
	}
}

func TestProviderHealthTracker_HalfOpenFailureReopens(t *testing.T) {
	now := time.Date(2026, 2, 28, 12, 0, 0, 0, time.UTC)
	tracker := NewProviderHealthTracker(BreakerConfig{
		FailureThreshold:    1,
		OpenDuration:        5 * time.Second,
		HalfOpenMaxRequests: 1,
	}, func() time.Time { return now })

	tracker.RecordFailure("openai")
	now = now.Add(6 * time.Second)
	if err := tracker.Allow("openai"); err != nil {
		t.Fatalf("half-open probe should be allowed: %v", err)
	}
	tracker.RecordFailure("openai")
	if st := tracker.State("openai"); st != BreakerStateOpen {
		t.Fatalf("state after half-open failure = %s; want %s", st, BreakerStateOpen)
	}
}

func TestProviderHealthTracker_ProviderIsolation(t *testing.T) {
	now := time.Date(2026, 2, 28, 12, 0, 0, 0, time.UTC)
	tracker := NewProviderHealthTracker(BreakerConfig{
		FailureThreshold:    1,
		OpenDuration:        10 * time.Second,
		HalfOpenMaxRequests: 1,
	}, func() time.Time { return now })

	tracker.RecordFailure("openai")
	if err := tracker.Allow("anthropic"); err != nil {
		t.Fatalf("anthropic should remain healthy: %v", err)
	}
}

type breakerRetryMockProvider struct {
	calls int
	err   error
}

func (m *breakerRetryMockProvider) GenerateText(_ context.Context, _ GenerateTextRequest) (GenerateTextResponse, error) {
	m.calls++
	if m.err != nil {
		return GenerateTextResponse{}, m.err
	}
	return GenerateTextResponse{Text: "ok"}, nil
}

func TestRetryBreakerInteraction_OpenStateShortCircuitsAndNoDoubleCount(t *testing.T) {
	now := time.Date(2026, 2, 28, 12, 0, 0, 0, time.UTC)
	tracker := NewProviderHealthTracker(BreakerConfig{
		FailureThreshold:    1,
		OpenDuration:        30 * time.Second,
		HalfOpenMaxRequests: 1,
	}, func() time.Time { return now })

	inner := &breakerRetryMockProvider{
		err: &ProviderError{StatusCode: 500, Message: "boom", Provider: "openai"},
	}
	retried := NewRetryProvider(inner, 2, 0)
	wrapped := NewBreakerProvider(retried, "openai", tracker)

	_, err := wrapped.GenerateText(context.Background(), GenerateTextRequest{})
	if err == nil {
		t.Fatal("expected error from first call")
	}
	if inner.calls != 3 {
		t.Fatalf("retry inner calls = %d; want 3", inner.calls)
	}
	if st := tracker.State("openai"); st != BreakerStateOpen {
		t.Fatalf("breaker state = %s; want open", st)
	}

	_, err = wrapped.GenerateText(context.Background(), GenerateTextRequest{})
	if err == nil {
		t.Fatal("expected open short-circuit on second call")
	}
	if inner.calls != 3 {
		t.Fatalf("open short-circuit should not call provider; calls=%d", inner.calls)
	}
}
