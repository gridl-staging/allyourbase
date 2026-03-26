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

type teamHandlerMockStore struct {
	createFn func(ctx context.Context, orgID, name, slug string) (*tenant.Team, error)
	getFn    func(ctx context.Context, id string) (*tenant.Team, error)
	listFn   func(ctx context.Context, orgID string) ([]tenant.Team, error)
	updateFn func(ctx context.Context, id string, update tenant.TeamUpdate) (*tenant.Team, error)
	deleteFn func(ctx context.Context, id string) error
}

func (m *teamHandlerMockStore) CreateTeam(ctx context.Context, orgID, name, slug string) (*tenant.Team, error) {
	if m.createFn != nil {
		return m.createFn(ctx, orgID, name, slug)
	}
	return nil, errors.New("createFn not set")
}

func (m *teamHandlerMockStore) GetTeam(ctx context.Context, id string) (*tenant.Team, error) {
	if m.getFn != nil {
		return m.getFn(ctx, id)
	}
	return nil, tenant.ErrTeamNotFound
}

func (m *teamHandlerMockStore) ListTeams(ctx context.Context, orgID string) ([]tenant.Team, error) {
	if m.listFn != nil {
		return m.listFn(ctx, orgID)
	}
	return []tenant.Team{}, nil
}

func (m *teamHandlerMockStore) UpdateTeam(ctx context.Context, id string, update tenant.TeamUpdate) (*tenant.Team, error) {
	if m.updateFn != nil {
		return m.updateFn(ctx, id, update)
	}
	return nil, tenant.ErrTeamNotFound
}

func (m *teamHandlerMockStore) DeleteTeam(ctx context.Context, id string) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, id)
	}
	return tenant.ErrTeamNotFound
}

func newTeamHandlerTestStores() (*orgHandlerMockStore, *teamHandlerMockStore) {
	orgStore := &orgHandlerMockStore{getFn: func(_ context.Context, id string) (*tenant.Organization, error) {
		if id == "missing-org" {
			return nil, tenant.ErrOrgNotFound
		}
		return &tenant.Organization{ID: id}, nil
	}}
	teamStore := &teamHandlerMockStore{
		createFn: func(_ context.Context, orgID, name, slug string) (*tenant.Team, error) {
			if slug == "taken" {
				return nil, tenant.ErrTeamSlugTaken
			}
			return &tenant.Team{ID: "team-1", OrgID: orgID, Name: name, Slug: slug}, nil
		},
		getFn: func(_ context.Context, id string) (*tenant.Team, error) {
			if id == "missing" {
				return nil, tenant.ErrTeamNotFound
			}
			return &tenant.Team{ID: id, OrgID: "org-1", Name: "Core", Slug: "core"}, nil
		},
		listFn: func(_ context.Context, _ string) ([]tenant.Team, error) {
			return []tenant.Team{{ID: "team-1", OrgID: "org-1", Name: "Core", Slug: "core"}}, nil
		},
		updateFn: func(_ context.Context, id string, update tenant.TeamUpdate) (*tenant.Team, error) {
			if id == "missing" {
				return nil, tenant.ErrTeamNotFound
			}
			if update.Slug != nil && *update.Slug == "taken" {
				return nil, tenant.ErrTeamSlugTaken
			}
			return &tenant.Team{ID: id, OrgID: "org-1", Name: "Updated", Slug: "updated"}, nil
		},
		deleteFn: func(_ context.Context, id string) error {
			if id == "missing" {
				return tenant.ErrTeamNotFound
			}
			return nil
		},
	}
	return orgStore, teamStore
}

func TestTeamHandlerCreate(t *testing.T) {
	t.Parallel()
	orgStore, teamStore := newTeamHandlerTestStores()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		req := withURLParams(httptest.NewRequest(http.MethodPost, "/api/admin/orgs/org-1/teams", strings.NewReader(`{"name":"Core","slug":"core"}`)), map[string]string{"orgId": "org-1"})
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handleAdminCreateTeam(orgStore, teamStore).ServeHTTP(w, req)
		testutil.Equal(t, http.StatusCreated, w.Code)
	})

	t.Run("conflict", func(t *testing.T) {
		t.Parallel()
		req := withURLParams(httptest.NewRequest(http.MethodPost, "/api/admin/orgs/org-1/teams", strings.NewReader(`{"name":"Core","slug":"taken"}`)), map[string]string{"orgId": "org-1"})
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handleAdminCreateTeam(orgStore, teamStore).ServeHTTP(w, req)
		testutil.Equal(t, http.StatusConflict, w.Code)
	})

	t.Run("concurrent org deletion returns not found", func(t *testing.T) {
		t.Parallel()
		raceTeamStore := &teamHandlerMockStore{
			createFn: func(_ context.Context, _ string, _ string, _ string) (*tenant.Team, error) {
				return nil, tenant.ErrOrgNotFound
			},
		}
		req := withURLParams(httptest.NewRequest(http.MethodPost, "/api/admin/orgs/org-1/teams", strings.NewReader(`{"name":"Core","slug":"core"}`)), map[string]string{"orgId": "org-1"})
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handleAdminCreateTeam(orgStore, raceTeamStore).ServeHTTP(w, req)
		testutil.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestTeamHandlerList(t *testing.T) {
	t.Parallel()
	orgStore, teamStore := newTeamHandlerTestStores()
	req := withURLParams(httptest.NewRequest(http.MethodGet, "/api/admin/orgs/org-1/teams", nil), map[string]string{"orgId": "org-1"})
	w := httptest.NewRecorder()
	handleAdminListTeams(orgStore, teamStore).ServeHTTP(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)
}

func TestTeamHandlerListConcurrentOrgDeletion(t *testing.T) {
	t.Parallel()
	orgStore, _ := newTeamHandlerTestStores()
	raceTeamStore := &teamHandlerMockStore{
		listFn: func(_ context.Context, _ string) ([]tenant.Team, error) {
			return nil, tenant.ErrOrgNotFound
		},
	}
	req := withURLParams(httptest.NewRequest(http.MethodGet, "/api/admin/orgs/org-1/teams", nil), map[string]string{"orgId": "org-1"})
	w := httptest.NewRecorder()
	handleAdminListTeams(orgStore, raceTeamStore).ServeHTTP(w, req)
	testutil.Equal(t, http.StatusNotFound, w.Code)
}

func TestTeamHandlerGetNotFound(t *testing.T) {
	t.Parallel()
	orgStore, teamStore := newTeamHandlerTestStores()
	req := withURLParams(httptest.NewRequest(http.MethodGet, "/api/admin/orgs/org-1/teams/missing", nil), map[string]string{"orgId": "org-1", "teamId": "missing"})
	w := httptest.NewRecorder()
	handleAdminGetTeam(orgStore, teamStore).ServeHTTP(w, req)
	testutil.Equal(t, http.StatusNotFound, w.Code)
}

func TestTeamHandlerUpdate(t *testing.T) {
	t.Parallel()
	orgStore, teamStore := newTeamHandlerTestStores()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		req := withURLParams(httptest.NewRequest(http.MethodPut, "/api/admin/orgs/org-1/teams/team-1", strings.NewReader(`{"name":"Updated"}`)), map[string]string{"orgId": "org-1", "teamId": "team-1"})
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handleAdminUpdateTeam(orgStore, teamStore).ServeHTTP(w, req)
		testutil.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("conflict", func(t *testing.T) {
		t.Parallel()
		req := withURLParams(httptest.NewRequest(http.MethodPut, "/api/admin/orgs/org-1/teams/team-1", strings.NewReader(`{"slug":"taken"}`)), map[string]string{"orgId": "org-1", "teamId": "team-1"})
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handleAdminUpdateTeam(orgStore, teamStore).ServeHTTP(w, req)
		testutil.Equal(t, http.StatusConflict, w.Code)
	})
}

func TestTeamHandlerDeleteSuccess(t *testing.T) {
	t.Parallel()
	orgStore, teamStore := newTeamHandlerTestStores()
	req := withURLParams(httptest.NewRequest(http.MethodDelete, "/api/admin/orgs/org-1/teams/team-1", nil), map[string]string{"orgId": "org-1", "teamId": "team-1"})
	w := httptest.NewRecorder()
	handleAdminDeleteTeam(orgStore, teamStore).ServeHTTP(w, req)
	testutil.Equal(t, http.StatusNoContent, w.Code)
}
