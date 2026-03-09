package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/allyourbase/ayb/internal/migrate"
	"github.com/allyourbase/ayb/internal/pbmigrate"
	"github.com/spf13/cobra"
)

var migratePocketbaseCmd = &cobra.Command{
	Use:   "pocketbase",
	Short: "Import data from PocketBase SQLite database",
	Long: `Import collections, data, and RLS policies from a PocketBase database.

This command reads PocketBase's data.db SQLite file and migrates:
- Collections -> PostgreSQL tables
- Collection fields -> table columns
- API rules -> RLS policies
- Records -> table data
- Auth users -> _ayb_users
- Files -> AYB storage (optional)

Example:
  ayb migrate pocketbase --source ./pb_data --database-url postgres://...
  ayb migrate pocketbase --source ./pb_data  # uses managed Postgres

The migration runs in a single transaction, so either everything succeeds or
nothing is changed. Use --dry-run to preview what would be migrated.`,
	RunE: runMigratePocketbase,
}

func init() {
	migrateCmd.AddCommand(migratePocketbaseCmd)

	migratePocketbaseCmd.Flags().String("source", "", "Path to PocketBase data directory (pb_data)")
	migratePocketbaseCmd.Flags().String("database-url", "", "PostgreSQL connection URL (default: managed Postgres)")
	migratePocketbaseCmd.Flags().Bool("dry-run", false, "Show what would be migrated without making changes")
	migratePocketbaseCmd.Flags().Bool("skip-files", false, "Skip file migration (only migrate schema and data)")
	migratePocketbaseCmd.Flags().Bool("force", false, "Allow migration to non-empty database")
	migratePocketbaseCmd.Flags().Bool("verbose", false, "Show detailed progress")
	migratePocketbaseCmd.Flags().BoolP("yes", "y", false, "Skip confirmation prompt")
	migratePocketbaseCmd.Flags().Bool("json", false, "Output migration stats as JSON")

	migratePocketbaseCmd.MarkFlagRequired("source")
}

func runMigratePocketbase(cmd *cobra.Command, args []string) error {
	sourcePath, _ := cmd.Flags().GetString("source")
	databaseURL, _ := cmd.Flags().GetString("database-url")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	skipFiles, _ := cmd.Flags().GetBool("skip-files")
	force, _ := cmd.Flags().GetBool("force")
	verbose, _ := cmd.Flags().GetBool("verbose")
	yes, _ := cmd.Flags().GetBool("yes")
	jsonOut, _ := cmd.Flags().GetBool("json")

	// Pre-flight analysis
	report, err := pbmigrate.Analyze(sourcePath)
	if err != nil {
		return fmt.Errorf("analysis failed: %w", err)
	}

	// Show pre-flight report and ask for confirmation (unless -y or --json)
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

	// If no database URL, report that managed Postgres is needed
	if databaseURL == "" {
		return fmt.Errorf("--database-url is required for standalone migration (use 'ayb start --from %s' for managed Postgres)", sourcePath)
	}

	// Set up progress reporter
	var progress migrate.ProgressReporter
	if jsonOut {
		progress = migrate.NopReporter{}
	} else {
		progress = migrate.NewCLIReporter(os.Stderr)
	}

	migrator, err := pbmigrate.NewMigrator(pbmigrate.MigrationOptions{
		SourcePath:  sourcePath,
		DatabaseURL: databaseURL,
		DryRun:      dryRun,
		SkipFiles:   skipFiles,
		Force:       force,
		Verbose:     verbose,
		Progress:    progress,
	})
	if err != nil {
		return fmt.Errorf("failed to create migrator: %w", err)
	}
	defer migrator.Close()

	ctx := context.Background()
	stats, err := migrator.Migrate(ctx)
	if err != nil {
		return fmt.Errorf("migration failed: %w", err)
	}

	// Post-migration validation summary
	if !jsonOut && !dryRun {
		summary := pbmigrate.BuildValidationSummary(report, stats)
		summary.PrintSummary(os.Stderr)
	}

	if jsonOut {
		return json.NewEncoder(os.Stdout).Encode(stats)
	}

	return nil
}
