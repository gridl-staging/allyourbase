package storage

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/allyourbase/ayb/internal/audit"
	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/allyourbase/ayb/internal/tenant"
)

type tenantQuotaReader interface {
	GetQuotas(ctx context.Context, tenantID string) (*tenant.TenantQuotas, error)
}

type tenantQuotaMetricsRecorder interface {
	RecordQuotaUtilization(ctx context.Context, tenantID, resource string, current, limit int64)
	IncrQuotaViolation(ctx context.Context, tenantID, resource string)
}

type tenantQuotaAuditEmitter interface {
	EmitQuotaViolation(ctx context.Context, tenantID string, resource string, current, limit int64, actorID *string, ipAddress *string) error
}

func (h *Handler) SetTenantQuota(reader tenantQuotaReader, checker tenant.QuotaChecker, acc *tenant.UsageAccumulator) {
	h.tenantQuotaReader = reader
	h.tenantQuotaChecker = checker
	h.tenantUsageAccumulator = acc
}

func (h *Handler) SetTenantQuotaTelemetry(metrics tenantQuotaMetricsRecorder, auditEmitter tenantQuotaAuditEmitter) {
	h.tenantQuotaMetrics = metrics
	h.tenantQuotaAudit = auditEmitter
}

// applyTenantQuotaChecks validates whether a tenant can upload additional bytes within their storage quota, returning the soft warning flag, projected usage, and effective limit. Returns ErrQuotaExceeded if the projected usage would exceed the hard limit.
func (h *Handler) applyTenantQuotaChecks(ctx context.Context, tenantID string, additionalBytes int64) (bool, int64, int64, error) {
	if h.tenantQuotaReader == nil || h.tenantQuotaChecker == nil || h.tenantUsageAccumulator == nil {
		return false, 0, 0, nil
	}

	quotas, err := h.tenantQuotaReader.GetQuotas(ctx, tenantID)
	if err != nil {
		return false, 0, 0, fmt.Errorf("loading tenant quotas: %w", err)
	}
	if quotas == nil {
		return false, 0, 0, nil
	}

	currentUsage, err := h.tenantUsageAccumulator.GetCurrentUsage(ctx, tenantID, tenant.ResourceTypeDBSizeBytes)
	if err != nil {
		return false, 0, 0, fmt.Errorf("loading tenant storage usage: %w", err)
	}

	projectedUsage := currentUsage + additionalBytes
	if projectedUsage < 0 {
		projectedUsage = 0
	}
	limit := effectiveStorageLimit(quotas.DBSizeBytesHard, quotas.DBSizeBytesSoft)
	if h.tenantQuotaMetrics != nil && limit > 0 {
		h.tenantQuotaMetrics.RecordQuotaUtilization(ctx, tenantID, string(tenant.ResourceTypeDBSizeBytes), projectedUsage, limit)
	}

	decision := h.tenantQuotaChecker.CheckQuota(quotas, tenant.ResourceTypeDBSizeBytes, currentUsage, additionalBytes)
	if decision.HardLimited {
		if h.tenantQuotaMetrics != nil {
			h.tenantQuotaMetrics.IncrQuotaViolation(ctx, tenantID, string(tenant.ResourceTypeDBSizeBytes))
		}
		return false, projectedUsage, limit, ErrQuotaExceeded
	}
	return decision.SoftWarning, projectedUsage, limit, nil
}

func effectiveStorageLimit(hard, soft *int64) int64 {
	switch {
	case hard != nil:
		return *hard
	case soft != nil:
		return *soft
	default:
		return 0
	}
}

// emitTenantStorageQuotaViolation sends an audit event when a tenant exceeds
// their storage quota, attributing actor identity from trusted auth/audit
// context and client IP from the request.
func (h *Handler) emitTenantStorageQuotaViolation(r *http.Request, tenantID string, currentUsage, limit int64) {
	if h == nil || h.tenantQuotaAudit == nil || r == nil || tenantID == "" {
		return
	}
	actorID := quotaActorIDFromRequest(r)

	ipAddress := httputil.AuditIPFromRequest(r)

	_ = h.tenantQuotaAudit.EmitQuotaViolation(r.Context(), tenantID, string(tenant.ResourceTypeDBSizeBytes), currentUsage, limit, actorID, ipAddress)
}

func quotaActorIDFromRequest(r *http.Request) *string {
	if r == nil {
		return nil
	}
	if claims := auth.ClaimsFromContext(r.Context()); claims != nil {
		if actor := strings.TrimSpace(claims.Subject); httputil.IsValidUUID(actor) {
			return &actor
		}
	}
	if actor := strings.TrimSpace(audit.PrincipalFromContext(r.Context())); httputil.IsValidUUID(actor) {
		return &actor
	}
	return nil
}
