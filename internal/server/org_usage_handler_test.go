package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/tenant"
	"github.com/allyourbase/ayb/internal/testutil"
)

type mockOrgUsageQuerier struct {
	rows        []tenant.TenantUsageDaily
	tenantCount int
	err         error
}

func (m *mockOrgUsageQuerier) GetOrgUsageRange(_ context.Context, _ string, _, _ time.Time) ([]tenant.TenantUsageDaily, int, error) {
	if m.err != nil {
		return nil, 0, m.err
	}
	return m.rows, m.tenantCount, nil
}

func TestHandleAdminOrgUsage_Success(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	store := &orgHandlerMockStore{
		getFn: func(_ context.Context, id string) (*tenant.Organization, error) {
			return &tenant.Organization{ID: id, Name: "Acme"}, nil
		},
	}
	querier := &mockOrgUsageQuerier{
		rows: []tenant.TenantUsageDaily{
			{Date: now, RequestCount: 100, DBBytesUsed: 2000, BandwidthBytes: 500, FunctionInvocations: 10},
		},
		tenantCount: 3,
	}
	h := handleAdminOrgUsage(store, querier)

	req := withURLParams(
		httptest.NewRequest(http.MethodGet, "/api/admin/orgs/org-1/usage?period=week", nil),
		map[string]string{"orgId": "org-1"},
	)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)

	var body OrgUsageSummary
	testutil.NoError(t, json.NewDecoder(w.Body).Decode(&body))
	testutil.Equal(t, "org-1", body.OrgID)
	testutil.Equal(t, 3, body.TenantCount)
	testutil.Equal(t, 1, len(body.Data))
	testutil.Equal(t, int64(100), body.Totals.APIRequests)
	testutil.Equal(t, int64(2000), body.Totals.StorageBytesUsed)
}

func TestHandleAdminOrgUsage_UnknownOrg(t *testing.T) {
	t.Parallel()

	store := &orgHandlerMockStore{} // getFn not set → returns ErrOrgNotFound
	querier := &mockOrgUsageQuerier{}
	h := handleAdminOrgUsage(store, querier)

	req := withURLParams(
		httptest.NewRequest(http.MethodGet, "/api/admin/orgs/missing/usage", nil),
		map[string]string{"orgId": "missing"},
	)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	testutil.Equal(t, http.StatusNotFound, w.Code)
}

func TestHandleAdminOrgUsage_EmptyUsage(t *testing.T) {
	t.Parallel()

	store := &orgHandlerMockStore{
		getFn: func(_ context.Context, id string) (*tenant.Organization, error) {
			return &tenant.Organization{ID: id, Name: "Acme"}, nil
		},
	}
	querier := &mockOrgUsageQuerier{rows: []tenant.TenantUsageDaily{}, tenantCount: 2}
	h := handleAdminOrgUsage(store, querier)

	req := withURLParams(
		httptest.NewRequest(http.MethodGet, "/api/admin/orgs/org-1/usage?period=week", nil),
		map[string]string{"orgId": "org-1"},
	)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)

	var body OrgUsageSummary
	testutil.NoError(t, json.NewDecoder(w.Body).Decode(&body))
	testutil.Equal(t, 0, len(body.Data))
	testutil.Equal(t, int64(0), body.Totals.APIRequests)
}

func TestHandleAdminOrgUsage_QuerierError(t *testing.T) {
	t.Parallel()

	store := &orgHandlerMockStore{
		getFn: func(_ context.Context, id string) (*tenant.Organization, error) {
			return &tenant.Organization{ID: id, Name: "Acme"}, nil
		},
	}
	querier := &mockOrgUsageQuerier{err: errors.New("db error")}
	h := handleAdminOrgUsage(store, querier)

	req := withURLParams(
		httptest.NewRequest(http.MethodGet, "/api/admin/orgs/org-1/usage?period=week", nil),
		map[string]string{"orgId": "org-1"},
	)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	testutil.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestHandleAdminOrgUsage_NilQuerier(t *testing.T) {
	t.Parallel()

	store := &orgHandlerMockStore{
		getFn: func(_ context.Context, id string) (*tenant.Organization, error) {
			return &tenant.Organization{ID: id, Name: "Acme"}, nil
		},
	}
	h := handleAdminOrgUsage(store, nil)

	req := withURLParams(
		httptest.NewRequest(http.MethodGet, "/api/admin/orgs/org-1/usage?period=week", nil),
		map[string]string{"orgId": "org-1"},
	)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	testutil.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestHandleAdminOrgUsage_UnknownOrgTakesPrecedenceOverMissingUsageService(t *testing.T) {
	t.Parallel()

	store := &orgHandlerMockStore{}
	h := handleAdminOrgUsage(store, nil)

	req := withURLParams(
		httptest.NewRequest(http.MethodGet, "/api/admin/orgs/missing/usage?period=week", nil),
		map[string]string{"orgId": "missing"},
	)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	testutil.Equal(t, http.StatusNotFound, w.Code)
}

func TestHandleAdminOrgUsage_MultiTenantAggregation(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	store := &orgHandlerMockStore{
		getFn: func(_ context.Context, id string) (*tenant.Organization, error) {
			return &tenant.Organization{ID: id, Name: "Acme"}, nil
		},
	}
	querier := &mockOrgUsageQuerier{
		rows: []tenant.TenantUsageDaily{
			{Date: now, RequestCount: 50, DBBytesUsed: 1000, BandwidthBytes: 200, FunctionInvocations: 5},
			{Date: now.AddDate(0, 0, 1), RequestCount: 75, DBBytesUsed: 1500, BandwidthBytes: 300, FunctionInvocations: 8},
		},
		tenantCount: 5,
	}
	h := handleAdminOrgUsage(store, querier)

	req := withURLParams(
		httptest.NewRequest(http.MethodGet, "/api/admin/orgs/org-1/usage?period=month", nil),
		map[string]string{"orgId": "org-1"},
	)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)

	var body OrgUsageSummary
	testutil.NoError(t, json.NewDecoder(w.Body).Decode(&body))
	testutil.Equal(t, 5, body.TenantCount)
	testutil.Equal(t, 2, len(body.Data))
	testutil.Equal(t, int64(125), body.Totals.APIRequests)
	testutil.Equal(t, int64(1500), body.Totals.StorageBytesUsed) // max of 1000, 1500
	testutil.Equal(t, int64(500), body.Totals.BandwidthBytes)
	testutil.Equal(t, int64(13), body.Totals.FunctionInvocations)
}

func TestBuildOrgUsageSummary_EmptyRows(t *testing.T) {
	t.Parallel()

	result := buildOrgUsageSummary("org-1", "week", nil, 0)
	testutil.Equal(t, "org-1", result.OrgID)
	testutil.Equal(t, 0, result.TenantCount)
	testutil.Equal(t, 0, len(result.Data))
	testutil.Equal(t, int64(0), result.Totals.APIRequests)
}

func TestHandleAdminOrgUsage_InvalidDateRange(t *testing.T) {
	t.Parallel()

	store := &orgHandlerMockStore{
		getFn: func(_ context.Context, id string) (*tenant.Organization, error) {
			return &tenant.Organization{ID: id, Name: "Acme"}, nil
		},
	}
	querier := &mockOrgUsageQuerier{}
	h := handleAdminOrgUsage(store, querier)

	req := withURLParams(
		httptest.NewRequest(http.MethodGet, "/api/admin/orgs/org-1/usage?from=2025-03-10", nil),
		map[string]string{"orgId": "org-1"},
	)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	testutil.Equal(t, http.StatusBadRequest, w.Code)
}
