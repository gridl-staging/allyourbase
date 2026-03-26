// Package cli This file implements the NHost migration command, which migrates schema and data from NHost-hosted databases (Hasura metadata and PostgreSQL dump) to AYB.
package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/allyourbase/ayb/internal/migrate"
	"github.com/allyourbase/ayb/internal/nhostmigrate"
	"github.com/spf13/cobra"
)

type nhostMigrator interface {
	Analyze(context.Context) (*migrate.AnalysisReport, error)
	Migrate(context.Context) (*nhostmigrate.MigrationStats, error)
	Close() error
}

var newNhostMigrator = func(opts nhostmigrate.MigrationOptions) (nhostMigrator, error) {
	return nhostmigrate.NewMigrator(opts)
}

var buildNhostValidationSummary = nhostmigrate.BuildValidationSummary

var migrateNhostCmd = &cobra.Command{
	Use:   "nhost",
	Short: "Migrate schema/data from NHost exports (pg_dump + Hasura metadata)",
	RunE:  runMigrateNhost,
}

func init() {
	migrateCmd.AddCommand(migrateNhostCmd)

	migrateNhostCmd.Flags().String("hasura-metadata", "", "Path to Hasura metadata directory (v3)")
	migrateNhostCmd.Flags().String("pg-dump", "", "Path to PostgreSQL pg_dump SQL file")
	migrateNhostCmd.Flags().String("database-url", "", "AYB PostgreSQL connection URL (target)")
	migrateNhostCmd.Flags().Bool("dry-run", false, "Preview what would be migrated without making changes")
	migrateNhostCmd.Flags().Bool("verbose", false, "Show detailed progress")
	migrateNhostCmd.Flags().Bool("skip-rls", false, "Skip RLS policy generation")
	migrateNhostCmd.Flags().BoolP("yes", "y", false, "Skip confirmation prompt")
	migrateNhostCmd.Flags().Bool("json", false, "Output migration stats as JSON")

	migrateNhostCmd.MarkFlagRequired("hasura-metadata")
	migrateNhostCmd.MarkFlagRequired("pg-dump")
	migrateNhostCmd.MarkFlagRequired("database-url")
}

// runMigrateNhost executes a schema and data migration from NHost exports to an AYB database. It validates the plan, prompts for confirmation, and outputs statistics as text or JSON.
func runMigrateNhost(cmd *cobra.Command, args []string) error {
	hasuraMetadata, _ := cmd.Flags().GetString("hasura-metadata")
	pgDump, _ := cmd.Flags().GetString("pg-dump")
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

	migrator, err := newNhostMigrator(nhostmigrate.MigrationOptions{
		HasuraMetadataPath: hasuraMetadata,
		PgDumpPath:         pgDump,
		DatabaseURL:        databaseURL,
		DryRun:             dryRun,
		Verbose:            verbose,
		Progress:           progress,
		SkipRLS:            skipRLS,
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
		summary := buildNhostValidationSummary(report, stats)
		summary.PrintSummary(os.Stderr)
	}

	if jsonOut {
		return json.NewEncoder(os.Stdout).Encode(stats)
	}
	return nil
}
