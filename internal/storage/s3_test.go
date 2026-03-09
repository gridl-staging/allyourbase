package storage

import (
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

// Compile-time check that S3Backend implements Backend.
var _ Backend = (*S3Backend)(nil)

func TestS3BackendKey(t *testing.T) {
	t.Parallel()
	b := &S3Backend{} // key() doesn't use client or bucket
	tests := []struct {
		name       string
		aybBucket  string
		objectName string
		want       string
	}{
		{"simple", "images", "photo.jpg", "images/photo.jpg"},
		{"nested name", "docs", "a/b/c/file.txt", "docs/a/b/c/file.txt"},
		{"empty bucket", "", "file.txt", "/file.txt"},
		{"empty name", "images", "", "images/"},
		{"special chars", "uploads", "hello world.txt", "uploads/hello world.txt"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := b.key(tt.aybBucket, tt.objectName)
			testutil.Equal(t, tt.want, got)
		})
	}
}

// Note: NewS3Backend requires a live S3-compatible endpoint (BucketExists check).
// Full Put/Get/Delete/Exists tests belong in integration tests with MinIO.
// See storage_integration_test.go.
