//go:build integration

package directusmigrate_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/allyourbase/ayb/internal/directusmigrate"
	"github.com/allyourbase/ayb/internal/testutil"
	_ "github.com/jackc/pgx/v5/stdlib"
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

// resetSchema drops and recreates the public schema so each test starts clean.
// It also ensures roles referenced by Directus RLS policies exist.
func resetSchema(t *testing.T, ctx context.Context) {
	t.Helper()
	_, err := sharedPG.Pool.Exec(ctx, "DROP SCHEMA public CASCADE; CREATE SCHEMA public")
	testutil.NoError(t, err)
	// Create roles that the fixture's permissions reference. Ignore "already exists" errors.
	for _, role := range []string{"editor"} {
		_, _ = sharedPG.Pool.Exec(ctx, "CREATE ROLE "+role+" NOLOGIN")
	}
}

func TestMigrateBasicSnapshot(t *testing.T) {
	ctx := context.Background()
	resetSchema(t, ctx)

	fixturePath, err := filepath.Abs("testdata/snapshot.json")
	testutil.NoError(t, err)

	migrator, err := directusmigrate.NewMigrator(directusmigrate.MigrationOptions{
		SnapshotPath: fixturePath,
		DatabaseURL:  sharedPG.ConnString,
	})
	testutil.NoError(t, err)
	defer migrator.Close()

	stats, err := migrator.Migrate(ctx)
	testutil.NoError(t, err)

	// The fixture has 2 user collections (authors, articles) plus directus_users (skipped).
	testutil.True(t, stats.Collections > 0, "should have processed collections")
	testutil.True(t, stats.Fields > 0, "should have created fields")
	testutil.Equal(t, 0, len(stats.Errors))

	// Verify tables exist in the database.
	db, err := sql.Open("pgx", sharedPG.ConnString)
	testutil.NoError(t, err)
	defer db.Close()

	var tableCount int
	err = db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = 'public' AND table_type = 'BASE TABLE'").Scan(&tableCount)
	testutil.NoError(t, err)
	testutil.True(t, tableCount >= 2, "should have at least 2 tables (authors, articles)")
}

func TestMigrateWithRelations(t *testing.T) {
	ctx := context.Background()
	resetSchema(t, ctx)

	fixturePath, err := filepath.Abs("testdata/snapshot.json")
	testutil.NoError(t, err)

	migrator, err := directusmigrate.NewMigrator(directusmigrate.MigrationOptions{
		SnapshotPath: fixturePath,
		DatabaseURL:  sharedPG.ConnString,
	})
	testutil.NoError(t, err)
	defer migrator.Close()

	stats, err := migrator.Migrate(ctx)
	testutil.NoError(t, err)

	// The fixture has a relation from articles.author_id -> authors.id.
	testutil.True(t, stats.Relations > 0, "should have created foreign key relations")

	// Verify the FK constraint exists.
	db, err := sql.Open("pgx", sharedPG.ConnString)
	testutil.NoError(t, err)
	defer db.Close()

	var fkCount int
	err = db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM information_schema.table_constraints
		 WHERE constraint_type = 'FOREIGN KEY' AND table_schema = 'public'`).Scan(&fkCount)
	testutil.NoError(t, err)
	testutil.True(t, fkCount > 0, "should have at least one FK constraint")
}

func TestMigrateWithRLS(t *testing.T) {
	ctx := context.Background()
	resetSchema(t, ctx)

	fixturePath, err := filepath.Abs("testdata/snapshot.json")
	testutil.NoError(t, err)

	migrator, err := directusmigrate.NewMigrator(directusmigrate.MigrationOptions{
		SnapshotPath: fixturePath,
		DatabaseURL:  sharedPG.ConnString,
	})
	testutil.NoError(t, err)
	defer migrator.Close()

	stats, err := migrator.Migrate(ctx)
	testutil.NoError(t, err)

	// The fixture has permissions for public and editor roles.
	testutil.True(t, stats.Policies > 0, "should have created RLS policies")

	// Verify RLS is enabled on at least one table.
	db, err := sql.Open("pgx", sharedPG.ConnString)
	testutil.NoError(t, err)
	defer db.Close()

	var rlsCount int
	err = db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM pg_class WHERE relrowsecurity = true AND relnamespace = 'public'::regnamespace").Scan(&rlsCount)
	testutil.NoError(t, err)
	testutil.True(t, rlsCount > 0, "at least one table should have RLS enabled")
}

func TestMigrateSkipRLS(t *testing.T) {
	ctx := context.Background()
	resetSchema(t, ctx)

	fixturePath, err := filepath.Abs("testdata/snapshot.json")
	testutil.NoError(t, err)

	migrator, err := directusmigrate.NewMigrator(directusmigrate.MigrationOptions{
		SnapshotPath: fixturePath,
		DatabaseURL:  sharedPG.ConnString,
		SkipRLS:      true,
	})
	testutil.NoError(t, err)
	defer migrator.Close()

	stats, err := migrator.Migrate(ctx)
	testutil.NoError(t, err)

	// With SkipRLS, no policies should be created.
	testutil.Equal(t, 0, stats.Policies)
}

func TestMigrateDryRun(t *testing.T) {
	ctx := context.Background()
	resetSchema(t, ctx)

	fixturePath, err := filepath.Abs("testdata/snapshot.json")
	testutil.NoError(t, err)

	migrator, err := directusmigrate.NewMigrator(directusmigrate.MigrationOptions{
		SnapshotPath: fixturePath,
		DatabaseURL:  sharedPG.ConnString,
		DryRun:       true,
	})
	testutil.NoError(t, err)
	defer migrator.Close()

	stats, err := migrator.Migrate(ctx)
	testutil.NoError(t, err)

	// DryRun should still report stats but roll back the transaction.
	testutil.True(t, stats.Collections > 0, "dry run should report collections")

	// Verify tables were NOT actually created (rolled back).
	db, err := sql.Open("pgx", sharedPG.ConnString)
	testutil.NoError(t, err)
	defer db.Close()

	var tableCount int
	err = db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = 'public' AND table_type = 'BASE TABLE'").Scan(&tableCount)
	testutil.NoError(t, err)
	testutil.Equal(t, 0, tableCount)
}

func TestNewMigratorInvalidSnapshotPath(t *testing.T) {
	_, err := directusmigrate.NewMigrator(directusmigrate.MigrationOptions{
		SnapshotPath: "/nonexistent/path.json",
		DatabaseURL:  sharedPG.ConnString,
	})
	testutil.Error(t, err)
	testutil.Contains(t, err.Error(), "snapshot path")
}

func TestNewMigratorEmptySnapshotPath(t *testing.T) {
	_, err := directusmigrate.NewMigrator(directusmigrate.MigrationOptions{
		SnapshotPath: "",
		DatabaseURL:  sharedPG.ConnString,
	})
	testutil.Error(t, err)
	testutil.Contains(t, err.Error(), "snapshot path")
}

func TestNewMigratorEmptyDatabaseURL(t *testing.T) {
	_, err := directusmigrate.NewMigrator(directusmigrate.MigrationOptions{
		SnapshotPath: "testdata/snapshot.json",
		DatabaseURL:  "",
	})
	testutil.Error(t, err)
	testutil.Contains(t, err.Error(), "database URL")
}
