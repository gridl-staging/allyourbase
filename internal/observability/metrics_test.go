package observability

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"go.opentelemetry.io/otel"
	otelmetric "go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

func TestMiddlewareDecrementsActiveConnectionsOnPanic(t *testing.T) {
	m, err := NewHTTPMetrics()
	if err != nil {
		t.Fatalf("NewHTTPMetrics error: %v", err)
	}

	panicHandler := m.Middleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	}))

	func() {
		defer func() { _ = recover() }()
		panicHandler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/panic", nil))
	}()

	w := httptest.NewRecorder()
	m.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 from metrics handler, got %d", w.Code)
	}

	matched, matchErr := regexp.MatchString(`(?m)^ayb_http_active_connections 0$`, w.Body.String())
	if matchErr != nil {
		t.Fatalf("regex error: %v", matchErr)
	}
	if !matched {
		t.Fatalf("expected ayb_http_active_connections to be 0 after panic, got metrics:\n%s", w.Body.String())
	}
}

func TestNewHTTPMetrics_DoesNotOverrideGlobalProviderOnCollectorError(t *testing.T) {
	orig := otel.GetMeterProvider()
	t.Cleanup(func() {
		otel.SetMeterProvider(orig)
	})

	sentinel := sdkmetric.NewMeterProvider()
	otel.SetMeterProvider(sentinel)

	_, err := NewHTTPMetrics(func(_ otelmetric.Meter) error {
		return errors.New("collector init failed")
	})
	if err == nil {
		t.Fatal("expected collector init error")
	}
	if otel.GetMeterProvider() != sentinel {
		t.Fatal("expected global meter provider to remain unchanged on init error")
	}
}
