package server

import (
	"encoding/json"
	"net/http"

	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/allyourbase/ayb/internal/tenant"
)

type enableMaintenanceRequest struct {
	Reason string `json:"reason"`
}

type maintenanceStateResponse struct {
	*tenant.TenantMaintenanceState
}

// Enables maintenance mode for a tenant with a specified reason and actor ID for auditing purposes. Emits a maintenance enabled audit event if an emitter is configured.
func handleAdminEnableMaintenance(svc tenantAdmin, emitter *tenant.AuditEmitter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID, ok := tenantIDFromURL(r, w)
		if !ok {
			return
		}

		var req enableMaintenanceRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httputil.WriteError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		actorID := getActorID(r)
		var actorIDStr string
		if actorID != nil {
			actorIDStr = *actorID
		}
		state, err := svc.EnableMaintenance(r.Context(), tenantID, req.Reason, actorIDStr)
		if err != nil {
			httputil.WriteError(w, http.StatusInternalServerError, "failed to enable maintenance")
			return
		}

		if emitter != nil {
			ipAddress := getIPAddress(r)
			emitter.EmitMaintenanceEnabled(r.Context(), tenantID, req.Reason, actorID, ipAddress)
		}

		httputil.WriteJSON(w, http.StatusOK, maintenanceStateResponse{state})
	}
}

// Disables maintenance mode for a tenant. Returns a default maintenance state with Enabled=false if no state exists. Emits a maintenance disabled audit event if an emitter is configured.
func handleAdminDisableMaintenance(svc tenantAdmin, emitter *tenant.AuditEmitter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID, ok := tenantIDFromURL(r, w)
		if !ok {
			return
		}

		actorID := getActorID(r)
		var actorIDStr string
		if actorID != nil {
			actorIDStr = *actorID
		}
		state, err := svc.DisableMaintenance(r.Context(), tenantID, actorIDStr)
		if err != nil {
			httputil.WriteError(w, http.StatusInternalServerError, "failed to disable maintenance")
			return
		}

		if emitter != nil {
			ipAddress := getIPAddress(r)
			emitter.EmitMaintenanceDisabled(r.Context(), tenantID, actorID, ipAddress)
		}

		if state == nil {
			state = &tenant.TenantMaintenanceState{TenantID: tenantID, Enabled: false}
		}
		httputil.WriteJSON(w, http.StatusOK, maintenanceStateResponse{state})
	}
}

// Retrieves the maintenance state for a tenant. Returns a default state with Enabled=false if no state is configured.
func handleAdminGetMaintenance(svc tenantAdmin) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID, ok := tenantIDFromURL(r, w)
		if !ok {
			return
		}

		state, err := svc.GetMaintenanceState(r.Context(), tenantID)
		if err != nil {
			httputil.WriteError(w, http.StatusInternalServerError, "failed to get maintenance state")
			return
		}

		if state == nil {
			state = &tenant.TenantMaintenanceState{TenantID: tenantID, Enabled: false}
		}
		httputil.WriteJSON(w, http.StatusOK, maintenanceStateResponse{state})
	}
}

type breakerStateResponse struct {
	State               string `json:"state"`
	ConsecutiveFailures int    `json:"consecutiveFailures"`
	HalfOpenProbes      int    `json:"halfOpenProbes"`
}

func breakerSnapshotToResponse(snap tenant.BreakerSnapshot) breakerStateResponse {
	return breakerStateResponse{
		State:               string(snap.State),
		ConsecutiveFailures: snap.ConsecutiveFailures,
		HalfOpenProbes:      snap.HalfOpenProbes,
	}
}

func handleAdminGetBreaker(breakerTracker *tenant.TenantBreakerTracker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID, ok := tenantIDFromURL(r, w)
		if !ok {
			return
		}

		httputil.WriteJSON(w, http.StatusOK, breakerSnapshotToResponse(breakerTracker.StateSnapshot(tenantID)))
	}
}

// Resets the circuit breaker state for a tenant, clearing consecutive failures and probe counters. Emits a breaker reset audit event if an emitter is configured.
func handleAdminResetBreaker(emitter *tenant.AuditEmitter, breakerTracker *tenant.TenantBreakerTracker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID, ok := tenantIDFromURL(r, w)
		if !ok {
			return
		}

		breakerTracker.ResetBreaker(tenantID)

		if emitter != nil {
			actorID := getActorID(r)
			ipAddress := getIPAddress(r)
			emitter.EmitBreakerReset(r.Context(), tenantID, actorID, ipAddress)
		}

		httputil.WriteJSON(w, http.StatusOK, breakerSnapshotToResponse(breakerTracker.StateSnapshot(tenantID)))
	}
}
