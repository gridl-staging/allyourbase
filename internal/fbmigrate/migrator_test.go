package fbmigrate

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/migrate"
	"github.com/allyourbase/ayb/internal/testutil"
)

func TestNewMigratorValidation(t *testing.T) {
	t.Parallel()
	t.Run("no paths at all", func(t *testing.T) {
		t.Parallel()
		_, err := NewMigrator(MigrationOptions{
			DatabaseURL: "postgres://localhost/test",
		})
		testutil.ErrorContains(t, err, "at least one export path is required")
	})

	t.Run("no database URL", func(t *testing.T) {
		t.Parallel()
		_, err := NewMigrator(MigrationOptions{
			AuthExportPath: "/some/file.json",
		})
		testutil.ErrorContains(t, err, "database URL is required")
	})

	t.Run("auth file does not exist", func(t *testing.T) {
		t.Parallel()
		_, err := NewMigrator(MigrationOptions{
			AuthExportPath: "/nonexistent/file.json",
			DatabaseURL:    "postgres://localhost/test",
		})
		testutil.ErrorContains(t, err, "auth export file")
	})

	t.Run("Firestore path is not a directory", func(t *testing.T) {
		t.Parallel()
		tmp := t.TempDir()
		f, err := writeTestFile(t, tmp, "file.txt", "data")
		testutil.NoError(t, err)

		_, err = NewMigrator(MigrationOptions{
			FirestoreExportPath: f,
			DatabaseURL:         "postgres://localhost/test",
		})
		testutil.ErrorContains(t, err, "firestore export path must be a directory")
	})
}

func TestPhaseCount(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		opts MigrationOptions
		want int
	}{
		{
			name: "auth only",
			opts: MigrationOptions{AuthExportPath: "/some.json"},
			want: 2, // auth users + oauth
		},
		{
			name: "firestore only",
			opts: MigrationOptions{FirestoreExportPath: "/some/dir"},
			want: 1, // firestore
		},
		{
			name: "both",
			opts: MigrationOptions{AuthExportPath: "/some.json", FirestoreExportPath: "/some/dir"},
			want: 3, // auth + oauth + firestore
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			m := &Migrator{opts: tt.opts}
			testutil.Equal(t, tt.want, m.phaseCount())
		})
	}
}

func TestBuildValidationSummary(t *testing.T) {
	t.Parallel()
	t.Run("full migration", func(t *testing.T) {
		t.Parallel()
		report := &migrate.AnalysisReport{
			AuthUsers:  50,
			OAuthLinks: 10,
			Tables:     3,
			Records:    500,
		}
		stats := &MigrationStats{
			Users:       50,
			OAuthLinks:  10,
			Collections: 3,
			Documents:   500,
		}
		summary := BuildValidationSummary(report, stats)
		testutil.Equal(t, "Firebase (source)", summary.SourceLabel)
		testutil.Equal(t, "AYB (target)", summary.TargetLabel)
		testutil.Equal(t, 4, len(summary.Rows))

		testutil.Equal(t, "Auth users", summary.Rows[0].Label)
		testutil.Equal(t, 50, summary.Rows[0].SourceCount)
		testutil.Equal(t, 50, summary.Rows[0].TargetCount)

		testutil.Equal(t, "OAuth links", summary.Rows[1].Label)
		testutil.Equal(t, "Collections", summary.Rows[2].Label)
		testutil.Equal(t, "Documents", summary.Rows[3].Label)
	})

	t.Run("auth only", func(t *testing.T) {
		t.Parallel()
		report := &migrate.AnalysisReport{AuthUsers: 10}
		stats := &MigrationStats{Users: 10}
		summary := BuildValidationSummary(report, stats)
		testutil.Equal(t, 1, len(summary.Rows))
		testutil.Equal(t, "Auth users", summary.Rows[0].Label)
	})

	t.Run("with skipped and errors", func(t *testing.T) {
		t.Parallel()
		report := &migrate.AnalysisReport{AuthUsers: 10}
		stats := &MigrationStats{Users: 10, Skipped: 2, Errors: []string{"err1"}}
		summary := BuildValidationSummary(report, stats)
		testutil.Equal(t, 2, len(summary.Warnings))
		testutil.Contains(t, summary.Warnings[0], "2 items skipped")
		testutil.Contains(t, summary.Warnings[1], "1 errors occurred")
	})

	t.Run("adds mismatch warning", func(t *testing.T) {
		t.Parallel()
		report := &migrate.AnalysisReport{AuthUsers: 10}
		stats := &MigrationStats{Users: 8}
		summary := BuildValidationSummary(report, stats)

		testutil.Equal(t, 1, len(summary.Warnings))
		testutil.Contains(t, summary.Warnings[0], "Auth users count mismatch")
		testutil.Contains(t, summary.Warnings[0], "source=10")
		testutil.Contains(t, summary.Warnings[0], "target=8")
	})

	t.Run("rtdb only has matching collection counts", func(t *testing.T) {
		t.Parallel()
		report := &migrate.AnalysisReport{
			Tables:  2,
			Records: 50,
		}
		stats := &MigrationStats{
			RTDBNodes:   2,
			RTDBRecords: 50,
		}
		summary := BuildValidationSummary(report, stats)

		var collectionsFound bool
		for _, row := range summary.Rows {
			if row.Label == "Collections" {
				collectionsFound = true
				testutil.Equal(t, 2, row.SourceCount)
				testutil.Equal(t, 2, row.TargetCount)
			}
		}
		testutil.True(t, collectionsFound, "expected Collections row")
		testutil.Equal(t, 0, len(summary.Warnings))
	})
}

func TestAnalyzeAuthCounts(t *testing.T) {
	// Test the actual Analyze() method with a real auth export file.
	t.Parallel()

	export := FirebaseAuthExport{
		Users: []FirebaseUser{
			{LocalID: "u1", Email: "a@b.com", PasswordHash: "hash", Salt: "salt"}, // email user
			{LocalID: "u2", Email: "b@b.com", ProviderInfo: []ProviderInfo{ // email + OAuth
				{ProviderID: "google.com", RawID: "g1"},
				{ProviderID: "github.com", RawID: "gh1"},
			}},
			{LocalID: "u3"}, // anonymous — skipped
			{LocalID: "u4", ProviderInfo: []ProviderInfo{{ProviderID: "phone"}}}, // phone-only — skipped
			{LocalID: "u5", Email: "disabled@b.com", Disabled: true},             // disabled — skipped
		},
		HashConfig: FirebaseHashConfig{Algorithm: "SCRYPT", Rounds: 8, MemCost: 14},
	}
	path := makeAuthExportFile(t, export)

	m := &Migrator{opts: MigrationOptions{AuthExportPath: path}}
	report, err := m.Analyze(context.TODO())
	testutil.NoError(t, err)
	testutil.Equal(t, "Firebase", report.SourceType)
	testutil.Equal(t, 2, report.AuthUsers)  // u1, u2 (u3 anon, u4 phone, u5 disabled)
	testutil.Equal(t, 2, report.OAuthLinks) // u2:google, u2:github
}

func TestAnalyzeFirestoreCounts(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// Create a Firestore export directory with 2 collections.
	usersJSON := `[{"__name__":"projects/p/databases/(default)/documents/users/u1","fields":{"name":{"stringValue":"Alice"}}}]`
	postsJSON := `[{"__name__":"projects/p/databases/(default)/documents/posts/p1","fields":{}},{"__name__":"projects/p/databases/(default)/documents/posts/p2","fields":{}}]`
	os.WriteFile(dir+"/users.json", []byte(usersJSON), 0644)
	os.WriteFile(dir+"/posts.json", []byte(postsJSON), 0644)

	m := &Migrator{opts: MigrationOptions{FirestoreExportPath: dir}}
	report, err := m.Analyze(context.TODO())
	testutil.NoError(t, err)
	testutil.Equal(t, 2, report.Tables)  // users, posts
	testutil.Equal(t, 3, report.Records) // 1 user + 2 posts
}

func TestAnalyzeCountsMatchMigrateFiltering(t *testing.T) {
	// Verify that Analyze() counts only active email users for auth and OAuth,
	// skipping anonymous, phone-only, and disabled users.
	t.Parallel()

	export := FirebaseAuthExport{
		Users: []FirebaseUser{
			{LocalID: "u1", Email: "a@b.com", ProviderInfo: []ProviderInfo{
				{ProviderID: "google.com", RawID: "g1", Email: "a@b.com"},
			}},
			{LocalID: "u2", Email: "b@b.com"}, // email user, no OAuth
			{LocalID: "u3"},                   // anonymous — should be skipped
			{LocalID: "u4", ProviderInfo: []ProviderInfo{ // phone-only — should be skipped
				{ProviderID: "phone", RawID: "p1"},
			}},
			{LocalID: "u5", Email: "c@b.com", ProviderInfo: []ProviderInfo{ // email user with 2 OAuth providers
				{ProviderID: "google.com", RawID: "g2"},
				{ProviderID: "github.com", RawID: "gh1"},
			}},
			{LocalID: "u6", Email: "disabled@b.com", Disabled: true, ProviderInfo: []ProviderInfo{ // disabled — should be skipped
				{ProviderID: "google.com", RawID: "g3"},
			}},
		},
		HashConfig: FirebaseHashConfig{Algorithm: "SCRYPT", Rounds: 8, MemCost: 14},
	}
	path := makeAuthExportFile(t, export)

	m := &Migrator{opts: MigrationOptions{AuthExportPath: path}}
	report, err := m.Analyze(context.TODO())
	testutil.NoError(t, err)
	testutil.Equal(t, 3, report.AuthUsers)  // u1, u2, u5 (u3 anon, u4 phone, u6 disabled)
	testutil.Equal(t, 3, report.OAuthLinks) // u1:google, u5:google, u5:github (u6 skipped)
}

func TestPrintStats(t *testing.T) {
	t.Parallel()
	t.Run("shows all fields when non-zero", func(t *testing.T) {
		t.Parallel()
		var buf strings.Builder
		m := &Migrator{
			output: &buf,
			stats: MigrationStats{
				Users:       10,
				OAuthLinks:  5,
				Collections: 3,
				Documents:   500,
				Skipped:     2,
			},
		}
		m.printStats()
		out := buf.String()
		testutil.Contains(t, out, "Users:       10")
		testutil.Contains(t, out, "OAuth:       5")
		testutil.Contains(t, out, "Collections: 3")
		testutil.Contains(t, out, "Documents:   500")
		testutil.Contains(t, out, "Skipped:     2")
	})

	t.Run("omits zero fields", func(t *testing.T) {
		t.Parallel()
		var buf strings.Builder
		m := &Migrator{
			output: &buf,
			stats:  MigrationStats{Users: 5},
		}
		m.printStats()
		out := buf.String()
		testutil.Contains(t, out, "Users:       5")
		testutil.False(t, strings.Contains(out, "OAuth:"), "should omit zero OAuth")
		testutil.False(t, strings.Contains(out, "Collections:"), "should omit zero Collections")
		testutil.False(t, strings.Contains(out, "Documents:"), "should omit zero Documents")
		testutil.False(t, strings.Contains(out, "Skipped:"), "should omit zero Skipped")
	})

	t.Run("shows errors", func(t *testing.T) {
		t.Parallel()
		var buf strings.Builder
		m := &Migrator{
			output: &buf,
			stats: MigrationStats{
				Errors: []string{"something failed"},
			},
		}
		m.printStats()
		out := buf.String()
		testutil.Contains(t, out, "Errors:      1")
		testutil.Contains(t, out, "something failed")
	})
}

func TestParseEpochMs(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
		year  int
	}{
		{"valid epoch", "1700000000000", 2023},
		{"empty string", "", time.Now().Year()},
		{"invalid", "not-a-number", time.Now().Year()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := parseEpochMs(tt.input)
			testutil.Equal(t, tt.year, result.Year())
		})
	}
}

// writeTestFile is a helper that creates a file in a temp directory.
func writeTestFile(t *testing.T, dir, name, content string) (string, error) {
	t.Helper()
	path := dir + "/" + name
	return path, os.WriteFile(path, []byte(content), 0644)
}
