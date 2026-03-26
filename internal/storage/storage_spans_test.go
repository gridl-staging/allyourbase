package storage

import (
	"bytes"
	"context"
	"errors"
	"testing"

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

func spanNames(spans tracetest.SpanStubs) []string {
	names := make([]string, len(spans))
	for i, s := range spans {
		names[i] = s.Name
	}
	return names
}

func TestUploadEmitsSpanOnBucketValidationError(t *testing.T) {
	exp := setTestTracerProvider(t)

	svc := NewService(nil, nil, "", testutil.DiscardLogger(), 0)
	_, err := svc.Upload(context.Background(), "invalid bucket!", "file.txt", "text/plain", nil, bytes.NewReader([]byte("data")))
	if !errors.Is(err, ErrInvalidBucket) {
		t.Fatalf("expected ErrInvalidBucket, got %v", err)
	}

	spans := exp.GetSpans()
	found := false
	for _, s := range spans {
		if s.Name == "storage.upload" {
			found = true
			if s.Status.Code != codes.Error {
				t.Errorf("expected span status Error, got %v", s.Status.Code)
			}
			break
		}
	}
	if !found {
		t.Errorf("no span named 'storage.upload' found; got spans: %v", spanNames(spans))
	}
}

func TestUploadEmitsSpanOnNameValidationError(t *testing.T) {
	exp := setTestTracerProvider(t)

	svc := NewService(nil, nil, "", testutil.DiscardLogger(), 0)
	_, err := svc.Upload(context.Background(), "validbucket", "", "text/plain", nil, bytes.NewReader([]byte("data")))
	if !errors.Is(err, ErrInvalidName) {
		t.Fatalf("expected ErrInvalidName, got %v", err)
	}

	spans := exp.GetSpans()
	found := false
	for _, s := range spans {
		if s.Name == "storage.upload" {
			found = true
			if s.Status.Code != codes.Error {
				t.Errorf("expected span status Error, got %v", s.Status.Code)
			}
			break
		}
	}
	if !found {
		t.Errorf("no span named 'storage.upload' found; got spans: %v", spanNames(spans))
	}
}

func TestDeleteObjectEmitsSpanOnBucketValidationError(t *testing.T) {
	exp := setTestTracerProvider(t)

	svc := NewService(nil, nil, "", testutil.DiscardLogger(), 0)
	err := svc.DeleteObject(context.Background(), "invalid bucket!", "file.txt")
	if !errors.Is(err, ErrInvalidBucket) {
		t.Fatalf("expected ErrInvalidBucket, got %v", err)
	}

	spans := exp.GetSpans()
	found := false
	for _, s := range spans {
		if s.Name == "storage.delete" {
			found = true
			if s.Status.Code != codes.Error {
				t.Errorf("expected span status Error, got %v", s.Status.Code)
			}
			break
		}
	}
	if !found {
		t.Errorf("no span named 'storage.delete' found; got spans: %v", spanNames(spans))
	}
}
