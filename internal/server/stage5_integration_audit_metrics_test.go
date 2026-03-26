//go:build integration

package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/tenant"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/golang-jwt/jwt/v5"
)

type tenantAuditResultEnvelope struct {
	Items  []tenant.TenantAuditEvent `json:"items"`
	Count  int                       `json:"count"`
	Limit  int                       `json:"limit"`
	Offset int                       `json:"offset"`
}

const (
	stageIntegrationOwnerUserID  = "11111111-1111-1111-1111-111111111111"
	stageIntegrationMemberUserID = "22222222-2222-2222-2222-222222222222"
)

func stage5TenantAdminRequest(t *testing.T, srv *Server, method, path, token, tenantID, body string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if tenantID != "" {
		req.Header.Set("X-Tenant-ID", tenantID)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}

	resp := httptest.NewRecorder()
	srv.Router().ServeHTTP(resp, req)
	return resp
}

func stage5EnsureUser(t *testing.T, srv *Server, userID string) {
	t.Helper()

	email := fmt.Sprintf("integration-%s@example.com", strings.ReplaceAll(userID, "-", ""))
	_, err := srv.pool.Exec(context.Background(),
		`INSERT INTO _ayb_users (id, email, password_hash)
		 VALUES ($1, $2, 'integration-password-hash')
		 ON CONFLICT (id) DO NOTHING`,
		userID,
		email,
	)
	testutil.NoError(t, err)
}

func stage5ActivateTenant(t *testing.T, srv *Server, tenantID string) tenant.Tenant {
	t.Helper()

	tenantSvc := tenant.NewService(srv.pool, testutil.DiscardLogger())
	current, err := tenantSvc.GetTenant(context.Background(), tenantID)
	testutil.NoError(t, err)
	if current.State == tenant.TenantStateActive {
		return *current
	}

	activated, err := tenantSvc.TransitionState(context.Background(), tenantID, current.State, tenant.TenantStateActive)
	testutil.NoError(t, err)
	return *activated
}

func stage5TriggerQuotaViolation(t *testing.T, srv *Server, tenantID string, hardLimit int) {
	t.Helper()

	allowedRequests := effectiveRateLimitPerMinute(&hardLimit, nil)
	for i := 0; i < allowedRequests; i++ {
		resp := stage5TenantAdminRequest(t, srv, http.MethodGet, "/api/admin/status", "", tenantID, "")
		testutil.Equal(t, http.StatusOK, resp.Code)
	}

	blocked := stage5TenantAdminRequest(t, srv, http.MethodGet, "/api/admin/status", "", tenantID, "")
	testutil.Equal(t, http.StatusTooManyRequests, blocked.Code)
}

func stage5MetricLineExists(body string, filters ...string) bool {
	metricName := "ayb_http_requests_total"

	for _, line := range strings.Split(body, "\n") {
		if !strings.HasPrefix(line, metricName) {
			continue
		}
		allMatched := true
		for _, filter := range filters {
			if !strings.Contains(line, filter) {
				allMatched = false
				break
			}
		}
		if allMatched {
			return true
		}
	}
	return false
}

func stage5HasMetricLine(body, tenantID, route, status string) bool {
	return stage5MetricLineExists(
		body,
		fmt.Sprintf(`tenant_id="%s"`, tenantID),
		fmt.Sprintf(`route="%s"`, route),
		fmt.Sprintf(`status="%s"`, status),
	)
}

func stage5TenantHasNoMetricLine(body, tenantID, route string) bool {
	return !stage5MetricLineExists(
		body,
		fmt.Sprintf(`tenant_id="%s"`, tenantID),
		fmt.Sprintf(`route="%s"`, route),
	)
}

func stage5AdminLogin(t *testing.T, srv *Server) string {
	t.Helper()

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/auth", strings.NewReader(`{"password":"testpass"}`))
	req.Header.Set("Content-Type", "application/json")
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)

	var body map[string]string
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	token := body["token"]
	testutil.True(t, token != "", "admin login should return non-empty token")
	return token
}

func TestStage5AuditTrailIntegrationIncludesLifecycleMembershipQuotaAndCrossTenantSignals(t *testing.T) {
	ctx := context.Background()
	pg := newRequestLoggerTestDB(t)
	ensureIntegrationMigrations(t, ctx, pg.Pool)

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(pg.Pool, logger)
	err := ch.Load(ctx)
	testutil.NoError(t, err)

	cfg := config.Default()
	cfg.Admin.Password = "testpass"
	srv := New(cfg, logger, ch, pg.Pool, nil, nil)

	tenantSvc := tenant.NewService(pg.Pool, logger)
	usageAcc := tenant.NewUsageAccumulator(pg.Pool, logger)
	srv.SetTenantService(tenantSvc)
	srv.SetUsageAccumulator(usageAcc)
	srv.SetQuotaChecker(tenant.DefaultQuotaChecker{})

	r := tenant.NewTenantRateLimiter(time.Minute)
	srv.SetTenantRateLimiter(r)
	defer r.Stop()

	adminToken := stage5AdminLogin(t, srv)
	stage5EnsureUser(t, srv, stageIntegrationOwnerUserID)
	stage5EnsureUser(t, srv, stageIntegrationMemberUserID)

	createBody := fmt.Sprintf(`{"name":"audit-integration","slug":"%s","ownerUserId":"%s","isolationMode":"schema","planTier":"free","region":"region-1"}`,
		fmt.Sprintf("tenant-%d", time.Now().UnixNano()),
		stageIntegrationOwnerUserID,
	)
	createResp := stage5TenantAdminRequest(t, srv, http.MethodPost, "/api/admin/tenants", adminToken, "", createBody)
	testutil.Equal(t, http.StatusCreated, createResp.Code)

	var createdTenant tenant.Tenant
	testutil.NoError(t, json.Unmarshal(createResp.Body.Bytes(), &createdTenant))
	testutil.True(t, createdTenant.ID != "")
	createdTenant = stage5ActivateTenant(t, srv, createdTenant.ID)

	updateResp := stage5TenantAdminRequest(t, srv, http.MethodPut, "/api/admin/tenants/"+createdTenant.ID, adminToken, "", `{"name":"audit-integration-updated"}`)
	testutil.Equal(t, http.StatusOK, updateResp.Code)

	suspendResp := stage5TenantAdminRequest(t, srv, http.MethodPost, "/api/admin/tenants/"+createdTenant.ID+"/suspend", adminToken, "", "")
	testutil.Equal(t, http.StatusOK, suspendResp.Code)

	resumeResp := stage5TenantAdminRequest(t, srv, http.MethodPost, "/api/admin/tenants/"+createdTenant.ID+"/resume", adminToken, "", "")
	testutil.Equal(t, http.StatusOK, resumeResp.Code)

	memberAddResp := stage5TenantAdminRequest(t, srv, http.MethodPost, "/api/admin/tenants/"+createdTenant.ID+"/members", adminToken, "",
		fmt.Sprintf(`{"userId":"%s","role":"%s"}`, stageIntegrationMemberUserID, tenant.MemberRoleMember),
	)
	testutil.Equal(t, http.StatusCreated, memberAddResp.Code)

	memberUpdateResp := stage5TenantAdminRequest(t, srv, http.MethodPut, "/api/admin/tenants/"+createdTenant.ID+"/members/"+stageIntegrationMemberUserID, adminToken, "",
		fmt.Sprintf(`{"role":"%s"}`, tenant.MemberRoleAdmin),
	)
	testutil.Equal(t, http.StatusOK, memberUpdateResp.Code)

	memberRemoveResp := stage5TenantAdminRequest(t, srv, http.MethodDelete, "/api/admin/tenants/"+createdTenant.ID+"/members/"+stageIntegrationMemberUserID, adminToken, "", "")
	testutil.Equal(t, http.StatusNoContent, memberRemoveResp.Code)

	quota := 1
	_, err = tenantSvc.SetQuotas(ctx, createdTenant.ID, tenant.TenantQuotas{RequestRateRPSHard: &quota})
	testutil.NoError(t, err)

	stage5TriggerQuotaViolation(t, srv, createdTenant.ID, quota)

	// Trigger cross-tenant blocked action: claim tenant does not match request tenant.
	claimContext := auth.ContextWithClaims(context.Background(), &auth.Claims{
		TenantID:         createdTenant.ID,
		RegisteredClaims: jwt.RegisteredClaims{Subject: stageIntegrationMemberUserID},
	})
	claimContext = tenant.ContextWithTenantID(claimContext, "another-tenant")
	blockedReq := httptest.NewRequest(http.MethodGet, "/api/admin/status", nil).WithContext(claimContext)
	blockedRecorder := httptest.NewRecorder()
	srv.enforceTenantMatch(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(blockedRecorder, blockedReq)
	testutil.Equal(t, http.StatusForbidden, blockedRecorder.Code)

	deleteResp := stage5TenantAdminRequest(t, srv, http.MethodDelete, "/api/admin/tenants/"+createdTenant.ID, adminToken, "", "")
	testutil.Equal(t, http.StatusOK, deleteResp.Code)

	auditReq := httptest.NewRequest(http.MethodGet, "/api/admin/tenants/"+createdTenant.ID+"/audit", nil)
	auditReq.Header.Set("Authorization", "Bearer "+adminToken)
	auditResp := httptest.NewRecorder()
	srv.Router().ServeHTTP(auditResp, auditReq)
	testutil.Equal(t, http.StatusOK, auditResp.Code)

	var body tenantAuditResultEnvelope
	testutil.NoError(t, json.Unmarshal(auditResp.Body.Bytes(), &body))
	expectedActions := []string{
		tenant.AuditActionTenantDeleted,
		tenant.AuditActionCrossTenantBlocked,
		tenant.AuditActionQuotaViolation,
		tenant.AuditActionMembershipRemoved,
		tenant.AuditActionMembershipRoleChange,
		tenant.AuditActionMembershipAdded,
		tenant.AuditActionTenantResumed,
		tenant.AuditActionTenantSuspended,
		tenant.AuditActionTenantUpdated,
		tenant.AuditActionTenantCreated,
	}
	expectedResults := []string{
		tenant.AuditResultSuccess,
		tenant.AuditResultDenied,
		tenant.AuditResultDenied,
		tenant.AuditResultSuccess,
		tenant.AuditResultSuccess,
		tenant.AuditResultSuccess,
		tenant.AuditResultSuccess,
		tenant.AuditResultSuccess,
		tenant.AuditResultSuccess,
		tenant.AuditResultSuccess,
	}

	testutil.Equal(t, len(expectedActions), body.Count)
	testutil.Equal(t, len(expectedActions), len(body.Items))
	for i, item := range body.Items {
		testutil.Equal(t, expectedActions[i], item.Action)
		testutil.Equal(t, expectedResults[i], item.Result)
	}
}

func TestStage5TenantMetricsExposeTenantLabeledSeriesForScopedTraffic(t *testing.T) {
	ctx := context.Background()
	pg := newRequestLoggerTestDB(t)
	ensureIntegrationMigrations(t, ctx, pg.Pool)

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(pg.Pool, logger)
	err := ch.Load(ctx)
	testutil.NoError(t, err)

	cfg := config.Default()
	cfg.Metrics.Enabled = true
	srv := New(cfg, logger, ch, pg.Pool, nil, nil)

	tenantSvc := tenant.NewService(pg.Pool, logger)
	usageAcc := tenant.NewUsageAccumulator(pg.Pool, logger)
	srv.SetTenantService(tenantSvc)
	srv.SetUsageAccumulator(usageAcc)
	srv.SetQuotaChecker(tenant.DefaultQuotaChecker{})

	r := tenant.NewTenantRateLimiter(time.Minute)
	srv.SetTenantRateLimiter(r)
	defer r.Stop()

	createdTenant, err := tenantSvc.CreateTenant(context.Background(), "metric-tenant", fmt.Sprintf("metric-tenant-%d", time.Now().UnixNano()), "schema", "free", "metric-region", nil, "")
	testutil.NoError(t, err)
	activatedTenant := stage5ActivateTenant(t, srv, createdTenant.ID)

	hardLimit := 1
	_, err = tenantSvc.SetQuotas(ctx, activatedTenant.ID, tenant.TenantQuotas{RequestRateRPSHard: &hardLimit})
	testutil.NoError(t, err)

	stage5TenantAdminRequest(t, srv, http.MethodGet, "/health", "", "", "")
	stage5TriggerQuotaViolation(t, srv, activatedTenant.ID, hardLimit)

	metricsReq := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	metricsResp := httptest.NewRecorder()
	srv.Router().ServeHTTP(metricsResp, metricsReq)
	testutil.Equal(t, http.StatusOK, metricsResp.Code)

	body := metricsResp.Body.String()
	testutil.True(t, stage5HasMetricLine(body, activatedTenant.ID, "/api/admin/status", "200"))
	testutil.True(t, stage5MetricLineExists(body, fmt.Sprintf(`tenant_id="%s"`, activatedTenant.ID), `status="429"`))
	testutil.True(t, strings.Contains(body, `route="/health"`))
	testutil.True(t, stage5TenantHasNoMetricLine(body, activatedTenant.ID, "/health"))
	testutil.True(t, strings.Contains(body, "ayb_http_requests_total"))
}
