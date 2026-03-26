package cli

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/directusmigrate"
	"github.com/allyourbase/ayb/internal/migrate"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/spf13/cobra"
)

type fakeDirectusMigrator struct {
	analyzeFn func(context.Context) (*migrate.AnalysisReport, error)
	migrateFn func(context.Context) (*directusmigrate.MigrationStats, error)
	closeFn   func() error
}

func (f fakeDirectusMigrator) Analyze(ctx context.Context) (*migrate.AnalysisReport, error) {
	if f.analyzeFn != nil {
		return f.analyzeFn(ctx)
	}
	return &migrate.AnalysisReport{SourceType: "Directus"}, nil
}

func (f fakeDirectusMigrator) Migrate(ctx context.Context) (*directusmigrate.MigrationStats, error) {
	if f.migrateFn != nil {
		return f.migrateFn(ctx)
	}
	return &directusmigrate.MigrationStats{}, nil
}

func (f fakeDirectusMigrator) Close() error {
	if f.closeFn != nil {
		return f.closeFn()
	}
	return nil
}

func newDirectusTestCommand(t *testing.T, values map[string]string) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{}
	cmd.Flags().String("snapshot", "", "")
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

func TestRunMigrateDirectusPreflightPromptAndSummary(t *testing.T) {
	oldFactory := newDirectusMigrator
	oldSummary := buildDirectusValidationSummary
	t.Cleanup(func() {
		newDirectusMigrator = oldFactory
		buildDirectusValidationSummary = oldSummary
	})

	callOrder := make([]string, 0, 2)
	newDirectusMigrator = func(opts directusmigrate.MigrationOptions) (directusMigrator, error) {
		return fakeDirectusMigrator{
			analyzeFn: func(context.Context) (*migrate.AnalysisReport, error) {
				callOrder = append(callOrder, "analyze")
				return &migrate.AnalysisReport{SourceType: "Directus", Tables: 2}, nil
			},
			migrateFn: func(context.Context) (*directusmigrate.MigrationStats, error) {
				callOrder = append(callOrder, "migrate")
				return &directusmigrate.MigrationStats{Collections: 2}, nil
			},
		}, nil
	}
	buildDirectusValidationSummary = func(report *migrate.AnalysisReport, stats *directusmigrate.MigrationStats) *migrate.ValidationSummary {
		return &migrate.ValidationSummary{SourceLabel: "Directus (source)", TargetLabel: "AYB (target)"}
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

	cmd := newDirectusTestCommand(t, map[string]string{"snapshot": "snapshot.json", "database-url": "postgres://target"})
	output := captureStderr(t, func() {
		err := runMigrateDirectus(cmd, nil)
		testutil.NoError(t, err)
	})

	if !reflect.DeepEqual(callOrder, []string{"analyze", "migrate"}) {
		t.Fatalf("unexpected call order: %v", callOrder)
	}
	testutil.Contains(t, output, "Proceed? [Y/n]")
	testutil.Contains(t, output, "Validation Summary")
}

func TestRunMigrateDirectusYesSkipsPrompt(t *testing.T) {
	oldFactory := newDirectusMigrator
	t.Cleanup(func() { newDirectusMigrator = oldFactory })

	newDirectusMigrator = func(opts directusmigrate.MigrationOptions) (directusMigrator, error) {
		return fakeDirectusMigrator{}, nil
	}

	cmd := newDirectusTestCommand(t, map[string]string{"snapshot": "snapshot.json", "database-url": "postgres://target", "yes": "true"})
	output := captureStderr(t, func() {
		err := runMigrateDirectus(cmd, nil)
		testutil.NoError(t, err)
	})
	testutil.False(t, strings.Contains(output, "Proceed? [Y/n]"), "--yes should skip prompt")
}

func TestRunMigrateDirectusJSONOutputsStats(t *testing.T) {
	oldFactory := newDirectusMigrator
	t.Cleanup(func() { newDirectusMigrator = oldFactory })

	newDirectusMigrator = func(opts directusmigrate.MigrationOptions) (directusMigrator, error) {
		return fakeDirectusMigrator{
			migrateFn: func(context.Context) (*directusmigrate.MigrationStats, error) {
				return &directusmigrate.MigrationStats{Collections: 2}, nil
			},
		}, nil
	}

	cmd := newDirectusTestCommand(t, map[string]string{"snapshot": "snapshot.json", "database-url": "postgres://target", "json": "true"})
	var stdout string
	stderr := captureStderr(t, func() {
		stdout = captureStdout(t, func() {
			err := runMigrateDirectus(cmd, nil)
			testutil.NoError(t, err)
		})
	})
	testutil.False(t, strings.Contains(stderr, "Proceed? [Y/n]"), "json mode must skip prompt")

	var stats directusmigrate.MigrationStats
	testutil.NoError(t, json.Unmarshal([]byte(stdout), &stats))
	testutil.Equal(t, 2, stats.Collections)
}
