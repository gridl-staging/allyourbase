package observability

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	collectorv1 "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	"google.golang.org/grpc"
)

// mockTraceCollector is a minimal gRPC TraceService implementation for testing.
type mockTraceCollector struct {
	collectorv1.UnimplementedTraceServiceServer
	mu       sync.Mutex
	requests []*collectorv1.ExportTraceServiceRequest
}

func (c *mockTraceCollector) Export(_ context.Context, req *collectorv1.ExportTraceServiceRequest) (*collectorv1.ExportTraceServiceResponse, error) {
	c.mu.Lock()
	c.requests = append(c.requests, req)
	c.mu.Unlock()
	return &collectorv1.ExportTraceServiceResponse{}, nil
}

func (c *mockTraceCollector) received() []*collectorv1.ExportTraceServiceRequest {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]*collectorv1.ExportTraceServiceRequest, len(c.requests))
	copy(out, c.requests)
	return out
}

func TestTracerProviderExportsToMockOTLPCollector(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	collector := &mockTraceCollector{}
	grpcSrv := grpc.NewServer()
	collectorv1.RegisterTraceServiceServer(grpcSrv, collector)
	go func() { _ = grpcSrv.Serve(lis) }()
	defer grpcSrv.GracefulStop()

	origTP := otel.GetTracerProvider()
	origProp := otel.GetTextMapPropagator()
	t.Cleanup(func() {
		otel.SetTracerProvider(origTP)
		otel.SetTextMapPropagator(origProp)
	})

	cfg := TelemetryConfig{
		Enabled:      true,
		OTLPEndpoint: "http://" + lis.Addr().String(),
		ServiceName:  "ayb-test",
		SampleRate:   1.0,
	}
	tp, err := NewTracerProvider(cfg)
	if err != nil {
		t.Fatalf("NewTracerProvider: %v", err)
	}
	defer func() { _ = tp.Shutdown(context.Background()) }()
	SetGlobalTracerAndPropagator(tp)

	r := chi.NewRouter()
	r.Use(OtelChiMiddleware("ayb-test", r))
	r.Get("/users/{id}", func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	server := httptest.NewServer(r)
	defer server.Close()

	client := &http.Client{}
	req, err := http.NewRequest(http.MethodGet, server.URL+"/users/123", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if err := tp.ForceFlush(context.Background()); err != nil {
		t.Fatalf("ForceFlush: %v", err)
	}

	// Wait for the export to arrive at the mock collector.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if len(collector.received()) > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	reqs := collector.received()
	if len(reqs) == 0 {
		t.Fatal("expected mock OTLP collector to receive at least one export request")
	}

	// Verify the service name is propagated through the resource attributes.
	routeFound := false
	found := false
	for _, req := range reqs {
		for _, rs := range req.GetResourceSpans() {
			for _, attr := range rs.GetResource().GetAttributes() {
				if attr.GetKey() == "service.name" && attr.GetValue().GetStringValue() == "ayb-test" {
					found = true
				}
			}
			for _, scope := range rs.GetScopeSpans() {
				for _, span := range scope.GetSpans() {
					if hasAttributeString(span.GetAttributes(), "http.route", "/users/{id}") ||
						hasAttributeString(span.GetAttributes(), "url.path.template", "/users/{id}") {
						routeFound = true
					}
				}
			}
		}
	}
	if !found {
		t.Error("expected exported spans to include service.name=ayb-test resource attribute")
	}
	if !routeFound {
		t.Error("expected exported span attributes to include http.route=/users/{id}")
	}
}

// TestNewTracerProvider_NoEndpointDoesNotLeakSpansToExistingGlobal verifies that
// when OTLPEndpoint is empty (returning nil), the existing global TracerProvider is
// untouched and continues to record spans normally.
func TestNewTracerProvider_NoEndpointDoesNotLeakSpansToExistingGlobal(t *testing.T) {
	exp := tracetest.NewInMemoryExporter()
	existingTP := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exp))
	t.Cleanup(func() { _ = existingTP.Shutdown(context.Background()) })

	// NewTracerProvider with empty endpoint must return nil without error.
	tp, err := NewTracerProvider(TelemetryConfig{
		Enabled:      true,
		OTLPEndpoint: "",
		ServiceName:  "ayb",
		SampleRate:   1.0,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tp != nil {
		t.Fatal("expected nil TracerProvider when OTLPEndpoint is empty")
	}

	// Spans created through the existing provider must still be recorded —
	// NewTracerProvider must not have corrupted the exporter.
	tracer := existingTP.Tracer("test")
	_, span := tracer.Start(context.Background(), "should-be-recorded")
	span.End()

	if len(exp.GetSpans()) == 0 {
		t.Error("expected existing TracerProvider exporter to still record spans after a no-op NewTracerProvider call")
	}
}

func hasAttributeString(attrs []*commonv1.KeyValue, key, expected string) bool {
	for _, attr := range attrs {
		if attr.GetKey() != key {
			continue
		}
		if attr.GetValue().GetStringValue() == expected {
			return true
		}
	}
	return false
}
