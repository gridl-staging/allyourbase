package status

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakePinger struct {
	err error
}

func (f *fakePinger) Ping(ctx context.Context) error {
	return f.err
}

func TestDatabaseProbe_Check(t *testing.T) {
	t.Run("nil pool unhealthy", func(t *testing.T) {
		probe := NewDatabaseProbe(nil)
		res := probe.Check(context.Background())
		if res.Service != Database {
			t.Fatalf("service = %q, want %q", res.Service, Database)
		}
		if res.Healthy {
			t.Fatal("expected unhealthy for nil pool")
		}
		if res.Error == "" {
			t.Fatal("expected error message for nil pool")
		}
	})

	t.Run("healthy pinger", func(t *testing.T) {
		probe := &DatabaseProbe{pinger: &fakePinger{}}
		res := probe.Check(context.Background())
		if !res.Healthy {
			t.Fatalf("expected healthy result, got error=%q", res.Error)
		}
		if res.Service != Database {
			t.Fatalf("service = %q, want %q", res.Service, Database)
		}
		if res.CheckedAt.IsZero() {
			t.Fatal("expected checkedAt to be set")
		}
		if res.Latency < 0 {
			t.Fatalf("latency must be non-negative, got %s", res.Latency)
		}
	})

	t.Run("failing pinger unhealthy", func(t *testing.T) {
		probe := &DatabaseProbe{pinger: &fakePinger{err: errors.New("boom")}}
		res := probe.Check(context.Background())
		if res.Healthy {
			t.Fatal("expected unhealthy result")
		}
		if res.Error == "" {
			t.Fatal("expected error message")
		}
	})
}

func TestStubProbes(t *testing.T) {
	ctx := context.Background()
	probes := []Probe{
		NewStorageProbe(),
		NewAuthProbe(),
		NewRealtimeProbe(),
		NewFunctionsProbe(),
	}
	wantServices := []ServiceName{Storage, Auth, Realtime, Functions}

	for i, p := range probes {
		if p.Name() != wantServices[i] {
			t.Fatalf("Name() = %q, want %q", p.Name(), wantServices[i])
		}
		res := p.Check(ctx)
		if res.Service != wantServices[i] {
			t.Fatalf("service = %q, want %q", res.Service, wantServices[i])
		}
		if !res.Healthy {
			t.Fatalf("expected stub probe %q healthy", res.Service)
		}
		if res.CheckedAt.IsZero() {
			t.Fatalf("expected checkedAt for stub probe %q", res.Service)
		}
		if res.Latency > time.Second {
			t.Fatalf("unexpectedly high stub latency %s", res.Latency)
		}
	}
}
