package billing

import (
	"strings"
	"testing"
	"time"
)

func TestParseUsageSort(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		raw       string
		wantCol   string
		wantDir   string
		expectErr bool
	}{
		{name: "default", raw: "", wantCol: "request_count", wantDir: "DESC"},
		{name: "colon asc", raw: "tenant_name:asc", wantCol: "tenant_name", wantDir: "ASC"},
		{name: "prefix desc", raw: "-storage_bytes", wantCol: "storage_bytes_used", wantDir: "DESC"},
		{name: "prefix asc", raw: "+request_count", wantCol: "request_count", wantDir: "ASC"},
		{name: "invalid column", raw: "bad_column:asc", expectErr: true},
		{name: "invalid direction", raw: "request_count:sideways", expectErr: true},
		{name: "missing column with direction", raw: ":asc", expectErr: true},
		{name: "missing column with sign", raw: "-", expectErr: true},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			gotCol, gotDir, err := ParseUsageSort(tc.raw)
			if tc.expectErr {
				if err == nil {
					t.Fatalf("expected error for %q", tc.raw)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseUsageSort(%q) returned error: %v", tc.raw, err)
			}
			if gotCol != tc.wantCol || gotDir != tc.wantDir {
				t.Fatalf("ParseUsageSort(%q) = (%q, %q), want (%q, %q)", tc.raw, gotCol, gotDir, tc.wantCol, tc.wantDir)
			}
		})
	}
}

func TestNormalizeListUsageOpts(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 13, 14, 0, 0, 0, time.UTC)
	opts := ListUsageOpts{
		Period: "week",
		Sort: UsageSort{
			Column:    "tenant_name",
			Direction: "asc",
		},
	}

	got, err := normalizeListUsageOpts(opts, now)
	if err != nil {
		t.Fatalf("normalizeListUsageOpts returned error: %v", err)
	}
	if got.Limit != defaultListLimit {
		t.Fatalf("limit = %d, want %d", got.Limit, defaultListLimit)
	}
	if got.Offset != 0 {
		t.Fatalf("offset = %d, want 0", got.Offset)
	}
	if got.From.Format(time.DateOnly) != "2026-03-07" {
		t.Fatalf("from = %s, want 2026-03-07", got.From.Format(time.DateOnly))
	}
	if got.To.Format(time.DateOnly) != "2026-03-13" {
		t.Fatalf("to = %s, want 2026-03-13", got.To.Format(time.DateOnly))
	}
	if got.SortColumn != "tenant_name" || got.SortDir != "ASC" {
		t.Fatalf("sort = (%s %s), want (tenant_name ASC)", got.SortColumn, got.SortDir)
	}
}

func TestNormalizeListUsageOptsRejectsInvalidPagination(t *testing.T) {
	t.Parallel()

	_, err := normalizeListUsageOpts(ListUsageOpts{
		Period: "month",
		Limit:  -1,
	}, time.Now().UTC())
	if err == nil {
		t.Fatal("expected error for negative limit")
	}

	_, err = normalizeListUsageOpts(ListUsageOpts{
		Period: "month",
		Offset: -1,
	}, time.Now().UTC())
	if err == nil {
		t.Fatal("expected error for negative offset")
	}
}

func TestBuildTrendQueryChoosesExpectedBucketSource(t *testing.T) {
	t.Parallel()

	from := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC)

	query, args, err := buildTrendQuery(TrendOpts{
		Metric:      "api_requests",
		Granularity: "hour",
		From:        from,
		To:          to,
	})
	if err != nil {
		t.Fatalf("buildTrendQuery(api_requests/hour) returned error: %v", err)
	}
	if !strings.Contains(query, "_ayb_request_logs") || !strings.Contains(query, "date_trunc('hour', timestamp)") {
		t.Fatalf("unexpected hourly query: %s", query)
	}
	if len(args) != 2 {
		t.Fatalf("hourly args len = %d, want 2", len(args))
	}

	query, args, err = buildTrendQuery(TrendOpts{
		Metric:      "storage_bytes",
		Granularity: "week",
		From:        from,
		To:          to,
	})
	if err != nil {
		t.Fatalf("buildTrendQuery(storage_bytes/week) returned error: %v", err)
	}
	if !strings.Contains(query, "_ayb_tenant_usage_daily") || !strings.Contains(query, "date_trunc($1::text, date::timestamp)") {
		t.Fatalf("unexpected daily query: %s", query)
	}
	if len(args) != 3 {
		t.Fatalf("daily args len = %d, want 3", len(args))
	}
}

func TestValidateBreakdownMetricGroup(t *testing.T) {
	t.Parallel()

	if err := validateBreakdownMetricGroup("api_requests", "endpoint"); err != nil {
		t.Fatalf("validateBreakdownMetricGroup should allow api_requests/endpoint: %v", err)
	}
	if err := validateBreakdownMetricGroup("bandwidth_bytes", "tenant"); err != nil {
		t.Fatalf("validateBreakdownMetricGroup should allow bandwidth_bytes/tenant: %v", err)
	}
	if err := validateBreakdownMetricGroup("bandwidth_bytes", "status_code"); err == nil {
		t.Fatal("expected error for bandwidth_bytes/status_code")
	}
}

func TestBuildRequestLogBreakdownQueryGroupsByKeyExpression(t *testing.T) {
	t.Parallel()

	query := buildRequestLogBreakdownQuery("COALESCE(path, '')")
	if !strings.Contains(query, "GROUP BY COALESCE(path, '')") {
		t.Fatalf("expected normalized GROUP BY expression, query=%s", query)
	}
	if strings.Contains(query, "GROUP BY path") {
		t.Fatalf("unexpected raw GROUP BY path in query=%s", query)
	}
}
