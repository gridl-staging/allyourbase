package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/tenant"
	"github.com/allyourbase/ayb/internal/testutil"
)

type orgHandlerMockStore struct {
	createFn    func(ctx context.Context, name, slug string, parentOrgID *string, planTier string) (*tenant.Organization, error)
	getFn       func(ctx context.Context, id string) (*tenant.Organization, error)
	getBySlugFn func(ctx context.Context, slug string) (*tenant.Organization, error)
	listFn      func(ctx context.Context, userID string) ([]tenant.Organization, error)
	listChildFn func(ctx context.Context, parentOrgID string) ([]tenant.Organization, error)
	updateFn    func(ctx context.Context, id string, update tenant.OrgUpdate) (*tenant.Organization, error)
	deleteFn    func(ctx context.Context, id string) error
}

func (m *orgHandlerMockStore) CreateOrg(ctx context.Context, name, slug string, parentOrgID *string, planTier string) (*tenant.Organization, error) {
	if m.createFn != nil {
		return m.createFn(ctx, name, slug, parentOrgID, planTier)
	}
	return nil, errors.New("createFn not set")
}

func (m *orgHandlerMockStore) GetOrg(ctx context.Context, id string) (*tenant.Organization, error) {
	if m.getFn != nil {
		return m.getFn(ctx, id)
	}
	return nil, tenant.ErrOrgNotFound
}

func (m *orgHandlerMockStore) GetOrgBySlug(ctx context.Context, slug string) (*tenant.Organization, error) {
	if m.getBySlugFn != nil {
		return m.getBySlugFn(ctx, slug)
	}
	return nil, tenant.ErrOrgNotFound
}

func (m *orgHandlerMockStore) ListOrgs(ctx context.Context, userID string) ([]tenant.Organization, error) {
	if m.listFn != nil {
		return m.listFn(ctx, userID)
	}
	return []tenant.Organization{}, nil
}

func (m *orgHandlerMockStore) ListChildOrgs(ctx context.Context, parentOrgID string) ([]tenant.Organization, error) {
	if m.listChildFn != nil {
		return m.listChildFn(ctx, parentOrgID)
	}
	return []tenant.Organization{}, nil
}

func (m *orgHandlerMockStore) UpdateOrg(ctx context.Context, id string, update tenant.OrgUpdate) (*tenant.Organization, error) {
	if m.updateFn != nil {
		return m.updateFn(ctx, id, update)
	}
	return nil, tenant.ErrOrgNotFound
}

func (m *orgHandlerMockStore) DeleteOrg(ctx context.Context, id string) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, id)
	}
	return tenant.ErrOrgNotFound
}

type orgHandlerMockTeamStore struct {
	listFn func(ctx context.Context, orgID string) ([]tenant.Team, error)
}

func (m *orgHandlerMockTeamStore) CreateTeam(_ context.Context, _ string, _ string, _ string) (*tenant.Team, error) {
	return nil, errors.New("not implemented")
}

func (m *orgHandlerMockTeamStore) GetTeam(_ context.Context, _ string) (*tenant.Team, error) {
	return nil, errors.New("not implemented")
}

func (m *orgHandlerMockTeamStore) ListTeams(ctx context.Context, orgID string) ([]tenant.Team, error) {
	if m.listFn != nil {
		return m.listFn(ctx, orgID)
	}
	return []tenant.Team{}, nil
}

func (m *orgHandlerMockTeamStore) UpdateTeam(_ context.Context, _ string, _ tenant.TeamUpdate) (*tenant.Team, error) {
	return nil, errors.New("not implemented")
}

func (m *orgHandlerMockTeamStore) DeleteTeam(_ context.Context, _ string) error {
	return errors.New("not implemented")
}

func TestHandleAdminCreateOrg(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	store := &orgHandlerMockStore{
		createFn: func(_ context.Context, name, slug string, _ *string, _ string) (*tenant.Organization, error) {
			return &tenant.Organization{ID: "org-1", Name: name, Slug: slug, PlanTier: "free", CreatedAt: now, UpdatedAt: now}, nil
		},
	}
	h := handleAdminCreateOrg(store)

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodPost, "/api/admin/orgs", strings.NewReader(`{"name":"Acme","slug":"acme"}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		testutil.Equal(t, http.StatusCreated, w.Code)
	})

	t.Run("slug conflict", func(t *testing.T) {
		t.Parallel()
		conflictStore := &orgHandlerMockStore{createFn: func(_ context.Context, _ string, _ string, _ *string, _ string) (*tenant.Organization, error) {
			return nil, tenant.ErrOrgSlugTaken
		}}
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/admin/orgs", strings.NewReader(`{"name":"Acme","slug":"acme"}`))
		req.Header.Set("Content-Type", "application/json")
		handleAdminCreateOrg(conflictStore).ServeHTTP(w, req)
		testutil.Equal(t, http.StatusConflict, w.Code)
	})

	t.Run("invalid slug", func(t *testing.T) {
		t.Parallel()
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/admin/orgs", strings.NewReader(`{"name":"Acme","slug":"Bad!!"}`))
		req.Header.Set("Content-Type", "application/json")
		h.ServeHTTP(w, req)
		testutil.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("missing name", func(t *testing.T) {
		t.Parallel()
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/admin/orgs", strings.NewReader(`{"slug":"acme"}`))
		req.Header.Set("Content-Type", "application/json")
		h.ServeHTTP(w, req)
		testutil.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("missing parent org", func(t *testing.T) {
		t.Parallel()
		parentLookupStore := &orgHandlerMockStore{
			createFn: func(_ context.Context, _ string, _ string, _ *string, _ string) (*tenant.Organization, error) {
				t.Fatal("create should not be called when parent org is missing")
				return nil, nil
			},
			getFn: func(_ context.Context, id string) (*tenant.Organization, error) {
				if id == "missing-parent" {
					return nil, tenant.ErrOrgNotFound
				}
				return &tenant.Organization{ID: id}, nil
			},
		}
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/admin/orgs", strings.NewReader(`{"name":"Acme","slug":"acme","parentOrgId":"missing-parent"}`))
		req.Header.Set("Content-Type", "application/json")
		handleAdminCreateOrg(parentLookupStore).ServeHTTP(w, req)
		testutil.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("concurrent parent deletion returns not found", func(t *testing.T) {
		t.Parallel()
		raceStore := &orgHandlerMockStore{
			getFn: func(_ context.Context, id string) (*tenant.Organization, error) {
				return &tenant.Organization{ID: id}, nil
			},
			createFn: func(_ context.Context, _ string, _ string, _ *string, _ string) (*tenant.Organization, error) {
				return nil, tenant.ErrParentOrgNotFound
			},
		}
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/admin/orgs", strings.NewReader(`{"name":"Acme","slug":"acme","parentOrgId":"parent-1"}`))
		req.Header.Set("Content-Type", "application/json")
		handleAdminCreateOrg(raceStore).ServeHTTP(w, req)
		testutil.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestHandleAdminGetOrg(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	store := &orgHandlerMockStore{
		getFn: func(_ context.Context, id string) (*tenant.Organization, error) {
			if id == "missing" {
				return nil, tenant.ErrOrgNotFound
			}
			return &tenant.Organization{ID: id, Name: "Acme", Slug: "acme", PlanTier: "free", CreatedAt: now, UpdatedAt: now}, nil
		},
		listChildFn: func(_ context.Context, _ string) ([]tenant.Organization, error) {
			return []tenant.Organization{{ID: "child-1"}, {ID: "child-2"}}, nil
		},
	}
	teamStore := &orgHandlerMockTeamStore{listFn: func(_ context.Context, _ string) ([]tenant.Team, error) {
		return []tenant.Team{{ID: "team-1"}}, nil
	}}
	svc := &mockTenantService{orgTenants: []tenant.Tenant{{ID: "tenant-1"}, {ID: "tenant-2"}}}

	t.Run("with counts", func(t *testing.T) {
		t.Parallel()
		w := httptest.NewRecorder()
		req := withURLParams(httptest.NewRequest(http.MethodGet, "/api/admin/orgs/org-1", nil), map[string]string{"orgId": "org-1"})
		handleAdminGetOrg(store, teamStore, svc).ServeHTTP(w, req)
		testutil.Equal(t, http.StatusOK, w.Code)

		var body map[string]any
		testutil.NoError(t, json.NewDecoder(w.Body).Decode(&body))
		childOrgCount, childOrgCountOK := body["childOrgCount"].(float64)
		teamCount, teamCountOK := body["teamCount"].(float64)
		tenantCount, tenantCountOK := body["tenantCount"].(float64)
		testutil.True(t, childOrgCountOK)
		testutil.True(t, teamCountOK)
		testutil.True(t, tenantCountOK)
		testutil.Equal(t, float64(2), childOrgCount)
		testutil.Equal(t, float64(1), teamCount)
		testutil.Equal(t, float64(2), tenantCount)
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()
		w := httptest.NewRecorder()
		req := withURLParams(httptest.NewRequest(http.MethodGet, "/api/admin/orgs/missing", nil), map[string]string{"orgId": "missing"})
		handleAdminGetOrg(store, teamStore, svc).ServeHTTP(w, req)
		testutil.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("concurrent org deletion while listing teams returns not found", func(t *testing.T) {
		t.Parallel()
		raceTeamStore := &orgHandlerMockTeamStore{listFn: func(_ context.Context, _ string) ([]tenant.Team, error) {
			return nil, tenant.ErrOrgNotFound
		}}
		w := httptest.NewRecorder()
		req := withURLParams(httptest.NewRequest(http.MethodGet, "/api/admin/orgs/org-1", nil), map[string]string{"orgId": "org-1"})
		handleAdminGetOrg(store, raceTeamStore, svc).ServeHTTP(w, req)
		testutil.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestHandleAdminListOrgs(t *testing.T) {
	t.Parallel()

	store := &orgHandlerMockStore{
		listFn: func(_ context.Context, userID string) ([]tenant.Organization, error) {
			testutil.Equal(t, "", userID)
			return []tenant.Organization{{ID: "org-1", Name: "Acme", Slug: "acme"}}, nil
		},
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/orgs", nil)
	handleAdminListOrgs(store).ServeHTTP(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)
}

func TestHandleAdminUpdateOrg(t *testing.T) {
	t.Parallel()

	store := &orgHandlerMockStore{
		updateFn: func(_ context.Context, id string, update tenant.OrgUpdate) (*tenant.Organization, error) {
			if id == "missing" {
				return nil, tenant.ErrOrgNotFound
			}
			if update.Slug != nil && *update.Slug == "taken" {
				return nil, tenant.ErrOrgSlugTaken
			}
			return &tenant.Organization{ID: id, Name: "Updated", Slug: "updated"}, nil
		},
	}

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		w := httptest.NewRecorder()
		req := withURLParams(httptest.NewRequest(http.MethodPut, "/api/admin/orgs/org-1", strings.NewReader(`{"name":"Updated"}`)), map[string]string{"orgId": "org-1"})
		req.Header.Set("Content-Type", "application/json")
		handleAdminUpdateOrg(store).ServeHTTP(w, req)
		testutil.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("slug conflict", func(t *testing.T) {
		t.Parallel()
		w := httptest.NewRecorder()
		req := withURLParams(httptest.NewRequest(http.MethodPut, "/api/admin/orgs/org-1", strings.NewReader(`{"slug":"taken"}`)), map[string]string{"orgId": "org-1"})
		req.Header.Set("Content-Type", "application/json")
		handleAdminUpdateOrg(store).ServeHTTP(w, req)
		testutil.Equal(t, http.StatusConflict, w.Code)
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()
		w := httptest.NewRecorder()
		req := withURLParams(httptest.NewRequest(http.MethodPut, "/api/admin/orgs/missing", strings.NewReader(`{"name":"Updated"}`)), map[string]string{"orgId": "missing"})
		req.Header.Set("Content-Type", "application/json")
		handleAdminUpdateOrg(store).ServeHTTP(w, req)
		testutil.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("circular parent", func(t *testing.T) {
		t.Parallel()
		cycleStore := &orgHandlerMockStore{
			getFn: func(_ context.Context, id string) (*tenant.Organization, error) {
				return &tenant.Organization{ID: id}, nil
			},
			updateFn: func(_ context.Context, _ string, _ tenant.OrgUpdate) (*tenant.Organization, error) {
				return nil, tenant.ErrCircularParentOrg
			},
		}
		w := httptest.NewRecorder()
		req := withURLParams(httptest.NewRequest(http.MethodPut, "/api/admin/orgs/org-1", strings.NewReader(`{"parentOrgId":"parent-1"}`)), map[string]string{"orgId": "org-1"})
		req.Header.Set("Content-Type", "application/json")
		handleAdminUpdateOrg(cycleStore).ServeHTTP(w, req)
		testutil.Equal(t, http.StatusConflict, w.Code)
	})

	t.Run("missing parent org", func(t *testing.T) {
		t.Parallel()
		parentLookupStore := &orgHandlerMockStore{
			getFn: func(_ context.Context, id string) (*tenant.Organization, error) {
				if id == "missing-parent" {
					return nil, tenant.ErrOrgNotFound
				}
				return &tenant.Organization{ID: id}, nil
			},
			updateFn: func(_ context.Context, _ string, _ tenant.OrgUpdate) (*tenant.Organization, error) {
				t.Fatal("update should not be called when parent org is missing")
				return nil, nil
			},
		}
		w := httptest.NewRecorder()
		req := withURLParams(httptest.NewRequest(http.MethodPut, "/api/admin/orgs/org-1", strings.NewReader(`{"parentOrgId":"missing-parent"}`)), map[string]string{"orgId": "org-1"})
		req.Header.Set("Content-Type", "application/json")
		handleAdminUpdateOrg(parentLookupStore).ServeHTTP(w, req)
		testutil.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("concurrent parent deletion returns not found", func(t *testing.T) {
		t.Parallel()
		raceStore := &orgHandlerMockStore{
			getFn: func(_ context.Context, id string) (*tenant.Organization, error) {
				return &tenant.Organization{ID: id}, nil
			},
			updateFn: func(_ context.Context, _ string, _ tenant.OrgUpdate) (*tenant.Organization, error) {
				return nil, tenant.ErrParentOrgNotFound
			},
		}
		w := httptest.NewRecorder()
		req := withURLParams(httptest.NewRequest(http.MethodPut, "/api/admin/orgs/org-1", strings.NewReader(`{"parentOrgId":"parent-1"}`)), map[string]string{"orgId": "org-1"})
		req.Header.Set("Content-Type", "application/json")
		handleAdminUpdateOrg(raceStore).ServeHTTP(w, req)
		testutil.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestHandleAdminDeleteOrg(t *testing.T) {
	t.Parallel()

	store := &orgHandlerMockStore{deleteFn: func(_ context.Context, _ string) error { return nil }}

	t.Run("missing confirm", func(t *testing.T) {
		t.Parallel()
		w := httptest.NewRecorder()
		req := withURLParams(httptest.NewRequest(http.MethodDelete, "/api/admin/orgs/org-1", nil), map[string]string{"orgId": "org-1"})
		handleAdminDeleteOrg(store, &mockTenantService{}).ServeHTTP(w, req)
		testutil.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("has tenants", func(t *testing.T) {
		t.Parallel()
		w := httptest.NewRecorder()
		req := withURLParams(httptest.NewRequest(http.MethodDelete, "/api/admin/orgs/org-1?confirm=true", nil), map[string]string{"orgId": "org-1"})
		handleAdminDeleteOrg(store, &mockTenantService{orgTenants: []tenant.Tenant{{ID: "t1"}}}).ServeHTTP(w, req)
		testutil.Equal(t, http.StatusConflict, w.Code)
	})

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		w := httptest.NewRecorder()
		req := withURLParams(httptest.NewRequest(http.MethodDelete, "/api/admin/orgs/org-1?confirm=true", nil), map[string]string{"orgId": "org-1"})
		handleAdminDeleteOrg(store, &mockTenantService{}).ServeHTTP(w, req)
		testutil.Equal(t, http.StatusNoContent, w.Code)
	})
}

func TestHandleAdminOrgTenantAssignmentHandlers(t *testing.T) {
	t.Parallel()

	orgStore := &orgHandlerMockStore{
		getFn: func(_ context.Context, id string) (*tenant.Organization, error) {
			if id == "missing-org" {
				return nil, tenant.ErrOrgNotFound
			}
			return &tenant.Organization{ID: id, Name: "Acme", Slug: "acme"}, nil
		},
	}

	t.Run("assign success", func(t *testing.T) {
		t.Parallel()
		svc := &mockTenantService{}
		req := withURLParams(httptest.NewRequest(http.MethodPost, "/api/admin/orgs/org-1/tenants", strings.NewReader(`{"tenantId":"tenant-1"}`)), map[string]string{"orgId": "org-1"})
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handleAdminAssignTenantToOrg(orgStore, svc).ServeHTTP(w, req)
		testutil.Equal(t, http.StatusOK, w.Code)
		testutil.Equal(t, 1, svc.assignTenantToOrgCalls)
		testutil.Equal(t, "tenant-1", svc.lastAssignedTenantID)
		testutil.Equal(t, "org-1", svc.lastAssignedOrgID)
	})

	t.Run("list success", func(t *testing.T) {
		t.Parallel()
		svc := &mockTenantService{orgTenants: []tenant.Tenant{{ID: "tenant-1"}}}
		req := withURLParams(httptest.NewRequest(http.MethodGet, "/api/admin/orgs/org-1/tenants", nil), map[string]string{"orgId": "org-1"})
		w := httptest.NewRecorder()
		handleAdminListOrgTenants(orgStore, svc).ServeHTTP(w, req)
		testutil.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("unassign rejects tenant from another org", func(t *testing.T) {
		t.Parallel()
		svc := &mockTenantService{
			unassignTenantFromOrgErr: tenant.ErrTenantNotInOrg,
		}
		req := withURLParams(httptest.NewRequest(http.MethodDelete, "/api/admin/orgs/org-1/tenants/tenant-1", nil), map[string]string{"orgId": "org-1", "tenantId": "tenant-1"})
		w := httptest.NewRecorder()
		handleAdminUnassignTenantFromOrg(orgStore, svc).ServeHTTP(w, req)
		testutil.Equal(t, http.StatusNotFound, w.Code)
		testutil.Equal(t, 1, svc.unassignTenantFromOrgCalls)
		testutil.Equal(t, "tenant-1", svc.lastUnassignedTenantID)
		testutil.Equal(t, "org-1", svc.lastUnassignedOrgID)
	})

	t.Run("unassign success", func(t *testing.T) {
		t.Parallel()
		svc := &mockTenantService{}
		req := withURLParams(httptest.NewRequest(http.MethodDelete, "/api/admin/orgs/org-1/tenants/tenant-1", nil), map[string]string{"orgId": "org-1", "tenantId": "tenant-1"})
		w := httptest.NewRecorder()
		handleAdminUnassignTenantFromOrg(orgStore, svc).ServeHTTP(w, req)
		testutil.Equal(t, http.StatusNoContent, w.Code)
		testutil.Equal(t, 1, svc.unassignTenantFromOrgCalls)
		testutil.Equal(t, "tenant-1", svc.lastUnassignedTenantID)
		testutil.Equal(t, "org-1", svc.lastUnassignedOrgID)
	})
}
