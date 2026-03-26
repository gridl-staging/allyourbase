// Package tenant Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/internal/tenant/audit.go.
package tenant

import (
	"context"
	"encoding/json"
	"log/slog"
)

const (
	AuditActionTenantCreated           = "tenant.created"
	AuditActionTenantUpdated           = "tenant.updated"
	AuditActionTenantSuspended         = "tenant.suspended"
	AuditActionTenantResumed           = "tenant.resumed"
	AuditActionTenantDeleted           = "tenant.deleted"
	AuditActionTenantAssignedToOrg     = "tenant.assigned_to_org"
	AuditActionTenantUnassignedFromOrg = "tenant.unassigned_from_org"
	AuditActionMembershipAdded         = "membership.added"
	AuditActionMembershipRemoved       = "membership.removed"
	AuditActionMembershipRoleChange    = "membership.role_change"
	AuditActionQuotaChange             = "quota.change"
	AuditActionQuotaViolation          = "quota.violation"
	AuditActionCrossTenantBlocked      = "cross_tenant.blocked"
	AuditActionMaintenanceEnabled      = "maintenance.enabled"
	AuditActionMaintenanceDisabled     = "maintenance.disabled"
	AuditActionBreakerOpened           = "breaker.opened"
	AuditActionBreakerClosed           = "breaker.closed"
	AuditActionBreakerReset            = "breaker.reset"
)

const (
	AuditResultSuccess = "success"
	AuditResultFailure = "failure"
	AuditResultDenied  = "denied"
)

type auditInserter interface {
	InsertAuditEvent(ctx context.Context, tenantID string, actorID *string, action, result string, metadata json.RawMessage, ipAddress *string) error
}

// orgAwareAuditInserter extends auditInserter with org_id support.
// Implementations that support org_id should implement this interface.
type orgAwareAuditInserter interface {
	auditInserter
	InsertAuditEventWithOrg(ctx context.Context, tenantID string, actorID *string, action, result string, metadata json.RawMessage, ipAddress *string, orgID *string) error
}

type tenantOrgResolver interface {
	TenantOrgID(ctx context.Context, tenantID string) (*string, error)
}

type AuditEmitter struct {
	inserter auditInserter
	logger   *slog.Logger
}

func NewAuditEmitter(svc *Service, logger *slog.Logger) *AuditEmitter {
	return &AuditEmitter{
		inserter: svc,
		logger:   logger,
	}
}

func NewAuditEmitterWithInserter(inserter auditInserter, logger *slog.Logger) *AuditEmitter {
	return &AuditEmitter{
		inserter: inserter,
		logger:   logger,
	}
}

// Emit writes an audit event with the specified action and result, converting metadata to JSON as needed and logging errors if a logger is configured.
func (e *AuditEmitter) Emit(ctx context.Context, tenantID, action, result string, actorID *string, metadata any, ipAddress *string) error {
	metaBytes := e.marshalMetadata(tenantID, action, metadata)
	return e.insertEvent(ctx, tenantID, actorID, action, result, metaBytes, ipAddress, e.resolveTenantOrgID(ctx, tenantID, action))
}

// EmitWithOrg writes an audit event with an optional org_id for org-scoped queries.
// If the inserter supports org_id, the column is populated; otherwise falls back to Emit.
func (e *AuditEmitter) EmitWithOrg(ctx context.Context, tenantID, action, result string, actorID *string, metadata any, ipAddress *string, orgID *string) error {
	metaBytes := e.marshalMetadata(tenantID, action, metadata)
	return e.insertEvent(ctx, tenantID, actorID, action, result, metaBytes, ipAddress, orgID)
}

// TODO: Document AuditEmitter.insertEvent.
func (e *AuditEmitter) insertEvent(ctx context.Context, tenantID string, actorID *string, action, result string, metadata json.RawMessage, ipAddress *string, orgID *string) error {
	if orgID != nil {
		if orgInserter, ok := e.inserter.(orgAwareAuditInserter); ok {
			err := orgInserter.InsertAuditEventWithOrg(ctx, tenantID, actorID, action, result, metadata, ipAddress, orgID)
			if err != nil && e.logger != nil {
				e.logger.Error("audit event write failed", "error", err, "tenant_id", tenantID, "action", action, "result", result)
			}
			return err
		}
	}

	err := e.inserter.InsertAuditEvent(ctx, tenantID, actorID, action, result, metadata, ipAddress)
	if err != nil && e.logger != nil {
		e.logger.Error("audit event write failed", "error", err, "tenant_id", tenantID, "action", action, "result", result)
	}
	return err
}

// TODO: Document AuditEmitter.resolveTenantOrgID.
func (e *AuditEmitter) resolveTenantOrgID(ctx context.Context, tenantID, action string) *string {
	if tenantID == "" {
		return nil
	}
	if _, ok := e.inserter.(orgAwareAuditInserter); !ok {
		return nil
	}
	resolver, ok := e.inserter.(tenantOrgResolver)
	if !ok {
		return nil
	}

	orgID, err := resolver.TenantOrgID(ctx, tenantID)
	if err != nil {
		if e.logger != nil {
			e.logger.Error("failed to resolve tenant org for audit event", "error", err, "tenant_id", tenantID, "action", action)
		}
		return nil
	}
	return orgID
}

// marshalMetadata converts metadata to json.RawMessage.
func (e *AuditEmitter) marshalMetadata(tenantID, action string, metadata any) json.RawMessage {
	switch m := metadata.(type) {
	case json.RawMessage:
		return m
	case nil:
		return json.RawMessage("{}")
	default:
		data, err := json.Marshal(m)
		if err != nil {
			if e.logger != nil {
				e.logger.Error("failed to marshal audit metadata", "error", err, "tenant_id", tenantID, "action", action)
			}
			return json.RawMessage("{}")
		}
		return data
	}
}

// BuildMetadata constructs a JSON object from alternating key-value string pairs, returning an empty object if no pairs are provided or if marshaling fails.
func (e *AuditEmitter) BuildMetadata(pairs ...string) json.RawMessage {
	if len(pairs) == 0 {
		return json.RawMessage("{}")
	}

	m := make(map[string]string)
	for i := 0; i < len(pairs)-1; i += 2 {
		m[pairs[i]] = pairs[i+1]
	}

	data, err := json.Marshal(m)
	if err != nil {
		return json.RawMessage("{}")
	}
	return data
}

func (e *AuditEmitter) EmitTenantCreated(ctx context.Context, tenantID string, actorID *string, name string, ipAddress *string) error {
	meta := e.BuildMetadata("name", name)
	return e.Emit(ctx, tenantID, AuditActionTenantCreated, AuditResultSuccess, actorID, meta, ipAddress)
}

func (e *AuditEmitter) EmitTenantUpdated(ctx context.Context, tenantID string, actorID *string, changes map[string]any, ipAddress *string) error {
	meta := map[string]any{"changes": changes}
	return e.Emit(ctx, tenantID, AuditActionTenantUpdated, AuditResultSuccess, actorID, meta, ipAddress)
}

func (e *AuditEmitter) EmitTenantSuspended(ctx context.Context, tenantID string, actorID *string, ipAddress *string) error {
	return e.Emit(ctx, tenantID, AuditActionTenantSuspended, AuditResultSuccess, actorID, nil, ipAddress)
}

func (e *AuditEmitter) EmitTenantResumed(ctx context.Context, tenantID string, actorID *string, ipAddress *string) error {
	return e.Emit(ctx, tenantID, AuditActionTenantResumed, AuditResultSuccess, actorID, nil, ipAddress)
}

func (e *AuditEmitter) EmitTenantDeleted(ctx context.Context, tenantID string, actorID *string, ipAddress *string) error {
	return e.Emit(ctx, tenantID, AuditActionTenantDeleted, AuditResultSuccess, actorID, nil, ipAddress)
}

func (e *AuditEmitter) EmitTenantAssignedToOrg(ctx context.Context, tenantID, orgID string, actorID *string, ipAddress *string) error {
	meta := e.BuildMetadata("orgId", orgID)
	return e.EmitWithOrg(ctx, tenantID, AuditActionTenantAssignedToOrg, AuditResultSuccess, actorID, meta, ipAddress, &orgID)
}

func (e *AuditEmitter) EmitTenantUnassignedFromOrg(ctx context.Context, tenantID, orgID string, actorID *string, ipAddress *string) error {
	meta := e.BuildMetadata("orgId", orgID)
	return e.EmitWithOrg(ctx, tenantID, AuditActionTenantUnassignedFromOrg, AuditResultSuccess, actorID, meta, ipAddress, &orgID)
}

func (e *AuditEmitter) EmitMembershipAdded(ctx context.Context, tenantID, userID, role string, actorID *string, ipAddress *string) error {
	meta := e.BuildMetadata("userId", userID, "role", role)
	return e.Emit(ctx, tenantID, AuditActionMembershipAdded, AuditResultSuccess, actorID, meta, ipAddress)
}

func (e *AuditEmitter) EmitMembershipRemoved(ctx context.Context, tenantID, userID string, actorID *string, ipAddress *string) error {
	meta := e.BuildMetadata("userId", userID)
	return e.Emit(ctx, tenantID, AuditActionMembershipRemoved, AuditResultSuccess, actorID, meta, ipAddress)
}

func (e *AuditEmitter) EmitMembershipRoleChange(ctx context.Context, tenantID, userID, oldRole, newRole string, actorID *string, ipAddress *string) error {
	meta := e.BuildMetadata("userId", userID, "oldRole", oldRole, "newRole", newRole)
	return e.Emit(ctx, tenantID, AuditActionMembershipRoleChange, AuditResultSuccess, actorID, meta, ipAddress)
}

func (e *AuditEmitter) EmitQuotaChange(ctx context.Context, tenantID string, changes map[string]any, actorID *string, ipAddress *string) error {
	meta := map[string]any{"changes": changes}
	return e.Emit(ctx, tenantID, AuditActionQuotaChange, AuditResultSuccess, actorID, meta, ipAddress)
}

func (e *AuditEmitter) EmitQuotaViolation(ctx context.Context, tenantID string, resource string, current, limit int64, actorID *string, ipAddress *string) error {
	meta := map[string]any{
		"resource": resource,
		"current":  current,
		"limit":    limit,
	}
	return e.Emit(ctx, tenantID, AuditActionQuotaViolation, AuditResultDenied, actorID, meta, ipAddress)
}

func (e *AuditEmitter) EmitCrossTenantBlocked(ctx context.Context, tenantID, targetTenantID, resourceID, operation string, actorID *string, ipAddress *string) error {
	meta := e.BuildMetadata("targetTenantId", targetTenantID, "resourceId", resourceID, "operation", operation)
	return e.Emit(ctx, tenantID, AuditActionCrossTenantBlocked, AuditResultDenied, actorID, meta, ipAddress)
}

func (e *AuditEmitter) EmitMaintenanceEnabled(ctx context.Context, tenantID, reason string, actorID *string, ipAddress *string) error {
	meta := e.BuildMetadata("reason", reason)
	return e.Emit(ctx, tenantID, AuditActionMaintenanceEnabled, AuditResultSuccess, actorID, meta, ipAddress)
}

func (e *AuditEmitter) EmitMaintenanceDisabled(ctx context.Context, tenantID string, actorID *string, ipAddress *string) error {
	return e.Emit(ctx, tenantID, AuditActionMaintenanceDisabled, AuditResultSuccess, actorID, nil, ipAddress)
}

func (e *AuditEmitter) EmitBreakerOpened(ctx context.Context, tenantID string, consecutiveFailures int) error {
	meta := map[string]any{"consecutiveFailures": consecutiveFailures}
	return e.Emit(ctx, tenantID, AuditActionBreakerOpened, AuditResultDenied, nil, meta, nil)
}

func (e *AuditEmitter) EmitBreakerClosed(ctx context.Context, tenantID string) error {
	return e.Emit(ctx, tenantID, AuditActionBreakerClosed, AuditResultSuccess, nil, nil, nil)
}

func (e *AuditEmitter) EmitBreakerReset(ctx context.Context, tenantID string, actorID *string, ipAddress *string) error {
	return e.Emit(ctx, tenantID, AuditActionBreakerReset, AuditResultSuccess, actorID, nil, ipAddress)
}
