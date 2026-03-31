// Package jobs contains built-in job handler factory functions.
package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/allyourbase/ayb/internal/billing"
	"github.com/allyourbase/ayb/internal/storage"
	"github.com/jackc/pgx/v5/pgxpool"
	"log/slog"
	"time"
)

type providerTokenRefreshPayload struct {
	WindowSeconds int `json:"window_seconds"`
}
type aiUsageAggregationPayload struct {
	Day string `json:"day"` // optional YYYY-MM-DD UTC; defaults to yesterday UTC
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

func AIUsageAggregationJobHandler(aggregator AIUsageAggregator) JobHandler {
	return func(ctx context.Context, payload json.RawMessage) error {
		targetDay := time.Now().UTC().AddDate(0, 0, -1)
		targetDay = time.Date(targetDay.Year(), targetDay.Month(), targetDay.Day(), 0, 0, 0, 0, time.UTC)
		if hasJobPayload(payload) {
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
		if hasJobPayload(payload) {
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

func WebhookDeliveryPruneHandler(pool *pgxpool.Pool, logger *slog.Logger) JobHandler {
	return func(ctx context.Context, payload json.RawMessage) error {
		var p webhookPrunePayload
		if hasJobPayload(payload) {
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

func ResumableUploadCleanupHandler(storageSvc *storage.Service, logger *slog.Logger) JobHandler {
	return func(ctx context.Context, _ json.RawMessage) error {
		if storageSvc == nil {
			return fmt.Errorf("cleanup resumable uploads: storage service is nil")
		}
		deleted, err := storageSvc.CleanupExpiredResumableUploads(ctx)
		if err != nil {
			return fmt.Errorf("cleanup resumable uploads: %w", err)
		}
		if logger != nil {
			logger.Info("cleanup resumable uploads completed", "deleted", deleted)
		}
		return nil
	}
}

func AuditLogRetentionHandler(pool *pgxpool.Pool, defaultRetentionDays int, logger *slog.Logger) JobHandler {
	return func(ctx context.Context, payload json.RawMessage) error {
		retentionDays := defaultRetentionDays
		if retentionDays <= 0 {
			retentionDays = auditLogRetentionDefaultDays
		}
		var p auditRetentionPayload
		if hasJobPayload(payload) {
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

func RequestLogRetentionHandler(pool *pgxpool.Pool, defaultRetentionDays int, logger *slog.Logger) JobHandler {
	return func(ctx context.Context, payload json.RawMessage) error {
		retentionDays := defaultRetentionDays
		if retentionDays <= 0 {
			retentionDays = requestLogRetentionDefaultDays
		}
		var p requestLogRetentionPayload
		if hasJobPayload(payload) {
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
