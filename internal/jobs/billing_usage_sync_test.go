package jobs

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/billing"
	"github.com/allyourbase/ayb/internal/testutil"
)

type fakeBillingUsageSyncDataSource struct {
	tenantIDs     []string
	usageByTenant map[string]map[string]billing.UsageReport
	listErr       error
	getReportErr  map[string]error
	listCallCount int
	getCallCount  int
	lastGetTenant string
}

func (s *fakeBillingUsageSyncDataSource) ListBillableTenants(ctx context.Context) ([]string, error) {
	s.listCallCount++
	if s.listErr != nil {
		return nil, s.listErr
	}
	return append([]string(nil), s.tenantIDs...), nil
}

func (s *fakeBillingUsageSyncDataSource) GetUsageReport(ctx context.Context, tenantID string, usageDate time.Time) (billing.UsageReport, bool, error) {
	s.getCallCount++
	s.lastGetTenant = tenantID
	if err := s.getReportErr[tenantID]; err != nil {
		return billing.UsageReport{}, false, err
	}

	dateKey := usageDate.Format("2006-01-02")
	if byDate, ok := s.usageByTenant[tenantID]; ok {
		if report, ok := byDate[dateKey]; ok {
			return report, true, nil
		}
	}
	return billing.UsageReport{}, false, nil
}

type fakeBillingUsageService struct {
	reports     []billingUsageCall
	errByTenant map[string]error
}

type billingUsageCall struct {
	tenantID string
	usage    billing.UsageReport
}

func (s *fakeBillingUsageService) CreateCustomer(context.Context, string) (*billing.Customer, error) {
	return nil, errors.New("not implemented")
}

func (s *fakeBillingUsageService) CreateCheckoutSession(context.Context, string, billing.Plan, string, string) (*billing.CheckoutSession, error) {
	return nil, errors.New("not implemented")
}

func (s *fakeBillingUsageService) GetSubscription(context.Context, string) (*billing.Subscription, error) {
	return nil, errors.New("not implemented")
}

func (s *fakeBillingUsageService) CancelSubscription(context.Context, string) (*billing.Subscription, error) {
	return nil, errors.New("not implemented")
}

func (s *fakeBillingUsageService) ReportUsage(_ context.Context, tenantID string, usage billing.UsageReport) error {
	s.reports = append(s.reports, billingUsageCall{tenantID: tenantID, usage: usage})
	if err := s.errByTenant[tenantID]; err != nil {
		return err
	}
	return nil
}

func TestBillingUsageSyncJobHandler_QueriesTenantsAndReportsUsage(t *testing.T) {
	fixedNow := time.Date(2026, 3, 4, 10, 5, 0, 0, time.UTC)
	t.Cleanup(func() { usageSyncNow = time.Now })
	usageSyncNow = func() time.Time { return fixedNow }

	fakeStore := &fakeBillingUsageSyncDataSource{
		tenantIDs: []string{"tenant-1"},
		usageByTenant: map[string]map[string]billing.UsageReport{
			"tenant-1": {
				"2026-03-04": {
					RequestCount:        120,
					DBBytesUsed:         456,
					BandwidthBytes:      789,
					FunctionInvocations: 12,
					PeriodEnd:           fixedNow,
				},
			},
		},
		getReportErr: map[string]error{},
	}
	fakeSvc := &fakeBillingUsageService{errByTenant: map[string]error{}}

	err := BillingUsageSyncJobHandler(fakeSvc, fakeStore)(context.Background(), nil)
	testutil.NoError(t, err)
	testutil.Equal(t, 1, fakeStore.listCallCount)
	testutil.Equal(t, 1, fakeStore.getCallCount)
	testutil.Equal(t, 1, len(fakeSvc.reports))
	testutil.Equal(t, "tenant-1", fakeSvc.reports[0].tenantID)
	testutil.Equal(t, int64(120), fakeSvc.reports[0].usage.RequestCount)
	testutil.Equal(t, int64(456), fakeSvc.reports[0].usage.DBBytesUsed)
	testutil.Equal(t, int64(789), fakeSvc.reports[0].usage.BandwidthBytes)
	testutil.Equal(t, int64(12), fakeSvc.reports[0].usage.FunctionInvocations)
}

func TestBillingUsageSyncJobHandler_UsesFallbackUsageDate(t *testing.T) {
	fixedNow := time.Date(2026, 3, 4, 10, 5, 0, 0, time.UTC)
	t.Cleanup(func() { usageSyncNow = time.Now })
	usageSyncNow = func() time.Time { return fixedNow }

	fakeStore := &fakeBillingUsageSyncDataSource{
		tenantIDs: []string{"tenant-1"},
		usageByTenant: map[string]map[string]billing.UsageReport{
			"tenant-1": {
				"2026-03-03": {
					RequestCount: 99,
					PeriodEnd:    fixedNow.AddDate(0, 0, -1),
				},
			},
		},
		getReportErr: map[string]error{},
	}
	fakeSvc := &fakeBillingUsageService{errByTenant: map[string]error{}}

	err := BillingUsageSyncJobHandler(fakeSvc, fakeStore)(context.Background(), nil)
	testutil.NoError(t, err)
	testutil.Equal(t, 1, len(fakeSvc.reports))
	testutil.Equal(t, int64(99), fakeSvc.reports[0].usage.RequestCount)
	testutil.Equal(t, "2026-03-03", fakeSvc.reports[0].usage.PeriodEnd.Format("2006-01-02"))
}

func TestBillingUsageSyncJobHandler_ContinuesAfterTenantError(t *testing.T) {
	fixedNow := time.Date(2026, 3, 4, 10, 5, 0, 0, time.UTC)
	t.Cleanup(func() { usageSyncNow = time.Now })
	usageSyncNow = func() time.Time { return fixedNow }

	fakeStore := &fakeBillingUsageSyncDataSource{
		tenantIDs: []string{"tenant-1", "tenant-2"},
		usageByTenant: map[string]map[string]billing.UsageReport{
			"tenant-1": {
				"2026-03-04": {RequestCount: 1, PeriodEnd: fixedNow},
			},
			"tenant-2": {
				"2026-03-04": {RequestCount: 2, PeriodEnd: fixedNow},
			},
		},
		getReportErr: map[string]error{},
	}
	tenantErr := fmt.Errorf("billing service unavailable")
	fakeSvc := &fakeBillingUsageService{
		errByTenant: map[string]error{
			"tenant-1": tenantErr,
		},
	}
	fakeSvc.errByTenant["tenant-2"] = nil

	err := BillingUsageSyncJobHandler(fakeSvc, fakeStore)(context.Background(), nil)
	testutil.NoError(t, err)
	testutil.Equal(t, 2, len(fakeSvc.reports))
	testutil.Equal(t, "tenant-1", fakeSvc.reports[0].tenantID)
	testutil.Equal(t, "tenant-2", fakeSvc.reports[1].tenantID)
}

func TestUsageSyncCronExpr_DefaultsHourly(t *testing.T) {
	t.Parallel()

	expr, err := usageSyncCronExpr(3600)
	testutil.NoError(t, err)
	testutil.Equal(t, "0 * * * *", expr)
}

func TestUsageSyncCronExpr_Daily(t *testing.T) {
	t.Parallel()

	expr, err := usageSyncCronExpr(24 * 3600)
	testutil.NoError(t, err)
	testutil.Equal(t, "0 0 * * *", expr)
}

func TestUsageSyncCronExpr_RejectsUnsupportedIntervals(t *testing.T) {
	t.Parallel()

	_, err := usageSyncCronExpr(90 * 60) // 90 minutes cannot be represented exactly in cron.
	testutil.Error(t, err)
	testutil.Contains(t, err.Error(), "unsupported billing usage sync interval")

	_, err = usageSyncCronExpr(48 * 3600) // Multi-day intervals are not exact in standard 5-field cron.
	testutil.Error(t, err)
	testutil.Contains(t, err.Error(), "unsupported billing usage sync interval")
}
