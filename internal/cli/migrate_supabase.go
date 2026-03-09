package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/allyourbase/ayb/internal/migrate"
	"github.com/allyourbase/ayb/internal/sbmigrate"
	"github.com/spf13/cobra"
)

type supabaseMigrator interface {
	Analyze(context.Context) (*migrate.AnalysisReport, error)
	Migrate(context.Context) (*sbmigrate.MigrationStats, error)
	Close() error
}

var newSupabaseMigrator = func(opts sbmigrate.MigrationOptions) (supabaseMigrator, error) {
	return sbmigrate.NewMigrator(opts)
}

var buildSupabaseValidationSummary = sbmigrate.BuildValidationSummary

var migrateSupabaseCmd = &cobra.Command{
	Use:   "supabase",
	Short: "Migrate data, auth users, and RLS policies from a Supabase database",
	Long: `Migrate data tables, auth users, OAuth identities, and RLS policies from a Supabase PostgreSQL database
(Supabase Cloud or self-hosted Supabase).

This command connects directly to the Supabase PostgreSQL database and migrates:
- Public schema tables → recreated in AYB with data streamed in batches
- auth.users → _ayb_users (preserves UUIDs, bcrypt passwords work with AYB auth)
- auth.identities → _ayb_oauth_accounts (Google, GitHub, etc.)
- RLS policies → rewritten from auth.uid() to AYB session variables

Example:
  ayb migrate supabase \
    --source-url postgres://postgres:pass@db.xxx.supabase.co:5432/postgres \
    --database-url postgres://localhost:5432/myapp

	Example (with storage files):
	  ayb migrate supabase \
	    --source-url postgres://postgres:pass@db.xxx.supabase.co:5432/postgres \
	    --database-url postgres://localhost:5432/myapp \
	    --storage-export ./supabase-storage-export \
	    --storage-path ./ayb_storage

	Example (self-hosted Supabase):
	  ayb migrate supabase \
	    --source-url postgres://postgres:pass@supabase-db.internal:5432/postgres \
	    --database-url postgres://localhost:5432/myapp

	Use the direct database connection (port 5432), not the connection pooler (port 6543).
Use --skip-data to migrate only auth and RLS (no data tables).
Use --skip-storage to skip file migration, or --storage-export to include storage.
Use -y/--yes to skip confirmation prompts and --json for machine-readable output.

The migration runs in a single transaction, so either everything succeeds or
nothing is changed. Use --dry-run to preview what would be migrated.`,
	RunE: runMigrateSupabase,
}

func init() {
	migrateCmd.AddCommand(migrateSupabaseCmd)

	migrateSupabaseCmd.Flags().String("source-url", "", "Supabase PostgreSQL connection URL (source)")
	migrateSupabaseCmd.Flags().String("database-url", "", "AYB PostgreSQL connection URL (target)")
	migrateSupabaseCmd.Flags().String("storage-export", "", "Path to exported Supabase storage directory")
	migrateSupabaseCmd.Flags().String("storage-path", "", "Destination directory for AYB storage files (default: ./ayb_storage)")
	migrateSupabaseCmd.Flags().Bool("dry-run", false, "Preview what would be migrated without making changes")
	migrateSupabaseCmd.Flags().Bool("force", false, "Allow migration when _ayb_users is not empty")
	migrateSupabaseCmd.Flags().Bool("verbose", false, "Show detailed progress")
	migrateSupabaseCmd.Flags().Bool("skip-rls", false, "Skip RLS policy rewriting")
	migrateSupabaseCmd.Flags().Bool("skip-oauth", false, "Skip OAuth identity migration")
	migrateSupabaseCmd.Flags().Bool("skip-data", false, "Skip data table migration (auth and RLS only)")
	migrateSupabaseCmd.Flags().Bool("skip-storage", false, "Skip storage file migration")
	migrateSupabaseCmd.Flags().Bool("include-anonymous", false, "Include anonymous Supabase users")
	migrateSupabaseCmd.Flags().BoolP("yes", "y", false, "Skip confirmation prompt")
	migrateSupabaseCmd.Flags().Bool("json", false, "Output migration stats as JSON")

	migrateSupabaseCmd.MarkFlagRequired("source-url")
	migrateSupabaseCmd.MarkFlagRequired("database-url")
}

func runMigrateSupabase(cmd *cobra.Command, args []string) error {
	sourceURL, _ := cmd.Flags().GetString("source-url")
	databaseURL, _ := cmd.Flags().GetString("database-url")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	force, _ := cmd.Flags().GetBool("force")
	verbose, _ := cmd.Flags().GetBool("verbose")
	storageExport, _ := cmd.Flags().GetString("storage-export")
	storagePath, _ := cmd.Flags().GetString("storage-path")
	skipRLS, _ := cmd.Flags().GetBool("skip-rls")
	skipOAuth, _ := cmd.Flags().GetBool("skip-oauth")
	skipData, _ := cmd.Flags().GetBool("skip-data")
	skipStorage, _ := cmd.Flags().GetBool("skip-storage")
	includeAnon, _ := cmd.Flags().GetBool("include-anonymous")
	yes, _ := cmd.Flags().GetBool("yes")
	jsonOut, _ := cmd.Flags().GetBool("json")

	var progress migrate.ProgressReporter
	if jsonOut {
		progress = migrate.NopReporter{}
	} else {
		progress = migrate.NewCLIReporter(os.Stderr)
	}

	migrator, err := newSupabaseMigrator(sbmigrate.MigrationOptions{
		SourceURL:         sourceURL,
		TargetURL:         databaseURL,
		StorageExportPath: storageExport,
		StoragePath:       storagePath,
		DryRun:            dryRun,
		Force:             force,
		Verbose:           verbose,
		SkipRLS:           skipRLS,
		SkipOAuth:         skipOAuth,
		SkipData:          skipData,
		SkipStorage:       skipStorage,
		IncludeAnonymous:  includeAnon,
		Progress:          progress,
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
		summaryReport := normalizeSupabaseSummaryReport(report, skipData, skipOAuth, skipRLS, skipStorage, storageExport)
		summary := buildSupabaseValidationSummary(summaryReport, stats)
		summary.PrintSummary(os.Stderr)
	}

	if jsonOut {
		return json.NewEncoder(os.Stdout).Encode(stats)
	}

	return nil
}

func normalizeSupabaseSummaryReport(
	report *migrate.AnalysisReport,
	skipData bool,
	skipOAuth bool,
	skipRLS bool,
	skipStorage bool,
	storageExport string,
) *migrate.AnalysisReport {
	if report == nil {
		return nil
	}

	normalized := *report
	if skipData {
		normalized.Tables = 0
		normalized.Views = 0
		normalized.Records = 0
	}
	if skipOAuth {
		normalized.OAuthLinks = 0
	}
	if skipRLS {
		normalized.RLSPolicies = 0
	}
	if skipStorage || storageExport == "" {
		normalized.Files = 0
		normalized.FileSizeBytes = 0
	}

	return &normalized
}
