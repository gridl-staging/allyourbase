package billing

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestStripeService_ReportUsage_PositiveDeltaEmitsMeterEventsAndUpdatesCheckpoint(t *testing.T) {
	t.Parallel()

	usageDate := time.Date(2026, 3, 3, 23, 59, 0, 0, time.UTC)
	repo := &fakeBillingRepo{
		records: map[string]*BillingRecord{
			"tenant-1": {
				TenantID:         "tenant-1",
				StripeCustomerID: "cus_123",
			},
		},
		usageSync: map[string]int64{
			checkpointKey("tenant-1", "2026-03-03", "api_requests"):         100,
			checkpointKey("tenant-1", "2026-03-03", "storage_bytes"):        1000,
			checkpointKey("tenant-1", "2026-03-03", "bandwidth_bytes"):      2000,
			checkpointKey("tenant-1", "2026-03-03", "function_invocations"): 3,
		},
	}
	adapter := &fakeStripeAdapter{}
	svc := NewStripeBillingService(repo, defaultBillingCfg(), adapter, testNoopLogger())

	err := svc.ReportUsage(context.Background(), "tenant-1", UsageReport{
		RequestCount:        150,
		DBBytesUsed:         1000,
		BandwidthBytes:      1500,
		FunctionInvocations: 10,
		PeriodEnd:           usageDate,
	})
	testutil.NoError(t, err)
	testutil.Equal(t, 2, len(adapter.meterCalls))
	testutil.Equal(t, "meter.api_requests", adapter.meterCalls[0].eventName)
	testutil.Equal(t, int64(50), adapter.meterCalls[0].value)
	testutil.Equal(t, "ayb:tenant-1:2026-03-03:api_requests:150", adapter.meterCalls[0].identifier)
	testutil.Equal(t, "meter.function_invocations", adapter.meterCalls[1].eventName)
	testutil.Equal(t, int64(7), adapter.meterCalls[1].value)
	testutil.Equal(t, "ayb:tenant-1:2026-03-03:function_invocations:10", adapter.meterCalls[1].identifier)
	testutil.Equal(t, 2, len(repo.upsertUsageCalls))
	testutil.Equal(t, int64(150), repo.usageSync[checkpointKey("tenant-1", "2026-03-03", "api_requests")])
	testutil.Equal(t, int64(10), repo.usageSync[checkpointKey("tenant-1", "2026-03-03", "function_invocations")])
}

func TestStripeService_ReportUsage_ZeroOrNegativeDeltaSkips(t *testing.T) {
	t.Parallel()

	usageDate := time.Date(2026, 3, 3, 23, 59, 0, 0, time.UTC)
	repo := &fakeBillingRepo{
		records: map[string]*BillingRecord{
			"tenant-1": {
				TenantID:         "tenant-1",
				StripeCustomerID: "cus_123",
			},
		},
		usageSync: map[string]int64{
			checkpointKey("tenant-1", "2026-03-03", "api_requests"):         150,
			checkpointKey("tenant-1", "2026-03-03", "storage_bytes"):        1000,
			checkpointKey("tenant-1", "2026-03-03", "bandwidth_bytes"):      2001,
			checkpointKey("tenant-1", "2026-03-03", "function_invocations"): 11,
		},
	}
	adapter := &fakeStripeAdapter{}
	svc := NewStripeBillingService(repo, defaultBillingCfg(), adapter, testNoopLogger())

	err := svc.ReportUsage(context.Background(), "tenant-1", UsageReport{
		RequestCount:        150,
		DBBytesUsed:         1000,
		BandwidthBytes:      2000,
		FunctionInvocations: 10,
		PeriodEnd:           usageDate,
	})
	testutil.NoError(t, err)
	testutil.Equal(t, 0, len(adapter.meterCalls))
	testutil.Equal(t, 0, len(repo.upsertUsageCalls))
}

func TestStripeService_ReportUsage_AdapterErrorDoesNotUpdateCheckpoint(t *testing.T) {
	t.Parallel()

	usageDate := time.Date(2026, 3, 3, 23, 59, 0, 0, time.UTC)
	repo := &fakeBillingRepo{
		records: map[string]*BillingRecord{
			"tenant-1": {
				TenantID:         "tenant-1",
				StripeCustomerID: "cus_123",
			},
		},
		usageSync: map[string]int64{
			checkpointKey("tenant-1", "2026-03-03", "api_requests"): 100,
		},
	}
	adapter := &fakeStripeAdapter{
		sendMeterEventErr: map[string]error{
			"meter.api_requests": errors.New("stripe unavailable"),
		},
	}
	svc := NewStripeBillingService(repo, defaultBillingCfg(), adapter, testNoopLogger())

	err := svc.ReportUsage(context.Background(), "tenant-1", UsageReport{
		RequestCount: 150,
		PeriodEnd:    usageDate,
	})
	testutil.NoError(t, err)
	testutil.Equal(t, 1, len(adapter.meterCalls))
	testutil.Equal(t, 0, len(repo.upsertUsageCalls))
	testutil.Equal(t, int64(100), repo.usageSync[checkpointKey("tenant-1", "2026-03-03", "api_requests")])
}

func TestStripeService_ReportUsage_NilLoggerIsSafe(t *testing.T) {
	t.Parallel()

	repo := &fakeBillingRepo{records: map[string]*BillingRecord{
		"tenant-1": {TenantID: "tenant-1"},
	}}
	svc := NewStripeBillingService(repo, defaultBillingCfg(), &fakeStripeAdapter{}, nil)

	err := svc.ReportUsage(context.Background(), "tenant-1", UsageReport{})
	testutil.NoError(t, err)
}
