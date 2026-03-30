//go:build integration

package appwritemigrate_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/allyourbase/ayb/internal/appwritemigrate"
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
// It also ensures the "authenticated" role exists, which Appwrite RLS policies
// reference when granting access to user:* permissions.
func resetSchema(t *testing.T, ctx context.Context) {
	t.Helper()
	_, err := sharedPG.Pool.Exec(ctx, "DROP SCHEMA public CASCADE; CREATE SCHEMA public")
	testutil.NoError(t, err)
	// Create roles that RLS policies will reference. Ignore "already exists" errors.
	for _, role := range []string{"authenticated", "anon"} {
		_, _ = sharedPG.Pool.Exec(ctx, "CREATE ROLE "+role+" NOLOGIN")
	}
}

func TestMigrateBasicExport(t *testing.T) {
	ctx := context.Background()
	resetSchema(t, ctx)

	// Use the bundled test fixture.
	fixturePath, err := filepath.Abs("testdata/export.json")
	testutil.NoError(t, err)

	migrator, err := appwritemigrate.NewMigrator(appwritemigrate.MigrationOptions{
		ExportPath:  fixturePath,
		DatabaseURL: sharedPG.ConnString,
	})
	testutil.NoError(t, err)
	defer migrator.Close()

	stats, err := migrator.Migrate(ctx)
	testutil.NoError(t, err)

	// The fixture has 2 collections (authors, posts).
	testutil.Equal(t, 2, stats.Collections)
	testutil.True(t, stats.Attributes > 0, "should have created attributes")
	testutil.True(t, stats.Indexes > 0, "should have created indexes")
	testutil.True(t, stats.Documents > 0, "should have inserted documents")
	testutil.Equal(t, 0, len(stats.Errors))

	// Verify tables exist in the database.
	db, err := sql.Open("pgx", sharedPG.ConnString)
	testutil.NoError(t, err)
	defer db.Close()

	var tableCount int
	err = db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = 'public' AND table_type = 'BASE TABLE'").Scan(&tableCount)
	testutil.NoError(t, err)
	testutil.True(t, tableCount >= 2, "should have at least 2 tables (authors, posts)")

	// Verify data was inserted.
	var authorCount int
	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM authors").Scan(&authorCount)
	testutil.NoError(t, err)
	testutil.True(t, authorCount > 0, "authors table should have data")
}

func TestMigrateWithRLS(t *testing.T) {
	ctx := context.Background()
	resetSchema(t, ctx)

	fixturePath, err := filepath.Abs("testdata/export.json")
	testutil.NoError(t, err)

	migrator, err := appwritemigrate.NewMigrator(appwritemigrate.MigrationOptions{
		ExportPath:  fixturePath,
		DatabaseURL: sharedPG.ConnString,
	})
	testutil.NoError(t, err)
	defer migrator.Close()

	stats, err := migrator.Migrate(ctx)
	testutil.NoError(t, err)

	// The fixture has permissions on both collections, so RLS policies should be created.
	testutil.True(t, stats.Policies > 0, "should have created RLS policies")

	// Verify RLS is enabled on the tables.
	db, err := sql.Open("pgx", sharedPG.ConnString)
	testutil.NoError(t, err)
	defer db.Close()

	var rlsEnabled bool
	err = db.QueryRowContext(ctx,
		"SELECT relrowsecurity FROM pg_class WHERE relname = 'authors' AND relnamespace = 'public'::regnamespace").Scan(&rlsEnabled)
	testutil.NoError(t, err)
	testutil.True(t, rlsEnabled, "RLS should be enabled on authors table")
}

func TestMigrateSkipRLS(t *testing.T) {
	ctx := context.Background()
	resetSchema(t, ctx)

	fixturePath, err := filepath.Abs("testdata/export.json")
	testutil.NoError(t, err)

	migrator, err := appwritemigrate.NewMigrator(appwritemigrate.MigrationOptions{
		ExportPath:  fixturePath,
		DatabaseURL: sharedPG.ConnString,
		SkipRLS:     true,
	})
	testutil.NoError(t, err)
	defer migrator.Close()

	stats, err := migrator.Migrate(ctx)
	testutil.NoError(t, err)

	// With SkipRLS, no policies should be created.
	testutil.Equal(t, 0, stats.Policies)
}

func TestMigrateSkipData(t *testing.T) {
	ctx := context.Background()
	resetSchema(t, ctx)

	fixturePath, err := filepath.Abs("testdata/export.json")
	testutil.NoError(t, err)

	migrator, err := appwritemigrate.NewMigrator(appwritemigrate.MigrationOptions{
		ExportPath:  fixturePath,
		DatabaseURL: sharedPG.ConnString,
		SkipData:    true,
	})
	testutil.NoError(t, err)
	defer migrator.Close()

	stats, err := migrator.Migrate(ctx)
	testutil.NoError(t, err)

	// Schema should be created but no documents inserted.
	testutil.True(t, stats.Collections > 0, "should have created collections")
	testutil.Equal(t, 0, stats.Documents)

	// Verify table exists but is empty.
	db, err := sql.Open("pgx", sharedPG.ConnString)
	testutil.NoError(t, err)
	defer db.Close()

	var count int
	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM authors").Scan(&count)
	testutil.NoError(t, err)
	testutil.Equal(t, 0, count)
}

func TestMigrateDryRun(t *testing.T) {
	ctx := context.Background()
	resetSchema(t, ctx)

	fixturePath, err := filepath.Abs("testdata/export.json")
	testutil.NoError(t, err)

	migrator, err := appwritemigrate.NewMigrator(appwritemigrate.MigrationOptions{
		ExportPath:  fixturePath,
		DatabaseURL: sharedPG.ConnString,
		DryRun:      true,
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

func TestNewMigratorInvalidExportPath(t *testing.T) {
	_, err := appwritemigrate.NewMigrator(appwritemigrate.MigrationOptions{
		ExportPath:  "/nonexistent/path.json",
		DatabaseURL: sharedPG.ConnString,
	})
	testutil.Error(t, err)
	testutil.Contains(t, err.Error(), "export path")
}

func TestNewMigratorEmptyExportPath(t *testing.T) {
	_, err := appwritemigrate.NewMigrator(appwritemigrate.MigrationOptions{
		ExportPath:  "",
		DatabaseURL: sharedPG.ConnString,
	})
	testutil.Error(t, err)
	testutil.Contains(t, err.Error(), "export path")
}

func TestNewMigratorEmptyDatabaseURL(t *testing.T) {
	_, err := appwritemigrate.NewMigrator(appwritemigrate.MigrationOptions{
		ExportPath:  "testdata/export.json",
		DatabaseURL: "",
	})
	testutil.Error(t, err)
	testutil.Contains(t, err.Error(), "database URL")
}
