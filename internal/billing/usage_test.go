package billing

import (
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/tenant"
)

func TestLimitsForPlan(t *testing.T) {
	free := PlanLimits{
		APIRequests:         50_000,
		StorageBytesUsed:    1_073_741_824,
		BandwidthBytes:      5_368_709_120,
		FunctionInvocations: 100_000,
	}
	starter := PlanLimits{
		APIRequests:         250_000,
		StorageBytesUsed:    5_368_709_120,
		BandwidthBytes:      26_843_545_600,
		FunctionInvocations: 500_000,
	}
	pro := PlanLimits{
		APIRequests:         1_000_000,
		StorageBytesUsed:    10_737_418_240,
		BandwidthBytes:      53_687_091_200,
		FunctionInvocations: 2_000_000,
	}
	enterprise := PlanLimits{}

	tests := []struct {
		name string
		plan Plan
		want PlanLimits
	}{
		{name: "free", plan: PlanFree, want: free},
		{name: "starter", plan: PlanStarter, want: starter},
		{name: "pro", plan: PlanPro, want: pro},
		{name: "enterprise", plan: PlanEnterprise, want: enterprise},
		{name: "unknown falls back to free", plan: Plan("unknown"), want: free},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := LimitsForPlan(tc.plan)
			if got != tc.want {
				t.Fatalf("LimitsForPlan(%q) = %+v, want %+v", tc.plan, got, tc.want)
			}
		})
	}
}

func TestBuildUsageSummary_EmptyRows(t *testing.T) {
	summary := BuildUsageSummary(nil, PlanPro, "month")
	if summary == nil {
		t.Fatal("BuildUsageSummary returned nil")
	}
	if summary.TenantID != "" {
		t.Fatalf("TenantID = %q, want empty", summary.TenantID)
	}
	if summary.Period != "month" {
		t.Fatalf("Period = %q, want month", summary.Period)
	}
	if len(summary.Data) != 0 {
		t.Fatalf("Data len = %d, want 0", len(summary.Data))
	}
	if summary.Totals != (UsageTotals{}) {
		t.Fatalf("Totals = %+v, want zero", summary.Totals)
	}
	if summary.Limits != LimitsForPlan(PlanPro) {
		t.Fatalf("Limits = %+v, want %+v", summary.Limits, LimitsForPlan(PlanPro))
	}
	if summary.Plan != PlanPro {
		t.Fatalf("Plan = %q, want %q", summary.Plan, PlanPro)
	}
}

func TestBuildUsageSummary_SingleRow(t *testing.T) {
	rows := []tenant.TenantUsageDaily{{
		TenantID:            "tenant-1",
		Date:                time.Date(2026, 3, 1, 14, 30, 0, 0, time.UTC),
		RequestCount:        111,
		DBBytesUsed:         222,
		BandwidthBytes:      333,
		FunctionInvocations: 444,
	}}

	summary := BuildUsageSummary(rows, PlanStarter, "week")
	if summary == nil {
		t.Fatal("BuildUsageSummary returned nil")
	}
	if summary.TenantID != "tenant-1" {
		t.Fatalf("TenantID = %q, want tenant-1", summary.TenantID)
	}
	if summary.Period != "week" {
		t.Fatalf("Period = %q, want week", summary.Period)
	}
	if len(summary.Data) != 1 {
		t.Fatalf("Data len = %d, want 1", len(summary.Data))
	}
	gotEntry := summary.Data[0]
	if gotEntry.Date != "2026-03-01" {
		t.Fatalf("Date = %q, want 2026-03-01", gotEntry.Date)
	}
	if gotEntry.APIRequests != 111 || gotEntry.StorageBytesUsed != 222 || gotEntry.BandwidthBytes != 333 || gotEntry.FunctionInvocations != 444 {
		t.Fatalf("entry = %+v, want mapped values", gotEntry)
	}

	wantTotals := UsageTotals{
		APIRequests:         111,
		StorageBytesUsed:    222,
		BandwidthBytes:      333,
		FunctionInvocations: 444,
	}
	if summary.Totals != wantTotals {
		t.Fatalf("Totals = %+v, want %+v", summary.Totals, wantTotals)
	}
	if summary.Limits != LimitsForPlan(PlanStarter) {
		t.Fatalf("Limits = %+v, want %+v", summary.Limits, LimitsForPlan(PlanStarter))
	}
}

func TestBuildUsageSummary_MultiRowUsesStorageMax(t *testing.T) {
	rows := []tenant.TenantUsageDaily{
		{
			TenantID:            "tenant-2",
			Date:                time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
			RequestCount:        100,
			DBBytesUsed:         300,
			BandwidthBytes:      500,
			FunctionInvocations: 700,
		},
		{
			TenantID:            "tenant-2",
			Date:                time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC),
			RequestCount:        200,
			DBBytesUsed:         100,
			BandwidthBytes:      600,
			FunctionInvocations: 800,
		},
		{
			TenantID:            "tenant-2",
			Date:                time.Date(2026, 3, 3, 0, 0, 0, 0, time.UTC),
			RequestCount:        300,
			DBBytesUsed:         400,
			BandwidthBytes:      700,
			FunctionInvocations: 900,
		},
	}

	summary := BuildUsageSummary(rows, PlanFree, "week")
	wantTotals := UsageTotals{
		APIRequests:         600,
		StorageBytesUsed:    400,
		BandwidthBytes:      1_800,
		FunctionInvocations: 2_400,
	}
	if summary.Totals != wantTotals {
		t.Fatalf("Totals = %+v, want %+v", summary.Totals, wantTotals)
	}
}

func TestResolvePeriodRange(t *testing.T) {
	now := time.Date(2026, 3, 3, 16, 45, 0, 0, time.UTC)

	t.Run("default period month", func(t *testing.T) {
		start, end, err := ResolvePeriodRange("", "", "", now)
		if err != nil {
			t.Fatalf("ResolvePeriodRange returned error: %v", err)
		}
		if !start.Equal(time.Date(2026, 2, 2, 0, 0, 0, 0, time.UTC)) {
			t.Fatalf("start = %s, want 2026-02-02", start.Format(time.DateOnly))
		}
		if !end.Equal(time.Date(2026, 3, 3, 0, 0, 0, 0, time.UTC)) {
			t.Fatalf("end = %s, want 2026-03-03", end.Format(time.DateOnly))
		}
	})

	t.Run("explicit from and to", func(t *testing.T) {
		start, end, err := ResolvePeriodRange("week", "2026-01-10", "2026-01-15", now)
		if err != nil {
			t.Fatalf("ResolvePeriodRange returned error: %v", err)
		}
		if !start.Equal(time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC)) {
			t.Fatalf("start = %s, want 2026-01-10", start.Format(time.DateOnly))
		}
		if !end.Equal(time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)) {
			t.Fatalf("end = %s, want 2026-01-15", end.Format(time.DateOnly))
		}
	})

	t.Run("day", func(t *testing.T) {
		start, end, err := ResolvePeriodRange("day", "", "", now)
		if err != nil {
			t.Fatalf("ResolvePeriodRange returned error: %v", err)
		}
		want := time.Date(2026, 3, 3, 0, 0, 0, 0, time.UTC)
		if !start.Equal(want) || !end.Equal(want) {
			t.Fatalf("start/end = %s/%s, want %s/%s", start.Format(time.DateOnly), end.Format(time.DateOnly), want.Format(time.DateOnly), want.Format(time.DateOnly))
		}
	})

	t.Run("week", func(t *testing.T) {
		start, end, err := ResolvePeriodRange("week", "", "", now)
		if err != nil {
			t.Fatalf("ResolvePeriodRange returned error: %v", err)
		}
		if !start.Equal(time.Date(2026, 2, 25, 0, 0, 0, 0, time.UTC)) {
			t.Fatalf("start = %s, want 2026-02-25", start.Format(time.DateOnly))
		}
		if !end.Equal(time.Date(2026, 3, 3, 0, 0, 0, 0, time.UTC)) {
			t.Fatalf("end = %s, want 2026-03-03", end.Format(time.DateOnly))
		}
	})

	t.Run("month", func(t *testing.T) {
		start, end, err := ResolvePeriodRange("month", "", "", now)
		if err != nil {
			t.Fatalf("ResolvePeriodRange returned error: %v", err)
		}
		if !start.Equal(time.Date(2026, 2, 2, 0, 0, 0, 0, time.UTC)) {
			t.Fatalf("start = %s, want 2026-02-02", start.Format(time.DateOnly))
		}
		if !end.Equal(time.Date(2026, 3, 3, 0, 0, 0, 0, time.UTC)) {
			t.Fatalf("end = %s, want 2026-03-03", end.Format(time.DateOnly))
		}
	})

	t.Run("invalid period", func(t *testing.T) {
		_, _, err := ResolvePeriodRange("year", "", "", now)
		if err == nil {
			t.Fatal("expected error for invalid period")
		}
	})

	t.Run("bad date format", func(t *testing.T) {
		_, _, err := ResolvePeriodRange("month", "03-01-2026", "2026-03-03", now)
		if err == nil {
			t.Fatal("expected error for bad date format")
		}
	})

	t.Run("from greater than to", func(t *testing.T) {
		_, _, err := ResolvePeriodRange("month", "2026-03-03", "2026-03-01", now)
		if err == nil {
			t.Fatal("expected error for from > to")
		}
	})
}
