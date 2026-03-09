package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/allyourbase/ayb/internal/config"
	"github.com/spf13/cobra"
)

var dbCmd = &cobra.Command{
	Use:   "db",
	Short: "Database management commands",
}

var dbBackupCmd = &cobra.Command{
	Use:   "backup",
	Short: "Backup the PostgreSQL database using pg_dump",
	Long: `Create a database backup using pg_dump.
Requires pg_dump to be installed and accessible in PATH.
The database URL is read from config (ayb.toml or AYB_DATABASE_URL).

Examples:
  ayb db backup
  ayb db backup --output ./backups/my-backup.sql
  ayb db backup --format custom --output ./backups/my-backup.dump`,
	RunE: runDBBackup,
}

var dbRestoreCmd = &cobra.Command{
	Use:   "restore <path>",
	Short: "Restore a PostgreSQL database from a backup",
	Long: `Restore a database from a pg_dump backup file.
Requires psql (for SQL backups) or pg_restore (for custom/tar format) in PATH.

Examples:
  ayb db restore ./backups/my-backup.sql
  ayb db restore ./backups/my-backup.dump`,
	Args: cobra.ExactArgs(1),
	RunE: runDBRestore,
}

func init() {
	dbBackupCmd.Flags().String("output", "", "Output file path (default: ayb-backup-{timestamp}.sql)")
	dbBackupCmd.Flags().String("format", "plain", "Backup format: plain, custom, tar, directory")
	dbBackupCmd.Flags().String("database-url", "", "Database URL (overrides config)")
	dbBackupCmd.Flags().String("config", "", "Path to ayb.toml config file")

	dbRestoreCmd.Flags().String("database-url", "", "Database URL (overrides config)")
	dbRestoreCmd.Flags().String("config", "", "Path to ayb.toml config file")

	dbCmd.AddCommand(dbBackupCmd)
	dbCmd.AddCommand(dbRestoreCmd)
}

func resolveDBURL(cmd *cobra.Command) (string, error) {
	// Check flag first.
	if url, _ := cmd.Flags().GetString("database-url"); url != "" {
		return url, nil
	}
	// Check env var.
	if url := os.Getenv("AYB_DATABASE_URL"); url != "" {
		return url, nil
	}
	// Fall back to config.
	configPath, _ := cmd.Flags().GetString("config")
	if configPath == "" {
		configPath = "ayb.toml"
	}
	cfg, err := config.Load(configPath, nil)
	if err != nil {
		return "", fmt.Errorf("loading config: %w", err)
	}
	if cfg.Database.URL == "" {
		return "", fmt.Errorf("no database URL configured (set --database-url, AYB_DATABASE_URL, or database.url in ayb.toml)")
	}
	return cfg.Database.URL, nil
}

func runDBBackup(cmd *cobra.Command, args []string) error {
	dbURL, err := resolveDBURL(cmd)
	if err != nil {
		return err
	}

	format, _ := cmd.Flags().GetString("format")
	output, _ := cmd.Flags().GetString("output")

	// Validate format.
	validFormats := map[string]string{
		"plain": "p", "custom": "c", "tar": "t", "directory": "d",
		"p": "p", "c": "c", "t": "t", "d": "d",
	}
	pgFormat, ok := validFormats[format]
	if !ok {
		return fmt.Errorf("invalid format %q: must be plain, custom, tar, or directory", format)
	}

	// Default output path.
	if output == "" {
		ext := ".sql"
		if pgFormat == "c" {
			ext = ".dump"
		} else if pgFormat == "t" {
			ext = ".tar"
		}
		output = fmt.Sprintf("ayb-backup-%s%s", time.Now().Format("20060102-150405"), ext)
	}

	// Ensure output directory exists.
	if dir := filepath.Dir(output); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("creating output directory: %w", err)
		}
	}

	// Check pg_dump exists.
	pgDump, err := exec.LookPath("pg_dump")
	if err != nil {
		return fmt.Errorf("pg_dump not found in PATH: install PostgreSQL client tools")
	}

	cmdArgs := []string{
		"--dbname=" + dbURL,
		"--format=" + pgFormat,
		"--file=" + output,
	}

	fmt.Printf("Backing up database to %s (format: %s)...\n", output, format)

	pgCmd := exec.Command(pgDump, cmdArgs...)
	pgCmd.Stdout = os.Stdout
	pgCmd.Stderr = os.Stderr
	if err := pgCmd.Run(); err != nil {
		return fmt.Errorf("pg_dump failed: %w", err)
	}

	// Report file size.
	if info, err := os.Stat(output); err == nil {
		fmt.Printf("Backup complete: %s (%d bytes)\n", output, info.Size())
	} else {
		fmt.Printf("Backup complete: %s\n", output)
	}
	return nil
}

func runDBRestore(cmd *cobra.Command, args []string) error {
	dbURL, err := resolveDBURL(cmd)
	if err != nil {
		return err
	}

	inputPath := args[0]

	// Verify file exists.
	info, err := os.Stat(inputPath)
	if err != nil {
		return fmt.Errorf("backup file not found: %s", inputPath)
	}

	fmt.Printf("Restoring database from %s (%d bytes)...\n", inputPath, info.Size())

	// Detect format: .dump and .tar use pg_restore, .sql uses psql.
	ext := filepath.Ext(inputPath)
	if ext == ".dump" || ext == ".tar" {
		pgRestore, lookErr := exec.LookPath("pg_restore")
		if lookErr != nil {
			return fmt.Errorf("pg_restore not found in PATH: install PostgreSQL client tools")
		}
		pgCmd := exec.Command(pgRestore,
			"--dbname="+dbURL,
			"--clean",
			"--if-exists",
			inputPath,
		)
		pgCmd.Stdout = os.Stdout
		pgCmd.Stderr = os.Stderr
		if err := pgCmd.Run(); err != nil {
			return fmt.Errorf("pg_restore failed: %w", err)
		}
	} else {
		psql, lookErr := exec.LookPath("psql")
		if lookErr != nil {
			return fmt.Errorf("psql not found in PATH: install PostgreSQL client tools")
		}
		pgCmd := exec.Command(psql, "--dbname="+dbURL, "--file="+inputPath)
		pgCmd.Stdout = os.Stdout
		pgCmd.Stderr = os.Stderr
		if err := pgCmd.Run(); err != nil {
			return fmt.Errorf("psql failed: %w", err)
		}
	}

	fmt.Println("Restore complete.")
	return nil
}
