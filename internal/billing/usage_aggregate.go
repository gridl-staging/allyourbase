// Package billing Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/internal/billing/usage_aggregate.go.
package billing

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	defaultListLimit      = 50
	maxListLimit          = 500
	defaultBreakdownLimit = 10
	maxBreakdownLimit     = 100
)

// TenantUsageSummaryRow is one row in a paginated multi-tenant usage listing.
type TenantUsageSummaryRow struct {
	TenantID                string `json:"tenantId"`
	TenantName              string `json:"tenantName"`
	RequestCount            int64  `json:"requestCount"`
	StorageBytesUsed        int64  `json:"storageBytesUsed"`
	BandwidthBytes          int64  `json:"bandwidthBytes"`
	FunctionInvocations     int64  `json:"functionInvocations"`
	RealtimePeakConnections int64  `json:"realtimePeakConnections"`
	JobRuns                 int64  `json:"jobRuns"`
}

// TrendPoint is one data point in a time-series trend response.
type TrendPoint struct {
	Timestamp time.Time `json:"timestamp"`
	Value     int64     `json:"value"`
}

// BreakdownEntry is one row in a top-N breakdown response.
type BreakdownEntry struct {
	Key   string `json:"key"`
	Value int64  `json:"value"`
}

// MetricLimit holds used/limit/remaining for a single metric.
type MetricLimit struct {
	Limit     int64 `json:"limit"`
	Used      int64 `json:"used"`
	Remaining int64 `json:"remaining"`
}

// UsageLimitsResponse holds plan and per-metric usage limits.
type UsageLimitsResponse struct {
	Plan    Plan                   `json:"plan"`
	Metrics map[string]MetricLimit `json:"metrics"`
}

// UsageSort captures sort column and direction for listings.
type UsageSort struct {
	Column    string
	Direction string
}

// ListUsageOpts holds validated parameters for multi-tenant usage listing.
type ListUsageOpts struct {
	Period string
	From   time.Time
	To     time.Time
	Sort   UsageSort
	Limit  int
	Offset int
}

// TrendOpts holds validated parameters for time-series trend queries.
type TrendOpts struct {
	Metric      string
	Granularity string
	From        time.Time
	To          time.Time
}

// BreakdownOpts holds validated parameters for top-N breakdown queries.
type BreakdownOpts struct {
	Metric  string
	GroupBy string
	From    time.Time
	To      time.Time
	Limit   int
}

type metricDef struct {
	column      string
	aggregation string
}

var metricDefinitions = map[string]metricDef{
	"api_requests":         {column: "request_count", aggregation: "SUM"},
	"storage_bytes":        {column: "db_bytes_used", aggregation: "MAX"},
	"bandwidth_bytes":      {column: "bandwidth_bytes", aggregation: "SUM"},
	"function_invocations": {column: "function_invocations", aggregation: "SUM"},
}

var sortColumnMap = map[string]string{
	"request_count":             "request_count",
	"storage_bytes":             "storage_bytes_used",
	"storage_bytes_used":        "storage_bytes_used",
	"bandwidth_bytes":           "bandwidth_bytes",
	"function_invocations":      "function_invocations",
	"realtime_peak_connections": "realtime_peak_connections",
	"job_runs":                  "job_runs",
	"tenant_id":                 "tenant_id",
	"tenant_name":               "tenant_name",
}

var validSortDirections = map[string]string{
	"asc":  "ASC",
	"desc": "DESC",
	"":     "DESC",
}

var validGroupBys = map[string]bool{
	"tenant":      true,
	"endpoint":    true,
	"status_code": true,
}

var validGranularities = map[string]bool{
	"day":   true,
	"week":  true,
	"month": true,
}

// ValidateMetric checks whether m is a recognized usage metric name.
func ValidateMetric(m string) error {
	if _, ok := metricDefinitions[m]; !ok {
		return fmt.Errorf("invalid metric %q", m)
	}
	return nil
}

// ValidateGroupBy checks whether g is a recognized group_by value.
func ValidateGroupBy(g string) error {
	if !validGroupBys[g] {
		return fmt.Errorf("invalid group_by %q", g)
	}
	return nil
}

// ValidateGranularity checks whether g is valid for the given metric.
// Hourly granularity is only available for api_requests (via _ayb_request_logs).
func ValidateGranularity(g, metric string) error {
	if g == "hour" {
		if metric == "api_requests" {
			return nil
		}
		return fmt.Errorf("hourly granularity is only available for api_requests")
	}
	if !validGranularities[g] {
		return fmt.Errorf("invalid granularity %q", g)
	}
	return nil
}

// ValidateSortSpec validates a sort column and direction, returning safe SQL
// column name and direction constants. Defaults to request_count DESC.
func ValidateSortSpec(column, direction string) (string, string, error) {
	column = strings.TrimSpace(column)
	direction = strings.TrimSpace(strings.ToLower(direction))
	if column == "" {
		if direction != "" {
			return "", "", fmt.Errorf("invalid sort column %q", column)
		}
		return "request_count", "DESC", nil
	}
	col, ok := sortColumnMap[column]
	if !ok {
		return "", "", fmt.Errorf("invalid sort column %q", column)
	}
	dir, ok := validSortDirections[direction]
	if !ok {
		return "", "", fmt.Errorf("invalid sort direction %q", direction)
	}
	return col, dir, nil
}

// ParseUsageSort parses a sort query parameter into validated SQL-safe
// column and direction constants. Supported forms:
//   - "tenant_name:asc"
//   - "+tenant_name"
//   - "-request_count"
//   - "request_count" (defaults to DESC)
func ParseUsageSort(raw string) (string, string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ValidateSortSpec("", "")
	}

	if strings.Contains(raw, ":") {
		parts := strings.SplitN(raw, ":", 2)
		return ValidateSortSpec(parts[0], parts[1])
	}

	if strings.HasPrefix(raw, "-") {
		return ValidateSortSpec(strings.TrimPrefix(raw, "-"), "desc")
	}
	if strings.HasPrefix(raw, "+") {
		return ValidateSortSpec(strings.TrimPrefix(raw, "+"), "asc")
	}
	return ValidateSortSpec(raw, "desc")
}

// UsageAggregator is the single query layer for multi-tenant and time-series
// usage endpoints. It queries _ayb_tenant_usage_daily and _ayb_request_logs.
type UsageAggregator struct {
	pool        *pgxpool.Pool
	queryRunner usageAggregateQueryRunner
	billingRepo BillingRepository
	nowFn       func() time.Time
}

type usageAggregateQueryRunner interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// NewUsageAggregator creates a UsageAggregator backed by the given pool.
func NewUsageAggregator(pool *pgxpool.Pool, billingRepo BillingRepository) *UsageAggregator {
	return &UsageAggregator{
		pool:        pool,
		queryRunner: pool,
		billingRepo: billingRepo,
		nowFn: func() time.Time {
			return time.Now().UTC()
		},
	}
}

func newUsageAggregatorForTests(queryRunner usageAggregateQueryRunner, billingRepo BillingRepository, nowFn func() time.Time) *UsageAggregator {
	if nowFn == nil {
		nowFn = func() time.Time {
			return time.Now().UTC()
		}
	}
	return &UsageAggregator{
		queryRunner: queryRunner,
		billingRepo: billingRepo,
		nowFn:       nowFn,
	}
}

func (a *UsageAggregator) nowUTC() time.Time {
	if a == nil || a.nowFn == nil {
		return time.Now().UTC()
	}
	return a.nowFn()
}

// ListTenantUsageSummaries returns a paginated multi-tenant usage listing
// with SUM for additive metrics and MAX for peak metrics.
func (a *UsageAggregator) ListTenantUsageSummaries(ctx context.Context, opts ListUsageOpts) ([]TenantUsageSummaryRow, int, error) {
	if a == nil || a.queryRunner == nil {
		return nil, 0, fmt.Errorf("usage aggregator is not configured")
	}

	normalized, err := normalizeListUsageOpts(opts, a.nowUTC())
	if err != nil {
		return nil, 0, err
	}

	countQuery := `SELECT COUNT(DISTINCT tenant_id)
		FROM _ayb_tenant_usage_daily
		WHERE date >= $1 AND date <= $2`
	var total int
	if err := a.queryRunner.QueryRow(ctx, countQuery, normalized.From, normalized.To).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count tenant usage: %w", err)
	}
	if total == 0 {
		return []TenantUsageSummaryRow{}, 0, nil
	}

	rows, err := a.queryRunner.Query(ctx, buildListTenantUsageQuery(normalized.SortColumn, normalized.SortDir), normalized.From, normalized.To, normalized.Limit, normalized.Offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list tenant usage: %w", err)
	}
	defer rows.Close()

	items := make([]TenantUsageSummaryRow, 0, normalized.Limit)
	for rows.Next() {
		var row TenantUsageSummaryRow
		if err := rows.Scan(
			&row.TenantID,
			&row.TenantName,
			&row.RequestCount,
			&row.StorageBytesUsed,
			&row.BandwidthBytes,
			&row.FunctionInvocations,
			&row.RealtimePeakConnections,
			&row.JobRuns,
		); err != nil {
			return nil, 0, fmt.Errorf("scan tenant usage: %w", err)
		}
		items = append(items, row)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate tenant usage rows: %w", err)
	}

	return items, total, nil
}

// GetUsageTrends returns time-series data aggregated by the specified granularity.
func (a *UsageAggregator) GetUsageTrends(ctx context.Context, opts TrendOpts) ([]TrendPoint, error) {
	if a == nil || a.queryRunner == nil {
		return nil, fmt.Errorf("usage aggregator is not configured")
	}

	normalized, err := normalizeTrendOpts(opts, a.nowUTC())
	if err != nil {
		return nil, err
	}

	query, args, err := buildTrendQuery(TrendOpts(normalized))
	if err != nil {
		return nil, err
	}

	rows, err := a.queryRunner.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("get usage trends: %w", err)
	}
	defer rows.Close()

	points := make([]TrendPoint, 0)
	for rows.Next() {
		var point TrendPoint
		if err := rows.Scan(&point.Timestamp, &point.Value); err != nil {
			return nil, fmt.Errorf("scan trend point: %w", err)
		}
		points = append(points, point)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate trend points: %w", err)
	}

	return points, nil
}

// TODO: Document buildUsageBreakdownQuery.
func buildUsageBreakdownQuery(opts normalizedBreakdownOpts) (string, []any, error) {
	switch opts.GroupBy {
	case "tenant":
		query, err := buildTenantBreakdownQuery(opts.Metric)
		if err != nil {
			return "", nil, err
		}
		return query, []any{opts.From, opts.To, opts.Limit}, nil
	case "endpoint":
		return buildRequestLogBreakdownQuery("COALESCE(path, '')"), []any{opts.From, opts.To.Add(24 * time.Hour), opts.Limit}, nil
	case "status_code":
		return buildRequestLogBreakdownQuery("COALESCE(status_code, 0)::text"), []any{opts.From, opts.To.Add(24 * time.Hour), opts.Limit}, nil
	default:
		return "", nil, fmt.Errorf("invalid group_by %q", opts.GroupBy)
	}
}

// GetUsageBreakdown returns top-N grouped usage data.
// group_by=tenant queries _ayb_tenant_usage_daily; group_by=endpoint|status_code
// queries _ayb_request_logs (platform-wide only — those tables lack tenant_id).
func (a *UsageAggregator) GetUsageBreakdown(ctx context.Context, opts BreakdownOpts) ([]BreakdownEntry, error) {
	if a == nil || a.queryRunner == nil {
		return nil, fmt.Errorf("usage aggregator is not configured")
	}

	normalized, err := normalizeBreakdownOpts(opts, a.nowUTC())
	if err != nil {
		return nil, err
	}

	query, args, err := buildUsageBreakdownQuery(normalized)
	if err != nil {
		return nil, err
	}

	rows, err := a.queryRunner.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("get usage breakdown: %w", err)
	}
	defer rows.Close()

	entries := make([]BreakdownEntry, 0, normalized.Limit)
	for rows.Next() {
		var entry BreakdownEntry
		if err := rows.Scan(&entry.Key, &entry.Value); err != nil {
			return nil, fmt.Errorf("scan breakdown entry: %w", err)
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate breakdown entries: %w", err)
	}

	return entries, nil
}

// GetTenantUsageLimits returns usage totals and plan limits for a tenant
// within the specified date range.
func (a *UsageAggregator) GetTenantUsageLimits(ctx context.Context, tenantID, period string, from, to time.Time) (*UsageLimitsResponse, error) {
	if a == nil || a.queryRunner == nil {
		return nil, fmt.Errorf("usage aggregator is not configured")
	}

	tenantID = strings.TrimSpace(tenantID)
	if tenantID == "" {
		return nil, fmt.Errorf("tenant_id is required")
	}

	startDate, endDate, err := resolveUsageDateRange(period, from, to, a.nowUTC())
	if err != nil {
		return nil, err
	}

	var requestCount, storageBytesUsed, bandwidthBytes, functionInvocations int64
	totalsQuery := `SELECT
		COALESCE(SUM(request_count), 0),
		COALESCE(MAX(db_bytes_used), 0),
		COALESCE(SUM(bandwidth_bytes), 0),
		COALESCE(SUM(function_invocations), 0)
	FROM _ayb_tenant_usage_daily
	WHERE tenant_id = $1 AND date >= $2 AND date <= $3`
	if err := a.queryRunner.QueryRow(ctx, totalsQuery, tenantID, startDate, endDate).Scan(
		&requestCount,
		&storageBytesUsed,
		&bandwidthBytes,
		&functionInvocations,
	); err != nil {
		return nil, fmt.Errorf("query tenant usage totals: %w", err)
	}

	plan := PlanFree
	if a.billingRepo != nil {
		rec, err := a.billingRepo.Get(ctx, tenantID)
		if err != nil && !errors.Is(err, ErrBillingRecordNotFound) {
			return nil, fmt.Errorf("resolve plan: %w", err)
		}
		if rec != nil && rec.Plan != "" {
			plan = rec.Plan
		}
	}

	limits := LimitsForPlan(plan)
	return &UsageLimitsResponse{
		Plan: plan,
		Metrics: map[string]MetricLimit{
			"api_requests":         newMetricLimit(limits.APIRequests, requestCount),
			"storage_bytes":        newMetricLimit(limits.StorageBytesUsed, storageBytesUsed),
			"bandwidth_bytes":      newMetricLimit(limits.BandwidthBytes, bandwidthBytes),
			"function_invocations": newMetricLimit(limits.FunctionInvocations, functionInvocations),
		},
	}, nil
}

func newMetricLimit(limit, used int64) MetricLimit {
	return MetricLimit{Limit: limit, Used: used, Remaining: computeRemaining(used, limit)}
}

func computeRemaining(used, limit int64) int64 {
	if limit == 0 {
		return 0
	}
	remaining := limit - used
	if remaining < 0 {
		return 0
	}
	return remaining
}
