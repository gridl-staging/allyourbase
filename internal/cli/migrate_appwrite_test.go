package cli

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/appwritemigrate"
	"github.com/allyourbase/ayb/internal/migrate"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/spf13/cobra"
)

type fakeAppwriteMigrator struct {
	analyzeFn func(context.Context) (*migrate.AnalysisReport, error)
	migrateFn func(context.Context) (*appwritemigrate.MigrationStats, error)
	closeFn   func() error
}

func (f fakeAppwriteMigrator) Analyze(ctx context.Context) (*migrate.AnalysisReport, error) {
	if f.analyzeFn != nil {
		return f.analyzeFn(ctx)
	}
	return &migrate.AnalysisReport{SourceType: "Appwrite"}, nil
}

func (f fakeAppwriteMigrator) Migrate(ctx context.Context) (*appwritemigrate.MigrationStats, error) {
	if f.migrateFn != nil {
		return f.migrateFn(ctx)
	}
	return &appwritemigrate.MigrationStats{}, nil
}

func (f fakeAppwriteMigrator) Close() error {
	if f.closeFn != nil {
		return f.closeFn()
	}
	return nil
}

func newAppwriteTestCommand(t *testing.T, values map[string]string) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{}
	cmd.Flags().String("export", "", "")
	cmd.Flags().String("database-url", "", "")
	cmd.Flags().Bool("dry-run", false, "")
	cmd.Flags().Bool("verbose", false, "")
	cmd.Flags().Bool("skip-rls", false, "")
	cmd.Flags().Bool("skip-data", false, "")
	cmd.Flags().Bool("yes", false, "")
	cmd.Flags().Bool("json", false, "")
	for k, v := range values {
		testutil.NoError(t, cmd.Flags().Set(k, v))
	}
	return cmd
}

func TestRunMigrateAppwritePreflightPromptAndSummary(t *testing.T) {
	oldFactory := newAppwriteMigrator
	oldSummary := buildAppwriteValidationSummary
	t.Cleanup(func() {
		newAppwriteMigrator = oldFactory
		buildAppwriteValidationSummary = oldSummary
	})

	callOrder := make([]string, 0, 2)
	newAppwriteMigrator = func(opts appwritemigrate.MigrationOptions) (appwriteMigrator, error) {
		return fakeAppwriteMigrator{
			analyzeFn: func(context.Context) (*migrate.AnalysisReport, error) {
				callOrder = append(callOrder, "analyze")
				return &migrate.AnalysisReport{SourceType: "Appwrite", Tables: 2}, nil
			},
			migrateFn: func(context.Context) (*appwritemigrate.MigrationStats, error) {
				callOrder = append(callOrder, "migrate")
				return &appwritemigrate.MigrationStats{Collections: 2}, nil
			},
		}, nil
	}
	buildAppwriteValidationSummary = func(report *migrate.AnalysisReport, stats *appwritemigrate.MigrationStats) *migrate.ValidationSummary {
		return &migrate.ValidationSummary{SourceLabel: "Appwrite (source)", TargetLabel: "AYB (target)"}
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

	cmd := newAppwriteTestCommand(t, map[string]string{"export": "export.json", "database-url": "postgres://target"})
	output := captureStderr(t, func() {
		err := runMigrateAppwrite(cmd, nil)
		testutil.NoError(t, err)
	})

	if !reflect.DeepEqual(callOrder, []string{"analyze", "migrate"}) {
		t.Fatalf("unexpected call order: %v", callOrder)
	}
	testutil.Contains(t, output, "Proceed? [Y/n]")
	testutil.Contains(t, output, "Validation Summary")
}

func TestRunMigrateAppwriteYesSkipsPrompt(t *testing.T) {
	oldFactory := newAppwriteMigrator
	t.Cleanup(func() { newAppwriteMigrator = oldFactory })

	newAppwriteMigrator = func(opts appwritemigrate.MigrationOptions) (appwriteMigrator, error) {
		return fakeAppwriteMigrator{}, nil
	}

	cmd := newAppwriteTestCommand(t, map[string]string{"export": "export.json", "database-url": "postgres://target", "yes": "true"})
	output := captureStderr(t, func() {
		err := runMigrateAppwrite(cmd, nil)
		testutil.NoError(t, err)
	})
	testutil.False(t, strings.Contains(output, "Proceed? [Y/n]"), "--yes should skip prompt")
}

func TestRunMigrateAppwriteJSONOutputsStats(t *testing.T) {
	oldFactory := newAppwriteMigrator
	t.Cleanup(func() { newAppwriteMigrator = oldFactory })

	newAppwriteMigrator = func(opts appwritemigrate.MigrationOptions) (appwriteMigrator, error) {
		return fakeAppwriteMigrator{
			migrateFn: func(context.Context) (*appwritemigrate.MigrationStats, error) {
				return &appwritemigrate.MigrationStats{Collections: 2}, nil
			},
		}, nil
	}

	cmd := newAppwriteTestCommand(t, map[string]string{"export": "export.json", "database-url": "postgres://target", "json": "true"})
	var stdout string
	stderr := captureStderr(t, func() {
		stdout = captureStdout(t, func() {
			err := runMigrateAppwrite(cmd, nil)
			testutil.NoError(t, err)
		})
	})
	testutil.False(t, strings.Contains(stderr, "Proceed? [Y/n]"), "json mode must skip prompt")

	var stats appwritemigrate.MigrationStats
	testutil.NoError(t, json.Unmarshal([]byte(stdout), &stats))
	testutil.Equal(t, 2, stats.Collections)
}
