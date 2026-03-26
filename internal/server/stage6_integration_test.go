//go:build integration

package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/tenant"
	"github.com/allyourbase/ayb/internal/testutil"
)

// stage6AdminRequest reuses the stage5 integration helper with no X-Tenant-ID header.
func stage6AdminRequest(t *testing.T, srv *Server, method, path, token, body string) *httptest.ResponseRecorder {
	t.Helper()
	return stage5TenantAdminRequest(t, srv, method, path, token, "", body)
}

func stage6CreateTenant(t *testing.T, srv *Server, token, slug string) string {
	t.Helper()
	stage5EnsureUser(t, srv, stageIntegrationOwnerUserID)
	body := fmt.Sprintf(`{"name":"test-%s","slug":"%s","ownerUserId":"%s","isolationMode":"schema","planTier":"free"}`,
		slug, slug, stageIntegrationOwnerUserID)
	w := stage6AdminRequest(t, srv, http.MethodPost, "/api/admin/tenants", token, body)
	testutil.Equal(t, http.StatusCreated, w.Code)
	var created tenant.Tenant
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	created = stage5ActivateTenant(t, srv, created.ID)
	return created.ID
}

func TestStage6MaintenanceModeIntegration(t *testing.T) {
	ctx := context.Background()
	pg := newRequestLoggerTestDB(t)
	ensureIntegrationMigrations(t, ctx, pg.Pool)

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(pg.Pool, logger)
	testutil.NoError(t, ch.Load(ctx))

	cfg := config.Default()
	cfg.Admin.Password = "testpass"
	srv := New(cfg, logger, ch, pg.Pool, nil, nil)

	tenantSvc := tenant.NewService(pg.Pool, logger)
	srv.SetTenantService(tenantSvc)
	srv.SetUsageAccumulator(tenant.NewUsageAccumulator(pg.Pool, logger))
	srv.SetQuotaChecker(tenant.DefaultQuotaChecker{})
	rl := tenant.NewTenantRateLimiter(time.Minute)
	srv.SetTenantRateLimiter(rl)
	defer rl.Stop()
	srv.SetTenantConnCounter(tenant.NewTenantConnCounter())

	breakerTracker := tenant.NewTenantBreakerTracker(tenant.TenantBreakerConfig{}, nil)
	srv.SetTenantBreakerTracker(breakerTracker)

	adminToken := stage5AdminLogin(t, srv)

	// Create two tenants.
	slug1 := fmt.Sprintf("maint-a-%d", time.Now().UnixNano())
	slug2 := fmt.Sprintf("maint-b-%d", time.Now().UnixNano())
	tenantA := stage6CreateTenant(t, srv, adminToken, slug1)
	tenantB := stage6CreateTenant(t, srv, adminToken, slug2)

	// Enable maintenance on tenant A.
	w := stage6AdminRequest(t, srv, http.MethodPost,
		"/api/admin/tenants/"+tenantA+"/maintenance/enable", adminToken,
		`{"reason":"planned upgrade"}`)
	testutil.Equal(t, http.StatusOK, w.Code)

	// Verify tenant A admin endpoint is reachable (recovery bypass).
	w = stage6AdminRequest(t, srv, http.MethodGet,
		"/api/admin/tenants/"+tenantA+"/maintenance", adminToken, "")
	testutil.Equal(t, http.StatusOK, w.Code)
	var mState maintenanceStateResponse
	json.Unmarshal(w.Body.Bytes(), &mState)
	testutil.True(t, mState.Enabled, "expected tenant A under maintenance")

	// Verify tenant B is not affected.
	w = stage6AdminRequest(t, srv, http.MethodGet,
		"/api/admin/tenants/"+tenantB, adminToken, "")
	testutil.Equal(t, http.StatusOK, w.Code)

	// Disable maintenance on tenant A.
	w = stage6AdminRequest(t, srv, http.MethodPost,
		"/api/admin/tenants/"+tenantA+"/maintenance/disable", adminToken, "")
	testutil.Equal(t, http.StatusOK, w.Code)

	// Verify tenant A is accessible again.
	w = stage6AdminRequest(t, srv, http.MethodGet,
		"/api/admin/tenants/"+tenantA, adminToken, "")
	testutil.Equal(t, http.StatusOK, w.Code)
}

func TestStage6CircuitBreakerIntegration(t *testing.T) {
	ctx := context.Background()
	pg := newRequestLoggerTestDB(t)
	ensureIntegrationMigrations(t, ctx, pg.Pool)

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(pg.Pool, logger)
	testutil.NoError(t, ch.Load(ctx))

	cfg := config.Default()
	cfg.Admin.Password = "testpass"
	srv := New(cfg, logger, ch, pg.Pool, nil, nil)

	tenantSvc := tenant.NewService(pg.Pool, logger)
	srv.SetTenantService(tenantSvc)
	srv.SetUsageAccumulator(tenant.NewUsageAccumulator(pg.Pool, logger))
	srv.SetQuotaChecker(tenant.DefaultQuotaChecker{})
	rl := tenant.NewTenantRateLimiter(time.Minute)
	srv.SetTenantRateLimiter(rl)
	defer rl.Stop()
	srv.SetTenantConnCounter(tenant.NewTenantConnCounter())

	now := time.Now()
	breakerTracker := tenant.NewTenantBreakerTracker(tenant.TenantBreakerConfig{
		FailureThreshold:    2,
		OpenDuration:        100 * time.Millisecond,
		HalfOpenMaxRequests: 1,
	}, func() time.Time { return now })
	srv.SetTenantBreakerTracker(breakerTracker)

	adminToken := stage5AdminLogin(t, srv)
	slug := fmt.Sprintf("breaker-%d", time.Now().UnixNano())
	tenantID := stage6CreateTenant(t, srv, adminToken, slug)

	// Drive breaker to open by recording failures directly.
	breakerTracker.RecordFailure(tenantID)
	breakerTracker.RecordFailure(tenantID)

	// Verify breaker is open via admin endpoint.
	w := stage6AdminRequest(t, srv, http.MethodGet,
		"/api/admin/tenants/"+tenantID+"/breaker", adminToken, "")
	testutil.Equal(t, http.StatusOK, w.Code)
	var bState breakerStateResponse
	json.Unmarshal(w.Body.Bytes(), &bState)
	testutil.Equal(t, "open", bState.State)
	testutil.Equal(t, 2, bState.ConsecutiveFailures)

	// Admin reset resets the breaker.
	w = stage6AdminRequest(t, srv, http.MethodPost,
		"/api/admin/tenants/"+tenantID+"/breaker/reset", adminToken, "")
	testutil.Equal(t, http.StatusOK, w.Code)
	json.Unmarshal(w.Body.Bytes(), &bState)
	testutil.Equal(t, "closed", bState.State)
	testutil.Equal(t, 0, bState.ConsecutiveFailures)

	// Drive breaker open again, then advance time to half_open.
	breakerTracker.RecordFailure(tenantID)
	breakerTracker.RecordFailure(tenantID)
	testutil.Equal(t, tenant.BreakerStateOpen, breakerTracker.State(tenantID))

	// Advance time past cooldown.
	now = now.Add(200 * time.Millisecond)

	// Allow should succeed (half_open probe).
	err := breakerTracker.Allow(tenantID)
	testutil.True(t, err == nil, "expected allow in half_open")

	// Record success to close breaker.
	breakerTracker.RecordSuccess(tenantID)
	testutil.Equal(t, tenant.BreakerStateClosed, breakerTracker.State(tenantID))

	// Verify closed via admin endpoint.
	w = stage6AdminRequest(t, srv, http.MethodGet,
		"/api/admin/tenants/"+tenantID+"/breaker", adminToken, "")
	testutil.Equal(t, http.StatusOK, w.Code)
	json.Unmarshal(w.Body.Bytes(), &bState)
	testutil.Equal(t, "closed", bState.State)
}
