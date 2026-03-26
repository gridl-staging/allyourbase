package observability

import (
	"context"
	"sync/atomic"

	"go.opentelemetry.io/otel/attribute"
	otelmetric "go.opentelemetry.io/otel/metric"
)

// PoolStatFunc returns DB pool statistics: total, idle, in-use, max connections.
// Typically wraps pgxpool.Pool.Stat().
type PoolStatFunc func() (total, idle, inUse, max int32)

// InfraMetrics registers infrastructure-level OTel instruments for DB pool gauges,
// auth counters, edge function invocations, and storage byte totals.
type InfraMetrics struct {
	poolStatFn   PoolStatFunc
	storageBytes atomic.Int64

	signupCounter   otelmetric.Int64Counter
	loginCounter    otelmetric.Int64Counter
	edgeFuncCounter otelmetric.Int64Counter
}

// NewInfraMetrics creates an InfraMetrics instance. poolStatFn may be nil
// (DB pool gauges will report 0).
func NewInfraMetrics(poolStatFn PoolStatFunc) *InfraMetrics {
	return &InfraMetrics{poolStatFn: poolStatFn}
}

// Collector returns an InfraCollector callback that registers all infrastructure
// instruments on the provided OTel meter. Pass the result to NewHTTPMetrics.
func (im *InfraMetrics) Collector() InfraCollector {
	return func(meter otelmetric.Meter) error {
		// DB pool observable gauges.
		poolTotal, err := meter.Int64ObservableGauge("ayb_db_pool_total",
			otelmetric.WithDescription("Total DB pool connections"))
		if err != nil {
			return err
		}
		poolIdle, err := meter.Int64ObservableGauge("ayb_db_pool_idle",
			otelmetric.WithDescription("Idle DB pool connections"))
		if err != nil {
			return err
		}
		poolInUse, err := meter.Int64ObservableGauge("ayb_db_pool_in_use",
			otelmetric.WithDescription("In-use DB pool connections"))
		if err != nil {
			return err
		}
		poolMax, err := meter.Int64ObservableGauge("ayb_db_pool_max",
			otelmetric.WithDescription("Max DB pool connections"))
		if err != nil {
			return err
		}

		_, err = meter.RegisterCallback(func(_ context.Context, o otelmetric.Observer) error {
			if im.poolStatFn != nil {
				total, idle, inUse, max := im.poolStatFn()
				o.ObserveInt64(poolTotal, int64(total))
				o.ObserveInt64(poolIdle, int64(idle))
				o.ObserveInt64(poolInUse, int64(inUse))
				o.ObserveInt64(poolMax, int64(max))
			} else {
				o.ObserveInt64(poolTotal, 0)
				o.ObserveInt64(poolIdle, 0)
				o.ObserveInt64(poolInUse, 0)
				o.ObserveInt64(poolMax, 0)
			}
			return nil
		}, poolTotal, poolIdle, poolInUse, poolMax)
		if err != nil {
			return err
		}

		// Storage bytes observable gauge.
		storageBytesGauge, err := meter.Int64ObservableGauge("ayb_storage_bytes_total",
			otelmetric.WithDescription("Total storage bytes used"))
		if err != nil {
			return err
		}
		_, err = meter.RegisterCallback(func(_ context.Context, o otelmetric.Observer) error {
			o.ObserveInt64(storageBytesGauge, im.storageBytes.Load())
			return nil
		}, storageBytesGauge)
		if err != nil {
			return err
		}

		// Auth counters.
		im.signupCounter, err = meter.Int64Counter("ayb_auth_signups_total",
			otelmetric.WithDescription("Total auth signups"))
		if err != nil {
			return err
		}
		im.loginCounter, err = meter.Int64Counter("ayb_auth_logins_total",
			otelmetric.WithDescription("Total auth logins"))
		if err != nil {
			return err
		}

		// Edge function invocation counter.
		im.edgeFuncCounter, err = meter.Int64Counter("ayb_edge_function_invocations_total",
			otelmetric.WithDescription("Total edge function invocations"))
		if err != nil {
			return err
		}

		return nil
	}
}

// SetStorageBytes updates the storage bytes gauge value atomically.
func (im *InfraMetrics) SetStorageBytes(bytes int64) {
	im.storageBytes.Store(bytes)
}

// RecordAuthSignup increments the auth signup counter.
func (im *InfraMetrics) RecordAuthSignup(ctx context.Context) {
	if im.signupCounter != nil {
		im.signupCounter.Add(ctx, 1)
	}
}

// RecordAuthLogin increments the auth login counter.
func (im *InfraMetrics) RecordAuthLogin(ctx context.Context) {
	if im.loginCounter != nil {
		im.loginCounter.Add(ctx, 1)
	}
}

// RecordEdgeFuncInvocation increments the edge function invocation counter
// with function name and status labels.
func (im *InfraMetrics) RecordEdgeFuncInvocation(ctx context.Context, function, status string) {
	if im.edgeFuncCounter != nil {
		im.edgeFuncCounter.Add(ctx, 1, otelmetric.WithAttributes(
			attribute.String("function", function),
			attribute.String("status", status),
		))
	}
}
