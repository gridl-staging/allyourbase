// Package tenant provides quota management and enforcement for tenant resource limits.
package tenant

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

type ResourceType string

const (
	ResourceTypeRequestRate         ResourceType = "request_rate_rps"
	ResourceTypeDBSizeBytes         ResourceType = "db_size_bytes"
	ResourceTypeRealtimeConns       ResourceType = "realtime_connections"
	ResourceTypeJobConcurrency      ResourceType = "job_concurrency"
	ResourceTypeBandwidthBytes      ResourceType = "bandwidth_bytes"
	ResourceTypeFunctionInvocations ResourceType = "function_invocations"
)

type QuotaDecision struct {
	Allowed     bool
	HardLimited bool
	SoftWarning bool
	Limit       int64
	Current     int64
	Proposed    int64
	Message     string
}

// intToInt64Ptr converts an *int to *int64, returning nil for nil input.
func intToInt64Ptr(v *int) *int64 {
	if v == nil {
		return nil
	}
	n := int64(*v)
	return &n
}

// CheckQuota evaluates whether a proposed resource usage would exceed configured hard or soft quota limits for a given resource type. It returns a QuotaDecision containing the evaluation result, limit values, and a descriptive message. If quotas is nil, the operation is allowed. Hard limits block operations; soft limits trigger warnings but allow the request to proceed.
func CheckQuota(quotas *TenantQuotas, resource ResourceType, currentUsage, proposed int64) QuotaDecision {
	if quotas == nil {
		return QuotaDecision{
			Allowed:  true,
			Current:  currentUsage,
			Proposed: proposed,
			Message:  "no quota configured - unlimited",
		}
	}

	var hardLimit, softLimit *int64

	switch resource {
	case ResourceTypeRequestRate:
		hardLimit = intToInt64Ptr(quotas.RequestRateRPSHard)
		softLimit = intToInt64Ptr(quotas.RequestRateRPSSoft)
	case ResourceTypeDBSizeBytes:
		hardLimit = quotas.DBSizeBytesHard
		softLimit = quotas.DBSizeBytesSoft
	case ResourceTypeRealtimeConns:
		hardLimit = intToInt64Ptr(quotas.RealtimeConnectionsHard)
		softLimit = intToInt64Ptr(quotas.RealtimeConnectionsSoft)
	case ResourceTypeJobConcurrency:
		hardLimit = intToInt64Ptr(quotas.JobConcurrencyHard)
		softLimit = intToInt64Ptr(quotas.JobConcurrencySoft)
	}

	if hardLimit == nil && softLimit == nil {
		return QuotaDecision{
			Allowed:  true,
			Current:  currentUsage,
			Proposed: proposed,
			Message:  "no quota configured for this resource - unlimited",
		}
	}

	totalProposed := currentUsage + proposed

	if hardLimit != nil && totalProposed >= *hardLimit {
		return QuotaDecision{
			Allowed:     false,
			HardLimited: true,
			Limit:       *hardLimit,
			Current:     currentUsage,
			Proposed:    proposed,
			Message:     fmt.Sprintf("hard limit exceeded: proposed %d >= limit %d", totalProposed, *hardLimit),
		}
	}

	if softLimit != nil && totalProposed >= *softLimit {
		return QuotaDecision{
			Allowed:     true,
			SoftWarning: true,
			Limit:       *softLimit,
			Current:     currentUsage,
			Proposed:    proposed,
			Message:     fmt.Sprintf("soft threshold warning: proposed %d >= threshold %d", totalProposed, *softLimit),
		}
	}

	return QuotaDecision{
		Allowed:  true,
		Current:  currentUsage,
		Proposed: proposed,
		Message:  "under quota",
	}
}

type QuotaChecker interface {
	CheckQuota(quotas *TenantQuotas, resource ResourceType, currentUsage, proposed int64) QuotaDecision
}

type DefaultQuotaChecker struct{}

func (DefaultQuotaChecker) CheckQuota(quotas *TenantQuotas, resource ResourceType, currentUsage, proposed int64) QuotaDecision {
	return CheckQuota(quotas, resource, currentUsage, proposed)
}

const quotasColumns = `id, tenant_id, db_size_bytes_hard, db_size_bytes_soft, request_rate_rps_hard, 
	request_rate_rps_soft, realtime_connections_hard, realtime_connections_soft, 
	job_concurrency_hard, job_concurrency_soft, created_at, updated_at`

func scanQuotas(row pgx.Row) (*TenantQuotas, error) {
	var q TenantQuotas
	err := row.Scan(
		&q.ID, &q.TenantID, &q.DBSizeBytesHard, &q.DBSizeBytesSoft,
		&q.RequestRateRPSHard, &q.RequestRateRPSSoft,
		&q.RealtimeConnectionsHard, &q.RealtimeConnectionsSoft,
		&q.JobConcurrencyHard, &q.JobConcurrencySoft,
		&q.CreatedAt, &q.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &q, nil
}

// SetQuotas inserts or updates quota limits for a tenant in the database. It creates a new quota record or updates an existing one, persisting all limit configurations. The operation is logged and the resulting quota configuration is returned.
func (s *Service) SetQuotas(ctx context.Context, tenantID string, q TenantQuotas) (*TenantQuotas, error) {
	q.TenantID = tenantID
	q.ID = ""

	quota, err := scanQuotas(s.pool.QueryRow(ctx,
		`INSERT INTO _ayb_tenant_quotas 
			(tenant_id, db_size_bytes_hard, db_size_bytes_soft, request_rate_rps_hard, 
			request_rate_rps_soft, realtime_connections_hard, realtime_connections_soft, 
			job_concurrency_hard, job_concurrency_soft)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		 ON CONFLICT (tenant_id) DO UPDATE SET
			db_size_bytes_hard = EXCLUDED.db_size_bytes_hard,
			db_size_bytes_soft = EXCLUDED.db_size_bytes_soft,
			request_rate_rps_hard = EXCLUDED.request_rate_rps_hard,
			request_rate_rps_soft = EXCLUDED.request_rate_rps_soft,
			realtime_connections_hard = EXCLUDED.realtime_connections_hard,
			realtime_connections_soft = EXCLUDED.realtime_connections_soft,
			job_concurrency_hard = EXCLUDED.job_concurrency_hard,
			job_concurrency_soft = EXCLUDED.job_concurrency_soft,
			updated_at = NOW()
		 RETURNING `+quotasColumns,
		tenantID, q.DBSizeBytesHard, q.DBSizeBytesSoft,
		q.RequestRateRPSHard, q.RequestRateRPSSoft,
		q.RealtimeConnectionsHard, q.RealtimeConnectionsSoft,
		q.JobConcurrencyHard, q.JobConcurrencySoft,
	))
	if err != nil {
		return nil, fmt.Errorf("setting quotas: %w", err)
	}

	s.logger.Info("quotas set", "tenant_id", tenantID)
	return quota, nil
}

func (s *Service) GetQuotas(ctx context.Context, tenantID string) (*TenantQuotas, error) {
	quota, err := scanQuotas(s.pool.QueryRow(ctx,
		`SELECT `+quotasColumns+` FROM _ayb_tenant_quotas WHERE tenant_id = $1`,
		tenantID,
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("getting quotas: %w", err)
	}
	return quota, nil
}
