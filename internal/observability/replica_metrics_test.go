package observability

import (
	"strings"
	"testing"
)

func TestReplicaMetrics_GaugesReportLagAndState(t *testing.T) {
	rm := NewReplicaMetrics(
		func() []ReplicaStatEntry {
			return []ReplicaStatEntry{
				{URL: "postgres://replica-1", State: "healthy", LagBytes: 128},
				{URL: "postgres://replica-2", State: "suspect", LagBytes: 4096},
				{URL: "postgres://replica-3", State: "unhealthy", LagBytes: 0},
			}
		},
		func() (primaryReads, replicaReads uint64) { return 0, 0 },
	)

	m, err := NewHTTPMetrics(rm.Collector())
	if err != nil {
		t.Fatalf("NewHTTPMetrics: %v", err)
	}

	body := scrapeMetrics(t, m)
	for _, want := range []string{
		`ayb_db_replica_lag_bytes{replica="postgres://replica-1"} 128`,
		`ayb_db_replica_lag_bytes{replica="postgres://replica-2"} 4096`,
		`ayb_db_replica_status{replica="postgres://replica-1"} 0`,
		`ayb_db_replica_status{replica="postgres://replica-2"} 1`,
		`ayb_db_replica_status{replica="postgres://replica-3"} 2`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected %q in metrics output:\n%s", want, body)
		}
	}
}

func TestReplicaMetrics_ObservableCounterReportsRoutingTargets(t *testing.T) {
	rm := NewReplicaMetrics(
		func() []ReplicaStatEntry { return nil },
		func() (primaryReads, replicaReads uint64) { return 12, 34 },
	)

	m, err := NewHTTPMetrics(rm.Collector())
	if err != nil {
		t.Fatalf("NewHTTPMetrics: %v", err)
	}

	body := scrapeMetrics(t, m)
	for _, want := range []string{
		`ayb_db_queries_routed_total{target="primary"} 12`,
		`ayb_db_queries_routed_total{target="replica"} 34`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected %q in metrics output:\n%s", want, body)
		}
	}
}
