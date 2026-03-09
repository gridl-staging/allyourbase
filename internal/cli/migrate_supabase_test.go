package cli

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/migrate"
	"github.com/allyourbase/ayb/internal/sbmigrate"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/spf13/cobra"
)

type fakeSupabaseMigrator struct {
	analyzeFn func(context.Context) (*migrate.AnalysisReport, error)
	migrateFn func(context.Context) (*sbmigrate.MigrationStats, error)
	closeFn   func() error
}

func (f fakeSupabaseMigrator) Analyze(ctx context.Context) (*migrate.AnalysisReport, error) {
	if f.analyzeFn != nil {
		return f.analyzeFn(ctx)
	}
	return &migrate.AnalysisReport{SourceType: "Supabase"}, nil
}

func (f fakeSupabaseMigrator) Migrate(ctx context.Context) (*sbmigrate.MigrationStats, error) {
	if f.migrateFn != nil {
		return f.migrateFn(ctx)
	}
	return &sbmigrate.MigrationStats{}, nil
}

func (f fakeSupabaseMigrator) Close() error {
	if f.closeFn != nil {
		return f.closeFn()
	}
	return nil
}

func newSupabaseTestCommand(t *testing.T, values map[string]string) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{}
	cmd.Flags().String("source-url", "", "")
	cmd.Flags().String("database-url", "", "")
	cmd.Flags().String("storage-export", "", "")
	cmd.Flags().String("storage-path", "", "")
	cmd.Flags().Bool("dry-run", false, "")
	cmd.Flags().Bool("force", false, "")
	cmd.Flags().Bool("verbose", false, "")
	cmd.Flags().Bool("skip-rls", false, "")
	cmd.Flags().Bool("skip-oauth", false, "")
	cmd.Flags().Bool("skip-data", false, "")
	cmd.Flags().Bool("skip-storage", false, "")
	cmd.Flags().Bool("include-anonymous", false, "")
	cmd.Flags().Bool("yes", false, "")
	cmd.Flags().Bool("json", false, "")
	for k, v := range values {
		testutil.NoError(t, cmd.Flags().Set(k, v))
	}
	return cmd
}

func TestMigrateSupabaseForwardsStorageFlags(t *testing.T) {
	oldFactory := newSupabaseMigrator
	t.Cleanup(func() { newSupabaseMigrator = oldFactory })

	var got sbmigrate.MigrationOptions
	newSupabaseMigrator = func(opts sbmigrate.MigrationOptions) (supabaseMigrator, error) {
		got = opts
		return fakeSupabaseMigrator{}, nil
	}

	cmd := newSupabaseTestCommand(t, map[string]string{
		"source-url":     "postgres://source",
		"database-url":   "postgres://target",
		"storage-export": "./export",
		"storage-path":   "./storage",
		"skip-storage":   "true",
		"yes":            "true",
	})

	_ = captureStderr(t, func() {
		err := runMigrateSupabase(cmd, nil)
		testutil.NoError(t, err)
	})
	testutil.Equal(t, "./export", got.StorageExportPath)
	testutil.Equal(t, "./storage", got.StoragePath)
	testutil.True(t, got.SkipStorage, "expected skip-storage to be forwarded")
}

func TestRunMigrateSupabasePreflightPromptAndSummary(t *testing.T) {
	oldFactory := newSupabaseMigrator
	oldSummary := buildSupabaseValidationSummary
	t.Cleanup(func() {
		newSupabaseMigrator = oldFactory
		buildSupabaseValidationSummary = oldSummary
	})

	callOrder := make([]string, 0, 2)
	newSupabaseMigrator = func(opts sbmigrate.MigrationOptions) (supabaseMigrator, error) {
		return fakeSupabaseMigrator{
			analyzeFn: func(context.Context) (*migrate.AnalysisReport, error) {
				callOrder = append(callOrder, "analyze")
				return &migrate.AnalysisReport{SourceType: "Supabase", AuthUsers: 2}, nil
			},
			migrateFn: func(context.Context) (*sbmigrate.MigrationStats, error) {
				callOrder = append(callOrder, "migrate")
				return &sbmigrate.MigrationStats{Users: 2}, nil
			},
		}, nil
	}
	buildSupabaseValidationSummary = func(report *migrate.AnalysisReport, stats *sbmigrate.MigrationStats) *migrate.ValidationSummary {
		return &migrate.ValidationSummary{
			SourceLabel: "Supabase (source)",
			TargetLabel: "AYB (target)",
			Rows: []migrate.ValidationRow{{
				Label:       "Auth users",
				SourceCount: report.AuthUsers,
				TargetCount: stats.Users,
			}},
		}
	}

	oldStdin := os.Stdin
	r, w, err := os.Pipe()
	testutil.NoError(t, err)
	_, err = w.WriteString("y\n")
	testutil.NoError(t, err)
	testutil.NoError(t, w.Close())
	os.Stdin = r
	t.Cleanup(func() {
		os.Stdin = oldStdin
		_ = r.Close()
	})

	cmd := newSupabaseTestCommand(t, map[string]string{
		"source-url":   "postgres://source",
		"database-url": "postgres://target",
	})

	output := captureStderr(t, func() {
		err := runMigrateSupabase(cmd, nil)
		testutil.NoError(t, err)
	})

	if !reflect.DeepEqual(callOrder, []string{"analyze", "migrate"}) {
		t.Fatalf("unexpected call order: %v", callOrder)
	}
	testutil.Contains(t, output, "AYB Migration Report")
	testutil.Contains(t, output, "Proceed? [Y/n]")
	testutil.Contains(t, output, "Validation Summary")
}

func TestRunMigrateSupabaseJSONOutputsStats(t *testing.T) {
	oldFactory := newSupabaseMigrator
	t.Cleanup(func() { newSupabaseMigrator = oldFactory })

	newSupabaseMigrator = func(opts sbmigrate.MigrationOptions) (supabaseMigrator, error) {
		return fakeSupabaseMigrator{
			analyzeFn: func(context.Context) (*migrate.AnalysisReport, error) {
				return &migrate.AnalysisReport{SourceType: "Supabase", AuthUsers: 1}, nil
			},
			migrateFn: func(context.Context) (*sbmigrate.MigrationStats, error) {
				return &sbmigrate.MigrationStats{Users: 1}, nil
			},
		}, nil
	}

	cmd := newSupabaseTestCommand(t, map[string]string{
		"source-url":   "postgres://source",
		"database-url": "postgres://target",
		"json":         "true",
	})

	var stdout string
	stderr := captureStderr(t, func() {
		stdout = captureStdout(t, func() {
			err := runMigrateSupabase(cmd, nil)
			testutil.NoError(t, err)
		})
	})

	testutil.False(t, strings.Contains(stderr, "Proceed? [Y/n]"), "json mode must skip prompt")

	var stats sbmigrate.MigrationStats
	testutil.NoError(t, json.Unmarshal([]byte(stdout), &stats))
	testutil.Equal(t, 1, stats.Users)
}

func TestRunMigrateSupabaseSummaryIgnoresSkippedScopes(t *testing.T) {
	oldFactory := newSupabaseMigrator
	oldSummary := buildSupabaseValidationSummary
	t.Cleanup(func() {
		newSupabaseMigrator = oldFactory
		buildSupabaseValidationSummary = oldSummary
	})

	var gotReport *migrate.AnalysisReport
	newSupabaseMigrator = func(opts sbmigrate.MigrationOptions) (supabaseMigrator, error) {
		return fakeSupabaseMigrator{
			analyzeFn: func(context.Context) (*migrate.AnalysisReport, error) {
				return &migrate.AnalysisReport{
					SourceType:  "Supabase",
					Tables:      4,
					Views:       2,
					Records:     40,
					AuthUsers:   5,
					OAuthLinks:  3,
					RLSPolicies: 2,
					Files:       7,
				}, nil
			},
			migrateFn: func(context.Context) (*sbmigrate.MigrationStats, error) {
				return &sbmigrate.MigrationStats{Users: 5}, nil
			},
		}, nil
	}
	buildSupabaseValidationSummary = func(report *migrate.AnalysisReport, stats *sbmigrate.MigrationStats) *migrate.ValidationSummary {
		got := *report
		gotReport = &got
		return &migrate.ValidationSummary{
			SourceLabel: "Supabase (source)",
			TargetLabel: "AYB (target)",
		}
	}

	cmd := newSupabaseTestCommand(t, map[string]string{
		"source-url":   "postgres://source",
		"database-url": "postgres://target",
		"skip-data":    "true",
		"skip-oauth":   "true",
		"skip-rls":     "true",
		"yes":          "true",
	})

	_ = captureStderr(t, func() {
		err := runMigrateSupabase(cmd, nil)
		testutil.NoError(t, err)
	})

	if gotReport == nil {
		t.Fatal("expected validation summary to receive a report")
	}
	testutil.Equal(t, 0, gotReport.Tables)
	testutil.Equal(t, 0, gotReport.Views)
	testutil.Equal(t, 0, gotReport.Records)
	testutil.Equal(t, 0, gotReport.OAuthLinks)
	testutil.Equal(t, 0, gotReport.RLSPolicies)
	testutil.Equal(t, 0, gotReport.Files)
	testutil.Equal(t, 5, gotReport.AuthUsers)
}
