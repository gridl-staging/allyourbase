//go:build integration

package server_test

import (
	"context"
	"encoding/json"
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

func TestAdminAnalyticsQueriesExtensionMissingReturns503(t *testing.T) {
	ctx := context.Background()
	ensureIntegrationMigrations(t, ctx)

	_, err := sharedPG.Pool.Exec(ctx, `DROP EXTENSION IF EXISTS pg_stat_statements`)
	testutil.NoError(t, err)

	cfg := config.Default()
	cfg.Admin.Password = "testpass"
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	testutil.NoError(t, ch.Load(ctx))
	srv := server.New(cfg, logger, ch, sharedPG.Pool, nil, nil)
	token := adminLogin(t, srv)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/analytics/queries", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	srv.Router().ServeHTTP(w, req)

	testutil.Equal(t, http.StatusServiceUnavailable, w.Code)
	testutil.Contains(t, w.Body.String(), "pg_stat_statements extension not enabled")
}

func TestAdminAnalyticsQueriesExtensionPresentReturnsSortedStats(t *testing.T) {
	ctx := context.Background()
	ensureIntegrationMigrations(t, ctx)

	if _, err := sharedPG.Pool.Exec(ctx, `CREATE EXTENSION IF NOT EXISTS pg_stat_statements`); err != nil {
		t.Skipf("pg_stat_statements create extension unavailable: %v", err)
	}
	if _, err := sharedPG.Pool.Exec(ctx, `SELECT pg_stat_statements_reset()`); err != nil {
		t.Skipf("pg_stat_statements reset unavailable: %v", err)
	}

	for range 20 {
		if _, err := sharedPG.Pool.Exec(ctx, `SELECT 1`); err != nil {
			t.Fatalf("seed select 1: %v", err)
		}
	}
	for range 3 {
		if _, err := sharedPG.Pool.Exec(ctx, `SELECT generate_series(1, 2000)`); err != nil {
			t.Fatalf("seed generate_series: %v", err)
		}
	}

	cfg := config.Default()
	cfg.Admin.Password = "testpass"
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	testutil.NoError(t, ch.Load(ctx))
	srv := server.New(cfg, logger, ch, sharedPG.Pool, nil, nil)
	token := adminLogin(t, srv)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/analytics/queries?sort=calls&limit=5", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Items []struct {
			QueryID       string  `json:"queryid"`
			Query         string  `json:"query"`
			Calls         int64   `json:"calls"`
			TotalExecTime float64 `json:"total_exec_time"`
			MeanExecTime  float64 `json:"mean_exec_time"`
			Rows          int64   `json:"rows"`
		} `json:"items"`
		Count int `json:"count"`
	}
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	testutil.True(t, len(resp.Items) > 0, "expected at least one pg_stat_statements row")
	testutil.True(t, resp.Items[0].QueryID != "", "queryid should be present")
	testutil.True(t, resp.Items[0].Query != "", "query should be present")
	testutil.True(t, resp.Items[0].Calls > 0, "calls should be > 0")
	testutil.True(t, resp.Items[0].TotalExecTime >= 0, "total_exec_time should be present")
	testutil.True(t, resp.Items[0].MeanExecTime >= 0, "mean_exec_time should be present")
	testutil.True(t, resp.Items[0].Rows >= 0, "rows should be present")
	if len(resp.Items) > 1 {
		testutil.True(t, resp.Items[0].Calls >= resp.Items[1].Calls, "items should be sorted by calls desc")
	}
}
