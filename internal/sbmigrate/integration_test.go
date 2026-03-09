//go:build integration

package sbmigrate

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/migrate"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/jackc/pgx/v5/pgxpool"
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

// setupSourceAndTarget resets the schema and creates both Supabase-like source schemas
// and AYB target tables within the same database (using separate schemas).
// Returns the connection string usable for both source and target.
func setupSourceAndTarget(t *testing.T) string {
	t.Helper()
	ctx := context.Background()

	// Reset everything.
	_, err := sharedPG.Pool.Exec(ctx, `
		DROP SCHEMA IF EXISTS public CASCADE;
		CREATE SCHEMA public;
		DROP SCHEMA IF EXISTS auth CASCADE;
		DROP SCHEMA IF EXISTS storage CASCADE;
	`)
	testutil.NoError(t, err)

	// Create Supabase-like auth schema.
	_, err = sharedPG.Pool.Exec(ctx, `
		CREATE SCHEMA auth;

		CREATE TABLE auth.users (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			email TEXT,
			encrypted_password TEXT,
			email_confirmed_at TIMESTAMPTZ,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			deleted_at TIMESTAMPTZ,
			is_anonymous BOOLEAN DEFAULT false
		);

		CREATE TABLE auth.identities (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			user_id UUID NOT NULL REFERENCES auth.users(id),
			provider TEXT NOT NULL,
			identity_data JSONB NOT NULL,
			created_at TIMESTAMPTZ DEFAULT NOW()
		);
	`)
	testutil.NoError(t, err)

	// Create Supabase-like storage schema.
	_, err = sharedPG.Pool.Exec(ctx, `
		CREATE SCHEMA storage;

		CREATE TABLE storage.buckets (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			public BOOLEAN NOT NULL DEFAULT false
		);

		CREATE TABLE storage.objects (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			bucket_id TEXT NOT NULL REFERENCES storage.buckets(id),
			name TEXT NOT NULL,
			metadata JSONB DEFAULT '{}',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
	`)
	testutil.NoError(t, err)

	// Create AYB target tables on public schema.
	_, err = sharedPG.Pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS _ayb_users (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			email TEXT NOT NULL,
			password_hash TEXT NOT NULL,
			email_verified BOOLEAN NOT NULL DEFAULT false,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_ayb_users_email ON _ayb_users (LOWER(email));

		CREATE TABLE IF NOT EXISTS _ayb_oauth_accounts (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			user_id UUID NOT NULL REFERENCES _ayb_users(id) ON DELETE CASCADE,
			provider TEXT NOT NULL,
			provider_user_id TEXT NOT NULL,
			email TEXT,
			name TEXT,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			UNIQUE(provider, provider_user_id)
		);
		CREATE INDEX IF NOT EXISTS idx_ayb_oauth_accounts_user_id ON _ayb_oauth_accounts (user_id);
	`)
	testutil.NoError(t, err)

	return sharedPG.ConnString
}

// insertSourceUser inserts a user into auth.users with the given params.
func insertSourceUser(t *testing.T, pool *pgxpool.Pool, id, email, passwordHash string, confirmed bool, anonymous bool) {
	t.Helper()
	ctx := context.Background()

	var emailConfAt *time.Time
	if confirmed {
		now := time.Now()
		emailConfAt = &now
	}

	_, err := pool.Exec(ctx, `
		INSERT INTO auth.users (id, email, encrypted_password, email_confirmed_at, is_anonymous)
		VALUES ($1, $2, $3, $4, $5)
	`, id, email, passwordHash, emailConfAt, anonymous)
	testutil.NoError(t, err)
}

// insertSourceIdentity inserts an OAuth identity into auth.identities.
func insertSourceIdentity(t *testing.T, pool *pgxpool.Pool, userID, provider, identityDataJSON string) {
	t.Helper()
	ctx := context.Background()
	_, err := pool.Exec(ctx, `
		INSERT INTO auth.identities (user_id, provider, identity_data)
		VALUES ($1, $2, $3::jsonb)
	`, userID, provider, identityDataJSON)
	testutil.NoError(t, err)
}

// insertSourceTable creates a public table and inserts rows (for schema+data migration tests).
func insertSourceTable(t *testing.T, pool *pgxpool.Pool, ddl string, inserts ...string) {
	t.Helper()
	ctx := context.Background()
	_, err := pool.Exec(ctx, ddl)
	testutil.NoError(t, err)
	for _, ins := range inserts {
		_, err = pool.Exec(ctx, ins)
		testutil.NoError(t, err)
	}
}

// insertStorageBucket creates a storage bucket and its objects.
func insertStorageBucket(t *testing.T, pool *pgxpool.Pool, id, name string, public bool, objects []struct {
	name, mime string
	size       int
}) {
	t.Helper()
	ctx := context.Background()
	_, err := pool.Exec(ctx, `INSERT INTO storage.buckets (id, name, public) VALUES ($1, $2, $3)`, id, name, public)
	testutil.NoError(t, err)
	for _, o := range objects {
		_, err = pool.Exec(ctx, `
			INSERT INTO storage.objects (bucket_id, name, metadata)
			VALUES ($1, $2, $3::jsonb)
		`, id, o.name, `{"size": `+itoa(o.size)+`, "mimetype": "`+o.mime+`"}`)
		testutil.NoError(t, err)
	}
}

func itoa(n int) string {
	return fmt.Sprintf("%d", n)
}

// createStorageExportDir creates a local directory mirroring the bucket/path structure.
func createStorageExportDir(t *testing.T, buckets map[string]map[string][]byte) string {
	t.Helper()
	dir := t.TempDir()
	for bucket, files := range buckets {
		for path, content := range files {
			fullPath := filepath.Join(dir, bucket, path)
			err := os.MkdirAll(filepath.Dir(fullPath), 0755)
			testutil.NoError(t, err)
			err = os.WriteFile(fullPath, content, 0644)
			testutil.NoError(t, err)
		}
	}
	return dir
}

func createIsolatedDatabaseURL(t *testing.T, adminConnStr, prefix string) string {
	t.Helper()

	adminDB, err := sql.Open("pgx", adminConnStr)
	testutil.NoError(t, err)
	defer adminDB.Close()

	dbName := fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
	_, err = adminDB.Exec(`CREATE DATABASE ` + dbName)
	testutil.NoError(t, err)

	u, err := url.Parse(adminConnStr)
	testutil.NoError(t, err)
	u.Path = "/" + dbName
	return u.String()
}

func dropIsolatedDatabase(t *testing.T, adminConnStr, dbURL string) {
	t.Helper()

	u, err := url.Parse(dbURL)
	testutil.NoError(t, err)
	dbName := strings.TrimPrefix(u.Path, "/")

	adminDB, err := sql.Open("pgx", adminConnStr)
	testutil.NoError(t, err)
	defer adminDB.Close()

	_, err = adminDB.Exec(`DROP DATABASE IF EXISTS ` + dbName + ` WITH (FORCE)`)
	testutil.NoError(t, err)
}

// --- Tests ---

func TestE2E_FullMigration(t *testing.T) {
	connStr := setupSourceAndTarget(t)

	// Populate source data.
	insertSourceUser(t, sharedPG.Pool,
		"aaaaaaaa-0000-0000-0000-000000000001", "alice@example.com", "$2a$10$hashedpassword1", true, false)
	insertSourceUser(t, sharedPG.Pool,
		"aaaaaaaa-0000-0000-0000-000000000002", "bob@example.com", "$2a$10$hashedpassword2", false, false)

	insertSourceIdentity(t, sharedPG.Pool,
		"aaaaaaaa-0000-0000-0000-000000000001", "email",
		`{"sub": "aaaaaaaa-0000-0000-0000-000000000001", "email": "alice@example.com"}`)
	insertSourceIdentity(t, sharedPG.Pool,
		"aaaaaaaa-0000-0000-0000-000000000001", "google",
		`{"sub": "g-12345", "email": "alice@gmail.com", "name": "Alice S"}`)

	// Create source tables with data.
	insertSourceTable(t, sharedPG.Pool,
		`CREATE TABLE posts (
			id SERIAL PRIMARY KEY,
			title TEXT NOT NULL,
			body TEXT,
			published BOOLEAN DEFAULT false
		)`,
		`INSERT INTO posts (title, body, published) VALUES ('First Post', 'Hello world', true)`,
		`INSERT INTO posts (title, body, published) VALUES ('Draft', 'WIP', false)`,
	)

	insertSourceTable(t, sharedPG.Pool,
		`CREATE TABLE comments (
			id SERIAL PRIMARY KEY,
			post_id INTEGER REFERENCES posts(id),
			text TEXT NOT NULL
		)`,
		`INSERT INTO comments (post_id, text) VALUES (1, 'Great post!')`,
	)

	// Create auth.uid() function so RLS policies referencing it can be created.
	_, err := sharedPG.Pool.Exec(context.Background(), `
		CREATE OR REPLACE FUNCTION auth.uid() RETURNS UUID AS $$
			SELECT gen_random_uuid();
		$$ LANGUAGE SQL;
	`)
	testutil.NoError(t, err)

	// Create RLS policies on source.
	_, err = sharedPG.Pool.Exec(context.Background(), `
		ALTER TABLE posts ENABLE ROW LEVEL SECURITY;
		CREATE POLICY posts_select ON posts FOR SELECT USING (true);
		CREATE POLICY posts_insert ON posts FOR INSERT WITH CHECK (auth.uid() IS NOT NULL);
	`)
	testutil.NoError(t, err)

	// Storage.
	insertStorageBucket(t, sharedPG.Pool, "avatars", "avatars", true, []struct {
		name, mime string
		size       int
	}{
		{"photo.jpg", "image/jpeg", 14},
	})
	storageExport := createStorageExportDir(t, map[string]map[string][]byte{
		"avatars": {"photo.jpg": []byte("fake-jpeg-data")},
	})
	tmpStorage := t.TempDir()

	migrator, err := NewMigrator(MigrationOptions{
		SourceURL:         connStr,
		TargetURL:         connStr,
		Force:             true, // source and target are same DB
		Verbose:           true,
		StorageExportPath: storageExport,
		StoragePath:       tmpStorage,
	})
	testutil.NoError(t, err)
	defer migrator.Close()

	ctx := context.Background()
	stats, err := migrator.Migrate(ctx)
	testutil.NoError(t, err)

	// Verify stats.
	// Note: source and target share the same DB. Tables already exist so CREATE IF NOT EXISTS
	// succeeds silently, and ON CONFLICT DO NOTHING means rows already present return 0 affected.
	testutil.Equal(t, 2, stats.Tables)  // posts, comments (excludes _ayb_ tables)
	testutil.Equal(t, 0, stats.Records) // 0 because rows already exist (ON CONFLICT DO NOTHING)
	testutil.Equal(t, 2, stats.Users)
	testutil.Equal(t, 1, stats.OAuthLinks) // google for alice (email provider skipped)
	testutil.Equal(t, 2, stats.Policies)
	testutil.Equal(t, 1, stats.StorageFiles)
	testutil.True(t, stats.StorageBytes > 0)

	// Verify storage file.
	verifyFile(t, filepath.Join(tmpStorage, "avatars", "photo.jpg"), []byte("fake-jpeg-data"))
}

func TestE2E_SchemaAndData(t *testing.T) {
	connStr := setupSourceAndTarget(t)

	// Create source with no auth users — skip OAuth/RLS for this test.
	insertSourceUser(t, sharedPG.Pool,
		"aaaaaaaa-0000-0000-0000-000000000001", "admin@example.com", "$2a$10$hash", true, false)

	insertSourceTable(t, sharedPG.Pool,
		`CREATE TABLE products (
			id SERIAL PRIMARY KEY,
			name TEXT NOT NULL,
			price NUMERIC(10,2),
			in_stock BOOLEAN DEFAULT true,
			created_at TIMESTAMPTZ DEFAULT NOW()
		)`,
		`INSERT INTO products (name, price) VALUES ('Widget', 9.99)`,
		`INSERT INTO products (name, price, in_stock) VALUES ('Gadget', 19.99, false)`,
		`INSERT INTO products (name, price) VALUES ('Doohickey', 29.99)`,
	)

	migrator, err := NewMigrator(MigrationOptions{
		SourceURL: connStr,
		TargetURL: connStr,
		SkipRLS:   true,
		SkipOAuth: true,
		Verbose:   true,
	})
	testutil.NoError(t, err)
	defer migrator.Close()

	ctx := context.Background()
	stats, err := migrator.Migrate(ctx)
	testutil.NoError(t, err)

	testutil.Equal(t, 1, stats.Tables)   // products (_ayb_ tables filtered)
	testutil.Equal(t, 0, stats.Records)  // 0: same-DB test, rows exist → ON CONFLICT DO NOTHING
	testutil.Equal(t, 1, stats.Users)    // admin
	testutil.Equal(t, 0, stats.Policies) // RLS skipped

	// Verify data exists (was already in source which shares same DB).
	db, err := sql.Open("pgx", connStr)
	testutil.NoError(t, err)
	defer db.Close()

	var name string
	var price float64
	err = db.QueryRow("SELECT name, price FROM products WHERE name = 'Widget'").Scan(&name, &price)
	testutil.NoError(t, err)
	testutil.Equal(t, "Widget", name)
	testutil.Equal(t, 9.99, price)
}

func TestE2E_AuthMigration(t *testing.T) {
	connStr := setupSourceAndTarget(t)

	// Insert various user types.
	insertSourceUser(t, sharedPG.Pool,
		"aaaaaaaa-0000-0000-0000-000000000001", "alice@example.com", "$2a$10$hash1", true, false)
	insertSourceUser(t, sharedPG.Pool,
		"aaaaaaaa-0000-0000-0000-000000000002", "bob@example.com", "$2a$10$hash2", false, false)
	insertSourceUser(t, sharedPG.Pool,
		"aaaaaaaa-0000-0000-0000-000000000003", "", "", false, true) // anonymous — skipped
	insertSourceUser(t, sharedPG.Pool,
		"aaaaaaaa-0000-0000-0000-000000000004", "oauth@example.com", "", true, false) // OAuth-only — gets $none$

	migrator, err := NewMigrator(MigrationOptions{
		SourceURL: connStr,
		TargetURL: connStr,
		SkipData:  true,
		SkipRLS:   true,
		SkipOAuth: true,
		Verbose:   true,
	})
	testutil.NoError(t, err)
	defer migrator.Close()

	ctx := context.Background()
	stats, err := migrator.Migrate(ctx)
	testutil.NoError(t, err)

	testutil.Equal(t, 3, stats.Users)   // alice, bob, oauth (anonymous filtered by query)
	testutil.Equal(t, 0, stats.Skipped) // anonymous filtered at SQL level, not counted as skip

	db, err := sql.Open("pgx", connStr)
	testutil.NoError(t, err)
	defer db.Close()

	// Verify alice is verified.
	var verified bool
	err = db.QueryRow(
		"SELECT email_verified FROM _ayb_users WHERE email = 'alice@example.com'",
	).Scan(&verified)
	testutil.NoError(t, err)
	testutil.True(t, verified)

	// Verify bob is not verified.
	err = db.QueryRow(
		"SELECT email_verified FROM _ayb_users WHERE email = 'bob@example.com'",
	).Scan(&verified)
	testutil.NoError(t, err)
	testutil.False(t, verified)

	// Verify OAuth-only user got $none$ password.
	var passwordHash string
	err = db.QueryRow(
		"SELECT password_hash FROM _ayb_users WHERE email = 'oauth@example.com'",
	).Scan(&passwordHash)
	testutil.NoError(t, err)
	testutil.Equal(t, "$none$", passwordHash)
}

func TestE2E_OAuthMigration(t *testing.T) {
	connStr := setupSourceAndTarget(t)

	insertSourceUser(t, sharedPG.Pool,
		"aaaaaaaa-0000-0000-0000-000000000001", "alice@example.com", "$2a$10$hash1", true, false)

	// Email identity (should be skipped).
	insertSourceIdentity(t, sharedPG.Pool,
		"aaaaaaaa-0000-0000-0000-000000000001", "email",
		`{"sub": "aaaaaaaa-0000-0000-0000-000000000001", "email": "alice@example.com"}`)
	// Google identity (should be imported).
	insertSourceIdentity(t, sharedPG.Pool,
		"aaaaaaaa-0000-0000-0000-000000000001", "google",
		`{"sub": "google-uid-123", "email": "alice@gmail.com", "name": "Alice"}`)
	// GitHub identity (should be imported).
	insertSourceIdentity(t, sharedPG.Pool,
		"aaaaaaaa-0000-0000-0000-000000000001", "github",
		`{"sub": "github-uid-456", "email": "alice@github.com", "full_name": "Alice Dev"}`)

	migrator, err := NewMigrator(MigrationOptions{
		SourceURL: connStr,
		TargetURL: connStr,
		SkipData:  true,
		SkipRLS:   true,
		Verbose:   true,
	})
	testutil.NoError(t, err)
	defer migrator.Close()

	ctx := context.Background()
	stats, err := migrator.Migrate(ctx)
	testutil.NoError(t, err)

	testutil.Equal(t, 2, stats.OAuthLinks) // google + github (email skipped)

	db, err := sql.Open("pgx", connStr)
	testutil.NoError(t, err)
	defer db.Close()

	// Verify google OAuth.
	var provider, providerUID, email, name string
	err = db.QueryRow(`
		SELECT provider, provider_user_id, email, name
		FROM _ayb_oauth_accounts
		WHERE provider = 'google'
	`).Scan(&provider, &providerUID, &email, &name)
	testutil.NoError(t, err)
	testutil.Equal(t, "google", provider)
	testutil.Equal(t, "google-uid-123", providerUID)
	testutil.Equal(t, "alice@gmail.com", email)
	testutil.Equal(t, "Alice", name)

	// Verify github OAuth.
	err = db.QueryRow(`
		SELECT provider, provider_user_id, email, name
		FROM _ayb_oauth_accounts
		WHERE provider = 'github'
	`).Scan(&provider, &providerUID, &email, &name)
	testutil.NoError(t, err)
	testutil.Equal(t, "github", provider)
	testutil.Equal(t, "github-uid-456", providerUID)
	testutil.Equal(t, "alice@github.com", email)
	testutil.Equal(t, "Alice Dev", name)
}

func TestE2E_RLSMigration(t *testing.T) {
	connStr := setupSourceAndTarget(t)

	// Need at least one auth user for migration to proceed.
	insertSourceUser(t, sharedPG.Pool,
		"aaaaaaaa-0000-0000-0000-000000000001", "admin@example.com", "$2a$10$hash", true, false)

	// Create source table with RLS policies that use Supabase auth functions.
	insertSourceTable(t, sharedPG.Pool,
		`CREATE TABLE documents (
			id SERIAL PRIMARY KEY,
			owner_id UUID,
			title TEXT NOT NULL
		)`,
		`INSERT INTO documents (owner_id, title) VALUES ('aaaaaaaa-0000-0000-0000-000000000001', 'My Doc')`,
	)

	// Create an auth.uid() function stub so the policies can be created.
	_, err := sharedPG.Pool.Exec(context.Background(), `
		CREATE OR REPLACE FUNCTION auth.uid() RETURNS UUID AS $$
			SELECT gen_random_uuid();
		$$ LANGUAGE SQL;
	`)
	testutil.NoError(t, err)

	_, err = sharedPG.Pool.Exec(context.Background(), `
		ALTER TABLE documents ENABLE ROW LEVEL SECURITY;
		CREATE POLICY documents_select ON documents FOR SELECT USING (true);
		CREATE POLICY documents_update ON documents FOR UPDATE
			USING (owner_id = auth.uid())
			WITH CHECK (owner_id = auth.uid());
	`)
	testutil.NoError(t, err)

	migrator, err := NewMigrator(MigrationOptions{
		SourceURL: connStr,
		TargetURL: connStr,
		SkipOAuth: true,
		Verbose:   true,
	})
	testutil.NoError(t, err)
	defer migrator.Close()

	ctx := context.Background()
	stats, err := migrator.Migrate(ctx)
	testutil.NoError(t, err)

	testutil.Equal(t, 2, stats.Policies) // documents_select + documents_update

	db, err := sql.Open("pgx", connStr)
	testutil.NoError(t, err)
	defer db.Close()

	// Verify RLS is enabled on documents table.
	var rlsEnabled bool
	err = db.QueryRow(`
		SELECT relrowsecurity FROM pg_class WHERE relname = 'documents'
	`).Scan(&rlsEnabled)
	testutil.NoError(t, err)
	testutil.True(t, rlsEnabled)

	// Verify policies exist.
	var policyCount int
	err = db.QueryRow(`
		SELECT COUNT(*) FROM pg_policies WHERE tablename = 'documents'
	`).Scan(&policyCount)
	testutil.NoError(t, err)
	testutil.Equal(t, 2, policyCount)

	// Verify the update policy's USING expression was rewritten.
	var policyDef string
	err = db.QueryRow(`
		SELECT pg_get_expr(pol.polqual, pol.polrelid)
		FROM pg_policy pol
		JOIN pg_class c ON c.oid = pol.polrelid
		WHERE c.relname = 'documents' AND pol.polname = 'documents_update'
	`).Scan(&policyDef)
	testutil.NoError(t, err)
	testutil.Contains(t, policyDef, "ayb.user_id")
}

func TestE2E_StorageMigration(t *testing.T) {
	connStr := setupSourceAndTarget(t)

	insertSourceUser(t, sharedPG.Pool,
		"aaaaaaaa-0000-0000-0000-000000000001", "admin@example.com", "$2a$10$hash", true, false)

	insertStorageBucket(t, sharedPG.Pool, "uploads", "uploads", true, []struct {
		name, mime string
		size       int
	}{
		{"images/photo.jpg", "image/jpeg", 14},
		{"images/banner.png", "image/png", 12},
		{"docs/readme.txt", "text/plain", 15},
	})
	insertStorageBucket(t, sharedPG.Pool, "private", "private", false, []struct {
		name, mime string
		size       int
	}{
		{"secret.pdf", "application/pdf", 8},
	})

	storageExport := createStorageExportDir(t, map[string]map[string][]byte{
		"uploads": {
			"images/photo.jpg":  []byte("fake-jpeg-data"),
			"images/banner.png": []byte("fake-png-data"),
			"docs/readme.txt":   []byte("fake-readme-data"),
		},
		"private": {
			"secret.pdf": []byte("fake-pdf"),
		},
	})

	tmpStorage := t.TempDir()

	migrator, err := NewMigrator(MigrationOptions{
		SourceURL:         connStr,
		TargetURL:         connStr,
		SkipData:          true,
		SkipRLS:           true,
		SkipOAuth:         true,
		StorageExportPath: storageExport,
		StoragePath:       tmpStorage,
		Verbose:           true,
	})
	testutil.NoError(t, err)
	defer migrator.Close()

	ctx := context.Background()
	stats, err := migrator.Migrate(ctx)
	testutil.NoError(t, err)

	testutil.Equal(t, 4, stats.StorageFiles)
	testutil.True(t, stats.StorageBytes > 0)

	verifyFile(t, filepath.Join(tmpStorage, "uploads", "images", "photo.jpg"), []byte("fake-jpeg-data"))
	verifyFile(t, filepath.Join(tmpStorage, "uploads", "images", "banner.png"), []byte("fake-png-data"))
	verifyFile(t, filepath.Join(tmpStorage, "uploads", "docs", "readme.txt"), []byte("fake-readme-data"))
	verifyFile(t, filepath.Join(tmpStorage, "private", "secret.pdf"), []byte("fake-pdf"))
}

func TestE2E_DryRun(t *testing.T) {
	connStr := setupSourceAndTarget(t)

	insertSourceUser(t, sharedPG.Pool,
		"aaaaaaaa-0000-0000-0000-000000000001", "alice@example.com", "$2a$10$hash", true, false)

	insertSourceTable(t, sharedPG.Pool,
		`CREATE TABLE notes (id SERIAL PRIMARY KEY, text TEXT NOT NULL)`,
		`INSERT INTO notes (text) VALUES ('hello')`,
	)

	migrator, err := NewMigrator(MigrationOptions{
		SourceURL: connStr,
		TargetURL: connStr,
		SkipRLS:   true,
		SkipOAuth: true,
		DryRun:    true,
		Verbose:   true,
	})
	testutil.NoError(t, err)
	defer migrator.Close()

	ctx := context.Background()
	stats, err := migrator.Migrate(ctx)
	testutil.NoError(t, err)

	// Stats should be populated even in dry-run.
	testutil.Equal(t, 1, stats.Tables)
	testutil.Equal(t, 0, stats.Records) // same-DB: rows already exist → ON CONFLICT DO NOTHING
	testutil.Equal(t, 1, stats.Users)

	// Verify the user was rolled back (not persisted).
	db, err := sql.Open("pgx", connStr)
	testutil.NoError(t, err)
	defer db.Close()

	var userCount int
	err = db.QueryRow("SELECT COUNT(*) FROM _ayb_users").Scan(&userCount)
	testutil.NoError(t, err)
	// The user INSERT happened inside the transaction, and DryRun triggers rollback.
	testutil.Equal(t, 0, userCount)
}

func TestE2E_Analyze(t *testing.T) {
	connStr := setupSourceAndTarget(t)

	insertSourceUser(t, sharedPG.Pool,
		"aaaaaaaa-0000-0000-0000-000000000001", "alice@example.com", "$2a$10$hash1", true, false)
	insertSourceUser(t, sharedPG.Pool,
		"aaaaaaaa-0000-0000-0000-000000000002", "bob@example.com", "$2a$10$hash2", false, false)

	insertSourceIdentity(t, sharedPG.Pool,
		"aaaaaaaa-0000-0000-0000-000000000001", "google",
		`{"sub": "g-123", "email": "alice@gmail.com"}`)

	insertSourceTable(t, sharedPG.Pool,
		`CREATE TABLE items (id SERIAL PRIMARY KEY, name TEXT)`,
		`INSERT INTO items (name) VALUES ('a')`,
		`INSERT INTO items (name) VALUES ('b')`,
	)

	// Add RLS policy.
	_, err := sharedPG.Pool.Exec(context.Background(), `
		CREATE OR REPLACE FUNCTION auth.uid() RETURNS UUID AS $$
			SELECT gen_random_uuid();
		$$ LANGUAGE SQL;
		ALTER TABLE items ENABLE ROW LEVEL SECURITY;
		CREATE POLICY items_select ON items FOR SELECT USING (true);
	`)
	testutil.NoError(t, err)

	// Storage buckets.
	insertStorageBucket(t, sharedPG.Pool, "media", "media", true, []struct {
		name, mime string
		size       int
	}{
		{"a.jpg", "image/jpeg", 100},
		{"b.jpg", "image/jpeg", 200},
	})

	migrator, err := NewMigrator(MigrationOptions{
		SourceURL: connStr,
		TargetURL: connStr,
	})
	testutil.NoError(t, err)
	defer migrator.Close()

	ctx := context.Background()
	report, err := migrator.Analyze(ctx)
	testutil.NoError(t, err)

	testutil.Equal(t, "Supabase", report.SourceType)
	testutil.Equal(t, 2, report.AuthUsers)
	testutil.Equal(t, 1, report.OAuthLinks)
	testutil.Equal(t, 1, report.Tables)      // items (_ayb_ tables filtered)
	testutil.Equal(t, 2, report.Records)     // 2 items
	testutil.Equal(t, 1, report.RLSPolicies) // items_select
	testutil.Equal(t, 2, report.Files)       // 2 storage objects
	testutil.True(t, report.FileSizeBytes > 0)
}

func TestE2E_AnalyzeWithoutIsAnonymousColumn(t *testing.T) {
	connStr := setupSourceAndTarget(t)

	_, err := sharedPG.Pool.Exec(context.Background(), `ALTER TABLE auth.users DROP COLUMN is_anonymous`)
	testutil.NoError(t, err)

	_, err = sharedPG.Pool.Exec(context.Background(), `
		INSERT INTO auth.users (id, email, encrypted_password, email_confirmed_at)
		VALUES ('aaaaaaaa-0000-0000-0000-000000000011', 'legacy@example.com', '$2a$10$hash', NOW())
	`)
	testutil.NoError(t, err)

	migrator, err := NewMigrator(MigrationOptions{
		SourceURL: connStr,
		TargetURL: connStr,
	})
	testutil.NoError(t, err)
	defer migrator.Close()

	report, err := migrator.Analyze(context.Background())
	testutil.NoError(t, err)
	testutil.Equal(t, 1, report.AuthUsers)
}

func TestE2E_AuthMigrationWithoutIsAnonymousColumn(t *testing.T) {
	connStr := setupSourceAndTarget(t)

	_, err := sharedPG.Pool.Exec(context.Background(), `ALTER TABLE auth.users DROP COLUMN is_anonymous`)
	testutil.NoError(t, err)

	_, err = sharedPG.Pool.Exec(context.Background(), `
		INSERT INTO auth.users (id, email, encrypted_password, email_confirmed_at)
		VALUES
		  ('aaaaaaaa-0000-0000-0000-000000000021', 'alice@example.com', '$2a$10$hash1', NOW()),
		  ('aaaaaaaa-0000-0000-0000-000000000022', 'bob@example.com', '$2a$10$hash2', NULL)
	`)
	testutil.NoError(t, err)

	migrator, err := NewMigrator(MigrationOptions{
		SourceURL: connStr,
		TargetURL: connStr,
		SkipData:  true,
		SkipOAuth: true,
		SkipRLS:   true,
	})
	testutil.NoError(t, err)
	defer migrator.Close()

	stats, err := migrator.Migrate(context.Background())
	testutil.NoError(t, err)
	testutil.Equal(t, 2, stats.Users)
}

func TestE2E_OAuthMigrationWithoutIdentityDataAndCreatedAt(t *testing.T) {
	connStr := setupSourceAndTarget(t)

	insertSourceUser(t, sharedPG.Pool,
		"aaaaaaaa-0000-0000-0000-000000000031", "alice@example.com", "$2a$10$hash1", true, false)

	_, err := sharedPG.Pool.Exec(context.Background(), `
		ALTER TABLE auth.identities DROP COLUMN identity_data;
		ALTER TABLE auth.identities DROP COLUMN created_at;
		ALTER TABLE auth.identities ADD COLUMN provider_id TEXT;
	`)
	testutil.NoError(t, err)

	_, err = sharedPG.Pool.Exec(context.Background(), `
		INSERT INTO auth.identities (user_id, provider, provider_id)
		VALUES
		  ('aaaaaaaa-0000-0000-0000-000000000031', 'google', 'google-legacy-123'),
		  ('aaaaaaaa-0000-0000-0000-000000000031', 'email', 'email-legacy-ignored')
	`)
	testutil.NoError(t, err)

	migrator, err := NewMigrator(MigrationOptions{
		SourceURL: connStr,
		TargetURL: connStr,
		SkipData:  true,
		SkipRLS:   true,
	})
	testutil.NoError(t, err)
	defer migrator.Close()

	stats, err := migrator.Migrate(context.Background())
	testutil.NoError(t, err)
	testutil.Equal(t, 1, stats.OAuthLinks)

	db, err := sql.Open("pgx", connStr)
	testutil.NoError(t, err)
	defer db.Close()

	var providerUserID string
	err = db.QueryRow(`
		SELECT provider_user_id
		FROM _ayb_oauth_accounts
		WHERE provider = 'google'
	`).Scan(&providerUserID)
	testutil.NoError(t, err)
	testutil.Equal(t, "google-legacy-123", providerUserID)
}

func TestE2E_IntrospectTablesFiltersHostedManagedTables(t *testing.T) {
	connStr := setupSourceAndTarget(t)

	db, err := sql.Open("pgx", connStr)
	testutil.NoError(t, err)
	defer db.Close()

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS products (id SERIAL PRIMARY KEY, name TEXT);
		CREATE TABLE IF NOT EXISTS supabase_migrations (version TEXT);
		CREATE TABLE IF NOT EXISTS buckets_vectors (id TEXT);
		CREATE TABLE IF NOT EXISTS vector_indexes (id TEXT);
	`)
	testutil.NoError(t, err)

	tables, err := introspectTables(context.Background(), db)
	testutil.NoError(t, err)

	names := make(map[string]bool, len(tables))
	for _, tbl := range tables {
		names[tbl.Name] = true
	}
	testutil.True(t, names["products"], "expected products table")
	testutil.False(t, names["supabase_migrations"], "internal table should be filtered")
	testutil.False(t, names["buckets_vectors"], "internal table should be filtered")
	testutil.False(t, names["vector_indexes"], "internal table should be filtered")
}

func TestE2E_SchemaMigrationSkipsIncompatibleFKChain(t *testing.T) {
	sourceURL := createIsolatedDatabaseURL(t, sharedPG.ConnString, "sb_src")
	targetURL := createIsolatedDatabaseURL(t, sharedPG.ConnString, "sb_tgt")
	defer dropIsolatedDatabase(t, sharedPG.ConnString, sourceURL)
	defer dropIsolatedDatabase(t, sharedPG.ConnString, targetURL)

	sourceDB, err := sql.Open("pgx", sourceURL)
	testutil.NoError(t, err)
	defer sourceDB.Close()

	targetDB, err := sql.Open("pgx", targetURL)
	testutil.NoError(t, err)
	defer targetDB.Close()

	_, err = sourceDB.Exec(`
		CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
		CREATE SCHEMA auth;
		CREATE TABLE auth.users (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			email TEXT,
			encrypted_password TEXT,
			email_confirmed_at TIMESTAMPTZ,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			deleted_at TIMESTAMPTZ,
			is_anonymous BOOLEAN DEFAULT false
		);

		CREATE TABLE legacy_parent (
			id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
			name TEXT NOT NULL
		);
		CREATE TABLE legacy_child (
			id SERIAL PRIMARY KEY,
			parent_id UUID NOT NULL REFERENCES legacy_parent(id),
			note TEXT
		);
		INSERT INTO legacy_parent (name) VALUES ('parent-a');
		INSERT INTO legacy_child (parent_id, note)
		SELECT id, 'child-a' FROM legacy_parent LIMIT 1;
	`)
	testutil.NoError(t, err)

	_, err = targetDB.Exec(`
		CREATE TABLE _ayb_users (
			id UUID PRIMARY KEY,
			email TEXT NOT NULL,
			password_hash TEXT NOT NULL,
			email_verified BOOLEAN NOT NULL DEFAULT false,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);

		CREATE TABLE _ayb_oauth_accounts (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			user_id UUID NOT NULL REFERENCES _ayb_users(id) ON DELETE CASCADE,
			provider TEXT NOT NULL,
			provider_user_id TEXT NOT NULL,
			email TEXT,
			name TEXT,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			UNIQUE(provider, provider_user_id)
		);
	`)
	testutil.NoError(t, err)

	var out strings.Builder
	migrator, err := NewMigrator(MigrationOptions{
		SourceURL:         sourceURL,
		TargetURL:         targetURL,
		Force:             true,
		SkipOAuth:         true,
		SkipRLS:           true,
		SkipStorage:       true,
		Verbose:           true,
		StoragePath:       t.TempDir(),
		StorageExportPath: "",
		Progress:          migrate.NewCLIReporter(&out),
	})
	testutil.NoError(t, err)
	defer migrator.Close()
	migrator.output = &out

	stats, err := migrator.Migrate(context.Background())
	testutil.NoError(t, err)
	testutil.True(t, stats.Skipped >= 2, "expected incompatible FK chain tables to be skipped")
	testutil.Contains(t, out.String(), "skipping table legacy_parent")
	testutil.Contains(t, out.String(), "skipping table legacy_child")

	var parentExists bool
	err = targetDB.QueryRow(`
		SELECT EXISTS (
			SELECT 1 FROM information_schema.tables
			WHERE table_schema = 'public' AND table_name = 'legacy_parent'
		)
	`).Scan(&parentExists)
	testutil.NoError(t, err)
	testutil.False(t, parentExists, "legacy_parent should be skipped in target")

	var childExists bool
	err = targetDB.QueryRow(`
		SELECT EXISTS (
			SELECT 1 FROM information_schema.tables
			WHERE table_schema = 'public' AND table_name = 'legacy_child'
		)
	`).Scan(&childExists)
	testutil.NoError(t, err)
	testutil.False(t, childExists, "legacy_child should be skipped in target")
}

func TestE2E_SchemaMigrationRetriesDeferredFKDependencies(t *testing.T) {
	sourceURL := createIsolatedDatabaseURL(t, sharedPG.ConnString, "sb_src_retry")
	targetURL := createIsolatedDatabaseURL(t, sharedPG.ConnString, "sb_tgt_retry")
	defer dropIsolatedDatabase(t, sharedPG.ConnString, sourceURL)
	defer dropIsolatedDatabase(t, sharedPG.ConnString, targetURL)

	sourceDB, err := sql.Open("pgx", sourceURL)
	testutil.NoError(t, err)
	defer sourceDB.Close()

	targetDB, err := sql.Open("pgx", targetURL)
	testutil.NoError(t, err)
	defer targetDB.Close()

	_, err = sourceDB.Exec(`
		CREATE SCHEMA auth;
		CREATE TABLE auth.users (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			email TEXT,
			encrypted_password TEXT,
			email_confirmed_at TIMESTAMPTZ,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			deleted_at TIMESTAMPTZ,
			is_anonymous BOOLEAN DEFAULT false
		);

		INSERT INTO auth.users (id, email, encrypted_password, email_confirmed_at)
		VALUES ('aaaaaaaa-0000-0000-0000-000000000041', 'seed@example.com', '$2a$10$hash', NOW());

		CREATE TABLE products (
			id UUID PRIMARY KEY,
			name TEXT NOT NULL
		);

		CREATE TABLE orders (
			id UUID PRIMARY KEY,
			product_id UUID NOT NULL REFERENCES products(id),
			qty INTEGER NOT NULL
		);

		INSERT INTO products (id, name) VALUES ('11111111-1111-1111-1111-111111111111', 'Widget');
		INSERT INTO orders (id, product_id, qty) VALUES ('22222222-2222-2222-2222-222222222222', '11111111-1111-1111-1111-111111111111', 2);
	`)
	testutil.NoError(t, err)

	_, err = targetDB.Exec(`
		CREATE TABLE _ayb_users (
			id UUID PRIMARY KEY,
			email TEXT NOT NULL,
			password_hash TEXT NOT NULL,
			email_verified BOOLEAN NOT NULL DEFAULT false,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);

		CREATE TABLE _ayb_oauth_accounts (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			user_id UUID NOT NULL REFERENCES _ayb_users(id) ON DELETE CASCADE,
			provider TEXT NOT NULL,
			provider_user_id TEXT NOT NULL,
			email TEXT,
			name TEXT,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			UNIQUE(provider, provider_user_id)
		);
	`)
	testutil.NoError(t, err)

	migrator, err := NewMigrator(MigrationOptions{
		SourceURL:         sourceURL,
		TargetURL:         targetURL,
		Force:             true,
		SkipOAuth:         true,
		SkipRLS:           true,
		SkipStorage:       true,
		StoragePath:       t.TempDir(),
		StorageExportPath: "",
	})
	testutil.NoError(t, err)
	defer migrator.Close()

	stats, err := migrator.Migrate(context.Background())
	testutil.NoError(t, err)
	testutil.Equal(t, 2, stats.Tables)
	testutil.Equal(t, 2, stats.Records)
	testutil.Equal(t, 0, stats.Skipped)

	var orderCount int
	err = targetDB.QueryRow(`SELECT COUNT(*) FROM orders`).Scan(&orderCount)
	testutil.NoError(t, err)
	testutil.Equal(t, 1, orderCount)
}

// --- Helpers ---

func verifyFile(t *testing.T, path string, expected []byte) {
	t.Helper()
	content, err := os.ReadFile(path)
	testutil.NoError(t, err)
	testutil.Equal(t, string(expected), string(content))
}
