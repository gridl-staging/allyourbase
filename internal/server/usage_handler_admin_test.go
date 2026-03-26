package server

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/billing"
	"github.com/allyourbase/ayb/internal/tenant"
	"github.com/go-chi/chi/v5"
)

func TestHandleAdminUsageReturnsServiceUnavailableWithoutSource(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	handleAdminUsage(nil).ServeHTTP(w, usageRequestWithTenantID("/api/admin/usage/00000000-0000-0000-0000-000000000101", "00000000-0000-0000-0000-000000000101"))

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", w.Code)
	}
	if got := usageErrorMessage(t, w); got != "usage service not configured" {
		t.Fatalf("message = %q, want %q", got, "usage service not configured")
	}
}

func TestHandleAdminUsageReturnsUsageSummary(t *testing.T) {
	t.Parallel()

	tenantID := "00000000-0000-0000-0000-000000000101"
	src := usageSummaryFixtureSource(tenantID, "Acme", billing.PlanPro, []tenant.TenantUsageDaily{
		{TenantID: tenantID, Date: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC), RequestCount: 100, DBBytesUsed: 300, BandwidthBytes: 500, FunctionInvocations: 700},
		{TenantID: tenantID, Date: time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC), RequestCount: 200, DBBytesUsed: 250, BandwidthBytes: 600, FunctionInvocations: 900},
	})

	r := chi.NewRouter()
	r.Get("/api/admin/usage/{tenant_id}", handleAdminUsage(src))

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/admin/usage/"+tenantID+"?period=week", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", w.Code, w.Body.String())
	}

	got := decodeUsageTestResponse[billing.UsageSummary](t, w)
	if got.TenantID != tenantID || got.Period != "week" || got.Plan != billing.PlanPro {
		t.Fatalf("unexpected summary header: %+v", got)
	}
	if len(got.Data) != 2 {
		t.Fatalf("data len = %d, want 2", len(got.Data))
	}
	if got.Totals.APIRequests != 300 || got.Totals.BandwidthBytes != 1100 || got.Totals.FunctionInvocations != 1600 || got.Totals.StorageBytesUsed != 300 {
		t.Fatalf("unexpected totals: %+v", got.Totals)
	}
}

func TestHandleAdminUsageValidationErrors(t *testing.T) {
	t.Parallel()

	tenantID := "00000000-0000-0000-0000-000000000101"
	h := handleAdminUsage(usageSummaryFixtureSource(tenantID, "Acme", billing.PlanPro, nil))
	cases := []struct {
		name          string
		path          string
		routeTenantID string
		wantMessage   string
	}{
		{name: "missing tenant id", path: "/api/admin/usage", wantMessage: "tenant_id is required"},
		{name: "invalid tenant id format", path: "/api/admin/usage/not-a-uuid", routeTenantID: "not-a-uuid", wantMessage: "invalid tenant_id format"},
		{name: "invalid period", path: "/api/admin/usage/" + tenantID + "?period=year", routeTenantID: tenantID, wantMessage: "invalid period or date range"},
		{name: "invalid date range", path: "/api/admin/usage/" + tenantID + "?from=2026-03-10&to=2026-03-01", routeTenantID: tenantID, wantMessage: "invalid period or date range"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			h.ServeHTTP(w, usageRequestWithTenantID(tc.path, tc.routeTenantID))

			if w.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", w.Code)
			}
			if got := usageErrorMessage(t, w); got != tc.wantMessage {
				t.Fatalf("message = %q, want %q", got, tc.wantMessage)
			}
		})
	}
}

func TestHandleAdminUsageReturnsNotFoundAndBackendErrors(t *testing.T) {
	t.Parallel()

	tenantID := "00000000-0000-0000-0000-000000000101"
	rows := []tenant.TenantUsageDaily{{TenantID: tenantID, Date: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)}}
	cases := []struct {
		name        string
		src         *fakeUsageDataSource
		routeTenant string
		wantStatus  int
		wantMessage string
	}{
		{
			name:        "tenant not found",
			src:         usageSummaryFixtureSource(tenantID, "Acme", billing.PlanPro, rows),
			routeTenant: "00000000-0000-0000-0000-000000000999",
			wantStatus:  http.StatusNotFound,
			wantMessage: "tenant not found",
		},
		{
			name:        "tenant load failure",
			src:         &fakeUsageDataSource{tenantErr: errors.New("db down")},
			routeTenant: tenantID,
			wantStatus:  http.StatusInternalServerError,
			wantMessage: "failed to load tenant",
		},
		{
			name:        "billing lookup failure",
			src:         &fakeUsageDataSource{tenantByID: map[string]*tenant.Tenant{tenantID: {ID: tenantID}}, billingErr: errors.New("billing unavailable")},
			routeTenant: tenantID,
			wantStatus:  http.StatusInternalServerError,
			wantMessage: "failed to load billing plan",
		},
		{
			name:        "usage range failure",
			src:         &fakeUsageDataSource{tenantByID: map[string]*tenant.Tenant{tenantID: {ID: tenantID}}, billingByID: map[string]*billing.BillingRecord{tenantID: {TenantID: tenantID, Plan: billing.PlanFree}}, usageErr: errors.New("usage unavailable")},
			routeTenant: tenantID,
			wantStatus:  http.StatusInternalServerError,
			wantMessage: "failed to load usage",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			handleAdminUsage(tc.src).ServeHTTP(w, usageRequestWithTenantID("/api/admin/usage/"+tc.routeTenant, tc.routeTenant))

			if w.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", w.Code, tc.wantStatus)
			}
			if got := usageErrorMessage(t, w); got != tc.wantMessage {
				t.Fatalf("message = %q, want %q", got, tc.wantMessage)
			}
		})
	}
}
