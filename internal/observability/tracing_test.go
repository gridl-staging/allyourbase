package observability

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace/noop"
)

func TestNewTracerProvider_NoEndpointReturnsNoop(t *testing.T) {
	cfg := TelemetryConfig{
		Enabled:      true,
		OTLPEndpoint: "",
		ServiceName:  "ayb",
		SampleRate:   1.0,
	}
	tp, err := NewTracerProvider(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tp != nil {
		t.Fatal("expected nil TracerProvider when OTLPEndpoint is empty")
	}
}

func TestNewTracerProvider_DisabledReturnsNil(t *testing.T) {
	cfg := TelemetryConfig{
		Enabled:      false,
		OTLPEndpoint: "http://localhost:4317",
		ServiceName:  "ayb",
		SampleRate:   1.0,
	}
	tp, err := NewTracerProvider(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tp != nil {
		t.Fatal("expected nil TracerProvider when Enabled=false")
	}
}

func TestNormalizeOTLPEndpoint(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		input            string
		wantEndpoint     string
		wantInsecureMode bool
	}{
		{
			name:             "http endpoint enables insecure transport",
			input:            "http://localhost:4317",
			wantEndpoint:     "localhost:4317",
			wantInsecureMode: true,
		},
		{
			name:             "https endpoint keeps secure transport",
			input:            "https://otel.example.com:4317",
			wantEndpoint:     "otel.example.com:4317",
			wantInsecureMode: false,
		},
		{
			name:             "bare endpoint defaults to secure transport",
			input:            "collector.internal:4317",
			wantEndpoint:     "collector.internal:4317",
			wantInsecureMode: false,
		},
		{
			name:             "trim surrounding whitespace",
			input:            "  http://127.0.0.1:4317  ",
			wantEndpoint:     "127.0.0.1:4317",
			wantInsecureMode: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gotEndpoint, gotInsecureMode := normalizeOTLPEndpoint(tt.input)
			if gotEndpoint != tt.wantEndpoint {
				t.Fatalf("endpoint mismatch: got %q want %q", gotEndpoint, tt.wantEndpoint)
			}
			if gotInsecureMode != tt.wantInsecureMode {
				t.Fatalf("insecure flag mismatch: got %t want %t", gotInsecureMode, tt.wantInsecureMode)
			}
		})
	}
}

func TestNewTracerProvider_SamplerIsParentBased(t *testing.T) {
	// Use an in-memory exporter to verify sampler behavior.
	exp := tracetest.NewInMemoryExporter()

	cfg := TelemetryConfig{
		Enabled:     true,
		ServiceName: "ayb",
		SampleRate:  1.0,
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSyncer(exp),
		sdktrace.WithSampler(buildSampler(cfg.SampleRate)),
	)
	defer func() { _ = tp.Shutdown(context.Background()) }()

	tracer := tp.Tracer("test")
	_, span := tracer.Start(context.Background(), "test-op")
	span.End()

	spans := exp.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	if spans[0].Name != "test-op" {
		t.Errorf("unexpected span name: %q", spans[0].Name)
	}
}

func TestSetGlobalTracerAndPropagator_InjectsTraceContext(t *testing.T) {
	// Restore globals after test.
	origTP := otel.GetTracerProvider()
	origProp := otel.GetTextMapPropagator()
	t.Cleanup(func() {
		otel.SetTracerProvider(origTP)
		otel.SetTextMapPropagator(origProp)
	})

	exp := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exp))
	SetGlobalTracerAndPropagator(tp)

	// Start a span and inject into carrier; verify traceparent present.
	ctx := context.Background()
	tracer := otel.Tracer("test")
	ctx, span := tracer.Start(ctx, "inject-test")

	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, carrier)
	span.End()

	if carrier["traceparent"] == "" {
		t.Fatal("expected traceparent to be injected into carrier")
	}
}

func TestOtelChiMiddleware_AddsTraceparentToResponse(t *testing.T) {
	// Restore globals after test.
	origTP := otel.GetTracerProvider()
	origProp := otel.GetTextMapPropagator()
	t.Cleanup(func() {
		otel.SetTracerProvider(origTP)
		otel.SetTextMapPropagator(origProp)
	})

	exp := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exp))
	SetGlobalTracerAndPropagator(tp)

	mux := http.NewServeMux()
	mux.HandleFunc("/test", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := OtelChiMiddleware("ayb", nil)(mux)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	traceparent := rr.Header().Get("Traceparent")
	if traceparent == "" {
		t.Fatal("expected Traceparent response header to be present")
	}
	parts := strings.Split(traceparent, "-")
	if len(parts) != 4 {
		t.Fatalf("expected Traceparent to have 4 parts, got %q", traceparent)
	}
	if parts[0] != "00" {
		t.Fatalf("expected Traceparent version 00, got %q", parts[0])
	}
	if len(parts[1]) != 32 {
		t.Fatalf("expected 32-char trace ID, got %q", parts[1])
	}
	if len(parts[2]) != 16 {
		t.Fatalf("expected 16-char span ID, got %q", parts[2])
	}
	if len(parts[3]) != 2 {
		t.Fatalf("expected 2-char flags, got %q", parts[3])
	}

	spans := exp.GetSpans()
	if len(spans) == 0 {
		t.Fatal("expected at least one span to be recorded")
	}
}

func TestOtelChiMiddleware_NilProviderIsNoop(t *testing.T) {
	origTP := otel.GetTracerProvider()
	origProp := otel.GetTextMapPropagator()
	t.Cleanup(func() {
		otel.SetTracerProvider(origTP)
		otel.SetTextMapPropagator(origProp)
	})
	otel.SetTracerProvider(noop.NewTracerProvider())

	// With a noop global provider, middleware should be transparent and avoid
	// emitting a traceparent response header.
	mux := http.NewServeMux()
	mux.HandleFunc("/test", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	handler := OtelChiMiddleware("ayb", nil)(mux)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rr.Code)
	}
	if got := rr.Header().Get("Traceparent"); got != "" {
		t.Fatalf("expected no Traceparent header with noop tracer provider, got %q", got)
	}
}

func TestNewOtelHTTPTransport_NilProviderReturnDefault(t *testing.T) {
	tr := NewOtelHTTPTransport(nil)
	if tr == nil {
		t.Fatal("expected non-nil transport")
	}
}

func TestNewOtelHTTPTransport_WithProvider(t *testing.T) {
	exp := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exp))
	tr := NewOtelHTTPTransport(tp)
	if tr == nil {
		t.Fatal("expected non-nil transport")
	}
}

func TestNewOtelHTTPTransport_UsesProvidedTracerProvider(t *testing.T) {
	origTP := otel.GetTracerProvider()
	origProp := otel.GetTextMapPropagator()
	origDefaultTransport := http.DefaultTransport
	t.Cleanup(func() {
		otel.SetTracerProvider(origTP)
		otel.SetTextMapPropagator(origProp)
		http.DefaultTransport = origDefaultTransport
	})
	otel.SetTracerProvider(noop.NewTracerProvider())
	otel.SetTextMapPropagator(propagation.TraceContext{})

	exp := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exp))
	t.Cleanup(func() {
		_ = tp.Shutdown(context.Background())
	})

	var capturedTraceparent string
	base := &http.Transport{}
	base.RegisterProtocol("stub", roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		capturedTraceparent = r.Header.Get("Traceparent")
		return &http.Response{
			StatusCode: http.StatusNoContent,
			Body:       io.NopCloser(strings.NewReader("")),
			Header:     make(http.Header),
			Request:    r,
		}, nil
	}))
	http.DefaultTransport = base

	client := &http.Client{Transport: NewOtelHTTPTransport(tp)}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "stub://collector/v1/traces", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("client request failed: %v", err)
	}
	_ = resp.Body.Close()

	if capturedTraceparent == "" {
		t.Fatal("expected outbound request to include Traceparent header")
	}
	if len(exp.GetSpans()) == 0 {
		t.Fatal("expected outbound span to be recorded using provided tracer provider")
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestTraceLogFields_ExtractsTraceAndSpanID(t *testing.T) {
	exp := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exp))

	tracer := tp.Tracer("test")
	ctx, span := tracer.Start(context.Background(), "log-corr")
	defer span.End()

	fields := TraceLogFields(ctx)
	if fields["trace_id"] == "" {
		t.Error("expected non-empty trace_id")
	}
	if fields["span_id"] == "" {
		t.Error("expected non-empty span_id")
	}
}

func TestTraceLogFields_NoSpanReturnsEmptyFields(t *testing.T) {
	fields := TraceLogFields(context.Background())
	if fields["trace_id"] != "" {
		t.Errorf("expected empty trace_id for no-span context, got %q", fields["trace_id"])
	}
	if fields["span_id"] != "" {
		t.Errorf("expected empty span_id for no-span context, got %q", fields["span_id"])
	}
}

func TestOtelChiMiddleware_SetsRouteMethodAndStatusAttributes(t *testing.T) {
	origTP := otel.GetTracerProvider()
	origProp := otel.GetTextMapPropagator()
	t.Cleanup(func() {
		otel.SetTracerProvider(origTP)
		otel.SetTextMapPropagator(origProp)
	})

	exp := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exp))
	SetGlobalTracerAndPropagator(tp)
	t.Cleanup(func() {
		_ = tp.Shutdown(context.Background())
	})

	r := chi.NewRouter()
	r.Use(OtelChiMiddleware("ayb", r))
	r.Get("/users/{id}", func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})

	req := httptest.NewRequest(http.MethodGet, "/users/123", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rr.Code)
	}

	spans := exp.GetSpans()
	if len(spans) == 0 {
		t.Fatal("expected at least one span to be recorded")
	}

	attrs := spans[len(spans)-1].Attributes
	route := findAttrString(attrs, "http.route", "url.path.template")
	if route != "/users/{id}" {
		t.Fatalf("expected route pattern /users/{id}, got %q", route)
	}

	method := findAttrString(attrs, "http.request.method", "http.method")
	if method != http.MethodGet {
		t.Fatalf("expected method %q, got %q", http.MethodGet, method)
	}

	status := findAttrInt(attrs, "http.response.status_code", "http.status_code")
	if status != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, status)
	}
}

func findAttrString(attrs []attribute.KeyValue, keys ...string) string {
	for _, key := range keys {
		for _, kv := range attrs {
			if string(kv.Key) == key && kv.Value.Type() == attribute.STRING {
				return kv.Value.AsString()
			}
		}
	}
	return ""
}

func findAttrInt(attrs []attribute.KeyValue, keys ...string) int {
	for _, key := range keys {
		for _, kv := range attrs {
			if string(kv.Key) != key {
				continue
			}
			switch kv.Value.Type() {
			case attribute.INT64:
				return int(kv.Value.AsInt64())
			}
		}
	}
	return 0
}
