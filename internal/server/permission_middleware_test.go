package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/tenant"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/golang-jwt/jwt/v5"
)

type permissionResolverTenantSource struct {
	membership *tenant.TenantMembership
	tenant     *tenant.Tenant
}

func (s *permissionResolverTenantSource) GetMembership(_ context.Context, _ string, _ string) (*tenant.TenantMembership, error) {
	if s.membership != nil {
		return s.membership, nil
	}
	return nil, tenant.ErrMembershipNotFound
}

func (s *permissionResolverTenantSource) GetTenant(_ context.Context, _ string) (*tenant.Tenant, error) {
	if s.tenant != nil {
		return s.tenant, nil
	}
	return &tenant.Tenant{ID: "tenant-1"}, nil
}

type permissionResolverOrgStore struct {
	membership *tenant.OrgMembership
}

func (s *permissionResolverOrgStore) AddOrgMembership(_ context.Context, _ string, _ string, _ string) (*tenant.OrgMembership, error) {
	return nil, tenant.ErrOrgMembershipExists
}

func (s *permissionResolverOrgStore) GetOrgMembership(_ context.Context, _ string, _ string) (*tenant.OrgMembership, error) {
	if s.membership != nil {
		return s.membership, nil
	}
	return nil, tenant.ErrOrgMembershipNotFound
}

func (s *permissionResolverOrgStore) ListOrgMemberships(_ context.Context, _ string) ([]tenant.OrgMembership, error) {
	return []tenant.OrgMembership{}, nil
}

func (s *permissionResolverOrgStore) ListUserOrgMemberships(_ context.Context, _ string) ([]tenant.OrgMembership, error) {
	return []tenant.OrgMembership{}, nil
}

func (s *permissionResolverOrgStore) RemoveOrgMembership(_ context.Context, _ string, _ string) error {
	return nil
}

func (s *permissionResolverOrgStore) UpdateOrgMembershipRole(_ context.Context, _ string, _ string, _ string) (*tenant.OrgMembership, error) {
	return nil, tenant.ErrOrgMembershipNotFound
}

type permissionResolverTeamMembershipStore struct{}

func (s *permissionResolverTeamMembershipStore) AddTeamMembership(_ context.Context, _ string, _ string, _ string) (*tenant.TeamMembership, error) {
	return nil, tenant.ErrTeamMembershipExists
}

func (s *permissionResolverTeamMembershipStore) GetTeamMembership(_ context.Context, _ string, _ string) (*tenant.TeamMembership, error) {
	return nil, tenant.ErrTeamMembershipNotFound
}

func (s *permissionResolverTeamMembershipStore) ListTeamMemberships(_ context.Context, _ string) ([]tenant.TeamMembership, error) {
	return []tenant.TeamMembership{}, nil
}

func (s *permissionResolverTeamMembershipStore) ListUserTeamMemberships(_ context.Context, _ string) ([]tenant.TeamMembership, error) {
	return []tenant.TeamMembership{}, nil
}

func (s *permissionResolverTeamMembershipStore) RemoveTeamMembership(_ context.Context, _ string, _ string) error {
	return nil
}

func (s *permissionResolverTeamMembershipStore) UpdateTeamMembershipRole(_ context.Context, _ string, _ string, _ string) (*tenant.TeamMembership, error) {
	return nil, tenant.ErrTeamMembershipNotFound
}

type permissionResolverTeamStore struct{}

func (s *permissionResolverTeamStore) CreateTeam(_ context.Context, _ string, _ string, _ string) (*tenant.Team, error) {
	return nil, tenant.ErrTeamSlugTaken
}

func (s *permissionResolverTeamStore) GetTeam(_ context.Context, _ string) (*tenant.Team, error) {
	return nil, tenant.ErrTeamNotFound
}

func (s *permissionResolverTeamStore) ListTeams(_ context.Context, _ string) ([]tenant.Team, error) {
	return []tenant.Team{}, nil
}

func (s *permissionResolverTeamStore) UpdateTeam(_ context.Context, _ string, _ tenant.TeamUpdate) (*tenant.Team, error) {
	return nil, tenant.ErrTeamNotFound
}

func (s *permissionResolverTeamStore) DeleteTeam(_ context.Context, _ string) error {
	return tenant.ErrTeamNotFound
}

func requestWithAuthAndTenant(userID, tenantID string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/api/data", nil)
	claims := &auth.Claims{RegisteredClaims: jwt.RegisteredClaims{Subject: userID}}
	ctx := auth.ContextWithClaims(req.Context(), claims)
	ctx = tenant.ContextWithTenantID(ctx, tenantID)
	return req.WithContext(ctx)
}

func TestRequireTenantPermission(t *testing.T) {
	t.Parallel()

	t.Run("direct membership grants access", func(t *testing.T) {
		t.Parallel()
		s := &Server{permResolver: tenant.NewPermissionResolver(
			&permissionResolverTenantSource{membership: &tenant.TenantMembership{Role: tenant.MemberRoleAdmin}},
			&permissionResolverOrgStore{},
			&permissionResolverTeamMembershipStore{},
			&permissionResolverTeamStore{},
		)}
		called := false
		h := s.requireTenantPermission(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
			perm := tenant.PermissionFromContext(r.Context())
			testutil.True(t, perm != nil)
			w.WriteHeader(http.StatusOK)
		}))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, requestWithAuthAndTenant("user-1", "tenant-1"))
		testutil.Equal(t, http.StatusOK, w.Code)
		testutil.True(t, called)
	})

	t.Run("org membership grants inherited access", func(t *testing.T) {
		t.Parallel()
		orgID := "org-1"
		s := &Server{permResolver: tenant.NewPermissionResolver(
			&permissionResolverTenantSource{tenant: &tenant.Tenant{ID: "tenant-1", OrgID: &orgID}},
			&permissionResolverOrgStore{membership: &tenant.OrgMembership{Role: tenant.OrgRoleOwner}},
			&permissionResolverTeamMembershipStore{},
			&permissionResolverTeamStore{},
		)}
		h := s.requireTenantPermission(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, requestWithAuthAndTenant("user-1", "tenant-1"))
		testutil.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("no membership returns forbidden", func(t *testing.T) {
		t.Parallel()
		s := &Server{permResolver: tenant.NewPermissionResolver(
			&permissionResolverTenantSource{tenant: &tenant.Tenant{ID: "tenant-1"}},
			&permissionResolverOrgStore{},
			&permissionResolverTeamMembershipStore{},
			&permissionResolverTeamStore{},
		)}
		h := s.requireTenantPermission(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, requestWithAuthAndTenant("user-1", "tenant-1"))
		testutil.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("nil resolver fails closed", func(t *testing.T) {
		t.Parallel()
		s := &Server{}
		h := s.requireTenantPermission(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, requestWithAuthAndTenant("user-1", "tenant-1"))
		testutil.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("admin token bypasses resolver", func(t *testing.T) {
		t.Parallel()
		authz := newAdminAuth("pw")
		s := &Server{adminAuth: authz, permResolver: tenant.NewPermissionResolver(
			&permissionResolverTenantSource{tenant: &tenant.Tenant{ID: "tenant-1"}},
			&permissionResolverOrgStore{},
			&permissionResolverTeamMembershipStore{},
			&permissionResolverTeamStore{},
		)}
		h := s.requireTenantPermission(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		req := requestWithAuthAndTenant("user-1", "tenant-1")
		req.Header.Set("Authorization", "Bearer "+authz.token())
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		testutil.Equal(t, http.StatusOK, w.Code)
	})
}
