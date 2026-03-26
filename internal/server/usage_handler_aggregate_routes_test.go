package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/billing"
	"github.com/allyourbase/ayb/internal/tenant"
	"github.com/go-chi/chi/v5"
)

func TestRegisterAdminUsageRoutesAppliesAuthAndPreservesLiterals(t *testing.T) {
	t.Parallel()

	tenantID := "00000000-0000-0000-0000-000000000703"
	s := &Server{
		adminAuth: newAdminAuth("usage-admin"),
		usageSrc: &fakeUsageDataSource{
			tenantByID: map[string]*tenant.Tenant{tenantID: {ID: tenantID, Name: "Foxtrot"}},
			billingByID: map[string]*billing.BillingRecord{
				tenantID: {TenantID: tenantID, Plan: billing.PlanPro},
			},
		},
		usageAggregate: &fakeUsageAggregateService{
			trendRows: []billing.TrendPoint{{Timestamp: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC), Value: 99}},
		},
	}

	r := chi.NewRouter()
	s.registerAdminUsageRoutes(r)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/admin/usage/trends?metric=api_requests&granularity=day", nil))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status without auth = %d, want 401", w.Code)
	}

	req := httptest.NewRequest(http.MethodGet, "/admin/usage/trends?metric=api_requests&granularity=day", nil)
	req.Header.Set("Authorization", "Bearer "+s.adminAuth.token())
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status with auth = %d, want 200, body = %s", w.Code, w.Body.String())
	}
}

func TestRegisterAdminUsageRoutesNotMountedWhenUsageSrcNil(t *testing.T) {
	t.Parallel()

	s := &Server{
		adminAuth:      newAdminAuth("usage-admin"),
		usageAggregate: &fakeUsageAggregateService{},
	}

	r := chi.NewRouter()
	s.registerAdminUsageRoutes(r)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/admin/usage/trends?metric=api_requests&granularity=day", nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

func TestRegisterAdminUsageRoutesNotMountedWhenUsageAggregateNil(t *testing.T) {
	t.Parallel()

	s := &Server{
		adminAuth: newAdminAuth("usage-admin"),
		usageSrc: &fakeUsageDataSource{
			tenantByID: map[string]*tenant.Tenant{
				"00000000-0000-0000-0000-000000000703": {ID: "00000000-0000-0000-0000-000000000703", Name: "Foxtrot"},
			},
		},
	}

	r := chi.NewRouter()
	s.registerAdminUsageRoutes(r)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/admin/usage/trends?metric=api_requests&granularity=day", nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}
