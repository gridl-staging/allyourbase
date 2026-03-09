//go:build integration

package webhooks_test

import (
	"context"
	"os"
	"testing"

	"github.com/allyourbase/ayb/internal/migrations"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/allyourbase/ayb/internal/webhooks"
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

func resetAndMigrate(t *testing.T, ctx context.Context) {
	t.Helper()
	_, err := sharedPG.Pool.Exec(ctx, "DROP SCHEMA public CASCADE; CREATE SCHEMA public")
	if err != nil {
		t.Fatalf("resetting schema: %v", err)
	}
	logger := testutil.DiscardLogger()
	runner := migrations.NewRunner(sharedPG.Pool, logger)
	if err := runner.Bootstrap(ctx); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	if _, err := runner.Run(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
}

func TestStoreListEmpty(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	store := webhooks.NewStore(sharedPG.Pool)
	list, err := store.List(ctx)
	testutil.NoError(t, err)
	testutil.Equal(t, 0, len(list))
}

func TestStoreCreateAndGet(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	store := webhooks.NewStore(sharedPG.Pool)

	w := &webhooks.Webhook{
		URL:     "https://example.com/hook",
		Secret:  "secret123",
		Events:  []string{"create", "update"},
		Tables:  []string{"posts", "comments"},
		Enabled: true,
	}
	err := store.Create(ctx, w)
	testutil.NoError(t, err)
	testutil.True(t, w.ID != "", "ID should be populated after create")
	testutil.True(t, !w.CreatedAt.IsZero(), "CreatedAt should be populated")
	testutil.True(t, !w.UpdatedAt.IsZero(), "UpdatedAt should be populated")

	// Get the created webhook.
	got, err := store.Get(ctx, w.ID)
	testutil.NoError(t, err)
	testutil.Equal(t, "https://example.com/hook", got.URL)
	testutil.Equal(t, "secret123", got.Secret)
	testutil.Equal(t, 2, len(got.Events))
	testutil.Equal(t, "create", got.Events[0])
	testutil.Equal(t, "update", got.Events[1])
	testutil.Equal(t, 2, len(got.Tables))
	testutil.True(t, got.Enabled)
}

func TestStoreGetNotFound(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	store := webhooks.NewStore(sharedPG.Pool)
	_, err := store.Get(ctx, "00000000-0000-0000-0000-000000000000")
	testutil.ErrorContains(t, err, "no rows")
}

func TestStoreUpdate(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	store := webhooks.NewStore(sharedPG.Pool)

	w := &webhooks.Webhook{
		URL:     "https://example.com/hook",
		Secret:  "secret",
		Events:  []string{"create"},
		Tables:  []string{"posts"},
		Enabled: true,
	}
	testutil.NoError(t, store.Create(ctx, w))
	originalUpdatedAt := w.UpdatedAt

	// Update.
	updated := &webhooks.Webhook{
		URL:     "https://example.com/hook-v2",
		Secret:  "new-secret",
		Events:  []string{"create", "delete"},
		Tables:  []string{"posts", "users"},
		Enabled: false,
	}
	err := store.Update(ctx, w.ID, updated)
	testutil.NoError(t, err)
	testutil.Equal(t, w.ID, updated.ID)
	testutil.True(t, !updated.UpdatedAt.Before(originalUpdatedAt), "UpdatedAt should advance")

	// Verify via Get.
	got, err := store.Get(ctx, w.ID)
	testutil.NoError(t, err)
	testutil.Equal(t, "https://example.com/hook-v2", got.URL)
	testutil.Equal(t, "new-secret", got.Secret)
	testutil.Equal(t, 2, len(got.Events))
	testutil.False(t, got.Enabled)
}

func TestStoreUpdateNotFound(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	store := webhooks.NewStore(sharedPG.Pool)
	w := &webhooks.Webhook{
		URL:     "https://example.com/hook",
		Events:  []string{"create"},
		Tables:  []string{},
		Enabled: true,
	}
	err := store.Update(ctx, "00000000-0000-0000-0000-000000000000", w)
	testutil.ErrorContains(t, err, "no rows")
}

func TestStoreDelete(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	store := webhooks.NewStore(sharedPG.Pool)

	w := &webhooks.Webhook{
		URL:     "https://example.com/hook",
		Events:  []string{"create"},
		Tables:  []string{},
		Enabled: true,
	}
	testutil.NoError(t, store.Create(ctx, w))

	err := store.Delete(ctx, w.ID)
	testutil.NoError(t, err)

	// Should no longer exist.
	_, err = store.Get(ctx, w.ID)
	testutil.ErrorContains(t, err, "no rows")
}

func TestStoreDeleteNotFound(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	store := webhooks.NewStore(sharedPG.Pool)
	err := store.Delete(ctx, "00000000-0000-0000-0000-000000000000")
	testutil.ErrorContains(t, err, "no rows")
}

func TestStoreListMultiple(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	store := webhooks.NewStore(sharedPG.Pool)

	for i := 0; i < 3; i++ {
		w := &webhooks.Webhook{
			URL:     "https://example.com/hook",
			Events:  []string{"create"},
			Tables:  []string{},
			Enabled: i%2 == 0, // 0=true, 1=false, 2=true
		}
		testutil.NoError(t, store.Create(ctx, w))
	}

	list, err := store.List(ctx)
	testutil.NoError(t, err)
	testutil.Equal(t, 3, len(list))
}

func TestStoreListEnabled(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	store := webhooks.NewStore(sharedPG.Pool)

	// Create 3 webhooks: 2 enabled, 1 disabled.
	for _, enabled := range []bool{true, false, true} {
		w := &webhooks.Webhook{
			URL:     "https://example.com/hook",
			Events:  []string{"create"},
			Tables:  []string{},
			Enabled: enabled,
		}
		testutil.NoError(t, store.Create(ctx, w))
	}

	enabled, err := store.ListEnabled(ctx)
	testutil.NoError(t, err)
	testutil.Equal(t, 2, len(enabled))
	for _, w := range enabled {
		testutil.True(t, w.Enabled)
	}
}

func TestStoreCreateDefaults(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	store := webhooks.NewStore(sharedPG.Pool)

	// Create with minimal fields — DB defaults should kick in.
	w := &webhooks.Webhook{
		URL:     "https://example.com/minimal",
		Events:  []string{"create", "update", "delete"},
		Tables:  []string{},
		Enabled: true,
	}
	testutil.NoError(t, store.Create(ctx, w))

	got, err := store.Get(ctx, w.ID)
	testutil.NoError(t, err)
	testutil.Equal(t, "https://example.com/minimal", got.URL)
	testutil.Equal(t, "", got.Secret) // default empty
	testutil.True(t, got.Enabled)
}
