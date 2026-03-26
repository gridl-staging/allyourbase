// Package server Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/internal/server/org_handler.go.
package server

import (
	"errors"
	"net/http"
	"strings"

	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/allyourbase/ayb/internal/tenant"
	"github.com/go-chi/chi/v5"
)

type createOrgRequest struct {
	Name        string  `json:"name"`
	Slug        string  `json:"slug"`
	ParentOrgID *string `json:"parentOrgId"`
	PlanTier    string  `json:"planTier"`
}

type updateOrgRequest struct {
	Name        *string `json:"name"`
	Slug        *string `json:"slug"`
	ParentOrgID *string `json:"parentOrgId"`
}

type assignTenantToOrgRequest struct {
	TenantID string `json:"tenantId"`
}

type orgListResult struct {
	Items []tenant.Organization `json:"items"`
}

type orgTenantListResult struct {
	Items []tenant.Tenant `json:"items"`
}

type orgDetailResponse struct {
	tenant.Organization
	ChildOrgCount int `json:"childOrgCount"`
	TeamCount     int `json:"teamCount"`
	TenantCount   int `json:"tenantCount"`
}

func normalizeCreateParentOrgID(parentOrgID *string) *string {
	if parentOrgID == nil {
		return nil
	}
	trimmedParentOrgID := strings.TrimSpace(*parentOrgID)
	if trimmedParentOrgID == "" {
		return nil
	}
	return &trimmedParentOrgID
}

func normalizeUpdateParentOrgID(parentOrgID *string) *string {
	if parentOrgID == nil {
		return nil
	}
	trimmedParentOrgID := strings.TrimSpace(*parentOrgID)
	return &trimmedParentOrgID
}

func validateParentOrgReference(r *http.Request, w http.ResponseWriter, store tenant.OrgStore, parentOrgID *string) bool {
	if parentOrgID == nil || *parentOrgID == "" {
		return true
	}
	if _, err := store.GetOrg(r.Context(), *parentOrgID); err != nil {
		if errors.Is(err, tenant.ErrOrgNotFound) {
			httputil.WriteError(w, http.StatusNotFound, "parent org not found")
			return false
		}
		httputil.WriteError(w, http.StatusInternalServerError, "failed to get parent org")
		return false
	}
	return true
}

func orgIDFromURL(r *http.Request, w http.ResponseWriter) (string, bool) {
	orgID := chi.URLParam(r, "orgId")
	if orgID == "" {
		httputil.WriteError(w, http.StatusBadRequest, "org id is required")
		return "", false
	}
	return orgID, true
}

// TODO: Document lookupOrg.
func lookupOrg(r *http.Request, w http.ResponseWriter, store tenant.OrgStore) (*tenant.Organization, bool) {
	orgID, ok := orgIDFromURL(r, w)
	if !ok {
		return nil, false
	}
	org, err := store.GetOrg(r.Context(), orgID)
	if err != nil {
		if errors.Is(err, tenant.ErrOrgNotFound) {
			httputil.WriteError(w, http.StatusNotFound, "org not found")
			return nil, false
		}
		httputil.WriteError(w, http.StatusInternalServerError, "failed to get org")
		return nil, false
	}
	return org, true
}

func (s *Server) orgStoreHandler(handler func(tenant.OrgStore) http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s == nil || s.orgStore == nil {
			httputil.WriteError(w, http.StatusInternalServerError, "org store not configured")
			return
		}
		handler(s.orgStore).ServeHTTP(w, r)
	}
}

func (s *Server) orgStoreTenantAdminHandler(handler func(tenant.OrgStore, tenantAdmin) http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s == nil || s.orgStore == nil {
			httputil.WriteError(w, http.StatusInternalServerError, "org store not configured")
			return
		}
		if s.tenantSvc == nil {
			httputil.WriteError(w, http.StatusInternalServerError, "tenant service not configured")
			return
		}
		handler(s.orgStore, s.tenantSvc).ServeHTTP(w, r)
	}
}

func (s *Server) orgAndTeamStoreHandler(handler func(tenant.OrgStore, tenant.TeamStore) http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s == nil || s.orgStore == nil {
			httputil.WriteError(w, http.StatusInternalServerError, "org store not configured")
			return
		}
		if s.teamStore == nil {
			httputil.WriteError(w, http.StatusInternalServerError, "team store not configured")
			return
		}
		handler(s.orgStore, s.teamStore).ServeHTTP(w, r)
	}
}

// TODO: Document Server.orgTeamAndTenantAdminHandler.
func (s *Server) orgTeamAndTenantAdminHandler(handler func(tenant.OrgStore, tenant.TeamStore, tenantAdmin) http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s == nil || s.orgStore == nil {
			httputil.WriteError(w, http.StatusInternalServerError, "org store not configured")
			return
		}
		if s.teamStore == nil {
			httputil.WriteError(w, http.StatusInternalServerError, "team store not configured")
			return
		}
		if s.tenantSvc == nil {
			httputil.WriteError(w, http.StatusInternalServerError, "tenant service not configured")
			return
		}
		handler(s.orgStore, s.teamStore, s.tenantSvc).ServeHTTP(w, r)
	}
}

// TODO: Document handleAdminCreateOrg.
func handleAdminCreateOrg(store tenant.OrgStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createOrgRequest
		if !httputil.DecodeJSON(w, r, &req) {
			return
		}
		req.Name = strings.TrimSpace(req.Name)
		req.Slug = strings.TrimSpace(req.Slug)
		req.ParentOrgID = normalizeCreateParentOrgID(req.ParentOrgID)
		if req.Name == "" {
			httputil.WriteError(w, http.StatusBadRequest, "name is required")
			return
		}
		if req.Slug == "" {
			httputil.WriteError(w, http.StatusBadRequest, "slug is required")
			return
		}
		if !tenant.IsValidSlug(req.Slug) {
			httputil.WriteError(w, http.StatusBadRequest, "invalid slug")
			return
		}
		if !isValidPlanTier(req.PlanTier) {
			httputil.WriteError(w, http.StatusBadRequest, "invalid planTier")
			return
		}
		if !validateParentOrgReference(r, w, store, req.ParentOrgID) {
			return
		}

		createdOrg, err := store.CreateOrg(r.Context(), req.Name, req.Slug, req.ParentOrgID, req.PlanTier)
		if err != nil {
			if errors.Is(err, tenant.ErrParentOrgNotFound) {
				httputil.WriteError(w, http.StatusNotFound, "parent org not found")
				return
			}
			if errors.Is(err, tenant.ErrOrgSlugTaken) {
				httputil.WriteError(w, http.StatusConflict, "org slug is already taken")
				return
			}
			if errors.Is(err, tenant.ErrCircularParentOrg) {
				httputil.WriteError(w, http.StatusConflict, "org hierarchy cannot contain cycles")
				return
			}
			if errors.Is(err, tenant.ErrInvalidSlug) {
				httputil.WriteError(w, http.StatusBadRequest, "invalid slug")
				return
			}
			httputil.WriteError(w, http.StatusInternalServerError, "failed to create org")
			return
		}

		httputil.WriteJSON(w, http.StatusCreated, createdOrg)
	}
}

func handleAdminListOrgs(store tenant.OrgStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		items, err := store.ListOrgs(r.Context(), "")
		if err != nil {
			httputil.WriteError(w, http.StatusInternalServerError, "failed to list orgs")
			return
		}
		httputil.WriteJSON(w, http.StatusOK, orgListResult{Items: items})
	}
}

// TODO: Document handleAdminGetOrg.
func handleAdminGetOrg(orgStore tenant.OrgStore, teamStore tenant.TeamStore, svc tenantAdmin) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		org, ok := lookupOrg(r, w, orgStore)
		if !ok {
			return
		}

		childOrgs, err := orgStore.ListChildOrgs(r.Context(), org.ID)
		if err != nil {
			httputil.WriteError(w, http.StatusInternalServerError, "failed to list child orgs")
			return
		}
		teams, err := teamStore.ListTeams(r.Context(), org.ID)
		if err != nil {
			if errors.Is(err, tenant.ErrOrgNotFound) {
				httputil.WriteError(w, http.StatusNotFound, "org not found")
				return
			}
			httputil.WriteError(w, http.StatusInternalServerError, "failed to list teams")
			return
		}
		orgTenants, err := svc.ListOrgTenants(r.Context(), org.ID)
		if err != nil {
			httputil.WriteError(w, http.StatusInternalServerError, "failed to list org tenants")
			return
		}

		httputil.WriteJSON(w, http.StatusOK, orgDetailResponse{
			Organization:  *org,
			ChildOrgCount: len(childOrgs),
			TeamCount:     len(teams),
			TenantCount:   len(orgTenants),
		})
	}
}

// TODO: Document handleAdminUpdateOrg.
func handleAdminUpdateOrg(store tenant.OrgStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID, ok := orgIDFromURL(r, w)
		if !ok {
			return
		}

		var req updateOrgRequest
		if !httputil.DecodeJSON(w, r, &req) {
			return
		}

		if req.Name != nil {
			trimmedName := strings.TrimSpace(*req.Name)
			if trimmedName == "" {
				httputil.WriteError(w, http.StatusBadRequest, "name cannot be empty")
				return
			}
			req.Name = &trimmedName
		}
		if req.Slug != nil {
			trimmedSlug := strings.TrimSpace(*req.Slug)
			if !tenant.IsValidSlug(trimmedSlug) {
				httputil.WriteError(w, http.StatusBadRequest, "invalid slug")
				return
			}
			req.Slug = &trimmedSlug
		}
		req.ParentOrgID = normalizeUpdateParentOrgID(req.ParentOrgID)
		if !validateParentOrgReference(r, w, store, req.ParentOrgID) {
			return
		}

		updatedOrg, err := store.UpdateOrg(r.Context(), orgID, tenant.OrgUpdate{
			Name:        req.Name,
			Slug:        req.Slug,
			ParentOrgID: req.ParentOrgID,
		})
		if err != nil {
			if errors.Is(err, tenant.ErrParentOrgNotFound) {
				httputil.WriteError(w, http.StatusNotFound, "parent org not found")
				return
			}
			if errors.Is(err, tenant.ErrOrgSlugTaken) {
				httputil.WriteError(w, http.StatusConflict, "org slug is already taken")
				return
			}
			if errors.Is(err, tenant.ErrOrgNotFound) {
				httputil.WriteError(w, http.StatusNotFound, "org not found")
				return
			}
			if errors.Is(err, tenant.ErrInvalidSlug) {
				httputil.WriteError(w, http.StatusBadRequest, "invalid slug")
				return
			}
			if errors.Is(err, tenant.ErrCircularParentOrg) {
				httputil.WriteError(w, http.StatusConflict, "org hierarchy cannot contain cycles")
				return
			}
			httputil.WriteError(w, http.StatusInternalServerError, "failed to update org")
			return
		}

		httputil.WriteJSON(w, http.StatusOK, updatedOrg)
	}
}

// TODO: Document handleAdminDeleteOrg.
func handleAdminDeleteOrg(store tenant.OrgStore, svc tenantAdmin) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID, ok := orgIDFromURL(r, w)
		if !ok {
			return
		}
		if r.URL.Query().Get("confirm") != "true" {
			httputil.WriteError(w, http.StatusBadRequest, "confirm=true is required")
			return
		}

		orgTenants, err := svc.ListOrgTenants(r.Context(), orgID)
		if err != nil {
			httputil.WriteError(w, http.StatusInternalServerError, "failed to list org tenants")
			return
		}
		if len(orgTenants) > 0 {
			httputil.WriteError(w, http.StatusConflict, "unassign tenants before deleting org")
			return
		}

		err = store.DeleteOrg(r.Context(), orgID)
		if err != nil {
			if errors.Is(err, tenant.ErrOrgNotFound) {
				httputil.WriteError(w, http.StatusNotFound, "org not found")
				return
			}
			httputil.WriteError(w, http.StatusInternalServerError, "failed to delete org")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// TODO: Document handleAdminAssignTenantToOrg.
func handleAdminAssignTenantToOrg(store tenant.OrgStore, svc tenantAdmin) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		org, ok := lookupOrg(r, w, store)
		if !ok {
			return
		}

		var req assignTenantToOrgRequest
		if !httputil.DecodeJSON(w, r, &req) {
			return
		}
		req.TenantID = strings.TrimSpace(req.TenantID)
		if req.TenantID == "" {
			httputil.WriteError(w, http.StatusBadRequest, "tenantId is required")
			return
		}

		err := svc.AssignTenantToOrg(r.Context(), req.TenantID, org.ID)
		if err != nil {
			if errors.Is(err, tenant.ErrTenantNotFound) {
				httputil.WriteError(w, http.StatusNotFound, "tenant not found")
				return
			}
			if errors.Is(err, tenant.ErrOrgNotFound) {
				httputil.WriteError(w, http.StatusNotFound, "org not found")
				return
			}
			httputil.WriteError(w, http.StatusInternalServerError, "failed to assign tenant to org")
			return
		}

		httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "assigned"})
	}
}

// TODO: Document handleAdminUnassignTenantFromOrg.
func handleAdminUnassignTenantFromOrg(store tenant.OrgStore, svc tenantAdmin) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		org, ok := lookupOrg(r, w, store)
		if !ok {
			return
		}
		tenantID := strings.TrimSpace(chi.URLParam(r, "tenantId"))
		if tenantID == "" {
			httputil.WriteError(w, http.StatusBadRequest, "tenant id is required")
			return
		}

		err := svc.UnassignTenantFromOrg(r.Context(), tenantID, org.ID)
		if err != nil {
			if errors.Is(err, tenant.ErrTenantNotFound) {
				httputil.WriteError(w, http.StatusNotFound, "tenant not found")
				return
			}
			if errors.Is(err, tenant.ErrTenantNotInOrg) {
				httputil.WriteError(w, http.StatusNotFound, "tenant not found in org")
				return
			}
			httputil.WriteError(w, http.StatusInternalServerError, "failed to unassign tenant from org")
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Server) orgStoreUsageHandler(handler func(tenant.OrgStore, orgUsageQuerier) http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s == nil || s.orgStore == nil {
			httputil.WriteError(w, http.StatusInternalServerError, "org store not configured")
			return
		}
		handler(s.orgStore, s.orgUsageQuerier).ServeHTTP(w, r)
	}
}

func (s *Server) orgStoreAuditHandler(handler func(tenant.OrgStore, orgAuditQuerier) http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s == nil || s.orgStore == nil {
			httputil.WriteError(w, http.StatusInternalServerError, "org store not configured")
			return
		}
		handler(s.orgStore, s.orgAuditQuerier).ServeHTTP(w, r)
	}
}

func handleAdminListOrgTenants(store tenant.OrgStore, svc tenantAdmin) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		org, ok := lookupOrg(r, w, store)
		if !ok {
			return
		}

		orgTenants, err := svc.ListOrgTenants(r.Context(), org.ID)
		if err != nil {
			httputil.WriteError(w, http.StatusInternalServerError, "failed to list org tenants")
			return
		}
		httputil.WriteJSON(w, http.StatusOK, orgTenantListResult{Items: orgTenants})
	}
}
