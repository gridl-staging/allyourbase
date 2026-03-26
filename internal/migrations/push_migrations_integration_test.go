//go:build integration

package migrations_test

import (
	"context"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/migrations"
	"github.com/allyourbase/ayb/internal/testutil"
)

func TestPushMigrationsConstraints(t *testing.T) {
	ctx := context.Background()
	resetDB(t, ctx)

	runner := migrations.NewRunner(sharedPG.Pool, testutil.DiscardLogger())
	err := runner.Bootstrap(ctx)
	testutil.NoError(t, err)
	_, err = runner.Run(ctx)
	testutil.NoError(t, err)

	var tokensExists bool
	err = sharedPG.Pool.QueryRow(ctx,
		`SELECT EXISTS(
			SELECT 1 FROM information_schema.tables
			WHERE table_name = '_ayb_device_tokens'
		)`,
	).Scan(&tokensExists)
	testutil.NoError(t, err)
	testutil.True(t, tokensExists, "_ayb_device_tokens table should exist")

	var deliveriesExists bool
	err = sharedPG.Pool.QueryRow(ctx,
		`SELECT EXISTS(
			SELECT 1 FROM information_schema.tables
			WHERE table_name = '_ayb_push_deliveries'
		)`,
	).Scan(&deliveriesExists)
	testutil.NoError(t, err)
	testutil.True(t, deliveriesExists, "_ayb_push_deliveries table should exist")

	const (
		ownerUserID = "60000000-0000-0000-0000-000000000001"
		targetUser  = "60000000-0000-0000-0000-000000000002"
		appID       = "60000000-0000-0000-0000-000000000003"
	)

	_, err = sharedPG.Pool.Exec(ctx,
		`INSERT INTO _ayb_users (id, email, password_hash) VALUES
		 ($1, 'push-owner@example.com', 'hash'),
		 ($2, 'push-user@example.com', 'hash')`,
		ownerUserID, targetUser,
	)
	testutil.NoError(t, err)

	_, err = sharedPG.Pool.Exec(ctx,
		`INSERT INTO _ayb_apps (id, name, owner_user_id) VALUES ($1, 'push-app', $2)`,
		appID, ownerUserID,
	)
	testutil.NoError(t, err)

	var tokenID string
	err = sharedPG.Pool.QueryRow(ctx,
		`INSERT INTO _ayb_device_tokens (app_id, user_id, provider, platform, token, device_name)
		 VALUES ($1, $2, 'fcm', 'android', 'token-abc', 'Pixel')
		 RETURNING id`,
		appID, targetUser,
	).Scan(&tokenID)
	testutil.NoError(t, err)

	_, err = sharedPG.Pool.Exec(ctx,
		`INSERT INTO _ayb_device_tokens (app_id, user_id, provider, platform, token)
		 VALUES ($1, $2, 'invalid', 'android', 'token-2')`,
		appID, targetUser,
	)
	testutil.True(t, err != nil, "invalid provider should be rejected")

	_, err = sharedPG.Pool.Exec(ctx,
		`INSERT INTO _ayb_device_tokens (app_id, user_id, provider, platform, token)
		 VALUES ($1, $2, 'fcm', 'invalid', 'token-3')`,
		appID, targetUser,
	)
	testutil.True(t, err != nil, "invalid platform should be rejected")

	_, err = sharedPG.Pool.Exec(ctx,
		`INSERT INTO _ayb_device_tokens (app_id, user_id, provider, platform, token)
		 VALUES ($1, $2, 'fcm', 'android', $3)`,
		appID, targetUser, strings.Repeat("a", 4097),
	)
	testutil.True(t, err != nil, "token longer than 4096 should be rejected")

	_, err = sharedPG.Pool.Exec(ctx,
		`INSERT INTO _ayb_device_tokens (app_id, user_id, provider, platform, token)
		 VALUES ($1, $2, 'fcm', 'android', 'token-abc')`,
		appID, targetUser,
	)
	testutil.True(t, err != nil, "duplicate app/provider/token should be rejected")

	var jobID string
	err = sharedPG.Pool.QueryRow(ctx,
		`INSERT INTO _ayb_jobs (type) VALUES ('push_delivery') RETURNING id`,
	).Scan(&jobID)
	testutil.NoError(t, err)

	_, err = sharedPG.Pool.Exec(ctx,
		`INSERT INTO _ayb_push_deliveries
		 (device_token_id, job_id, app_id, user_id, provider, title, body, data_payload, status)
		 VALUES ($1, $2, $3, $4, 'fcm', 'Title', 'Body', '{"k":"v"}', 'pending')`,
		tokenID, jobID, appID, targetUser,
	)
	testutil.NoError(t, err)

	_, err = sharedPG.Pool.Exec(ctx,
		`INSERT INTO _ayb_push_deliveries
		 (device_token_id, app_id, user_id, provider, title, body, status)
		 VALUES ($1, '60000000-0000-0000-0000-000000000009', $2, 'fcm', 'Title', 'Body', 'pending')`,
		tokenID, targetUser,
	)
	testutil.True(t, err != nil, "delivery app_id must match the referenced device token app_id")

	_, err = sharedPG.Pool.Exec(ctx,
		`INSERT INTO _ayb_push_deliveries
		 (device_token_id, app_id, user_id, provider, title, body, status)
		 VALUES ($1, $2, $3, 'fcm', 'Title', 'Body', 'pending')`,
		tokenID, appID, ownerUserID,
	)
	testutil.True(t, err != nil, "delivery user_id must match the referenced device token user_id")

	_, err = sharedPG.Pool.Exec(ctx,
		`INSERT INTO _ayb_push_deliveries
		 (device_token_id, app_id, user_id, provider, title, body, status)
		 VALUES ($1, $2, $3, 'apns', 'Title', 'Body', 'pending')`,
		tokenID, appID, targetUser,
	)
	testutil.True(t, err != nil, "delivery provider must match the referenced device token provider")

	_, err = sharedPG.Pool.Exec(ctx,
		`INSERT INTO _ayb_push_deliveries
		 (device_token_id, app_id, user_id, provider, title, body, status)
		 VALUES ($1, $2, $3, 'fcm', '', 'Body', 'pending')`,
		tokenID, appID, targetUser,
	)
	testutil.True(t, err != nil, "empty title should be rejected")

	_, err = sharedPG.Pool.Exec(ctx,
		`INSERT INTO _ayb_push_deliveries
		 (device_token_id, app_id, user_id, provider, title, body, status)
		 VALUES ($1, $2, $3, 'fcm', 'Title', '', 'pending')`,
		tokenID, appID, targetUser,
	)
	testutil.True(t, err != nil, "empty body should be rejected")

	_, err = sharedPG.Pool.Exec(ctx,
		`INSERT INTO _ayb_push_deliveries
		 (device_token_id, app_id, user_id, provider, title, body, status)
		 VALUES ($1, $2, $3, 'fcm', 'Title', 'Body', 'unknown')`,
		tokenID, appID, targetUser,
	)
	testutil.True(t, err != nil, "invalid status should be rejected")

	_, err = sharedPG.Pool.Exec(ctx,
		`INSERT INTO _ayb_push_deliveries
		 (device_token_id, app_id, user_id, provider, title, body, data_payload, status)
		 VALUES (
		   $1, $2, $3, 'fcm', 'Title', 'Body',
		   jsonb_build_object('blob', repeat('x', 9000)),
		   'pending'
		 )`,
		tokenID, appID, targetUser,
	)
	testutil.True(t, err != nil, "oversized data_payload should be rejected")

	_, err = sharedPG.Pool.Exec(ctx,
		`DELETE FROM _ayb_jobs WHERE id = $1`,
		jobID,
	)
	testutil.NoError(t, err)

	var deliveryJobID *string
	err = sharedPG.Pool.QueryRow(ctx,
		`SELECT job_id
		 FROM _ayb_push_deliveries
		 WHERE device_token_id = $1
		 ORDER BY created_at DESC
		 LIMIT 1`,
		tokenID,
	).Scan(&deliveryJobID)
	testutil.NoError(t, err)
	testutil.Nil(t, deliveryJobID)
}

func TestPushMigrationsCascadeAndRollback(t *testing.T) {
	ctx := context.Background()
	resetDB(t, ctx)

	runner := migrations.NewRunner(sharedPG.Pool, testutil.DiscardLogger())
	err := runner.Bootstrap(ctx)
	testutil.NoError(t, err)
	_, err = runner.Run(ctx)
	testutil.NoError(t, err)

	const (
		ownerUserID   = "70000000-0000-0000-0000-000000000001"
		targetUserID  = "70000000-0000-0000-0000-000000000002"
		appID         = "70000000-0000-0000-0000-000000000003"
		secondOwnerID = "70000000-0000-0000-0000-000000000004"
		secondUserID  = "70000000-0000-0000-0000-000000000005"
		secondAppID   = "70000000-0000-0000-0000-000000000006"
	)

	_, err = sharedPG.Pool.Exec(ctx,
		`INSERT INTO _ayb_users (id, email, password_hash) VALUES
		 ($1, 'push-cascade-owner@example.com', 'hash'),
		 ($2, 'push-cascade-user@example.com', 'hash'),
		 ($3, 'push-cascade-owner-2@example.com', 'hash'),
		 ($4, 'push-cascade-user-2@example.com', 'hash')`,
		ownerUserID, targetUserID, secondOwnerID, secondUserID,
	)
	testutil.NoError(t, err)

	_, err = sharedPG.Pool.Exec(ctx,
		`INSERT INTO _ayb_apps (id, name, owner_user_id) VALUES
		 ($1, 'push-cascade-app-1', $2),
		 ($3, 'push-cascade-app-2', $4)`,
		appID, ownerUserID, secondAppID, secondOwnerID,
	)
	testutil.NoError(t, err)

	var tokenByUser string
	err = sharedPG.Pool.QueryRow(ctx,
		`INSERT INTO _ayb_device_tokens (app_id, user_id, provider, platform, token)
		 VALUES ($1, $2, 'fcm', 'android', 'cascade-user-token')
		 RETURNING id`,
		appID, targetUserID,
	).Scan(&tokenByUser)
	testutil.NoError(t, err)

	_, err = sharedPG.Pool.Exec(ctx,
		`INSERT INTO _ayb_push_deliveries
		 (device_token_id, app_id, user_id, provider, title, body, status)
		 VALUES ($1, $2, $3, 'fcm', 'Title', 'Body', 'pending')`,
		tokenByUser, appID, targetUserID,
	)
	testutil.NoError(t, err)

	_, err = sharedPG.Pool.Exec(ctx,
		`DELETE FROM _ayb_users WHERE id = $1`,
		targetUserID,
	)
	testutil.NoError(t, err)

	var userCascadeCount int
	err = sharedPG.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM _ayb_push_deliveries WHERE device_token_id = $1`,
		tokenByUser,
	).Scan(&userCascadeCount)
	testutil.NoError(t, err)
	testutil.Equal(t, 0, userCascadeCount)

	var tokenByApp string
	err = sharedPG.Pool.QueryRow(ctx,
		`INSERT INTO _ayb_device_tokens (app_id, user_id, provider, platform, token)
		 VALUES ($1, $2, 'apns', 'ios', 'cascade-app-token')
		 RETURNING id`,
		secondAppID, secondUserID,
	).Scan(&tokenByApp)
	testutil.NoError(t, err)

	_, err = sharedPG.Pool.Exec(ctx,
		`INSERT INTO _ayb_push_deliveries
		 (device_token_id, app_id, user_id, provider, title, body, status)
		 VALUES ($1, $2, $3, 'apns', 'Title', 'Body', 'pending')`,
		tokenByApp, secondAppID, secondUserID,
	)
	testutil.NoError(t, err)

	_, err = sharedPG.Pool.Exec(ctx,
		`DELETE FROM _ayb_apps WHERE id = $1`,
		secondAppID,
	)
	testutil.NoError(t, err)

	var appCascadeCount int
	err = sharedPG.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM _ayb_push_deliveries WHERE device_token_id = $1`,
		tokenByApp,
	).Scan(&appCascadeCount)
	testutil.NoError(t, err)
	testutil.Equal(t, 0, appCascadeCount)

	tx, err := sharedPG.Pool.Begin(ctx)
	testutil.NoError(t, err)

	var rolledTokenID string
	err = tx.QueryRow(ctx,
		`INSERT INTO _ayb_device_tokens (app_id, user_id, provider, platform, token)
		 VALUES ($1, $2, 'fcm', 'android', 'rolled-back-token')
		 RETURNING id`,
		appID, ownerUserID,
	).Scan(&rolledTokenID)
	testutil.NoError(t, err)

	err = tx.Rollback(ctx)
	testutil.NoError(t, err)

	var rollbackCount int
	err = sharedPG.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM _ayb_device_tokens WHERE id = $1`,
		rolledTokenID,
	).Scan(&rollbackCount)
	testutil.NoError(t, err)
	testutil.Equal(t, 0, rollbackCount)
}
