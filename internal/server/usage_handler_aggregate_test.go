package server

import (
	"context"
	"time"

	"github.com/allyourbase/ayb/internal/billing"
)

type fakeUsageAggregateService struct {
	listRows  []billing.TenantUsageSummaryRow
	listTotal int
	listErr   error
	listOpts  billing.ListUsageOpts

	trendRows []billing.TrendPoint
	trendErr  error
	trendOpts billing.TrendOpts

	breakdownRows []billing.BreakdownEntry
	breakdownErr  error
	breakdownOpts billing.BreakdownOpts

	limitsResp     *billing.UsageLimitsResponse
	limitsErr      error
	limitsTenantID string
	limitsPeriod   string
}

func (f *fakeUsageAggregateService) ListTenantUsageSummaries(_ context.Context, opts billing.ListUsageOpts) ([]billing.TenantUsageSummaryRow, int, error) {
	f.listOpts = opts
	return f.listRows, f.listTotal, f.listErr
}

func (f *fakeUsageAggregateService) GetUsageTrends(_ context.Context, opts billing.TrendOpts) ([]billing.TrendPoint, error) {
	f.trendOpts = opts
	return f.trendRows, f.trendErr
}

func (f *fakeUsageAggregateService) GetUsageBreakdown(_ context.Context, opts billing.BreakdownOpts) ([]billing.BreakdownEntry, error) {
	f.breakdownOpts = opts
	return f.breakdownRows, f.breakdownErr
}

func (f *fakeUsageAggregateService) GetTenantUsageLimits(_ context.Context, tenantID, period string, from, to time.Time) (*billing.UsageLimitsResponse, error) {
	f.limitsTenantID = tenantID
	f.limitsPeriod = period
	_ = from
	_ = to
	if f.limitsResp == nil {
		return &billing.UsageLimitsResponse{Plan: billing.PlanFree, Metrics: map[string]billing.MetricLimit{}}, f.limitsErr
	}
	return f.limitsResp, f.limitsErr
}
