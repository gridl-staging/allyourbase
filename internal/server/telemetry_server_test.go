package server

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/testutil"
	"go.opentelemetry.io/otel"
)

func TestNewServerTelemetryEndpointConfiguredDoesNotFailServerConstruction(t *testing.T) {
	origTP := otel.GetTracerProvider()
	origProp := otel.GetTextMapPropagator()
	t.Cleanup(func() {
		otel.SetTracerProvider(origTP)
		otel.SetTextMapPropagator(origProp)
	})

	cfg := config.Default()
	cfg.Telemetry.Enabled = true
	cfg.Telemetry.OTLPEndpoint = "localhost:4317"
	cfg.Telemetry.ServiceName = "ayb-test"
	cfg.Telemetry.SampleRate = 1.0

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(nil, logger)
	srv := newServer(cfg, logger, ch, nil, nil, nil, nil)
	t.Cleanup(func() {
		testutil.NoError(t, srv.Shutdown(context.Background()))
	})
	testutil.NotNil(t, srv.tracerProvider)
}

func TestNewServerTelemetryNoEndpointLeavesTracerProviderNil(t *testing.T) {
	cfg := config.Default()
	cfg.Telemetry.Enabled = true
	cfg.Telemetry.OTLPEndpoint = ""

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(nil, logger)
	srv := newServer(cfg, logger, ch, nil, nil, nil, nil)
	t.Cleanup(func() {
		testutil.NoError(t, srv.Shutdown(context.Background()))
	})

	testutil.True(t, srv.tracerProvider == nil, "tracer provider should remain nil without endpoint")
}
