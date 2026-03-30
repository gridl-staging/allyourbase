//go:build integration

package notifications_test

import (
	"context"
	"os"
	"testing"

	"github.com/allyourbase/ayb/internal/migrations"
	"github.com/allyourbase/ayb/internal/notifications"
	"github.com/allyourbase/ayb/internal/testutil"
)

var sharedPG *testutil.PGContainer

func TestMain(m *testing.M) {
	ctx := context.Background()
	pg, cleanup := testutil.StartPostgresForTestMain(ctx)
	sharedPG = pg
	code := m.Run()
	cleanup()
	os.Exit(code)
}

// resetAndMigrate drops and recreates the public schema, then runs all
// migrations so that the _ayb_notifications table exists for store tests.
func resetAndMigrate(t *testing.T, ctx context.Context) {
	t.Helper()
	_, err := sharedPG.Pool.Exec(ctx, "DROP SCHEMA public CASCADE; CREATE SCHEMA public")
	testutil.NoError(t, err)
	r := migrations.NewRunner(sharedPG.Pool, testutil.DiscardLogger())
	testutil.NoError(t, r.Bootstrap(ctx))
	_, err = r.Run(ctx)
	testutil.NoError(t, err)
}

func TestStoreCreateAndGetByID(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	store := notifications.NewStore(sharedPG.Pool)

	// Create a notification with metadata.
	meta := map[string]any{"key": "value", "count": float64(42)}
	n, err := store.Create(ctx, "00000000-0000-0000-0000-000000000001", "Welcome", "Hello world", meta, "general")
	testutil.NoError(t, err)
	testutil.True(t, n.ID != "", "ID should be populated after create")
	testutil.Equal(t, "00000000-0000-0000-0000-000000000001", n.UserID)
	testutil.Equal(t, "Welcome", n.Title)
	testutil.Equal(t, "Hello world", n.Body)
	testutil.Equal(t, "general", n.Channel)
	testutil.Nil(t, n.ReadAt) // new notifications are unread
	testutil.True(t, !n.CreatedAt.IsZero(), "CreatedAt should be populated")

	// Verify metadata was stored and retrieved correctly.
	testutil.Equal(t, "value", n.Metadata["key"].(string))
	testutil.Equal(t, float64(42), n.Metadata["count"].(float64))

	// GetByID should return the same notification for the correct user.
	got, err := store.GetByID(ctx, n.ID, "00000000-0000-0000-0000-000000000001")
	testutil.NoError(t, err)
	testutil.Equal(t, n.ID, got.ID)
	testutil.Equal(t, "Welcome", got.Title)

	// GetByID with wrong user should return an error (visibility isolation).
	_, err = store.GetByID(ctx, n.ID, "00000000-0000-0000-0000-000000000999")
	testutil.Error(t, err)
}

func TestStoreCreateDefaultChannel(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	store := notifications.NewStore(sharedPG.Pool)

	// Empty channel should default to "general".
	n, err := store.Create(ctx, "00000000-0000-0000-0000-000000000001", "Test", "body", nil, "")
	testutil.NoError(t, err)
	testutil.Equal(t, "general", n.Channel)
}

func TestStoreListByUserPagination(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	store := notifications.NewStore(sharedPG.Pool)

	// Create 5 notifications for user-1.
	for i := 0; i < 5; i++ {
		_, err := store.Create(ctx, "00000000-0000-0000-0000-000000000001", "Notification", "body", nil, "general")
		testutil.NoError(t, err)
	}
	// Create 2 for user-2 (should not appear in user-1's list).
	for i := 0; i < 2; i++ {
		_, err := store.Create(ctx, "00000000-0000-0000-0000-000000000002", "Other", "body", nil, "general")
		testutil.NoError(t, err)
	}

	// Page 1 of 3 should return 3 items, total 5.
	items, total, err := store.ListByUser(ctx, "00000000-0000-0000-0000-000000000001", false, 1, 3)
	testutil.NoError(t, err)
	testutil.Equal(t, 5, total)
	testutil.Equal(t, 3, len(items))

	// Page 2 should return the remaining 2.
	items, total, err = store.ListByUser(ctx, "00000000-0000-0000-0000-000000000001", false, 2, 3)
	testutil.NoError(t, err)
	testutil.Equal(t, 5, total)
	testutil.Equal(t, 2, len(items))
}

func TestStoreMarkRead(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	store := notifications.NewStore(sharedPG.Pool)

	n, err := store.Create(ctx, "00000000-0000-0000-0000-000000000001", "Read Me", "body", nil, "general")
	testutil.NoError(t, err)
	testutil.Nil(t, n.ReadAt)

	// Mark as read.
	err = store.MarkRead(ctx, n.ID, "00000000-0000-0000-0000-000000000001")
	testutil.NoError(t, err)

	// Verify it's now marked as read.
	got, err := store.GetByID(ctx, n.ID, "00000000-0000-0000-0000-000000000001")
	testutil.NoError(t, err)
	testutil.NotNil(t, got.ReadAt)

	// Marking read for wrong user should fail (store returns not-found
	// because WHERE includes user_id for visibility isolation).
	err = store.MarkRead(ctx, n.ID, "00000000-0000-0000-0000-000000000999")
	testutil.Error(t, err)
}

func TestStoreMarkAllRead(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	store := notifications.NewStore(sharedPG.Pool)

	// Create 3 unread notifications.
	for i := 0; i < 3; i++ {
		_, err := store.Create(ctx, "00000000-0000-0000-0000-000000000001", "Unread", "body", nil, "general")
		testutil.NoError(t, err)
	}
	// Also create one for user-2 that should not be affected.
	_, err := store.Create(ctx, "00000000-0000-0000-0000-000000000002", "Other", "body", nil, "general")
	testutil.NoError(t, err)

	// Mark all read for user-1.
	count, err := store.MarkAllRead(ctx, "00000000-0000-0000-0000-000000000001")
	testutil.NoError(t, err)
	testutil.Equal(t, int64(3), count)

	// Verify only unread notifications are listed with unreadOnly=true.
	items, total, err := store.ListByUser(ctx, "00000000-0000-0000-0000-000000000001", true, 1, 100)
	testutil.NoError(t, err)
	testutil.Equal(t, 0, total)
	testutil.Equal(t, 0, len(items))

	// User-2's notification should still be unread.
	items2, total2, err := store.ListByUser(ctx, "00000000-0000-0000-0000-000000000002", true, 1, 100)
	testutil.NoError(t, err)
	testutil.Equal(t, 1, total2)
	testutil.Equal(t, 1, len(items2))
}

func TestStoreListUnreadOnly(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	store := notifications.NewStore(sharedPG.Pool)

	// Create 2 notifications, mark one as read.
	n1, err := store.Create(ctx, "00000000-0000-0000-0000-000000000001", "First", "body", nil, "general")
	testutil.NoError(t, err)
	_, err = store.Create(ctx, "00000000-0000-0000-0000-000000000001", "Second", "body", nil, "general")
	testutil.NoError(t, err)

	err = store.MarkRead(ctx, n1.ID, "00000000-0000-0000-0000-000000000001")
	testutil.NoError(t, err)

	// unreadOnly should return only the second notification.
	items, total, err := store.ListByUser(ctx, "00000000-0000-0000-0000-000000000001", true, 1, 100)
	testutil.NoError(t, err)
	testutil.Equal(t, 1, total)
	testutil.Equal(t, 1, len(items))
	testutil.Equal(t, "Second", items[0].Title)
}
