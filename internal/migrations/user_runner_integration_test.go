//go:build integration

package migrations_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/allyourbase/ayb/internal/migrations"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestUserRunnerBootstrap(t *testing.T) {
	ctx := context.Background()
	resetDB(t, ctx)

	dir := t.TempDir()
	runner := migrations.NewUserRunner(sharedPG.Pool, dir, testutil.DiscardLogger())

	err := runner.Bootstrap(ctx)
	testutil.NoError(t, err)

	// Verify table exists.
	var exists bool
	err = sharedPG.Pool.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_name = '_ayb_user_migrations')").
		Scan(&exists)
	testutil.NoError(t, err)
	testutil.True(t, exists, "_ayb_user_migrations table should exist")
}

func TestUserRunnerBootstrapIdempotent(t *testing.T) {
	ctx := context.Background()
	resetDB(t, ctx)

	dir := t.TempDir()
	runner := migrations.NewUserRunner(sharedPG.Pool, dir, testutil.DiscardLogger())

	err := runner.Bootstrap(ctx)
	testutil.NoError(t, err)
	err = runner.Bootstrap(ctx)
	testutil.NoError(t, err)
}

func TestUserRunnerUp(t *testing.T) {
	ctx := context.Background()
	resetDB(t, ctx)

	dir := t.TempDir()
	runner := migrations.NewUserRunner(sharedPG.Pool, dir, testutil.DiscardLogger())
	testutil.NoError(t, runner.Bootstrap(ctx))

	// Create two migration files.
	os.WriteFile(filepath.Join(dir, "20260201_create_posts.sql"), []byte(`
		CREATE TABLE posts (
			id SERIAL PRIMARY KEY,
			title TEXT NOT NULL,
			body TEXT
		)
	`), 0o644)
	os.WriteFile(filepath.Join(dir, "20260202_create_comments.sql"), []byte(`
		CREATE TABLE comments (
			id SERIAL PRIMARY KEY,
			post_id INT REFERENCES posts(id),
			body TEXT NOT NULL
		)
	`), 0o644)

	applied, err := runner.Up(ctx)
	testutil.NoError(t, err)
	testutil.Equal(t, 2, applied)

	// Verify tables exist.
	var exists bool
	err = sharedPG.Pool.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_name = 'posts')").
		Scan(&exists)
	testutil.NoError(t, err)
	testutil.True(t, exists, "posts table should exist")

	err = sharedPG.Pool.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_name = 'comments')").
		Scan(&exists)
	testutil.NoError(t, err)
	testutil.True(t, exists, "comments table should exist")
}

func TestUserRunnerUpIdempotent(t *testing.T) {
	ctx := context.Background()
	resetDB(t, ctx)

	dir := t.TempDir()
	runner := migrations.NewUserRunner(sharedPG.Pool, dir, testutil.DiscardLogger())
	testutil.NoError(t, runner.Bootstrap(ctx))

	os.WriteFile(filepath.Join(dir, "20260201_init.sql"), []byte(`
		CREATE TABLE items (id SERIAL PRIMARY KEY)
	`), 0o644)

	applied1, err := runner.Up(ctx)
	testutil.NoError(t, err)
	testutil.Equal(t, 1, applied1)

	// Second run should apply zero.
	applied2, err := runner.Up(ctx)
	testutil.NoError(t, err)
	testutil.Equal(t, 0, applied2)
}

func TestUserRunnerUpEmptyDir(t *testing.T) {
	ctx := context.Background()
	resetDB(t, ctx)

	dir := t.TempDir()
	runner := migrations.NewUserRunner(sharedPG.Pool, dir, testutil.DiscardLogger())
	testutil.NoError(t, runner.Bootstrap(ctx))

	applied, err := runner.Up(ctx)
	testutil.NoError(t, err)
	testutil.Equal(t, 0, applied)
}

func TestUserRunnerUpRollsBackOnError(t *testing.T) {
	ctx := context.Background()
	resetDB(t, ctx)

	dir := t.TempDir()
	runner := migrations.NewUserRunner(sharedPG.Pool, dir, testutil.DiscardLogger())
	testutil.NoError(t, runner.Bootstrap(ctx))

	os.WriteFile(filepath.Join(dir, "20260201_good.sql"), []byte(`
		CREATE TABLE good_table (id SERIAL PRIMARY KEY)
	`), 0o644)
	os.WriteFile(filepath.Join(dir, "20260202_bad.sql"), []byte(`
		CREATE TABLE bad_table (id SERIAL PRIMARY KEY);
		INVALID SQL HERE;
	`), 0o644)

	applied, err := runner.Up(ctx)
	testutil.Equal(t, applied, 1) // First migration succeeded.
	testutil.NotNil(t, err)

	// Good table should exist, bad table should not (rolled back).
	var exists bool
	err = sharedPG.Pool.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_name = 'good_table')").
		Scan(&exists)
	testutil.NoError(t, err)
	testutil.True(t, exists, "good_table should exist")

	err = sharedPG.Pool.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_name = 'bad_table')").
		Scan(&exists)
	testutil.NoError(t, err)
	testutil.False(t, exists, "bad_table should not exist (rolled back)")
}

func TestUserRunnerStatus(t *testing.T) {
	ctx := context.Background()
	resetDB(t, ctx)

	dir := t.TempDir()
	runner := migrations.NewUserRunner(sharedPG.Pool, dir, testutil.DiscardLogger())
	testutil.NoError(t, runner.Bootstrap(ctx))

	// Create 3 migration files, apply 2.
	os.WriteFile(filepath.Join(dir, "20260201_a.sql"), []byte("CREATE TABLE a (id INT)"), 0o644)
	os.WriteFile(filepath.Join(dir, "20260202_b.sql"), []byte("CREATE TABLE b (id INT)"), 0o644)

	_, err := runner.Up(ctx)
	testutil.NoError(t, err)

	// Add a third file (not yet applied).
	os.WriteFile(filepath.Join(dir, "20260203_c.sql"), []byte("CREATE TABLE c (id INT)"), 0o644)

	status, err := runner.Status(ctx)
	testutil.NoError(t, err)
	testutil.SliceLen(t, status, 3)

	// First two should be applied.
	testutil.Equal(t, "20260201_a.sql", status[0].Name)
	testutil.NotNil(t, status[0].AppliedAt)

	testutil.Equal(t, "20260202_b.sql", status[1].Name)
	testutil.NotNil(t, status[1].AppliedAt)

	// Third should be pending.
	testutil.Equal(t, "20260203_c.sql", status[2].Name)
	testutil.Nil(t, status[2].AppliedAt)
}

func postGISPoolForMigrations(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("AYB_TEST_POSTGIS_URL")
	if url == "" {
		t.Skip("AYB_TEST_POSTGIS_URL not set — skipping PostGIS migration test")
	}
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatalf("connecting to PostGIS database: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func resetPostGISDBForMigrations(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	_, err := pool.Exec(ctx, "DROP SCHEMA public CASCADE; CREATE SCHEMA public")
	if err != nil {
		t.Fatalf("resetting PostGIS schema: %v", err)
	}
	_, err = pool.Exec(ctx, "CREATE EXTENSION IF NOT EXISTS postgis")
	if err != nil {
		t.Fatalf("creating PostGIS extension: %v", err)
	}
}

func TestUserRunnerPostGISSpatialDDLAndReload(t *testing.T) {
	ctx := context.Background()
	pool := postGISPoolForMigrations(t)
	resetPostGISDBForMigrations(t, ctx, pool)

	dir := t.TempDir()
	runner := migrations.NewUserRunner(pool, dir, testutil.DiscardLogger())
	testutil.NoError(t, runner.Bootstrap(ctx))

	err := os.WriteFile(filepath.Join(dir, "20260223_create_spatial_places.sql"), []byte(`
		CREATE TABLE spatial_places (
			id SERIAL PRIMARY KEY,
			name TEXT NOT NULL,
			location geometry(Point, 4326)
		);
		CREATE INDEX idx_spatial_places_location ON spatial_places USING GIST (location);
	`), 0o644)
	testutil.NoError(t, err)

	cacheHolder := schema.NewCacheHolder(pool, testutil.DiscardLogger())
	testutil.NoError(t, cacheHolder.Load(ctx))
	testutil.Nil(t, cacheHolder.Get().Tables["public.spatial_places"])

	applied, err := runner.Up(ctx)
	testutil.NoError(t, err)
	testutil.Equal(t, 1, applied)

	testutil.NoError(t, cacheHolder.Reload(ctx))
	cacheAfterCreate := cacheHolder.Get()
	testutil.True(t, cacheAfterCreate.HasPostGIS, "cache should detect PostGIS extension")

	places := cacheAfterCreate.Tables["public.spatial_places"]
	testutil.NotNil(t, places)
	testutil.True(t, places.HasGeometry(), "spatial_places should have geometry after migration")

	locationCol := places.ColumnByName("location")
	testutil.NotNil(t, locationCol)
	testutil.True(t, locationCol.IsGeometry, "location should be geometry")
	testutil.Equal(t, "Point", locationCol.GeometryType)
	testutil.Equal(t, 4326, locationCol.SRID)
	testutil.Equal(t, "object", locationCol.JSONType)

	var gistIdx *schema.Index
	for _, idx := range places.Indexes {
		if idx.Name == "idx_spatial_places_location" {
			gistIdx = idx
			break
		}
	}
	testutil.NotNil(t, gistIdx)
	testutil.Equal(t, "gist", gistIdx.Method)

	err = os.WriteFile(filepath.Join(dir, "20260224_drop_spatial_location.sql"), []byte(`
		ALTER TABLE spatial_places DROP COLUMN location;
	`), 0o644)
	testutil.NoError(t, err)

	applied, err = runner.Up(ctx)
	testutil.NoError(t, err)
	testutil.Equal(t, 1, applied)

	testutil.NoError(t, cacheHolder.Reload(ctx))
	cacheAfterDrop := cacheHolder.Get()
	placesAfterDrop := cacheAfterDrop.Tables["public.spatial_places"]
	testutil.NotNil(t, placesAfterDrop)
	testutil.False(t, placesAfterDrop.HasGeometry(), "spatial_places should not have geometry after drop migration")
	testutil.Nil(t, placesAfterDrop.ColumnByName("location"))
}

func TestUserRunnerWithSchemaAppliesMigrationsInTenantSchema(t *testing.T) {
	ctx := context.Background()
	resetDB(t, ctx)

	schemaName := "runner_schema_test"
	_, err := sharedPG.Pool.Exec(ctx, `CREATE SCHEMA "runner_schema_test"`)
	testutil.NoError(t, err)

	dir := t.TempDir()
	runner := migrations.NewUserRunnerWithSchema(sharedPG.Pool, dir, schemaName, testutil.DiscardLogger())
	testutil.NoError(t, runner.Bootstrap(ctx))

	err = os.WriteFile(filepath.Join(dir, "20260304_create_schema_items.sql"), []byte(`
		CREATE TABLE tenant_items (
			id SERIAL PRIMARY KEY,
			name TEXT NOT NULL
		)
	`), 0o644)
	testutil.NoError(t, err)

	applied, err := runner.Up(ctx)
	testutil.NoError(t, err)
	testutil.Equal(t, 1, applied)

	var tableInSchema bool
	err = sharedPG.Pool.QueryRow(ctx,
		`SELECT EXISTS(
			SELECT 1 FROM information_schema.tables
			WHERE table_schema = $1 AND table_name = 'tenant_items'
		)`,
		schemaName,
	).Scan(&tableInSchema)
	testutil.NoError(t, err)
	testutil.True(t, tableInSchema, "tenant_items should exist in tenant schema")

	var trackingInSchema bool
	err = sharedPG.Pool.QueryRow(ctx,
		`SELECT EXISTS(
			SELECT 1 FROM information_schema.tables
			WHERE table_schema = $1 AND table_name = '_ayb_user_migrations'
		)`,
		schemaName,
	).Scan(&trackingInSchema)
	testutil.NoError(t, err)
	testutil.True(t, trackingInSchema, "_ayb_user_migrations should exist in tenant schema")
}
