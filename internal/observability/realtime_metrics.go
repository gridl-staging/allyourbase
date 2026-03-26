package observability

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	otelmetric "go.opentelemetry.io/otel/metric"
)

// RealtimeMetricsAggregator provides observable gauge values for realtime connections and channels.
type RealtimeMetricsAggregator struct {
	connectionsSSE func() int
	connectionsWS  func() int
	broadcastChans func() int
	presenceChans  func() int
	broadcastTotal func() uint64
	presenceTotal  func() uint64
}

// NewRealtimeMetricsAggregator creates a new aggregator with function providers.
func NewRealtimeMetricsAggregator(
	connectionsSSE func() int,
	connectionsWS func() int,
	broadcastChans func() int,
	presenceChans func() int,
	broadcastTotal func() uint64,
	presenceTotal func() uint64,
) *RealtimeMetricsAggregator {
	return &RealtimeMetricsAggregator{
		connectionsSSE: connectionsSSE,
		connectionsWS:  connectionsWS,
		broadcastChans: broadcastChans,
		presenceChans:  presenceChans,
		broadcastTotal: broadcastTotal,
		presenceTotal:  presenceTotal,
	}
}

// Collector returns an InfraCollector callback that registers realtime OTel
// observable instruments.
func (rma *RealtimeMetricsAggregator) Collector() InfraCollector {
	return func(meter otelmetric.Meter) error {
		connections, err := meter.Int64ObservableGauge("ayb_realtime_connections_active",
			otelmetric.WithDescription("Active realtime connections"),
			otelmetric.WithUnit("connections"))
		if err != nil {
			return err
		}

		channels, err := meter.Int64ObservableGauge("ayb_realtime_channels_active",
			otelmetric.WithDescription("Active realtime channels"),
			otelmetric.WithUnit("channels"))
		if err != nil {
			return err
		}

		broadcastTotal, err := meter.Int64ObservableCounter("ayb_realtime_broadcast_messages_total",
			otelmetric.WithDescription("Total broadcast messages relayed"))
		if err != nil {
			return err
		}
		presenceTotal, err := meter.Int64ObservableCounter("ayb_realtime_presence_syncs_total",
			otelmetric.WithDescription("Total presence sync/diff messages sent"))
		if err != nil {
			return err
		}

		_, err = meter.RegisterCallback(func(_ context.Context, o otelmetric.Observer) error {
			if rma.connectionsSSE != nil {
				o.ObserveInt64(connections, int64(rma.connectionsSSE()), otelmetric.WithAttributes(attribute.String("transport", "sse")))
			}
			if rma.connectionsWS != nil {
				o.ObserveInt64(connections, int64(rma.connectionsWS()), otelmetric.WithAttributes(attribute.String("transport", "ws")))
			}
			if rma.broadcastChans != nil {
				o.ObserveInt64(channels, int64(rma.broadcastChans()), otelmetric.WithAttributes(attribute.String("type", "broadcast")))
			}
			if rma.presenceChans != nil {
				o.ObserveInt64(channels, int64(rma.presenceChans()), otelmetric.WithAttributes(attribute.String("type", "presence")))
			}
			if rma.broadcastTotal != nil {
				o.ObserveInt64(broadcastTotal, int64(rma.broadcastTotal()))
			}
			if rma.presenceTotal != nil {
				o.ObserveInt64(presenceTotal, int64(rma.presenceTotal()))
			}

			return nil
		}, connections, channels, broadcastTotal, presenceTotal)

		return err
	}
}
