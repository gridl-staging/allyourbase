package tenant

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

const usageDailyColumns = `id, tenant_id, date, request_count, db_bytes_used, bandwidth_bytes, function_invocations, realtime_peak_connections, job_runs, created_at`

func normalizeUsageDate(t time.Time) time.Time {
	return t.UTC().Truncate(24 * time.Hour)
}

func scanUsageDaily(row pgx.Row) (*TenantUsageDaily, error) {
	var u TenantUsageDaily
	err := row.Scan(
		&u.ID, &u.TenantID, &u.Date, &u.RequestCount, &u.DBBytesUsed, &u.BandwidthBytes, &u.FunctionInvocations,
		&u.RealtimePeakConnections, &u.JobRuns, &u.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// scanUsageDailies unmarshals multiple tenant usage daily rows from a pgx.Rows query result into TenantUsageDaily structs, returning an empty slice if no rows are present.
func scanUsageDailies(rows pgx.Rows) ([]TenantUsageDaily, error) {
	var items []TenantUsageDaily
	for rows.Next() {
		u, err := scanUsageDaily(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *u)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if items == nil {
		items = []TenantUsageDaily{}
	}
	return items, nil
}

func (s *Service) GetDailyUsage(ctx context.Context, tenantID string, date time.Time) (*TenantUsageDaily, error) {
	day := normalizeUsageDate(date)
	usage, err := scanUsageDaily(s.pool.QueryRow(ctx,
		`SELECT `+usageDailyColumns+` FROM _ayb_tenant_usage_daily WHERE tenant_id = $1 AND date = $2::date`,
		tenantID, day,
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("getting daily usage: %w", err)
	}
	return usage, nil
}

// GetUsageRange retrieves daily usage records for a tenant between two dates inclusive, ordered chronologically. Returns an empty slice if endDate is before startDate.
func (s *Service) GetUsageRange(ctx context.Context, tenantID string, startDate, endDate time.Time) ([]TenantUsageDaily, error) {
	start := normalizeUsageDate(startDate)
	end := normalizeUsageDate(endDate)
	if end.Before(start) {
		return []TenantUsageDaily{}, nil
	}

	rows, err := s.pool.Query(ctx,
		`SELECT `+usageDailyColumns+` FROM _ayb_tenant_usage_daily
		 WHERE tenant_id = $1 AND date >= $2::date AND date <= $3::date
		 ORDER BY date ASC`,
		tenantID, start, end,
	)
	if err != nil {
		return nil, fmt.Errorf("getting usage range: %w", err)
	}
	defer rows.Close()

	return scanUsageDailies(rows)
}

const maintenanceColumns = `id, tenant_id, enabled, reason, enabled_at, enabled_by, created_at, updated_at`

func scanMaintenance(row pgx.Row) (*TenantMaintenanceState, error) {
	var m TenantMaintenanceState
	var reason *string
	var enabledBy *string
	var enabledAt *time.Time
	err := row.Scan(&m.ID, &m.TenantID, &m.Enabled, &reason, &enabledAt, &enabledBy, &m.CreatedAt, &m.UpdatedAt)
	if err != nil {
		return nil, err
	}
	m.Reason = reason
	m.EnabledBy = enabledBy
	m.EnabledAt = enabledAt
	return &m, nil
}

// EnableMaintenance activates maintenance mode for a tenant with an optional reason and actor ID, recording the timestamp and user who enabled it. Uses upsert logic to ensure idempotency.
func (s *Service) EnableMaintenance(ctx context.Context, tenantID string, reason string, actorID string) (*TenantMaintenanceState, error) {
	var actorPtr *string
	if actorID != "" {
		actorPtr = &actorID
	}
	var reasonPtr *string
	if reason != "" {
		reasonPtr = &reason
	}

	state, err := scanMaintenance(s.pool.QueryRow(ctx,
		`INSERT INTO _ayb_tenant_maintenance (tenant_id, enabled, reason, enabled_at, enabled_by)
		 VALUES ($1, true, $2, NOW(), $3)
		 ON CONFLICT (tenant_id) DO UPDATE SET
			enabled = true,
			reason = $2,
			enabled_at = NOW(),
			enabled_by = $3,
			updated_at = NOW()
		 RETURNING `+maintenanceColumns,
		tenantID, reasonPtr, actorPtr,
	))
	if err != nil {
		return nil, fmt.Errorf("enabling maintenance: %w", err)
	}

	s.logger.Info("maintenance enabled", "tenant_id", tenantID, "actor_id", actorID)
	return state, nil
}

// DisableMaintenance sets a tenant's maintenance mode to disabled. Returns nil if no maintenance record exists for the tenant.
func (s *Service) DisableMaintenance(ctx context.Context, tenantID string, actorID string) (*TenantMaintenanceState, error) {
	state, err := scanMaintenance(s.pool.QueryRow(ctx,
		`UPDATE _ayb_tenant_maintenance
		 SET enabled = false, updated_at = NOW()
		 WHERE tenant_id = $1
		 RETURNING `+maintenanceColumns,
		tenantID,
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("disabling maintenance: %w", err)
	}

	s.logger.Info("maintenance disabled", "tenant_id", tenantID, "actor_id", actorID)
	return state, nil
}

func (s *Service) GetMaintenanceState(ctx context.Context, tenantID string) (*TenantMaintenanceState, error) {
	state, err := scanMaintenance(s.pool.QueryRow(ctx,
		`SELECT `+maintenanceColumns+` FROM _ayb_tenant_maintenance WHERE tenant_id = $1`,
		tenantID,
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("getting maintenance state: %w", err)
	}
	return state, nil
}

func (s *Service) IsUnderMaintenance(ctx context.Context, tenantID string) (bool, error) {
	state, err := s.GetMaintenanceState(ctx, tenantID)
	if err != nil {
		return false, err
	}
	if state == nil {
		return false, nil
	}
	return state.Enabled, nil
}
