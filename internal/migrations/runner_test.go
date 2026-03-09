//go:build integration

package migrations_test

import (
	"context"
	"os"
	"testing"
	"testing/fstest"

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

// resetDB drops and recreates the public schema for test isolation.
func resetDB(t *testing.T, ctx context.Context) {
	t.Helper()
	_, err := sharedPG.Pool.Exec(ctx, "DROP SCHEMA public CASCADE; CREATE SCHEMA public")
	if err != nil {
		t.Fatalf("resetting schema: %v", err)
	}
}

func TestBootstrap(t *testing.T) {
	ctx := context.Background()
	resetDB(t, ctx)

	runner := migrations.NewRunner(sharedPG.Pool, testutil.DiscardLogger())

	// Bootstrap should create _ayb_migrations table.
	err := runner.Bootstrap(ctx)
	testutil.NoError(t, err)

	// Verify table exists.
	var exists bool
	err = sharedPG.Pool.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_name = '_ayb_migrations')").
		Scan(&exists)
	testutil.NoError(t, err)
	testutil.True(t, exists, "_ayb_migrations table should exist")
}

func TestBootstrapIdempotent(t *testing.T) {
	ctx := context.Background()
	resetDB(t, ctx)

	runner := migrations.NewRunner(sharedPG.Pool, testutil.DiscardLogger())

	// Run bootstrap twice — should not error.
	err := runner.Bootstrap(ctx)
	testutil.NoError(t, err)
	err = runner.Bootstrap(ctx)
	testutil.NoError(t, err)
}

func TestRunMigrations(t *testing.T) {
	ctx := context.Background()
	resetDB(t, ctx)

	runner := migrations.NewRunner(sharedPG.Pool, testutil.DiscardLogger())
	err := runner.Bootstrap(ctx)
	testutil.NoError(t, err)

	// Run migrations.
	applied, err := runner.Run(ctx)
	testutil.NoError(t, err)
	testutil.True(t, applied >= 1, "should apply at least 1 migration")

	// Verify _ayb_meta table was created by 001_ayb_meta.sql.
	var exists bool
	err = sharedPG.Pool.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_name = '_ayb_meta')").
		Scan(&exists)
	testutil.NoError(t, err)
	testutil.True(t, exists, "_ayb_meta table should exist")
}

func TestRunMigrationsIdempotent(t *testing.T) {
	ctx := context.Background()
	resetDB(t, ctx)

	runner := migrations.NewRunner(sharedPG.Pool, testutil.DiscardLogger())
	err := runner.Bootstrap(ctx)
	testutil.NoError(t, err)

	// First run applies migrations.
	applied1, err := runner.Run(ctx)
	testutil.NoError(t, err)

	// Second run should apply zero.
	applied2, err := runner.Run(ctx)
	testutil.NoError(t, err)
	testutil.Equal(t, 0, applied2)

	// First run should have applied at least one.
	testutil.True(t, applied1 >= 1, "first run should apply migrations")
}

func TestAppsTableMigration(t *testing.T) {
	ctx := context.Background()
	resetDB(t, ctx)

	runner := migrations.NewRunner(sharedPG.Pool, testutil.DiscardLogger())
	err := runner.Bootstrap(ctx)
	testutil.NoError(t, err)

	_, err = runner.Run(ctx)
	testutil.NoError(t, err)

	// Verify _ayb_apps table exists.
	var exists bool
	err = sharedPG.Pool.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_name = '_ayb_apps')").
		Scan(&exists)
	testutil.NoError(t, err)
	testutil.True(t, exists, "_ayb_apps table should exist")

	// Verify expected columns exist with correct types.
	type colInfo struct {
		name     string
		dataType string
		nullable bool
	}
	rows, err := sharedPG.Pool.Query(ctx,
		`SELECT column_name, data_type, is_nullable = 'YES'
		 FROM information_schema.columns
		 WHERE table_name = '_ayb_apps'
		 ORDER BY ordinal_position`)
	testutil.NoError(t, err)
	defer rows.Close()

	var cols []colInfo
	for rows.Next() {
		var c colInfo
		err := rows.Scan(&c.name, &c.dataType, &c.nullable)
		testutil.NoError(t, err)
		cols = append(cols, c)
	}
	testutil.NoError(t, rows.Err())

	// Build lookup map for assertions.
	colMap := make(map[string]colInfo)
	for _, c := range cols {
		colMap[c.name] = c
	}

	testutil.True(t, len(colMap) >= 8, "expected at least 8 columns, got %d", len(colMap))

	// Check key columns exist.
	for _, expected := range []string{"id", "name", "description", "owner_user_id", "rate_limit_rps", "rate_limit_window_seconds", "created_at", "updated_at"} {
		_, ok := colMap[expected]
		testutil.True(t, ok, "column %s should exist in _ayb_apps", expected)
	}

	// Verify NOT NULL constraints on required columns.
	testutil.False(t, colMap["name"].nullable, "name should be NOT NULL")
	testutil.False(t, colMap["owner_user_id"].nullable, "owner_user_id should be NOT NULL")
	testutil.False(t, colMap["rate_limit_rps"].nullable, "rate_limit_rps should be NOT NULL")

	// Verify app_id column was added to _ayb_api_keys.
	var appIDExists bool
	err = sharedPG.Pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM information_schema.columns
		 WHERE table_name = '_ayb_api_keys' AND column_name = 'app_id')`,
	).Scan(&appIDExists)
	testutil.NoError(t, err)
	testutil.True(t, appIDExists, "app_id column should exist on _ayb_api_keys")

	// Verify app_id is nullable (null = legacy user-scoped key).
	var appIDNullable bool
	err = sharedPG.Pool.QueryRow(ctx,
		`SELECT is_nullable = 'YES' FROM information_schema.columns
		 WHERE table_name = '_ayb_api_keys' AND column_name = 'app_id'`,
	).Scan(&appIDNullable)
	testutil.NoError(t, err)
	testutil.True(t, appIDNullable, "app_id on _ayb_api_keys should be nullable")

	// Verify owner FK constraint exists.
	var fkExists bool
	err = sharedPG.Pool.QueryRow(ctx,
		`SELECT EXISTS(
			SELECT 1 FROM information_schema.table_constraints tc
			JOIN information_schema.constraint_column_usage ccu ON tc.constraint_name = ccu.constraint_name
			WHERE tc.table_name = '_ayb_apps'
			AND tc.constraint_type = 'FOREIGN KEY'
			AND ccu.table_name = '_ayb_users'
		)`).Scan(&fkExists)
	testutil.NoError(t, err)
	testutil.True(t, fkExists, "_ayb_apps should have FK to _ayb_users")

	// Verify index on owner_user_id.
	var ownerIdxExists bool
	err = sharedPG.Pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM pg_indexes WHERE tablename = '_ayb_apps' AND indexname = 'idx_ayb_apps_owner')`).
		Scan(&ownerIdxExists)
	testutil.NoError(t, err)
	testutil.True(t, ownerIdxExists, "idx_ayb_apps_owner should exist")

	// Verify partial index on api_keys.app_id.
	var appIdIdxExists bool
	err = sharedPG.Pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM pg_indexes WHERE tablename = '_ayb_api_keys' AND indexname = 'idx_ayb_api_keys_app_id')`).
		Scan(&appIdIdxExists)
	testutil.NoError(t, err)
	testutil.True(t, appIdIdxExists, "idx_ayb_api_keys_app_id should exist")
}

func TestAppsTableMigrationIdempotent(t *testing.T) {
	ctx := context.Background()
	resetDB(t, ctx)

	runner := migrations.NewRunner(sharedPG.Pool, testutil.DiscardLogger())
	err := runner.Bootstrap(ctx)
	testutil.NoError(t, err)

	// Run all migrations twice — should not error (CREATE IF NOT EXISTS, ADD COLUMN IF NOT EXISTS).
	_, err = runner.Run(ctx)
	testutil.NoError(t, err)

	applied2, err := runner.Run(ctx)
	testutil.NoError(t, err)
	testutil.Equal(t, 0, applied2)

	// Verify the apps table still works after second run.
	var exists bool
	err = sharedPG.Pool.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_name = '_ayb_apps')").
		Scan(&exists)
	testutil.NoError(t, err)
	testutil.True(t, exists, "_ayb_apps table should still exist after idempotent run")
}

func TestOAuthMigrationsEnforceProviderConstraints(t *testing.T) {
	ctx := context.Background()
	resetDB(t, ctx)

	runner := migrations.NewRunner(sharedPG.Pool, testutil.DiscardLogger())
	err := runner.Bootstrap(ctx)
	testutil.NoError(t, err)

	_, err = runner.Run(ctx)
	testutil.NoError(t, err)

	// Seed minimal user/app required by OAuth FK relationships.
	const (
		userID     = "11111111-1111-1111-1111-111111111111"
		appID      = "22222222-2222-2222-2222-222222222222"
		validCID   = "ayb_cid_0123456789abcdef0123456789abcdef0123456789abcdef"
		validHash  = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
		validCode  = "abcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcd"
		validToken = "fedcbafedcbafedcbafedcbafedcbafedcbafedcbafedcbafedcbafedcba12"
	)

	_, err = sharedPG.Pool.Exec(ctx,
		`INSERT INTO _ayb_users (id, email, password_hash) VALUES ($1, $2, $3)`,
		userID, "oauth-migration@example.com", "hash",
	)
	testutil.NoError(t, err)

	_, err = sharedPG.Pool.Exec(ctx,
		`INSERT INTO _ayb_apps (id, name, owner_user_id) VALUES ($1, $2, $3)`,
		appID, "oauth-migration-app", userID,
	)
	testutil.NoError(t, err)

	// client_id must match ayb_cid_ + 24 random hex bytes.
	_, err = sharedPG.Pool.Exec(ctx,
		`INSERT INTO _ayb_oauth_clients (app_id, client_id, client_secret_hash, name, redirect_uris, scopes, client_type)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		appID, "bad_client_id", validHash, "bad-cid",
		[]string{"https://example.com/callback"}, []string{"readonly"}, "confidential",
	)
	testutil.True(t, err != nil, "invalid client_id format should be rejected")

	// Client scopes must be limited to readonly/readwrite/*.
	_, err = sharedPG.Pool.Exec(ctx,
		`INSERT INTO _ayb_oauth_clients (app_id, client_id, client_secret_hash, name, redirect_uris, scopes, client_type)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		appID, "ayb_cid_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", validHash, "bad-scope",
		[]string{"https://example.com/callback"}, []string{"admin"}, "confidential",
	)
	testutil.True(t, err != nil, "invalid oauth client scope should be rejected")

	// Confidential/public type must align with secret presence.
	_, err = sharedPG.Pool.Exec(ctx,
		`INSERT INTO _ayb_oauth_clients (app_id, client_id, client_secret_hash, name, redirect_uris, scopes, client_type)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		appID, "ayb_cid_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", validHash, "public-with-secret",
		[]string{"https://example.com/callback"}, []string{"readonly"}, "public",
	)
	testutil.True(t, err != nil, "public clients should not allow client_secret_hash")

	_, err = sharedPG.Pool.Exec(ctx,
		`INSERT INTO _ayb_oauth_clients (app_id, client_id, client_secret_hash, name, redirect_uris, scopes, client_type)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		appID, validCID, validHash, "valid-client",
		[]string{"https://example.com/callback"}, []string{"readonly", "readwrite"}, "confidential",
	)
	testutil.NoError(t, err)

	// Authorization codes must enforce PKCE S256-only and valid scopes.
	_, err = sharedPG.Pool.Exec(ctx,
		`INSERT INTO _ayb_oauth_authorization_codes
		 (code_hash, client_id, user_id, redirect_uri, scope, code_challenge, code_challenge_method, state, expires_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW() + INTERVAL '10 minute')`,
		validCode, validCID, userID, "https://example.com/callback", "readonly", "challenge", "plain", "state-1",
	)
	testutil.True(t, err != nil, "plain PKCE method should be rejected")

	_, err = sharedPG.Pool.Exec(ctx,
		`INSERT INTO _ayb_oauth_authorization_codes
		 (code_hash, client_id, user_id, redirect_uri, scope, code_challenge, code_challenge_method, state, expires_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW() + INTERVAL '10 minute')`,
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		validCID, userID, "https://example.com/callback", "admin", "challenge", "S256", "state-2",
	)
	testutil.True(t, err != nil, "invalid authorization code scope should be rejected")

	// Tokens and consents should enforce valid scope values.
	_, err = sharedPG.Pool.Exec(ctx,
		`INSERT INTO _ayb_oauth_tokens
		 (token_hash, token_type, client_id, user_id, scope, grant_id, expires_at)
		 VALUES ($1, 'access', $2, $3, $4, gen_random_uuid(), NOW() + INTERVAL '1 hour')`,
		validToken, validCID, userID, "admin",
	)
	testutil.True(t, err != nil, "invalid token scope should be rejected")

	_, err = sharedPG.Pool.Exec(ctx,
		`INSERT INTO _ayb_oauth_consents (user_id, client_id, scope) VALUES ($1, $2, $3)`,
		userID, validCID, "admin",
	)
	testutil.True(t, err != nil, "invalid consent scope should be rejected")
}

func TestRunMigrationsRollsBackFailedMigration(t *testing.T) {
	ctx := context.Background()
	resetDB(t, ctx)

	customMigrations := fstest.MapFS{
		"sql/001_bad_apps.sql": &fstest.MapFile{
			Data: []byte(`
CREATE TABLE _ayb_apps (
	id UUID PRIMARY KEY
);

CREATE TABLE _ayb_api_keys (
	id UUID PRIMARY KEY
);

ALTER TABLE _ayb_api_keys
	ADD COLUMN app_id UUID REFERENCES _ayb_apps(id);

SELECT definitely_invalid_sql();
`),
		},
	}

	runner := migrations.NewRunnerWithFS(sharedPG.Pool, testutil.DiscardLogger(), customMigrations)
	err := runner.Bootstrap(ctx)
	testutil.NoError(t, err)

	applied, err := runner.Run(ctx)
	testutil.Equal(t, 0, applied)
	testutil.NotNil(t, err)

	var appsExists bool
	err = sharedPG.Pool.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_name = '_ayb_apps')").
		Scan(&appsExists)
	testutil.NoError(t, err)
	testutil.False(t, appsExists, "_ayb_apps should not exist when migration fails in-transaction")

	var apiKeysExists bool
	err = sharedPG.Pool.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_name = '_ayb_api_keys')").
		Scan(&apiKeysExists)
	testutil.NoError(t, err)
	testutil.False(t, apiKeysExists, "_ayb_api_keys should not exist when migration fails in-transaction")

	var appliedCount int
	err = sharedPG.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM _ayb_migrations").Scan(&appliedCount)
	testutil.NoError(t, err)
	testutil.Equal(t, 0, appliedCount)
}

func TestGetApplied(t *testing.T) {
	ctx := context.Background()
	resetDB(t, ctx)

	runner := migrations.NewRunner(sharedPG.Pool, testutil.DiscardLogger())
	err := runner.Bootstrap(ctx)
	testutil.NoError(t, err)

	// Before running, no applied migrations.
	applied, err := runner.GetApplied(ctx)
	testutil.NoError(t, err)
	testutil.SliceLen(t, applied, 0)

	// Run migrations.
	_, err = runner.Run(ctx)
	testutil.NoError(t, err)

	// After running, should have applied migrations.
	applied, err = runner.GetApplied(ctx)
	testutil.NoError(t, err)
	testutil.True(t, len(applied) >= 1, "should have applied migrations")
	testutil.Equal(t, "001_ayb_meta.sql", applied[0].Name)
	testutil.False(t, applied[0].AppliedAt.IsZero(), "applied_at should be set")
}
