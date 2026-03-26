// Package cli provides database management subcommands for backup, restore, reset, and seed operations.
package cli

import "github.com/spf13/cobra"

var dbCmd = &cobra.Command{
	Use:   "db",
	Short: "Database management commands",
}

var dbSeedCmd = &cobra.Command{
	Use:   "seed [file]",
	Short: "Run a SQL seed file against the database",
	Long: `Execute a SQL seed file against the configured database in a single transaction.
If no file argument is given, uses the database.seed_file path from ayb.toml.

Examples:
  ayb db seed seed.sql
  ayb db seed --database-url postgres://localhost/mydb`,
	Args: cobra.MaximumNArgs(1),
	RunE: runDBSeed,
}

var dbResetCmd = &cobra.Command{
	Use:   "reset",
	Short: "Drop all user tables, re-run migrations, and optionally re-seed",
	Long: `Drop all user-created tables, types, and extensions (preserving _ayb_* system
tables and the migrations tracking table), then re-run migrate up. If
database.seed_file is configured, the seed file is executed after migrations.

Requires --yes to confirm.

Examples:
  ayb db reset --yes
  ayb db reset --yes --database-url postgres://localhost/mydb`,
	RunE: runDBReset,
}

var dbBackupCmd = &cobra.Command{
	Use:   "backup",
	Short: "Trigger an on-demand backup to S3",
	Long: `Trigger an on-demand database backup that uploads a compressed dump to S3.
Requires backup to be enabled in ayb.toml ([backup] section).

Examples:
  ayb db backup
  ayb db backup --output json`,
	RunE: runDBBackupS3,
}

var dbBackupListCmd = &cobra.Command{
	Use:   "list",
	Short: "List backups from the _ayb_backups metadata table",
	Long: `List backup records from the _ayb_backups metadata table.

Examples:
  ayb db backup list
  ayb db backup list --status completed --limit 10
  ayb db backup list --output json`,
	RunE: runDBBackupList,
}

var dbRestoreCmd = &cobra.Command{
	Use:   "restore",
	Short: "Restore the PostgreSQL database",
	Long: `Restore a database.

  From an S3 backup:
    ayb db restore --from <backup-id>
    ayb db restore --from <s3-key>

  From a local file (legacy):
    ayb db restore <path>`,
	RunE: runDBRestore,
}

func init() {
	dbBackupCmd.Flags().String("database-url", "", "Database URL (overrides config)")
	dbBackupCmd.Flags().String("config", "", "Path to ayb.toml config file")
	dbBackupCmd.AddCommand(dbBackupListCmd)

	dbBackupListCmd.Flags().String("status", "", "Filter by status (running|completed|failed)")
	dbBackupListCmd.Flags().Int("limit", 20, "Maximum number of records to return")
	dbBackupListCmd.Flags().String("database-url", "", "Database URL (overrides config)")
	dbBackupListCmd.Flags().String("config", "", "Path to ayb.toml config file")

	dbRestoreCmd.Flags().String("from", "", "Backup ID or S3 object key to restore from")
	dbRestoreCmd.Flags().String("database-url", "", "Database URL (overrides config)")
	dbRestoreCmd.Flags().String("config", "", "Path to ayb.toml config file")
	dbRestoreCmd.Flags().BoolP("yes", "y", false, "Skip confirmation prompt")

	dbSeedCmd.Flags().String("database-url", "", "Database URL (overrides config)")
	dbSeedCmd.Flags().String("config", "", "Path to ayb.toml config file")

	dbResetCmd.Flags().String("database-url", "", "Database URL (overrides config)")
	dbResetCmd.Flags().String("config", "", "Path to ayb.toml config file")
	dbResetCmd.Flags().BoolP("yes", "y", false, "Skip confirmation prompt")

	dbCmd.AddCommand(dbBackupCmd)
	dbCmd.AddCommand(dbRestoreCmd)
	dbCmd.AddCommand(dbSeedCmd)
	dbCmd.AddCommand(dbResetCmd)
}
