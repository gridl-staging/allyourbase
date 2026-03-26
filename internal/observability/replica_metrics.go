package observability

import (
	"context"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	otelmetric "go.opentelemetry.io/otel/metric"
)

// ReplicaStatEntry is a replica status snapshot used by observability callbacks.
type ReplicaStatEntry struct {
	URL      string
	State    string
	LagBytes int64
}

// ReplicaStatFunc returns the current replica status snapshots.
type ReplicaStatFunc func() []ReplicaStatEntry

// RoutingStatFunc returns cumulative read-routing counters:
// primary reads first, then replica reads.
type RoutingStatFunc func() (primaryReads, replicaReads uint64)

// ReplicaMetrics wires replica observability without importing replica package types.
type ReplicaMetrics struct {
	replicaStatFn ReplicaStatFunc
	routingStatFn RoutingStatFunc
}

// NewReplicaMetrics constructs a ReplicaMetrics collector adapter.
func NewReplicaMetrics(replicaStatFn ReplicaStatFunc, routingStatFn RoutingStatFunc) *ReplicaMetrics {
	return &ReplicaMetrics{
		replicaStatFn: replicaStatFn,
		routingStatFn: routingStatFn,
	}
}

// Collector registers replica lag/status gauges and routing counters.
func (rm *ReplicaMetrics) Collector() InfraCollector {
	return func(meter otelmetric.Meter) error {
		lagGauge, err := meter.Int64ObservableGauge("ayb_db_replica_lag_bytes",
			otelmetric.WithDescription("Replica WAL lag in bytes by replica URL"))
		if err != nil {
			return err
		}
		statusGauge, err := meter.Int64ObservableGauge("ayb_db_replica_status",
			otelmetric.WithDescription("Replica health state by replica URL (0=healthy,1=suspect,2=unhealthy)"))
		if err != nil {
			return err
		}
		_, err = meter.RegisterCallback(func(_ context.Context, o otelmetric.Observer) error {
			if rm.replicaStatFn == nil {
				return nil
			}
			for _, entry := range rm.replicaStatFn() {
				attrs := otelmetric.WithAttributes(attribute.String("replica", entry.URL))
				o.ObserveInt64(lagGauge, entry.LagBytes, attrs)
				o.ObserveInt64(statusGauge, stateToMetricValue(entry.State), attrs)
			}
			return nil
		}, lagGauge, statusGauge)
		if err != nil {
			return err
		}

		routedCounter, err := meter.Int64ObservableCounter("ayb_db_queries_routed_total",
			otelmetric.WithDescription("Total queries routed by read target (primary/replica)"))
		if err != nil {
			return err
		}
		_, err = meter.RegisterCallback(func(_ context.Context, o otelmetric.Observer) error {
			var primaryReads, replicaReads uint64
			if rm.routingStatFn != nil {
				primaryReads, replicaReads = rm.routingStatFn()
			}
			o.ObserveInt64(routedCounter, int64(primaryReads), otelmetric.WithAttributes(attribute.String("target", "primary")))
			o.ObserveInt64(routedCounter, int64(replicaReads), otelmetric.WithAttributes(attribute.String("target", "replica")))
			return nil
		}, routedCounter)
		if err != nil {
			return err
		}

		return nil
	}
}

func stateToMetricValue(state string) int64 {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "healthy":
		return 0
	case "suspect":
		return 1
	case "unhealthy":
		return 2
	default:
		return 2
	}
}
