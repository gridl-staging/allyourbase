package pbmigrate

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestClose(t *testing.T) {
	t.Parallel()
	t.Run("close migrator", func(t *testing.T) {
		// Create temporary PocketBase directory
		t.Parallel()

		tmpDir := t.TempDir()
		pbDataPath := filepath.Join(tmpDir, "pb_data")
		err := os.MkdirAll(pbDataPath, 0755)
		testutil.NoError(t, err)

		// Create empty data.db file
		dbPath := filepath.Join(pbDataPath, "data.db")
		err = os.WriteFile(dbPath, []byte{}, 0644)
		testutil.NoError(t, err)

		// Create reader
		reader, err := NewReader(pbDataPath)
		testutil.NoError(t, err)

		// Create migrator without database connection
		m := &Migrator{
			reader: reader,
			db:     nil,
		}

		// Close should not error even with nil db
		err = m.Close()
		testutil.NoError(t, err)
	})

	t.Run("close with nil reader", func(t *testing.T) {
		t.Parallel()
		m := &Migrator{
			reader: nil,
			db:     nil,
		}

		err := m.Close()
		testutil.NoError(t, err)
	})
}

func TestPrintStats(t *testing.T) {
	t.Parallel()
	t.Run("print all stats", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer

		m := &Migrator{
			output: &buf,
			stats: MigrationStats{
				Collections: 5,
				Tables:      3,
				Views:       1,
				Records:     100,
				Files:       25,
				Policies:    6,
			},
			opts: MigrationOptions{
				SkipFiles: false,
			},
		}

		m.printStats()

		output := buf.String()
		testutil.Contains(t, output, "Collections: 5")
		testutil.Contains(t, output, "Tables: 3")
		testutil.Contains(t, output, "Views: 1")
		testutil.Contains(t, output, "Records: 100")
		testutil.Contains(t, output, "Files: 25")
		testutil.Contains(t, output, "Policies: 6")
	})

	t.Run("skip files in output", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer

		m := &Migrator{
			output: &buf,
			stats: MigrationStats{
				Collections: 2,
				Tables:      1,
				Views:       0,
				Records:     50,
				Files:       0,
				Policies:    3,
			},
			opts: MigrationOptions{
				SkipFiles: true,
			},
		}

		m.printStats()

		output := buf.String()
		testutil.Contains(t, output, "Collections: 2")
		testutil.Contains(t, output, "Tables: 1")
		testutil.Contains(t, output, "Records: 50")
		testutil.Contains(t, output, "Policies: 3")
		// Must NOT contain Files line when SkipFiles is true
		if strings.Contains(output, "Files:") {
			t.Errorf("expected no 'Files:' line when SkipFiles=true, got:\n%s", output)
		}
	})
}

func TestNewMigrator_Errors(t *testing.T) {
	t.Parallel()
	t.Run("missing source path", func(t *testing.T) {
		t.Parallel()
		opts := MigrationOptions{
			DatabaseURL: "postgres://localhost",
		}

		_, err := NewMigrator(opts)
		testutil.ErrorContains(t, err, "source path is required")
	})

	t.Run("missing database URL", func(t *testing.T) {
		t.Parallel()
		opts := MigrationOptions{
			SourcePath: "/tmp/pb_data",
		}

		_, err := NewMigrator(opts)
		testutil.ErrorContains(t, err, "database URL is required")
	})

	t.Run("invalid source path", func(t *testing.T) {
		t.Parallel()
		opts := MigrationOptions{
			SourcePath:  "/nonexistent/path",
			DatabaseURL: "postgres://localhost",
		}

		_, err := NewMigrator(opts)
		testutil.ErrorContains(t, err, "failed to create reader")
	})
}

func TestMigrateFiles_EdgeCases(t *testing.T) {
	t.Parallel()
	t.Run("no storage directory", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()
		pbDataPath := filepath.Join(tmpDir, "pb_data")
		err := os.MkdirAll(pbDataPath, 0755)
		testutil.NoError(t, err)

		var buf bytes.Buffer
		m := &Migrator{
			opts: MigrationOptions{
				SourcePath: pbDataPath,
			},
			output:  &buf,
			verbose: false,
		}

		collections := []PBCollection{
			{
				Name:   "posts",
				Type:   "base",
				System: false,
				Schema: []PBField{
					{Name: "image", Type: "file"},
				},
			},
		}

		err = m.migrateFiles(context.Background(), collections)
		testutil.NoError(t, err)

		output := buf.String()
		testutil.Contains(t, output, "No storage directory found")
	})

	t.Run("collection directory not exist", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()
		pbDataPath := filepath.Join(tmpDir, "pb_data")
		storagePath := filepath.Join(pbDataPath, "storage")
		err := os.MkdirAll(storagePath, 0755)
		testutil.NoError(t, err)

		var buf bytes.Buffer
		m := &Migrator{
			opts: MigrationOptions{
				SourcePath: pbDataPath,
				Verbose:    true,
			},
			output:  &buf,
			verbose: true,
		}

		collections := []PBCollection{
			{
				Name:   "posts",
				Type:   "base",
				System: false,
				Schema: []PBField{
					{Name: "image", Type: "file"},
				},
			},
		}

		err = m.migrateFiles(context.Background(), collections)
		testutil.NoError(t, err)

		output := buf.String()
		testutil.Contains(t, output, "posts: no files (skipping)")
	})

	t.Run("collection directory is empty", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()
		pbDataPath := filepath.Join(tmpDir, "pb_data")
		storagePath := filepath.Join(pbDataPath, "storage", "posts")
		err := os.MkdirAll(storagePath, 0755)
		testutil.NoError(t, err)

		var buf bytes.Buffer
		m := &Migrator{
			opts: MigrationOptions{
				SourcePath: pbDataPath,
				Verbose:    true,
			},
			output:  &buf,
			verbose: true,
			stats:   MigrationStats{},
		}

		collections := []PBCollection{
			{
				Name:   "posts",
				Type:   "base",
				System: false,
				Schema: []PBField{
					{Name: "image", Type: "file"},
				},
			},
		}

		err = m.migrateFiles(context.Background(), collections)
		testutil.NoError(t, err)

		output := buf.String()
		testutil.Contains(t, output, "posts: 0 files")
	})

	t.Run("copy files successfully", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()
		pbDataPath := filepath.Join(tmpDir, "pb_data")
		storagePath := filepath.Join(pbDataPath, "storage", "posts")
		err := os.MkdirAll(storagePath, 0755)
		testutil.NoError(t, err)

		// Create test files
		err = os.WriteFile(filepath.Join(storagePath, "image1.jpg"), []byte("test-image"), 0644)
		testutil.NoError(t, err)

		// Create nested directory
		nestedPath := filepath.Join(storagePath, "nested")
		err = os.MkdirAll(nestedPath, 0755)
		testutil.NoError(t, err)
		err = os.WriteFile(filepath.Join(nestedPath, "doc.pdf"), []byte("test-pdf"), 0644)
		testutil.NoError(t, err)

		// Create destination
		destPath := filepath.Join(tmpDir, "ayb_storage")

		var buf bytes.Buffer
		m := &Migrator{
			opts: MigrationOptions{
				SourcePath:  pbDataPath,
				StoragePath: destPath,
				Verbose:     true,
			},
			output:  &buf,
			verbose: true,
			stats:   MigrationStats{},
		}

		collections := []PBCollection{
			{
				Name:   "posts",
				Type:   "base",
				System: false,
				Schema: []PBField{
					{Name: "image", Type: "file"},
				},
			},
		}

		err = m.migrateFiles(context.Background(), collections)
		testutil.NoError(t, err)

		// Verify files were copied
		content1, err := os.ReadFile(filepath.Join(destPath, "posts", "image1.jpg"))
		testutil.NoError(t, err)
		testutil.Equal(t, "test-image", string(content1))

		content2, err := os.ReadFile(filepath.Join(destPath, "posts", "nested", "doc.pdf"))
		testutil.NoError(t, err)
		testutil.Equal(t, "test-pdf", string(content2))

		// Check stats
		testutil.Equal(t, 2, m.stats.Files)

		output := buf.String()
		testutil.Contains(t, output, "posts: 2 files copied")
	})

	t.Run("s3 backend not implemented", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()
		pbDataPath := filepath.Join(tmpDir, "pb_data")
		storagePath := filepath.Join(pbDataPath, "storage")
		err := os.MkdirAll(storagePath, 0755)
		testutil.NoError(t, err)

		m := &Migrator{
			opts: MigrationOptions{
				SourcePath:     pbDataPath,
				StorageBackend: "s3",
			},
			output: os.Stdout,
		}

		collections := []PBCollection{
			{
				Name:   "posts",
				Type:   "base",
				System: false,
				Schema: []PBField{
					{Name: "image", Type: "file"},
				},
			},
		}

		err = m.migrateFiles(context.Background(), collections)
		testutil.ErrorContains(t, err, "S3 storage backend not yet implemented")
	})
}

func TestMigrateAuthUsers_EdgeCases(t *testing.T) {
	t.Parallel()
	t.Run("parse users with verified as int type", func(t *testing.T) {
		t.Parallel()
		schema := []PBField{
			{Name: "email", Type: "email", System: false},
			{Name: "passwordHash", Type: "text", System: false},
			{Name: "verified", Type: "bool", System: false},
		}

		records := []PBRecord{
			{
				ID: "user1",
				Data: map[string]interface{}{
					"email":        "test@example.com",
					"passwordHash": "$2a$10$hash",
					"verified":     int(1), // int instead of int64
				},
			},
		}

		users, err := parseAuthUsers(records, schema)
		testutil.NoError(t, err)
		testutil.Equal(t, 1, len(users))
		testutil.True(t, users[0].Verified)
	})

	t.Run("parse users with missing timestamps", func(t *testing.T) {
		t.Parallel()
		schema := []PBField{
			{Name: "email", Type: "email", System: false},
			{Name: "passwordHash", Type: "text", System: false},
		}

		records := []PBRecord{
			{
				ID: "user1",
				Data: map[string]interface{}{
					"email":        "test@example.com",
					"passwordHash": "$2a$10$hash",
				},
			},
		}

		users, err := parseAuthUsers(records, schema)
		testutil.NoError(t, err)
		testutil.Equal(t, 1, len(users))
		testutil.Equal(t, "", users[0].CreatedAt)
		testutil.Equal(t, "", users[0].UpdatedAt)
	})

	t.Run("parse users with missing custom fields", func(t *testing.T) {
		t.Parallel()
		schema := []PBField{
			{Name: "email", Type: "email", System: false},
			{Name: "passwordHash", Type: "text", System: false},
			{Name: "name", Type: "text", System: false},
			{Name: "role", Type: "select", System: false},
		}

		records := []PBRecord{
			{
				ID: "user1",
				Data: map[string]interface{}{
					"email":        "test@example.com",
					"passwordHash": "$2a$10$hash",
					"name":         "Test User",
					// role is missing
				},
			},
		}

		users, err := parseAuthUsers(records, schema)
		testutil.NoError(t, err)
		testutil.Equal(t, 1, len(users))
		testutil.Equal(t, 1, len(users[0].CustomFields))
		testutil.Equal(t, "Test User", users[0].CustomFields["name"])
		_, hasRole := users[0].CustomFields["role"]
		testutil.False(t, hasRole)
	})

	t.Run("empty email error", func(t *testing.T) {
		t.Parallel()
		schema := []PBField{
			{Name: "email", Type: "email", System: false},
			{Name: "passwordHash", Type: "text", System: false},
		}

		records := []PBRecord{
			{
				ID: "user1",
				Data: map[string]interface{}{
					"email":        "", // empty string
					"passwordHash": "$2a$10$hash",
				},
			},
		}

		users, err := parseAuthUsers(records, schema)
		testutil.NoError(t, err) // Empty email is allowed (will fail at DB level if required)
		testutil.Equal(t, 1, len(users))
		testutil.Equal(t, "", users[0].Email)
	})

	t.Run("case insensitive standard field check", func(t *testing.T) {
		// Test that isStandardAuthField is case-insensitive
		t.Parallel()

		testutil.True(t, isStandardAuthField("EMAIL"))
		testutil.True(t, isStandardAuthField("Email"))
		testutil.True(t, isStandardAuthField("PASSWORDHASH"))
		testutil.True(t, isStandardAuthField("PasswordHash"))
		testutil.True(t, isStandardAuthField("VERIFIED"))
		testutil.False(t, isStandardAuthField("CustomField"))
	})
}

func TestCoerceToBool(t *testing.T) {
	t.Parallel()
	t.Run("int64 truthy", func(t *testing.T) {
		t.Parallel()
		testutil.Equal(t, true, coerceToBool(int64(1)))
	})

	t.Run("int64 falsy", func(t *testing.T) {
		t.Parallel()
		testutil.Equal(t, false, coerceToBool(int64(0)))
	})

	t.Run("int truthy", func(t *testing.T) {
		t.Parallel()
		testutil.Equal(t, true, coerceToBool(int(1)))
	})

	t.Run("int falsy", func(t *testing.T) {
		t.Parallel()
		testutil.Equal(t, false, coerceToBool(int(0)))
	})

	t.Run("float64 truthy", func(t *testing.T) {
		t.Parallel()
		testutil.Equal(t, true, coerceToBool(float64(1)))
	})

	t.Run("float64 falsy", func(t *testing.T) {
		t.Parallel()
		testutil.Equal(t, false, coerceToBool(float64(0)))
	})

	t.Run("bool passthrough true", func(t *testing.T) {
		t.Parallel()
		testutil.Equal(t, true, coerceToBool(true))
	})

	t.Run("bool passthrough false", func(t *testing.T) {
		t.Parallel()
		testutil.Equal(t, false, coerceToBool(false))
	})

	t.Run("nil passthrough", func(t *testing.T) {
		t.Parallel()
		testutil.Nil(t, coerceToBool(nil))
	})

	t.Run("string passthrough", func(t *testing.T) {
		t.Parallel()
		testutil.Equal(t, "true", coerceToBool("true"))
	})
}

func TestUserProfilesTableName(t *testing.T) {
	// Verify the table name is properly constructed with the identifier quoted
	// around the full name, not embedded inside it (bug #2).
	t.Parallel()

	tests := []struct {
		collName string
		want     string
	}{
		{"users", `"_ayb_user_profiles_users"`},
		{"members", `"_ayb_user_profiles_members"`},
		{"my_collection", `"_ayb_user_profiles_my_collection"`},
	}
	for _, tt := range tests {
		t.Run(tt.collName, func(t *testing.T) {
			t.Parallel()
			got := SanitizeIdentifier(fmt.Sprintf("_ayb_user_profiles_%s", tt.collName))
			testutil.Equal(t, tt.want, got)
		})
	}
}

func TestCountSchemaTables(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		collections []PBCollection
		expected    int
	}{
		{
			name:        "empty",
			collections: nil,
			expected:    0,
		},
		{
			name: "skips system collections",
			collections: []PBCollection{
				{Name: "posts", Type: "base", System: false},
				{Name: "_pb_system", Type: "base", System: true},
			},
			expected: 1,
		},
		{
			name: "skips auth collections",
			collections: []PBCollection{
				{Name: "posts", Type: "base", System: false},
				{Name: "users", Type: "auth", System: false},
			},
			expected: 1,
		},
		{
			name: "includes views",
			collections: []PBCollection{
				{Name: "posts", Type: "base", System: false},
				{Name: "stats", Type: "view", System: false},
			},
			expected: 2,
		},
		{
			name: "mixed collections",
			collections: []PBCollection{
				{Name: "posts", Type: "base", System: false},
				{Name: "comments", Type: "base", System: false},
				{Name: "users", Type: "auth", System: false},
				{Name: "stats", Type: "view", System: false},
				{Name: "_pb_sys", Type: "base", System: true},
			},
			expected: 3, // posts + comments + stats (not auth, not system)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := countSchemaTables(tt.collections)
			testutil.Equal(t, tt.expected, result)
		})
	}
}

func TestCountUserStats(t *testing.T) {
	t.Parallel()
	stats := &MigrationStats{AuthUsers: 42, Records: 100}
	testutil.Equal(t, 42, countUserStats(stats))

	stats2 := &MigrationStats{AuthUsers: 0, Records: 100}
	testutil.Equal(t, 0, countUserStats(stats2))
}

func TestFormatElapsed(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    time.Duration
		expected string
	}{
		{"sub-second", 50 * time.Millisecond, "50ms"},
		{"exactly 1 second", time.Second, "1.0s"},
		{"seconds with decimal", 2500 * time.Millisecond, "2.5s"},
		{"zero", 0, "0ms"},
		{"999ms", 999 * time.Millisecond, "999ms"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := formatElapsed(tt.input)
			testutil.Equal(t, tt.expected, result)
		})
	}
}
