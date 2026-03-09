package fbmigrate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/migrate"
	"github.com/allyourbase/ayb/internal/testutil"
)

func TestParseRTDBExport(t *testing.T) {
	t.Parallel()
	t.Run("basic export with two collections", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		exportPath := filepath.Join(dir, "rtdb.json")
		data := `{
			"users": {
				"user1": {"name": "Alice", "age": 30},
				"user2": {"name": "Bob", "age": 25}
			},
			"posts": {
				"post1": {"title": "Hello", "body": "World"},
				"post2": {"title": "Foo", "body": "Bar"},
				"post3": {"title": "Baz", "body": "Qux"}
			}
		}`
		testutil.NoError(t, os.WriteFile(exportPath, []byte(data), 0644))

		nodes, err := ParseRTDBExport(exportPath)
		testutil.NoError(t, err)
		testutil.Equal(t, 2, len(nodes))

		// ParseRTDBExport returns nodes sorted by name.
		testutil.Equal(t, "posts", nodes[0].Name)
		testutil.Equal(t, 3, len(nodes[0].Children))
		testutil.Equal(t, "users", nodes[1].Name)
		testutil.Equal(t, 2, len(nodes[1].Children))
	})

	t.Run("scalar value becomes single root row", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		exportPath := filepath.Join(dir, "rtdb.json")
		data := `{
			"counter": 42
		}`
		testutil.NoError(t, os.WriteFile(exportPath, []byte(data), 0644))

		nodes, err := ParseRTDBExport(exportPath)
		testutil.NoError(t, err)
		testutil.Equal(t, 1, len(nodes))
		testutil.Equal(t, "counter", nodes[0].Name)
		testutil.Equal(t, 1, len(nodes[0].Children))
		_, hasRoot := nodes[0].Children["_root"]
		testutil.True(t, hasRoot, "scalar should be stored under _root key")
	})

	t.Run("array value becomes single root row", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		exportPath := filepath.Join(dir, "rtdb.json")
		data := `{
			"tags": ["go", "postgres", "firebase"]
		}`
		testutil.NoError(t, os.WriteFile(exportPath, []byte(data), 0644))

		nodes, err := ParseRTDBExport(exportPath)
		testutil.NoError(t, err)
		testutil.Equal(t, 1, len(nodes))
		testutil.Equal(t, "tags", nodes[0].Name)
		testutil.Equal(t, 1, len(nodes[0].Children))
	})

	t.Run("empty object", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		exportPath := filepath.Join(dir, "rtdb.json")
		testutil.NoError(t, os.WriteFile(exportPath, []byte(`{}`), 0644))

		nodes, err := ParseRTDBExport(exportPath)
		testutil.NoError(t, err)
		testutil.Equal(t, 0, len(nodes))
	})

	t.Run("nested objects", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		exportPath := filepath.Join(dir, "rtdb.json")
		data := `{
			"chats": {
				"room1": {
					"messages": {"msg1": {"text": "hi"}, "msg2": {"text": "hello"}}
				}
			}
		}`
		testutil.NoError(t, os.WriteFile(exportPath, []byte(data), 0644))

		nodes, err := ParseRTDBExport(exportPath)
		testutil.NoError(t, err)
		testutil.Equal(t, 1, len(nodes))
		testutil.Equal(t, "chats", nodes[0].Name)
		testutil.Equal(t, 1, len(nodes[0].Children))
		// "room1" is the child key, its value is the full nested JSON.
		_, hasRoom1 := nodes[0].Children["room1"]
		testutil.True(t, hasRoom1, "should have room1 as a child")
	})

	t.Run("nonexistent file", func(t *testing.T) {
		t.Parallel()
		_, err := ParseRTDBExport("/nonexistent/rtdb.json")
		testutil.ErrorContains(t, err, "reading RTDB export")
	})

	t.Run("invalid JSON", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		exportPath := filepath.Join(dir, "rtdb.json")
		testutil.NoError(t, os.WriteFile(exportPath, []byte(`{invalid`), 0644))

		_, err := ParseRTDBExport(exportPath)
		testutil.ErrorContains(t, err, "parsing RTDB export")
	})
}

func TestNormalizeRTDBTableName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"simple", "users", "users"},
		{"uppercase", "Users", "users"},
		{"hyphens to underscores", "my-collection", "my_collection"},
		{"dots to underscores", "my.collection", "my_collection"},
		{"slashes to underscores", "path/to/data", "path_to_data"},
		{"spaces to underscores", "my collection", "my_collection"},
		{"special chars stripped", "data@#!", "data"},
		{"starts with digit", "123data", "t_123data"},
		{"empty becomes default", "", "rtdb_data"},
		{"only special chars", "@#$", "rtdb_data"},
		{"long name truncated", strings.Repeat("x", 100), strings.Repeat("x", 63)},
		{"underscores preserved", "my_data", "my_data"},
		{"digits preserved", "data123", "data123"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := NormalizeRTDBTableName(tt.input)
			testutil.Equal(t, tt.want, got)
		})
	}
}

func TestCreateRTDBTableSQL(t *testing.T) {
	t.Parallel()
	got := createRTDBTableSQL("users")
	testutil.Contains(t, got, `CREATE TABLE IF NOT EXISTS "users"`)
	testutil.Contains(t, got, `"id" text PRIMARY KEY`)
	testutil.Contains(t, got, `"data" jsonb NOT NULL`)
}

func TestCreateRTDBIndexSQL(t *testing.T) {
	t.Parallel()
	got := createRTDBIndexSQL("users")
	testutil.Contains(t, got, `CREATE INDEX IF NOT EXISTS "idx_users_data"`)
	testutil.Contains(t, got, `ON "users" USING GIN ("data")`)
}

func TestCreateRTDBIndexSQL_LongTableName(t *testing.T) {
	// Table name at max 63 chars would produce a 72-char index name without truncation.
	t.Parallel()

	longName := strings.Repeat("x", 63)
	got := createRTDBIndexSQL(longName)
	// Extract the index name between the first pair of quotes.
	start := strings.Index(got, `"`) + 1
	end := strings.Index(got[start:], `"`) + start
	indexName := got[start:end]
	testutil.True(t, len(indexName) <= 63,
		"index name should be <= 63 chars, got %d: %s", len(indexName), indexName)
}

func TestPhaseCountWithRTDB(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		opts MigrationOptions
		want int
	}{
		{
			name: "RTDB only",
			opts: MigrationOptions{RTDBExportPath: "/tmp/rtdb.json"},
			want: 1,
		},
		{
			name: "auth + RTDB",
			opts: MigrationOptions{AuthExportPath: "/tmp/auth.json", RTDBExportPath: "/tmp/rtdb.json"},
			want: 3, // auth + oauth + rtdb
		},
		{
			name: "Firestore + RTDB",
			opts: MigrationOptions{FirestoreExportPath: "/tmp/firestore", RTDBExportPath: "/tmp/rtdb.json"},
			want: 2, // firestore + rtdb
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			m := &Migrator{opts: tt.opts}
			got := m.phaseCount()
			testutil.Equal(t, tt.want, got)
		})
	}
}

func TestBuildValidationSummaryWithRTDB(t *testing.T) {
	t.Parallel()
	report := &migrate.AnalysisReport{
		AuthUsers: 10,
		Tables:    2,
		Records:   50,
	}
	stats := &MigrationStats{
		Users:       10,
		RTDBNodes:   2,
		RTDBRecords: 50,
	}
	summary := BuildValidationSummary(report, stats)

	// RTDB records should be included in Documents row.
	var docsFound, rtdbFound bool
	for _, row := range summary.Rows {
		if row.Label == "Documents" {
			docsFound = true
			testutil.Equal(t, 50, row.SourceCount)
			testutil.Equal(t, 50, row.TargetCount) // Documents(0) + RTDBRecords(50)
		}
		if row.Label == "RTDB nodes" {
			rtdbFound = true
			testutil.Equal(t, 2, row.SourceCount)
			testutil.Equal(t, 2, row.TargetCount)
		}
	}
	testutil.True(t, docsFound, "should have Documents row")
	testutil.True(t, rtdbFound, "should have RTDB nodes row")
}

func TestBuildValidationSummaryWithStorageFiles(t *testing.T) {
	t.Parallel()
	report := &migrate.AnalysisReport{
		Files: 15,
	}
	stats := &MigrationStats{
		StorageFiles: 15,
	}
	summary := BuildValidationSummary(report, stats)

	var found bool
	for _, row := range summary.Rows {
		if row.Label == "Storage files" {
			found = true
			testutil.Equal(t, 15, row.SourceCount)
			testutil.Equal(t, 15, row.TargetCount)
		}
	}
	testutil.True(t, found, "should have Storage files row")
}
