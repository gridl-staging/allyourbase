// Package cli This file implements the Appwrite-to-PostgreSQL migration CLI command with export analysis, validation, and configurable data migration workflows.
package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/allyourbase/ayb/internal/appwritemigrate"
	"github.com/allyourbase/ayb/internal/migrate"
	"github.com/spf13/cobra"
)

type appwriteMigrator interface {
	Analyze(context.Context) (*migrate.AnalysisReport, error)
	Migrate(context.Context) (*appwritemigrate.MigrationStats, error)
	Close() error
}

var newAppwriteMigrator = func(opts appwritemigrate.MigrationOptions) (appwriteMigrator, error) {
	return appwritemigrate.NewMigrator(opts)
}

var buildAppwriteValidationSummary = appwritemigrate.BuildValidationSummary

var migrateAppwriteCmd = &cobra.Command{
	Use:   "appwrite",
	Short: "Migrate schema and data from Appwrite export",
	RunE:  runMigrateAppwrite,
}

func init() {
	migrateCmd.AddCommand(migrateAppwriteCmd)

	migrateAppwriteCmd.Flags().String("export", "", "Path to Appwrite database export JSON")
	migrateAppwriteCmd.Flags().String("database-url", "", "AYB PostgreSQL connection URL (target)")
	migrateAppwriteCmd.Flags().Bool("dry-run", false, "Preview what would be migrated without making changes")
	migrateAppwriteCmd.Flags().Bool("verbose", false, "Show detailed progress")
	migrateAppwriteCmd.Flags().Bool("skip-rls", false, "Skip RLS policy generation")
	migrateAppwriteCmd.Flags().Bool("skip-data", false, "Skip document data migration")
	migrateAppwriteCmd.Flags().BoolP("yes", "y", false, "Skip confirmation prompt")
	migrateAppwriteCmd.Flags().Bool("json", false, "Output migration stats as JSON")

	migrateAppwriteCmd.MarkFlagRequired("export")
	migrateAppwriteCmd.MarkFlagRequired("database-url")
}

// runMigrateAppwrite orchestrates the Appwrite-to-PostgreSQL migration. It analyzes the export, prompts for confirmation (unless -y or dry-run is set), performs the migration, and outputs results as JSON or a summary.
func runMigrateAppwrite(cmd *cobra.Command, args []string) error {
	exportPath, _ := cmd.Flags().GetString("export")
	databaseURL, _ := cmd.Flags().GetString("database-url")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	verbose, _ := cmd.Flags().GetBool("verbose")
	skipRLS, _ := cmd.Flags().GetBool("skip-rls")
	skipData, _ := cmd.Flags().GetBool("skip-data")
	yes, _ := cmd.Flags().GetBool("yes")
	jsonOut, _ := cmd.Flags().GetBool("json")

	var progress migrate.ProgressReporter
	if jsonOut {
		progress = migrate.NopReporter{}
	} else {
		progress = migrate.NewCLIReporter(os.Stderr)
	}

	migrator, err := newAppwriteMigrator(appwritemigrate.MigrationOptions{
		ExportPath:  exportPath,
		DatabaseURL: databaseURL,
		DryRun:      dryRun,
		Verbose:     verbose,
		Progress:    progress,
		SkipRLS:     skipRLS,
		SkipData:    skipData,
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
		summary := buildAppwriteValidationSummary(report, stats)
		summary.PrintSummary(os.Stderr)
	}

	if jsonOut {
		return json.NewEncoder(os.Stdout).Encode(stats)
	}
	return nil
}
