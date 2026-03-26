package server

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/allyourbase/ayb/internal/billing"
	"github.com/allyourbase/ayb/internal/tenant"
)

func TestHandleAdminUsageLimitsReturnsServiceUnavailableWithoutServices(t *testing.T) {
	t.Parallel()

	tenantID := "00000000-0000-0000-0000-000000000701"
	cases := []struct {
		name string
		src  usageDataSource
		agg  usageAggregateService
	}{
		{name: "missing source", src: nil, agg: &fakeUsageAggregateService{}},
		{name: "missing aggregate", src: &fakeUsageDataSource{}, agg: nil},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			handleAdminUsageLimits(tc.src, tc.agg).ServeHTTP(w, usageRequestWithTenantID("/admin/usage/"+tenantID+"/limits", tenantID))

			if w.Code != http.StatusServiceUnavailable {
				t.Fatalf("status = %d, want 503", w.Code)
			}
			if got := usageErrorMessage(t, w); got != "usage service not configured" {
				t.Fatalf("message = %q, want %q", got, "usage service not configured")
			}
		})
	}
}

func TestHandleAdminUsageLimitsValidationAndBackendErrors(t *testing.T) {
	t.Parallel()

	tenantID := "00000000-0000-0000-0000-000000000701"
	cases := []struct {
		name        string
		src         *fakeUsageDataSource
		agg         *fakeUsageAggregateService
		path        string
		routeTenant string
		wantStatus  int
		wantMessage string
	}{
		{
			name:        "invalid tenant id format",
			src:         &fakeUsageDataSource{},
			agg:         &fakeUsageAggregateService{},
			path:        "/admin/usage/not-a-uuid/limits",
			routeTenant: "not-a-uuid",
			wantStatus:  http.StatusBadRequest,
			wantMessage: "invalid tenant_id format",
		},
		{
			name:        "invalid period",
			src:         &fakeUsageDataSource{tenantByID: map[string]*tenant.Tenant{tenantID: {ID: tenantID}}},
			agg:         &fakeUsageAggregateService{},
			path:        "/admin/usage/" + tenantID + "/limits?period=year",
			routeTenant: tenantID,
			wantStatus:  http.StatusBadRequest,
			wantMessage: "invalid period or date range",
		},
		{
			name:        "tenant load failure",
			src:         &fakeUsageDataSource{tenantErr: errors.New("db down")},
			agg:         &fakeUsageAggregateService{},
			path:        "/admin/usage/" + tenantID + "/limits",
			routeTenant: tenantID,
			wantStatus:  http.StatusInternalServerError,
			wantMessage: "failed to load tenant",
		},
		{
			name:        "limits backend failure",
			src:         &fakeUsageDataSource{tenantByID: map[string]*tenant.Tenant{tenantID: {ID: tenantID}}},
			agg:         &fakeUsageAggregateService{limitsErr: errors.New("boom")},
			path:        "/admin/usage/" + tenantID + "/limits",
			routeTenant: tenantID,
			wantStatus:  http.StatusInternalServerError,
			wantMessage: "failed to load usage limits",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			handleAdminUsageLimits(tc.src, tc.agg).ServeHTTP(w, usageRequestWithTenantID(tc.path, tc.routeTenant))

			if w.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", w.Code, tc.wantStatus)
			}
			if got := usageErrorMessage(t, w); got != tc.wantMessage {
				t.Fatalf("message = %q, want %q", got, tc.wantMessage)
			}
		})
	}
}

func TestHandleAdminUsageLimitsReturnsNotFoundForMissingTenant(t *testing.T) {
	t.Parallel()

	tenantID := "00000000-0000-0000-0000-000000000701"
	w := httptest.NewRecorder()
	handleAdminUsageLimits(&fakeUsageDataSource{tenantByID: map[string]*tenant.Tenant{}}, &fakeUsageAggregateService{}).ServeHTTP(w, usageRequestWithTenantID("/admin/usage/"+tenantID+"/limits", tenantID))

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
	if got := usageErrorMessage(t, w); got != "tenant not found" {
		t.Fatalf("message = %q, want %q", got, "tenant not found")
	}
}

func TestHandleAdminUsageLimitsReturnsLimitsResponse(t *testing.T) {
	t.Parallel()

	tenantID := "00000000-0000-0000-0000-000000000701"
	src := &fakeUsageDataSource{tenantByID: map[string]*tenant.Tenant{tenantID: {ID: tenantID, Name: "Delta"}}}
	agg := &fakeUsageAggregateService{
		limitsResp: &billing.UsageLimitsResponse{
			Plan: billing.PlanStarter,
			Metrics: map[string]billing.MetricLimit{
				"api_requests": {Limit: 250_000, Used: 100, Remaining: 249_900},
			},
		},
	}

	w := httptest.NewRecorder()
	handleAdminUsageLimits(src, agg).ServeHTTP(w, usageRequestWithTenantID("/admin/usage/"+tenantID+"/limits?period=week", tenantID))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", w.Code, w.Body.String())
	}
	if agg.limitsTenantID != tenantID || agg.limitsPeriod != "week" {
		t.Fatalf("unexpected limits call: tenant=%s period=%s", agg.limitsTenantID, agg.limitsPeriod)
	}
	if src.billingCalls != 0 {
		t.Fatalf("billingCalls = %d, want 0", src.billingCalls)
	}
}

func TestHandleTenantUsageLimitsRejectsMissingTenantContext(t *testing.T) {
	t.Parallel()

	tenantID := "00000000-0000-0000-0000-000000000702"
	src := &fakeUsageDataSource{tenantByID: map[string]*tenant.Tenant{tenantID: {ID: tenantID, Name: "Echo"}}}
	agg := &fakeUsageAggregateService{limitsResp: &billing.UsageLimitsResponse{Plan: billing.PlanPro}}

	w := httptest.NewRecorder()
	handleTenantUsageLimits(src, agg).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/usage/limits", nil))

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", w.Code)
	}
	if got := usageErrorMessage(t, w); got != "tenant context required" {
		t.Fatalf("message = %q, want %q", got, "tenant context required")
	}
	if src.billingCalls != 0 {
		t.Fatalf("billingCalls = %d, want 0", src.billingCalls)
	}
}

func TestHandleTenantUsageLimitsReturnsServiceUnavailableWithoutServices(t *testing.T) {
	t.Parallel()

	tenantID := "00000000-0000-0000-0000-000000000702"
	cases := []struct {
		name string
		src  usageDataSource
		agg  usageAggregateService
	}{
		{name: "missing source", src: nil, agg: &fakeUsageAggregateService{}},
		{name: "missing aggregate", src: &fakeUsageDataSource{}, agg: nil},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			handleTenantUsageLimits(tc.src, tc.agg).ServeHTTP(w, usageClaimsRequest("/usage/limits?period=day", tenantID))

			if w.Code != http.StatusServiceUnavailable {
				t.Fatalf("status = %d, want 503", w.Code)
			}
			if got := usageErrorMessage(t, w); got != "usage service not configured" {
				t.Fatalf("message = %q, want %q", got, "usage service not configured")
			}
		})
	}
}

func TestHandleTenantUsageLimitsReturnsServiceAndBackendErrors(t *testing.T) {
	t.Parallel()

	tenantID := "00000000-0000-0000-0000-000000000702"
	cases := []struct {
		name        string
		src         *fakeUsageDataSource
		agg         *fakeUsageAggregateService
		path        string
		wantStatus  int
		wantMessage string
	}{
		{
			name:        "invalid period",
			src:         &fakeUsageDataSource{},
			agg:         &fakeUsageAggregateService{},
			path:        "/usage/limits?period=year",
			wantStatus:  http.StatusBadRequest,
			wantMessage: "invalid period or date range",
		},
		{
			name:        "limits backend failure",
			src:         &fakeUsageDataSource{},
			agg:         &fakeUsageAggregateService{limitsErr: errors.New("boom")},
			path:        "/usage/limits?period=day",
			wantStatus:  http.StatusInternalServerError,
			wantMessage: "failed to load usage limits",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			handleTenantUsageLimits(tc.src, tc.agg).ServeHTTP(w, usageClaimsRequest(tc.path, tenantID))

			if w.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", w.Code, tc.wantStatus)
			}
			if got := usageErrorMessage(t, w); got != tc.wantMessage {
				t.Fatalf("message = %q, want %q", got, tc.wantMessage)
			}
		})
	}
}

func TestHandleTenantUsageLimitsUsesClaimsTenantID(t *testing.T) {
	t.Parallel()

	tenantID := "00000000-0000-0000-0000-000000000702"
	src := &fakeUsageDataSource{tenantByID: map[string]*tenant.Tenant{tenantID: {ID: tenantID, Name: "Echo"}}}
	agg := &fakeUsageAggregateService{
		limitsResp: &billing.UsageLimitsResponse{
			Plan: billing.PlanPro,
			Metrics: map[string]billing.MetricLimit{
				"api_requests": {Limit: 1_000_000, Used: 500, Remaining: 999_500},
			},
		},
	}

	w := httptest.NewRecorder()
	handleTenantUsageLimits(src, agg).ServeHTTP(w, usageClaimsRequest("/usage/limits?period=day", tenantID))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if agg.limitsTenantID != tenantID || agg.limitsPeriod != "day" {
		t.Fatalf("unexpected limits call: tenant=%s period=%s", agg.limitsTenantID, agg.limitsPeriod)
	}
	if src.billingCalls != 0 {
		t.Fatalf("billingCalls = %d, want 0", src.billingCalls)
	}
}
