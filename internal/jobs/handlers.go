// Package jobs defines job handlers and scheduling logic for background tasks including OAuth token refresh, usage aggregation, billing sync, and maintenance operations.
package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/allyourbase/ayb/internal/billing"
	"github.com/allyourbase/ayb/internal/matview"
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

type providerTokenRefreshPayload struct {
	WindowSeconds int `json:"window_seconds"`
}

type aiUsageAggregationPayload struct {
	Day string `json:"day"` // optional YYYY-MM-DD UTC; defaults to yesterday UTC
}

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
func RegisterBuiltinHandlers(svc *Service, pool *pgxpool.Pool, logger *slog.Logger) {
	svc.RegisterHandler("stale_session_cleanup", StaleSessionCleanupHandler(pool, logger))
	svc.RegisterHandler("webhook_delivery_prune", WebhookDeliveryPruneHandler(pool, logger))
	svc.RegisterHandler("expired_oauth_cleanup", ExpiredOAuthCleanupHandler(pool, logger))
	svc.RegisterHandler("expired_auth_cleanup", ExpiredAuthCleanupHandler(pool, logger))
	svc.RegisterHandler(resumableUploadCleanupJobType, ResumableUploadCleanupHandler(pool, logger))
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

// AIUsageAggregationJobHandler aggregates AI usage for a UTC day.
func AIUsageAggregationJobHandler(aggregator AIUsageAggregator) JobHandler {
	return func(ctx context.Context, payload json.RawMessage) error {
		targetDay := time.Now().UTC().AddDate(0, 0, -1)
		targetDay = time.Date(targetDay.Year(), targetDay.Month(), targetDay.Day(), 0, 0, 0, 0, time.UTC)

		if len(payload) > 0 && string(payload) != "{}" {
			var p aiUsageAggregationPayload
			if err := json.Unmarshal(payload, &p); err != nil {
				return fmt.Errorf("ai_usage_aggregate_daily: invalid payload: %w", err)
			}
			if p.Day != "" {
				parsed, err := time.Parse("2006-01-02", p.Day)
				if err != nil {
					return fmt.Errorf("ai_usage_aggregate_daily: invalid day %q: %w", p.Day, err)
				}
				targetDay = parsed.UTC()
			}
		}

		if _, err := aggregator.AggregateDailyUsage(ctx, targetDay); err != nil {
			return fmt.Errorf("ai_usage_aggregate_daily: %w", err)
		}
		return nil
	}
}

// BillingUsageSyncJobHandler reports metered usage deltas for billable tenants.
func BillingUsageSyncJobHandler(billingSvc billing.BillingService, ds billingUsageSyncDataSource) JobHandler {
	return func(ctx context.Context, payload json.RawMessage) error {
		_ = payload
		if billingSvc == nil {
			return fmt.Errorf("billing service is nil")
		}
		if ds == nil {
			return fmt.Errorf("billing usage sync data source is nil")
		}

		targetDate := usageSyncNow().UTC().Truncate(24 * time.Hour)
		tenantIDs, err := ds.ListBillableTenants(ctx)
		if err != nil {
			return fmt.Errorf("list billable tenants: %w", err)
		}

		successes := 0
		failures := 0
		for _, tenantID := range tenantIDs {
			report, found, err := ds.GetUsageReport(ctx, tenantID, targetDate)
			if err != nil {
				failures++
				slog.Default().Error("failed to query tenant usage", "tenant_id", tenantID, "error", err)
				continue
			}
			if !found {
				yesterday := targetDate.AddDate(0, 0, -1)
				report, found, err = ds.GetUsageReport(ctx, tenantID, yesterday)
				if err != nil {
					failures++
					slog.Default().Error("failed to query tenant usage", "tenant_id", tenantID, "error", err)
					continue
				}
				if !found {
					slog.Default().Debug("no usage row for tenant in sync window", "tenant_id", tenantID)
					continue
				}
			}

			if err := billingSvc.ReportUsage(ctx, tenantID, report); err != nil {
				failures++
				slog.Default().Error("billing usage report failed", "tenant_id", tenantID, "error", err)
				continue
			}
			successes++
		}
		slog.Default().Info("billing usage sync summary",
			"tenants", len(tenantIDs),
			"success", successes,
			"failed", failures)
		return nil
	}
}

// ProviderTokenRefreshJobHandler refreshes OAuth provider tokens nearing expiration.
func ProviderTokenRefreshJobHandler(refresher ProviderTokenRefreshService) JobHandler {
	return func(ctx context.Context, payload json.RawMessage) error {
		window := providerTokenRefreshDefaultWindow
		if len(payload) > 0 && string(payload) != "{}" {
			var p providerTokenRefreshPayload
			if err := json.Unmarshal(payload, &p); err != nil {
				return fmt.Errorf("oauth_provider_tokens_refresh: invalid payload: %w", err)
			}
			if p.WindowSeconds > 0 {
				window = time.Duration(p.WindowSeconds) * time.Second
			}
		}
		return refresher.RefreshExpiringProviderTokens(ctx, window)
	}
}

// RegisterProviderTokenRefreshSchedule registers a 5-minute schedule for proactive refresh.
func RegisterProviderTokenRefreshSchedule(ctx context.Context, svc *Service) error {
	if svc == nil {
		return fmt.Errorf("job service is nil")
	}

	schedule := &Schedule{
		Name:        providerTokenRefreshScheduleName,
		JobType:     providerTokenRefreshJobType,
		Payload:     json.RawMessage(`{"window_seconds":600}`),
		CronExpr:    providerTokenRefreshCronExpression,
		Timezone:    "UTC",
		Enabled:     true,
		MaxAttempts: 3,
	}
	next, err := CronNextTime(schedule.CronExpr, schedule.Timezone, time.Now())
	if err != nil {
		return fmt.Errorf("compute next_run_at for %s: %w", schedule.Name, err)
	}
	schedule.NextRunAt = &next

	if _, err := svc.store.UpsertSchedule(ctx, schedule); err != nil {
		return fmt.Errorf("upsert provider token refresh schedule %s: %w", schedule.Name, err)
	}
	return nil
}

// RegisterAIUsageAggregationSchedule registers a daily UTC schedule for AI usage rollups.
func RegisterAIUsageAggregationSchedule(ctx context.Context, svc *Service) error {
	if svc == nil {
		return fmt.Errorf("job service is nil")
	}

	schedule := &Schedule{
		Name:        aiUsageAggregationScheduleName,
		JobType:     AIUsageAggregationJobType,
		Payload:     json.RawMessage(`{}`),
		CronExpr:    aiUsageAggregationCronExpr,
		Timezone:    "UTC",
		Enabled:     true,
		MaxAttempts: 3,
	}
	next, err := CronNextTime(schedule.CronExpr, schedule.Timezone, time.Now())
	if err != nil {
		return fmt.Errorf("compute next_run_at for %s: %w", schedule.Name, err)
	}
	schedule.NextRunAt = &next

	if _, err := svc.store.UpsertSchedule(ctx, schedule); err != nil {
		return fmt.Errorf("upsert ai usage aggregation schedule %s: %w", schedule.Name, err)
	}
	return nil
}

// RegisterBillingUsageSyncSchedule registers the recurring billing usage sync schedule.
func RegisterBillingUsageSyncSchedule(ctx context.Context, svc *Service, usageSyncIntervalSecs int) error {
	if svc == nil {
		return fmt.Errorf("job service is nil")
	}
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
	next, err := CronNextTime(schedule.CronExpr, schedule.Timezone, time.Now())
	if err != nil {
		return fmt.Errorf("compute next_run_at for %s: %w", schedule.Name, err)
	}
	schedule.NextRunAt = &next

	if _, err := svc.store.UpsertSchedule(ctx, schedule); err != nil {
		return fmt.Errorf("upsert billing usage sync schedule %s: %w", schedule.Name, err)
	}
	return nil
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

// StaleSessionCleanupHandler deletes expired refresh-token sessions.
func StaleSessionCleanupHandler(pool *pgxpool.Pool, logger *slog.Logger) JobHandler {
	return func(ctx context.Context, payload json.RawMessage) error {
		tag, err := pool.Exec(ctx,
			`DELETE FROM _ayb_sessions WHERE expires_at < NOW()`)
		if err != nil {
			return fmt.Errorf("stale_session_cleanup: %w", err)
		}
		logger.Info("stale_session_cleanup completed", "deleted", tag.RowsAffected())
		return nil
	}
}

// webhookPrunePayload is the expected payload for webhook_delivery_prune jobs.
type webhookPrunePayload struct {
	RetentionHours int `json:"retention_hours"`
}

type auditRetentionPayload struct {
	RetentionDays int `json:"retention_days"`
}

type requestLogRetentionPayload struct {
	RetentionDays int `json:"retention_days"`
}

// WebhookDeliveryPruneHandler deletes old webhook delivery logs.
func WebhookDeliveryPruneHandler(pool *pgxpool.Pool, logger *slog.Logger) JobHandler {
	return func(ctx context.Context, payload json.RawMessage) error {
		var p webhookPrunePayload
		if len(payload) > 0 && string(payload) != "{}" {
			if err := json.Unmarshal(payload, &p); err != nil {
				return fmt.Errorf("webhook_delivery_prune: invalid payload: %w", err)
			}
		}
		if p.RetentionHours <= 0 {
			p.RetentionHours = 168 // 7 days default
		}

		tag, err := pool.Exec(ctx,
			`DELETE FROM _ayb_webhook_deliveries
			 WHERE delivered_at < NOW() - make_interval(hours => $1)`,
			p.RetentionHours)
		if err != nil {
			return fmt.Errorf("webhook_delivery_prune: %w", err)
		}
		logger.Info("webhook_delivery_prune completed",
			"deleted", tag.RowsAffected(), "retention_hours", p.RetentionHours)
		return nil
	}
}

// ExpiredOAuthCleanupHandler deletes expired/revoked OAuth tokens and used auth codes.
func ExpiredOAuthCleanupHandler(pool *pgxpool.Pool, logger *slog.Logger) JobHandler {
	return func(ctx context.Context, payload json.RawMessage) error {
		// Delete expired OAuth tokens (expired > 1 day ago).
		tagTokens, err := pool.Exec(ctx,
			`DELETE FROM _ayb_oauth_tokens
			 WHERE (expires_at < NOW() - interval '1 day')
			    OR (revoked_at IS NOT NULL AND revoked_at < NOW() - interval '1 day')`)
		if err != nil {
			return fmt.Errorf("expired_oauth_cleanup tokens: %w", err)
		}

		// Delete expired authorization codes.
		tagCodes, err := pool.Exec(ctx,
			`DELETE FROM _ayb_oauth_authorization_codes
			 WHERE expires_at < NOW()
			    OR (used_at IS NOT NULL AND used_at < NOW() - interval '1 day')`)
		if err != nil {
			return fmt.Errorf("expired_oauth_cleanup codes: %w", err)
		}

		logger.Info("expired_oauth_cleanup completed",
			"tokens_deleted", tagTokens.RowsAffected(),
			"codes_deleted", tagCodes.RowsAffected())
		return nil
	}
}

// ExpiredAuthCleanupHandler deletes expired magic links and password resets.
func ExpiredAuthCleanupHandler(pool *pgxpool.Pool, logger *slog.Logger) JobHandler {
	return func(ctx context.Context, payload json.RawMessage) error {
		tagLinks, err := pool.Exec(ctx,
			`DELETE FROM _ayb_magic_links WHERE expires_at < NOW()`)
		if err != nil {
			return fmt.Errorf("expired_auth_cleanup magic_links: %w", err)
		}

		tagResets, err := pool.Exec(ctx,
			`DELETE FROM _ayb_password_resets WHERE expires_at < NOW()`)
		if err != nil {
			return fmt.Errorf("expired_auth_cleanup password_resets: %w", err)
		}

		logger.Info("expired_auth_cleanup completed",
			"magic_links_deleted", tagLinks.RowsAffected(),
			"password_resets_deleted", tagResets.RowsAffected())
		return nil
	}
}

// ResumableUploadCleanupHandler removes stale resumable uploads and temp files.
func ResumableUploadCleanupHandler(pool *pgxpool.Pool, logger *slog.Logger) JobHandler {
	return func(ctx context.Context, _ json.RawMessage) error {
		rows, err := pool.Query(ctx, `SELECT path FROM _ayb_storage_uploads WHERE expires_at < NOW()`)
		if err != nil {
			return fmt.Errorf("cleanup resumable uploads: query paths: %w", err)
		}
		defer rows.Close()

		var paths []string
		for rows.Next() {
			var path string
			if err := rows.Scan(&path); err != nil {
				return fmt.Errorf("cleanup resumable uploads: scan path: %w", err)
			}
			paths = append(paths, path)
		}
		if err := rows.Err(); err != nil {
			return fmt.Errorf("cleanup resumable uploads: read paths: %w", err)
		}

		tag, err := pool.Exec(ctx, `DELETE FROM _ayb_storage_uploads WHERE expires_at < NOW()`)
		if err != nil {
			return fmt.Errorf("cleanup resumable uploads: delete stale sessions: %w", err)
		}

		for _, path := range paths {
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				if logger != nil {
					logger.Warn("failed to remove stale resumable file", "path", path, "error", err)
				}
			}
		}

		if logger != nil {
			logger.Info("cleanup resumable uploads completed", "deleted", tag.RowsAffected())
		}
		return nil
	}
}

// AuditLogRetentionHandler deletes audit log entries older than retention_days.
func AuditLogRetentionHandler(pool *pgxpool.Pool, defaultRetentionDays int, logger *slog.Logger) JobHandler {
	return func(ctx context.Context, payload json.RawMessage) error {
		retentionDays := defaultRetentionDays
		if retentionDays <= 0 {
			retentionDays = auditLogRetentionDefaultDays
		}

		var p auditRetentionPayload
		if len(payload) > 0 && string(payload) != "{}" {
			if err := json.Unmarshal(payload, &p); err != nil {
				return fmt.Errorf("audit_log_retention: invalid payload: %w", err)
			}
			if p.RetentionDays > 0 {
				retentionDays = p.RetentionDays
			}
		}

		tag, err := pool.Exec(ctx, `
			DELETE FROM _ayb_audit_log
			 WHERE timestamp < NOW() - make_interval(days => $1)`,
			retentionDays)
		if err != nil {
			return fmt.Errorf("audit_log_retention: %w", err)
		}
		if logger != nil {
			logger.Info("audit_log_retention completed",
				"deleted", tag.RowsAffected(),
				"retention_days", retentionDays)
		}
		return nil
	}
}

// RequestLogRetentionHandler deletes request logs older than retention_days.
func RequestLogRetentionHandler(pool *pgxpool.Pool, defaultRetentionDays int, logger *slog.Logger) JobHandler {
	return func(ctx context.Context, payload json.RawMessage) error {
		retentionDays := defaultRetentionDays
		if retentionDays <= 0 {
			retentionDays = requestLogRetentionDefaultDays
		}

		var p requestLogRetentionPayload
		if len(payload) > 0 && string(payload) != "{}" {
			if err := json.Unmarshal(payload, &p); err != nil {
				return fmt.Errorf("request_log_retention: invalid payload: %w", err)
			}
			if p.RetentionDays > 0 {
				retentionDays = p.RetentionDays
			}
		}

		tag, err := pool.Exec(ctx, `
			DELETE FROM _ayb_request_logs
			 WHERE timestamp < NOW() - make_interval(days => $1)`,
			retentionDays)
		if err != nil {
			return fmt.Errorf("request_log_retention: %w", err)
		}
		if logger != nil {
			logger.Info("request_log_retention completed",
				"deleted", tag.RowsAffected(),
				"retention_days", retentionDays)
		}
		return nil
	}
}
