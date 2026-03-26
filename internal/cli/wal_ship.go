// Package cli Wal_ship implements the wal-ship CLI command for uploading PostgreSQL WAL segments to point-in-time recovery archive storage. It manages configuration loading, S3 connectivity, and segment shipping.
package cli

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/allyourbase/ayb/internal/backup"
	"github.com/allyourbase/ayb/internal/config"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/cobra"
)

var walShipCmd = &cobra.Command{
	Use:   "wal-ship <wal-file-path> <wal-file-name>",
	Short: "Ship a single PostgreSQL WAL segment to PITR archive storage",
	Args:  cobra.ExactArgs(2),
	RunE:  runWALShip,
}

func init() {
	walShipCmd.Flags().String("config", "", "Path to ayb.toml config file")
}

// runWALShip executes the wal-ship command to ship a single PostgreSQL WAL segment to PITR archive storage. It loads configuration from ayb.toml, validates PITR is enabled and database URL is set, establishes an S3 connection for the archive, and invokes the shipper to upload the segment.
func runWALShip(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	configPath, _ := cmd.Flags().GetString("config")
	if configPath == "" {
		configPath = "ayb.toml"
	}

	cfg, err := config.Load(configPath, nil)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	if !cfg.Backup.PITR.Enabled {
		return fmt.Errorf("WAL shipping is not enabled (set [backup.pitr] enabled = true)")
	}
	if cfg.Database.URL == "" {
		return fmt.Errorf("no database URL configured (set database.url in ayb.toml)")
	}

	endpoint := cfg.Backup.Endpoint
	if endpoint == "" {
		endpoint = fmt.Sprintf("s3.%s.amazonaws.com", cfg.Backup.Region)
	}

	store, err := backup.NewS3Store(ctx, backup.S3Config{
		Endpoint:   endpoint,
		Bucket:     cfg.Backup.PITR.ArchiveBucket,
		Region:     cfg.Backup.Region,
		AccessKey:  cfg.Backup.AccessKey,
		SecretKey:  cfg.Backup.SecretKey,
		Encryption: cfg.Backup.Encryption,
		UseSSL:     cfg.Backup.UseSSL,
	})
	if err != nil {
		return fmt.Errorf("initialising PITR S3 store: %w", err)
	}

	pool, err := pgxpool.New(ctx, cfg.Database.URL)
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer pool.Close()

	projectID := os.Getenv("AYB_PROJECT_ID")
	if projectID == "" {
		projectID = "default"
	}
	databaseID := extractDBName(cfg.Database.URL)

	shipper := backup.NewWALShipper(
		store,
		backup.NewPgWALSegmentRepo(pool),
		backup.PITRConfig{ArchivePrefix: cfg.Backup.PITR.ArchivePrefix},
		projectID,
		databaseID,
		backup.NewLogNotifier(slog.Default()),
	)

	if err := shipper.Ship(ctx, args[0], args[1]); err != nil {
		return fmt.Errorf("shipping WAL segment: %w", err)
	}
	return nil
}
