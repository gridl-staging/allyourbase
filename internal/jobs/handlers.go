package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/allyourbase/ayb/internal/billing"
	"github.com/allyourbase/ayb/internal/matview"
	"github.com/allyourbase/ayb/internal/storage"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	providerTokenRefreshJobType        = "oauth_provider_tokens_refresh"
	providerTokenRefreshScheduleName   = "oauth_provider_tokens_refresh_10m"
	providerTokenRefreshDefaultWindow  = 10 * time.Minute
	providerTokenRefreshCronExpression = "*/5 * * * *"
	AIUsageAggregationJobType          = "ai_usage_aggregate_daily"
	aiUsageAggregationScheduleName     = "ai_usage_aggregate_daily"
	aiUsageAggregationCronExpr         = "15 0 * * *"
	resumableUploadCleanupJobType      = "expired_resumable_upload_cleanup"
	resumableUploadCleanupScheduleName = "expired_resumable_upload_cleanup"
	resumableUploadCleanupCronExpr     = "*/10 * * * *"
	billingUsageSyncJobType            = "billing_usage_sync"
	billingUsageSyncScheduleName       = "billing_usage_sync"
	auditLogRetentionDefaultDays       = 90
	requestLogRetentionDefaultDays     = 7
)

var usageSyncNow = func() time.Time { return time.Now() }

type ProviderTokenRefreshService interface {
	RefreshExpiringProviderTokens(ctx context.Context, window time.Duration) error
}

type AIUsageAggregator interface {
	AggregateDailyUsage(ctx context.Context, day time.Time) (int64, error)
}

type billingUsageSyncDataSource interface {
	ListBillableTenants(ctx context.Context) ([]string, error)
	GetUsageReport(ctx context.Context, tenantID string, usageDate time.Time) (billing.UsageReport, bool, error)
}

type billingUsageSyncStore struct {
	pool *pgxpool.Pool
}

func hasJobPayload(payload json.RawMessage) bool {
	return len(payload) > 0 && string(payload) != "{}"
}

func registerSchedule(ctx context.Context, svc *Service, schedule *Schedule) error {
	if svc == nil {
		return fmt.Errorf("job service is nil")
	}

	next, err := CronNextTime(schedule.CronExpr, schedule.Timezone, time.Now())
	if err != nil {
		return fmt.Errorf("compute next_run_at for %s: %w", schedule.Name, err)
	}
	schedule.NextRunAt = &next

	if _, err := svc.store.UpsertSchedule(ctx, schedule); err != nil {
		return fmt.Errorf("upsert schedule %s: %w", schedule.Name, err)
	}
	return nil
}

// ListBillableTenants queries the database for tenant IDs with active billing plans and Stripe customer IDs.
func (s billingUsageSyncStore) ListBillableTenants(ctx context.Context) ([]string, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT DISTINCT tenant_id
		 FROM _ayb_billing
		 WHERE plan <> $1
		   AND stripe_customer_id IS NOT NULL`,
		string(billing.PlanFree),
	)
	if err != nil {
		return nil, fmt.Errorf("query billable tenants: %w", err)
	}
	defer rows.Close()

	var tenantIDs []string
	for rows.Next() {
		var tenantID string
		if err := rows.Scan(&tenantID); err != nil {
			return nil, fmt.Errorf("scan tenant id: %w", err)
		}
		tenantIDs = append(tenantIDs, tenantID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate tenant ids: %w", err)
	}
	return tenantIDs, nil
}

// GetUsageReport retrieves usage metrics for a tenant on a specific date from the database. It returns the report, a boolean indicating if a row was found, and any database error.
func (s billingUsageSyncStore) GetUsageReport(ctx context.Context, tenantID string, usageDate time.Time) (billing.UsageReport, bool, error) {
	var report billing.UsageReport
	var usageDay time.Time
	err := s.pool.QueryRow(ctx,
		`SELECT date, request_count, db_bytes_used, bandwidth_bytes, function_invocations
		   FROM _ayb_tenant_usage_daily
		  WHERE tenant_id = $1 AND date = $2::date`,
		tenantID,
		usageDate.Format("2006-01-02"),
	).Scan(
		&usageDay,
		&report.RequestCount,
		&report.DBBytesUsed,
		&report.BandwidthBytes,
		&report.FunctionInvocations,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return billing.UsageReport{}, false, nil
		}
		return billing.UsageReport{}, false, fmt.Errorf("query usage report for tenant %q at %q: %w", tenantID, usageDate.Format("2006-01-02"), err)
	}
	report.TenantID = tenantID
	report.PeriodEnd = usageDay
	return report, true, nil
}

// RegisterBuiltinHandlers registers all built-in job type handlers.
func RegisterBuiltinHandlers(svc *Service, pool *pgxpool.Pool, storageSvc *storage.Service, logger *slog.Logger) {
	svc.RegisterHandler("stale_session_cleanup", StaleSessionCleanupHandler(pool, logger))
	svc.RegisterHandler("webhook_delivery_prune", WebhookDeliveryPruneHandler(pool, logger))
	svc.RegisterHandler("expired_oauth_cleanup", ExpiredOAuthCleanupHandler(pool, logger))
	svc.RegisterHandler("expired_auth_cleanup", ExpiredAuthCleanupHandler(pool, logger))
	svc.RegisterHandler(resumableUploadCleanupJobType, ResumableUploadCleanupHandler(storageSvc, logger))
	svc.RegisterHandler("audit_log_retention", AuditLogRetentionHandler(pool, auditLogRetentionDefaultDays, logger))
	svc.RegisterHandler("request_log_retention", RequestLogRetentionHandler(pool, requestLogRetentionDefaultDays, logger))

	mvStore := matview.NewStore(pool)
	mvSvc := matview.NewService(mvStore)
	svc.RegisterHandler("materialized_view_refresh", matview.MatviewRefreshHandler(mvSvc, mvStore))
}

// RegisterProviderTokenRefreshHandler registers the OAuth provider token refresh handler.
func RegisterProviderTokenRefreshHandler(svc *Service, refresher ProviderTokenRefreshService) {
	if svc == nil || refresher == nil {
		return
	}
	svc.RegisterHandler(providerTokenRefreshJobType, ProviderTokenRefreshJobHandler(refresher))
}

// RegisterAIUsageAggregationHandler registers the daily usage aggregation job handler.
func RegisterAIUsageAggregationHandler(svc *Service, aggregator AIUsageAggregator) {
	if svc == nil || aggregator == nil {
		return
	}
	svc.RegisterHandler(AIUsageAggregationJobType, AIUsageAggregationJobHandler(aggregator))
}

// RegisterBillingUsageSyncHandler registers the metered usage sync handler.
func RegisterBillingUsageSyncHandler(svc *Service, billingSvc billing.BillingService, pool *pgxpool.Pool) {
	if svc == nil || billingSvc == nil || pool == nil {
		return
	}
	svc.RegisterHandler(billingUsageSyncJobType, BillingUsageSyncJobHandler(billingSvc, billingUsageSyncStore{pool: pool}))
}

// RegisterProviderTokenRefreshSchedule registers a 5-minute schedule for proactive refresh.
func RegisterProviderTokenRefreshSchedule(ctx context.Context, svc *Service) error {
	schedule := &Schedule{
		Name:        providerTokenRefreshScheduleName,
		JobType:     providerTokenRefreshJobType,
		Payload:     json.RawMessage(`{"window_seconds":600}`),
		CronExpr:    providerTokenRefreshCronExpression,
		Timezone:    "UTC",
		Enabled:     true,
		MaxAttempts: 3,
	}
	return registerSchedule(ctx, svc, schedule)
}

// RegisterAIUsageAggregationSchedule registers a daily UTC schedule for AI usage rollups.
func RegisterAIUsageAggregationSchedule(ctx context.Context, svc *Service) error {
	schedule := &Schedule{
		Name:        aiUsageAggregationScheduleName,
		JobType:     AIUsageAggregationJobType,
		Payload:     json.RawMessage(`{}`),
		CronExpr:    aiUsageAggregationCronExpr,
		Timezone:    "UTC",
		Enabled:     true,
		MaxAttempts: 3,
	}
	return registerSchedule(ctx, svc, schedule)
}

func RegisterBillingUsageSyncSchedule(ctx context.Context, svc *Service, usageSyncIntervalSecs int) error {
	cronExpr, err := usageSyncCronExpr(usageSyncIntervalSecs)
	if err != nil {
		return fmt.Errorf("compute billing usage sync cron expression: %w", err)
	}

	schedule := &Schedule{
		Name:        billingUsageSyncScheduleName,
		JobType:     billingUsageSyncJobType,
		Payload:     json.RawMessage(`{}`),
		CronExpr:    cronExpr,
		Timezone:    "UTC",
		Enabled:     true,
		MaxAttempts: 3,
	}
	return registerSchedule(ctx, svc, schedule)
}

// usageSyncCronExpr generates a cron expression for the given billing sync interval in seconds. The interval must be positive and a multiple of 60; the returned expression matches the appropriate schedule granularity.
func usageSyncCronExpr(usageSyncIntervalSecs int) (string, error) {
	if usageSyncIntervalSecs <= 0 {
		return "", fmt.Errorf("billing usage sync interval must be positive, got %d", usageSyncIntervalSecs)
	}
	if usageSyncIntervalSecs%60 != 0 {
		return "", fmt.Errorf("billing usage sync interval must be a multiple of 60, got %d", usageSyncIntervalSecs)
	}
	minutes := usageSyncIntervalSecs / 60
	const minutesPerDay = 24 * 60
	switch {
	case minutes < 60 && minutes > 0:
		return fmt.Sprintf("*/%d * * * *", minutes), nil
	case minutes == 60:
		return "0 * * * *", nil
	case minutes < minutesPerDay && minutes%60 == 0:
		hours := minutes / 60
		return fmt.Sprintf("0 */%d * * *", hours), nil
	case minutes == minutesPerDay:
		return "0 0 * * *", nil
	default:
		return "", fmt.Errorf("unsupported billing usage sync interval: %d seconds", usageSyncIntervalSecs)
	}
}
