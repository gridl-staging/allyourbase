package server_test

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/server"
	"github.com/allyourbase/ayb/internal/testutil"
)

func newTestServerWithConfig(t *testing.T, cfg *config.Config) *server.Server {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(nil, logger)
	return server.New(cfg, logger, ch, nil, nil, nil)
}

func TestMetricsEndpointEnabledExposesPrometheusMetrics(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	srv := newTestServerWithConfig(t, cfg)

	// Generate at least one instrumented request before scraping.
	wHealth := httptest.NewRecorder()
	reqHealth := httptest.NewRequest(http.MethodGet, "/health", nil)
	srv.Router().ServeHTTP(wHealth, reqHealth)
	testutil.Equal(t, http.StatusOK, wHealth.Code)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	srv.Router().ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)
	testutil.Contains(t, w.Header().Get("Content-Type"), "text/plain")
	body := w.Body.String()
	testutil.Contains(t, body, "ayb_http_requests_total")
	testutil.Contains(t, body, "ayb_http_request_duration_seconds_bucket")
	testutil.Contains(t, body, `route="/health"`)
	testutil.Contains(t, body, `le="+Inf"`)
}

func TestMetricsEndpointAuthTokenEnforced(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.Metrics.AuthToken = "metrics-secret"
	srv := newTestServerWithConfig(t, cfg)

	for _, tc := range []struct {
		name   string
		auth   string
		status int
	}{
		{name: "missing bearer token", auth: "", status: http.StatusUnauthorized},
		{name: "wrong bearer token", auth: "Bearer nope", status: http.StatusUnauthorized},
		{name: "lowercase bearer scheme", auth: "bearer metrics-secret", status: http.StatusOK},
		{name: "correct bearer token", auth: "Bearer metrics-secret", status: http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
			if tc.auth != "" {
				req.Header.Set("Authorization", tc.auth)
			}
			srv.Router().ServeHTTP(w, req)
			testutil.Equal(t, tc.status, w.Code)
		})
	}
}

func TestMetricsEndpointDisabledReturnsNotFound(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.Metrics.Enabled = false
	srv := newTestServerWithConfig(t, cfg)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	srv.Router().ServeHTTP(w, req)

	testutil.Equal(t, http.StatusNotFound, w.Code)
}

func TestMetricsEndpointRespectsCustomPath(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.Metrics.Path = "/internal-metrics"
	srv := newTestServerWithConfig(t, cfg)

	wDefault := httptest.NewRecorder()
	reqDefault := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	srv.Router().ServeHTTP(wDefault, reqDefault)
	testutil.Equal(t, http.StatusNotFound, wDefault.Code)

	wCustom := httptest.NewRecorder()
	reqCustom := httptest.NewRequest(http.MethodGet, "/internal-metrics", nil)
	srv.Router().ServeHTTP(wCustom, reqCustom)
	testutil.Equal(t, http.StatusOK, wCustom.Code)
	testutil.Contains(t, wCustom.Body.String(), "ayb_http_requests_total")
}

func TestMetricsInfraGaugeNamesPresent(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	srv := newTestServerWithConfig(t, cfg)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	srv.Router().ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	testutil.Contains(t, body, "ayb_db_pool_total")
	testutil.Contains(t, body, "ayb_db_pool_idle")
	testutil.Contains(t, body, "ayb_db_pool_in_use")
	testutil.Contains(t, body, "ayb_db_pool_max")
	testutil.Contains(t, body, "ayb_storage_bytes_total")
}

func TestMetricsEndpointIncludesRealtimeMetrics(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	srv := newTestServerWithConfig(t, cfg)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	srv.Router().ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	for _, want := range []string{
		`ayb_realtime_connections_active{transport="sse"}`,
		`ayb_realtime_connections_active{transport="ws"}`,
		`ayb_realtime_channels_active{type="broadcast"}`,
		`ayb_realtime_channels_active{type="presence"}`,
		"ayb_realtime_broadcast_messages_total",
		"ayb_realtime_presence_syncs_total",
	} {
		testutil.Contains(t, body, want)
	}
}

func TestMetricsMiddlewareUsesRoutePatternsNotRawPaths(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	srv := newTestServerWithConfig(t, cfg)

	// Hit two concrete paths that should map to the same route pattern label.
	for _, p := range []string{"/functions/v1/foo", "/functions/v1/bar"} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, p, nil)
		srv.Router().ServeHTTP(w, req)
		testutil.Equal(t, http.StatusServiceUnavailable, w.Code)
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)

	body := w.Body.String()
	testutil.Contains(t, body, `route="/functions/v1/{name}"`)
	testutil.True(t, !strings.Contains(body, `route="/functions/v1/foo"`), "raw path label leaked into metrics")
	testutil.True(t, !strings.Contains(body, `route="/functions/v1/bar"`), "raw path label leaked into metrics")
}

func TestMetricsEndpointExposesConfiguredHistogramBucketBoundaries(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	srv := newTestServerWithConfig(t, cfg)

	wHealth := httptest.NewRecorder()
	reqHealth := httptest.NewRequest(http.MethodGet, "/health", nil)
	srv.Router().ServeHTTP(wHealth, reqHealth)
	testutil.Equal(t, http.StatusOK, wHealth.Code)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)

	body := w.Body.String()
	for _, bound := range []string{
		"0.005",
		"0.01",
		"0.025",
		"0.05",
		"0.1",
		"0.25",
		"0.5",
		"1",
		"2.5",
		"5",
		"10",
		"+Inf",
	} {
		pat := `(?m)^ayb_http_request_duration_seconds_bucket\{[^}]*le="` + regexp.QuoteMeta(bound) + `"[^}]*\}\s+\d+(\.\d+)?$`
		testutil.True(t, regexp.MustCompile(pat).MatchString(body), "missing histogram bucket boundary %q in metrics output", bound)
	}
}
