package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/allyourbase/ayb/internal/matview"
	"github.com/jackc/pgx/v5/pgxpool"
)

// RegisterBuiltinHandlers registers all built-in job type handlers.
func RegisterBuiltinHandlers(svc *Service, pool *pgxpool.Pool, logger *slog.Logger) {
	svc.RegisterHandler("stale_session_cleanup", StaleSessionCleanupHandler(pool, logger))
	svc.RegisterHandler("webhook_delivery_prune", WebhookDeliveryPruneHandler(pool, logger))
	svc.RegisterHandler("expired_oauth_cleanup", ExpiredOAuthCleanupHandler(pool, logger))
	svc.RegisterHandler("expired_auth_cleanup", ExpiredAuthCleanupHandler(pool, logger))

	mvStore := matview.NewStore(pool)
	mvSvc := matview.NewService(mvStore)
	svc.RegisterHandler("materialized_view_refresh", matview.MatviewRefreshHandler(mvSvc, mvStore))
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
