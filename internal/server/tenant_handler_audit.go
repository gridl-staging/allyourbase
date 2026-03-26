package server

import (
	"encoding/json"
	"net/http"

	"github.com/allyourbase/ayb/internal/audit"
	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/allyourbase/ayb/internal/tenant"
)

func getActorID(r *http.Request) *string {
	if r == nil {
		return nil
	}
	if claims := auth.ClaimsFromContext(r.Context()); claims != nil {
		if actor := nonEmptyTrimmed(claims.Subject); actor != nil && httputil.IsValidUUID(*actor) {
			return actor
		}
	}
	if actor := nonEmptyTrimmed(audit.PrincipalFromContext(r.Context())); actor != nil && httputil.IsValidUUID(*actor) {
		return actor
	}
	return nil
}

func getIPAddress(r *http.Request) *string {
	return httputil.AuditIPFromRequest(r)
}

// emitTenantAuditEvent is a convenience wrapper that marshals metadata, extracts
// actor/IP from the request, and fires an audit event via the centralized emitter
// if available, or falls back to direct InsertAuditEvent for backward compatibility.
// The error from audit emission is intentionally discarded — audit failures must not block
// business operations.
func emitTenantAuditEvent(r *http.Request, svc tenantAdmin, tenantID, action string, meta any, emitter *tenant.AuditEmitter) {
	actorID := getActorID(r)
	ipAddress := getIPAddress(r)

	if emitter != nil {
		switch action {
		case tenant.AuditActionTenantCreated:
			name := ""
			if m, ok := meta.(map[string]string); ok {
				name = m["name"]
			}
			emitter.EmitTenantCreated(r.Context(), tenantID, actorID, name, ipAddress)
		case tenant.AuditActionTenantUpdated:
			emitter.EmitTenantUpdated(r.Context(), tenantID, actorID, auditMetaToAnyMap(meta), ipAddress)
		case tenant.AuditActionTenantSuspended:
			emitter.EmitTenantSuspended(r.Context(), tenantID, actorID, ipAddress)
		case tenant.AuditActionTenantResumed:
			emitter.EmitTenantResumed(r.Context(), tenantID, actorID, ipAddress)
		case tenant.AuditActionTenantDeleted:
			emitter.EmitTenantDeleted(r.Context(), tenantID, actorID, ipAddress)
		case tenant.AuditActionMembershipAdded:
			userID, role := "", ""
			if m, ok := meta.(map[string]string); ok {
				userID = m["userId"]
				role = m["role"]
			}
			emitter.EmitMembershipAdded(r.Context(), tenantID, userID, role, actorID, ipAddress)
		case tenant.AuditActionMembershipRemoved:
			userID := ""
			if m, ok := meta.(map[string]string); ok {
				userID = m["userId"]
			}
			emitter.EmitMembershipRemoved(r.Context(), tenantID, userID, actorID, ipAddress)
		case tenant.AuditActionMembershipRoleChange:
			userID, newRole := "", ""
			if m, ok := meta.(map[string]string); ok {
				userID = m["userId"]
				newRole = m["role"]
			}
			emitter.EmitMembershipRoleChange(r.Context(), tenantID, userID, "", newRole, actorID, ipAddress)
		case tenant.AuditActionQuotaChange:
			emitter.EmitQuotaChange(r.Context(), tenantID, auditMetaToAnyMap(meta), actorID, ipAddress)
		case tenant.AuditActionQuotaViolation:
			resource, current, limit := "", int64(0), int64(0)
			if m, ok := meta.(map[string]any); ok {
				if v, ok := m["resource"].(string); ok {
					resource = v
				}
				if v, ok := m["current"].(int64); ok {
					current = v
				}
				if v, ok := m["limit"].(int64); ok {
					limit = v
				}
			}
			emitter.EmitQuotaViolation(r.Context(), tenantID, resource, current, limit, actorID, ipAddress)
		case tenant.AuditActionCrossTenantBlocked:
			targetTenantID, resourceID, operation := "", "", ""
			if m, ok := meta.(map[string]string); ok {
				targetTenantID = m["targetTenantId"]
				resourceID = m["resourceId"]
				operation = m["operation"]
			}
			emitter.EmitCrossTenantBlocked(r.Context(), tenantID, targetTenantID, resourceID, operation, actorID, ipAddress)
		default:
			data, _ := json.Marshal(meta)
			svc.InsertAuditEvent(r.Context(), tenantID, actorID, action, "success", data, ipAddress)
		}
		return
	}

	data, _ := json.Marshal(meta)
	svc.InsertAuditEvent(r.Context(), tenantID, actorID, action, "success", data, ipAddress)
}

func auditMetaToAnyMap(meta any) map[string]any {
	switch m := meta.(type) {
	case map[string]any:
		return m
	case map[string]string:
		changes := make(map[string]any, len(m))
		for key, value := range m {
			changes[key] = value
		}
		return changes
	default:
		return map[string]any{}
	}
}
