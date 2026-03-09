//go:build integration

package jobs_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/jobs"
	"github.com/allyourbase/ayb/internal/migrations"
	"github.com/allyourbase/ayb/internal/testutil"
)

// setupHandlerDB sets up a clean DB with migrations and returns the pool.
func setupHandlerDB(t *testing.T) {
	t.Helper()
	ctx := context.Background()

	_, err := sharedPG.Pool.Exec(ctx, "DROP SCHEMA public CASCADE; CREATE SCHEMA public")
	testutil.NoError(t, err)

	runner := migrations.NewRunner(sharedPG.Pool, testutil.DiscardLogger())
	err = runner.Bootstrap(ctx)
	testutil.NoError(t, err)
	_, err = runner.Run(ctx)
	testutil.NoError(t, err)
}

func TestStaleSessionCleanupHandler(t *testing.T) {
	setupHandlerDB(t)
	ctx := context.Background()
	pool := sharedPG.Pool

	// Seed a user (sessions have FK to users).
	var userID string
	err := pool.QueryRow(ctx,
		`INSERT INTO _ayb_users (email, password_hash) VALUES ('test@example.com', 'hash')
		 RETURNING id`).Scan(&userID)
	testutil.NoError(t, err)

	// Insert 2 expired sessions and 1 active.
	_, err = pool.Exec(ctx,
		`INSERT INTO _ayb_sessions (user_id, token_hash, expires_at) VALUES
		 ($1, 'expired1', NOW() - interval '1 hour'),
		 ($1, 'expired2', NOW() - interval '2 hours'),
		 ($1, 'active1', NOW() + interval '1 hour')`, userID)
	testutil.NoError(t, err)

	// Run handler.
	handler := jobs.StaleSessionCleanupHandler(pool, testutil.DiscardLogger())
	err = handler(ctx, nil)
	testutil.NoError(t, err)

	// Verify: only active session remains.
	var count int
	err = pool.QueryRow(ctx, `SELECT COUNT(*) FROM _ayb_sessions`).Scan(&count)
	testutil.NoError(t, err)
	testutil.Equal(t, 1, count)
}

func TestWebhookDeliveryPruneHandler(t *testing.T) {
	setupHandlerDB(t)
	ctx := context.Background()
	pool := sharedPG.Pool

	// Create a webhook first (deliveries have FK).
	var whID string
	err := pool.QueryRow(ctx,
		`INSERT INTO _ayb_webhooks (url, secret, events, tables, enabled)
		 VALUES ('https://example.com/hook', 'secret', '{}', '{}', true)
		 RETURNING id`).Scan(&whID)
	testutil.NoError(t, err)

	// Insert old and recent deliveries.
	_, err = pool.Exec(ctx,
		`INSERT INTO _ayb_webhook_deliveries (webhook_id, event_action, event_table, success, status_code, attempt, duration_ms, delivered_at)
		 VALUES
		 ($1, 'INSERT', 'test', true, 200, 1, 50, NOW() - interval '10 days'),
		 ($1, 'INSERT', 'test', true, 200, 1, 50, NOW() - interval '8 days'),
		 ($1, 'INSERT', 'test', true, 200, 1, 50, NOW() - interval '1 day')`, whID)
	testutil.NoError(t, err)

	// Run with 168h retention (7 days).
	handler := jobs.WebhookDeliveryPruneHandler(pool, testutil.DiscardLogger())
	err = handler(ctx, json.RawMessage(`{"retention_hours": 168}`))
	testutil.NoError(t, err)

	// 2 older than 7 days should be deleted, 1 remains.
	var count int
	err = pool.QueryRow(ctx, `SELECT COUNT(*) FROM _ayb_webhook_deliveries`).Scan(&count)
	testutil.NoError(t, err)
	testutil.Equal(t, 1, count)
}

func TestWebhookDeliveryPruneHandlerDefaultRetention(t *testing.T) {
	setupHandlerDB(t)
	ctx := context.Background()
	pool := sharedPG.Pool

	// Empty payload should use default 168h.
	handler := jobs.WebhookDeliveryPruneHandler(pool, testutil.DiscardLogger())
	err := handler(ctx, nil)
	testutil.NoError(t, err) // no deliveries, no error
}

func TestExpiredOAuthCleanupHandler(t *testing.T) {
	setupHandlerDB(t)
	ctx := context.Background()
	pool := sharedPG.Pool

	// Seed user + OAuth client (required by FKs).
	var userID string
	err := pool.QueryRow(ctx,
		`INSERT INTO _ayb_users (email, password_hash) VALUES ('oauth@example.com', 'hash')
		 RETURNING id`).Scan(&userID)
	testutil.NoError(t, err)

	// Need an app first (oauth_clients references apps, apps require owner_user_id).
	var appID string
	err = pool.QueryRow(ctx,
		`INSERT INTO _ayb_apps (name, description, owner_user_id) VALUES ('test-app', 'test', $1)
		 RETURNING id`, userID).Scan(&appID)
	testutil.NoError(t, err)

	// client_id must match ^ayb_cid_[0-9a-f]{48}$ per CHECK constraint.
	testClientID := "ayb_cid_aabbccdd00112233445566778899aabbccdd001122334455"
	// client_secret_hash must be 64 hex chars (SHA-256) for confidential client type.
	testSecretHash := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

	_, err = pool.Exec(ctx,
		`INSERT INTO _ayb_oauth_clients (client_id, client_secret_hash, app_id, name, redirect_uris, scopes, client_type)
		 VALUES ($2, $3, $1, 'test-client', '{"https://example.com/cb"}', '{"readonly"}', 'confidential')`,
		appID, testClientID, testSecretHash)
	testutil.NoError(t, err)

	// Insert expired token (> 1 day old).
	_, err = pool.Exec(ctx,
		`INSERT INTO _ayb_oauth_tokens (token_hash, token_type, client_id, user_id, scope, grant_id, expires_at)
		 VALUES ('expired_tok', 'access', $2, $1, 'readonly', gen_random_uuid(), NOW() - interval '2 days')`, userID, testClientID)
	testutil.NoError(t, err)

	// Insert active token.
	_, err = pool.Exec(ctx,
		`INSERT INTO _ayb_oauth_tokens (token_hash, token_type, client_id, user_id, scope, grant_id, expires_at)
		 VALUES ('active_tok', 'access', $2, $1, 'readonly', gen_random_uuid(), NOW() + interval '1 hour')`, userID, testClientID)
	testutil.NoError(t, err)

	// Insert expired auth code.
	_, err = pool.Exec(ctx,
		`INSERT INTO _ayb_oauth_authorization_codes (code_hash, client_id, user_id, redirect_uri, scope, code_challenge, state, expires_at)
		 VALUES ('expired_code', $2, $1, 'https://example.com/cb', 'readonly', 'challenge', 'state1', NOW() - interval '1 hour')`, userID, testClientID)
	testutil.NoError(t, err)

	// Insert active auth code.
	_, err = pool.Exec(ctx,
		`INSERT INTO _ayb_oauth_authorization_codes (code_hash, client_id, user_id, redirect_uri, scope, code_challenge, state, expires_at)
		 VALUES ('active_code', $2, $1, 'https://example.com/cb', 'readonly', 'challenge', 'state2', NOW() + interval '10 minutes')`, userID, testClientID)
	testutil.NoError(t, err)

	handler := jobs.ExpiredOAuthCleanupHandler(pool, testutil.DiscardLogger())
	err = handler(ctx, nil)
	testutil.NoError(t, err)

	// Expired token deleted, active remains.
	var tokenCount int
	err = pool.QueryRow(ctx, `SELECT COUNT(*) FROM _ayb_oauth_tokens`).Scan(&tokenCount)
	testutil.NoError(t, err)
	testutil.Equal(t, 1, tokenCount)

	// Expired code deleted, active remains.
	var codeCount int
	err = pool.QueryRow(ctx, `SELECT COUNT(*) FROM _ayb_oauth_authorization_codes`).Scan(&codeCount)
	testutil.NoError(t, err)
	testutil.Equal(t, 1, codeCount)
}

func TestExpiredAuthCleanupHandler(t *testing.T) {
	setupHandlerDB(t)
	ctx := context.Background()
	pool := sharedPG.Pool

	// Seed user (password resets have FK).
	var userID string
	err := pool.QueryRow(ctx,
		`INSERT INTO _ayb_users (email, password_hash) VALUES ('auth@example.com', 'hash')
		 RETURNING id`).Scan(&userID)
	testutil.NoError(t, err)

	// Insert expired and active magic links.
	_, err = pool.Exec(ctx,
		`INSERT INTO _ayb_magic_links (email, token_hash, expires_at) VALUES
		 ('auth@example.com', 'expired_link', NOW() - interval '1 hour'),
		 ('auth@example.com', 'active_link', NOW() + interval '1 hour')`)
	testutil.NoError(t, err)

	// Insert expired and active password resets.
	_, err = pool.Exec(ctx,
		`INSERT INTO _ayb_password_resets (user_id, token_hash, expires_at) VALUES
		 ($1, 'expired_reset', NOW() - interval '1 hour'),
		 ($1, 'active_reset', NOW() + interval '1 hour')`, userID)
	testutil.NoError(t, err)

	handler := jobs.ExpiredAuthCleanupHandler(pool, testutil.DiscardLogger())
	err = handler(ctx, nil)
	testutil.NoError(t, err)

	// Only active magic link remains.
	var linkCount int
	err = pool.QueryRow(ctx, `SELECT COUNT(*) FROM _ayb_magic_links`).Scan(&linkCount)
	testutil.NoError(t, err)
	testutil.Equal(t, 1, linkCount)

	// Only active password reset remains.
	var resetCount int
	err = pool.QueryRow(ctx, `SELECT COUNT(*) FROM _ayb_password_resets`).Scan(&resetCount)
	testutil.NoError(t, err)
	testutil.Equal(t, 1, resetCount)
}

func TestMatviewRefreshHandlerIntegration(t *testing.T) {
	setupHandlerDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	pool := sharedPG.Pool

	// Create a materialized view to refresh.
	_, err := pool.Exec(ctx, `CREATE TABLE public.mv_scores (id serial PRIMARY KEY, score int)`)
	testutil.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO public.mv_scores (score) VALUES (10), (20)`)
	testutil.NoError(t, err)
	_, err = pool.Exec(ctx, `CREATE MATERIALIZED VIEW public.mv_totals AS SELECT sum(score) AS total FROM public.mv_scores`)
	testutil.NoError(t, err)

	// Set up the jobs service with the matview refresh handler.
	store := jobs.NewStore(pool)
	cfg := jobs.DefaultServiceConfig()
	cfg.PollInterval = 100 * time.Millisecond
	cfg.LeaseDuration = 5 * time.Second
	cfg.WorkerConcurrency = 2
	cfg.SchedulerTick = 200 * time.Millisecond

	svc := jobs.NewService(store, testutil.DiscardLogger(), cfg)
	jobs.RegisterBuiltinHandlers(svc, pool, testutil.DiscardLogger())

	// Enqueue a matview refresh job.
	_, err = svc.Enqueue(ctx, "materialized_view_refresh",
		json.RawMessage(`{"schema":"public","view_name":"mv_totals"}`),
		jobs.EnqueueOpts{})
	testutil.NoError(t, err)

	svc.Start(ctx)
	defer svc.Stop()

	// Wait for job to complete.
	deadline := time.After(10 * time.Second)
	for {
		completed, err := svc.List(ctx, "completed", "materialized_view_refresh", 10, 0)
		testutil.NoError(t, err)
		if len(completed) == 1 {
			break
		}
		// Also check for failed jobs to avoid hanging on errors.
		failed, err := svc.List(ctx, "failed", "materialized_view_refresh", 10, 0)
		testutil.NoError(t, err)
		if len(failed) > 0 {
			t.Fatalf("matview refresh job failed: %v", failed[0].LastError)
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for matview refresh job")
		default:
			time.Sleep(50 * time.Millisecond)
		}
	}

	// Insert more data, enqueue another refresh, verify data is actually refreshed.
	_, err = pool.Exec(ctx, `INSERT INTO public.mv_scores (score) VALUES (30)`)
	testutil.NoError(t, err)

	_, err = svc.Enqueue(ctx, "materialized_view_refresh",
		json.RawMessage(`{"schema":"public","view_name":"mv_totals"}`),
		jobs.EnqueueOpts{})
	testutil.NoError(t, err)

	deadline2 := time.After(10 * time.Second)
	for {
		completed, err := svc.List(ctx, "completed", "materialized_view_refresh", 10, 0)
		testutil.NoError(t, err)
		if len(completed) == 2 {
			break
		}
		select {
		case <-deadline2:
			t.Fatal("timed out waiting for second matview refresh job")
		default:
			time.Sleep(50 * time.Millisecond)
		}
	}

	var total int
	err = pool.QueryRow(ctx, `SELECT total FROM public.mv_totals`).Scan(&total)
	testutil.NoError(t, err)
	testutil.Equal(t, 60, total) // 10+20+30
}

func TestHandlersRunThroughService(t *testing.T) {
	setupHandlerDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	store := jobs.NewStore(sharedPG.Pool)
	cfg := jobs.DefaultServiceConfig()
	cfg.PollInterval = 100 * time.Millisecond
	cfg.LeaseDuration = 5 * time.Second
	cfg.WorkerConcurrency = 2
	cfg.SchedulerTick = 200 * time.Millisecond

	svc := jobs.NewService(store, testutil.DiscardLogger(), cfg)
	jobs.RegisterBuiltinHandlers(svc, sharedPG.Pool, testutil.DiscardLogger())

	// Seed a user for the cleanup jobs to delete from.
	var userID string
	err := sharedPG.Pool.QueryRow(ctx,
		`INSERT INTO _ayb_users (email, password_hash) VALUES ('svc@example.com', 'hash')
		 RETURNING id`).Scan(&userID)
	testutil.NoError(t, err)

	// Insert expired session.
	_, err = sharedPG.Pool.Exec(ctx,
		`INSERT INTO _ayb_sessions (user_id, token_hash, expires_at)
		 VALUES ($1, 'exp_sess', NOW() - interval '1 hour')`, userID)
	testutil.NoError(t, err)

	// Enqueue the cleanup job.
	_, err = svc.Enqueue(ctx, "stale_session_cleanup", nil, jobs.EnqueueOpts{})
	testutil.NoError(t, err)

	svc.Start(ctx)
	defer svc.Stop()

	// Wait for job to complete.
	deadline := time.After(5 * time.Second)
	for {
		completed, err := svc.List(ctx, "completed", "stale_session_cleanup", 10, 0)
		testutil.NoError(t, err)
		if len(completed) == 1 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for handler execution")
		default:
			time.Sleep(50 * time.Millisecond)
		}
	}

	// Verify expired session was cleaned up.
	var count int
	err = sharedPG.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM _ayb_sessions`).Scan(&count)
	testutil.NoError(t, err)
	testutil.Equal(t, 0, count)
}
