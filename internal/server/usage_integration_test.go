//go:build integration

package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/billing"
	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/server"
	"github.com/allyourbase/ayb/internal/tenant"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/golang-jwt/jwt/v5"
)

func usageAdminLogin(t *testing.T, ts *httptest.Server, password string) string {
	t.Helper()

	resp, err := http.Post(ts.URL+"/api/admin/auth", "application/json", strings.NewReader(`{"password":"`+password+`"}`))
	testutil.NoError(t, err)
	defer resp.Body.Close()
	testutil.StatusCode(t, http.StatusOK, resp.StatusCode)

	var out map[string]string
	testutil.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
	token := out["token"]
	if token == "" {
		t.Fatal("admin auth returned empty token")
	}
	return token
}

func issueTenantJWT(t *testing.T, secret, userID, tenantID string) string {
	t.Helper()
	now := time.Now().UTC()
	claims := &auth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(1 * time.Hour)),
		},
		Email:    "user@example.com",
		TenantID: tenantID,
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString([]byte(secret))
	testutil.NoError(t, err)
	return signed
}

func setupUsageIntegrationServer(t *testing.T) (context.Context, *httptest.Server, string) {
	t.Helper()

	ctx := context.Background()
	_, err := sharedPG.Pool.Exec(ctx, "DROP SCHEMA public CASCADE; CREATE SCHEMA public")
	testutil.NoError(t, err)
	ensureIntegrationMigrations(t, ctx)

	cfg := config.Default()
	cfg.Admin.Password = "test-admin-pass"
	authSecret := "this-is-a-secret-that-is-at-least-32-characters-long"
	authSvc := auth.NewService(sharedPG.Pool, authSecret, time.Hour, 7*24*time.Hour, 8, testutil.DiscardLogger())

	srv := server.New(cfg, testutil.DiscardLogger(), nil, sharedPG.Pool, authSvc, nil)
	ts := httptest.NewServer(srv.Router())
	t.Cleanup(ts.Close)

	return ctx, ts, authSecret
}

func seedTenantUsageAndPlan(t *testing.T, ctx context.Context, name, slug string, plan billing.Plan, rows []tenant.TenantUsageDaily) string {
	t.Helper()

	tenantSvc := tenant.NewService(sharedPG.Pool, testutil.DiscardLogger())
	ten, err := tenantSvc.CreateTenant(ctx, name, slug, "schema", "free", "default", nil, "")
	testutil.NoError(t, err)

	for _, row := range rows {
		_, err := sharedPG.Pool.Exec(ctx,
			`INSERT INTO _ayb_tenant_usage_daily
				(tenant_id, date, request_count, db_bytes_used, bandwidth_bytes, function_invocations, realtime_peak_connections, job_runs)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			ten.ID,
			row.Date.UTC(),
			row.RequestCount,
			row.DBBytesUsed,
			row.BandwidthBytes,
			row.FunctionInvocations,
			row.RealtimePeakConnections,
			row.JobRuns,
		)
		testutil.NoError(t, err)
	}

	store := billing.NewStore(sharedPG.Pool)
	_, err = store.Create(ctx, ten.ID)
	testutil.NoError(t, err)
	err = store.UpdatePlanAndPayment(ctx, ten.ID, plan, billing.PaymentStatusActive)
	testutil.NoError(t, err)

	return ten.ID
}

func TestUsageIntegration_AdminUsageWeekSummary(t *testing.T) {
	ctx, ts, _ := setupUsageIntegrationServer(t)

	now := time.Now().UTC().Truncate(24 * time.Hour)
	tenantID := seedTenantUsageAndPlan(t, ctx, "Admin Usage Tenant", "admin-usage-tenant-"+strconv.FormatInt(time.Now().UnixNano(), 10), billing.PlanPro,
		[]tenant.TenantUsageDaily{
			{Date: now.AddDate(0, 0, -2), RequestCount: 100, DBBytesUsed: 1024, BandwidthBytes: 2048, FunctionInvocations: 300},
			{Date: now.AddDate(0, 0, -1), RequestCount: 200, DBBytesUsed: 4096, BandwidthBytes: 1024, FunctionInvocations: 400},
			{Date: now, RequestCount: 300, DBBytesUsed: 2048, BandwidthBytes: 512, FunctionInvocations: 500},
		},
	)

	adminToken := usageAdminLogin(t, ts, "test-admin-pass")
	req, err := http.NewRequest(http.MethodGet, ts.URL+"/api/admin/usage/"+tenantID+"?period=week", nil)
	testutil.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+adminToken)

	resp, err := http.DefaultClient.Do(req)
	testutil.NoError(t, err)
	defer resp.Body.Close()
	testutil.StatusCode(t, http.StatusOK, resp.StatusCode)

	var summary billing.UsageSummary
	testutil.NoError(t, json.NewDecoder(resp.Body).Decode(&summary))
	testutil.Equal(t, tenantID, summary.TenantID)
	testutil.Equal(t, "week", summary.Period)
	testutil.Equal(t, billing.PlanPro, summary.Plan)
	testutil.Equal(t, 3, len(summary.Data))
	testutil.Equal(t, int64(600), summary.Totals.APIRequests)
	testutil.Equal(t, int64(4096), summary.Totals.StorageBytesUsed)
	testutil.Equal(t, int64(3584), summary.Totals.BandwidthBytes)
	testutil.Equal(t, int64(1200), summary.Totals.FunctionInvocations)
	testutil.Equal(t, billing.LimitsForPlan(billing.PlanPro), summary.Limits)
}

func TestUsageIntegration_TenantUsageIsolatedByJWTTenant(t *testing.T) {
	ctx, ts, authSecret := setupUsageIntegrationServer(t)

	now := time.Now().UTC().Truncate(24 * time.Hour)
	tenantOneID := seedTenantUsageAndPlan(t, ctx, "Tenant One", "tenant-one-"+strconv.FormatInt(time.Now().UnixNano(), 10), billing.PlanStarter,
		[]tenant.TenantUsageDaily{
			{Date: now.AddDate(0, 0, -1), RequestCount: 10, DBBytesUsed: 100, BandwidthBytes: 200, FunctionInvocations: 300},
			{Date: now, RequestCount: 20, DBBytesUsed: 150, BandwidthBytes: 250, FunctionInvocations: 350},
		},
	)
	_ = seedTenantUsageAndPlan(t, ctx, "Tenant Two", "tenant-two-"+strconv.FormatInt(time.Now().UnixNano(), 10), billing.PlanPro,
		[]tenant.TenantUsageDaily{
			{Date: now, RequestCount: 9999, DBBytesUsed: 9999, BandwidthBytes: 9999, FunctionInvocations: 9999},
		},
	)

	jwtToken := issueTenantJWT(t, authSecret, "00000000-0000-0000-0000-00000000abcd", tenantOneID)
	req, err := http.NewRequest(http.MethodGet, ts.URL+"/api/usage?period=week", nil)
	testutil.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+jwtToken)

	resp, err := http.DefaultClient.Do(req)
	testutil.NoError(t, err)
	defer resp.Body.Close()
	testutil.StatusCode(t, http.StatusOK, resp.StatusCode)

	var summary billing.UsageSummary
	testutil.NoError(t, json.NewDecoder(resp.Body).Decode(&summary))
	testutil.Equal(t, tenantOneID, summary.TenantID)
	testutil.Equal(t, billing.PlanStarter, summary.Plan)
	testutil.Equal(t, int64(30), summary.Totals.APIRequests)
	testutil.Equal(t, int64(150), summary.Totals.StorageBytesUsed)
	testutil.Equal(t, int64(450), summary.Totals.BandwidthBytes)
	testutil.Equal(t, int64(650), summary.Totals.FunctionInvocations)
}
