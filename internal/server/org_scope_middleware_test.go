package server

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/tenant"
	"github.com/allyourbase/ayb/internal/testutil"
)

func TestEnforceOrgScopeAccess_NoClaims(t *testing.T) {
	t.Parallel()

	s := &Server{tenantSvc: &mockTenantService{}}
	h := s.enforceOrgScopeAccess(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/data", nil)
	ctx := tenant.ContextWithTenantID(req.Context(), "tenant-1")
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)
}

func TestEnforceOrgScopeAccess_NoOrgID(t *testing.T) {
	t.Parallel()

	s := &Server{tenantSvc: &mockTenantService{}}
	h := s.enforceOrgScopeAccess(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/data", nil)
	ctx := tenant.ContextWithTenantID(req.Context(), "tenant-1")
	ctx = auth.ContextWithClaims(ctx, &auth.Claims{OrgID: ""})
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)
}

func TestEnforceOrgScopeAccess_NoTenantID(t *testing.T) {
	t.Parallel()

	s := &Server{tenantSvc: &mockTenantService{}}
	h := s.enforceOrgScopeAccess(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/data", nil)
	ctx := auth.ContextWithClaims(req.Context(), &auth.Claims{OrgID: "org-1"})
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)
}

func TestEnforceOrgScopeAccess_MatchingOrg(t *testing.T) {
	t.Parallel()

	orgID := "org-1"
	s := &Server{tenantSvc: &mockTenantService{
		tenant: &tenant.Tenant{ID: "tenant-1", OrgID: &orgID},
	}}
	h := s.enforceOrgScopeAccess(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/data", nil)
	ctx := tenant.ContextWithTenantID(req.Context(), "tenant-1")
	ctx = auth.ContextWithClaims(ctx, &auth.Claims{OrgID: "org-1"})
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)
}

func TestEnforceOrgScopeAccess_MismatchedOrg(t *testing.T) {
	t.Parallel()

	otherOrg := "org-2"
	s := &Server{tenantSvc: &mockTenantService{
		tenant: &tenant.Tenant{ID: "tenant-1", OrgID: &otherOrg},
	}}
	h := s.enforceOrgScopeAccess(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/data", nil)
	ctx := tenant.ContextWithTenantID(req.Context(), "tenant-1")
	ctx = auth.ContextWithClaims(ctx, &auth.Claims{OrgID: "org-1"})
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	testutil.Equal(t, http.StatusForbidden, w.Code)
}

func TestEnforceOrgScopeAccess_TenantHasNoOrg(t *testing.T) {
	t.Parallel()

	s := &Server{tenantSvc: &mockTenantService{
		tenant: &tenant.Tenant{ID: "tenant-1", OrgID: nil},
	}}
	h := s.enforceOrgScopeAccess(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/data", nil)
	ctx := tenant.ContextWithTenantID(req.Context(), "tenant-1")
	ctx = auth.ContextWithClaims(ctx, &auth.Claims{OrgID: "org-1"})
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	testutil.Equal(t, http.StatusForbidden, w.Code)
}

func TestEnforceOrgScopeAccess_TenantLookupError(t *testing.T) {
	t.Parallel()

	s := &Server{tenantSvc: &mockTenantService{
		err: tenant.ErrTenantNotFound,
	}}
	h := s.enforceOrgScopeAccess(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/data", nil)
	ctx := tenant.ContextWithTenantID(req.Context(), "tenant-1")
	ctx = auth.ContextWithClaims(ctx, &auth.Claims{OrgID: "org-1"})
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	testutil.Equal(t, http.StatusForbidden, w.Code)
}

func TestEnforceOrgScopeAccess_TenantLookupInfrastructureError(t *testing.T) {
	t.Parallel()

	s := &Server{tenantSvc: &mockTenantService{
		err: errors.New("db unavailable"),
	}}
	h := s.enforceOrgScopeAccess(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/data", nil)
	ctx := tenant.ContextWithTenantID(req.Context(), "tenant-1")
	ctx = auth.ContextWithClaims(ctx, &auth.Claims{OrgID: "org-1"})
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	testutil.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestEnforceOrgScopeAccess_NoTenantSvc(t *testing.T) {
	t.Parallel()

	s := &Server{tenantSvc: nil}
	h := s.enforceOrgScopeAccess(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/data", nil)
	ctx := tenant.ContextWithTenantID(req.Context(), "tenant-1")
	ctx = auth.ContextWithClaims(ctx, &auth.Claims{OrgID: "org-1"})
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	testutil.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestEnforceOrgScopeAccess_LegacyKeyPassesThrough(t *testing.T) {
	t.Parallel()

	s := &Server{tenantSvc: &mockTenantService{}}
	h := s.enforceOrgScopeAccess(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/data", nil)
	ctx := tenant.ContextWithTenantID(req.Context(), "tenant-1")
	ctx = auth.ContextWithClaims(ctx, &auth.Claims{AppID: "app-1"})
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)
}
