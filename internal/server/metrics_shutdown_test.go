package server

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/schema"
)

func TestServerShutdownCleansUpMetricsProvider(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(nil, logger)
	srv := newServer(cfg, logger, ch, nil, nil, nil, nil)

	if srv.httpMetrics == nil {
		t.Fatal("expected httpMetrics to be initialized when metrics enabled")
	}

	// Generate a request so metrics are populated.
	srv.router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/health", nil))

	// Verify metrics are present before shutdown.
	w := httptest.NewRecorder()
	srv.httpMetrics.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if !strings.Contains(w.Body.String(), "ayb_http_requests_total") {
		t.Fatal("expected metrics to contain ayb_http_requests_total before shutdown")
	}

	// Set a dummy http.Server so Shutdown doesn't nil-deref (Start() wasn't called).
	srv.http = &http.Server{}

	if err := srv.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown error: %v", err)
	}

	// After server shutdown, the OTel meter provider should have been shut down.
	// The Prometheus scrape handler should return an empty body (no metric families
	// collected) because the exporter reader was shut down.
	w2 := httptest.NewRecorder()
	srv.httpMetrics.Handler().ServeHTTP(w2, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if strings.Contains(w2.Body.String(), "ayb_http_requests_total") {
		t.Fatal("expected metrics to be empty after meter provider shutdown, but ayb_http_requests_total still present")
	}
}

func TestShutdownHTTPThenMetrics_OrderAndErrors(t *testing.T) {
	t.Parallel()

	t.Run("http first then metrics", func(t *testing.T) {
		t.Parallel()
		order := make([]string, 0, 2)
		err := shutdownHTTPThenMetrics(
			context.Background(),
			func(context.Context) error {
				order = append(order, "http")
				return nil
			},
			func(context.Context) error {
				order = append(order, "metrics")
				return nil
			},
		)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if len(order) != 2 || order[0] != "http" || order[1] != "metrics" {
			t.Fatalf("expected shutdown order [http metrics], got %v", order)
		}
	})

	t.Run("returns joined errors from both shutdown paths", func(t *testing.T) {
		t.Parallel()
		httpErr := errors.New("http shutdown failed")
		metricsErr := errors.New("metrics shutdown failed")
		err := shutdownHTTPThenMetrics(
			context.Background(),
			func(context.Context) error { return httpErr },
			func(context.Context) error { return metricsErr },
		)
		if !errors.Is(err, httpErr) {
			t.Fatalf("expected joined error to include http error: %v", err)
		}
		if !errors.Is(err, metricsErr) {
			t.Fatalf("expected joined error to include metrics error: %v", err)
		}
	})
}
