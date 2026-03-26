// Package cli File provides the CLI command for migrating Directus schema snapshots to PostgreSQL databases.
package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/allyourbase/ayb/internal/directusmigrate"
	"github.com/allyourbase/ayb/internal/migrate"
	"github.com/spf13/cobra"
)

type directusMigrator interface {
	Analyze(context.Context) (*migrate.AnalysisReport, error)
	Migrate(context.Context) (*directusmigrate.MigrationStats, error)
	Close() error
}

var newDirectusMigrator = func(opts directusmigrate.MigrationOptions) (directusMigrator, error) {
	return directusmigrate.NewMigrator(opts)
}

var buildDirectusValidationSummary = directusmigrate.BuildValidationSummary

var migrateDirectusCmd = &cobra.Command{
	Use:   "directus",
	Short: "Migrate schema from Directus snapshot export",
	RunE:  runMigrateDirectus,
}

func init() {
	migrateCmd.AddCommand(migrateDirectusCmd)

	migrateDirectusCmd.Flags().String("snapshot", "", "Path to Directus /schema/snapshot JSON export")
	migrateDirectusCmd.Flags().String("database-url", "", "AYB PostgreSQL connection URL (target)")
	migrateDirectusCmd.Flags().Bool("dry-run", false, "Preview what would be migrated without making changes")
	migrateDirectusCmd.Flags().Bool("verbose", false, "Show detailed progress")
	migrateDirectusCmd.Flags().Bool("skip-rls", false, "Skip RLS policy generation")
	migrateDirectusCmd.Flags().BoolP("yes", "y", false, "Skip confirmation prompt")
	migrateDirectusCmd.Flags().Bool("json", false, "Output migration stats as JSON")

	migrateDirectusCmd.MarkFlagRequired("snapshot")
	migrateDirectusCmd.MarkFlagRequired("database-url")
}

// runMigrateDirectus is the Cobra command handler for the migrate directus subcommand. It analyzes a Directus schema snapshot, displays the analysis, optionally prompts for confirmation, executes the migration against the target database, and outputs results as formatted text or JSON.
func runMigrateDirectus(cmd *cobra.Command, args []string) error {
	snapshot, _ := cmd.Flags().GetString("snapshot")
	databaseURL, _ := cmd.Flags().GetString("database-url")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	verbose, _ := cmd.Flags().GetBool("verbose")
	skipRLS, _ := cmd.Flags().GetBool("skip-rls")
	yes, _ := cmd.Flags().GetBool("yes")
	jsonOut, _ := cmd.Flags().GetBool("json")

	var progress migrate.ProgressReporter
	if jsonOut {
		progress = migrate.NopReporter{}
	} else {
		progress = migrate.NewCLIReporter(os.Stderr)
	}

	migrator, err := newDirectusMigrator(directusmigrate.MigrationOptions{
		SnapshotPath: snapshot,
		DatabaseURL:  databaseURL,
		DryRun:       dryRun,
		Verbose:      verbose,
		Progress:     progress,
		SkipRLS:      skipRLS,
	})
	if err != nil {
		return fmt.Errorf("failed to create migrator: %w", err)
	}
	defer migrator.Close()

	ctx := context.Background()
	report, err := migrator.Analyze(ctx)
	if err != nil {
		return fmt.Errorf("analysis failed: %w", err)
	}

	if !jsonOut {
		report.PrintReport(os.Stderr)
		if !yes && !dryRun {
			fmt.Fprint(os.Stderr, "  Proceed? [Y/n] ")
			reader := bufio.NewReader(os.Stdin)
			answer, _ := reader.ReadString('\n')
			answer = strings.TrimSpace(strings.ToLower(answer))
			if answer != "" && answer != "y" && answer != "yes" {
				fmt.Fprintln(os.Stderr, "  Migration cancelled.")
				return nil
			}
		}
		fmt.Fprintln(os.Stderr)
	}

	stats, err := migrator.Migrate(ctx)
	if err != nil {
		return fmt.Errorf("migration failed: %w", err)
	}

	if !jsonOut && !dryRun {
		summary := buildDirectusValidationSummary(report, stats)
		summary.PrintSummary(os.Stderr)
	}

	if jsonOut {
		return json.NewEncoder(os.Stdout).Encode(stats)
	}
	return nil
}
