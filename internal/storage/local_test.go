package storage

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestLocalBackendPutAndGet(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	b, err := NewLocalBackend(dir)
	testutil.NoError(t, err)

	ctx := context.Background()
	data := []byte("hello world")
	n, err := b.Put(ctx, "images", "photo.jpg", bytes.NewReader(data))
	testutil.NoError(t, err)
	testutil.Equal(t, int64(len(data)), n)

	// File exists on disk.
	_, err = os.Stat(filepath.Join(dir, "images", "photo.jpg"))
	testutil.NoError(t, err)

	// Get returns the data.
	rc, err := b.Get(ctx, "images", "photo.jpg")
	testutil.NoError(t, err)
	defer rc.Close()

	got, err := io.ReadAll(rc)
	testutil.NoError(t, err)
	testutil.Equal(t, "hello world", string(got))
}

func TestLocalBackendNestedPath(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	b, err := NewLocalBackend(dir)
	testutil.NoError(t, err)

	ctx := context.Background()
	data := []byte("nested file")
	_, err = b.Put(ctx, "docs", "a/b/c/file.txt", bytes.NewReader(data))
	testutil.NoError(t, err)

	rc, err := b.Get(ctx, "docs", "a/b/c/file.txt")
	testutil.NoError(t, err)
	defer rc.Close()

	got, err := io.ReadAll(rc)
	testutil.NoError(t, err)
	testutil.Equal(t, "nested file", string(got))
}

func TestLocalBackendGetNotFound(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	b, err := NewLocalBackend(dir)
	testutil.NoError(t, err)

	_, err = b.Get(context.Background(), "bucket", "nope.txt")
	testutil.ErrorContains(t, err, "not found")
}

func TestLocalBackendDelete(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	b, err := NewLocalBackend(dir)
	testutil.NoError(t, err)

	ctx := context.Background()
	_, err = b.Put(ctx, "tmp", "delete-me.txt", bytes.NewReader([]byte("data")))
	testutil.NoError(t, err)

	exists, err := b.Exists(ctx, "tmp", "delete-me.txt")
	testutil.NoError(t, err)
	testutil.True(t, exists, "file should exist before delete")

	err = b.Delete(ctx, "tmp", "delete-me.txt")
	testutil.NoError(t, err)

	exists, err = b.Exists(ctx, "tmp", "delete-me.txt")
	testutil.NoError(t, err)
	testutil.False(t, exists, "file should not exist after delete")
}

func TestLocalBackendDeleteNotExist(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	b, err := NewLocalBackend(dir)
	testutil.NoError(t, err)

	// Deleting a non-existent file should not error.
	err = b.Delete(context.Background(), "bucket", "nope.txt")
	testutil.NoError(t, err)
}

func TestLocalBackendExists(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	b, err := NewLocalBackend(dir)
	testutil.NoError(t, err)

	ctx := context.Background()

	exists, err := b.Exists(ctx, "bucket", "nope.txt")
	testutil.NoError(t, err)
	testutil.False(t, exists, "should not exist")

	_, err = b.Put(ctx, "bucket", "yes.txt", bytes.NewReader([]byte("data")))
	testutil.NoError(t, err)

	exists, err = b.Exists(ctx, "bucket", "yes.txt")
	testutil.NoError(t, err)
	testutil.True(t, exists, "should exist")
}

func TestLocalBackendOverwrite(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	b, err := NewLocalBackend(dir)
	testutil.NoError(t, err)

	ctx := context.Background()
	_, err = b.Put(ctx, "b", "file.txt", bytes.NewReader([]byte("version 1")))
	testutil.NoError(t, err)

	_, err = b.Put(ctx, "b", "file.txt", bytes.NewReader([]byte("version 2")))
	testutil.NoError(t, err)

	rc, err := b.Get(ctx, "b", "file.txt")
	testutil.NoError(t, err)
	defer rc.Close()

	got, err := io.ReadAll(rc)
	testutil.NoError(t, err)
	testutil.Equal(t, "version 2", string(got))
}

func TestNewLocalBackendCreatesDir(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "nested", "storage")
	b, err := NewLocalBackend(dir)
	testutil.NoError(t, err)

	info, err := os.Stat(b.root)
	testutil.NoError(t, err)
	testutil.True(t, info.IsDir(), "root should be a directory")
}
