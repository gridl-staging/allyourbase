package status

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

type staticProbe struct {
	name   ServiceName
	result ProbeResult
	calls  *atomic.Int64
}

func (p staticProbe) Name() ServiceName {
	return p.name
}

func (p staticProbe) Check(ctx context.Context) ProbeResult {
	_ = ctx
	if p.calls != nil {
		p.calls.Add(1)
	}
	out := p.result
	out.Service = p.name
	out.CheckedAt = time.Now().UTC()
	return out
}

func TestChecker_RunOncePopulatesHistory(t *testing.T) {
	h := NewStatusHistory(10)
	c := NewChecker([]Probe{
		staticProbe{name: Database, result: ProbeResult{Healthy: true, Latency: 25 * time.Millisecond}},
		staticProbe{name: Storage, result: ProbeResult{Healthy: false, Error: "storage unavailable"}},
	}, h, 50*time.Millisecond)

	snap := c.RunOnce(context.Background())
	if snap.Status != PartialOutage {
		t.Fatalf("RunOnce status = %q, want %q", snap.Status, PartialOutage)
	}
	if len(snap.Services) != 2 {
		t.Fatalf("RunOnce services len = %d, want 2", len(snap.Services))
	}
	if snap.CheckedAt.IsZero() {
		t.Fatal("RunOnce checkedAt must be set")
	}

	latest := h.Latest()
	if latest == nil {
		t.Fatal("history Latest() = nil, want snapshot")
	}
	if latest.Status != PartialOutage {
		t.Fatalf("history latest status = %q, want %q", latest.Status, PartialOutage)
	}
}

func TestChecker_StartStop(t *testing.T) {
	var calls atomic.Int64
	h := NewStatusHistory(10)
	c := NewChecker([]Probe{
		staticProbe{
			name:   Database,
			result: ProbeResult{Healthy: true},
			calls:  &calls,
		},
	}, h, 10*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c.Start(ctx)
	time.Sleep(45 * time.Millisecond)
	c.Stop()

	before := calls.Load()
	if before == 0 {
		t.Fatal("expected at least one probe call after Start")
	}

	// Ensure checker loop is stopped and does not continue probing.
	time.Sleep(30 * time.Millisecond)
	after := calls.Load()
	if after != before {
		t.Fatalf("probe calls changed after Stop: before=%d after=%d", before, after)
	}
}
