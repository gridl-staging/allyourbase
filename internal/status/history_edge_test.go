package status

import (
	"testing"
	"time"
)

func TestNewStatusHistory_ZeroCapacity(t *testing.T) {
	// Capacity <= 0 should be clamped to 1 so the ring buffer always works.
	h := NewStatusHistory(0)
	if h.capacity != 1 {
		t.Fatalf("NewStatusHistory(0).capacity = %d, want 1", h.capacity)
	}
}

func TestNewStatusHistory_NegativeCapacity(t *testing.T) {
	h := NewStatusHistory(-5)
	if h.capacity != 1 {
		t.Fatalf("NewStatusHistory(-5).capacity = %d, want 1", h.capacity)
	}
}

func TestStatusHistory_Capacity1WrapAround(t *testing.T) {
	// With capacity 1, every Push overwrites the single slot.
	h := NewStatusHistory(1)
	base := time.Date(2026, 3, 28, 10, 0, 0, 0, time.UTC)

	s1 := StatusSnapshot{Status: Operational, CheckedAt: base}
	s2 := StatusSnapshot{Status: Degraded, CheckedAt: base.Add(time.Minute)}

	h.Push(s1)
	got := h.Latest()
	if got == nil || got.Status != Operational {
		t.Fatalf("after Push(s1): Latest().Status = %v, want %q", got, Operational)
	}

	// Second push replaces the only slot.
	h.Push(s2)
	got = h.Latest()
	if got == nil || got.Status != Degraded {
		t.Fatalf("after Push(s2): Latest().Status = %v, want %q", got, Degraded)
	}

	// Recent should return exactly 1 entry, the latest.
	recent := h.Recent(10)
	if len(recent) != 1 {
		t.Fatalf("Recent(10) len = %d, want 1", len(recent))
	}
	if recent[0].Status != Degraded {
		t.Fatalf("Recent(10)[0].Status = %q, want %q", recent[0].Status, Degraded)
	}
}

func TestStatusHistory_FullWrapAround(t *testing.T) {
	// Fill a capacity-3 buffer, then push 5 more entries.
	// This exercises the ring buffer wrapping past one full cycle.
	h := NewStatusHistory(3)
	base := time.Date(2026, 3, 28, 10, 0, 0, 0, time.UTC)

	// Push 8 entries total; only the last 3 should remain.
	for i := range 8 {
		h.Push(StatusSnapshot{
			Status:    Operational,
			CheckedAt: base.Add(time.Duration(i) * time.Minute),
		})
	}

	recent := h.Recent(10)
	if len(recent) != 3 {
		t.Fatalf("Recent(10) len = %d, want 3", len(recent))
	}

	// Oldest remaining should be entry 5 (index 5, minutes offset).
	if recent[0].CheckedAt != base.Add(5*time.Minute) {
		t.Errorf("oldest remaining CheckedAt = %v, want %v", recent[0].CheckedAt, base.Add(5*time.Minute))
	}
	// Newest should be entry 7.
	if recent[2].CheckedAt != base.Add(7*time.Minute) {
		t.Errorf("newest remaining CheckedAt = %v, want %v", recent[2].CheckedAt, base.Add(7*time.Minute))
	}
}

func TestStatusHistory_RecentNegative(t *testing.T) {
	h := NewStatusHistory(5)
	h.Push(StatusSnapshot{Status: Operational, CheckedAt: time.Now()})

	// Negative n should return empty, not panic.
	got := h.Recent(-1)
	if len(got) != 0 {
		t.Fatalf("Recent(-1) len = %d, want 0", len(got))
	}
}

func TestStatusHistory_NilSafety(t *testing.T) {
	// All methods on nil receiver should be safe — no panics.
	var h *StatusHistory

	// Push on nil should not panic.
	h.Push(StatusSnapshot{Status: Operational})

	if got := h.Latest(); got != nil {
		t.Fatalf("nil.Latest() = %+v, want nil", got)
	}
	if got := h.Recent(5); len(got) != 0 {
		t.Fatalf("nil.Recent(5) len = %d, want 0", len(got))
	}
}

func TestStatusHistory_CloneIsolation(t *testing.T) {
	// Mutations to the returned snapshot's Services slice must not affect history.
	h := NewStatusHistory(5)
	h.Push(StatusSnapshot{
		Status: Operational,
		Services: []ProbeResult{
			{Service: Database, Healthy: true},
			{Service: Storage, Healthy: true},
		},
	})

	got := h.Latest()
	if got == nil {
		t.Fatal("Latest() = nil, want snapshot")
	}

	// Mutate the returned slice.
	got.Services[0].Healthy = false

	// The copy in history should be unchanged.
	got2 := h.Latest()
	if !got2.Services[0].Healthy {
		t.Fatal("mutating returned snapshot should not affect history; got Healthy=false in history copy")
	}
}
