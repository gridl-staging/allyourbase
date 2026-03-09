package cli

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/fbmigrate"
	"github.com/allyourbase/ayb/internal/migrate"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/spf13/cobra"
)

type fakeFirebaseMigrator struct {
	analyzeFn func(context.Context) (*migrate.AnalysisReport, error)
	migrateFn func(context.Context) (*fbmigrate.MigrationStats, error)
	closeFn   func() error
}

func (f fakeFirebaseMigrator) Analyze(ctx context.Context) (*migrate.AnalysisReport, error) {
	if f.analyzeFn != nil {
		return f.analyzeFn(ctx)
	}
	return &migrate.AnalysisReport{SourceType: "Firebase"}, nil
}

func (f fakeFirebaseMigrator) Migrate(ctx context.Context) (*fbmigrate.MigrationStats, error) {
	if f.migrateFn != nil {
		return f.migrateFn(ctx)
	}
	return &fbmigrate.MigrationStats{}, nil
}

func (f fakeFirebaseMigrator) Close() error {
	if f.closeFn != nil {
		return f.closeFn()
	}
	return nil
}

func newFirebaseTestCommand(t *testing.T, values map[string]string) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{}
	cmd.Flags().String("auth-export", "", "")
	cmd.Flags().String("firestore-export", "", "")
	cmd.Flags().String("rtdb-export", "", "")
	cmd.Flags().String("storage-export", "", "")
	cmd.Flags().String("storage-path", "", "")
	cmd.Flags().String("database-url", "", "")
	cmd.Flags().Bool("dry-run", false, "")
	cmd.Flags().Bool("verbose", false, "")
	cmd.Flags().Bool("yes", false, "")
	cmd.Flags().Bool("json", false, "")
	for k, v := range values {
		testutil.NoError(t, cmd.Flags().Set(k, v))
	}
	return cmd
}

func TestRunMigrateFirebasePreflightPromptAndSummary(t *testing.T) {
	oldFactory := newFirebaseMigrator
	oldSummary := buildFirebaseValidationSummary
	t.Cleanup(func() {
		newFirebaseMigrator = oldFactory
		buildFirebaseValidationSummary = oldSummary
	})

	callOrder := make([]string, 0, 2)
	newFirebaseMigrator = func(opts fbmigrate.MigrationOptions) (firebaseMigrator, error) {
		return fakeFirebaseMigrator{
			analyzeFn: func(context.Context) (*migrate.AnalysisReport, error) {
				callOrder = append(callOrder, "analyze")
				return &migrate.AnalysisReport{SourceType: "Firebase", AuthUsers: 3}, nil
			},
			migrateFn: func(context.Context) (*fbmigrate.MigrationStats, error) {
				callOrder = append(callOrder, "migrate")
				return &fbmigrate.MigrationStats{Users: 3}, nil
			},
		}, nil
	}
	buildFirebaseValidationSummary = func(report *migrate.AnalysisReport, stats *fbmigrate.MigrationStats) *migrate.ValidationSummary {
		return &migrate.ValidationSummary{
			SourceLabel: "Firebase (source)",
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
	_, err = w.WriteString("yes\n")
	testutil.NoError(t, err)
	testutil.NoError(t, w.Close())
	os.Stdin = r
	t.Cleanup(func() {
		os.Stdin = oldStdin
		_ = r.Close()
	})

	cmd := newFirebaseTestCommand(t, map[string]string{
		"auth-export":  "auth.json",
		"database-url": "postgres://target",
	})

	output := captureStderr(t, func() {
		err := runMigrateFirebase(cmd, nil)
		testutil.NoError(t, err)
	})

	if !reflect.DeepEqual(callOrder, []string{"analyze", "migrate"}) {
		t.Fatalf("unexpected call order: %v", callOrder)
	}
	testutil.Contains(t, output, "AYB Migration Report")
	testutil.Contains(t, output, "Proceed? [Y/n]")
	testutil.Contains(t, output, "Validation Summary")
}

func TestRunMigrateFirebaseJSONOutputsStats(t *testing.T) {
	oldFactory := newFirebaseMigrator
	t.Cleanup(func() { newFirebaseMigrator = oldFactory })

	newFirebaseMigrator = func(opts fbmigrate.MigrationOptions) (firebaseMigrator, error) {
		return fakeFirebaseMigrator{
			analyzeFn: func(context.Context) (*migrate.AnalysisReport, error) {
				return &migrate.AnalysisReport{SourceType: "Firebase", AuthUsers: 1}, nil
			},
			migrateFn: func(context.Context) (*fbmigrate.MigrationStats, error) {
				return &fbmigrate.MigrationStats{Users: 1}, nil
			},
		}, nil
	}

	cmd := newFirebaseTestCommand(t, map[string]string{
		"auth-export":  "auth.json",
		"database-url": "postgres://target",
		"json":         "true",
	})

	var stdout string
	stderr := captureStderr(t, func() {
		stdout = captureStdout(t, func() {
			err := runMigrateFirebase(cmd, nil)
			testutil.NoError(t, err)
		})
	})

	testutil.False(t, strings.Contains(stderr, "Proceed? [Y/n]"), "json mode must skip prompt")

	var stats fbmigrate.MigrationStats
	testutil.NoError(t, json.Unmarshal([]byte(stdout), &stats))
	testutil.Equal(t, 1, stats.Users)
}
