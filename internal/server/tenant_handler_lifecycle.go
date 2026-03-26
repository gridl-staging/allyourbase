// Package server Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/internal/server/tenant_handler_lifecycle.go.
package server

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/allyourbase/ayb/internal/tenant"
)

type createTenantRequest struct {
	Name           string          `json:"name"`
	Slug           string          `json:"slug"`
	OwnerUserID    string          `json:"ownerUserId"`
	IsolationMode  string          `json:"isolationMode"`
	PlanTier       string          `json:"planTier"`
	Region         string          `json:"region"`
	OrgMetadata    json.RawMessage `json:"orgMetadata"`
	IdempotencyKey string          `json:"idempotencyKey"`
}

type updateTenantRequest struct {
	Name        string          `json:"name"`
	OrgMetadata json.RawMessage `json:"orgMetadata"`
}

// TODO: Document validateCreateTenantRequest.
func validateCreateTenantRequest(w http.ResponseWriter, req createTenantRequest) bool {
	switch {
	case req.Name == "":
		httputil.WriteError(w, http.StatusBadRequest, "name is required")
		return false
	case req.Slug == "":
		httputil.WriteError(w, http.StatusBadRequest, "slug is required")
		return false
	case !isValidSlug(req.Slug):
		httputil.WriteError(w, http.StatusBadRequest, "invalid slug format")
		return false
	case !isValidIsolationMode(req.IsolationMode):
		httputil.WriteError(w, http.StatusBadRequest, "invalid isolationMode")
		return false
	case !isValidPlanTier(req.PlanTier):
		httputil.WriteError(w, http.StatusBadRequest, "invalid planTier")
		return false
	}

	if req.OwnerUserID != "" && !validateOwnerUserID(w, req.OwnerUserID) {
		return false
	}

	return true
}

func requestIdempotencyKey(r *http.Request, bodyKey string) string {
	if bodyKey != "" {
		return bodyKey
	}
	return r.Header.Get("Idempotency-Key")
}

func writeTenantTransitionError(w http.ResponseWriter, err error, invalidMessage, failureMessage string) {
	if errors.Is(err, tenant.ErrInvalidStateTransition) {
		httputil.WriteError(w, http.StatusBadRequest, invalidMessage)
		return
	}

	httputil.WriteError(w, http.StatusInternalServerError, failureMessage)
}

// Creates a new tenant with validated slug format, isolation mode, and plan tier.
// When ownerUserId is provided, it also adds that user as an owner member.
// Returns 409 if the slug is already taken. Supports idempotency via IdempotencyKey.
// Emits a tenant created audit event.
func handleAdminCreateTenant(svc tenantAdmin, emitter *tenant.AuditEmitter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createTenantRequest
		if !httputil.DecodeJSON(w, r, &req) {
			return
		}

		if !validateCreateTenantRequest(w, req) {
			return
		}

		idempotencyKey := requestIdempotencyKey(r, req.IdempotencyKey)
		isolationMode := tenant.NormalizeIsolationMode(req.IsolationMode)

		createdTenant, err := svc.CreateTenant(r.Context(), req.Name, req.Slug, isolationMode, req.PlanTier, req.Region, req.OrgMetadata, idempotencyKey)
		if err != nil {
			if errors.Is(err, tenant.ErrTenantSlugTaken) {
				httputil.WriteError(w, http.StatusConflict, "tenant slug is already taken")
				return
			}
			if errors.Is(err, tenant.ErrTenantNameRequired) {
				httputil.WriteError(w, http.StatusBadRequest, "name is required")
				return
			}
			httputil.WriteError(w, http.StatusInternalServerError, "failed to create tenant")
			return
		}

		if req.OwnerUserID != "" {
			_, err = svc.AddMembership(r.Context(), createdTenant.ID, req.OwnerUserID, tenant.MemberRoleOwner)
			if err != nil && !errors.Is(err, tenant.ErrMembershipExists) {
				httputil.WriteError(w, http.StatusInternalServerError, "failed to create owner membership")
				return
			}
		}

		emitTenantAuditEvent(r, svc, createdTenant.ID, tenant.AuditActionTenantCreated, map[string]string{
			"name":          req.Name,
			"slug":          req.Slug,
			"ownerUserId":   req.OwnerUserID,
			"isolationMode": isolationMode,
			"planTier":      req.PlanTier,
			"region":        req.Region,
		}, emitter)

		httputil.WriteJSON(w, http.StatusCreated, createdTenant)
	}
}

func handleAdminListTenants(svc tenantAdmin) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		perPage, _ := strconv.Atoi(r.URL.Query().Get("perPage"))

		result, err := svc.ListTenants(r.Context(), page, perPage)
		if err != nil {
			httputil.WriteError(w, http.StatusInternalServerError, "failed to list tenants")
			return
		}

		httputil.WriteJSON(w, http.StatusOK, result)
	}
}

func handleAdminGetTenant(svc tenantAdmin) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		currentTenant, ok := lookupTenant(r, w, svc)
		if !ok {
			return
		}

		httputil.WriteJSON(w, http.StatusOK, currentTenant)
	}
}

// Updates a tenant's name and/or organizational metadata. Returns a 400 error if the tenant is deleted or no fields are provided. Emits a tenant updated audit event.
func handleAdminUpdateTenant(svc tenantAdmin, emitter *tenant.AuditEmitter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		currentTenant, ok := lookupTenant(r, w, svc)
		if !ok {
			return
		}

		if currentTenant.State == tenant.TenantStateDeleted {
			httputil.WriteError(w, http.StatusBadRequest, "cannot update deleted tenant")
			return
		}

		var req updateTenantRequest
		if !httputil.DecodeJSON(w, r, &req) {
			return
		}

		if req.Name == "" && req.OrgMetadata == nil {
			httputil.WriteError(w, http.StatusBadRequest, "no fields to update")
			return
		}

		updatedTenant, err := svc.UpdateTenant(r.Context(), currentTenant.ID, req.Name, req.OrgMetadata)
		if err != nil {
			httputil.WriteError(w, http.StatusInternalServerError, "failed to update tenant")
			return
		}

		emitTenantAuditEvent(r, svc, currentTenant.ID, tenant.AuditActionTenantUpdated, map[string]any{
			"name":        req.Name,
			"orgMetadata": req.OrgMetadata,
		}, emitter)

		httputil.WriteJSON(w, http.StatusOK, updatedTenant)
	}
}

// Suspends an active tenant by transitioning it to the suspended state. Returns a 400 error if the tenant is not currently active or the state transition is invalid. Emits a tenant suspended audit event.
func handleAdminSuspendTenant(svc tenantAdmin, emitter *tenant.AuditEmitter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		currentTenant, ok := lookupTenant(r, w, svc)
		if !ok {
			return
		}

		if currentTenant.State != tenant.TenantStateActive {
			httputil.WriteError(w, http.StatusBadRequest, "tenant is not active")
			return
		}

		updatedTenant, err := svc.TransitionState(r.Context(), currentTenant.ID, tenant.TenantStateActive, tenant.TenantStateSuspended)
		if err != nil {
			writeTenantTransitionError(w, err, "cannot suspend tenant", "failed to suspend tenant")
			return
		}

		emitTenantAuditEvent(r, svc, currentTenant.ID, tenant.AuditActionTenantSuspended, map[string]string{
			"previousState": string(tenant.TenantStateActive),
		}, emitter)

		httputil.WriteJSON(w, http.StatusOK, updatedTenant)
	}
}

// Resumes a suspended tenant by transitioning it back to the active state. Returns a 400 error if the tenant is not suspended or the state transition is invalid. Emits a tenant resumed audit event.
func handleAdminResumeTenant(svc tenantAdmin, emitter *tenant.AuditEmitter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		currentTenant, ok := lookupTenant(r, w, svc)
		if !ok {
			return
		}

		if currentTenant.State != tenant.TenantStateSuspended {
			httputil.WriteError(w, http.StatusBadRequest, "tenant is not suspended")
			return
		}

		updatedTenant, err := svc.TransitionState(r.Context(), currentTenant.ID, tenant.TenantStateSuspended, tenant.TenantStateActive)
		if err != nil {
			writeTenantTransitionError(w, err, "cannot resume tenant", "failed to resume tenant")
			return
		}

		emitTenantAuditEvent(r, svc, currentTenant.ID, tenant.AuditActionTenantResumed, map[string]string{
			"previousState": string(tenant.TenantStateSuspended),
		}, emitter)

		httputil.WriteJSON(w, http.StatusOK, updatedTenant)
	}
}

// Deletes a tenant by transitioning it to the deleting state and, for schema-isolated tenants, drops the associated schema. Returns a 400 error if the tenant is already deleted or the transition is invalid. Emits a tenant deleted audit event and logs schema deletion errors as non-fatal.
func handleAdminDeleteTenant(svc tenantAdmin, emitter *tenant.AuditEmitter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		currentTenant, ok := lookupTenant(r, w, svc)
		if !ok {
			return
		}

		if currentTenant.State == tenant.TenantStateDeleted {
			httputil.WriteError(w, http.StatusBadRequest, "tenant is already deleted")
			return
		}

		updatedTenant, err := svc.TransitionState(r.Context(), currentTenant.ID, currentTenant.State, tenant.TenantStateDeleting)
		if err != nil {
			writeTenantTransitionError(w, err, "cannot delete tenant", "failed to delete tenant")
			return
		}

		if currentTenant.IsolationMode == "schema" {
			if err := svc.DeleteTenantSchema(r.Context(), currentTenant.Slug); err != nil {
				slog.Error("failed to drop tenant schema", "tenant_id", currentTenant.ID, "slug", currentTenant.Slug, "error", err)
			}
		}

		emitTenantAuditEvent(r, svc, currentTenant.ID, tenant.AuditActionTenantDeleted, map[string]string{
			"name":          currentTenant.Name,
			"slug":          currentTenant.Slug,
			"previousState": string(currentTenant.State),
		}, emitter)

		httputil.WriteJSON(w, http.StatusOK, updatedTenant)
	}
}
