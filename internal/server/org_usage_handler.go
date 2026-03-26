package server

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/allyourbase/ayb/internal/billing"
	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/allyourbase/ayb/internal/tenant"
	"github.com/jackc/pgx/v5/pgxpool"
)

// OrgUsageSummary is the response shape for org-level usage endpoints.
type OrgUsageSummary struct {
	OrgID       string                  `json:"orgId"`
	TenantCount int                     `json:"tenantCount"`
	Period      string                  `json:"period"`
	Data        []billing.UsageDayEntry `json:"data"`
	Totals      billing.UsageTotals     `json:"totals"`
}

// orgUsageQuerier queries org-level aggregated usage directly from the database.
type orgUsageQuerier interface {
	GetOrgUsageRange(ctx context.Context, orgID string, start, end time.Time) ([]tenant.TenantUsageDaily, int, error)
}

// dbOrgUsageQuerier implements orgUsageQuerier using the database pool.
type dbOrgUsageQuerier struct {
	pool *pgxpool.Pool
}

// handleAdminOrgUsage returns usage aggregated across all tenants in an org.
func handleAdminOrgUsage(store tenant.OrgStore, querier orgUsageQuerier) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		org, ok := lookupOrg(r, w, store)
		if !ok {
			return
		}

		if querier == nil {
			httputil.WriteError(w, http.StatusServiceUnavailable, "usage service not configured")
			return
		}

		period, start, end, err := resolveUsageQueryRange(r, time.Now().UTC())
		if err != nil {
			httputil.WriteError(w, http.StatusBadRequest, "invalid period or date range")
			return
		}

		rows, tenantCount, err := querier.GetOrgUsageRange(r.Context(), org.ID, start, end)
		if err != nil {
			httputil.WriteError(w, http.StatusInternalServerError, "failed to load usage")
			return
		}

		summary := buildOrgUsageSummary(org.ID, period, rows, tenantCount)
		httputil.WriteJSON(w, http.StatusOK, summary)
	}
}

// buildOrgUsageSummary constructs an OrgUsageSummary from aggregated daily rows.
func buildOrgUsageSummary(orgID, period string, rows []tenant.TenantUsageDaily, tenantCount int) *OrgUsageSummary {
	summary := &OrgUsageSummary{
		OrgID:       orgID,
		TenantCount: tenantCount,
		Period:      period,
		Data:        make([]billing.UsageDayEntry, 0, len(rows)),
	}

	for _, row := range rows {
		entry := billing.UsageDayEntry{
			Date:                row.Date.UTC().Format(time.DateOnly),
			APIRequests:         row.RequestCount,
			StorageBytesUsed:    row.DBBytesUsed,
			BandwidthBytes:      row.BandwidthBytes,
			FunctionInvocations: row.FunctionInvocations,
		}
		summary.Data = append(summary.Data, entry)

		summary.Totals.APIRequests += row.RequestCount
		if row.DBBytesUsed > summary.Totals.StorageBytesUsed {
			summary.Totals.StorageBytesUsed = row.DBBytesUsed
		}
		summary.Totals.BandwidthBytes += row.BandwidthBytes
		summary.Totals.FunctionInvocations += row.FunctionInvocations
	}

	return summary
}

// GetOrgUsageRange aggregates _ayb_tenant_usage_daily across all tenants in an org.
func (q *dbOrgUsageQuerier) GetOrgUsageRange(ctx context.Context, orgID string, start, end time.Time) ([]tenant.TenantUsageDaily, int, error) {
	var tenantCount int
	err := q.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM _ayb_tenants WHERE org_id = $1`, orgID,
	).Scan(&tenantCount)
	if err != nil {
		return nil, 0, fmt.Errorf("counting org tenants for usage: %w", err)
	}

	rows, err := q.pool.Query(ctx,
		`SELECT
			'' AS id, '' AS tenant_id, u.date,
			COALESCE(SUM(u.request_count), 0),
			COALESCE(SUM(u.db_bytes_used), 0),
			COALESCE(SUM(u.bandwidth_bytes), 0),
			COALESCE(SUM(u.function_invocations), 0),
			COALESCE(SUM(u.realtime_peak_connections), 0),
			COALESCE(SUM(u.job_runs), 0),
			MIN(u.created_at)
		 FROM _ayb_tenant_usage_daily u
		 JOIN _ayb_tenants t ON t.id = u.tenant_id
		 WHERE t.org_id = $1 AND u.date >= $2 AND u.date <= $3
		 GROUP BY u.date ORDER BY u.date ASC`,
		orgID, start, end,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("querying org usage: %w", err)
	}
	defer rows.Close()

	var items []tenant.TenantUsageDaily
	for rows.Next() {
		var item tenant.TenantUsageDaily
		if err := rows.Scan(
			&item.ID, &item.TenantID, &item.Date,
			&item.RequestCount, &item.DBBytesUsed, &item.BandwidthBytes,
			&item.FunctionInvocations, &item.RealtimePeakConnections, &item.JobRuns,
			&item.CreatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scanning org usage row: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterating org usage rows: %w", err)
	}
	if items == nil {
		items = []tenant.TenantUsageDaily{}
	}

	return items, tenantCount, nil
}
