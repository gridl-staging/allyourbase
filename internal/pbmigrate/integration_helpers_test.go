//go:build integration

package pbmigrate

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func createPocketBaseFixture(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()

	// Create pb_data directory
	pbDataPath := filepath.Join(tmpDir, "pb_data")
	err := os.MkdirAll(pbDataPath, 0755)
	testutil.NoError(t, err)

	// Create data.db (SQLite database)
	dbPath := filepath.Join(pbDataPath, "data.db")
	db, err := sql.Open("sqlite3", dbPath)
	testutil.NoError(t, err)
	defer db.Close()

	// Create _collections table
	_, err = db.Exec(`
		CREATE TABLE _collections (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			type TEXT NOT NULL,
			system INTEGER NOT NULL,
			schema TEXT NOT NULL,
			indexes TEXT,
			listRule TEXT,
			viewRule TEXT,
			createRule TEXT,
			updateRule TEXT,
			deleteRule TEXT,
			options TEXT,
			created TEXT,
			updated TEXT
		)
	`)
	testutil.NoError(t, err)

	// Insert collections
	insertCollection(t, db, PBCollection{
		ID:     "posts123",
		Name:   "posts",
		Type:   "base",
		System: false,
		Schema: []PBField{
			{Name: "title", Type: "text", Required: true},
			{Name: "body", Type: "editor", Required: false},
			{Name: "image", Type: "file", Required: false},
			{Name: "published", Type: "bool", Required: false},
		},
		ListRule:   stringPtr(""),
		ViewRule:   stringPtr(""),
		CreateRule: stringPtr("@request.auth.id != ''"),
		UpdateRule: stringPtr("@request.auth.id != ''"),
		DeleteRule: stringPtr("@request.auth.id != ''"),
	})

	insertCollection(t, db, PBCollection{
		ID:     "users123",
		Name:   "users",
		Type:   "auth",
		System: false,
		Schema: []PBField{
			{Name: "email", Type: "email", Required: true, System: true},
			{Name: "passwordHash", Type: "text", Required: true, System: true},
			{Name: "verified", Type: "bool", Required: false, System: true},
		},
		ListRule:   stringPtr(""),
		ViewRule:   stringPtr(""),
		CreateRule: stringPtr(""),
		UpdateRule: stringPtr("id = @request.auth.id"),
		DeleteRule: nil,
	})

	insertCollection(t, db, PBCollection{
		ID:     "comments123",
		Name:   "comments",
		Type:   "base",
		System: false,
		Schema: []PBField{
			{Name: "text", Type: "text", Required: true},
			{Name: "post", Type: "relation", Required: true},
		},
		ListRule:   stringPtr(""),
		ViewRule:   stringPtr(""),
		CreateRule: stringPtr(""),
		UpdateRule: nil,
		DeleteRule: nil,
	})

	insertCollection(t, db, PBCollection{
		ID:     "stats123",
		Name:   "stats_view",
		Type:   "view",
		System: false,
		Schema: []PBField{
			{Name: "count", Type: "number", Required: false},
		},
		ViewQuery: "SELECT COUNT(*) as count FROM posts",
	})

	// Create posts table
	_, err = db.Exec(`
		CREATE TABLE posts (
			id TEXT PRIMARY KEY,
			created TEXT,
			updated TEXT,
			title TEXT,
			body TEXT,
			image TEXT,
			published INTEGER
		)
	`)
	testutil.NoError(t, err)

	// Insert posts
	_, err = db.Exec(`
		INSERT INTO posts (id, created, updated, title, body, image, published)
		VALUES
			('post1', '2024-01-01 00:00:00.000Z', '2024-01-01 00:00:00.000Z', 'First Post', 'Hello world', 'image1.jpg', 1),
			('post2', '2024-01-02 00:00:00.000Z', '2024-01-02 00:00:00.000Z', 'Second Post', 'More content', 'image2.png', 1),
			('post3', '2024-01-03 00:00:00.000Z', '2024-01-03 00:00:00.000Z', 'Draft', 'Draft content', '', 0)
	`)
	testutil.NoError(t, err)

	// Create comments table
	_, err = db.Exec(`
		CREATE TABLE comments (
			id TEXT PRIMARY KEY,
			created TEXT,
			updated TEXT,
			text TEXT,
			post TEXT
		)
	`)
	testutil.NoError(t, err)

	// Insert comments
	_, err = db.Exec(`
		INSERT INTO comments (id, created, updated, text, post)
		VALUES
			('comment1', '2024-01-01 01:00:00.000Z', '2024-01-01 01:00:00.000Z', 'Great post!', 'post1'),
			('comment2', '2024-01-02 01:00:00.000Z', '2024-01-02 01:00:00.000Z', 'Nice!', 'post2')
	`)
	testutil.NoError(t, err)

	// Create users table (auth collection)
	_, err = db.Exec(`
		CREATE TABLE users (
			id TEXT PRIMARY KEY,
			created TEXT,
			updated TEXT,
			email TEXT,
			passwordHash TEXT,
			verified INTEGER
		)
	`)
	testutil.NoError(t, err)

	// Insert users
	_, err = db.Exec(`
		INSERT INTO users (id, created, updated, email, passwordHash, verified)
		VALUES
			('user1', '2024-01-01 00:00:00.000Z', '2024-01-01 00:00:00.000Z', 'user@example.com', '$2a$10$hashedpassword', 1)
	`)
	testutil.NoError(t, err)

	// Create storage directory with files
	storagePath := filepath.Join(pbDataPath, "storage", "posts")
	err = os.MkdirAll(storagePath, 0755)
	testutil.NoError(t, err)

	err = os.WriteFile(filepath.Join(storagePath, "image1.jpg"), []byte("fake-jpeg-data"), 0644)
	testutil.NoError(t, err)

	err = os.WriteFile(filepath.Join(storagePath, "image2.png"), []byte("fake-png-data"), 0644)
	testutil.NoError(t, err)

	return pbDataPath
}

func createPocketBaseWithIndexes(t *testing.T) string {
	return createPocketBaseWithIndexStatements(t, []string{
		"CREATE INDEX idx_posts_title ON posts (title)",
		"CREATE INDEX idx_posts_title_updated ON posts (title, updated DESC)",
		"CREATE UNIQUE INDEX idx_posts_title_unique ON posts (title)",
	})
}

func createPocketBaseWithUnsupportedIndex(t *testing.T) string {
	return createPocketBaseWithIndexStatements(t, []string{
		"CREATE INDEX idx_posts_title_nocase ON posts (title COLLATE NOCASE)",
	})
}

func createPocketBaseWithIndexStatements(t *testing.T, indexes []string) string {
	t.Helper()

	tmpDir := t.TempDir()
	pbDataPath := filepath.Join(tmpDir, "pb_data")
	err := os.MkdirAll(pbDataPath, 0755)
	testutil.NoError(t, err)

	dbPath := filepath.Join(pbDataPath, "data.db")
	db, err := sql.Open("sqlite3", dbPath)
	testutil.NoError(t, err)
	defer db.Close()

	_, err = db.Exec(`
		CREATE TABLE _collections (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			type TEXT NOT NULL,
			system INTEGER NOT NULL,
			schema TEXT NOT NULL,
			indexes TEXT,
			listRule TEXT,
			viewRule TEXT,
			createRule TEXT,
			updateRule TEXT,
			deleteRule TEXT,
			options TEXT,
			created TEXT,
			updated TEXT
		)
	`)
	testutil.NoError(t, err)

	collSchema := []PBField{
		{Name: "title", Type: "text", Required: true},
	}
	schemaJSON, err := json.Marshal(collSchema)
	testutil.NoError(t, err)

	indexesJSON, err := json.Marshal(indexes)
	testutil.NoError(t, err)

	_, err = db.Exec(`
		INSERT INTO _collections (id, name, type, system, schema, indexes, options, created, updated)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, "posts123", "posts", "base", 0, string(schemaJSON), string(indexesJSON), "{}", "2024-01-01 00:00:00.000Z", "2024-01-01 00:00:00.000Z")
	testutil.NoError(t, err)

	_, err = db.Exec(`
		CREATE TABLE posts (
			id TEXT PRIMARY KEY,
			created TEXT,
			updated TEXT,
			title TEXT
		)
	`)
	testutil.NoError(t, err)

	_, err = db.Exec(`
		INSERT INTO posts (id, created, updated, title)
		VALUES ('post1', '2024-01-01 00:00:00.000Z', '2024-01-01 00:00:00.000Z', 'Hello')
	`)
	testutil.NoError(t, err)

	return pbDataPath
}

func createPocketBaseWithAuthUsers(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()

	pbDataPath := filepath.Join(tmpDir, "pb_data")
	err := os.MkdirAll(pbDataPath, 0755)
	testutil.NoError(t, err)

	dbPath := filepath.Join(pbDataPath, "data.db")
	db, err := sql.Open("sqlite3", dbPath)
	testutil.NoError(t, err)
	defer db.Close()

	// Create _collections table
	_, err = db.Exec(`
		CREATE TABLE _collections (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			type TEXT NOT NULL,
			system INTEGER NOT NULL,
			schema TEXT NOT NULL,
			indexes TEXT,
			listRule TEXT,
			viewRule TEXT,
			createRule TEXT,
			updateRule TEXT,
			deleteRule TEXT,
			options TEXT,
			created TEXT,
			updated TEXT
		)
	`)
	testutil.NoError(t, err)

	// Insert auth collection with custom fields
	insertCollection(t, db, PBCollection{
		ID:     "users123",
		Name:   "users",
		Type:   "auth",
		System: false,
		Schema: []PBField{
			{Name: "email", Type: "email", Required: true, System: true},
			{Name: "passwordHash", Type: "text", Required: true, System: true},
			{Name: "verified", Type: "bool", Required: false, System: true},
			{Name: "name", Type: "text", Required: true, System: false},
			{Name: "role", Type: "select", Required: false, System: false},
			{Name: "avatar", Type: "file", Required: false, System: false},
		},
	})

	// Create users table
	_, err = db.Exec(`
		CREATE TABLE users (
			id TEXT PRIMARY KEY,
			created TEXT,
			updated TEXT,
			email TEXT,
			passwordHash TEXT,
			verified INTEGER,
			name TEXT,
			role TEXT,
			avatar TEXT
		)
	`)
	testutil.NoError(t, err)

	// Insert users
	_, err = db.Exec(`
		INSERT INTO users (id, created, updated, email, passwordHash, verified, name, role, avatar)
		VALUES
			('user1abc', '2024-01-01 00:00:00.000Z', '2024-01-01 00:00:00.000Z', 'alice@example.com', '$2a$10$hashedpassword1', 1, 'Alice Smith', 'admin', 'avatar1.jpg'),
			('user2abc', '2024-01-02 00:00:00.000Z', '2024-01-02 00:00:00.000Z', 'bob@example.com', '$2a$10$hashedpassword2', 0, 'Bob Jones', 'user', ''),
			('user3abc', '2024-01-03 00:00:00.000Z', '2024-01-03 00:00:00.000Z', 'carol@example.com', '$2a$10$hashedpassword3', 1, 'Carol White', 'moderator', 'avatar3.png')
	`)
	testutil.NoError(t, err)

	return pbDataPath
}

func createPocketBaseWithFiles(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()

	pbDataPath := filepath.Join(tmpDir, "pb_data")
	err := os.MkdirAll(pbDataPath, 0755)
	testutil.NoError(t, err)

	dbPath := filepath.Join(pbDataPath, "data.db")
	db, err := sql.Open("sqlite3", dbPath)
	testutil.NoError(t, err)
	defer db.Close()

	// Create _collections table
	_, err = db.Exec(`
		CREATE TABLE _collections (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			type TEXT NOT NULL,
			system INTEGER NOT NULL,
			schema TEXT NOT NULL,
			indexes TEXT,
			listRule TEXT,
			viewRule TEXT,
			createRule TEXT,
			updateRule TEXT,
			deleteRule TEXT,
			options TEXT,
			created TEXT,
			updated TEXT
		)
	`)
	testutil.NoError(t, err)

	// Insert collection with file field
	insertCollection(t, db, PBCollection{
		ID:     "posts123",
		Name:   "posts",
		Type:   "base",
		System: false,
		Schema: []PBField{
			{Name: "title", Type: "text", Required: true},
			{Name: "image", Type: "file", Required: false},
		},
	})

	// Create posts table
	_, err = db.Exec(`
		CREATE TABLE posts (
			id TEXT PRIMARY KEY,
			created TEXT,
			updated TEXT,
			title TEXT,
			image TEXT
		)
	`)
	testutil.NoError(t, err)

	_, err = db.Exec(`
		INSERT INTO posts (id, created, updated, title, image)
		VALUES
			('post1', '2024-01-01 00:00:00.000Z', '2024-01-01 00:00:00.000Z', 'Post 1', 'image1.jpg'),
			('post2', '2024-01-02 00:00:00.000Z', '2024-01-02 00:00:00.000Z', 'Post 2', 'image2.png')
	`)
	testutil.NoError(t, err)

	// Create storage with files
	storagePath := filepath.Join(pbDataPath, "storage", "posts")
	err = os.MkdirAll(filepath.Join(storagePath, "nested"), 0755)
	testutil.NoError(t, err)

	err = os.WriteFile(filepath.Join(storagePath, "image1.jpg"), []byte("fake-jpeg-data"), 0644)
	testutil.NoError(t, err)

	err = os.WriteFile(filepath.Join(storagePath, "image2.png"), []byte("fake-png-data"), 0644)
	testutil.NoError(t, err)

	err = os.WriteFile(filepath.Join(storagePath, "nested", "doc.pdf"), []byte("fake-pdf-data"), 0644)
	testutil.NoError(t, err)

	return pbDataPath
}

func insertCollection(t *testing.T, db *sql.DB, coll PBCollection) {
	t.Helper()

	schemaJSON, err := json.Marshal(coll.Schema)
	testutil.NoError(t, err)

	var listRule, viewRule, createRule, updateRule, deleteRule interface{}
	if coll.ListRule != nil {
		listRule = *coll.ListRule
	}
	if coll.ViewRule != nil {
		viewRule = *coll.ViewRule
	}
	if coll.CreateRule != nil {
		createRule = *coll.CreateRule
	}
	if coll.UpdateRule != nil {
		updateRule = *coll.UpdateRule
	}
	if coll.DeleteRule != nil {
		deleteRule = *coll.DeleteRule
	}

	var optionsJSON []byte
	if coll.Type == "view" {
		optionsJSON, _ = json.Marshal(map[string]interface{}{
			"query": coll.ViewQuery,
		})
	} else {
		optionsJSON = []byte("{}")
	}

	_, err = db.Exec(`
		INSERT INTO _collections (id, name, type, system, schema, listRule, viewRule, createRule, updateRule, deleteRule, options, created, updated)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, coll.ID, coll.Name, coll.Type, boolToInt(coll.System), string(schemaJSON), listRule, viewRule, createRule, updateRule, deleteRule, string(optionsJSON), "2024-01-01 00:00:00.000Z", "2024-01-01 00:00:00.000Z")
	testutil.NoError(t, err)
}

func createTestDatabase(t *testing.T, name string) string {
	t.Helper()

	// Use shared PostgreSQL container and reset schema
	ctx := context.Background()
	_, err := sharedPG.Pool.Exec(ctx, "DROP SCHEMA IF EXISTS public CASCADE; CREATE SCHEMA public")
	testutil.NoError(t, err)

	// Return connection string
	return sharedPG.ConnString
}

func dropTestDatabase(t *testing.T, dbURL, name string) {
	t.Helper()
	// No-op, TestMain handles cleanup
}

func verifyIndexExists(t *testing.T, dbURL, indexName string, expectUnique bool) {
	t.Helper()

	db, err := sql.Open("pgx", dbURL)
	testutil.NoError(t, err)
	defer db.Close()

	var exists bool
	err = db.QueryRow(`
		SELECT to_regclass('public.' || $1) IS NOT NULL
	`, indexName).Scan(&exists)
	testutil.NoError(t, err)
	testutil.True(t, exists)

	var isUnique bool
	err = db.QueryRow(`
		SELECT i.indisunique
		FROM pg_class c
		INNER JOIN pg_index i ON i.indexrelid = c.oid
		INNER JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = 'public' AND c.relname = $1
	`, indexName).Scan(&isUnique)
	testutil.NoError(t, err)
	testutil.Equal(t, expectUnique, isUnique)
}

func verifySchemaCreated(t *testing.T, dbURL string) {
	t.Helper()

	db, err := sql.Open("pgx", dbURL)
	testutil.NoError(t, err)
	defer db.Close()

	// Check posts table exists
	var exists bool
	err = db.QueryRow(`
		SELECT EXISTS (
			SELECT 1 FROM information_schema.tables
			WHERE table_name = 'posts'
		)
	`).Scan(&exists)
	testutil.NoError(t, err)
	testutil.True(t, exists)

	// Check comments table exists
	err = db.QueryRow(`
		SELECT EXISTS (
			SELECT 1 FROM information_schema.tables
			WHERE table_name = 'comments'
		)
	`).Scan(&exists)
	testutil.NoError(t, err)
	testutil.True(t, exists)

	// Check stats_view exists
	err = db.QueryRow(`
		SELECT EXISTS (
			SELECT 1 FROM information_schema.views
			WHERE table_name = 'stats_view'
		)
	`).Scan(&exists)
	testutil.NoError(t, err)
	testutil.True(t, exists)

	// Check _ayb_users table exists
	err = db.QueryRow(`
		SELECT EXISTS (
			SELECT 1 FROM information_schema.tables
			WHERE table_name = '_ayb_users'
		)
	`).Scan(&exists)
	testutil.NoError(t, err)
	testutil.True(t, exists)
}

func verifyDataMigrated(t *testing.T, dbURL string) {
	t.Helper()

	db, err := sql.Open("pgx", dbURL)
	testutil.NoError(t, err)
	defer db.Close()

	// Check posts
	var postCount int
	err = db.QueryRow("SELECT COUNT(*) FROM posts").Scan(&postCount)
	testutil.NoError(t, err)
	testutil.Equal(t, 3, postCount)

	// Check specific post
	var title, body string
	var published bool
	err = db.QueryRow("SELECT title, body, published FROM posts WHERE id = $1", "post1").
		Scan(&title, &body, &published)
	testutil.NoError(t, err)
	testutil.Equal(t, "First Post", title)
	testutil.Equal(t, "Hello world", body)
	testutil.True(t, published)

	// Check comments
	var commentCount int
	err = db.QueryRow("SELECT COUNT(*) FROM comments").Scan(&commentCount)
	testutil.NoError(t, err)
	testutil.Equal(t, 2, commentCount)
}

func verifyAuthUsersMigrated(t *testing.T, dbURL string) {
	t.Helper()

	db, err := sql.Open("pgx", dbURL)
	testutil.NoError(t, err)
	defer db.Close()

	// Check users were migrated (fixture has exactly 1 user)
	var userCount int
	err = db.QueryRow("SELECT COUNT(*) FROM _ayb_users").Scan(&userCount)
	testutil.NoError(t, err)
	testutil.Equal(t, 1, userCount)

	// Check ID mapping
	var mapCount int
	err = db.QueryRow("SELECT COUNT(*) FROM _ayb_pb_id_map").Scan(&mapCount)
	testutil.NoError(t, err)
	testutil.Equal(t, userCount, mapCount)

	// Verify user data
	var email, passwordHash string
	var verified bool
	err = db.QueryRow("SELECT email, password_hash, email_verified FROM _ayb_users WHERE email = $1", "user@example.com").
		Scan(&email, &passwordHash, &verified)
	testutil.NoError(t, err)
	testutil.Equal(t, "user@example.com", email)
	testutil.NotEqual(t, "", passwordHash)
	testutil.True(t, verified)
}

func verifyFilesCopied(t *testing.T, storagePath string) {
	t.Helper()

	// Check files exist
	verifyFile(t, filepath.Join(storagePath, "posts", "image1.jpg"), []byte("fake-jpeg-data"))
	verifyFile(t, filepath.Join(storagePath, "posts", "image2.png"), []byte("fake-png-data"))
}

func verifyFile(t *testing.T, path string, expectedContent []byte) {
	t.Helper()

	content, err := os.ReadFile(path)
	testutil.NoError(t, err)
	testutil.Equal(t, string(expectedContent), string(content))
}

func verifyRLSPolicies(t *testing.T, dbURL string) {
	t.Helper()

	db, err := sql.Open("pgx", dbURL)
	testutil.NoError(t, err)
	defer db.Close()

	// Check RLS is enabled on posts
	var rlsEnabled bool
	err = db.QueryRow(`
		SELECT relrowsecurity
		FROM pg_class
		WHERE relname = 'posts'
	`).Scan(&rlsEnabled)
	testutil.NoError(t, err)
	testutil.True(t, rlsEnabled)

	// Check policies exist
	var policyCount int
	err = db.QueryRow(`
		SELECT COUNT(*)
		FROM pg_policies
		WHERE tablename = 'posts'
	`).Scan(&policyCount)
	testutil.NoError(t, err)
	testutil.True(t, policyCount >= 3) // At least SELECT, INSERT, UPDATE policies
}

func stringPtr(s string) *string {
	return &s
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// removeFixtureFile deletes a file from the fixture's storage directory to
// simulate a missing source file during migration.
func removeFixtureFile(t *testing.T, pbDataPath, collName, fileName string) {
	t.Helper()
	p := filepath.Join(pbDataPath, "storage", collName, fileName)
	err := os.Remove(p)
	testutil.NoError(t, err)
}

// makeFixtureFileUnreadable sets a fixture file's permissions to 0 so that
// copyFile fails with a permission error. This triggers recordFileCopyFailure
// while still allowing filepath.Walk to visit the entry.
func makeFixtureFileUnreadable(t *testing.T, pbDataPath, collName, fileName string) {
	t.Helper()
	p := filepath.Join(pbDataPath, "storage", collName, fileName)
	err := os.Chmod(p, 0)
	testutil.NoError(t, err)
}
