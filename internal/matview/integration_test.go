//go:build integration

package matview_test

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/allyourbase/ayb/internal/matview"
	"github.com/allyourbase/ayb/internal/migrations"
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

func resetDB(t *testing.T, ctx context.Context) {
	t.Helper()
	_, err := sharedPG.Pool.Exec(ctx, "DROP SCHEMA public CASCADE; CREATE SCHEMA public")
	if err != nil {
		t.Fatalf("resetting schema: %v", err)
	}
	runner := migrations.NewRunner(sharedPG.Pool, testutil.DiscardLogger())
	err = runner.Bootstrap(ctx)
	if err != nil {
		t.Fatalf("bootstrap migrations: %v", err)
	}
	_, err = runner.Run(ctx)
	if err != nil {
		t.Fatalf("run migrations: %v", err)
	}
}

// createTestMatview creates a real materialized view for testing.
func createTestMatview(t *testing.T, ctx context.Context, schema, name string) {
	t.Helper()
	// Create source table and matview
	_, err := sharedPG.Pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS `+schema+`.test_scores (id serial PRIMARY KEY, score int)`)
	if err != nil {
		t.Fatalf("creating source table: %v", err)
	}
	_, err = sharedPG.Pool.Exec(ctx, `INSERT INTO `+schema+`.test_scores (score) VALUES (10), (20), (30)`)
	if err != nil {
		t.Fatalf("seeding source table: %v", err)
	}
	_, err = sharedPG.Pool.Exec(ctx, `CREATE MATERIALIZED VIEW `+schema+`.`+name+` AS SELECT sum(score) AS total FROM `+schema+`.test_scores`)
	if err != nil {
		t.Fatalf("creating matview: %v", err)
	}
}

// createTestMatviewWithUniqueIndex creates a matview with a unique index (required for CONCURRENTLY).
func createTestMatviewWithUniqueIndex(t *testing.T, ctx context.Context, schema, name string) {
	t.Helper()
	_, err := sharedPG.Pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS `+schema+`.test_items (id serial PRIMARY KEY, name text)`)
	if err != nil {
		t.Fatalf("creating source table: %v", err)
	}
	_, err = sharedPG.Pool.Exec(ctx, `INSERT INTO `+schema+`.test_items (name) VALUES ('a'), ('b'), ('c')`)
	if err != nil {
		t.Fatalf("seeding source table: %v", err)
	}
	_, err = sharedPG.Pool.Exec(ctx, `CREATE MATERIALIZED VIEW `+schema+`.`+name+` AS SELECT id, name FROM `+schema+`.test_items`)
	if err != nil {
		t.Fatalf("creating matview: %v", err)
	}
	_, err = sharedPG.Pool.Exec(ctx, `CREATE UNIQUE INDEX ON `+schema+`.`+name+` (id)`)
	if err != nil {
		t.Fatalf("creating unique index: %v", err)
	}
}

// createUnpopulatedMatviewWithUniqueIndex creates a matview WITH NO DATA and a unique index.
func createUnpopulatedMatviewWithUniqueIndex(t *testing.T, ctx context.Context, schema, name string) {
	t.Helper()
	_, err := sharedPG.Pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS `+schema+`.test_unpopulated_items (id serial PRIMARY KEY, name text)`)
	if err != nil {
		t.Fatalf("creating source table: %v", err)
	}
	_, err = sharedPG.Pool.Exec(ctx, `INSERT INTO `+schema+`.test_unpopulated_items (name) VALUES ('x'), ('y')`)
	if err != nil {
		t.Fatalf("seeding source table: %v", err)
	}
	_, err = sharedPG.Pool.Exec(ctx, `CREATE MATERIALIZED VIEW `+schema+`.`+name+` AS SELECT id, name FROM `+schema+`.test_unpopulated_items WITH NO DATA`)
	if err != nil {
		t.Fatalf("creating unpopulated matview: %v", err)
	}
	_, err = sharedPG.Pool.Exec(ctx, `CREATE UNIQUE INDEX ON `+schema+`.`+name+` (id)`)
	if err != nil {
		t.Fatalf("creating unique index: %v", err)
	}
}

// --- Store CRUD Integration Tests ---

func TestStoreRegisterAndGet(t *testing.T) {
	ctx := context.Background()
	resetDB(t, ctx)
	createTestMatview(t, ctx, "public", "test_leaderboard")

	store := matview.NewStore(sharedPG.Pool)

	// Register
	reg, err := store.Register(ctx, "public", "test_leaderboard", matview.RefreshModeStandard)
	testutil.NoError(t, err)
	testutil.True(t, reg.ID != "", "registration ID should be set")
	testutil.Equal(t, "public", reg.SchemaName)
	testutil.Equal(t, "test_leaderboard", reg.ViewName)
	testutil.Equal(t, matview.RefreshModeStandard, reg.RefreshMode)
	testutil.Nil(t, reg.LastRefreshAt)
	testutil.Nil(t, reg.LastRefreshStatus)

	// Get by ID
	got, err := store.Get(ctx, reg.ID)
	testutil.NoError(t, err)
	testutil.Equal(t, reg.ID, got.ID)
	testutil.Equal(t, "test_leaderboard", got.ViewName)

	// Get by name
	got2, err := store.GetByName(ctx, "public", "test_leaderboard")
	testutil.NoError(t, err)
	testutil.Equal(t, reg.ID, got2.ID)
}

func TestStoreRegisterRejectsNonMatview(t *testing.T) {
	ctx := context.Background()
	resetDB(t, ctx)

	// Create a regular table, not a matview
	_, err := sharedPG.Pool.Exec(ctx, `CREATE TABLE public.not_a_matview (id int)`)
	testutil.NoError(t, err)

	store := matview.NewStore(sharedPG.Pool)
	_, err = store.Register(ctx, "public", "not_a_matview", matview.RefreshModeStandard)
	testutil.True(t, errors.Is(err, matview.ErrNotMaterializedView), "expected ErrNotMaterializedView, got: %v", err)
}

func TestStoreRegisterRejectsDuplicate(t *testing.T) {
	ctx := context.Background()
	resetDB(t, ctx)
	createTestMatview(t, ctx, "public", "dup_test_mv")

	store := matview.NewStore(sharedPG.Pool)
	_, err := store.Register(ctx, "public", "dup_test_mv", matview.RefreshModeStandard)
	testutil.NoError(t, err)

	_, err = store.Register(ctx, "public", "dup_test_mv", matview.RefreshModeStandard)
	testutil.True(t, errors.Is(err, matview.ErrDuplicateRegistration), "expected ErrDuplicateRegistration, got: %v", err)
}

func TestStoreUpdate(t *testing.T) {
	ctx := context.Background()
	resetDB(t, ctx)
	createTestMatview(t, ctx, "public", "update_test_mv")

	store := matview.NewStore(sharedPG.Pool)
	reg, err := store.Register(ctx, "public", "update_test_mv", matview.RefreshModeStandard)
	testutil.NoError(t, err)
	testutil.Equal(t, matview.RefreshModeStandard, reg.RefreshMode)

	updated, err := store.Update(ctx, reg.ID, matview.RefreshModeConcurrent)
	testutil.NoError(t, err)
	testutil.Equal(t, matview.RefreshModeConcurrent, updated.RefreshMode)
	testutil.Equal(t, reg.ID, updated.ID)
}

func TestStoreUpdateNotFound(t *testing.T) {
	ctx := context.Background()
	resetDB(t, ctx)

	store := matview.NewStore(sharedPG.Pool)
	_, err := store.Update(ctx, "00000000-0000-0000-0000-000000000000", matview.RefreshModeStandard)
	testutil.True(t, errors.Is(err, matview.ErrRegistrationNotFound), "expected ErrRegistrationNotFound, got: %v", err)
}

func TestStoreDelete(t *testing.T) {
	ctx := context.Background()
	resetDB(t, ctx)
	createTestMatview(t, ctx, "public", "delete_test_mv")

	store := matview.NewStore(sharedPG.Pool)
	reg, err := store.Register(ctx, "public", "delete_test_mv", matview.RefreshModeStandard)
	testutil.NoError(t, err)

	err = store.Delete(ctx, reg.ID)
	testutil.NoError(t, err)

	_, err = store.Get(ctx, reg.ID)
	testutil.True(t, errors.Is(err, matview.ErrRegistrationNotFound), "expected ErrRegistrationNotFound after delete")
}

func TestStoreDeleteNotFound(t *testing.T) {
	ctx := context.Background()
	resetDB(t, ctx)

	store := matview.NewStore(sharedPG.Pool)
	err := store.Delete(ctx, "00000000-0000-0000-0000-000000000000")
	testutil.True(t, errors.Is(err, matview.ErrRegistrationNotFound), "expected ErrRegistrationNotFound")
}

func TestStoreList(t *testing.T) {
	ctx := context.Background()
	resetDB(t, ctx)
	createTestMatview(t, ctx, "public", "list_mv_a")
	createTestMatview(t, ctx, "public", "list_mv_b")

	store := matview.NewStore(sharedPG.Pool)
	_, err := store.Register(ctx, "public", "list_mv_a", matview.RefreshModeStandard)
	testutil.NoError(t, err)
	_, err = store.Register(ctx, "public", "list_mv_b", matview.RefreshModeConcurrent)
	testutil.NoError(t, err)

	all, err := store.List(ctx)
	testutil.NoError(t, err)
	testutil.SliceLen(t, all, 2)
	// Should be sorted by schema, view_name
	testutil.Equal(t, "list_mv_a", all[0].ViewName)
	testutil.Equal(t, "list_mv_b", all[1].ViewName)
}

func TestStoreMatviewState(t *testing.T) {
	ctx := context.Background()
	resetDB(t, ctx)
	createTestMatview(t, ctx, "public", "state_test_mv")

	store := matview.NewStore(sharedPG.Pool)

	exists, populated, err := store.MatviewState(ctx, "public", "state_test_mv")
	testutil.NoError(t, err)
	testutil.True(t, exists, "matview should exist")
	testutil.True(t, populated, "matview should be populated after CREATE")

	exists2, _, err := store.MatviewState(ctx, "public", "nonexistent_view")
	testutil.NoError(t, err)
	testutil.False(t, exists2, "nonexistent view should not exist")
}

func TestLockedRefreshMutualExclusion(t *testing.T) {
	ctx := context.Background()
	resetDB(t, ctx)
	createTestMatview(t, ctx, "public", "lock_test_mv")

	store := matview.NewStore(sharedPG.Pool)

	// Hold an advisory lock on a separate connection, simulating a concurrent refresh.
	conn, err := sharedPG.Pool.Acquire(ctx)
	testutil.NoError(t, err)
	defer conn.Release()

	var locked bool
	err = conn.QueryRow(ctx, `SELECT pg_try_advisory_lock(hashtext('public.lock_test_mv'))`).Scan(&locked)
	testutil.NoError(t, err)
	testutil.True(t, locked, "should acquire lock on external connection")

	// LockedRefresh should fail because the lock is held externally.
	err = store.LockedRefresh(ctx, "public.lock_test_mv", `REFRESH MATERIALIZED VIEW "public"."lock_test_mv"`)
	testutil.True(t, errors.Is(err, matview.ErrRefreshInProgress), "expected ErrRefreshInProgress when lock held, got: %v", err)

	// Release the external lock.
	_, err = conn.Exec(ctx, `SELECT pg_advisory_unlock(hashtext('public.lock_test_mv'))`)
	testutil.NoError(t, err)

	// Now LockedRefresh should succeed.
	err = store.LockedRefresh(ctx, "public.lock_test_mv", `REFRESH MATERIALIZED VIEW "public"."lock_test_mv"`)
	testutil.NoError(t, err)
}

func TestStoreConcurrentUniqueIndex(t *testing.T) {
	ctx := context.Background()
	resetDB(t, ctx)

	// Matview without unique index
	createTestMatview(t, ctx, "public", "no_idx_mv")
	store := matview.NewStore(sharedPG.Pool)

	has, err := store.HasConcurrentUniqueIndex(ctx, "public", "no_idx_mv")
	testutil.NoError(t, err)
	testutil.False(t, has, "matview without unique index")

	// Matview with unique index
	createTestMatviewWithUniqueIndex(t, ctx, "public", "with_idx_mv")
	has2, err := store.HasConcurrentUniqueIndex(ctx, "public", "with_idx_mv")
	testutil.NoError(t, err)
	testutil.True(t, has2, "matview with unique index")
}

// --- Service.RefreshNow Integration Tests ---

func TestServiceRefreshNowStandard(t *testing.T) {
	ctx := context.Background()
	resetDB(t, ctx)
	createTestMatview(t, ctx, "public", "refresh_std_mv")

	store := matview.NewStore(sharedPG.Pool)
	reg, err := store.Register(ctx, "public", "refresh_std_mv", matview.RefreshModeStandard)
	testutil.NoError(t, err)

	svc := matview.NewService(store)
	result, err := svc.RefreshNow(ctx, reg.ID)
	testutil.NoError(t, err)
	testutil.True(t, result.DurationMs >= 0, "duration should be non-negative")

	// Verify metadata was updated
	updated, err := store.Get(ctx, reg.ID)
	testutil.NoError(t, err)
	testutil.NotNil(t, updated.LastRefreshAt)
	testutil.NotNil(t, updated.LastRefreshStatus)
	s := matview.RefreshStatusSuccess
	testutil.Equal(t, s, *updated.LastRefreshStatus)
	testutil.Nil(t, updated.LastRefreshError)

	// Verify data is actually refreshed: insert more data, refresh, check
	_, err = sharedPG.Pool.Exec(ctx, `INSERT INTO public.test_scores (score) VALUES (40)`)
	testutil.NoError(t, err)

	_, err = svc.RefreshNow(ctx, reg.ID)
	testutil.NoError(t, err)

	var total int
	err = sharedPG.Pool.QueryRow(ctx, `SELECT total FROM public.refresh_std_mv`).Scan(&total)
	testutil.NoError(t, err)
	testutil.Equal(t, 100, total) // 10+20+30+40
}

func TestServiceRefreshNowConcurrentWithIndex(t *testing.T) {
	ctx := context.Background()
	resetDB(t, ctx)
	createTestMatviewWithUniqueIndex(t, ctx, "public", "refresh_conc_mv")

	store := matview.NewStore(sharedPG.Pool)
	reg, err := store.Register(ctx, "public", "refresh_conc_mv", matview.RefreshModeConcurrent)
	testutil.NoError(t, err)

	svc := matview.NewService(store)
	result, err := svc.RefreshNow(ctx, reg.ID)
	testutil.NoError(t, err)
	testutil.True(t, result.DurationMs >= 0, "duration should be non-negative")

	updated, err := store.Get(ctx, reg.ID)
	testutil.NoError(t, err)
	testutil.NotNil(t, updated.LastRefreshStatus)
	s := matview.RefreshStatusSuccess
	testutil.Equal(t, s, *updated.LastRefreshStatus)
}

func TestServiceRefreshNowConcurrentWithoutIndex(t *testing.T) {
	ctx := context.Background()
	resetDB(t, ctx)
	createTestMatview(t, ctx, "public", "refresh_noindex_mv")

	store := matview.NewStore(sharedPG.Pool)
	reg, err := store.Register(ctx, "public", "refresh_noindex_mv", matview.RefreshModeConcurrent)
	testutil.NoError(t, err)

	svc := matview.NewService(store)
	_, err = svc.RefreshNow(ctx, reg.ID)
	testutil.True(t, errors.Is(err, matview.ErrConcurrentRefreshRequiresIndex),
		"expected ErrConcurrentRefreshRequiresIndex, got: %v", err)

	// Verify error was recorded in metadata
	updated, err := store.Get(ctx, reg.ID)
	testutil.NoError(t, err)
	testutil.NotNil(t, updated.LastRefreshStatus)
	s := matview.RefreshStatusError
	testutil.Equal(t, s, *updated.LastRefreshStatus)
	testutil.NotNil(t, updated.LastRefreshError)
}

func TestServiceRefreshNowConcurrentRequiresPopulated(t *testing.T) {
	ctx := context.Background()
	resetDB(t, ctx)
	createUnpopulatedMatviewWithUniqueIndex(t, ctx, "public", "refresh_nodata_mv")

	store := matview.NewStore(sharedPG.Pool)
	reg, err := store.Register(ctx, "public", "refresh_nodata_mv", matview.RefreshModeConcurrent)
	testutil.NoError(t, err)

	svc := matview.NewService(store)
	_, err = svc.RefreshNow(ctx, reg.ID)
	testutil.True(t, errors.Is(err, matview.ErrConcurrentRefreshRequiresPopulated),
		"expected ErrConcurrentRefreshRequiresPopulated, got: %v", err)

	updated, err := store.Get(ctx, reg.ID)
	testutil.NoError(t, err)
	testutil.NotNil(t, updated.LastRefreshStatus)
	s := matview.RefreshStatusError
	testutil.Equal(t, s, *updated.LastRefreshStatus)
	testutil.NotNil(t, updated.LastRefreshError)
}

func TestServiceRefreshNowMatviewDropped(t *testing.T) {
	ctx := context.Background()
	resetDB(t, ctx)
	createTestMatview(t, ctx, "public", "drop_test_mv")

	store := matview.NewStore(sharedPG.Pool)
	reg, err := store.Register(ctx, "public", "drop_test_mv", matview.RefreshModeStandard)
	testutil.NoError(t, err)

	// Drop the matview after registration
	_, err = sharedPG.Pool.Exec(ctx, `DROP MATERIALIZED VIEW public.drop_test_mv`)
	testutil.NoError(t, err)

	svc := matview.NewService(store)
	_, err = svc.RefreshNow(ctx, reg.ID)
	testutil.True(t, errors.Is(err, matview.ErrNotMaterializedView),
		"expected ErrNotMaterializedView, got: %v", err)
}

func TestServiceRefreshNowRegistrationNotFound(t *testing.T) {
	ctx := context.Background()
	resetDB(t, ctx)

	store := matview.NewStore(sharedPG.Pool)
	svc := matview.NewService(store)

	_, err := svc.RefreshNow(ctx, "00000000-0000-0000-0000-000000000000")
	testutil.True(t, errors.Is(err, matview.ErrRegistrationNotFound),
		"expected ErrRegistrationNotFound, got: %v", err)
}
