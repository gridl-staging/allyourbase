package auth

import (
	"context"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/testutil"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// setTestTracerProvider installs an in-memory span exporter and returns the
// exporter plus a cleanup function that restores the previous global provider.
func setTestTracerProvider(t *testing.T) *tracetest.InMemoryExporter {
	t.Helper()
	exp := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exp))
	orig := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	t.Cleanup(func() {
		_ = tp.Shutdown(context.Background())
		otel.SetTracerProvider(orig)
	})
	return exp
}

func TestRegisterEmitsSpanOnEmailValidationError(t *testing.T) {
	exp := setTestTracerProvider(t)

	svc := NewService(nil, testSecret, time.Hour, time.Hour, 8, testutil.DiscardLogger())
	_, _, _, err := svc.Register(context.Background(), "not-an-email", "password123")
	testutil.ErrorContains(t, err, "invalid email")

	spans := exp.GetSpans()
	if len(spans) == 0 {
		t.Fatal("expected auth.register span to be emitted even on validation error")
	}
	found := false
	for _, s := range spans {
		if s.Name == "auth.register" {
			found = true
			if s.Status.Code != codes.Error {
				t.Errorf("expected span status Error, got %v", s.Status.Code)
			}
			break
		}
	}
	if !found {
		t.Errorf("no span named 'auth.register' found; got spans: %v", spanNames(spans))
	}
}

func TestRegisterEmitsSpanOnPasswordValidationError(t *testing.T) {
	exp := setTestTracerProvider(t)

	svc := NewService(nil, testSecret, time.Hour, time.Hour, 8, testutil.DiscardLogger())
	_, _, _, err := svc.Register(context.Background(), "user@example.com", "short")
	testutil.ErrorContains(t, err, "password")

	spans := exp.GetSpans()
	found := false
	for _, s := range spans {
		if s.Name == "auth.register" {
			found = true
			if s.Status.Code != codes.Error {
				t.Errorf("expected span status Error, got %v", s.Status.Code)
			}
			break
		}
	}
	if !found {
		t.Errorf("no span named 'auth.register' found; got spans: %v", spanNames(spans))
	}
}

func spanNames(spans tracetest.SpanStubs) []string {
	names := make([]string, len(spans))
	for i, s := range spans {
		names[i] = s.Name
	}
	return names
}
