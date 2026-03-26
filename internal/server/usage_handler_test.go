package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/billing"
	"github.com/allyourbase/ayb/internal/tenant"
	"github.com/go-chi/chi/v5"
)

type fakeUsageDataSource struct {
	tenantByID  map[string]*tenant.Tenant
	usageByID   map[string][]tenant.TenantUsageDaily
	billingByID map[string]*billing.BillingRecord

	tenantErr  error
	usageErr   error
	billingErr error

	billingCalls int
}

func (f *fakeUsageDataSource) GetTenant(ctx context.Context, tenantID string) (*tenant.Tenant, error) {
	if f.tenantErr != nil {
		return nil, f.tenantErr
	}
	t, ok := f.tenantByID[tenantID]
	if !ok {
		return nil, tenant.ErrTenantNotFound
	}
	return t, nil
}

func (f *fakeUsageDataSource) GetUsageRange(ctx context.Context, tenantID string, startDate, endDate time.Time) ([]tenant.TenantUsageDaily, error) {
	if f.usageErr != nil {
		return nil, f.usageErr
	}
	rows, ok := f.usageByID[tenantID]
	if !ok {
		return []tenant.TenantUsageDaily{}, nil
	}
	return rows, nil
}

func (f *fakeUsageDataSource) GetBillingRecord(ctx context.Context, tenantID string) (*billing.BillingRecord, error) {
	f.billingCalls++
	if f.billingErr != nil {
		return nil, f.billingErr
	}
	rec, ok := f.billingByID[tenantID]
	if !ok {
		return nil, billing.ErrBillingRecordNotFound
	}
	return rec, nil
}

func usageRequestWithTenantID(path, tenantID string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if tenantID == "" {
		return req
	}
	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("tenant_id", tenantID)
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))
}

func usageClaimsRequest(path, tenantID string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	return req.WithContext(auth.ContextWithClaims(req.Context(), &auth.Claims{TenantID: tenantID}))
}

func usageTenantContextRequest(path, tenantID string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	return req.WithContext(tenant.ContextWithTenantID(req.Context(), tenantID))
}

func usageSummaryFixtureSource(tenantID, tenantName string, plan billing.Plan, rows []tenant.TenantUsageDaily) *fakeUsageDataSource {
	return &fakeUsageDataSource{
		tenantByID: map[string]*tenant.Tenant{
			tenantID: {ID: tenantID, Name: tenantName},
		},
		billingByID: map[string]*billing.BillingRecord{
			tenantID: {TenantID: tenantID, Plan: plan},
		},
		usageByID: map[string][]tenant.TenantUsageDaily{
			tenantID: rows,
		},
	}
}

func decodeUsageTestResponse[T any](t *testing.T, w *httptest.ResponseRecorder) T {
	t.Helper()

	var got T
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return got
}

func usageErrorMessage(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()

	body := decodeUsageTestResponse[struct {
		Message string `json:"message"`
	}](t, w)
	return body.Message
}
