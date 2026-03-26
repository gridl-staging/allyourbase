package server

import (
	"errors"
	"net/http"

	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/allyourbase/ayb/internal/tenant"
	"github.com/go-chi/chi/v5"
)

type addMemberRequest struct {
	UserID string `json:"userId"`
	Role   string `json:"role"`
}

type updateRoleRequest struct {
	Role string `json:"role"`
}

type membershipListResult struct {
	Items []tenant.TenantMembership `json:"items"`
}

// Lists all members of a tenant, returning their user IDs and assigned roles.
func handleAdminListTenantMembers(svc tenantAdmin) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		currentTenant, ok := lookupTenant(r, w, svc)
		if !ok {
			return
		}

		members, err := svc.ListMemberships(r.Context(), currentTenant.ID)
		if err != nil {
			httputil.WriteError(w, http.StatusInternalServerError, "failed to list members")
			return
		}

		httputil.WriteJSON(w, http.StatusOK, membershipListResult{Items: members})
	}
}

// Adds a member to a tenant with a specified role. Validates that the userId is a valid UUID and the role is valid. Returns 409 if the membership already exists. Emits a membership added audit event.
func handleAdminAddTenantMember(svc tenantAdmin, emitter *tenant.AuditEmitter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		currentTenant, ok := lookupTenant(r, w, svc)
		if !ok {
			return
		}

		var req addMemberRequest
		if !httputil.DecodeJSON(w, r, &req) {
			return
		}

		if req.UserID == "" {
			httputil.WriteError(w, http.StatusBadRequest, "userId is required")
			return
		}

		if !httputil.IsValidUUID(req.UserID) {
			httputil.WriteError(w, http.StatusBadRequest, "invalid userId format")
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

		membership, err := svc.AddMembership(r.Context(), currentTenant.ID, req.UserID, req.Role)
		if err != nil {
			if errors.Is(err, tenant.ErrMembershipExists) {
				httputil.WriteError(w, http.StatusConflict, "membership already exists")
				return
			}
			httputil.WriteError(w, http.StatusInternalServerError, "failed to add member")
			return
		}

		emitTenantAuditEvent(r, svc, currentTenant.ID, tenant.AuditActionMembershipAdded, map[string]string{
			"userId": req.UserID,
			"role":   req.Role,
		}, emitter)

		httputil.WriteJSON(w, http.StatusCreated, membership)
	}
}

// Removes a member from a tenant, returning 204 on success. Returns 404 if the membership does not exist. Emits a membership removed audit event.
func handleAdminRemoveTenantMember(svc tenantAdmin, emitter *tenant.AuditEmitter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID, ok := tenantIDFromURL(r, w)
		if !ok {
			return
		}

		userID := chi.URLParam(r, "userId")
		if userID == "" {
			httputil.WriteError(w, http.StatusBadRequest, "user id is required")
			return
		}

		currentTenant, ok := lookupTenantByID(r, w, svc, tenantID)
		if !ok {
			return
		}

		err := svc.RemoveMembership(r.Context(), currentTenant.ID, userID)
		if err != nil {
			if errors.Is(err, tenant.ErrMembershipNotFound) {
				httputil.WriteError(w, http.StatusNotFound, "membership not found")
				return
			}
			httputil.WriteError(w, http.StatusInternalServerError, "failed to remove member")
			return
		}

		emitTenantAuditEvent(r, svc, currentTenant.ID, tenant.AuditActionMembershipRemoved, map[string]string{
			"userId": userID,
		}, emitter)

		w.WriteHeader(http.StatusNoContent)
	}
}

// Updates the role of an existing tenant member. Validates the new role. Returns 404 if the membership does not exist. Emits a membership role change audit event.
func handleAdminUpdateTenantMemberRole(svc tenantAdmin, emitter *tenant.AuditEmitter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID, ok := tenantIDFromURL(r, w)
		if !ok {
			return
		}

		userID := chi.URLParam(r, "userId")
		if userID == "" {
			httputil.WriteError(w, http.StatusBadRequest, "user id is required")
			return
		}

		currentTenant, ok := lookupTenantByID(r, w, svc, tenantID)
		if !ok {
			return
		}

		var req updateRoleRequest
		if !httputil.DecodeJSON(w, r, &req) {
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

		membership, err := svc.UpdateMembershipRole(r.Context(), currentTenant.ID, userID, req.Role)
		if err != nil {
			if errors.Is(err, tenant.ErrMembershipNotFound) {
				httputil.WriteError(w, http.StatusNotFound, "membership not found")
				return
			}
			httputil.WriteError(w, http.StatusInternalServerError, "failed to update member role")
			return
		}

		emitTenantAuditEvent(r, svc, currentTenant.ID, tenant.AuditActionMembershipRoleChange, map[string]string{
			"userId": userID,
			"role":   req.Role,
		}, emitter)

		httputil.WriteJSON(w, http.StatusOK, membership)
	}
}
