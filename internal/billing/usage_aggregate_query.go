// Package billing Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/internal/billing/usage_aggregate_query.go.
package billing

import (
	"fmt"
	"strings"
	"time"
)

type normalizedListUsageOpts struct {
	From       time.Time
	To         time.Time
	SortColumn string
	SortDir    string
	Limit      int
	Offset     int
}

// TODO: Document normalizeListUsageOpts.
func normalizeListUsageOpts(opts ListUsageOpts, now time.Time) (normalizedListUsageOpts, error) {
	from, to, err := resolveUsageDateRange(opts.Period, opts.From, opts.To, now)
	if err != nil {
		return normalizedListUsageOpts{}, err
	}

	sortCol, sortDir, err := ValidateSortSpec(opts.Sort.Column, opts.Sort.Direction)
	if err != nil {
		return normalizedListUsageOpts{}, err
	}

	limit := opts.Limit
	if limit == 0 {
		limit = defaultListLimit
	}
	if limit < 0 {
		return normalizedListUsageOpts{}, fmt.Errorf("limit must be non-negative")
	}
	if limit > maxListLimit {
		return normalizedListUsageOpts{}, fmt.Errorf("limit must be <= %d", maxListLimit)
	}
	if opts.Offset < 0 {
		return normalizedListUsageOpts{}, fmt.Errorf("offset must be non-negative")
	}

	return normalizedListUsageOpts{
		From:       from,
		To:         to,
		SortColumn: sortCol,
		SortDir:    sortDir,
		Limit:      limit,
		Offset:     opts.Offset,
	}, nil
}

type normalizedTrendOpts struct {
	Metric      string
	Granularity string
	From        time.Time
	To          time.Time
}

// TODO: Document normalizeTrendOpts.
func normalizeTrendOpts(opts TrendOpts, now time.Time) (normalizedTrendOpts, error) {
	metric := strings.TrimSpace(opts.Metric)
	if err := ValidateMetric(metric); err != nil {
		return normalizedTrendOpts{}, err
	}

	granularity := strings.TrimSpace(opts.Granularity)
	if granularity == "" {
		granularity = "day"
	}
	if err := ValidateGranularity(granularity, metric); err != nil {
		return normalizedTrendOpts{}, err
	}

	from, to, err := resolveUsageDateRange("month", opts.From, opts.To, now)
	if err != nil {
		return normalizedTrendOpts{}, err
	}

	return normalizedTrendOpts{
		Metric:      metric,
		Granularity: granularity,
		From:        from,
		To:          to,
	}, nil
}

type normalizedBreakdownOpts struct {
	Metric  string
	GroupBy string
	From    time.Time
	To      time.Time
	Limit   int
}

// TODO: Document normalizeBreakdownOpts.
func normalizeBreakdownOpts(opts BreakdownOpts, now time.Time) (normalizedBreakdownOpts, error) {
	metric := strings.TrimSpace(opts.Metric)
	if err := ValidateMetric(metric); err != nil {
		return normalizedBreakdownOpts{}, err
	}

	groupBy := strings.TrimSpace(opts.GroupBy)
	if groupBy == "" {
		groupBy = "tenant"
	}
	if err := ValidateGroupBy(groupBy); err != nil {
		return normalizedBreakdownOpts{}, err
	}
	if err := validateBreakdownMetricGroup(metric, groupBy); err != nil {
		return normalizedBreakdownOpts{}, err
	}

	from, to, err := resolveUsageDateRange("month", opts.From, opts.To, now)
	if err != nil {
		return normalizedBreakdownOpts{}, err
	}

	limit := opts.Limit
	if limit == 0 {
		limit = defaultBreakdownLimit
	}
	if limit < 0 {
		return normalizedBreakdownOpts{}, fmt.Errorf("limit must be non-negative")
	}
	if limit > maxBreakdownLimit {
		return normalizedBreakdownOpts{}, fmt.Errorf("limit must be <= %d", maxBreakdownLimit)
	}

	return normalizedBreakdownOpts{
		Metric:  metric,
		GroupBy: groupBy,
		From:    from,
		To:      to,
		Limit:   limit,
	}, nil
}

func validateBreakdownMetricGroup(metric, groupBy string) error {
	if (groupBy == "endpoint" || groupBy == "status_code") && metric != "api_requests" {
		return fmt.Errorf("metric %q is only supported with group_by=tenant", metric)
	}
	return nil
}

// ValidateBreakdownMetricGroup ensures metric/group_by combinations only use
// request-log-backed groupings with api_requests.
func ValidateBreakdownMetricGroup(metric, groupBy string) error {
	return validateBreakdownMetricGroup(metric, groupBy)
}

// TODO: Document resolveUsageDateRange.
func resolveUsageDateRange(period string, from, to time.Time, now time.Time) (time.Time, time.Time, error) {
	period = strings.TrimSpace(period)
	if period == "" {
		period = "month"
	}

	fromSet := !from.IsZero()
	toSet := !to.IsZero()
	if fromSet != toSet {
		return time.Time{}, time.Time{}, fmt.Errorf("both from and to are required when either is set")
	}

	fromStr := ""
	toStr := ""
	if fromSet {
		fromStr = from.UTC().Format(time.DateOnly)
		toStr = to.UTC().Format(time.DateOnly)
	}

	return ResolvePeriodRange(period, fromStr, toStr, now)
}

// TODO: Document buildListTenantUsageQuery.
func buildListTenantUsageQuery(sortColumn, sortDir string) string {
	return fmt.Sprintf(`SELECT
		u.tenant_id::text,
		t.name,
		COALESCE(SUM(u.request_count), 0) AS request_count,
		COALESCE(MAX(u.db_bytes_used), 0) AS storage_bytes_used,
		COALESCE(SUM(u.bandwidth_bytes), 0) AS bandwidth_bytes,
		COALESCE(SUM(u.function_invocations), 0) AS function_invocations,
		COALESCE(MAX(u.realtime_peak_connections), 0) AS realtime_peak_connections,
		COALESCE(SUM(u.job_runs), 0) AS job_runs
	FROM _ayb_tenant_usage_daily u
	JOIN _ayb_tenants t ON t.id = u.tenant_id
	WHERE u.date >= $1 AND u.date <= $2
	GROUP BY u.tenant_id, t.name
	ORDER BY %s %s
	LIMIT $3 OFFSET $4`, sortColumn, sortDir)
}

// TODO: Document buildTrendQuery.
func buildTrendQuery(opts TrendOpts) (string, []any, error) {
	if err := ValidateMetric(opts.Metric); err != nil {
		return "", nil, err
	}
	if err := ValidateGranularity(opts.Granularity, opts.Metric); err != nil {
		return "", nil, err
	}

	if opts.Granularity == "hour" {
		toExclusive := opts.To.Add(24 * time.Hour)
		q := `SELECT date_trunc('hour', timestamp) AS bucket, COUNT(*)::bigint AS value
			FROM _ayb_request_logs
			WHERE timestamp >= $1 AND timestamp < $2
			GROUP BY bucket
			ORDER BY bucket`
		return q, []any{opts.From, toExclusive}, nil
	}

	def := metricDefinitions[opts.Metric]
	q := fmt.Sprintf(`SELECT date_trunc($1::text, date::timestamp) AS bucket, %s(%s) AS value
		FROM _ayb_tenant_usage_daily
		WHERE date >= $2 AND date <= $3
		GROUP BY bucket
		ORDER BY bucket`, def.aggregation, def.column)
	return q, []any{opts.Granularity, opts.From, opts.To}, nil
}

func buildTenantBreakdownQuery(metric string) (string, error) {
	def, ok := metricDefinitions[metric]
	if !ok {
		return "", fmt.Errorf("invalid metric %q", metric)
	}
	q := fmt.Sprintf(`SELECT tenant_id::text AS key, %s(%s)::bigint AS value
		FROM _ayb_tenant_usage_daily
		WHERE date >= $1 AND date <= $2
		GROUP BY tenant_id
		ORDER BY value DESC
		LIMIT $3`, def.aggregation, def.column)
	return q, nil
}

func buildRequestLogBreakdownQuery(keyExpr string) string {
	return fmt.Sprintf(`SELECT %s AS key, COUNT(*)::bigint AS value
		FROM _ayb_request_logs
		WHERE timestamp >= $1 AND timestamp < $2
		GROUP BY %s
		ORDER BY value DESC
		LIMIT $3`, keyExpr, keyExpr)
}
