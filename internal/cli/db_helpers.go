// Package cli Helper functions for database connection management and URL parsing in CLI commands.
package cli

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/allyourbase/ayb/internal/backup"
	"github.com/allyourbase/ayb/internal/config"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/cobra"
)

// resolveDBURL returns the database URL by checking the --database-url flag, the AYB_DATABASE_URL environment variable, or the database.url field in the configuration file, in that order of priority. It returns an error if no database URL is found.
func resolveDBURL(cmd *cobra.Command) (string, error) {
	if dbURL, _ := cmd.Flags().GetString("database-url"); dbURL != "" {
		return dbURL, nil
	}

	if dbURL := os.Getenv("AYB_DATABASE_URL"); dbURL != "" {
		return dbURL, nil
	}

	cfg, err := loadDBConfig(cmd)
	if err != nil {
		return "", err
	}
	if cfg.Database.URL == "" {
		return "", fmt.Errorf("no database URL configured (set --database-url, AYB_DATABASE_URL, or database.url in ayb.toml)")
	}
	return cfg.Database.URL, nil
}

func openPool(ctx context.Context, dbURL string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		return nil, fmt.Errorf("connecting to database: %w", err)
	}
	return pool, nil
}

func connectForDB(dbURL string) (*pgxpool.Pool, func(), error) {
	poolConfig, err := pgxpool.ParseConfig(dbURL)
	if err != nil {
		return nil, nil, fmt.Errorf("parsing database URL: %w", err)
	}

	pool, err := pgxpool.NewWithConfig(context.Background(), poolConfig)
	if err != nil {
		return nil, nil, fmt.Errorf("connecting to database: %w", err)
	}
	return pool, func() { pool.Close() }, nil
}

func s3StoreFromConfig(ctx context.Context, cfg *config.Config) (*backup.S3Store, error) {
	endpoint := cfg.Backup.Endpoint
	if endpoint == "" {
		endpoint = fmt.Sprintf("s3.%s.amazonaws.com", cfg.Backup.Region)
	}

	return backup.NewS3Store(ctx, backup.S3Config{
		Endpoint:  endpoint,
		Bucket:    cfg.Backup.Bucket,
		Region:    cfg.Backup.Region,
		AccessKey: cfg.Backup.AccessKey,
		SecretKey: cfg.Backup.SecretKey,
		UseSSL:    cfg.Backup.UseSSL,
	})
}

// extractDBName extracts the database name from a database URL, handling URLs with schemes, authority sections, paths, and query parameters. It returns "default" if a database name cannot be determined.
func extractDBName(dbURL string) string {
	rest := dbURL
	if schemeIndex := strings.Index(rest, "://"); schemeIndex >= 0 {
		rest = rest[schemeIndex+3:]
		if pathIndex := strings.IndexByte(rest, '/'); pathIndex >= 0 {
			rest = rest[pathIndex+1:]
		} else {
			return "default"
		}
	} else if slashIndex := strings.LastIndex(rest, "/"); slashIndex >= 0 {
		rest = rest[slashIndex+1:]
	} else {
		return "default"
	}

	if queryIndex := strings.IndexByte(rest, '?'); queryIndex >= 0 {
		rest = rest[:queryIndex]
	}
	if rest == "" {
		return "default"
	}

	return rest
}

func loadDBConfig(cmd *cobra.Command) (*config.Config, error) {
	configPath, _ := cmd.Flags().GetString("config")
	if configPath == "" {
		configPath = "ayb.toml"
	}

	cfg, err := config.Load(configPath, nil)
	if err != nil {
		return nil, fmt.Errorf("loading config: %w", err)
	}
	return cfg, nil
}
