// Package cli Provides CLI command handlers for database backup operations, including executing S3 backups and listing backup records.
package cli

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"text/tabwriter"

	"github.com/allyourbase/ayb/internal/backup"
	"github.com/spf13/cobra"
)

// runDBBackupS3 is a Cobra command handler that executes a manual database backup to S3, validating backup enablement in the configuration and outputting the result in JSON or table format.
func runDBBackupS3(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	cfg, err := loadDBConfig(cmd)
	if err != nil {
		return err
	}
	if !cfg.Backup.Enabled {
		return fmt.Errorf("backups are not enabled — set [backup] enabled = true in ayb.toml")
	}

	dbURL, err := resolveDBURL(cmd)
	if err != nil {
		return err
	}
	dbName := extractDBName(dbURL)

	store, err := s3StoreFromConfig(ctx, cfg)
	if err != nil {
		return fmt.Errorf("initialising S3 client: %w", err)
	}

	pool, err := openPool(ctx, dbURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	logger := slog.Default()
	repo := backup.NewRepository(pool)
	dumper := &backup.DumpRunner{}
	notifier := backup.NewLogNotifier(logger)

	engine := backup.NewEngine(
		backup.Config{
			Prefix:         cfg.Backup.Prefix,
			RetentionCount: cfg.Backup.RetentionCount,
			RetentionDays:  cfg.Backup.RetentionDays,
		},
		store, repo, dumper, notifier, logger, dbName, dbURL,
	)

	fmt.Fprintf(cmd.OutOrStdout(), "Starting backup of database %q...\n", dbName)
	result := engine.Run(ctx, "manual")

	if result.Status == "failed" {
		return fmt.Errorf("backup failed: %v", result.Err)
	}
	if result.Status == "skipped" {
		return fmt.Errorf("backup skipped: %v", result.Err)
	}

	if outputFormat(cmd) == "json" {
		encoder := json.NewEncoder(cmd.OutOrStdout())
		encoder.SetIndent("", "  ")
		return encoder.Encode(result)
	}

	writer := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	fmt.Fprintf(writer, "ID\t%s\n", result.BackupID)
	fmt.Fprintf(writer, "Status\t%s\n", result.Status)
	fmt.Fprintf(writer, "Object Key\t%s\n", result.ObjectKey)
	fmt.Fprintf(writer, "Size\t%d bytes\n", result.SizeBytes)
	fmt.Fprintf(writer, "Checksum\t%s\n", result.Checksum)
	return writer.Flush()
}

// runDBBackupList is a Cobra command handler that retrieves and displays database backup records with optional filtering by status and limit, supporting JSON and table output formats.
func runDBBackupList(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	dbURL, err := resolveDBURL(cmd)
	if err != nil {
		return err
	}

	pool, err := openPool(ctx, dbURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	statusFilter, _ := cmd.Flags().GetString("status")
	limit, _ := cmd.Flags().GetInt("limit")

	repo := backup.NewRepository(pool)
	records, total, err := repo.List(ctx, backup.ListFilter{
		Status: statusFilter,
		Limit:  limit,
	})
	if err != nil {
		return fmt.Errorf("listing backups: %w", err)
	}

	if outputFormat(cmd) == "json" {
		return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{
			"backups": records,
			"total":   total,
		})
	}

	if len(records) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No backup records found.")
		return nil
	}

	writer := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	fmt.Fprintln(writer, "ID\tDB\tSTATUS\tSIZE\tSTARTED")
	for _, record := range records {
		size := fmt.Sprintf("%d B", record.SizeBytes)
		if record.SizeBytes > 1<<20 {
			size = fmt.Sprintf("%.1f MB", float64(record.SizeBytes)/(1<<20))
		} else if record.SizeBytes > 1<<10 {
			size = fmt.Sprintf("%.1f KB", float64(record.SizeBytes)/(1<<10))
		}
		fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\n",
			record.ID, record.DBName, record.Status, size,
			record.StartedAt.Format("2006-01-02 15:04:05"),
		)
	}
	writer.Flush()
	fmt.Fprintf(cmd.OutOrStdout(), "\nShowing %d of %d backups.\n", len(records), total)
	return nil
}
