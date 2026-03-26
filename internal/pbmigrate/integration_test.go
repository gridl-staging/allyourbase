//go:build integration

package pbmigrate

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "github.com/mattn/go-sqlite3"
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

// TestE2E_FullMigration tests the complete migration flow from PocketBase to PostgreSQL
func TestE2E_FullMigration(t *testing.T) {
	// Create temporary PocketBase fixture
	pbData := createPocketBaseFixture(t)
	defer os.RemoveAll(pbData)

	// Create temporary PostgreSQL database
	pgURL := createTestDatabase(t, "e2e_full_migration")
	defer dropTestDatabase(t, pgURL, "e2e_full_migration")

	// Use temp dir for storage to avoid stale state from prior runs
	tmpStorage := t.TempDir()

	// Run migration
	opts := MigrationOptions{
		SourcePath:  pbData,
		DatabaseURL: pgURL,
		StoragePath: tmpStorage,
		Verbose:     true,
	}

	migrator, err := NewMigrator(opts)
	testutil.NoError(t, err)
	defer migrator.Close()

	ctx := context.Background()
	stats, err := migrator.Migrate(ctx)
	testutil.NoError(t, err)

	// Verify statistics with exact values
	testutil.Equal(t, 4, stats.Collections) // posts, users (auth), comments, stats_view
	testutil.Equal(t, 2, stats.Tables)      // posts, comments (users is auth, stats_view is view)
	testutil.Equal(t, 1, stats.Views)       // stats_view
	testutil.Equal(t, 5, stats.Records)     // 3 posts + 2 comments (auth users counted separately in AuthUsers)
	testutil.Equal(t, 2, stats.Files)       // image1.jpg + image2.png
	testutil.True(t, stats.Policies >= 6)   // At least 6 RLS policies (3 per table)

	// Verify schema was created
	verifySchemaCreated(t, pgURL)

	// Verify data was migrated
	verifyDataMigrated(t, pgURL)

	// Verify auth users were migrated
	verifyAuthUsersMigrated(t, pgURL)

	// Verify files were copied to temp dir (not stale CWD)
	verifyFilesCopied(t, tmpStorage)

	// Verify RLS policies were created
	verifyRLSPolicies(t, pgURL)
}

// TestE2E_AuthMigration tests auth user migration with custom fields
func TestE2E_AuthMigration(t *testing.T) {
	pbData := createPocketBaseWithAuthUsers(t)
	defer os.RemoveAll(pbData)

	pgURL := createTestDatabase(t, "e2e_auth_migration")
	defer dropTestDatabase(t, pgURL, "e2e_auth_migration")

	opts := MigrationOptions{
		SourcePath:  pbData,
		DatabaseURL: pgURL,
		Verbose:     true,
	}

	migrator, err := NewMigrator(opts)
	testutil.NoError(t, err)
	defer migrator.Close()

	ctx := context.Background()
	stats, err := migrator.Migrate(ctx)
	testutil.NoError(t, err)

	// Verify auth users
	db, err := sql.Open("pgx", pgURL)
	testutil.NoError(t, err)
	defer db.Close()

	// Check _ayb_users table
	var userCount int
	err = db.QueryRow("SELECT COUNT(*) FROM _ayb_users").Scan(&userCount)
	testutil.NoError(t, err)
	testutil.Equal(t, 3, userCount)

	// Check ID mapping table exists
	var mapCount int
	err = db.QueryRow("SELECT COUNT(*) FROM _ayb_pb_id_map").Scan(&mapCount)
	testutil.NoError(t, err)
	testutil.Equal(t, 3, mapCount)

	// Check user profiles table exists and has custom fields
	var profileCount int
	err = db.QueryRow("SELECT COUNT(*) FROM _ayb_user_profiles_users").Scan(&profileCount)
	testutil.NoError(t, err)
	testutil.Equal(t, 3, profileCount)

	// Verify user data
	var email, passwordHash, name, role string
	var verified bool
	err = db.QueryRow(`
		SELECT u.email, u.password_hash, u.email_verified, p.name, p.role
		FROM _ayb_users u
		JOIN _ayb_user_profiles_users p ON u.id = p.user_id
		WHERE u.email = $1
	`, "alice@example.com").Scan(&email, &passwordHash, &verified, &name, &role)
	testutil.NoError(t, err)
	testutil.Equal(t, "alice@example.com", email)
	testutil.True(t, verified)
	testutil.Equal(t, "Alice Smith", name)
	testutil.Equal(t, "admin", role)
	testutil.NotEqual(t, "", passwordHash)

	testutil.Equal(t, 3, stats.AuthUsers) // 3 auth users migrated
}

// TestE2E_FileMigration tests file migration
func TestE2E_FileMigration(t *testing.T) {
	pbData := createPocketBaseWithFiles(t)
	defer os.RemoveAll(pbData)

	pgURL := createTestDatabase(t, "e2e_file_migration")
	defer dropTestDatabase(t, pgURL, "e2e_file_migration")

	// Create temp storage directory
	tmpStorage := t.TempDir()

	opts := MigrationOptions{
		SourcePath:  pbData,
		DatabaseURL: pgURL,
		StoragePath: tmpStorage,
		Verbose:     true,
	}

	migrator, err := NewMigrator(opts)
	testutil.NoError(t, err)
	defer migrator.Close()

	ctx := context.Background()
	stats, err := migrator.Migrate(ctx)
	testutil.NoError(t, err)

	testutil.True(t, stats.Files >= 3)

	// Verify files were copied
	verifyFile(t, filepath.Join(tmpStorage, "posts", "image1.jpg"), []byte("fake-jpeg-data"))
	verifyFile(t, filepath.Join(tmpStorage, "posts", "image2.png"), []byte("fake-png-data"))
	verifyFile(t, filepath.Join(tmpStorage, "posts", "nested", "doc.pdf"), []byte("fake-pdf-data"))
}

// TestE2E_DryRun tests dry run mode
func TestE2E_DryRun(t *testing.T) {
	pbData := createPocketBaseFixture(t)
	defer os.RemoveAll(pbData)

	pgURL := createTestDatabase(t, "e2e_dry_run")
	defer dropTestDatabase(t, pgURL, "e2e_dry_run")

	opts := MigrationOptions{
		SourcePath:  pbData,
		DatabaseURL: pgURL,
		DryRun:      true,
		Verbose:     false,
	}

	migrator, err := NewMigrator(opts)
	testutil.NoError(t, err)
	defer migrator.Close()

	ctx := context.Background()
	stats, err := migrator.Migrate(ctx)
	testutil.NoError(t, err)

	// Stats should be populated even in dry-run mode
	testutil.Equal(t, 4, stats.Collections) // posts, users, comments, stats_view
	testutil.True(t, stats.Tables > 0)

	// But database should be empty (no tables created)
	db, err := sql.Open("pgx", pgURL)
	testutil.NoError(t, err)
	defer db.Close()

	var tableCount int
	err = db.QueryRow(`
		SELECT COUNT(*)
		FROM information_schema.tables
		WHERE table_schema = 'public'
		AND table_type = 'BASE TABLE'
		AND table_name NOT LIKE 'pg_%'
	`).Scan(&tableCount)
	testutil.NoError(t, err)
	testutil.Equal(t, 0, tableCount)
}

// TestE2E_SkipFiles tests skipping file migration
func TestE2E_SkipFiles(t *testing.T) {
	pbData := createPocketBaseWithFiles(t)
	defer os.RemoveAll(pbData)

	pgURL := createTestDatabase(t, "e2e_skip_files")
	defer dropTestDatabase(t, pgURL, "e2e_skip_files")

	tmpStorage := t.TempDir()

	opts := MigrationOptions{
		SourcePath:  pbData,
		DatabaseURL: pgURL,
		SkipFiles:   true,
		StoragePath: tmpStorage,
		Verbose:     false,
	}

	migrator, err := NewMigrator(opts)
	testutil.NoError(t, err)
	defer migrator.Close()

	ctx := context.Background()
	stats, err := migrator.Migrate(ctx)
	testutil.NoError(t, err)

	// Files should not be copied
	testutil.Equal(t, 0, stats.Files)

	// Storage directory should be empty
	entries, err := os.ReadDir(tmpStorage)
	testutil.NoError(t, err)
	testutil.Equal(t, 0, len(entries))
}

func TestE2E_IndexMigration(t *testing.T) {
	pbData := createPocketBaseWithIndexes(t)
	defer os.RemoveAll(pbData)

	pgURL := createTestDatabase(t, "e2e_index_migration")
	defer dropTestDatabase(t, pgURL, "e2e_index_migration")

	opts := MigrationOptions{
		SourcePath:  pbData,
		DatabaseURL: pgURL,
		Verbose:     true,
	}

	migrator, err := NewMigrator(opts)
	testutil.NoError(t, err)
	defer migrator.Close()

	_, err = migrator.Migrate(context.Background())
	testutil.NoError(t, err)

	verifyIndexExists(t, pgURL, "idx_posts_title", false)
	verifyIndexExists(t, pgURL, "idx_posts_title_updated", false)
	verifyIndexExists(t, pgURL, "idx_posts_title_unique", true)
}

func TestE2E_IndexMigrationRejectsUnsupportedDefinition(t *testing.T) {
	pbData := createPocketBaseWithUnsupportedIndex(t)
	defer os.RemoveAll(pbData)

	pgURL := createTestDatabase(t, "e2e_index_migration_unsupported")
	defer dropTestDatabase(t, pgURL, "e2e_index_migration_unsupported")

	opts := MigrationOptions{
		SourcePath:  pbData,
		DatabaseURL: pgURL,
		Verbose:     true,
	}

	migrator, err := NewMigrator(opts)
	testutil.NoError(t, err)
	defer migrator.Close()

	_, err = migrator.Migrate(context.Background())
	testutil.ErrorContains(t, err, "failed to translate indexes for posts")
	testutil.ErrorContains(t, err, "unsupported SQLite index definition")

	db, err := sql.Open("pgx", pgURL)
	testutil.NoError(t, err)
	defer db.Close()

	var exists bool
	err = db.QueryRow(`
		SELECT EXISTS (
			SELECT 1 FROM information_schema.tables
			WHERE table_name = 'posts'
		)
	`).Scan(&exists)
	testutil.NoError(t, err)
	testutil.False(t, exists)
}

func TestE2E_Regression_SuccessFixtureIndexesFilesRLSReporting(t *testing.T) {
	pbData := createPocketBaseFixture(t)
	defer os.RemoveAll(pbData)

	fixtureDB, err := sql.Open("sqlite3", filepath.Join(pbData, "data.db"))
	testutil.NoError(t, err)
	defer fixtureDB.Close()

	indexesJSON, err := json.Marshal([]string{
		"CREATE INDEX idx_posts_title ON posts (title)",
		"CREATE UNIQUE INDEX idx_posts_title_unique ON posts (title)",
	})
	testutil.NoError(t, err)

	_, err = fixtureDB.Exec(`UPDATE _collections SET indexes = ? WHERE name = ?`, string(indexesJSON), "posts")
	testutil.NoError(t, err)

	pgURL := createTestDatabase(t, "e2e_regression_success_fixture")
	defer dropTestDatabase(t, pgURL, "e2e_regression_success_fixture")

	tmpStorage := t.TempDir()
	opts := MigrationOptions{
		SourcePath:  pbData,
		DatabaseURL: pgURL,
		StoragePath: tmpStorage,
		Verbose:     false,
	}

	migrator, err := NewMigrator(opts)
	testutil.NoError(t, err)
	defer migrator.Close()

	stats, err := migrator.Migrate(context.Background())
	testutil.NoError(t, err)

	verifyIndexExists(t, pgURL, "idx_posts_title", false)
	verifyIndexExists(t, pgURL, "idx_posts_title_unique", true)
	verifyFilesCopied(t, tmpStorage)
	verifyRLSPolicies(t, pgURL)

	testutil.Equal(t, 2, stats.Files)
	testutil.True(t, stats.Policies >= 6)
	testutil.Nil(t, stats.Errors)
	testutil.Nil(t, stats.FailedFiles)
}

func TestE2E_Regression_AuthValidationFailureRollsBackMigration(t *testing.T) {
	tests := []struct {
		name    string
		dbName  string
		wantErr string
		mutate  func(t *testing.T, db *sql.DB)
	}{
		{
			name:    "duplicate emails",
			dbName:  "e2e_auth_validation_duplicate",
			wantErr: "duplicate email",
			mutate: func(t *testing.T, db *sql.DB) {
				t.Helper()
				_, err := db.Exec(`
					INSERT INTO users (id, created, updated, email, passwordHash, verified)
					VALUES ('user2', '2024-01-01 00:00:00.000Z', '2024-01-01 00:00:00.000Z', 'user@example.com', '$2a$10$anotherhash', 1)
				`)
				testutil.NoError(t, err)
			},
		},
		{
			name:    "empty password hash",
			dbName:  "e2e_auth_validation_empty_hash",
			wantErr: "empty password hash",
			mutate: func(t *testing.T, db *sql.DB) {
				t.Helper()
				_, err := db.Exec(`UPDATE users SET passwordHash = '   ' WHERE id = 'user1'`)
				testutil.NoError(t, err)
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			pbData := createPocketBaseFixture(t)
			defer os.RemoveAll(pbData)

			fixtureDB, err := sql.Open("sqlite3", filepath.Join(pbData, "data.db"))
			testutil.NoError(t, err)
			defer fixtureDB.Close()

			tt.mutate(t, fixtureDB)

			pgURL := createTestDatabase(t, tt.dbName)
			defer dropTestDatabase(t, pgURL, tt.dbName)

			tmpStorage := t.TempDir()
			opts := MigrationOptions{
				SourcePath:  pbData,
				DatabaseURL: pgURL,
				StoragePath: tmpStorage,
				Verbose:     false,
			}

			migrator, err := NewMigrator(opts)
			testutil.NoError(t, err)
			defer migrator.Close()

			_, err = migrator.Migrate(context.Background())
			testutil.ErrorContains(t, err, tt.wantErr)

			db, err := sql.Open("pgx", pgURL)
			testutil.NoError(t, err)
			defer db.Close()

			var postsExists bool
			err = db.QueryRow(`SELECT to_regclass('public.posts') IS NOT NULL`).Scan(&postsExists)
			testutil.NoError(t, err)
			testutil.False(t, postsExists)

			var usersExists bool
			err = db.QueryRow(`SELECT to_regclass('public._ayb_users') IS NOT NULL`).Scan(&usersExists)
			testutil.NoError(t, err)
			testutil.False(t, usersExists)

			entries, err := os.ReadDir(tmpStorage)
			testutil.NoError(t, err)
			testutil.Equal(t, 0, len(entries))
		})
	}
}

// TestE2E_NilCustomFieldLandsAsSQLNull verifies that an auth user whose
// custom fields are nil/absent in PocketBase is migrated successfully with
// SQL NULL values in the _ayb_user_profiles_<collection> table — not as
// empty strings or missing rows.
func TestE2E_NilCustomFieldLandsAsSQLNull(t *testing.T) {
	// Start from the auth fixture which has custom fields (name, role, avatar).
	pbData := createPocketBaseWithAuthUsers(t)
	defer os.RemoveAll(pbData)

	fixtureDB, err := sql.Open("sqlite3", filepath.Join(pbData, "data.db"))
	testutil.NoError(t, err)
	defer fixtureDB.Close()

	// Relax the "name" field's Required flag so the profiles table column
	// is nullable and NULL inserts are accepted by PostgreSQL.
	schemaJSON, err := json.Marshal([]PBField{
		{Name: "email", Type: "email", Required: true, System: true},
		{Name: "passwordHash", Type: "text", Required: true, System: true},
		{Name: "verified", Type: "bool", Required: false, System: true},
		{Name: "name", Type: "text", Required: false, System: false},
		{Name: "role", Type: "select", Required: false, System: false},
		{Name: "avatar", Type: "file", Required: false, System: false},
	})
	testutil.NoError(t, err)
	_, err = fixtureDB.Exec(`UPDATE _collections SET schema = ? WHERE name = 'users'`, string(schemaJSON))
	testutil.NoError(t, err)

	// Add a user whose custom fields are all NULL.
	_, err = fixtureDB.Exec(`
		INSERT INTO users (id, created, updated, email, passwordHash, verified, name, role, avatar)
		VALUES ('user4nil', '2024-01-04 00:00:00.000Z', '2024-01-04 00:00:00.000Z',
		        'niluser@example.com', '$2a$10$hashedpassword4', 1, NULL, NULL, NULL)
	`)
	testutil.NoError(t, err)

	pgURL := createTestDatabase(t, "e2e_nil_custom_field")
	defer dropTestDatabase(t, pgURL, "e2e_nil_custom_field")

	opts := MigrationOptions{
		SourcePath:  pbData,
		DatabaseURL: pgURL,
		Verbose:     true,
	}

	migrator, err := NewMigrator(opts)
	testutil.NoError(t, err)
	defer migrator.Close()

	stats, err := migrator.Migrate(context.Background())
	testutil.NoError(t, err)
	testutil.Equal(t, 4, stats.AuthUsers) // 3 original + 1 nil-field user

	// Verify the nil-field user has a profile row with SQL NULLs.
	db, err := sql.Open("pgx", pgURL)
	testutil.NoError(t, err)
	defer db.Close()

	var profileCount int
	err = db.QueryRow("SELECT COUNT(*) FROM _ayb_user_profiles_users").Scan(&profileCount)
	testutil.NoError(t, err)
	testutil.Equal(t, 4, profileCount) // all 4 users must have profile rows

	// Query the nil-field user's profile via the user email.
	var name, role, avatar sql.NullString
	err = db.QueryRow(`
		SELECT p.name, p.role, p.avatar
		FROM _ayb_user_profiles_users p
		JOIN _ayb_users u ON u.id = p.user_id
		WHERE u.email = $1
	`, "niluser@example.com").Scan(&name, &role, &avatar)
	testutil.NoError(t, err)

	// All three custom fields must be SQL NULL, not empty strings.
	testutil.False(t, name.Valid, "expected name to be SQL NULL")
	testutil.False(t, role.Valid, "expected role to be SQL NULL")
	testutil.False(t, avatar.Valid, "expected avatar to be SQL NULL")
}

// TestE2E_FileCopyFailureSurfacesInStats verifies that when a source file
// is missing/corrupt, migration still succeeds but records the failure in
// MigrationStats.FailedFiles. Non-failed files must still be copied.
func TestE2E_FileCopyFailureSurfacesInStats(t *testing.T) {
	pbData := createPocketBaseFixture(t)
	defer os.RemoveAll(pbData)

	// Make one source file unreadable to trigger a copy failure.
	// (Removing the file would cause Walk to skip it silently.)
	makeFixtureFileUnreadable(t, pbData, "posts", "image1.jpg")

	pgURL := createTestDatabase(t, "e2e_file_copy_failure")
	defer dropTestDatabase(t, pgURL, "e2e_file_copy_failure")

	tmpStorage := t.TempDir()
	opts := MigrationOptions{
		SourcePath:  pbData,
		DatabaseURL: pgURL,
		StoragePath: tmpStorage,
		Verbose:     true,
	}

	migrator, err := NewMigrator(opts)
	testutil.NoError(t, err)
	defer migrator.Close()

	stats, err := migrator.Migrate(context.Background())
	testutil.NoError(t, err) // migration must still succeed

	// The removed file should appear in FailedFiles.
	testutil.Equal(t, 1, len(stats.FailedFiles))
	testutil.Equal(t, "posts/image1.jpg", stats.FailedFiles[0])

	// The surviving file must still be copied.
	verifyFile(t, filepath.Join(tmpStorage, "posts", "image2.png"), []byte("fake-png-data"))
}

// TestE2E_UnsupportedRLSTokenRollsBack verifies that an unsupported
// PocketBase rule token causes Migrate() to return an error and roll back
// the transaction — no partial schema committed, storage dir empty.
func TestE2E_UnsupportedRLSTokenRollsBack(t *testing.T) {
	pbData := createPocketBaseFixture(t)
	defer os.RemoveAll(pbData)

	// Mutate: inject an unsupported PocketBase array operator into the
	// posts collection's listRule so GenerateRLSPolicies fails.
	fixtureDB, err := sql.Open("sqlite3", filepath.Join(pbData, "data.db"))
	testutil.NoError(t, err)
	defer fixtureDB.Close()

	_, err = fixtureDB.Exec(`UPDATE _collections SET listRule = 'tags ?= ''featured''' WHERE name = 'posts'`)
	testutil.NoError(t, err)

	pgURL := createTestDatabase(t, "e2e_unsupported_rls")
	defer dropTestDatabase(t, pgURL, "e2e_unsupported_rls")

	tmpStorage := t.TempDir()
	opts := MigrationOptions{
		SourcePath:  pbData,
		DatabaseURL: pgURL,
		StoragePath: tmpStorage,
		Verbose:     false,
	}

	migrator, err := NewMigrator(opts)
	testutil.NoError(t, err)
	defer migrator.Close()

	// Migrate() returns nil stats on error paths, so only check the error.
	_, err = migrator.Migrate(context.Background())
	testutil.True(t, err != nil, "expected Migrate() to return an error for unsupported RLS token")
	testutil.True(t, strings.Contains(err.Error(), "failed to convert rule") ||
		strings.Contains(err.Error(), "RLS"),
		"error should reference RLS conversion failure, got: %s", err.Error())

	// No partial schema should be committed (transaction rolled back).
	db, err := sql.Open("pgx", pgURL)
	testutil.NoError(t, err)
	defer db.Close()

	var postsExists bool
	err = db.QueryRow(`SELECT to_regclass('public.posts') IS NOT NULL`).Scan(&postsExists)
	testutil.NoError(t, err)
	testutil.False(t, postsExists, "posts table should not exist after rollback")

	// Storage directory should be empty — file phase runs after commit,
	// so it should never execute.
	entries, err := os.ReadDir(tmpStorage)
	testutil.NoError(t, err)
	testutil.Equal(t, 0, len(entries))
}
