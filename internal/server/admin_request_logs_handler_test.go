package server_test

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/server"
	"github.com/allyourbase/ayb/internal/testutil"
)

// TestAdminAnalyticsRequestsNilPoolReturns503 verifies 503 when no DB is configured.
func TestAdminAnalyticsRequestsNilPoolReturns503(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.Admin.Password = "testpass"
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(nil, logger)
	srv := server.New(cfg, logger, ch, nil, nil, nil)

	token := adminLogin(t, srv)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/analytics/requests", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	srv.Router().ServeHTTP(w, req)

	testutil.Equal(t, http.StatusServiceUnavailable, w.Code)
}

// TestAdminAnalyticsRequestsRequiresAdminAuth verifies unauthenticated requests are rejected.
func TestAdminAnalyticsRequestsRequiresAdminAuth(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.Admin.Password = "testpass"
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(nil, logger)
	srv := server.New(cfg, logger, ch, nil, nil, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/analytics/requests", nil)
	srv.Router().ServeHTTP(w, req)

	testutil.Equal(t, http.StatusUnauthorized, w.Code)
}

// TestAdminAnalyticsRequestsBadQueryParams verifies invalid query params return 400.
func TestAdminAnalyticsRequestsBadQueryParams(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.Admin.Password = "testpass"
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(nil, logger)
	srv := server.New(cfg, logger, ch, nil, nil, nil)
	token := adminLogin(t, srv)

	cases := []struct {
		name   string
		query  string
		status int
	}{
		{name: "invalid status", query: "status=abc", status: http.StatusBadRequest},
		{name: "invalid limit", query: "limit=bad", status: http.StatusBadRequest},
		{name: "invalid offset", query: "offset=bad", status: http.StatusBadRequest},
		{name: "invalid from", query: "from=notadate", status: http.StatusBadRequest},
		{name: "invalid to", query: "to=notadate", status: http.StatusBadRequest},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api/admin/analytics/requests?"+tc.query, nil)
			req.Header.Set("Authorization", "Bearer "+token)
			srv.Router().ServeHTTP(w, req)
			testutil.Equal(t, tc.status, w.Code)
		})
	}
}

func TestAdminAnalyticsRequestsToBeforeFromReturns400(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.Admin.Password = "testpass"
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(nil, logger)
	srv := server.New(cfg, logger, ch, nil, nil, nil)
	token := adminLogin(t, srv)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/analytics/requests?from=2026-03-02&to=2026-03-01", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	srv.Router().ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
}
