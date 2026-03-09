//go:build integration

package fbmigrate

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

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

// bootstrapAYBSchema creates the minimal AYB tables needed by the migrator.
func bootstrapAYBSchema(t *testing.T) {
	t.Helper()
	ctx := context.Background()

	// Reset schema.
	_, err := sharedPG.Pool.Exec(ctx, "DROP SCHEMA IF EXISTS public CASCADE; CREATE SCHEMA public")
	testutil.NoError(t, err)

	// Create _ayb_users table (from 002_ayb_users.sql + 007_ayb_email_verification.sql).
	_, err = sharedPG.Pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS _ayb_users (
			id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			email         TEXT NOT NULL,
			password_hash TEXT NOT NULL,
			email_verified BOOLEAN NOT NULL DEFAULT false,
			created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_ayb_users_email ON _ayb_users (LOWER(email));
	`)
	testutil.NoError(t, err)

	// Create _ayb_oauth_accounts table (from 004_ayb_oauth_accounts.sql).
	_, err = sharedPG.Pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS _ayb_oauth_accounts (
			id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			user_id          UUID NOT NULL REFERENCES _ayb_users(id) ON DELETE CASCADE,
			provider         TEXT NOT NULL,
			provider_user_id TEXT NOT NULL,
			email            TEXT,
			name             TEXT,
			created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			UNIQUE(provider, provider_user_id)
		);
		CREATE INDEX IF NOT EXISTS idx_ayb_oauth_accounts_user_id ON _ayb_oauth_accounts (user_id);
	`)
	testutil.NoError(t, err)
}

// --- Fixture Helpers ---

func createAuthExportFile(t *testing.T, export FirebaseAuthExport) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "auth-export.json")
	data, err := json.MarshalIndent(export, "", "  ")
	testutil.NoError(t, err)
	err = os.WriteFile(path, data, 0644)
	testutil.NoError(t, err)
	return path
}

func createFirestoreExportDir(t *testing.T, collections map[string][]FirestoreDocument) string {
	t.Helper()
	dir := t.TempDir()
	for name, docs := range collections {
		data, err := json.MarshalIndent(docs, "", "  ")
		testutil.NoError(t, err)
		err = os.WriteFile(filepath.Join(dir, name+".json"), data, 0644)
		testutil.NoError(t, err)
	}
	return dir
}

func createRTDBExportFile(t *testing.T, data map[string]any) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "rtdb-export.json")
	raw, err := json.MarshalIndent(data, "", "  ")
	testutil.NoError(t, err)
	err = os.WriteFile(path, raw, 0644)
	testutil.NoError(t, err)
	return path
}

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

func testHashConfig() *FirebaseHashConfig {
	return &FirebaseHashConfig{
		Algorithm:           "SCRYPT",
		Base64SignerKey:     "dGVzdC1zaWduZXIta2V5",
		Base64SaltSeparator: "Bw==",
		Rounds:              8,
		MemCost:             14,
	}
}

// Test UUIDs for Firebase users.
const (
	aliceUID    = "aaaaaaaa-0000-0000-0000-000000000001"
	bobUID      = "aaaaaaaa-0000-0000-0000-000000000002"
	disabledUID = "aaaaaaaa-0000-0000-0000-000000000003"
	phoneUID    = "aaaaaaaa-0000-0000-0000-000000000004"
	carolUID    = "aaaaaaaa-0000-0000-0000-000000000005"
)

func testAuthExport() FirebaseAuthExport {
	return FirebaseAuthExport{
		Users: []FirebaseUser{
			{
				LocalID:       aliceUID,
				Email:         "alice@example.com",
				PasswordHash:  "dGVzdC1oYXNo",
				Salt:          "dGVzdC1zYWx0",
				EmailVerified: true,
				DisplayName:   "Alice Smith",
				CreatedAt:     "1704067200000", // 2024-01-01 00:00:00 UTC
				ProviderInfo: []ProviderInfo{
					{ProviderID: "password", RawID: "alice@example.com", Email: "alice@example.com"},
					{ProviderID: "google.com", RawID: "g-12345", Email: "alice@gmail.com", DisplayName: "Alice S"},
				},
			},
			{
				LocalID:       bobUID,
				Email:         "bob@example.com",
				PasswordHash:  "dGVzdC1oYXNoMg==",
				Salt:          "dGVzdC1zYWx0Mg==",
				EmailVerified: false,
				DisplayName:   "Bob Jones",
				CreatedAt:     "1704153600000", // 2024-01-02 00:00:00 UTC
				ProviderInfo: []ProviderInfo{
					{ProviderID: "password", RawID: "bob@example.com", Email: "bob@example.com"},
				},
			},
			{
				// Disabled user — should be skipped.
				LocalID:       disabledUID,
				Email:         "disabled@example.com",
				PasswordHash:  "dGVzdC1oYXNoMw==",
				Salt:          "dGVzdC1zYWx0Mw==",
				EmailVerified: true,
				Disabled:      true,
				CreatedAt:     "1704240000000",
			},
			{
				// Phone-only user — should be skipped.
				LocalID:   phoneUID,
				CreatedAt: "1704326400000",
				ProviderInfo: []ProviderInfo{
					{ProviderID: "phone", RawID: "+1234567890"},
				},
			},
			{
				// OAuth-only user (no password) — should still be imported.
				LocalID:       carolUID,
				Email:         "carol@example.com",
				EmailVerified: true,
				CreatedAt:     "1704412800000",
				ProviderInfo: []ProviderInfo{
					{ProviderID: "github.com", RawID: "gh-99999", Email: "carol@github.com", DisplayName: "Carol Dev"},
				},
			},
		},
		HashConfig: *testHashConfig(),
	}
}

// --- Tests ---

func TestE2E_FullMigration(t *testing.T) {
	bootstrapAYBSchema(t)

	authExport := testAuthExport()
	authPath := createAuthExportFile(t, authExport)

	firestoreDir := createFirestoreExportDir(t, map[string][]FirestoreDocument{
		"posts": {
			{ID: "doc1", Fields: map[string]any{"title": map[string]any{"stringValue": "Hello World"}}},
			{ID: "doc2", Fields: map[string]any{"title": map[string]any{"stringValue": "Second Post"}}},
		},
		"comments": {
			{ID: "c1", Fields: map[string]any{"text": map[string]any{"stringValue": "Nice!"}, "likes": map[string]any{"integerValue": "5"}}},
		},
	})

	rtdbPath := createRTDBExportFile(t, map[string]any{
		"messages": map[string]any{
			"msg1": map[string]any{"text": "hello", "ts": 1234567890},
			"msg2": map[string]any{"text": "world", "ts": 1234567891},
		},
		"config": map[string]any{
			"theme": map[string]any{"color": "blue"},
		},
	})

	storageDir := createStorageExportDir(t, map[string]map[string][]byte{
		"my-bucket": {
			"images/photo.jpg": []byte("fake-jpeg-data"),
			"docs/readme.txt":  []byte("fake-readme-data"),
		},
	})

	tmpStorage := t.TempDir()

	migrator, err := NewMigrator(MigrationOptions{
		AuthExportPath:      authPath,
		FirestoreExportPath: firestoreDir,
		RTDBExportPath:      rtdbPath,
		StorageExportPath:   storageDir,
		StoragePath:         tmpStorage,
		DatabaseURL:         sharedPG.ConnString,
		HashConfig:          testHashConfig(),
		Verbose:             true,
	})
	testutil.NoError(t, err)
	defer migrator.Close()

	ctx := context.Background()
	stats, err := migrator.Migrate(ctx)
	testutil.NoError(t, err)

	// Auth: alice, bob, carol imported; disabled + phone-only skipped.
	testutil.Equal(t, 3, stats.Users)
	testutil.Equal(t, 2, stats.Skipped) // disabled + phone-only

	// OAuth: google for alice, github for carol.
	testutil.Equal(t, 2, stats.OAuthLinks)

	// Firestore: 2 collections, 3 documents.
	testutil.Equal(t, 2, stats.Collections)
	testutil.Equal(t, 3, stats.Documents)

	// RTDB: 2 nodes, 3 records (2 messages + 1 config).
	testutil.Equal(t, 2, stats.RTDBNodes)
	testutil.Equal(t, 3, stats.RTDBRecords)

	// Storage: 2 files.
	testutil.Equal(t, 2, stats.StorageFiles)
	testutil.True(t, stats.StorageBytes > 0)

	// Verify database state.
	db, err := sql.Open("pgx", sharedPG.ConnString)
	testutil.NoError(t, err)
	defer db.Close()

	// Check users table.
	var userCount int
	err = db.QueryRow("SELECT COUNT(*) FROM _ayb_users").Scan(&userCount)
	testutil.NoError(t, err)
	testutil.Equal(t, 3, userCount)

	// Check Firestore tables.
	var postsCount int
	err = db.QueryRow(`SELECT COUNT(*) FROM "posts"`).Scan(&postsCount)
	testutil.NoError(t, err)
	testutil.Equal(t, 2, postsCount)

	// Check RTDB tables.
	var messagesCount int
	err = db.QueryRow(`SELECT COUNT(*) FROM "messages"`).Scan(&messagesCount)
	testutil.NoError(t, err)
	testutil.Equal(t, 2, messagesCount)

	// Verify storage files.
	verifyFile(t, filepath.Join(tmpStorage, "my-bucket", "images", "photo.jpg"), []byte("fake-jpeg-data"))
	verifyFile(t, filepath.Join(tmpStorage, "my-bucket", "docs", "readme.txt"), []byte("fake-readme-data"))
}

func TestE2E_AuthMigration(t *testing.T) {
	bootstrapAYBSchema(t)

	authExport := testAuthExport()
	authPath := createAuthExportFile(t, authExport)

	migrator, err := NewMigrator(MigrationOptions{
		AuthExportPath: authPath,
		DatabaseURL:    sharedPG.ConnString,
		HashConfig:     testHashConfig(),
		Verbose:        true,
	})
	testutil.NoError(t, err)
	defer migrator.Close()

	ctx := context.Background()
	stats, err := migrator.Migrate(ctx)
	testutil.NoError(t, err)

	testutil.Equal(t, 3, stats.Users)   // alice, bob, carol
	testutil.Equal(t, 2, stats.Skipped) // disabled + phone-only
	testutil.Equal(t, 2, stats.OAuthLinks)

	db, err := sql.Open("pgx", sharedPG.ConnString)
	testutil.NoError(t, err)
	defer db.Close()

	// Verify alice.
	var email, passwordHash string
	var verified bool
	err = db.QueryRow(
		"SELECT email, password_hash, email_verified FROM _ayb_users WHERE id = $1",
		aliceUID,
	).Scan(&email, &passwordHash, &verified)
	testutil.NoError(t, err)
	testutil.Equal(t, "alice@example.com", email)
	testutil.True(t, verified)
	testutil.Contains(t, passwordHash, "$firebase-scrypt$")

	// Verify bob (not verified).
	err = db.QueryRow(
		"SELECT email, email_verified FROM _ayb_users WHERE id = $1",
		bobUID,
	).Scan(&email, &verified)
	testutil.NoError(t, err)
	testutil.Equal(t, "bob@example.com", email)
	testutil.False(t, verified)

	// Verify carol (OAuth-only, no password).
	err = db.QueryRow(
		"SELECT password_hash FROM _ayb_users WHERE id = $1",
		carolUID,
	).Scan(&passwordHash)
	testutil.NoError(t, err)
	testutil.Equal(t, "$none$", passwordHash)

	// Verify disabled user was NOT imported.
	var disabledCount int
	err = db.QueryRow("SELECT COUNT(*) FROM _ayb_users WHERE id = $1", disabledUID).Scan(&disabledCount)
	testutil.NoError(t, err)
	testutil.Equal(t, 0, disabledCount)

	// Verify OAuth accounts.
	var oauthCount int
	err = db.QueryRow("SELECT COUNT(*) FROM _ayb_oauth_accounts").Scan(&oauthCount)
	testutil.NoError(t, err)
	testutil.Equal(t, 2, oauthCount)

	// Verify alice's Google OAuth.
	var provider, providerUserID string
	err = db.QueryRow(
		"SELECT provider, provider_user_id FROM _ayb_oauth_accounts WHERE user_id = $1",
		aliceUID,
	).Scan(&provider, &providerUserID)
	testutil.NoError(t, err)
	testutil.Equal(t, "google", provider)
	testutil.Equal(t, "g-12345", providerUserID)

	// Verify carol's GitHub OAuth.
	err = db.QueryRow(
		"SELECT provider, provider_user_id FROM _ayb_oauth_accounts WHERE user_id = $1",
		carolUID,
	).Scan(&provider, &providerUserID)
	testutil.NoError(t, err)
	testutil.Equal(t, "github", provider)
	testutil.Equal(t, "gh-99999", providerUserID)
}

func TestE2E_FirestoreMigration(t *testing.T) {
	bootstrapAYBSchema(t)

	firestoreDir := createFirestoreExportDir(t, map[string][]FirestoreDocument{
		"users": {
			{ID: "projects/p/databases/(default)/documents/users/u1", Fields: map[string]any{
				"name":  map[string]any{"stringValue": "Alice"},
				"age":   map[string]any{"integerValue": "30"},
				"admin": map[string]any{"booleanValue": true},
			}},
			{ID: "projects/p/databases/(default)/documents/users/u2", Fields: map[string]any{
				"name":  map[string]any{"stringValue": "Bob"},
				"age":   map[string]any{"integerValue": "25"},
				"admin": map[string]any{"booleanValue": false},
			}},
		},
		"orders": {
			{ID: "projects/p/databases/(default)/documents/orders/o1", Fields: map[string]any{
				"total":  map[string]any{"doubleValue": 99.99},
				"items":  map[string]any{"arrayValue": map[string]any{"values": []any{map[string]any{"stringValue": "item1"}, map[string]any{"stringValue": "item2"}}}},
				"status": map[string]any{"stringValue": "shipped"},
			}},
		},
	})

	migrator, err := NewMigrator(MigrationOptions{
		FirestoreExportPath: firestoreDir,
		DatabaseURL:         sharedPG.ConnString,
		Verbose:             true,
	})
	testutil.NoError(t, err)
	defer migrator.Close()

	ctx := context.Background()
	stats, err := migrator.Migrate(ctx)
	testutil.NoError(t, err)

	testutil.Equal(t, 2, stats.Collections)
	testutil.Equal(t, 3, stats.Documents)

	db, err := sql.Open("pgx", sharedPG.ConnString)
	testutil.NoError(t, err)
	defer db.Close()

	// Check users table exists with correct row count.
	var userCount int
	err = db.QueryRow(`SELECT COUNT(*) FROM "users"`).Scan(&userCount)
	testutil.NoError(t, err)
	testutil.Equal(t, 2, userCount)

	// Verify data column contains flattened values.
	var dataJSON string
	err = db.QueryRow(`SELECT data::text FROM "users" WHERE id = 'u1'`).Scan(&dataJSON)
	testutil.NoError(t, err)
	testutil.Contains(t, dataJSON, `"name"`)
	testutil.Contains(t, dataJSON, `"Alice"`)

	// Check orders table.
	var orderCount int
	err = db.QueryRow(`SELECT COUNT(*) FROM "orders"`).Scan(&orderCount)
	testutil.NoError(t, err)
	testutil.Equal(t, 1, orderCount)

	// Verify GIN index exists on data column.
	var indexExists bool
	err = db.QueryRow(`
		SELECT EXISTS (
			SELECT 1 FROM pg_indexes
			WHERE tablename = 'users' AND indexname = 'idx_users_data'
		)
	`).Scan(&indexExists)
	testutil.NoError(t, err)
	testutil.True(t, indexExists)
}

func TestE2E_RTDBMigration(t *testing.T) {
	bootstrapAYBSchema(t)

	rtdbPath := createRTDBExportFile(t, map[string]any{
		"chat_messages": map[string]any{
			"msg1": map[string]any{"text": "hello", "uid": "u1", "ts": 1234567890},
			"msg2": map[string]any{"text": "world", "uid": "u2", "ts": 1234567891},
			"msg3": map[string]any{"text": "foo", "uid": "u1", "ts": 1234567892},
		},
		"settings": map[string]any{
			"theme": map[string]any{"color": "blue", "mode": "dark"},
		},
		// Scalar value — should be stored as _root row.
		"counter": 42,
	})

	migrator, err := NewMigrator(MigrationOptions{
		RTDBExportPath: rtdbPath,
		DatabaseURL:    sharedPG.ConnString,
		Verbose:        true,
	})
	testutil.NoError(t, err)
	defer migrator.Close()

	ctx := context.Background()
	stats, err := migrator.Migrate(ctx)
	testutil.NoError(t, err)

	testutil.Equal(t, 3, stats.RTDBNodes)
	testutil.Equal(t, 5, stats.RTDBRecords) // 3 messages + 1 setting + 1 scalar

	db, err := sql.Open("pgx", sharedPG.ConnString)
	testutil.NoError(t, err)
	defer db.Close()

	// Check chat_messages table.
	var msgCount int
	err = db.QueryRow(`SELECT COUNT(*) FROM "chat_messages"`).Scan(&msgCount)
	testutil.NoError(t, err)
	testutil.Equal(t, 3, msgCount)

	// Verify data content.
	var dataJSON string
	err = db.QueryRow(`SELECT data::text FROM "chat_messages" WHERE id = 'msg1'`).Scan(&dataJSON)
	testutil.NoError(t, err)
	testutil.Contains(t, dataJSON, `"hello"`)

	// Check settings table.
	var settingsCount int
	err = db.QueryRow(`SELECT COUNT(*) FROM "settings"`).Scan(&settingsCount)
	testutil.NoError(t, err)
	testutil.Equal(t, 1, settingsCount)

	// Check counter table (scalar stored as _root).
	var counterCount int
	err = db.QueryRow(`SELECT COUNT(*) FROM "counter"`).Scan(&counterCount)
	testutil.NoError(t, err)
	testutil.Equal(t, 1, counterCount)

	var counterID string
	err = db.QueryRow(`SELECT id FROM "counter"`).Scan(&counterID)
	testutil.NoError(t, err)
	testutil.Equal(t, "_root", counterID)

	// Verify GIN index exists.
	var indexExists bool
	err = db.QueryRow(`
		SELECT EXISTS (
			SELECT 1 FROM pg_indexes
			WHERE tablename = 'chat_messages' AND indexname = 'idx_chat_messages_data'
		)
	`).Scan(&indexExists)
	testutil.NoError(t, err)
	testutil.True(t, indexExists)
}

func TestE2E_StorageMigration(t *testing.T) {
	bootstrapAYBSchema(t)

	storageDir := createStorageExportDir(t, map[string]map[string][]byte{
		"user-uploads": {
			"images/avatar.png":    []byte("fake-png-data"),
			"images/banner.jpg":    []byte("fake-jpg-data"),
			"documents/report.pdf": []byte("fake-pdf-data"),
		},
		"public-assets": {
			"logo.svg": []byte("<svg>test</svg>"),
		},
	})

	tmpStorage := t.TempDir()

	// Use Firestore as a minimal DB phase to satisfy the "at least one export" check,
	// but the real test focus is storage.
	firestoreDir := createFirestoreExportDir(t, map[string][]FirestoreDocument{
		"dummy": {{ID: "d1", Fields: map[string]any{"x": map[string]any{"stringValue": "y"}}}},
	})

	migrator, err := NewMigrator(MigrationOptions{
		FirestoreExportPath: firestoreDir,
		StorageExportPath:   storageDir,
		StoragePath:         tmpStorage,
		DatabaseURL:         sharedPG.ConnString,
		Verbose:             true,
	})
	testutil.NoError(t, err)
	defer migrator.Close()

	ctx := context.Background()
	stats, err := migrator.Migrate(ctx)
	testutil.NoError(t, err)

	testutil.Equal(t, 4, stats.StorageFiles)
	testutil.True(t, stats.StorageBytes > 0)

	// Verify all files were copied with correct content.
	verifyFile(t, filepath.Join(tmpStorage, "user-uploads", "images", "avatar.png"), []byte("fake-png-data"))
	verifyFile(t, filepath.Join(tmpStorage, "user-uploads", "images", "banner.jpg"), []byte("fake-jpg-data"))
	verifyFile(t, filepath.Join(tmpStorage, "user-uploads", "documents", "report.pdf"), []byte("fake-pdf-data"))
	verifyFile(t, filepath.Join(tmpStorage, "public-assets", "logo.svg"), []byte("<svg>test</svg>"))
}

func TestE2E_DryRun(t *testing.T) {
	bootstrapAYBSchema(t)

	authExport := testAuthExport()
	authPath := createAuthExportFile(t, authExport)

	firestoreDir := createFirestoreExportDir(t, map[string][]FirestoreDocument{
		"posts": {{ID: "d1", Fields: map[string]any{"title": map[string]any{"stringValue": "test"}}}},
	})

	storageDir := createStorageExportDir(t, map[string]map[string][]byte{
		"bucket": {"file.txt": []byte("data")},
	})

	tmpStorage := t.TempDir()

	migrator, err := NewMigrator(MigrationOptions{
		AuthExportPath:      authPath,
		FirestoreExportPath: firestoreDir,
		StorageExportPath:   storageDir,
		StoragePath:         tmpStorage,
		DatabaseURL:         sharedPG.ConnString,
		HashConfig:          testHashConfig(),
		DryRun:              true,
		Verbose:             true,
	})
	testutil.NoError(t, err)
	defer migrator.Close()

	ctx := context.Background()
	stats, err := migrator.Migrate(ctx)
	testutil.NoError(t, err)

	// Stats should still be populated.
	testutil.Equal(t, 3, stats.Users)
	testutil.Equal(t, 2, stats.OAuthLinks)
	testutil.Equal(t, 1, stats.Collections)
	testutil.Equal(t, 1, stats.Documents)

	// But database should have no user data (transaction was rolled back).
	db, err := sql.Open("pgx", sharedPG.ConnString)
	testutil.NoError(t, err)
	defer db.Close()

	var userCount int
	err = db.QueryRow("SELECT COUNT(*) FROM _ayb_users").Scan(&userCount)
	testutil.NoError(t, err)
	testutil.Equal(t, 0, userCount)

	// Firestore table should not exist.
	var tableExists bool
	err = db.QueryRow(`
		SELECT EXISTS (
			SELECT 1 FROM information_schema.tables
			WHERE table_schema = 'public' AND table_name = 'posts'
		)
	`).Scan(&tableExists)
	testutil.NoError(t, err)
	testutil.False(t, tableExists)

	// Storage files should NOT be copied in dry-run mode.
	testutil.Equal(t, 0, stats.StorageFiles)
	entries, err := os.ReadDir(tmpStorage)
	testutil.NoError(t, err)
	testutil.Equal(t, 0, len(entries))
}

func TestE2E_Analyze(t *testing.T) {
	bootstrapAYBSchema(t)

	authExport := testAuthExport()
	authPath := createAuthExportFile(t, authExport)

	firestoreDir := createFirestoreExportDir(t, map[string][]FirestoreDocument{
		"posts":    {{ID: "d1", Fields: map[string]any{"x": map[string]any{"stringValue": "y"}}}},
		"comments": {{ID: "c1", Fields: map[string]any{"x": map[string]any{"stringValue": "y"}}}, {ID: "c2", Fields: map[string]any{"x": map[string]any{"stringValue": "z"}}}},
	})

	rtdbPath := createRTDBExportFile(t, map[string]any{
		"chat": map[string]any{
			"m1": map[string]any{"text": "hi"},
		},
	})

	storageDir := createStorageExportDir(t, map[string]map[string][]byte{
		"bucket": {
			"a.txt": []byte("aaa"),
			"b.txt": []byte("bbb"),
		},
	})

	migrator, err := NewMigrator(MigrationOptions{
		AuthExportPath:      authPath,
		FirestoreExportPath: firestoreDir,
		RTDBExportPath:      rtdbPath,
		StorageExportPath:   storageDir,
		DatabaseURL:         sharedPG.ConnString,
		HashConfig:          testHashConfig(),
	})
	testutil.NoError(t, err)
	defer migrator.Close()

	ctx := context.Background()
	report, err := migrator.Analyze(ctx)
	testutil.NoError(t, err)

	testutil.Equal(t, "Firebase", report.SourceType)
	testutil.Equal(t, 3, report.AuthUsers)  // alice, bob, carol (disabled + phone skipped)
	testutil.Equal(t, 2, report.OAuthLinks) // google for alice, github for carol
	testutil.Equal(t, 3, report.Tables)     // 2 Firestore + 1 RTDB
	testutil.Equal(t, 4, report.Records)    // 3 Firestore docs + 1 RTDB child
	testutil.Equal(t, 2, report.Files)
	testutil.True(t, report.FileSizeBytes > 0)
}

func TestE2E_NonUUIDLocalIDs(t *testing.T) {
	bootstrapAYBSchema(t)

	// Firebase LocalIDs are typically 28-char alphanumeric strings, not UUIDs.
	authExport := FirebaseAuthExport{
		Users: []FirebaseUser{
			{
				LocalID:       "abc123def456ghi789jkl012mn", // non-UUID
				Email:         "firebase-user@example.com",
				PasswordHash:  "dGVzdC1oYXNo",
				Salt:          "dGVzdC1zYWx0",
				EmailVerified: true,
				CreatedAt:     "1704067200000",
				ProviderInfo: []ProviderInfo{
					{ProviderID: "password", RawID: "firebase-user@example.com"},
					{ProviderID: "google.com", RawID: "g-firebase-1", Email: "fb@gmail.com"},
				},
			},
			{
				LocalID:       "xyz789abc123def456ghi012mn", // another non-UUID
				Email:         "firebase-user2@example.com",
				PasswordHash:  "dGVzdC1oYXNoMg==",
				Salt:          "dGVzdC1zYWx0Mg==",
				EmailVerified: false,
				CreatedAt:     "1704153600000",
				ProviderInfo: []ProviderInfo{
					{ProviderID: "password", RawID: "firebase-user2@example.com"},
				},
			},
		},
		HashConfig: *testHashConfig(),
	}
	authPath := createAuthExportFile(t, authExport)

	migrator, err := NewMigrator(MigrationOptions{
		AuthExportPath: authPath,
		DatabaseURL:    sharedPG.ConnString,
		HashConfig:     testHashConfig(),
		Verbose:        true,
	})
	testutil.NoError(t, err)
	defer migrator.Close()

	ctx := context.Background()
	stats, err := migrator.Migrate(ctx)
	testutil.NoError(t, err)

	testutil.Equal(t, 2, stats.Users)
	testutil.Equal(t, 1, stats.OAuthLinks) // google for firebase-user

	db, err := sql.Open("pgx", sharedPG.ConnString)
	testutil.NoError(t, err)
	defer db.Close()

	// Both users should be inserted with valid UUIDs (not their Firebase IDs).
	var userCount int
	err = db.QueryRow("SELECT COUNT(*) FROM _ayb_users").Scan(&userCount)
	testutil.NoError(t, err)
	testutil.Equal(t, 2, userCount)

	// Verify the generated UUID is deterministic (same as FirebaseIDToUUID).
	expectedUUID := FirebaseIDToUUID("abc123def456ghi789jkl012mn")
	var email string
	err = db.QueryRow(
		"SELECT email FROM _ayb_users WHERE id = $1",
		expectedUUID,
	).Scan(&email)
	testutil.NoError(t, err)
	testutil.Equal(t, "firebase-user@example.com", email)

	// Verify OAuth account references the generated UUID.
	var oauthUserID string
	err = db.QueryRow(
		"SELECT user_id FROM _ayb_oauth_accounts WHERE provider = 'google'",
	).Scan(&oauthUserID)
	testutil.NoError(t, err)
	testutil.Equal(t, expectedUUID, oauthUserID)
}

// --- Helpers ---

func verifyFile(t *testing.T, path string, expected []byte) {
	t.Helper()
	content, err := os.ReadFile(path)
	testutil.NoError(t, err)
	testutil.Equal(t, string(expected), string(content))
}
