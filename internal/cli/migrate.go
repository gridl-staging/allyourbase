package cli

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/migrations"
	"github.com/allyourbase/ayb/internal/postgres"
	"github.com/spf13/cobra"
)

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Manage database migrations",
	Long: `Manage user SQL migrations. Migrations are .sql files in the migrations
directory (default: ./migrations), applied in filename order.

Create a new migration:
  ayb migrate create add_posts_table

Apply pending migrations:
  ayb migrate up

Check migration status:
  ayb migrate status`,
}

var migrateUpCmd = &cobra.Command{
	Use:   "up",
	Short: "Apply all pending migrations",
	RunE:  runMigrateUp,
}

var migrateCreateCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create a new migration file",
	Args:  cobra.ExactArgs(1),
	RunE:  runMigrateCreate,
}

var migrateStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show migration status (applied/pending)",
	RunE:  runMigrateStatus,
}

func init() {
	migrateCmd.AddCommand(migrateUpCmd)
	migrateCmd.AddCommand(migrateCreateCmd)
	migrateCmd.AddCommand(migrateStatusCmd)

	for _, cmd := range []*cobra.Command{migrateUpCmd, migrateCreateCmd, migrateStatusCmd} {
		cmd.Flags().String("config", "", "Path to ayb.toml config file")
		cmd.Flags().String("migrations-dir", "", "Migrations directory (overrides config)")
	}
	migrateUpCmd.Flags().String("database-url", "", "PostgreSQL connection URL (overrides config)")
	migrateStatusCmd.Flags().String("database-url", "", "PostgreSQL connection URL (overrides config)")
}

func runMigrateCreate(cmd *cobra.Command, args []string) error {
	cfg, err := loadMigrateConfig(cmd)
	if err != nil {
		return err
	}

	dir := migrationsDir(cmd, cfg)
	runner := migrations.NewUserRunner(nil, dir, slog.New(slog.NewTextHandler(os.Stderr, nil)))

	path, err := runner.CreateFile(args[0])
	if err != nil {
		return fmt.Errorf("creating migration: %w", err)
	}
	fmt.Printf("Created migration: %s\n", path)
	return nil
}

func runMigrateUp(cmd *cobra.Command, args []string) error {
	cfg, err := loadMigrateConfig(cmd)
	if err != nil {
		return err
	}

	dir := migrationsDir(cmd, cfg)
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	pool, cleanup, err := connectForMigrate(cmd, cfg, logger)
	if err != nil {
		return err
	}
	defer cleanup()

	runner := migrations.NewUserRunner(pool.DB(), dir, logger)
	ctx := context.Background()

	if err := runner.Bootstrap(ctx); err != nil {
		return fmt.Errorf("bootstrapping: %w", err)
	}

	applied, err := runner.Up(ctx)
	if err != nil {
		return fmt.Errorf("applying migrations: %w", err)
	}

	if applied == 0 {
		fmt.Println("No pending migrations.")
	} else {
		fmt.Printf("Applied %d migration(s).\n", applied)
	}
	return nil
}

func runMigrateStatus(cmd *cobra.Command, args []string) error {
	cfg, err := loadMigrateConfig(cmd)
	if err != nil {
		return err
	}

	dir := migrationsDir(cmd, cfg)
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	pool, cleanup, err := connectForMigrate(cmd, cfg, logger)
	if err != nil {
		return err
	}
	defer cleanup()

	runner := migrations.NewUserRunner(pool.DB(), dir, logger)
	ctx := context.Background()

	if err := runner.Bootstrap(ctx); err != nil {
		return fmt.Errorf("bootstrapping: %w", err)
	}

	statuses, err := runner.Status(ctx)
	if err != nil {
		return fmt.Errorf("getting status: %w", err)
	}

	if len(statuses) == 0 {
		fmt.Printf("No migrations found in %s\n", dir)
		return nil
	}

	fmt.Printf("%-50s  %s\n", "MIGRATION", "STATUS")
	fmt.Printf("%-50s  %s\n", "---------", "------")
	for _, s := range statuses {
		if s.AppliedAt != nil {
			fmt.Printf("%-50s  applied %s\n", s.Name, s.AppliedAt.Format(time.RFC3339))
		} else {
			fmt.Printf("%-50s  pending\n", s.Name)
		}
	}
	return nil
}

func loadMigrateConfig(cmd *cobra.Command) (*config.Config, error) {
	configPath, _ := cmd.Flags().GetString("config")
	cfg, err := config.Load(configPath, nil)
	if err != nil {
		return nil, fmt.Errorf("loading config: %w", err)
	}
	return cfg, nil
}

func migrationsDir(cmd *cobra.Command, cfg *config.Config) string {
	if dir, _ := cmd.Flags().GetString("migrations-dir"); dir != "" {
		return dir
	}
	return cfg.Database.MigrationsDir
}

func connectForMigrate(cmd *cobra.Command, cfg *config.Config, logger *slog.Logger) (*postgres.Pool, func(), error) {
	dbURL := cfg.Database.URL
	if v, _ := cmd.Flags().GetString("database-url"); v != "" {
		dbURL = v
	}
	if dbURL == "" {
		return nil, nil, fmt.Errorf("no database URL configured (set database.url in ayb.toml, AYB_DATABASE_URL env, or --database-url flag)")
	}

	ctx := context.Background()
	pool, err := postgres.New(ctx, postgres.Config{
		URL:             dbURL,
		MaxConns:        5,
		MinConns:        1,
		HealthCheckSecs: 0,
	}, logger)
	if err != nil {
		return nil, nil, fmt.Errorf("connecting to database: %w", err)
	}
	return pool, func() { pool.Close() }, nil
}
