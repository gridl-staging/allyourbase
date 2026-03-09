package sbmigrate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/migrate"
	"github.com/allyourbase/ayb/internal/testutil"
)

func TestNormalizeBucketName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"lowercase passthrough", "avatars", "avatars"},
		{"uppercase converted", "Avatars", "avatars"},
		{"mixed case", "My-Bucket_123", "my-bucket_123"},
		{"spaces to hyphens", "my bucket", "my-bucket"},
		{"dots to hyphens", "my.bucket", "my-bucket"},
		{"special chars stripped", "my@bucket!", "mybucket"},
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
			got := normalizeBucketName(tt.input)
			testutil.Equal(t, tt.want, got)
		})
	}
}

func TestCopyFile(t *testing.T) {
	t.Parallel()
	t.Run("copies file contents", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		srcPath := filepath.Join(dir, "source.txt")
		dstPath := filepath.Join(dir, "dest.txt")

		content := []byte("hello world")
		testutil.NoError(t, os.WriteFile(srcPath, content, 0644))

		n, err := copyFile(srcPath, dstPath)
		testutil.NoError(t, err)
		testutil.Equal(t, int64(len(content)), n)

		got, err := os.ReadFile(dstPath)
		testutil.NoError(t, err)
		testutil.Equal(t, string(content), string(got))
	})

	t.Run("copies binary data", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		srcPath := filepath.Join(dir, "binary.bin")
		dstPath := filepath.Join(dir, "copy.bin")

		// Write binary data with null bytes.
		data := []byte{0x00, 0x01, 0xFF, 0xFE, 0x00, 0x80}
		testutil.NoError(t, os.WriteFile(srcPath, data, 0644))

		n, err := copyFile(srcPath, dstPath)
		testutil.NoError(t, err)
		testutil.Equal(t, int64(len(data)), n)

		got, err := os.ReadFile(dstPath)
		testutil.NoError(t, err)
		testutil.SliceLen(t, got, len(data))
		testutil.True(t, string(data) == string(got), "binary content mismatch")
	})

	t.Run("source not found", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		_, err := copyFile(filepath.Join(dir, "nonexistent"), filepath.Join(dir, "dest"))
		testutil.ErrorContains(t, err, "opening source")
	})

	t.Run("destination directory missing", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		srcPath := filepath.Join(dir, "source.txt")
		testutil.NoError(t, os.WriteFile(srcPath, []byte("data"), 0644))

		_, err := copyFile(srcPath, filepath.Join(dir, "nonexistent", "dest"))
		testutil.ErrorContains(t, err, "creating destination")
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
			name: "all phases with storage",
			opts: MigrationOptions{StorageExportPath: "/tmp/export"},
			want: 6, // schema + data + auth + oauth + rls + storage
		},
		{
			name: "skip storage explicitly",
			opts: MigrationOptions{SkipStorage: true, StorageExportPath: "/tmp/export"},
			want: 5, // schema + data + auth + oauth + rls
		},
		{
			name: "no storage path = no storage phase",
			opts: MigrationOptions{},
			want: 5, // schema + data + auth + oauth + rls
		},
		{
			name: "skip all with storage",
			opts: MigrationOptions{SkipData: true, SkipOAuth: true, SkipRLS: true, StorageExportPath: "/tmp/export"},
			want: 2, // auth + storage
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

func TestPrintStatsWithStorage(t *testing.T) {
	t.Parallel()
	var buf strings.Builder
	m := &Migrator{
		output: &buf,
		stats: MigrationStats{
			Users:        10,
			StorageFiles: 25,
			StorageBytes: 5 * 1024 * 1024,
		},
	}
	m.printStats()
	out := buf.String()
	testutil.Contains(t, out, "Files:      25 (5.0 MB)")
	testutil.Contains(t, out, "Users:      10")
}

func TestPrintStatsNoStorage(t *testing.T) {
	t.Parallel()
	var buf strings.Builder
	m := &Migrator{
		output: &buf,
		stats:  MigrationStats{Users: 10},
	}
	m.printStats()
	out := buf.String()
	testutil.False(t, strings.Contains(out, "Files:"), "should not show Files when zero")
}

func TestBuildValidationSummaryWithStorage(t *testing.T) {
	t.Parallel()
	report := &migrate.AnalysisReport{
		AuthUsers: 10,
		Files:     25,
	}
	stats := &MigrationStats{
		Users:        10,
		StorageFiles: 25,
	}
	summary := BuildValidationSummary(report, stats)

	// Find storage row.
	var found bool
	for _, row := range summary.Rows {
		if row.Label == "Storage files" {
			found = true
			testutil.Equal(t, 25, row.SourceCount)
			testutil.Equal(t, 25, row.TargetCount)
		}
	}
	testutil.True(t, found, "should have Storage files row in validation summary")
}

func TestBuildValidationSummaryNoStorage(t *testing.T) {
	t.Parallel()
	report := &migrate.AnalysisReport{AuthUsers: 10}
	stats := &MigrationStats{Users: 10}
	summary := BuildValidationSummary(report, stats)

	for _, row := range summary.Rows {
		testutil.True(t, row.Label != "Storage files",
			"should not have Storage files row when counts are zero")
	}
}
