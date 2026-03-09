package fbmigrate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestNormalizeFirebaseBucketName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"lowercase passthrough", "avatars", "avatars"},
		{"uppercase converted", "Avatars", "avatars"},
		{"gcs bucket name", "my-project.appspot.com", "my-project-appspot-com"},
		{"spaces to hyphens", "my bucket", "my-bucket"},
		{"dots to hyphens", "my.bucket.name", "my-bucket-name"},
		{"special chars stripped", "bucket@#$!", "bucket"},
		{"empty becomes default", "", "default"},
		{"only special chars", "@#$%", "default"},
		{"long name truncated", strings.Repeat("a", 100), strings.Repeat("a", 63)},
		{"digits preserved", "bucket123", "bucket123"},
		{"hyphens preserved", "my-bucket", "my-bucket"},
		{"underscores preserved", "my_bucket", "my_bucket"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := normalizeFirebaseBucketName(tt.input)
			testutil.Equal(t, tt.want, got)
		})
	}
}

func TestScanStorageExport(t *testing.T) {
	t.Parallel()
	t.Run("single bucket with files", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		// Create bucket directory with files.
		bucketDir := filepath.Join(dir, "avatars")
		testutil.NoError(t, os.MkdirAll(bucketDir, 0755))
		testutil.NoError(t, os.WriteFile(filepath.Join(bucketDir, "user1.jpg"), []byte("jpeg-data"), 0644))
		testutil.NoError(t, os.WriteFile(filepath.Join(bucketDir, "user2.png"), []byte("png-data-longer"), 0644))

		buckets, err := scanStorageExport(dir)
		testutil.NoError(t, err)
		testutil.Equal(t, 1, len(buckets))
		testutil.Equal(t, 2, len(buckets["avatars"]))

		// Check file info.
		for _, f := range buckets["avatars"] {
			testutil.Equal(t, "avatars", f.Bucket)
			testutil.True(t, f.Size > 0, "file size should be > 0")
			testutil.True(t, f.FullPath != "", "full path should be set")
		}
	})

	t.Run("multiple buckets", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		testutil.NoError(t, os.MkdirAll(filepath.Join(dir, "avatars"), 0755))
		testutil.NoError(t, os.WriteFile(filepath.Join(dir, "avatars", "a.jpg"), []byte("data"), 0644))

		testutil.NoError(t, os.MkdirAll(filepath.Join(dir, "documents"), 0755))
		testutil.NoError(t, os.WriteFile(filepath.Join(dir, "documents", "report.pdf"), []byte("pdf"), 0644))
		testutil.NoError(t, os.WriteFile(filepath.Join(dir, "documents", "notes.txt"), []byte("text"), 0644))

		buckets, err := scanStorageExport(dir)
		testutil.NoError(t, err)
		testutil.Equal(t, 2, len(buckets))
		testutil.Equal(t, 1, len(buckets["avatars"]))
		testutil.Equal(t, 2, len(buckets["documents"]))
	})

	t.Run("nested subdirectories", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		nested := filepath.Join(dir, "uploads", "images", "2024")
		testutil.NoError(t, os.MkdirAll(nested, 0755))
		testutil.NoError(t, os.WriteFile(filepath.Join(nested, "photo.jpg"), []byte("data"), 0644))

		buckets, err := scanStorageExport(dir)
		testutil.NoError(t, err)
		testutil.Equal(t, 1, len(buckets))

		files := buckets["uploads"]
		testutil.Equal(t, 1, len(files))
		testutil.Equal(t, filepath.Join("images", "2024", "photo.jpg"), files[0].Path)
	})

	t.Run("empty directory", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		buckets, err := scanStorageExport(dir)
		testutil.NoError(t, err)
		testutil.Equal(t, 0, len(buckets))
	})

	t.Run("skips top-level files", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		// File at top level (not in a bucket).
		testutil.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("readme"), 0644))

		buckets, err := scanStorageExport(dir)
		testutil.NoError(t, err)
		testutil.Equal(t, 0, len(buckets))
	})

	t.Run("nonexistent directory", func(t *testing.T) {
		t.Parallel()
		_, err := scanStorageExport("/nonexistent/path")
		testutil.NotNil(t, err)
	})
}

func TestCopyFileFS(t *testing.T) {
	t.Parallel()
	t.Run("copies content", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		src := filepath.Join(dir, "source.txt")
		dst := filepath.Join(dir, "dest.txt")

		testutil.NoError(t, os.WriteFile(src, []byte("hello"), 0644))

		n, err := copyFileFS(src, dst)
		testutil.NoError(t, err)
		testutil.Equal(t, int64(5), n)

		got, err := os.ReadFile(dst)
		testutil.NoError(t, err)
		testutil.Equal(t, "hello", string(got))
	})

	t.Run("source not found", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		_, err := copyFileFS(filepath.Join(dir, "missing"), filepath.Join(dir, "dest"))
		testutil.ErrorContains(t, err, "opening source")
	})
}

func TestPhaseCountWithStorage(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		opts MigrationOptions
		want int
	}{
		{
			name: "storage only",
			opts: MigrationOptions{StorageExportPath: "/tmp/storage"},
			want: 1,
		},
		{
			name: "auth + storage",
			opts: MigrationOptions{AuthExportPath: "/tmp/auth.json", StorageExportPath: "/tmp/storage"},
			want: 3, // auth + oauth + storage
		},
		{
			name: "all phases",
			opts: MigrationOptions{
				AuthExportPath:      "/tmp/auth.json",
				FirestoreExportPath: "/tmp/firestore",
				RTDBExportPath:      "/tmp/rtdb.json",
				StorageExportPath:   "/tmp/storage",
			},
			want: 5, // auth + oauth + firestore + rtdb + storage
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

func TestPrintStatsWithStorageAndRTDB(t *testing.T) {
	t.Parallel()
	var buf strings.Builder
	m := &Migrator{
		output: &buf,
		stats: MigrationStats{
			Users:        10,
			RTDBNodes:    3,
			RTDBRecords:  150,
			StorageFiles: 25,
			StorageBytes: 5 * 1024 * 1024,
		},
	}
	m.printStats()
	out := buf.String()
	testutil.Contains(t, out, "RTDB nodes:  3")
	testutil.Contains(t, out, "RTDB records: 150")
	testutil.Contains(t, out, "Files:       25 (5.0 MB)")
	testutil.Contains(t, out, "Users:       10")
}
