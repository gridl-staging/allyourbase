package migrate

import (
	"bytes"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestDetectSource(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		from     string
		expected SourceType
	}{
		{
			name:     "pocketbase directory path",
			from:     "./pb_data",
			expected: SourcePocketBase,
		},
		{
			name:     "pocketbase absolute path",
			from:     "/home/user/myapp/pb_data",
			expected: SourcePocketBase,
		},
		{
			name:     "supabase postgres URL",
			from:     "postgres://user:pass@db.abc123.supabase.co:5432/postgres",
			expected: SourceSupabase,
		},
		{
			name:     "supabase postgresql URL",
			from:     "postgresql://user:pass@db.abc123.supabase.co:5432/postgres",
			expected: SourceSupabase,
		},
		{
			name:     "generic postgres URL",
			from:     "postgres://user:pass@localhost:5432/mydb",
			expected: SourcePostgres,
		},
		{
			name:     "generic postgresql URL",
			from:     "postgresql://user:pass@localhost:5432/mydb",
			expected: SourcePostgres,
		},
		{
			name:     "firebase URL scheme",
			from:     "firebase://my-project-id",
			expected: SourceFirebase,
		},
		{
			name:     "firebase service account JSON",
			from:     "service-account.json",
			expected: SourceFirebase,
		},
		{
			name:     "firebase service account JSON absolute path",
			from:     "/path/to/my-firebase-adminsdk.json",
			expected: SourceFirebase,
		},
		{
			name:     "unknown scheme",
			from:     "mysql://user:pass@localhost/mydb",
			expected: SourceUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := DetectSource(tt.from)
			testutil.Equal(t, tt.expected, result)
		})
	}
}

func TestSourceTypeString(t *testing.T) {
	t.Parallel()
	tests := []struct {
		st       SourceType
		expected string
	}{
		{SourcePocketBase, "PocketBase"},
		{SourceSupabase, "Supabase"},
		{SourceFirebase, "Firebase"},
		{SourcePostgres, "PostgreSQL"},
		{SourceUnknown, "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			t.Parallel()
			testutil.Equal(t, tt.expected, tt.st.String())
		})
	}
}

func TestCLIReporter(t *testing.T) {
	t.Parallel()
	t.Run("complete phase output", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer
		r := NewCLIReporter(&buf)

		phase := Phase{Name: "Schema", Index: 1, Total: 5}
		r.StartPhase(phase, 10)
		r.CompletePhase(phase, 10, 200*time.Millisecond)

		output := buf.String()
		testutil.Contains(t, output, "[1/5]")
		testutil.Contains(t, output, "Schema")
		testutil.Contains(t, output, "10 items")
		testutil.Contains(t, output, "✓")
		testutil.Contains(t, output, "200ms")
	})

	t.Run("zero items shows skipped", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer
		r := NewCLIReporter(&buf)

		phase := Phase{Name: "Auth users", Index: 3, Total: 5}
		r.StartPhase(phase, 0)
		r.CompletePhase(phase, 0, 5*time.Millisecond)

		output := buf.String()
		testutil.Contains(t, output, "skipped")
	})

	t.Run("seconds formatting", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer
		r := NewCLIReporter(&buf)

		phase := Phase{Name: "Data", Index: 2, Total: 5}
		r.CompletePhase(phase, 5000, 2500*time.Millisecond)

		output := buf.String()
		testutil.Contains(t, output, "2.5s")
	})

	t.Run("warn output", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer
		r := NewCLIReporter(&buf)

		r.Warn("4 RLS policies need manual review")

		output := buf.String()
		testutil.Contains(t, output, "⚠")
		testutil.Contains(t, output, "4 RLS policies need manual review")
	})
}

func TestNopReporter(t *testing.T) {
	// NopReporter should not panic on any method call.
	t.Parallel()

	r := NopReporter{}
	phase := Phase{Name: "test", Index: 1, Total: 1}
	r.StartPhase(phase, 10)
	r.Progress(phase, 5, 10)
	r.CompletePhase(phase, 10, time.Second)
	r.Warn("test warning")
}

func TestAnalysisReport_PrintReport(t *testing.T) {
	t.Parallel()
	t.Run("full report", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer
		report := &AnalysisReport{
			SourceType:    "PocketBase",
			SourceInfo:    "SQLite 7.2 MB",
			Tables:        12,
			Views:         2,
			Records:       8432,
			AuthUsers:     347,
			RLSPolicies:   8,
			Files:         1204,
			FileSizeBytes: 89 * 1024 * 1024, // 89 MB
			Warnings:      []string{"4 RLS policies use @collection references"},
		}

		report.PrintReport(&buf)
		output := buf.String()

		testutil.Contains(t, output, "PocketBase")
		testutil.Contains(t, output, "SQLite 7.2 MB")
		testutil.Contains(t, output, "Tables:       12")
		testutil.Contains(t, output, "Views:        2")
		testutil.Contains(t, output, "Records:      8432")
		testutil.Contains(t, output, "Auth users:   347")
		testutil.Contains(t, output, "RLS policies: 8")
		testutil.Contains(t, output, "Files:        1204")
		testutil.Contains(t, output, "89.0 MB")
		testutil.Contains(t, output, "Warnings:")
		testutil.Contains(t, output, "@collection references")
	})

	t.Run("minimal report hides zero fields", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer
		report := &AnalysisReport{
			SourceType: "PocketBase",
			Tables:     3,
			Records:    100,
		}

		report.PrintReport(&buf)
		output := buf.String()

		testutil.Contains(t, output, "Tables:       3")
		testutil.Contains(t, output, "Records:      100")
		// Should not contain lines for zero-value fields
		if bytes.Contains(buf.Bytes(), []byte("Views:")) {
			t.Error("should not show Views when 0")
		}
		if bytes.Contains(buf.Bytes(), []byte("Auth users:")) {
			t.Error("should not show Auth users when 0")
		}
		if bytes.Contains(buf.Bytes(), []byte("Files:")) {
			t.Error("should not show Files when 0")
		}
	})
}

func TestValidationSummary_PrintSummary(t *testing.T) {
	t.Parallel()
	t.Run("matching counts", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer
		summary := &ValidationSummary{
			SourceLabel: "Source (PocketBase)",
			TargetLabel: "Target (AYB)",
			Rows: []ValidationRow{
				{Label: "Tables", SourceCount: 12, TargetCount: 12},
				{Label: "Records", SourceCount: 8432, TargetCount: 8432},
				{Label: "Auth users", SourceCount: 347, TargetCount: 347},
			},
		}

		summary.PrintSummary(&buf)
		output := buf.String()

		testutil.Contains(t, output, "Validation Summary")
		testutil.Contains(t, output, "All counts match")
		testutil.Contains(t, output, "Tables")
		testutil.Contains(t, output, "12")
	})

	t.Run("mismatched counts", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer
		summary := &ValidationSummary{
			SourceLabel: "Source (PocketBase)",
			TargetLabel: "Target (AYB)",
			Rows: []ValidationRow{
				{Label: "Tables", SourceCount: 12, TargetCount: 12},
				{Label: "Files", SourceCount: 100, TargetCount: 98},
			},
		}

		summary.PrintSummary(&buf)
		output := buf.String()

		testutil.Contains(t, output, "MISMATCH")
		if bytes.Contains(buf.Bytes(), []byte("All counts match")) {
			t.Error("should not say 'All counts match' when there is a mismatch")
		}
	})

	t.Run("with warnings", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer
		summary := &ValidationSummary{
			SourceLabel: "Source (PocketBase)",
			TargetLabel: "Target (AYB)",
			Rows: []ValidationRow{
				{Label: "Tables", SourceCount: 5, TargetCount: 5},
			},
			Warnings: []string{"2 file fields have S3 URLs"},
		}

		summary.PrintSummary(&buf)
		output := buf.String()

		testutil.Contains(t, output, "Warnings:")
		testutil.Contains(t, output, "2 file fields have S3 URLs")
	})
}

func TestFormatBytes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		bytes    int64
		expected string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
		{89 * 1024 * 1024, "89.0 MB"},
		{int64(2.5 * 1024 * 1024 * 1024), "2.5 GB"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			t.Parallel()
			result := FormatBytes(tt.bytes)
			testutil.Equal(t, tt.expected, result)
		})
	}
}

func TestFormatDuration(t *testing.T) {
	t.Parallel()
	tests := []struct {
		d        time.Duration
		expected string
	}{
		{50 * time.Millisecond, "50ms"},
		{999 * time.Millisecond, "999ms"},
		{1 * time.Second, "1.0s"},
		{2500 * time.Millisecond, "2.5s"},
		{14100 * time.Millisecond, "14.1s"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			t.Parallel()
			result := formatDuration(tt.d)
			testutil.Equal(t, tt.expected, result)
		})
	}
}
