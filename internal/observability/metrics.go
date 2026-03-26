package observability

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	promexporter "go.opentelemetry.io/otel/exporters/prometheus"
	otelmetric "go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// HTTPMetrics collects request metrics via OTel and exposes a Prometheus scrape endpoint.
type HTTPMetrics struct {
	meterProvider *sdkmetric.MeterProvider
	registry      *prometheus.Registry

	requestCounter otelmetric.Int64Counter
	durationHist   otelmetric.Float64Histogram
	activeConns    otelmetric.Int64UpDownCounter
}

type requestMetricsScope struct {
	recordedByTenant bool
}

type requestMetricsScopeKey struct{}

func requestMetricsScopeFromContext(ctx context.Context) *requestMetricsScope {
	if ctx == nil {
		return nil
	}
	scope, _ := ctx.Value(requestMetricsScopeKey{}).(*requestMetricsScope)
	return scope
}

// InfraCollector is a callback that registers observable instruments via OTel.
// It is called once during metrics initialization.
type InfraCollector func(meter otelmetric.Meter) error

// NewHTTPMetrics creates an OTel meter provider with Prometheus exporter and
// registers HTTP request instruments.
func NewHTTPMetrics(infraCollectors ...InfraCollector) (*HTTPMetrics, error) {
	registry := prometheus.NewRegistry()

	exporter, err := promexporter.New(
		promexporter.WithRegisterer(registry),
		promexporter.WithoutScopeInfo(),
		promexporter.WithoutTargetInfo(),
	)
	if err != nil {
		return nil, err
	}

	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(exporter))
	meter := provider.Meter("ayb")
	cleanupProvider := func(err error) (*HTTPMetrics, error) {
		_ = provider.Shutdown(context.Background())
		return nil, err
	}

	requestCounter, err := meter.Int64Counter("ayb_http_requests",
		otelmetric.WithDescription("Total number of HTTP requests"),
	)
	if err != nil {
		return cleanupProvider(err)
	}

	durationHist, err := meter.Float64Histogram("ayb_http_request_duration",
		otelmetric.WithDescription("HTTP request duration in seconds"),
		otelmetric.WithUnit("s"),
		otelmetric.WithExplicitBucketBoundaries(0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10),
	)
	if err != nil {
		return cleanupProvider(err)
	}

	activeConns, err := meter.Int64UpDownCounter("ayb_http_active_connections",
		otelmetric.WithDescription("Current number of active HTTP requests"),
	)
	if err != nil {
		return cleanupProvider(err)
	}

	for _, collector := range infraCollectors {
		if err := collector(meter); err != nil {
			return cleanupProvider(err)
		}
	}

	otel.SetMeterProvider(provider)

	return &HTTPMetrics{
		meterProvider:  provider,
		registry:       registry,
		requestCounter: requestCounter,
		durationHist:   durationHist,
		activeConns:    activeConns,
	}, nil
}

// Handler returns the Prometheus scrape endpoint handler.
func (m *HTTPMetrics) Handler() http.Handler {
	if m == nil {
		return http.NotFoundHandler()
	}
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}

// Middleware records request count, latency histogram, and active in-flight requests.
func (m *HTTPMetrics) Middleware(next http.Handler) http.Handler {
	if m == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ctx := r.Context()
		scope := &requestMetricsScope{}
		ctx = context.WithValue(ctx, requestMetricsScopeKey{}, scope)
		r = r.WithContext(ctx)

		m.activeConns.Add(ctx, 1)
		defer m.activeConns.Add(ctx, -1)

		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)

		status := ww.Status()
		if status == 0 {
			status = http.StatusOK
		}
		if scope.recordedByTenant {
			return
		}
		m.recordRequest(ctx, r, status, time.Since(start).Seconds())
	})
}

func (m *HTTPMetrics) recordRequest(ctx context.Context, r *http.Request, status int, duration float64, extra ...attribute.KeyValue) {
	if m == nil {
		return
	}
	statusStr := strconv.Itoa(status)
	attrs := []attribute.KeyValue{
		methodAttr(r.Method),
		routeAttr(routePattern(r)),
		statusAttr(statusStr),
	}
	attrs = append(attrs, extra...)
	options := otelmetric.WithAttributes(attrs...)
	m.requestCounter.Add(ctx, 1, options)
	m.durationHist.Record(ctx, duration, options)
}

// Shutdown gracefully shuts down the meter provider.
func (m *HTTPMetrics) Shutdown(ctx context.Context) error {
	if m == nil || m.meterProvider == nil {
		return nil
	}
	return m.meterProvider.Shutdown(ctx)
}

// MeterProvider returns the underlying OTel meter provider for use by other subsystems.
func (m *HTTPMetrics) MeterProvider() *sdkmetric.MeterProvider {
	if m == nil {
		return nil
	}
	return m.meterProvider
}

func routePattern(r *http.Request) string {
	rctx := chi.RouteContext(r.Context())
	if rctx == nil {
		return "unknown"
	}
	pattern := rctx.RoutePattern()
	if pattern == "" {
		return "unknown"
	}
	return pattern
}
