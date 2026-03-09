package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/allyourbase/ayb/internal/fbmigrate"
	"github.com/allyourbase/ayb/internal/migrate"
	"github.com/spf13/cobra"
)

type firebaseMigrator interface {
	Analyze(context.Context) (*migrate.AnalysisReport, error)
	Migrate(context.Context) (*fbmigrate.MigrationStats, error)
	Close() error
}

var newFirebaseMigrator = func(opts fbmigrate.MigrationOptions) (firebaseMigrator, error) {
	return fbmigrate.NewMigrator(opts)
}

var buildFirebaseValidationSummary = fbmigrate.BuildValidationSummary

var migrateFirebaseCmd = &cobra.Command{
	Use:   "firebase",
	Short: "Migrate auth users, Firestore, RTDB, and storage from Firebase",
	Long: `Migrate auth users, OAuth identities, Firestore collections, Realtime Database nodes,
and Cloud Storage files from a Firebase project.

This command reads offline Firebase export files (no GCP SDK required) and migrates:
- Auth users → _ayb_users (Firebase scrypt hashes preserved, progressive re-hash on login)
- OAuth providers → _ayb_oauth_accounts (Google, GitHub, etc.)
- Firestore collections → PostgreSQL tables with JSONB data + GIN indexes
- Realtime Database nodes → PostgreSQL tables with JSONB data + GIN indexes
- Cloud Storage files → AYB local storage layout

Export your Firebase data:
  firebase auth:export auth-export.json --format=json
  firebase database:get / > rtdb-export.json

Export Firestore and Storage via the firebase-admin SDK (see _dev/migration_test/).

Example (auth + Firestore only):
  ayb migrate firebase \
    --auth-export auth-export.json \
    --firestore-export ./firestore-data/ \
    --database-url postgres://localhost:5432/myapp

Example (full migration):
  ayb migrate firebase \
    --auth-export auth-export.json \
    --firestore-export ./firestore-data/ \
    --rtdb-export rtdb-export.json \
    --storage-export ./storage-data/ \
    --database-url postgres://localhost:5432/myapp

At least one of --auth-export, --firestore-export, --rtdb-export, or --storage-export is required.
Use --dry-run to preview what would be migrated.
Use -y/--yes to skip confirmation prompts and --json for machine-readable output.`,
	RunE: runMigrateFirebase,
}

func init() {
	migrateCmd.AddCommand(migrateFirebaseCmd)

	migrateFirebaseCmd.Flags().String("auth-export", "", "Path to Firebase auth export JSON file (firebase auth:export)")
	migrateFirebaseCmd.Flags().String("firestore-export", "", "Path to Firestore export directory")
	migrateFirebaseCmd.Flags().String("rtdb-export", "", "Path to Realtime Database export JSON file (firebase database:get /)")
	migrateFirebaseCmd.Flags().String("storage-export", "", "Path to Cloud Storage export directory (bucket subdirectories)")
	migrateFirebaseCmd.Flags().String("storage-path", "", "Destination directory for AYB storage files (default: ./ayb_storage)")
	migrateFirebaseCmd.Flags().String("database-url", "", "AYB PostgreSQL connection URL (target)")
	migrateFirebaseCmd.Flags().Bool("dry-run", false, "Preview what would be migrated without making changes")
	migrateFirebaseCmd.Flags().Bool("verbose", false, "Show detailed progress")
	migrateFirebaseCmd.Flags().BoolP("yes", "y", false, "Skip confirmation prompt")
	migrateFirebaseCmd.Flags().Bool("json", false, "Output stats as JSON")

	migrateFirebaseCmd.MarkFlagRequired("database-url")
}

func runMigrateFirebase(cmd *cobra.Command, args []string) error {
	authExport, _ := cmd.Flags().GetString("auth-export")
	firestoreExport, _ := cmd.Flags().GetString("firestore-export")
	rtdbExport, _ := cmd.Flags().GetString("rtdb-export")
	storageExport, _ := cmd.Flags().GetString("storage-export")
	storagePath, _ := cmd.Flags().GetString("storage-path")
	databaseURL, _ := cmd.Flags().GetString("database-url")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	verbose, _ := cmd.Flags().GetBool("verbose")
	yes, _ := cmd.Flags().GetBool("yes")
	jsonOut, _ := cmd.Flags().GetBool("json")

	if authExport == "" && firestoreExport == "" && rtdbExport == "" && storageExport == "" {
		return fmt.Errorf("at least one of --auth-export, --firestore-export, --rtdb-export, or --storage-export is required")
	}

	var progress migrate.ProgressReporter
	if jsonOut {
		progress = migrate.NopReporter{}
	} else {
		progress = migrate.NewCLIReporter(os.Stderr)
	}

	migrator, err := newFirebaseMigrator(fbmigrate.MigrationOptions{
		AuthExportPath:      authExport,
		FirestoreExportPath: firestoreExport,
		RTDBExportPath:      rtdbExport,
		StorageExportPath:   storageExport,
		StoragePath:         storagePath,
		DatabaseURL:         databaseURL,
		DryRun:              dryRun,
		Verbose:             verbose,
		Progress:            progress,
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
		summary := buildFirebaseValidationSummary(report, stats)
		summary.PrintSummary(os.Stderr)
	}

	if jsonOut {
		return json.NewEncoder(os.Stdout).Encode(stats)
	}

	return nil
}
