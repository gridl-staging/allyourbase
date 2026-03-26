//go:build integration

package observability_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/allyourbase/ayb/internal/observability"
	"github.com/exaring/otelpgx"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// TestPgxDBChildSpansCreated verifies that when a pgxpool is configured with the
// OTel pgx tracer, queries executed within a traced context produce child spans
// with db.system=postgresql and are parented to the active span.
func TestPgxDBChildSpansCreated(t *testing.T) {
	connStr := os.Getenv("TEST_DATABASE_URL")
	if connStr == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping DB child spans integration test")
	}

	exp := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exp))
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })
	origTP := otel.GetTracerProvider()
	origProp := otel.GetTextMapPropagator()
	t.Cleanup(func() {
		otel.SetTracerProvider(origTP)
		otel.SetTextMapPropagator(origProp)
	})
	observability.SetGlobalTracerAndPropagator(tp)

	cfg, err := pgxpool.ParseConfig(connStr)
	if err != nil {
		t.Fatalf("parse pool config: %v", err)
	}
	cfg.ConnConfig.Tracer = otelpgx.NewTracer(otelpgx.WithTracerProvider(tp))

	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatalf("new pool: %v", err)
	}
	t.Cleanup(pool.Close)

	r := chi.NewRouter()
	r.Use(observability.OtelChiMiddleware("ayb", r))
	r.Get("/users/{id}", func(w http.ResponseWriter, req *http.Request) {
		var result int
		if err := pool.QueryRow(req.Context(), "SELECT 1").Scan(&result); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if result != 1 {
			http.Error(w, "unexpected result", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	srv := httptest.NewServer(r)
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/users/123")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}

	spans := exp.GetSpans()
	if len(spans) < 2 {
		t.Fatalf("expected at least 2 spans (parent + DB child), got %d: %v", len(spans), spans)
	}

	// Find the HTTP request span and the DB query span.
	var dbSpan *tracetest.SpanStub
	var parentSpan *tracetest.SpanStub
	for i := range spans {
		attrs := spans[i].Attributes
		if hasStringAttribute(attrs, "db.system", "postgresql") {
			dbSpan = &spans[i]
			continue
		}
		if hasStringAttribute(attrs, "http.route", "/users/{id}") || hasStringAttribute(attrs, "url.path.template", "/users/{id}") {
			parentSpan = &spans[i]
		}
	}
	if dbSpan == nil {
		t.Fatalf("expected a DB child span among recorded spans: %v", spans)
	}
	if parentSpan == nil {
		t.Fatalf("expected an HTTP parent span in recorded spans: %v", spans)
	}

	// The DB span must be parented to the HTTP span.
	if !dbSpan.Parent.IsValid() {
		t.Error("expected DB span to have a valid parent span context")
	}
	if dbSpan.Parent.SpanID() != parentSpan.SpanContext.SpanID() {
		t.Errorf("expected DB span parent to be %v, got %v",
			parentSpan.SpanContext.SpanID(), dbSpan.Parent.SpanID())
	}

	// Verify db.system=postgresql attribute.
	if !hasStringAttribute(dbSpan.Attributes, "db.system", "postgresql") {
		t.Fatalf("expected db.system=postgresql in DB span; got %v", dbSpan.Attributes)
	}
	if !hasStringAttribute(dbSpan.Attributes, "db.query.text", "SELECT 1") {
		t.Fatalf("expected db.query.text=SELECT 1 in DB span; got %v", dbSpan.Attributes)
	}
}

func hasStringAttribute(attrs []attribute.KeyValue, key, expected string) bool {
	for _, attr := range attrs {
		if string(attr.Key) != key {
			continue
		}
		if attr.Value.Type() == attribute.STRING && attr.Value.AsString() == expected {
			return true
		}
	}
	return false
}
