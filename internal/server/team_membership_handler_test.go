package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/tenant"
	"github.com/allyourbase/ayb/internal/testutil"
)

const (
	testTeamID                   = "550e8400-e29b-41d4-a716-446655440021"
	testMissingTeamID            = "550e8400-e29b-41d4-a716-446655440099"
	testTeamMemberUserID         = "550e8400-e29b-41d4-a716-446655440031"
	testTeamMemberConflictUserID = "550e8400-e29b-41d4-a716-446655440032"
	testTeamMissingMemberUserID  = "550e8400-e29b-41d4-a716-446655440033"
	testNonOrgMemberUserID       = "550e8400-e29b-41d4-a716-446655440034"
)

type teamMembershipHandlerMockStore struct {
	addFn    func(ctx context.Context, teamID, userID, role string) (*tenant.TeamMembership, error)
	listFn   func(ctx context.Context, teamID string) ([]tenant.TeamMembership, error)
	updateFn func(ctx context.Context, teamID, userID, role string) (*tenant.TeamMembership, error)
	removeFn func(ctx context.Context, teamID, userID string) error
	getFn    func(ctx context.Context, teamID, userID string) (*tenant.TeamMembership, error)
}

func (m *teamMembershipHandlerMockStore) AddTeamMembership(ctx context.Context, teamID, userID, role string) (*tenant.TeamMembership, error) {
	if m.addFn != nil {
		return m.addFn(ctx, teamID, userID, role)
	}
	return nil, errors.New("addFn not set")
}

func (m *teamMembershipHandlerMockStore) GetTeamMembership(ctx context.Context, teamID, userID string) (*tenant.TeamMembership, error) {
	if m.getFn != nil {
		return m.getFn(ctx, teamID, userID)
	}
	return nil, tenant.ErrTeamMembershipNotFound
}

func (m *teamMembershipHandlerMockStore) ListTeamMemberships(ctx context.Context, teamID string) ([]tenant.TeamMembership, error) {
	if m.listFn != nil {
		return m.listFn(ctx, teamID)
	}
	return []tenant.TeamMembership{}, nil
}

func (m *teamMembershipHandlerMockStore) ListUserTeamMemberships(_ context.Context, _ string) ([]tenant.TeamMembership, error) {
	return []tenant.TeamMembership{}, nil
}

func (m *teamMembershipHandlerMockStore) RemoveTeamMembership(ctx context.Context, teamID, userID string) error {
	if m.removeFn != nil {
		return m.removeFn(ctx, teamID, userID)
	}
	return nil
}

func (m *teamMembershipHandlerMockStore) UpdateTeamMembershipRole(ctx context.Context, teamID, userID, role string) (*tenant.TeamMembership, error) {
	if m.updateFn != nil {
		return m.updateFn(ctx, teamID, userID, role)
	}
	return &tenant.TeamMembership{TeamID: teamID, UserID: userID, Role: role}, nil
}

func TestTeamMembershipHandlers(t *testing.T) {
	t.Parallel()

	teamStore := &teamHandlerMockStore{
		getFn: func(_ context.Context, id string) (*tenant.Team, error) {
			if id == testMissingTeamID {
				return nil, tenant.ErrTeamNotFound
			}
			return &tenant.Team{ID: id, OrgID: testOrgID, Name: "Core", Slug: "core"}, nil
		},
	}
	orgMembershipStore := &orgMembershipHandlerMockStore{
		getFn: func(_ context.Context, _ string, userID string) (*tenant.OrgMembership, error) {
			if userID == testNonOrgMemberUserID {
				return nil, tenant.ErrOrgMembershipNotFound
			}
			return &tenant.OrgMembership{OrgID: testOrgID, UserID: userID, Role: tenant.OrgRoleMember}, nil
		},
	}
	teamMembershipStore := &teamMembershipHandlerMockStore{
		addFn: func(_ context.Context, teamID, userID, role string) (*tenant.TeamMembership, error) {
			if userID == testTeamMemberConflictUserID {
				return nil, tenant.ErrTeamMembershipExists
			}
			return &tenant.TeamMembership{TeamID: teamID, UserID: userID, Role: role}, nil
		},
		listFn: func(_ context.Context, teamID string) ([]tenant.TeamMembership, error) {
			return []tenant.TeamMembership{{TeamID: teamID, UserID: "u1", Role: tenant.TeamRoleMember}}, nil
		},
		updateFn: func(_ context.Context, teamID, userID, role string) (*tenant.TeamMembership, error) {
			if userID == testTeamMissingMemberUserID {
				return nil, tenant.ErrTeamMembershipNotFound
			}
			return &tenant.TeamMembership{TeamID: teamID, UserID: userID, Role: role}, nil
		},
		removeFn: func(_ context.Context, _ string, userID string) error {
			if userID == testTeamMissingMemberUserID {
				return tenant.ErrTeamMembershipNotFound
			}
			return nil
		},
	}

	t.Run("add success", func(t *testing.T) {
		t.Parallel()
		req := withURLParams(httptest.NewRequest(http.MethodPost, "/api/admin/orgs/org-1/teams/team-1/members", strings.NewReader(`{"userId":"`+testTeamMemberUserID+`","role":"member"}`)), map[string]string{"orgId": testOrgID, "teamId": testTeamID})
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handleAdminAddTeamMember(teamMembershipStore, orgMembershipStore, teamStore).ServeHTTP(w, req)
		testutil.Equal(t, http.StatusCreated, w.Code)
	})

	t.Run("add invalid role", func(t *testing.T) {
		t.Parallel()
		req := withURLParams(httptest.NewRequest(http.MethodPost, "/api/admin/orgs/org-1/teams/team-1/members", strings.NewReader(`{"userId":"`+testTeamMemberUserID+`","role":"invalid"}`)), map[string]string{"orgId": testOrgID, "teamId": testTeamID})
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handleAdminAddTeamMember(teamMembershipStore, orgMembershipStore, teamStore).ServeHTTP(w, req)
		testutil.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("add invalid user id format", func(t *testing.T) {
		t.Parallel()
		req := withURLParams(httptest.NewRequest(http.MethodPost, "/api/admin/orgs/org-1/teams/team-1/members", strings.NewReader(`{"userId":"not-a-uuid","role":"member"}`)), map[string]string{"orgId": testOrgID, "teamId": testTeamID})
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handleAdminAddTeamMember(teamMembershipStore, orgMembershipStore, teamStore).ServeHTTP(w, req)
		testutil.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("add invalid org id format", func(t *testing.T) {
		t.Parallel()
		req := withURLParams(httptest.NewRequest(http.MethodPost, "/api/admin/orgs/not-a-uuid/teams/team-1/members", strings.NewReader(`{"userId":"`+testTeamMemberUserID+`","role":"member"}`)), map[string]string{"orgId": "not-a-uuid", "teamId": testTeamID})
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handleAdminAddTeamMember(teamMembershipStore, orgMembershipStore, teamStore).ServeHTTP(w, req)
		testutil.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("add invalid team id format", func(t *testing.T) {
		t.Parallel()
		req := withURLParams(httptest.NewRequest(http.MethodPost, "/api/admin/orgs/org-1/teams/not-a-uuid/members", strings.NewReader(`{"userId":"`+testTeamMemberUserID+`","role":"member"}`)), map[string]string{"orgId": testOrgID, "teamId": "not-a-uuid"})
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handleAdminAddTeamMember(teamMembershipStore, orgMembershipStore, teamStore).ServeHTTP(w, req)
		testutil.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("add conflict", func(t *testing.T) {
		t.Parallel()
		req := withURLParams(httptest.NewRequest(http.MethodPost, "/api/admin/orgs/org-1/teams/team-1/members", strings.NewReader(`{"userId":"`+testTeamMemberConflictUserID+`","role":"member"}`)), map[string]string{"orgId": testOrgID, "teamId": testTeamID})
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handleAdminAddTeamMember(teamMembershipStore, orgMembershipStore, teamStore).ServeHTTP(w, req)
		testutil.Equal(t, http.StatusConflict, w.Code)
	})

	t.Run("list success", func(t *testing.T) {
		t.Parallel()
		req := withURLParams(httptest.NewRequest(http.MethodGet, "/api/admin/orgs/org-1/teams/team-1/members", nil), map[string]string{"orgId": testOrgID, "teamId": testTeamID})
		w := httptest.NewRecorder()
		handleAdminListTeamMembers(teamMembershipStore, teamStore).ServeHTTP(w, req)
		testutil.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("list invalid org id format", func(t *testing.T) {
		t.Parallel()
		req := withURLParams(httptest.NewRequest(http.MethodGet, "/api/admin/orgs/not-a-uuid/teams/team-1/members", nil), map[string]string{"orgId": "not-a-uuid", "teamId": testTeamID})
		w := httptest.NewRecorder()
		handleAdminListTeamMembers(teamMembershipStore, teamStore).ServeHTTP(w, req)
		testutil.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("list invalid team id format", func(t *testing.T) {
		t.Parallel()
		req := withURLParams(httptest.NewRequest(http.MethodGet, "/api/admin/orgs/org-1/teams/not-a-uuid/members", nil), map[string]string{"orgId": testOrgID, "teamId": "not-a-uuid"})
		w := httptest.NewRecorder()
		handleAdminListTeamMembers(teamMembershipStore, teamStore).ServeHTTP(w, req)
		testutil.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("update not found", func(t *testing.T) {
		t.Parallel()
		req := withURLParams(httptest.NewRequest(http.MethodPut, "/api/admin/orgs/org-1/teams/team-1/members/missing/role", strings.NewReader(`{"role":"lead"}`)), map[string]string{"orgId": testOrgID, "teamId": testTeamID, "userId": testTeamMissingMemberUserID})
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handleAdminUpdateTeamMemberRole(teamMembershipStore, teamStore).ServeHTTP(w, req)
		testutil.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("update invalid user id format", func(t *testing.T) {
		t.Parallel()
		req := withURLParams(httptest.NewRequest(http.MethodPut, "/api/admin/orgs/org-1/teams/team-1/members/not-a-uuid/role", strings.NewReader(`{"role":"lead"}`)), map[string]string{"orgId": testOrgID, "teamId": testTeamID, "userId": "not-a-uuid"})
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handleAdminUpdateTeamMemberRole(teamMembershipStore, teamStore).ServeHTTP(w, req)
		testutil.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("remove not found", func(t *testing.T) {
		t.Parallel()
		req := withURLParams(httptest.NewRequest(http.MethodDelete, "/api/admin/orgs/org-1/teams/team-1/members/missing", nil), map[string]string{"orgId": testOrgID, "teamId": testTeamID, "userId": testTeamMissingMemberUserID})
		w := httptest.NewRecorder()
		handleAdminRemoveTeamMember(teamMembershipStore, teamStore).ServeHTTP(w, req)
		testutil.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("remove invalid user id format", func(t *testing.T) {
		t.Parallel()
		req := withURLParams(httptest.NewRequest(http.MethodDelete, "/api/admin/orgs/org-1/teams/team-1/members/not-a-uuid", nil), map[string]string{"orgId": testOrgID, "teamId": testTeamID, "userId": "not-a-uuid"})
		w := httptest.NewRecorder()
		handleAdminRemoveTeamMember(teamMembershipStore, teamStore).ServeHTTP(w, req)
		testutil.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestTeamMembershipHandlersConcurrentTeamDeletion(t *testing.T) {
	t.Parallel()

	teamStore := &teamHandlerMockStore{
		getFn: func(_ context.Context, id string) (*tenant.Team, error) {
			return &tenant.Team{ID: id, OrgID: testOrgID, Name: "Core", Slug: "core"}, nil
		},
	}
	orgMembershipStore := &orgMembershipHandlerMockStore{
		getFn: func(_ context.Context, _ string, userID string) (*tenant.OrgMembership, error) {
			return &tenant.OrgMembership{OrgID: testOrgID, UserID: userID, Role: tenant.OrgRoleMember}, nil
		},
	}
	teamMembershipStore := &teamMembershipHandlerMockStore{
		addFn: func(_ context.Context, _ string, _ string, _ string) (*tenant.TeamMembership, error) {
			return nil, tenant.ErrTeamNotFound
		},
		listFn: func(_ context.Context, _ string) ([]tenant.TeamMembership, error) {
			return nil, tenant.ErrTeamNotFound
		},
		updateFn: func(_ context.Context, _ string, _ string, _ string) (*tenant.TeamMembership, error) {
			return nil, tenant.ErrTeamNotFound
		},
		removeFn: func(_ context.Context, _ string, _ string) error {
			return tenant.ErrTeamNotFound
		},
	}

	t.Run("add returns team 404", func(t *testing.T) {
		t.Parallel()
		req := withURLParams(httptest.NewRequest(http.MethodPost, "/api/admin/orgs/org-1/teams/team-1/members", strings.NewReader(`{"userId":"`+testTeamMemberUserID+`","role":"member"}`)), map[string]string{"orgId": testOrgID, "teamId": testTeamID})
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handleAdminAddTeamMember(teamMembershipStore, orgMembershipStore, teamStore).ServeHTTP(w, req)
		testutil.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("list returns team 404", func(t *testing.T) {
		t.Parallel()
		req := withURLParams(httptest.NewRequest(http.MethodGet, "/api/admin/orgs/org-1/teams/team-1/members", nil), map[string]string{"orgId": testOrgID, "teamId": testTeamID})
		w := httptest.NewRecorder()
		handleAdminListTeamMembers(teamMembershipStore, teamStore).ServeHTTP(w, req)
		testutil.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("update returns team 404", func(t *testing.T) {
		t.Parallel()
		req := withURLParams(httptest.NewRequest(http.MethodPut, "/api/admin/orgs/org-1/teams/team-1/members/u1/role", strings.NewReader(`{"role":"lead"}`)), map[string]string{"orgId": testOrgID, "teamId": testTeamID, "userId": testTeamMemberUserID})
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handleAdminUpdateTeamMemberRole(teamMembershipStore, teamStore).ServeHTTP(w, req)
		testutil.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("remove returns team 404", func(t *testing.T) {
		t.Parallel()
		req := withURLParams(httptest.NewRequest(http.MethodDelete, "/api/admin/orgs/org-1/teams/team-1/members/u1", nil), map[string]string{"orgId": testOrgID, "teamId": testTeamID, "userId": testTeamMemberUserID})
		w := httptest.NewRecorder()
		handleAdminRemoveTeamMember(teamMembershipStore, teamStore).ServeHTTP(w, req)
		testutil.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestTeamMembershipHandlersConcurrentOrgDeletion(t *testing.T) {
	t.Parallel()

	teamStore := &teamHandlerMockStore{
		getFn: func(_ context.Context, id string) (*tenant.Team, error) {
			return &tenant.Team{ID: id, OrgID: testOrgID, Name: "Core", Slug: "core"}, nil
		},
	}
	orgMembershipStore := &orgMembershipHandlerMockStore{
		getFn: func(_ context.Context, _ string, _ string) (*tenant.OrgMembership, error) {
			return nil, tenant.ErrOrgNotFound
		},
	}
	teamMembershipStore := &teamMembershipHandlerMockStore{}

	req := withURLParams(
		httptest.NewRequest(
			http.MethodPost,
			"/api/admin/orgs/org-1/teams/team-1/members",
			strings.NewReader(`{"userId":"`+testTeamMemberUserID+`","role":"member"}`),
		),
		map[string]string{"orgId": testOrgID, "teamId": testTeamID},
	)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handleAdminAddTeamMember(teamMembershipStore, orgMembershipStore, teamStore).ServeHTTP(w, req)

	testutil.Equal(t, http.StatusNotFound, w.Code)
}
