package billing

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestUsageAggregatorListTenantUsageSummariesAppliesSortPaginationAndDateRange(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 14, 10, 0, 0, 0, time.UTC)
	runner := &stubUsageQueryRunner{
		queryRowResults: []pgx.Row{
			newFakeRow([]any{2}),
		},
		queryResults: []pgx.Rows{
			newFakeRows([][]any{
				{"00000000-0000-0000-0000-000000000101", "Acme", int64(15), int64(1024), int64(900), int64(8), int64(3), int64(2)},
				{"00000000-0000-0000-0000-000000000102", "Bravo", int64(10), int64(512), int64(700), int64(5), int64(2), int64(1)},
			}),
		},
	}
	agg := newUsageAggregatorForTests(runner, nil, func() time.Time { return now })

	rows, total, err := agg.ListTenantUsageSummaries(context.Background(), ListUsageOpts{
		Period: "week",
		Sort: UsageSort{
			Column:    "tenant_name",
			Direction: "asc",
		},
		Limit:  25,
		Offset: 5,
	})
	if err != nil {
		t.Fatalf("ListTenantUsageSummaries returned error: %v", err)
	}
	if total != 2 {
		t.Fatalf("total = %d, want 2", total)
	}
	if len(rows) != 2 {
		t.Fatalf("len(rows) = %d, want 2", len(rows))
	}

	if len(runner.queryRowCalls) != 1 {
		t.Fatalf("queryRowCalls = %d, want 1", len(runner.queryRowCalls))
	}
	countArgs := runner.queryRowCalls[0].args
	if len(countArgs) != 2 {
		t.Fatalf("count args len = %d, want 2", len(countArgs))
	}
	if got := countArgs[0].(time.Time).Format(time.DateOnly); got != "2026-03-08" {
		t.Fatalf("count from = %s, want 2026-03-08", got)
	}
	if got := countArgs[1].(time.Time).Format(time.DateOnly); got != "2026-03-14" {
		t.Fatalf("count to = %s, want 2026-03-14", got)
	}

	if len(runner.queryCalls) != 1 {
		t.Fatalf("queryCalls = %d, want 1", len(runner.queryCalls))
	}
	listCall := runner.queryCalls[0]
	if !strings.Contains(listCall.query, "ORDER BY tenant_name ASC") {
		t.Fatalf("expected ORDER BY tenant_name ASC, query=%s", listCall.query)
	}
	if len(listCall.args) != 4 {
		t.Fatalf("list args len = %d, want 4", len(listCall.args))
	}
	if got := listCall.args[0].(time.Time).Format(time.DateOnly); got != "2026-03-08" {
		t.Fatalf("list from = %s, want 2026-03-08", got)
	}
	if got := listCall.args[1].(time.Time).Format(time.DateOnly); got != "2026-03-14" {
		t.Fatalf("list to = %s, want 2026-03-14", got)
	}
	if got := listCall.args[2].(int); got != 25 {
		t.Fatalf("list limit = %d, want 25", got)
	}
	if got := listCall.args[3].(int); got != 5 {
		t.Fatalf("list offset = %d, want 5", got)
	}
}

func TestUsageAggregatorListTenantUsageSummariesRejectsInvalidSort(t *testing.T) {
	t.Parallel()

	runner := &stubUsageQueryRunner{}
	agg := newUsageAggregatorForTests(runner, nil, func() time.Time { return time.Date(2026, 3, 14, 0, 0, 0, 0, time.UTC) })

	_, _, err := agg.ListTenantUsageSummaries(context.Background(), ListUsageOpts{
		Period: "month",
		Sort: UsageSort{
			Column: "not_a_column",
		},
	})
	if err == nil {
		t.Fatal("expected invalid sort error")
	}
	if len(runner.queryRowCalls) != 0 || len(runner.queryCalls) != 0 {
		t.Fatalf("expected zero db calls for invalid sort, got queryRow=%d query=%d", len(runner.queryRowCalls), len(runner.queryCalls))
	}
}

func TestUsageAggregatorGetUsageTrendsHourlyBucketsWithRequestLogs(t *testing.T) {
	t.Parallel()

	from := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC)
	runner := &stubUsageQueryRunner{
		queryResults: []pgx.Rows{
			newFakeRows([][]any{
				{time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC), int64(42)},
			}),
		},
	}
	agg := newUsageAggregatorForTests(runner, nil, func() time.Time { return time.Date(2026, 3, 14, 0, 0, 0, 0, time.UTC) })

	points, err := agg.GetUsageTrends(context.Background(), TrendOpts{
		Metric:      "api_requests",
		Granularity: "hour",
		From:        from,
		To:          to,
	})
	if err != nil {
		t.Fatalf("GetUsageTrends returned error: %v", err)
	}
	if len(points) != 1 {
		t.Fatalf("len(points) = %d, want 1", len(points))
	}
	if len(runner.queryCalls) != 1 {
		t.Fatalf("queryCalls = %d, want 1", len(runner.queryCalls))
	}
	call := runner.queryCalls[0]
	if !strings.Contains(call.query, "_ayb_request_logs") || !strings.Contains(call.query, "date_trunc('hour', timestamp)") {
		t.Fatalf("expected hourly request-log query, got %s", call.query)
	}
	if len(call.args) != 2 {
		t.Fatalf("args len = %d, want 2", len(call.args))
	}
	if got := call.args[0].(time.Time); !got.Equal(from) {
		t.Fatalf("from arg = %s, want %s", got, from)
	}
	wantToExclusive := to.Add(24 * time.Hour)
	if got := call.args[1].(time.Time); !got.Equal(wantToExclusive) {
		t.Fatalf("to arg = %s, want %s", got, wantToExclusive)
	}
}

func TestUsageAggregatorGetUsageTrendsRejectsInvalidMetric(t *testing.T) {
	t.Parallel()

	runner := &stubUsageQueryRunner{}
	agg := newUsageAggregatorForTests(runner, nil, func() time.Time { return time.Date(2026, 3, 14, 0, 0, 0, 0, time.UTC) })

	_, err := agg.GetUsageTrends(context.Background(), TrendOpts{
		Metric:      "bad_metric",
		Granularity: "day",
	})
	if err == nil {
		t.Fatal("expected invalid metric error")
	}
	if len(runner.queryCalls) != 0 {
		t.Fatalf("expected zero db calls, got %d", len(runner.queryCalls))
	}
}

func TestUsageAggregatorGetUsageBreakdownUsesRequestLogQueryAndDateRange(t *testing.T) {
	t.Parallel()

	from := time.Date(2026, 3, 3, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 3, 5, 0, 0, 0, 0, time.UTC)
	runner := &stubUsageQueryRunner{
		queryResults: []pgx.Rows{
			newFakeRows([][]any{
				{"/v1/widgets", int64(7)},
			}),
		},
	}
	agg := newUsageAggregatorForTests(runner, nil, func() time.Time { return time.Date(2026, 3, 14, 0, 0, 0, 0, time.UTC) })

	entries, err := agg.GetUsageBreakdown(context.Background(), BreakdownOpts{
		Metric:  "api_requests",
		GroupBy: "endpoint",
		From:    from,
		To:      to,
		Limit:   3,
	})
	if err != nil {
		t.Fatalf("GetUsageBreakdown returned error: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("len(entries) = %d, want 1", len(entries))
	}
	if entries[0].Key != "/v1/widgets" || entries[0].Value != 7 {
		t.Fatalf("breakdown entry = %+v, want key=/v1/widgets value=7", entries[0])
	}
	if len(runner.queryCalls) != 1 {
		t.Fatalf("queryCalls = %d, want 1", len(runner.queryCalls))
	}
	call := runner.queryCalls[0]
	if !strings.Contains(call.query, "_ayb_request_logs") || !strings.Contains(call.query, "GROUP BY COALESCE(path, '')") {
		t.Fatalf("expected endpoint request-log query, got %s", call.query)
	}
	if len(call.args) != 3 {
		t.Fatalf("args len = %d, want 3", len(call.args))
	}
	if got := call.args[0].(time.Time); !got.Equal(from) {
		t.Fatalf("from arg = %s, want %s", got, from)
	}
	wantToExclusive := to.Add(24 * time.Hour)
	if got := call.args[1].(time.Time); !got.Equal(wantToExclusive) {
		t.Fatalf("to arg = %s, want %s", got, wantToExclusive)
	}
	if got := call.args[2].(int); got != 3 {
		t.Fatalf("limit arg = %d, want 3", got)
	}
}

func TestUsageAggregatorGetTenantUsageLimitsUsesPlanLimitsAndDateRange(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 14, 15, 0, 0, 0, time.UTC)
	repo := &fakeBillingRepo{
		records: map[string]*BillingRecord{
			"tenant-1": {
				TenantID: "tenant-1",
				Plan:     PlanPro,
			},
		},
	}
	runner := &stubUsageQueryRunner{
		queryRowResults: []pgx.Row{
			newFakeRow([]any{int64(120), int64(2048), int64(4096), int64(8)}),
		},
	}
	agg := newUsageAggregatorForTests(runner, repo, func() time.Time { return now })

	limits, err := agg.GetTenantUsageLimits(context.Background(), "tenant-1", "week", time.Time{}, time.Time{})
	if err != nil {
		t.Fatalf("GetTenantUsageLimits returned error: %v", err)
	}
	if limits.Plan != PlanPro {
		t.Fatalf("plan = %q, want %q", limits.Plan, PlanPro)
	}
	if repo.getCount != 1 {
		t.Fatalf("repo getCount = %d, want 1", repo.getCount)
	}
	if len(runner.queryRowCalls) != 1 {
		t.Fatalf("queryRowCalls = %d, want 1", len(runner.queryRowCalls))
	}
	call := runner.queryRowCalls[0]
	if len(call.args) != 3 {
		t.Fatalf("args len = %d, want 3", len(call.args))
	}
	if got := call.args[0].(string); got != "tenant-1" {
		t.Fatalf("tenant_id arg = %q, want tenant-1", got)
	}
	if got := call.args[1].(time.Time).Format(time.DateOnly); got != "2026-03-08" {
		t.Fatalf("from arg = %s, want 2026-03-08", got)
	}
	if got := call.args[2].(time.Time).Format(time.DateOnly); got != "2026-03-14" {
		t.Fatalf("to arg = %s, want 2026-03-14", got)
	}

	wantPlanLimits := LimitsForPlan(PlanPro)
	if got := limits.Metrics["api_requests"]; got.Used != 120 || got.Limit != wantPlanLimits.APIRequests || got.Remaining != wantPlanLimits.APIRequests-120 {
		t.Fatalf("api_requests = %+v, want used=120 limit=%d remaining=%d", got, wantPlanLimits.APIRequests, wantPlanLimits.APIRequests-120)
	}
	if got := limits.Metrics["storage_bytes"]; got.Used != 2048 || got.Limit != wantPlanLimits.StorageBytesUsed || got.Remaining != wantPlanLimits.StorageBytesUsed-2048 {
		t.Fatalf("storage_bytes = %+v, want used=2048 limit=%d remaining=%d", got, wantPlanLimits.StorageBytesUsed, wantPlanLimits.StorageBytesUsed-2048)
	}
	if got := limits.Metrics["bandwidth_bytes"]; got.Used != 4096 || got.Limit != wantPlanLimits.BandwidthBytes || got.Remaining != wantPlanLimits.BandwidthBytes-4096 {
		t.Fatalf("bandwidth_bytes = %+v, want used=4096 limit=%d remaining=%d", got, wantPlanLimits.BandwidthBytes, wantPlanLimits.BandwidthBytes-4096)
	}
	if got := limits.Metrics["function_invocations"]; got.Used != 8 || got.Limit != wantPlanLimits.FunctionInvocations || got.Remaining != wantPlanLimits.FunctionInvocations-8 {
		t.Fatalf("function_invocations = %+v, want used=8 limit=%d remaining=%d", got, wantPlanLimits.FunctionInvocations, wantPlanLimits.FunctionInvocations-8)
	}
}

type capturedUsageQuery struct {
	query string
	args  []any
}

type stubUsageQueryRunner struct {
	queryRowResults []pgx.Row
	queryResults    []pgx.Rows
	queryRowCalls   []capturedUsageQuery
	queryCalls      []capturedUsageQuery
}

func (s *stubUsageQueryRunner) QueryRow(_ context.Context, query string, args ...any) pgx.Row {
	s.queryRowCalls = append(s.queryRowCalls, capturedUsageQuery{query: query, args: cloneAnySlice(args)})
	if len(s.queryRowResults) == 0 {
		return errorRow{err: errors.New("unexpected QueryRow call")}
	}
	next := s.queryRowResults[0]
	s.queryRowResults = s.queryRowResults[1:]
	return next
}

func (s *stubUsageQueryRunner) Query(_ context.Context, query string, args ...any) (pgx.Rows, error) {
	s.queryCalls = append(s.queryCalls, capturedUsageQuery{query: query, args: cloneAnySlice(args)})
	if len(s.queryResults) == 0 {
		return nil, errors.New("unexpected Query call")
	}
	next := s.queryResults[0]
	s.queryResults = s.queryResults[1:]
	return next, nil
}

type errorRow struct {
	err error
}

func (r errorRow) Scan(_ ...any) error {
	return r.err
}

type fakeRow struct {
	values []any
}

func newFakeRow(values []any) pgx.Row {
	return fakeRow{values: cloneAnySlice(values)}
}

func (r fakeRow) Scan(dest ...any) error {
	return copyRowValues(dest, r.values)
}

type fakeRows struct {
	rows   [][]any
	index  int
	closed bool
}

func newFakeRows(rows [][]any) pgx.Rows {
	cloned := make([][]any, 0, len(rows))
	for _, row := range rows {
		cloned = append(cloned, cloneAnySlice(row))
	}
	return &fakeRows{rows: cloned}
}

func (r *fakeRows) Close() {
	r.closed = true
}

func (r *fakeRows) Err() error {
	return nil
}

func (r *fakeRows) CommandTag() pgconn.CommandTag {
	return pgconn.CommandTag{}
}

func (r *fakeRows) FieldDescriptions() []pgconn.FieldDescription {
	return nil
}

func (r *fakeRows) Next() bool {
	if r.closed {
		return false
	}
	if r.index >= len(r.rows) {
		r.closed = true
		return false
	}
	r.index++
	return true
}

func (r *fakeRows) Scan(dest ...any) error {
	if r.index == 0 || r.index > len(r.rows) {
		return errors.New("scan called without an active row")
	}
	return copyRowValues(dest, r.rows[r.index-1])
}

func (r *fakeRows) Values() ([]any, error) {
	if r.index == 0 || r.index > len(r.rows) {
		return nil, errors.New("values called without an active row")
	}
	return cloneAnySlice(r.rows[r.index-1]), nil
}

func (r *fakeRows) RawValues() [][]byte {
	return nil
}

func (r *fakeRows) Conn() *pgx.Conn {
	return nil
}

func cloneAnySlice(values []any) []any {
	if len(values) == 0 {
		return nil
	}
	cloned := make([]any, len(values))
	copy(cloned, values)
	return cloned
}

func copyRowValues(dest []any, values []any) error {
	if len(dest) != len(values) {
		return errors.New("destination/value length mismatch")
	}
	for i := range dest {
		target := reflect.ValueOf(dest[i])
		if target.Kind() != reflect.Ptr || target.IsNil() {
			return errors.New("destination must be a non-nil pointer")
		}
		source := reflect.ValueOf(values[i])
		targetElem := target.Elem()
		if source.Type().AssignableTo(targetElem.Type()) {
			targetElem.Set(source)
			continue
		}
		if source.Type().ConvertibleTo(targetElem.Type()) {
			targetElem.Set(source.Convert(targetElem.Type()))
			continue
		}
		return errors.New("value type is not assignable to destination")
	}
	return nil
}
