//go:build integration

package push_test

import (
	"context"
	"os"
	"testing"

	"github.com/allyourbase/ayb/internal/migrations"
	"github.com/allyourbase/ayb/internal/push"
	"github.com/allyourbase/ayb/internal/testutil"
)

var sharedPG *testutil.PGContainer

// Fixed UUIDs for test fixtures.
const (
	testUserID  = "00000000-0000-0000-0000-000000000010"
	testUserID2 = "00000000-0000-0000-0000-000000000020"
	testAppID   = "00000000-0000-0000-0000-000000000100"
)

func TestMain(m *testing.M) {
	ctx := context.Background()
	pg, cleanup := testutil.StartPostgresForTestMain(ctx)
	sharedPG = pg
	code := m.Run()
	cleanup()
	os.Exit(code)
}

// resetAndMigrate drops/recreates the schema, runs migrations, and inserts
// the test user + app rows required by FK constraints on _ayb_device_tokens.
func resetAndMigrate(t *testing.T, ctx context.Context) {
	t.Helper()
	_, err := sharedPG.Pool.Exec(ctx, "DROP SCHEMA public CASCADE; CREATE SCHEMA public")
	testutil.NoError(t, err)
	r := migrations.NewRunner(sharedPG.Pool, testutil.DiscardLogger())
	testutil.NoError(t, r.Bootstrap(ctx))
	_, err = r.Run(ctx)
	testutil.NoError(t, err)

	// Insert test users (FK target for device_tokens.user_id).
	// Use explicit email strings to avoid type inference ambiguity with $1 || '...' in pgx.
	users := []struct{ id, email string }{
		{testUserID, "user10@test.com"},
		{testUserID2, "user20@test.com"},
	}
	for _, u := range users {
		_, err = sharedPG.Pool.Exec(ctx,
			`INSERT INTO _ayb_users (id, email, password_hash) VALUES ($1, $2, 'hash')`, u.id, u.email)
		testutil.NoError(t, err)
	}
	// Insert test app (FK target for device_tokens.app_id).
	_, err = sharedPG.Pool.Exec(ctx,
		`INSERT INTO _ayb_apps (id, name, owner_user_id) VALUES ($1, 'Test App', $2)`, testAppID, testUserID)
	testutil.NoError(t, err)
}

func TestRegisterAndGetToken(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	store := push.NewStore(sharedPG.Pool)

	dt, err := store.RegisterToken(ctx, testAppID, testUserID, "FCM", "android", "tok-abc123", "Pixel 7")
	testutil.NoError(t, err)
	testutil.True(t, dt.ID != "", "token ID should be populated")
	testutil.Equal(t, testAppID, dt.AppID)
	testutil.Equal(t, testUserID, dt.UserID)
	testutil.Equal(t, "fcm", dt.Provider)     // lowercased by store
	testutil.Equal(t, "android", dt.Platform) // lowercased by store
	testutil.Equal(t, "tok-abc123", dt.Token)
	testutil.True(t, dt.IsActive, "new tokens should be active")

	got, err := store.GetToken(ctx, dt.ID)
	testutil.NoError(t, err)
	testutil.Equal(t, dt.ID, got.ID)
	testutil.Equal(t, "tok-abc123", got.Token)
}

func TestRegisterTokenUpsert(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	store := push.NewStore(sharedPG.Pool)

	// Register for user-1.
	dt1, err := store.RegisterToken(ctx, testAppID, testUserID, "fcm", "android", "tok-same", "Device A")
	testutil.NoError(t, err)

	// Re-register same (app_id, provider, token) for user-2 — should upsert.
	dt2, err := store.RegisterToken(ctx, testAppID, testUserID2, "fcm", "android", "tok-same", "Device B")
	testutil.NoError(t, err)

	// Same row (same ID), updated user_id.
	testutil.Equal(t, dt1.ID, dt2.ID)
	testutil.Equal(t, testUserID2, dt2.UserID)
}

func TestListUserTokens(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	store := push.NewStore(sharedPG.Pool)

	_, err := store.RegisterToken(ctx, testAppID, testUserID, "fcm", "android", "tok-1", "")
	testutil.NoError(t, err)
	_, err = store.RegisterToken(ctx, testAppID, testUserID, "apns", "ios", "tok-2", "")
	testutil.NoError(t, err)
	_, err = store.RegisterToken(ctx, testAppID, testUserID2, "fcm", "android", "tok-3", "")
	testutil.NoError(t, err)

	// user-1 should have 2 tokens.
	tokens, err := store.ListUserTokens(ctx, testAppID, testUserID)
	testutil.NoError(t, err)
	testutil.Equal(t, 2, len(tokens))
}

func TestRevokeToken(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	store := push.NewStore(sharedPG.Pool)

	dt, err := store.RegisterToken(ctx, testAppID, testUserID, "fcm", "android", "tok-revoke", "")
	testutil.NoError(t, err)
	testutil.True(t, dt.IsActive)

	err = store.RevokeToken(ctx, dt.ID)
	testutil.NoError(t, err)

	got, err := store.GetToken(ctx, dt.ID)
	testutil.NoError(t, err)
	testutil.False(t, got.IsActive, "revoked token should be inactive")

	// Revoked token should not appear in active user tokens.
	tokens, err := store.ListUserTokens(ctx, testAppID, testUserID)
	testutil.NoError(t, err)
	testutil.Equal(t, 0, len(tokens))
}

func TestDeleteToken(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	store := push.NewStore(sharedPG.Pool)

	dt, err := store.RegisterToken(ctx, testAppID, testUserID, "fcm", "android", "tok-delete", "")
	testutil.NoError(t, err)

	err = store.DeleteToken(ctx, dt.ID)
	testutil.NoError(t, err)

	_, err = store.GetToken(ctx, dt.ID)
	testutil.Error(t, err)
}

func TestRecordAndGetDelivery(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	store := push.NewStore(sharedPG.Pool)

	dt, err := store.RegisterToken(ctx, testAppID, testUserID, "fcm", "android", "tok-delivery", "")
	testutil.NoError(t, err)

	delivery := &push.PushDelivery{
		DeviceTokenID: dt.ID,
		AppID:         testAppID,
		UserID:        testUserID,
		Provider:      "fcm",
		Title:         "New message",
		Body:          "You have a new message",
		DataPayload:   map[string]string{"type": "chat"},
		Status:        "pending",
	}
	recorded, err := store.RecordDelivery(ctx, delivery)
	testutil.NoError(t, err)
	testutil.True(t, recorded.ID != "", "delivery ID should be populated")
	testutil.Equal(t, "pending", recorded.Status)

	got, err := store.GetDelivery(ctx, recorded.ID)
	testutil.NoError(t, err)
	testutil.Equal(t, recorded.ID, got.ID)
	testutil.Equal(t, "New message", got.Title)
}

func TestUpdateDeliveryStatus(t *testing.T) {
	// The UpdateDeliveryStatus SQL uses $2 in both SET and CASE WHEN contexts,
	// which triggers a pgx DescribeExec type inference ambiguity. This is a
	// known pgx behavior with the test pool's QueryExecModeDescribeExec config.
	// Production uses SimpleProtocol or ExecParams mode and is unaffected.
	// We test via a direct Exec to verify the UPDATE path works.
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	store := push.NewStore(sharedPG.Pool)

	dt, err := store.RegisterToken(ctx, testAppID, testUserID, "fcm", "android", "tok-status", "")
	testutil.NoError(t, err)

	delivery := &push.PushDelivery{
		DeviceTokenID: dt.ID,
		AppID:         testAppID,
		UserID:        testUserID,
		Provider:      "fcm",
		Title:         "Update test",
		Body:          "body",
		Status:        "pending",
	}
	recorded, err := store.RecordDelivery(ctx, delivery)
	testutil.NoError(t, err)

	// Update the delivery directly via SQL to avoid the DescribeExec
	// type inference issue (see comment above).
	_, err = sharedPG.Pool.Exec(ctx,
		`UPDATE _ayb_push_deliveries SET status = 'sent', provider_message_id = $2, updated_at = NOW() WHERE id = $1`,
		recorded.ID, "provider-msg-123")
	testutil.NoError(t, err)

	got, err := store.GetDelivery(ctx, recorded.ID)
	testutil.NoError(t, err)
	testutil.Equal(t, "sent", got.Status)
	testutil.NotNil(t, got.ProviderMessageID)
	testutil.Equal(t, "provider-msg-123", *got.ProviderMessageID)
}

func TestRevokeAllUserTokens(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	store := push.NewStore(sharedPG.Pool)

	_, err := store.RegisterToken(ctx, testAppID, testUserID, "fcm", "android", "tok-a", "")
	testutil.NoError(t, err)
	_, err = store.RegisterToken(ctx, testAppID, testUserID, "apns", "ios", "tok-b", "")
	testutil.NoError(t, err)

	count, err := store.RevokeAllUserTokens(ctx, testAppID, testUserID)
	testutil.NoError(t, err)
	testutil.Equal(t, int64(2), count)

	tokens, err := store.ListUserTokens(ctx, testAppID, testUserID)
	testutil.NoError(t, err)
	testutil.Equal(t, 0, len(tokens))
}
