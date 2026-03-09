package pbmigrate

import (
	"testing"

	"github.com/allyourbase/ayb/internal/migrate"
	"github.com/allyourbase/ayb/internal/testutil"
)

func TestAnalyze(t *testing.T) {
	t.Parallel()
	t.Run("nonexistent source path", func(t *testing.T) {
		t.Parallel()
		_, err := Analyze("/nonexistent/path")
		testutil.ErrorContains(t, err, "opening source")
	})

	t.Run("missing data.db", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()
		_, err := Analyze(tmpDir)
		testutil.ErrorContains(t, err, "data.db not found")
	})
}

func TestCountPolicies(t *testing.T) {
	t.Parallel()
	open := ""
	locked := "locked_expr"

	tests := []struct {
		name     string
		coll     PBCollection
		expected int
	}{
		{
			name: "no rules",
			coll: PBCollection{
				Name: "posts",
				Type: "base",
			},
			expected: 0,
		},
		{
			name: "all rules set",
			coll: PBCollection{
				Name:       "posts",
				Type:       "base",
				ListRule:   &open,
				ViewRule:   &open,
				CreateRule: &locked,
				UpdateRule: &locked,
				DeleteRule: &locked,
			},
			expected: 4, // ViewRule excluded: both List and View map to SELECT, only ListRule used
		},
		{
			name: "partial rules",
			coll: PBCollection{
				Name:     "posts",
				Type:     "base",
				ListRule: &open,
				ViewRule: &open,
			},
			expected: 1, // ViewRule excluded: only ListRule counted
		},
		{
			name: "system collection always 0",
			coll: PBCollection{
				Name:     "system_coll",
				Type:     "base",
				System:   true,
				ListRule: &open,
			},
			expected: 0,
		},
		{
			name: "auth collection always 0",
			coll: PBCollection{
				Name:     "users",
				Type:     "auth",
				ListRule: &open,
			},
			expected: 0,
		},
		{
			name: "view collection always 0",
			coll: PBCollection{
				Name:     "my_view",
				Type:     "view",
				ListRule: &open,
			},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := countPolicies(tt.coll)
			testutil.Equal(t, tt.expected, result)
		})
	}
}

func TestCountPoliciesMatchesGenerateRLSPolicies(t *testing.T) {
	// Verify countPolicies (used in Analyze) matches GenerateRLSPolicies (used in Migrate)
	// so the validation summary never shows a false mismatch.
	t.Parallel()

	open := ""
	locked := "some_expr"

	tests := []struct {
		name string
		coll PBCollection
	}{
		{
			name: "all rules set",
			coll: PBCollection{
				Name: "posts", Type: "base",
				ListRule: &open, ViewRule: &open, CreateRule: &locked, UpdateRule: &locked, DeleteRule: &locked,
			},
		},
		{
			name: "only list and create",
			coll: PBCollection{
				Name: "items", Type: "base",
				ListRule: &open, CreateRule: &locked,
			},
		},
		{
			name: "no rules (all locked)",
			coll: PBCollection{Name: "secrets", Type: "base"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			count := countPolicies(tt.coll)
			policies, err := GenerateRLSPolicies(tt.coll)
			testutil.NoError(t, err)
			testutil.Equal(t, count, len(policies))
		})
	}
}

func TestFormatSize(t *testing.T) {
	t.Parallel()
	tests := []struct {
		bytes    int64
		expected string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1023, "1023 B"},          // boundary: just below KB
		{1024, "1.0 KB"},          // boundary: exact KB
		{1025, "1.0 KB"},          // just above KB
		{1048575, "1024.0 KB"},    // just below MB
		{1048576, "1.0 MB"},       // boundary: exact MB
		{7340032, "7.0 MB"},       // 7 MB
		{1073741823, "1024.0 MB"}, // just below GB
		{1073741824, "1.0 GB"},    // boundary: exact GB
		{int64(1.5 * 1024 * 1024 * 1024), "1.5 GB"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			t.Parallel()
			result := formatSize(tt.bytes)
			testutil.Equal(t, tt.expected, result)
		})
	}
}

func TestBuildValidationSummary(t *testing.T) {
	t.Parallel()
	t.Run("builds summary from report and stats", func(t *testing.T) {
		t.Parallel()
		report := &migrate.AnalysisReport{
			SourceType:  "PocketBase",
			Tables:      5,
			Views:       1,
			Records:     1000,
			AuthUsers:   50,
			RLSPolicies: 10,
			Files:       200,
		}

		stats := &MigrationStats{
			Tables:    5,
			Views:     1,
			Records:   1000,
			AuthUsers: 50,
			Policies:  10,
			Files:     200,
		}

		summary := BuildValidationSummary(report, stats)

		testutil.Equal(t, "Source (PocketBase)", summary.SourceLabel)
		testutil.Equal(t, "Target (AYB)", summary.TargetLabel)
		testutil.Equal(t, 6, len(summary.Rows))

		// Verify every row label, source, and target value (not just spot checks)
		expected := []struct {
			label  string
			source int
			target int
		}{
			{"Tables", 5, 5},
			{"Views", 1, 1},
			{"Records", 1000, 1000},
			{"Auth users", 50, 50},
			{"RLS policies", 10, 10},
			{"Files", 200, 200},
		}
		for i, exp := range expected {
			testutil.Equal(t, exp.label, summary.Rows[i].Label)
			testutil.Equal(t, exp.source, summary.Rows[i].SourceCount)
			testutil.Equal(t, exp.target, summary.Rows[i].TargetCount)
		}
	})

	t.Run("detects mismatches", func(t *testing.T) {
		t.Parallel()
		report := &migrate.AnalysisReport{
			SourceType: "PocketBase",
			Tables:     5,
			Files:      100,
		}

		stats := &MigrationStats{
			Tables: 5,
			Files:  98, // 2 files failed
		}

		summary := BuildValidationSummary(report, stats)

		// Files row should show mismatch
		filesRow := summary.Rows[5]
		testutil.Equal(t, "Files", filesRow.Label)
		testutil.Equal(t, 100, filesRow.SourceCount)
		testutil.Equal(t, 98, filesRow.TargetCount)
	})
}
