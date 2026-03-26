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

type mockOrgAuditQuerier struct {
	events []tenant.TenantAuditEvent
	err    error
	lastQ  orgAuditQuery
}

func (m *mockOrgAuditQuerier) QueryOrgAuditEvents(_ context.Context, q orgAuditQuery) ([]tenant.TenantAuditEvent, error) {
	m.lastQ = q
	if m.err != nil {
		return nil, m.err
	}
	return m.events, nil
}

func TestHandleAdminOrgAudit_Success(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	store := &orgHandlerMockStore{
		getFn: func(_ context.Context, id string) (*tenant.Organization, error) {
			return &tenant.Organization{ID: id, Name: "Acme"}, nil
		},
	}
	querier := &mockOrgAuditQuerier{
		events: []tenant.TenantAuditEvent{
			{ID: "evt-1", TenantID: "t1", Action: "tenant.created", Result: "success", CreatedAt: now},
			{ID: "evt-2", TenantID: "t2", Action: "quota.change", Result: "success", CreatedAt: now},
		},
	}
	h := handleAdminOrgAudit(store, querier)

	req := withURLParams(
		httptest.NewRequest(http.MethodGet, "/api/admin/orgs/org-1/audit", nil),
		map[string]string{"orgId": "org-1"},
	)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)

	var body tenantAuditListResult
	testutil.NoError(t, json.NewDecoder(w.Body).Decode(&body))
	testutil.Equal(t, 2, body.Count)
	testutil.Equal(t, 50, body.Limit)
	testutil.Equal(t, 0, body.Offset)
}

func TestHandleAdminOrgAudit_UnknownOrg(t *testing.T) {
	t.Parallel()

	store := &orgHandlerMockStore{}
	querier := &mockOrgAuditQuerier{}
	h := handleAdminOrgAudit(store, querier)

	req := withURLParams(
		httptest.NewRequest(http.MethodGet, "/api/admin/orgs/missing/audit", nil),
		map[string]string{"orgId": "missing"},
	)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	testutil.Equal(t, http.StatusNotFound, w.Code)
}

func TestHandleAdminOrgAudit_EmptyEvents(t *testing.T) {
	t.Parallel()

	store := &orgHandlerMockStore{
		getFn: func(_ context.Context, id string) (*tenant.Organization, error) {
			return &tenant.Organization{ID: id, Name: "Acme"}, nil
		},
	}
	querier := &mockOrgAuditQuerier{events: []tenant.TenantAuditEvent{}}
	h := handleAdminOrgAudit(store, querier)

	req := withURLParams(
		httptest.NewRequest(http.MethodGet, "/api/admin/orgs/org-1/audit", nil),
		map[string]string{"orgId": "org-1"},
	)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)

	var body tenantAuditListResult
	testutil.NoError(t, json.NewDecoder(w.Body).Decode(&body))
	testutil.Equal(t, 0, body.Count)
}

func TestHandleAdminOrgAudit_WithFilters(t *testing.T) {
	t.Parallel()

	store := &orgHandlerMockStore{
		getFn: func(_ context.Context, id string) (*tenant.Organization, error) {
			return &tenant.Organization{ID: id, Name: "Acme"}, nil
		},
	}
	querier := &mockOrgAuditQuerier{events: []tenant.TenantAuditEvent{}}
	h := handleAdminOrgAudit(store, querier)

	req := withURLParams(
		httptest.NewRequest(http.MethodGet, "/api/admin/orgs/org-1/audit?action=tenant.created&result=success&limit=10&offset=5", nil),
		map[string]string{"orgId": "org-1"},
	)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)
	testutil.Equal(t, "org-1", querier.lastQ.OrgID)
	testutil.Equal(t, "tenant.created", querier.lastQ.Action)
	testutil.Equal(t, "success", querier.lastQ.Result)
	testutil.Equal(t, 10, querier.lastQ.Limit)
	testutil.Equal(t, 5, querier.lastQ.Offset)
}

func TestHandleAdminOrgAudit_InvalidFromTimestamp(t *testing.T) {
	t.Parallel()

	store := &orgHandlerMockStore{
		getFn: func(_ context.Context, id string) (*tenant.Organization, error) {
			return &tenant.Organization{ID: id, Name: "Acme"}, nil
		},
	}
	querier := &mockOrgAuditQuerier{}
	h := handleAdminOrgAudit(store, querier)

	req := withURLParams(
		httptest.NewRequest(http.MethodGet, "/api/admin/orgs/org-1/audit?from=bad-date", nil),
		map[string]string{"orgId": "org-1"},
	)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	testutil.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandleAdminOrgAudit_ToBeforeFrom(t *testing.T) {
	t.Parallel()

	store := &orgHandlerMockStore{
		getFn: func(_ context.Context, id string) (*tenant.Organization, error) {
			return &tenant.Organization{ID: id, Name: "Acme"}, nil
		},
	}
	querier := &mockOrgAuditQuerier{}
	h := handleAdminOrgAudit(store, querier)

	req := withURLParams(
		httptest.NewRequest(http.MethodGet, "/api/admin/orgs/org-1/audit?from=2025-03-15T00:00:00Z&to=2025-03-10T00:00:00Z", nil),
		map[string]string{"orgId": "org-1"},
	)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	testutil.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandleAdminOrgAudit_QuerierError(t *testing.T) {
	t.Parallel()

	store := &orgHandlerMockStore{
		getFn: func(_ context.Context, id string) (*tenant.Organization, error) {
			return &tenant.Organization{ID: id, Name: "Acme"}, nil
		},
	}
	querier := &mockOrgAuditQuerier{err: errors.New("db error")}
	h := handleAdminOrgAudit(store, querier)

	req := withURLParams(
		httptest.NewRequest(http.MethodGet, "/api/admin/orgs/org-1/audit", nil),
		map[string]string{"orgId": "org-1"},
	)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	testutil.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestHandleAdminOrgAudit_NilQuerier(t *testing.T) {
	t.Parallel()

	store := &orgHandlerMockStore{
		getFn: func(_ context.Context, id string) (*tenant.Organization, error) {
			return &tenant.Organization{ID: id, Name: "Acme"}, nil
		},
	}
	h := handleAdminOrgAudit(store, nil)

	req := withURLParams(
		httptest.NewRequest(http.MethodGet, "/api/admin/orgs/org-1/audit", nil),
		map[string]string{"orgId": "org-1"},
	)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	testutil.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestHandleAdminOrgAudit_InvalidActorID(t *testing.T) {
	t.Parallel()

	store := &orgHandlerMockStore{
		getFn: func(_ context.Context, id string) (*tenant.Organization, error) {
			return &tenant.Organization{ID: id, Name: "Acme"}, nil
		},
	}
	querier := &mockOrgAuditQuerier{}
	h := handleAdminOrgAudit(store, querier)

	req := withURLParams(
		httptest.NewRequest(http.MethodGet, "/api/admin/orgs/org-1/audit?actor_id=not-a-uuid", nil),
		map[string]string{"orgId": "org-1"},
	)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	testutil.Equal(t, http.StatusBadRequest, w.Code)
}

func TestParseOrgAuditFilters_Defaults(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/audit", nil)
	q, err := parseOrgAuditFilters(req, "org-1")
	testutil.NoError(t, err)
	testutil.Equal(t, "org-1", q.OrgID)
	testutil.Equal(t, 50, q.Limit)
	testutil.Equal(t, 0, q.Offset)
	testutil.Equal(t, "", q.Action)
	testutil.Equal(t, "", q.Result)
}

func TestParseOrgAuditFilters_LimitCapped(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/audit?limit=5000", nil)
	q, err := parseOrgAuditFilters(req, "org-1")
	testutil.NoError(t, err)
	testutil.Equal(t, 1000, q.Limit)
}

func TestParseOrgAuditFilters_InvalidLimit(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/audit?limit=abc", nil)
	_, err := parseOrgAuditFilters(req, "org-1")
	testutil.Error(t, err)
}

func TestParseOrgAuditFilters_NegativeLimit(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/audit?limit=-1", nil)
	_, err := parseOrgAuditFilters(req, "org-1")
	testutil.ErrorContains(t, err, "'limit' must be non-negative")
}

func TestParseOrgAuditFilters_NegativeOffset(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/audit?offset=-1", nil)
	_, err := parseOrgAuditFilters(req, "org-1")
	testutil.ErrorContains(t, err, "'offset' must be non-negative")
}
