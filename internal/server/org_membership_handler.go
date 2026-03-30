package server

import (
	"errors"
	"net/http"
	"strings"

	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/allyourbase/ayb/internal/tenant"
	"github.com/go-chi/chi/v5"
)

type orgMembershipListResult struct {
	Items []tenant.OrgMembership `json:"items"`
}

func requiredUUIDRouteParam(r *http.Request, w http.ResponseWriter, paramName, label string) (string, bool) {
	value := strings.TrimSpace(chi.URLParam(r, paramName))
	if value == "" {
		httputil.WriteError(w, http.StatusBadRequest, label+" is required")
		return "", false
	}
	if !httputil.IsValidUUID(value) {
		httputil.WriteError(w, http.StatusBadRequest, "invalid "+label+" format")
		return "", false
	}
	return value, true
}

func orgIDFromOrgMembershipRoute(r *http.Request, w http.ResponseWriter) (string, bool) {
	return requiredUUIDRouteParam(r, w, "orgId", "org id")
}

func lookupOrgMembershipRouteOrgID(r *http.Request, w http.ResponseWriter, orgStore tenant.OrgStore) (string, bool) {
	orgID, ok := orgIDFromOrgMembershipRoute(r, w)
	if !ok {
		return "", false
	}
	if _, err := orgStore.GetOrg(r.Context(), orgID); err != nil {
		if errors.Is(err, tenant.ErrOrgNotFound) {
			httputil.WriteError(w, http.StatusNotFound, "org not found")
			return "", false
		}
		httputil.WriteError(w, http.StatusInternalServerError, "failed to get org")
		return "", false
	}
	return orgID, true
}

func (s *Server) orgStoreMembershipHandler(handler func(tenant.OrgStore, tenant.OrgMembershipStore) http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s == nil || s.orgStore == nil {
			httputil.WriteError(w, http.StatusInternalServerError, "org store not configured")
			return
		}
		if s == nil || s.orgMembershipStore == nil {
			httputil.WriteError(w, http.StatusInternalServerError, "org membership store not configured")
			return
		}
		handler(s.orgStore, s.orgMembershipStore).ServeHTTP(w, r)
	}
}

// handleAdminAddOrgMember handles POST requests to add a user to an organization with a specified role, validating the user ID format and role value.
func handleAdminAddOrgMember(orgStore tenant.OrgStore, store tenant.OrgMembershipStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID, ok := lookupOrgMembershipRouteOrgID(r, w, orgStore)
		if !ok {
			return
		}

		var req addMemberRequest
		if !httputil.DecodeJSON(w, r, &req) {
			return
		}
		req.UserID = strings.TrimSpace(req.UserID)
		req.Role = strings.TrimSpace(req.Role)
		if req.UserID == "" {
			httputil.WriteError(w, http.StatusBadRequest, "userId is required")
			return
		}
		if !httputil.IsValidUUID(req.UserID) {
			httputil.WriteError(w, http.StatusBadRequest, "invalid user id format")
			return
		}
		if req.Role == "" {
			httputil.WriteError(w, http.StatusBadRequest, "role is required")
			return
		}
		if !tenant.IsValidRole(req.Role) {
			httputil.WriteError(w, http.StatusBadRequest, "invalid role")
			return
		}

		membership, err := store.AddOrgMembership(r.Context(), orgID, req.UserID, req.Role)
		if err != nil {
			if errors.Is(err, tenant.ErrOrgNotFound) {
				httputil.WriteError(w, http.StatusNotFound, "org not found")
				return
			}
			if errors.Is(err, tenant.ErrOrgMembershipExists) {
				httputil.WriteError(w, http.StatusConflict, "org membership already exists")
				return
			}
			httputil.WriteError(w, http.StatusInternalServerError, "failed to add org member")
			return
		}
		httputil.WriteJSON(w, http.StatusCreated, membership)
	}
}

// handleAdminListOrgMembers handles GET requests to list all memberships for an organization.
func handleAdminListOrgMembers(orgStore tenant.OrgStore, store tenant.OrgMembershipStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID, ok := lookupOrgMembershipRouteOrgID(r, w, orgStore)
		if !ok {
			return
		}
		memberships, err := store.ListOrgMemberships(r.Context(), orgID)
		if err != nil {
			if errors.Is(err, tenant.ErrOrgNotFound) {
				httputil.WriteError(w, http.StatusNotFound, "org not found")
				return
			}
			httputil.WriteError(w, http.StatusInternalServerError, "failed to list org members")
			return
		}
		httputil.WriteJSON(w, http.StatusOK, orgMembershipListResult{Items: memberships})
	}
}

// handleAdminUpdateOrgMemberRole handles PATCH requests to change a member's role within an organization, preventing demotion of the last owner.
func handleAdminUpdateOrgMemberRole(orgStore tenant.OrgStore, store tenant.OrgMembershipStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID, ok := lookupOrgMembershipRouteOrgID(r, w, orgStore)
		if !ok {
			return
		}
		userID, ok := requiredUUIDRouteParam(r, w, "userId", "user id")
		if !ok {
			return
		}

		var req updateRoleRequest
		if !httputil.DecodeJSON(w, r, &req) {
			return
		}
		req.Role = strings.TrimSpace(req.Role)
		if req.Role == "" {
			httputil.WriteError(w, http.StatusBadRequest, "role is required")
			return
		}
		if !tenant.IsValidRole(req.Role) {
			httputil.WriteError(w, http.StatusBadRequest, "invalid role")
			return
		}

		membership, err := store.UpdateOrgMembershipRole(r.Context(), orgID, userID, req.Role)
		if err != nil {
			if errors.Is(err, tenant.ErrOrgNotFound) {
				httputil.WriteError(w, http.StatusNotFound, "org not found")
				return
			}
			if errors.Is(err, tenant.ErrOrgMembershipNotFound) {
				httputil.WriteError(w, http.StatusNotFound, "org membership not found")
				return
			}
			if errors.Is(err, tenant.ErrLastOwner) {
				httputil.WriteError(w, http.StatusConflict, "cannot demote the last owner")
				return
			}
			httputil.WriteError(w, http.StatusInternalServerError, "failed to update org member role")
			return
		}
		httputil.WriteJSON(w, http.StatusOK, membership)
	}
}

// handleAdminRemoveOrgMember handles DELETE requests to remove a user's membership from an organization, preventing removal of the last owner.
func handleAdminRemoveOrgMember(orgStore tenant.OrgStore, store tenant.OrgMembershipStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID, ok := lookupOrgMembershipRouteOrgID(r, w, orgStore)
		if !ok {
			return
		}
		userID, ok := requiredUUIDRouteParam(r, w, "userId", "user id")
		if !ok {
			return
		}

		err := store.RemoveOrgMembership(r.Context(), orgID, userID)
		if err != nil {
			if errors.Is(err, tenant.ErrOrgNotFound) {
				httputil.WriteError(w, http.StatusNotFound, "org not found")
				return
			}
			if errors.Is(err, tenant.ErrOrgMembershipNotFound) {
				httputil.WriteError(w, http.StatusNotFound, "org membership not found")
				return
			}
			if errors.Is(err, tenant.ErrLastOwner) {
				httputil.WriteError(w, http.StatusConflict, "cannot demote the last owner")
				return
			}
			httputil.WriteError(w, http.StatusInternalServerError, "failed to remove org member")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
