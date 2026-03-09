package migrations

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestSanitizeName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input string
		want  string
	}{
		{"create_posts", "create_posts"},
		{"add users table", "add_users_table"},
		{"add-index", "add_index"},
		{"foo/bar", "foo_bar"},
		{"CamelCase", "CamelCase"},
		{"123_numbers", "123_numbers"},
		{"special!@#chars", "special___chars"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			testutil.Equal(t, tt.want, sanitizeName(tt.input))
		})
	}
}

func TestCreateFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	r := NewUserRunner(nil, dir, testutil.DiscardLogger())

	path, err := r.CreateFile("create_posts")
	testutil.NoError(t, err)

	// File should exist.
	info, err := os.Stat(path)
	testutil.NoError(t, err)
	testutil.False(t, info.IsDir())

	// Filename should have timestamp prefix and .sql suffix.
	name := filepath.Base(path)
	testutil.True(t, strings.HasSuffix(name, "_create_posts.sql"), "filename: %s", name)
	testutil.True(t, len(name) > 20, "filename should have timestamp prefix: %s", name)

	// Content should have header comment.
	data, err := os.ReadFile(path)
	testutil.NoError(t, err)
	testutil.Contains(t, string(data), "-- Migration: create_posts")
	testutil.Contains(t, string(data), "-- Created:")
}

func TestCreateFileCreatesDir(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "subdir", "migrations")
	r := NewUserRunner(nil, dir, testutil.DiscardLogger())

	path, err := r.CreateFile("init")
	testutil.NoError(t, err)

	_, err = os.Stat(path)
	testutil.NoError(t, err)
}

func TestCreateFileSanitizesName(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	r := NewUserRunner(nil, dir, testutil.DiscardLogger())

	path, err := r.CreateFile("add user-roles")
	testutil.NoError(t, err)

	name := filepath.Base(path)
	testutil.True(t, strings.HasSuffix(name, "_add_user_roles.sql"), "filename: %s", name)
}

func TestListFilesEmpty(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	r := NewUserRunner(nil, dir, testutil.DiscardLogger())

	files, err := r.listFiles()
	testutil.NoError(t, err)
	testutil.SliceLen(t, files, 0)
}

func TestListFilesNonExistentDir(t *testing.T) {
	t.Parallel()
	r := NewUserRunner(nil, "/tmp/nonexistent_ayb_test_dir", testutil.DiscardLogger())

	files, err := r.listFiles()
	testutil.NoError(t, err) // non-existent dir returns nil, not error
	testutil.SliceLen(t, files, 0)
}

func TestListFilesSorted(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// Create files out of order.
	for _, name := range []string{
		"20260207_c.sql",
		"20260205_a.sql",
		"20260206_b.sql",
		"readme.txt", // not .sql, should be ignored
	} {
		os.WriteFile(filepath.Join(dir, name), []byte("-- test"), 0o644)
	}

	r := NewUserRunner(nil, dir, testutil.DiscardLogger())
	files, err := r.listFiles()
	testutil.NoError(t, err)
	testutil.SliceLen(t, files, 3)
	testutil.Equal(t, "20260205_a.sql", files[0])
	testutil.Equal(t, "20260206_b.sql", files[1])
	testutil.Equal(t, "20260207_c.sql", files[2])
}

func TestListFilesIgnoresDirectories(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "001_init.sql"), []byte("-- test"), 0o644)
	os.Mkdir(filepath.Join(dir, "subdir.sql"), 0o755) // directory with .sql name

	r := NewUserRunner(nil, dir, testutil.DiscardLogger())
	files, err := r.listFiles()
	testutil.NoError(t, err)
	testutil.SliceLen(t, files, 1)
	testutil.Equal(t, "001_init.sql", files[0])
}
