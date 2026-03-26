package server

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/testutil"
)

func TestInfraMetricsNamesPresentWhenRecorded(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(nil, logger)
	srv := newServer(cfg, logger, ch, nil, nil, nil, nil)

	if srv.infraMetrics == nil {
		t.Fatal("expected infraMetrics to be initialized when metrics are enabled")
	}

	ctx := context.Background()
	srv.infraMetrics.RecordAuthSignup(ctx)
	srv.infraMetrics.RecordAuthLogin(ctx)
	srv.infraMetrics.RecordEdgeFuncInvocation(ctx, "health", "ok")

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	srv.httpMetrics.Handler().ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	testutil.Contains(t, body, "ayb_auth_signups_total")
	testutil.Contains(t, body, "ayb_auth_logins_total")
	testutil.Contains(t, body, "ayb_edge_function_invocations_total")
}

func TestAuthMetricsDoNotIncrementOnFailedAuthRequests(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.Auth.Enabled = true
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(nil, logger)
	srv := newServer(cfg, logger, ch, nil, &auth.Service{}, nil, nil)

	// Malformed JSON should fail before any auth service call.
	req := httptest.NewRequest(http.MethodPost, "/api/auth/register", strings.NewReader("{bad"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusBadRequest, w.Code)

	metrics := httptest.NewRecorder()
	metricsReq := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	srv.Router().ServeHTTP(metrics, metricsReq)
	testutil.Equal(t, http.StatusOK, metrics.Code)
	body := metrics.Body.String()

	// Request metric should exist for the failed auth request route.
	testutil.Contains(t, body, `route="/api/auth/register"`)

	// Auth counters should not show any positive increments on failure paths.
	testutil.True(t, !regexp.MustCompile(`(?m)^ayb_auth_signups_total [1-9]\d*$`).MatchString(body),
		"signup metric should not increment for failed auth requests")
	testutil.True(t, !regexp.MustCompile(`(?m)^ayb_auth_logins_total [1-9]\d*$`).MatchString(body),
		"login metric should not increment for failed auth requests")
}
