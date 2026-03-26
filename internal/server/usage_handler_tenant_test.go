package server

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/billing"
	"github.com/allyourbase/ayb/internal/tenant"
)

func TestHandleTenantUsageReturnsServiceUnavailableWithoutSource(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	handleTenantUsage(nil).ServeHTTP(w, usageClaimsRequest("/api/usage?period=day", "00000000-0000-0000-0000-000000000202"))

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", w.Code)
	}
	if got := usageErrorMessage(t, w); got != "usage service not configured" {
		t.Fatalf("message = %q, want %q", got, "usage service not configured")
	}
}

func TestHandleTenantUsageReturnsUsageSummaryFromClaims(t *testing.T) {
	t.Parallel()

	tenantID := "00000000-0000-0000-0000-000000000202"
	src := usageSummaryFixtureSource(tenantID, "Bravo", billing.PlanStarter, []tenant.TenantUsageDaily{
		{TenantID: tenantID, Date: time.Date(2026, 3, 3, 0, 0, 0, 0, time.UTC), RequestCount: 42, DBBytesUsed: 64, BandwidthBytes: 128, FunctionInvocations: 256},
	})

	w := httptest.NewRecorder()
	handleTenantUsage(src).ServeHTTP(w, usageClaimsRequest("/api/usage?period=day", tenantID))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", w.Code, w.Body.String())
	}

	got := decodeUsageTestResponse[billing.UsageSummary](t, w)
	if got.TenantID != tenantID || got.Plan != billing.PlanStarter || got.Period != "day" {
		t.Fatalf("unexpected summary: %+v", got)
	}
}

func TestHandleTenantUsageFallsBackToTenantContext(t *testing.T) {
	t.Parallel()

	tenantID := "00000000-0000-0000-0000-000000000202"
	src := usageSummaryFixtureSource(tenantID, "Bravo", billing.PlanStarter, []tenant.TenantUsageDaily{
		{TenantID: tenantID, Date: time.Date(2026, 3, 3, 0, 0, 0, 0, time.UTC), RequestCount: 42},
	})

	w := httptest.NewRecorder()
	handleTenantUsage(src).ServeHTTP(w, usageTenantContextRequest("/api/usage?period=day", tenantID))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}

func TestHandleTenantUsageRejectsMissingTenantContext(t *testing.T) {
	t.Parallel()

	tenantID := "00000000-0000-0000-0000-000000000202"
	h := handleTenantUsage(usageSummaryFixtureSource(tenantID, "Bravo", billing.PlanStarter, nil))
	cases := []struct {
		name string
		req  *http.Request
	}{
		{name: "no tenant context", req: httptest.NewRequest(http.MethodGet, "/api/usage?period=day", nil)},
		{name: "scoped claims without tenant id", req: func() *http.Request {
			req := usageClaimsRequest("/api/usage?period=day", "   ")
			return req.WithContext(tenant.ContextWithTenantID(req.Context(), tenantID))
		}()},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			h.ServeHTTP(w, tc.req)

			if w.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want 403", w.Code)
			}
			if got := usageErrorMessage(t, w); got != "tenant context required" {
				t.Fatalf("message = %q, want %q", got, "tenant context required")
			}
		})
	}
}

func TestHandleTenantUsageRejectsInvalidRange(t *testing.T) {
	t.Parallel()

	tenantID := "00000000-0000-0000-0000-000000000202"
	h := handleTenantUsage(usageSummaryFixtureSource(tenantID, "Bravo", billing.PlanStarter, nil))
	for _, path := range []string{"/api/usage?period=year", "/api/usage?from=2026-03-10&to=2026-03-01"} {
		path := path
		t.Run(path, func(t *testing.T) {
			w := httptest.NewRecorder()
			h.ServeHTTP(w, usageClaimsRequest(path, tenantID))

			if w.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", w.Code)
			}
			if got := usageErrorMessage(t, w); got != "invalid period or date range" {
				t.Fatalf("message = %q, want %q", got, "invalid period or date range")
			}
		})
	}
}

func TestHandleTenantUsageDefaultsPlanFreeWhenBillingMissing(t *testing.T) {
	t.Parallel()

	tenantID := "00000000-0000-0000-0000-000000000202"
	src := &fakeUsageDataSource{
		tenantByID: map[string]*tenant.Tenant{tenantID: {ID: tenantID}},
		usageByID: map[string][]tenant.TenantUsageDaily{
			tenantID: {{TenantID: tenantID, Date: time.Date(2026, 3, 3, 0, 0, 0, 0, time.UTC), RequestCount: 42}},
		},
	}

	w := httptest.NewRecorder()
	handleTenantUsage(src).ServeHTTP(w, usageClaimsRequest("/api/usage?period=day", tenantID))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if got := decodeUsageTestResponse[billing.UsageSummary](t, w); got.Plan != billing.PlanFree {
		t.Fatalf("plan = %q, want %q", got.Plan, billing.PlanFree)
	}
}

func TestHandleTenantUsageReturnsBackendErrors(t *testing.T) {
	t.Parallel()

	tenantID := "00000000-0000-0000-0000-000000000202"
	cases := []struct {
		name        string
		src         *fakeUsageDataSource
		wantMessage string
	}{
		{
			name:        "usage load failure",
			src:         &fakeUsageDataSource{tenantByID: map[string]*tenant.Tenant{tenantID: {ID: tenantID}}, usageErr: errors.New("query failed")},
			wantMessage: "failed to load usage",
		},
		{
			name:        "billing lookup failure",
			src:         &fakeUsageDataSource{tenantByID: map[string]*tenant.Tenant{tenantID: {ID: tenantID}}, billingErr: errors.New("billing unavailable")},
			wantMessage: "failed to load billing plan",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			handleTenantUsage(tc.src).ServeHTTP(w, usageClaimsRequest("/api/usage?period=day", tenantID))

			if w.Code != http.StatusInternalServerError {
				t.Fatalf("status = %d, want 500", w.Code)
			}
			if got := usageErrorMessage(t, w); got != tc.wantMessage {
				t.Fatalf("message = %q, want %q", got, tc.wantMessage)
			}
		})
	}
}
