package observability

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/riandyrn/otelchi"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

// TelemetryConfig holds settings for OTel distributed tracing.
type TelemetryConfig struct {
	Enabled      bool
	OTLPEndpoint string
	ServiceName  string
	SampleRate   float64
}

// NewTracerProvider creates an OTel SDK TracerProvider configured for OTLP gRPC export.
// Returns nil (no error) when tracing is disabled or OTLPEndpoint is empty.
func NewTracerProvider(cfg TelemetryConfig) (*sdktrace.TracerProvider, error) {
	if !cfg.Enabled || cfg.OTLPEndpoint == "" {
		return nil, nil
	}

	endpoint, insecure := normalizeOTLPEndpoint(cfg.OTLPEndpoint)
	if endpoint == "" {
		return nil, fmt.Errorf("otlp endpoint is required")
	}

	ctx := context.Background()
	opts := []otlptracegrpc.Option{
		otlptracegrpc.WithEndpoint(endpoint),
	}
	if insecure {
		opts = append(opts, otlptracegrpc.WithInsecure())
	}
	exporter, err := otlptracegrpc.New(ctx, opts...)
	if err != nil {
		return nil, err
	}

	svcName := cfg.ServiceName
	if svcName == "" {
		svcName = "ayb"
	}

	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			"",
			semconv.ServiceName(svcName),
		),
	)
	if err != nil {
		return nil, err
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithSampler(buildSampler(cfg.SampleRate)),
		sdktrace.WithResource(res),
	)
	return tp, nil
}

// normalizeOTLPEndpoint converts user config into OTLP gRPC endpoint format and
// whether plaintext transport should be used.
//
// Rules:
// - "http://host:port"  => endpoint "host:port", insecure=true
// - "https://host:port" => endpoint "host:port", insecure=false
// - "host:port"         => endpoint "host:port", insecure=false (secure default)
func normalizeOTLPEndpoint(raw string) (endpoint string, insecure bool) {
	trimmed := strings.TrimSpace(raw)
	switch {
	case strings.HasPrefix(trimmed, "http://"):
		return strings.TrimSpace(strings.TrimPrefix(trimmed, "http://")), true
	case strings.HasPrefix(trimmed, "https://"):
		return strings.TrimSpace(strings.TrimPrefix(trimmed, "https://")), false
	default:
		return trimmed, false
	}
}

// buildSampler returns a ParentBased sampler wrapping TraceIDRatioBased.
// For rate == 1.0 it uses AlwaysSample as an optimization.
func buildSampler(rate float64) sdktrace.Sampler {
	var root sdktrace.Sampler
	if rate >= 1.0 {
		root = sdktrace.AlwaysSample()
	} else if rate <= 0.0 {
		root = sdktrace.NeverSample()
	} else {
		root = sdktrace.TraceIDRatioBased(rate)
	}
	return sdktrace.ParentBased(root)
}

// SetGlobalTracerAndPropagator registers the TracerProvider and W3C TraceContext
// propagator as OTel globals.
func SetGlobalTracerAndPropagator(tp *sdktrace.TracerProvider) {
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))
}

// OtelChiMiddleware returns chi-compatible middleware that creates a span per request.
// When routes is non-nil, chi route patterns are used for span naming.
func OtelChiMiddleware(serverName string, routes chi.Routes) func(http.Handler) http.Handler {
	var opts []otelchi.Option
	if routes != nil {
		opts = append(opts, otelchi.WithChiRoutes(routes))
	}
	otelMW := otelchi.Middleware(serverName, opts...)
	return func(next http.Handler) http.Handler {
		return otelMW(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sc := trace.SpanFromContext(r.Context()).SpanContext()
			if sc.IsValid() {
				w.Header().Set("Traceparent", formatTraceparent(sc))
			}
			next.ServeHTTP(w, r)
		}))
	}
}

// NewOtelHTTPTransport wraps http.DefaultTransport with OTel trace propagation.
// When tp is nil, returns http.DefaultTransport unchanged.
func NewOtelHTTPTransport(tp *sdktrace.TracerProvider) http.RoundTripper {
	if tp == nil {
		return http.DefaultTransport
	}
	return otelhttp.NewTransport(http.DefaultTransport, otelhttp.WithTracerProvider(tp))
}

// TraceLogFields extracts trace_id and span_id from the context's active span.
// Returns an empty map when no valid span is present.
func TraceLogFields(ctx context.Context) map[string]string {
	sc := trace.SpanFromContext(ctx).SpanContext()
	if !sc.IsValid() {
		return map[string]string{}
	}
	return map[string]string{
		"trace_id": sc.TraceID().String(),
		"span_id":  sc.SpanID().String(),
	}
}

// RecordSpanError records err on span and sets span status to Error.
// No-op when err is nil.
func RecordSpanError(span trace.Span, err error) {
	if err == nil {
		return
	}
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
}

func formatTraceparent(sc trace.SpanContext) string {
	return fmt.Sprintf("00-%s-%s-%02x", sc.TraceID().String(), sc.SpanID().String(), byte(sc.TraceFlags()))
}
