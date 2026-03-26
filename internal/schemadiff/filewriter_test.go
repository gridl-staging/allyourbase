package schemadiff

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestWriteMigration_basic(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	upSQL := "CREATE TABLE t (id uuid);"
	downSQL := "DROP TABLE t;"

	upPath, downPath, err := WriteMigration(dir, "create_t", upSQL, downSQL)
	testutil.NoError(t, err)

	testutil.True(t, strings.HasSuffix(upPath, "0001_create_t.up.sql"), "unexpected up path: "+upPath)
	testutil.True(t, strings.HasSuffix(downPath, "0001_create_t.down.sql"), "unexpected down path: "+downPath)

	gotUp, err := os.ReadFile(upPath)
	testutil.NoError(t, err)
	testutil.Equal(t, upSQL, string(gotUp))

	gotDown, err := os.ReadFile(downPath)
	testutil.NoError(t, err)
	testutil.Equal(t, downSQL, string(gotDown))
}

func TestWriteMigration_sequenceIncrement(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	// Write three migrations.
	up1, _, err := WriteMigration(dir, "first", "-- up1", "-- down1")
	testutil.NoError(t, err)
	testutil.True(t, strings.Contains(up1, "0001_"), "expected 0001 in first: "+up1)

	up2, _, err := WriteMigration(dir, "second", "-- up2", "-- down2")
	testutil.NoError(t, err)
	testutil.True(t, strings.Contains(up2, "0002_"), "expected 0002 in second: "+up2)

	up3, _, err := WriteMigration(dir, "third", "-- up3", "-- down3")
	testutil.NoError(t, err)
	testutil.True(t, strings.Contains(up3, "0003_"), "expected 0003 in third: "+up3)
}

func TestWriteMigration_existingHighSeq(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	// Pre-create a migration with a high sequence number.
	existing := filepath.Join(dir, "0042_old_migration.up.sql")
	err := os.WriteFile(existing, []byte("-- old"), 0o644)
	testutil.NoError(t, err)
	existingDown := filepath.Join(dir, "0042_old_migration.down.sql")
	err = os.WriteFile(existingDown, []byte("-- old down"), 0o644)
	testutil.NoError(t, err)

	upPath, _, err := WriteMigration(dir, "new_one", "-- new up", "-- new down")
	testutil.NoError(t, err)
	testutil.True(t, strings.Contains(upPath, "0043_"), "expected 0043: "+upPath)
}

func TestWriteMigration_nonAlphanumericName(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	upPath, _, err := WriteMigration(dir, "Add Users Table!", "-- up", "-- down")
	testutil.NoError(t, err)
	testutil.True(t, strings.Contains(upPath, "add_users_table"), "unexpected name in path: "+upPath)
}

func TestWriteMigration_emptyNameFallsBack(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	upPath, _, err := WriteMigration(dir, "!!!!", "-- up", "-- down")
	testutil.NoError(t, err)
	testutil.True(t, strings.Contains(upPath, "_migration"), "expected fallback name in path: "+upPath)
}

func TestWriteMigration_createsDir(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "nested", "migrations")
	_, _, err := WriteMigration(dir, "create_table", "-- up", "-- down")
	testutil.NoError(t, err)

	_, err = os.Stat(dir)
	testutil.NoError(t, err)
}

func TestWriteMigration_ignoresNonMigrationFiles(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	// Put some non-migration files in the dir.
	_ = os.WriteFile(filepath.Join(dir, "README.md"), []byte("# hi"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "schema.json"), []byte("{}"), 0o644)

	upPath, _, err := WriteMigration(dir, "first", "-- up", "-- down")
	testutil.NoError(t, err)
	testutil.True(t, strings.Contains(upPath, "0001_"), "expected 0001 as first: "+upPath)
}

func TestSanitizeName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  string
	}{
		{"create_users", "create_users"},
		{"Add Users Table", "add_users_table"},
		{"Add Users Table!", "add_users_table"},
		{"  spaces  ", "spaces"},
		{"123_numeric", "123_numeric"},
		{"!@#$%", "migration"},
		{"CamelCase", "camelcase"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			got := sanitizeName(tt.input)
			testutil.Equal(t, tt.want, got)
		})
	}
}
