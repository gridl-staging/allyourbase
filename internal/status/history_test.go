package status

import (
	"sync"
	"testing"
	"time"
)

func TestStatusHistory_PushLatestRecent(t *testing.T) {
	h := NewStatusHistory(3)
	base := time.Date(2026, 3, 3, 10, 0, 0, 0, time.UTC)

	s1 := StatusSnapshot{Status: Operational, CheckedAt: base.Add(1 * time.Minute)}
	s2 := StatusSnapshot{Status: Degraded, CheckedAt: base.Add(2 * time.Minute)}
	s3 := StatusSnapshot{Status: PartialOutage, CheckedAt: base.Add(3 * time.Minute)}
	s4 := StatusSnapshot{Status: MajorOutage, CheckedAt: base.Add(4 * time.Minute)}

	h.Push(s1)
	h.Push(s2)
	h.Push(s3)

	if got := h.Latest(); got == nil || got.CheckedAt != s3.CheckedAt {
		t.Fatalf("Latest() checkedAt = %v, want %v", got, s3.CheckedAt)
	}

	recent2 := h.Recent(2)
	if len(recent2) != 2 {
		t.Fatalf("Recent(2) len = %d, want 2", len(recent2))
	}
	if recent2[0].CheckedAt != s2.CheckedAt || recent2[1].CheckedAt != s3.CheckedAt {
		t.Fatalf("Recent(2) order mismatch: got [%s, %s]", recent2[0].CheckedAt, recent2[1].CheckedAt)
	}

	// Exceed capacity: oldest (s1) should be evicted.
	h.Push(s4)
	recentAll := h.Recent(10)
	if len(recentAll) != 3 {
		t.Fatalf("Recent(10) len = %d, want 3", len(recentAll))
	}
	if recentAll[0].CheckedAt != s2.CheckedAt || recentAll[1].CheckedAt != s3.CheckedAt || recentAll[2].CheckedAt != s4.CheckedAt {
		t.Fatalf("eviction/order mismatch: got [%s, %s, %s]", recentAll[0].CheckedAt, recentAll[1].CheckedAt, recentAll[2].CheckedAt)
	}
}

func TestStatusHistory_Empty(t *testing.T) {
	h := NewStatusHistory(2)
	if got := h.Latest(); got != nil {
		t.Fatalf("Latest() = %+v, want nil", got)
	}
	if got := h.Recent(5); len(got) != 0 {
		t.Fatalf("Recent(5) len = %d, want 0", len(got))
	}
}

func TestStatusHistory_ConcurrentPushRead(t *testing.T) {
	h := NewStatusHistory(100)
	base := time.Date(2026, 3, 3, 10, 0, 0, 0, time.UTC)

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(offset int) {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				h.Push(StatusSnapshot{
					Status:    Operational,
					CheckedAt: base.Add(time.Duration(offset*200+j) * time.Millisecond),
				})
				_ = h.Latest()
				_ = h.Recent(5)
			}
		}(i)
	}
	wg.Wait()

	if got := len(h.Recent(200)); got > 100 {
		t.Fatalf("history size = %d, want <= 100", got)
	}
}
