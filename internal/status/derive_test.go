package status

import (
	"testing"
	"time"
)

func TestDeriveStatus(t *testing.T) {
	now := time.Date(2026, 3, 4, 12, 0, 0, 0, time.UTC)

	t.Run("all healthy operational", func(t *testing.T) {
		results := []ProbeResult{
			{Service: Database, Healthy: true, Latency: 100 * time.Millisecond, CheckedAt: now},
			{Service: Storage, Healthy: true, Latency: 120 * time.Millisecond, CheckedAt: now},
			{Service: Auth, Healthy: true, Latency: 140 * time.Millisecond, CheckedAt: now},
		}
		if got := DeriveStatus(results); got != Operational {
			t.Fatalf("DeriveStatus() = %q, want %q", got, Operational)
		}
	})

	t.Run("one unhealthy out of five partial outage", func(t *testing.T) {
		results := []ProbeResult{
			{Service: Database, Healthy: true, CheckedAt: now},
			{Service: Storage, Healthy: true, CheckedAt: now},
			{Service: Auth, Healthy: false, CheckedAt: now},
			{Service: Realtime, Healthy: true, CheckedAt: now},
			{Service: Functions, Healthy: true, CheckedAt: now},
		}
		if got := DeriveStatus(results); got != PartialOutage {
			t.Fatalf("DeriveStatus() = %q, want %q", got, PartialOutage)
		}
	})

	t.Run("three unhealthy major outage", func(t *testing.T) {
		results := []ProbeResult{
			{Service: Database, Healthy: false, CheckedAt: now},
			{Service: Storage, Healthy: false, CheckedAt: now},
			{Service: Auth, Healthy: true, CheckedAt: now},
			{Service: Realtime, Healthy: false, CheckedAt: now},
			{Service: Functions, Healthy: true, CheckedAt: now},
		}
		if got := DeriveStatus(results); got != MajorOutage {
			t.Fatalf("DeriveStatus() = %q, want %q", got, MajorOutage)
		}
	})

	t.Run("all healthy but slow degraded", func(t *testing.T) {
		results := []ProbeResult{
			{Service: Database, Healthy: true, Latency: SlowProbeThreshold + 10*time.Millisecond, CheckedAt: now},
			{Service: Storage, Healthy: true, Latency: 100 * time.Millisecond, CheckedAt: now},
		}
		if got := DeriveStatus(results); got != Degraded {
			t.Fatalf("DeriveStatus() = %q, want %q", got, Degraded)
		}
	})

	t.Run("empty safe default operational", func(t *testing.T) {
		if got := DeriveStatus(nil); got != Operational {
			t.Fatalf("DeriveStatus() = %q, want %q", got, Operational)
		}
	})
}
