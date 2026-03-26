package cli

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/migrate"
	"github.com/allyourbase/ayb/internal/nhostmigrate"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/spf13/cobra"
)

type fakeNhostMigrator struct {
	analyzeFn func(context.Context) (*migrate.AnalysisReport, error)
	migrateFn func(context.Context) (*nhostmigrate.MigrationStats, error)
	closeFn   func() error
}

func (f fakeNhostMigrator) Analyze(ctx context.Context) (*migrate.AnalysisReport, error) {
	if f.analyzeFn != nil {
		return f.analyzeFn(ctx)
	}
	return &migrate.AnalysisReport{SourceType: "NHost"}, nil
}

func (f fakeNhostMigrator) Migrate(ctx context.Context) (*nhostmigrate.MigrationStats, error) {
	if f.migrateFn != nil {
		return f.migrateFn(ctx)
	}
	return &nhostmigrate.MigrationStats{}, nil
}

func (f fakeNhostMigrator) Close() error {
	if f.closeFn != nil {
		return f.closeFn()
	}
	return nil
}

func newNhostTestCommand(t *testing.T, values map[string]string) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{}
	cmd.Flags().String("hasura-metadata", "", "")
	cmd.Flags().String("pg-dump", "", "")
	cmd.Flags().String("database-url", "", "")
	cmd.Flags().Bool("dry-run", false, "")
	cmd.Flags().Bool("verbose", false, "")
	cmd.Flags().Bool("skip-rls", false, "")
	cmd.Flags().Bool("yes", false, "")
	cmd.Flags().Bool("json", false, "")
	for k, v := range values {
		testutil.NoError(t, cmd.Flags().Set(k, v))
	}
	return cmd
}

func TestRunMigrateNhostPreflightPromptAndSummary(t *testing.T) {
	oldFactory := newNhostMigrator
	oldSummary := buildNhostValidationSummary
	t.Cleanup(func() {
		newNhostMigrator = oldFactory
		buildNhostValidationSummary = oldSummary
	})

	callOrder := make([]string, 0, 2)
	newNhostMigrator = func(opts nhostmigrate.MigrationOptions) (nhostMigrator, error) {
		return fakeNhostMigrator{
			analyzeFn: func(context.Context) (*migrate.AnalysisReport, error) {
				callOrder = append(callOrder, "analyze")
				return &migrate.AnalysisReport{SourceType: "NHost", Tables: 2}, nil
			},
			migrateFn: func(context.Context) (*nhostmigrate.MigrationStats, error) {
				callOrder = append(callOrder, "migrate")
				return &nhostmigrate.MigrationStats{Tables: 2}, nil
			},
		}, nil
	}
	buildNhostValidationSummary = func(report *migrate.AnalysisReport, stats *nhostmigrate.MigrationStats) *migrate.ValidationSummary {
		return &migrate.ValidationSummary{
			SourceLabel: "NHost (source)",
			TargetLabel: "AYB (target)",
			Rows:        []migrate.ValidationRow{{Label: "Tables", SourceCount: report.Tables, TargetCount: stats.Tables}},
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

	cmd := newNhostTestCommand(t, map[string]string{
		"hasura-metadata": "metadata",
		"pg-dump":         "dump.sql",
		"database-url":    "postgres://target",
	})

	output := captureStderr(t, func() {
		err := runMigrateNhost(cmd, nil)
		testutil.NoError(t, err)
	})

	if !reflect.DeepEqual(callOrder, []string{"analyze", "migrate"}) {
		t.Fatalf("unexpected call order: %v", callOrder)
	}
	testutil.Contains(t, output, "Proceed? [Y/n]")
	testutil.Contains(t, output, "Validation Summary")
}

func TestRunMigrateNhostYesSkipsPrompt(t *testing.T) {
	oldFactory := newNhostMigrator
	t.Cleanup(func() { newNhostMigrator = oldFactory })

	newNhostMigrator = func(opts nhostmigrate.MigrationOptions) (nhostMigrator, error) {
		return fakeNhostMigrator{}, nil
	}

	cmd := newNhostTestCommand(t, map[string]string{
		"hasura-metadata": "metadata",
		"pg-dump":         "dump.sql",
		"database-url":    "postgres://target",
		"yes":             "true",
	})

	output := captureStderr(t, func() {
		err := runMigrateNhost(cmd, nil)
		testutil.NoError(t, err)
	})
	testutil.False(t, strings.Contains(output, "Proceed? [Y/n]"), "--yes should skip prompt")
}

func TestRunMigrateNhostJSONOutputsStats(t *testing.T) {
	oldFactory := newNhostMigrator
	t.Cleanup(func() { newNhostMigrator = oldFactory })

	newNhostMigrator = func(opts nhostmigrate.MigrationOptions) (nhostMigrator, error) {
		return fakeNhostMigrator{
			analyzeFn: func(context.Context) (*migrate.AnalysisReport, error) {
				return &migrate.AnalysisReport{SourceType: "NHost"}, nil
			},
			migrateFn: func(context.Context) (*nhostmigrate.MigrationStats, error) {
				return &nhostmigrate.MigrationStats{Tables: 2}, nil
			},
		}, nil
	}

	cmd := newNhostTestCommand(t, map[string]string{
		"hasura-metadata": "metadata",
		"pg-dump":         "dump.sql",
		"database-url":    "postgres://target",
		"json":            "true",
	})

	var stdout string
	stderr := captureStderr(t, func() {
		stdout = captureStdout(t, func() {
			err := runMigrateNhost(cmd, nil)
			testutil.NoError(t, err)
		})
	})
	testutil.False(t, strings.Contains(stderr, "Proceed? [Y/n]"), "json mode must skip prompt")

	var stats nhostmigrate.MigrationStats
	testutil.NoError(t, json.Unmarshal([]byte(stdout), &stats))
	testutil.Equal(t, 2, stats.Tables)
}
