package billing

import (
	"fmt"
	"time"

	"github.com/allyourbase/ayb/internal/tenant"
)

// UsageDayEntry represents one day of usage data in API responses.
type UsageDayEntry struct {
	Date                string `json:"date"`
	APIRequests         int64  `json:"apiRequests"`
	StorageBytesUsed    int64  `json:"storageBytesUsed"`
	BandwidthBytes      int64  `json:"bandwidthBytes"`
	FunctionInvocations int64  `json:"functionInvocations"`
}

// UsageTotals aggregates usage metrics over a date range.
type UsageTotals struct {
	APIRequests         int64 `json:"apiRequests"`
	StorageBytesUsed    int64 `json:"storageBytesUsed"`
	BandwidthBytes      int64 `json:"bandwidthBytes"`
	FunctionInvocations int64 `json:"functionInvocations"`
}

// UsageSummary is the response shape for tenant usage endpoints.
type UsageSummary struct {
	TenantID string          `json:"tenantId"`
	Period   string          `json:"period"`
	Data     []UsageDayEntry `json:"data"`
	Totals   UsageTotals     `json:"totals"`
	Limits   PlanLimits      `json:"limits"`
	Plan     Plan            `json:"plan"`
}

// BuildUsageSummary converts daily usage rows into API response format.
// StorageBytesUsed in totals is the max across rows (high-water mark).
func BuildUsageSummary(rows []tenant.TenantUsageDaily, plan Plan, period string) *UsageSummary {
	summary := &UsageSummary{
		Period: period,
		Data:   make([]UsageDayEntry, 0, len(rows)),
		Limits: LimitsForPlan(plan),
		Plan:   plan,
	}

	for i, row := range rows {
		if i == 0 {
			summary.TenantID = row.TenantID
		}

		entry := UsageDayEntry{
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

// ResolvePeriodRange resolves usage date range based on period or explicit from/to.
func ResolvePeriodRange(period, fromStr, toStr string, now time.Time) (time.Time, time.Time, error) {
	nowDay := now.UTC().Truncate(24 * time.Hour)

	if period == "" {
		period = "month"
	}

	if fromStr != "" || toStr != "" {
		if fromStr == "" || toStr == "" {
			return time.Time{}, time.Time{}, fmt.Errorf("both from and to are required when either is provided")
		}
		start, err := time.Parse(time.DateOnly, fromStr)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("invalid from date: %w", err)
		}
		end, err := time.Parse(time.DateOnly, toStr)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("invalid to date: %w", err)
		}
		if start.After(end) {
			return time.Time{}, time.Time{}, fmt.Errorf("from must be less than or equal to to")
		}
		return start.UTC(), end.UTC(), nil
	}

	switch period {
	case "day":
		return nowDay, nowDay, nil
	case "week":
		return nowDay.AddDate(0, 0, -6), nowDay, nil
	case "month":
		return nowDay.AddDate(0, 0, -29), nowDay, nil
	default:
		return time.Time{}, time.Time{}, fmt.Errorf("invalid period %q", period)
	}
}
