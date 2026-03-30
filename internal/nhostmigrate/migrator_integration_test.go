//go:build integration

package nhostmigrate_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/allyourbase/ayb/internal/nhostmigrate"
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
// It also ensures roles referenced by Hasura RLS metadata exist.
func resetSchema(t *testing.T, ctx context.Context) {
	t.Helper()
	_, err := sharedPG.Pool.Exec(ctx, "DROP SCHEMA public CASCADE; CREATE SCHEMA public")
	testutil.NoError(t, err)
	// Create roles that the Hasura metadata permissions reference.
	// "user" is a reserved keyword in PostgreSQL, so it must be quoted.
	// Ignore "already exists" errors.
	for _, role := range []string{"user", "editor"} {
		_, _ = sharedPG.Pool.Exec(ctx, `CREATE ROLE "`+role+`" NOLOGIN`)
	}
}

func TestMigrateBasicPgDump(t *testing.T) {
	ctx := context.Background()
	resetSchema(t, ctx)

	pgDumpPath, err := filepath.Abs("testdata/pg_dump.sql")
	testutil.NoError(t, err)
	metadataPath, err := filepath.Abs("testdata/metadata")
	testutil.NoError(t, err)

	migrator, err := nhostmigrate.NewMigrator(nhostmigrate.MigrationOptions{
		PgDumpPath:         pgDumpPath,
		HasuraMetadataPath: metadataPath,
		DatabaseURL:        sharedPG.ConnString,
	})
	testutil.NoError(t, err)
	defer migrator.Close()

	stats, err := migrator.Migrate(ctx)
	testutil.NoError(t, err)

	// The fixture has 2 public tables (authors, posts) plus hdb_catalog.hdb_table (skipped).
	testutil.True(t, stats.Tables >= 2, "should have created at least 2 tables")
	testutil.True(t, stats.Records > 0, "should have inserted records")
	testutil.True(t, stats.Indexes > 0, "should have created indexes")
	testutil.Equal(t, 0, len(stats.Errors))

	// Verify tables exist.
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

	pgDumpPath, err := filepath.Abs("testdata/pg_dump.sql")
	testutil.NoError(t, err)
	metadataPath, err := filepath.Abs("testdata/metadata")
	testutil.NoError(t, err)

	migrator, err := nhostmigrate.NewMigrator(nhostmigrate.MigrationOptions{
		PgDumpPath:         pgDumpPath,
		HasuraMetadataPath: metadataPath,
		DatabaseURL:        sharedPG.ConnString,
	})
	testutil.NoError(t, err)
	defer migrator.Close()

	stats, err := migrator.Migrate(ctx)
	testutil.NoError(t, err)

	// The metadata fixtures have select_permissions for user/editor roles.
	testutil.True(t, stats.Policies > 0, "should have created RLS policies from Hasura metadata")

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

	pgDumpPath, err := filepath.Abs("testdata/pg_dump.sql")
	testutil.NoError(t, err)
	metadataPath, err := filepath.Abs("testdata/metadata")
	testutil.NoError(t, err)

	migrator, err := nhostmigrate.NewMigrator(nhostmigrate.MigrationOptions{
		PgDumpPath:         pgDumpPath,
		HasuraMetadataPath: metadataPath,
		DatabaseURL:        sharedPG.ConnString,
		SkipRLS:            true,
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

	pgDumpPath, err := filepath.Abs("testdata/pg_dump.sql")
	testutil.NoError(t, err)
	metadataPath, err := filepath.Abs("testdata/metadata")
	testutil.NoError(t, err)

	migrator, err := nhostmigrate.NewMigrator(nhostmigrate.MigrationOptions{
		PgDumpPath:         pgDumpPath,
		HasuraMetadataPath: metadataPath,
		DatabaseURL:        sharedPG.ConnString,
		DryRun:             true,
	})
	testutil.NoError(t, err)
	defer migrator.Close()

	stats, err := migrator.Migrate(ctx)
	testutil.NoError(t, err)

	// DryRun should still report stats but roll back.
	testutil.True(t, stats.Tables > 0, "dry run should report tables")

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

func TestMigrateSkipsHdbCatalog(t *testing.T) {
	ctx := context.Background()
	resetSchema(t, ctx)

	pgDumpPath, err := filepath.Abs("testdata/pg_dump.sql")
	testutil.NoError(t, err)
	metadataPath, err := filepath.Abs("testdata/metadata")
	testutil.NoError(t, err)

	migrator, err := nhostmigrate.NewMigrator(nhostmigrate.MigrationOptions{
		PgDumpPath:         pgDumpPath,
		HasuraMetadataPath: metadataPath,
		DatabaseURL:        sharedPG.ConnString,
	})
	testutil.NoError(t, err)
	defer migrator.Close()

	_, err = migrator.Migrate(ctx)
	testutil.NoError(t, err)

	// The hdb_catalog schema should NOT be created — NHost migration
	// filters out Hasura internal schemas.
	db, err := sql.Open("pgx", sharedPG.ConnString)
	testutil.NoError(t, err)
	defer db.Close()

	var hdbCount int
	err = db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM information_schema.schemata WHERE schema_name = 'hdb_catalog'").Scan(&hdbCount)
	testutil.NoError(t, err)
	testutil.Equal(t, 0, hdbCount)
}

func TestNewMigratorInvalidPaths(t *testing.T) {
	// Missing metadata path (validated first).
	_, err := nhostmigrate.NewMigrator(nhostmigrate.MigrationOptions{
		PgDumpPath:         "testdata/pg_dump.sql",
		HasuraMetadataPath: "",
		DatabaseURL:        sharedPG.ConnString,
	})
	testutil.Error(t, err)
	testutil.Contains(t, err.Error(), "hasura metadata path")

	// Missing pg_dump path.
	_, err = nhostmigrate.NewMigrator(nhostmigrate.MigrationOptions{
		PgDumpPath:         "",
		HasuraMetadataPath: "testdata/metadata",
		DatabaseURL:        sharedPG.ConnString,
	})
	testutil.Error(t, err)
	testutil.Contains(t, err.Error(), "pg_dump path")

	// Missing database URL.
	_, err = nhostmigrate.NewMigrator(nhostmigrate.MigrationOptions{
		PgDumpPath:         "testdata/pg_dump.sql",
		HasuraMetadataPath: "testdata/metadata",
		DatabaseURL:        "",
	})
	testutil.Error(t, err)
	testutil.Contains(t, err.Error(), "database URL")
}
