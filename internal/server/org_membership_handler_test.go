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
	testOrgID                    = "550e8400-e29b-41d4-a716-446655440001"
	testMissingOrgID             = "550e8400-e29b-41d4-a716-446655440099"
	testOrgMemberUserID          = "550e8400-e29b-41d4-a716-446655440011"
	testOrgConflictUserID        = "550e8400-e29b-41d4-a716-446655440012"
	testOrgMissingMemberUserID   = "550e8400-e29b-41d4-a716-446655440013"
	testOrgLastOwnerMemberUserID = "550e8400-e29b-41d4-a716-446655440014"
)

type orgMembershipHandlerMockStore struct {
	addFn    func(ctx context.Context, orgID, userID, role string) (*tenant.OrgMembership, error)
	listFn   func(ctx context.Context, orgID string) ([]tenant.OrgMembership, error)
	updateFn func(ctx context.Context, orgID, userID, role string) (*tenant.OrgMembership, error)
	removeFn func(ctx context.Context, orgID, userID string) error
	getFn    func(ctx context.Context, orgID, userID string) (*tenant.OrgMembership, error)
}

func (m *orgMembershipHandlerMockStore) AddOrgMembership(ctx context.Context, orgID, userID, role string) (*tenant.OrgMembership, error) {
	if m.addFn != nil {
		return m.addFn(ctx, orgID, userID, role)
	}
	return nil, errors.New("addFn not set")
}

func (m *orgMembershipHandlerMockStore) GetOrgMembership(ctx context.Context, orgID, userID string) (*tenant.OrgMembership, error) {
	if m.getFn != nil {
		return m.getFn(ctx, orgID, userID)
	}
	return nil, tenant.ErrOrgMembershipNotFound
}

func (m *orgMembershipHandlerMockStore) ListOrgMemberships(ctx context.Context, orgID string) ([]tenant.OrgMembership, error) {
	if m.listFn != nil {
		return m.listFn(ctx, orgID)
	}
	return []tenant.OrgMembership{}, nil
}

func (m *orgMembershipHandlerMockStore) ListUserOrgMemberships(_ context.Context, _ string) ([]tenant.OrgMembership, error) {
	return []tenant.OrgMembership{}, nil
}

func (m *orgMembershipHandlerMockStore) RemoveOrgMembership(ctx context.Context, orgID, userID string) error {
	if m.removeFn != nil {
		return m.removeFn(ctx, orgID, userID)
	}
	return nil
}

func (m *orgMembershipHandlerMockStore) UpdateOrgMembershipRole(ctx context.Context, orgID, userID, role string) (*tenant.OrgMembership, error) {
	if m.updateFn != nil {
		return m.updateFn(ctx, orgID, userID, role)
	}
	return &tenant.OrgMembership{OrgID: orgID, UserID: userID, Role: role}, nil
}

func newOrgMembershipHandlerTestStores() (*orgHandlerMockStore, *orgMembershipHandlerMockStore) {
	orgStore := &orgHandlerMockStore{
		getFn: func(_ context.Context, id string) (*tenant.Organization, error) {
			if id == testMissingOrgID {
				return nil, tenant.ErrOrgNotFound
			}
			return &tenant.Organization{ID: id}, nil
		},
	}
	store := &orgMembershipHandlerMockStore{
		addFn: func(_ context.Context, orgID, userID, role string) (*tenant.OrgMembership, error) {
			if userID == testOrgConflictUserID {
				return nil, tenant.ErrOrgMembershipExists
			}
			return &tenant.OrgMembership{OrgID: orgID, UserID: userID, Role: role}, nil
		},
		listFn: func(_ context.Context, orgID string) ([]tenant.OrgMembership, error) {
			return []tenant.OrgMembership{{OrgID: orgID, UserID: "u1", Role: tenant.OrgRoleMember}}, nil
		},
		updateFn: func(_ context.Context, orgID, userID, role string) (*tenant.OrgMembership, error) {
			if userID == testOrgMissingMemberUserID {
				return nil, tenant.ErrOrgMembershipNotFound
			}
			if userID == testOrgLastOwnerMemberUserID {
				return nil, tenant.ErrLastOwner
			}
			return &tenant.OrgMembership{OrgID: orgID, UserID: userID, Role: role}, nil
		},
		removeFn: func(_ context.Context, _ string, userID string) error {
			if userID == testOrgMissingMemberUserID {
				return tenant.ErrOrgMembershipNotFound
			}
			if userID == testOrgLastOwnerMemberUserID {
				return tenant.ErrLastOwner
			}
			return nil
		},
	}
	return orgStore, store
}

func TestOrgMembershipHandlerAdd(t *testing.T) {
	t.Parallel()
	orgStore, store := newOrgMembershipHandlerTestStores()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		req := withURLParams(httptest.NewRequest(http.MethodPost, "/api/admin/orgs/org-1/members", strings.NewReader(`{"userId":"`+testOrgMemberUserID+`","role":"member"}`)), map[string]string{"orgId": testOrgID})
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handleAdminAddOrgMember(orgStore, store).ServeHTTP(w, req)
		testutil.Equal(t, http.StatusCreated, w.Code)
	})

	t.Run("invalid role", func(t *testing.T) {
		t.Parallel()
		req := withURLParams(httptest.NewRequest(http.MethodPost, "/api/admin/orgs/org-1/members", strings.NewReader(`{"userId":"`+testOrgMemberUserID+`","role":"invalid"}`)), map[string]string{"orgId": testOrgID})
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handleAdminAddOrgMember(orgStore, store).ServeHTTP(w, req)
		testutil.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("invalid user id format", func(t *testing.T) {
		t.Parallel()
		req := withURLParams(httptest.NewRequest(http.MethodPost, "/api/admin/orgs/org-1/members", strings.NewReader(`{"userId":"not-a-uuid","role":"member"}`)), map[string]string{"orgId": testOrgID})
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handleAdminAddOrgMember(orgStore, store).ServeHTTP(w, req)
		testutil.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("invalid org id format", func(t *testing.T) {
		t.Parallel()
		req := withURLParams(httptest.NewRequest(http.MethodPost, "/api/admin/orgs/not-a-uuid/members", strings.NewReader(`{"userId":"550e8400-e29b-41d4-a716-446655440011","role":"member"}`)), map[string]string{"orgId": "not-a-uuid"})
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handleAdminAddOrgMember(orgStore, store).ServeHTTP(w, req)
		testutil.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("conflict", func(t *testing.T) {
		t.Parallel()
		req := withURLParams(httptest.NewRequest(http.MethodPost, "/api/admin/orgs/org-1/members", strings.NewReader(`{"userId":"`+testOrgConflictUserID+`","role":"member"}`)), map[string]string{"orgId": testOrgID})
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handleAdminAddOrgMember(orgStore, store).ServeHTTP(w, req)
		testutil.Equal(t, http.StatusConflict, w.Code)
	})

	t.Run("missing org", func(t *testing.T) {
		t.Parallel()
		req := withURLParams(httptest.NewRequest(http.MethodPost, "/api/admin/orgs/missing-org/members", strings.NewReader(`{"userId":"`+testOrgMemberUserID+`","role":"member"}`)), map[string]string{"orgId": testMissingOrgID})
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handleAdminAddOrgMember(orgStore, store).ServeHTTP(w, req)
		testutil.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("concurrent org deletion", func(t *testing.T) {
		t.Parallel()
		raceStore := &orgMembershipHandlerMockStore{
			addFn: func(_ context.Context, _ string, _ string, _ string) (*tenant.OrgMembership, error) {
				return nil, tenant.ErrOrgNotFound
			},
		}
		req := withURLParams(httptest.NewRequest(http.MethodPost, "/api/admin/orgs/org-1/members", strings.NewReader(`{"userId":"`+testOrgMemberUserID+`","role":"member"}`)), map[string]string{"orgId": testOrgID})
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handleAdminAddOrgMember(orgStore, raceStore).ServeHTTP(w, req)
		testutil.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestOrgMembershipHandlerList(t *testing.T) {
	t.Parallel()
	orgStore, store := newOrgMembershipHandlerTestStores()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		req := withURLParams(httptest.NewRequest(http.MethodGet, "/api/admin/orgs/org-1/members", nil), map[string]string{"orgId": testOrgID})
		w := httptest.NewRecorder()
		handleAdminListOrgMembers(orgStore, store).ServeHTTP(w, req)
		testutil.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("missing org", func(t *testing.T) {
		t.Parallel()
		req := withURLParams(httptest.NewRequest(http.MethodGet, "/api/admin/orgs/missing-org/members", nil), map[string]string{"orgId": testMissingOrgID})
		w := httptest.NewRecorder()
		handleAdminListOrgMembers(orgStore, store).ServeHTTP(w, req)
		testutil.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("invalid org id format", func(t *testing.T) {
		t.Parallel()
		req := withURLParams(httptest.NewRequest(http.MethodGet, "/api/admin/orgs/not-a-uuid/members", nil), map[string]string{"orgId": "not-a-uuid"})
		w := httptest.NewRecorder()
		handleAdminListOrgMembers(orgStore, store).ServeHTTP(w, req)
		testutil.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("concurrent org deletion", func(t *testing.T) {
		t.Parallel()
		raceStore := &orgMembershipHandlerMockStore{
			listFn: func(_ context.Context, _ string) ([]tenant.OrgMembership, error) {
				return nil, tenant.ErrOrgNotFound
			},
		}
		req := withURLParams(httptest.NewRequest(http.MethodGet, "/api/admin/orgs/org-1/members", nil), map[string]string{"orgId": testOrgID})
		w := httptest.NewRecorder()
		handleAdminListOrgMembers(orgStore, raceStore).ServeHTTP(w, req)
		testutil.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestOrgMembershipHandlerUpdateRole(t *testing.T) {
	t.Parallel()
	orgStore, store := newOrgMembershipHandlerTestStores()

	t.Run("not found", func(t *testing.T) {
		t.Parallel()
		req := withURLParams(httptest.NewRequest(http.MethodPut, "/api/admin/orgs/org-1/members/missing/role", strings.NewReader(`{"role":"admin"}`)), map[string]string{"orgId": testOrgID, "userId": testOrgMissingMemberUserID})
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handleAdminUpdateOrgMemberRole(orgStore, store).ServeHTTP(w, req)
		testutil.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("last owner", func(t *testing.T) {
		t.Parallel()
		req := withURLParams(httptest.NewRequest(http.MethodPut, "/api/admin/orgs/org-1/members/last-owner/role", strings.NewReader(`{"role":"admin"}`)), map[string]string{"orgId": testOrgID, "userId": testOrgLastOwnerMemberUserID})
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handleAdminUpdateOrgMemberRole(orgStore, store).ServeHTTP(w, req)
		testutil.Equal(t, http.StatusConflict, w.Code)
	})

	t.Run("invalid user id format", func(t *testing.T) {
		t.Parallel()
		req := withURLParams(httptest.NewRequest(http.MethodPut, "/api/admin/orgs/org-1/members/not-a-uuid/role", strings.NewReader(`{"role":"admin"}`)), map[string]string{"orgId": testOrgID, "userId": "not-a-uuid"})
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handleAdminUpdateOrgMemberRole(orgStore, store).ServeHTTP(w, req)
		testutil.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("invalid org id format", func(t *testing.T) {
		t.Parallel()
		req := withURLParams(httptest.NewRequest(http.MethodPut, "/api/admin/orgs/not-a-uuid/members/550e8400-e29b-41d4-a716-446655440011/role", strings.NewReader(`{"role":"admin"}`)), map[string]string{"orgId": "not-a-uuid", "userId": testOrgMemberUserID})
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handleAdminUpdateOrgMemberRole(orgStore, store).ServeHTTP(w, req)
		testutil.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("concurrent org deletion", func(t *testing.T) {
		t.Parallel()
		raceStore := &orgMembershipHandlerMockStore{
			updateFn: func(_ context.Context, _ string, _ string, _ string) (*tenant.OrgMembership, error) {
				return nil, tenant.ErrOrgNotFound
			},
		}
		req := withURLParams(httptest.NewRequest(http.MethodPut, "/api/admin/orgs/org-1/members/u1/role", strings.NewReader(`{"role":"admin"}`)), map[string]string{"orgId": testOrgID, "userId": testOrgMemberUserID})
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handleAdminUpdateOrgMemberRole(orgStore, raceStore).ServeHTTP(w, req)
		testutil.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestOrgMembershipHandlerRemove(t *testing.T) {
	t.Parallel()
	orgStore, store := newOrgMembershipHandlerTestStores()

	t.Run("not found", func(t *testing.T) {
		t.Parallel()
		req := withURLParams(httptest.NewRequest(http.MethodDelete, "/api/admin/orgs/org-1/members/missing", nil), map[string]string{"orgId": testOrgID, "userId": testOrgMissingMemberUserID})
		w := httptest.NewRecorder()
		handleAdminRemoveOrgMember(orgStore, store).ServeHTTP(w, req)
		testutil.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("last owner", func(t *testing.T) {
		t.Parallel()
		req := withURLParams(httptest.NewRequest(http.MethodDelete, "/api/admin/orgs/org-1/members/last-owner", nil), map[string]string{"orgId": testOrgID, "userId": testOrgLastOwnerMemberUserID})
		w := httptest.NewRecorder()
		handleAdminRemoveOrgMember(orgStore, store).ServeHTTP(w, req)
		testutil.Equal(t, http.StatusConflict, w.Code)
	})

	t.Run("invalid user id format", func(t *testing.T) {
		t.Parallel()
		req := withURLParams(httptest.NewRequest(http.MethodDelete, "/api/admin/orgs/org-1/members/not-a-uuid", nil), map[string]string{"orgId": testOrgID, "userId": "not-a-uuid"})
		w := httptest.NewRecorder()
		handleAdminRemoveOrgMember(orgStore, store).ServeHTTP(w, req)
		testutil.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("invalid org id format", func(t *testing.T) {
		t.Parallel()
		req := withURLParams(httptest.NewRequest(http.MethodDelete, "/api/admin/orgs/not-a-uuid/members/550e8400-e29b-41d4-a716-446655440011", nil), map[string]string{"orgId": "not-a-uuid", "userId": testOrgMemberUserID})
		w := httptest.NewRecorder()
		handleAdminRemoveOrgMember(orgStore, store).ServeHTTP(w, req)
		testutil.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("concurrent org deletion", func(t *testing.T) {
		t.Parallel()
		raceStore := &orgMembershipHandlerMockStore{
			removeFn: func(_ context.Context, _ string, _ string) error {
				return tenant.ErrOrgNotFound
			},
		}
		req := withURLParams(httptest.NewRequest(http.MethodDelete, "/api/admin/orgs/org-1/members/u1", nil), map[string]string{"orgId": testOrgID, "userId": testOrgMemberUserID})
		w := httptest.NewRecorder()
		handleAdminRemoveOrgMember(orgStore, raceStore).ServeHTTP(w, req)
		testutil.Equal(t, http.StatusNotFound, w.Code)
	})
}
