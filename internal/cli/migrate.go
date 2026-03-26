// Package cli Implements CLI commands for database migration management, including creation, application, status checking, and automated generation from schema diffs.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/migrations"
	"github.com/allyourbase/ayb/internal/postgres"
	"github.com/allyourbase/ayb/internal/schemadiff"
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

var migrateEncryptColumnCmd = &cobra.Command{
	Use:   "encrypt-column <table> <column>",
	Short: "Encrypt an existing plaintext column in-place using the configured vault",
	Args:  cobra.ExactArgs(2),
	RunE:  runMigrateEncryptColumn,
}

var migrateDiffCmd = &cobra.Command{
	Use:   "diff",
	Short: "Show schema changes since last baseline snapshot",
	Long: `Compare the live database schema against the last saved snapshot
(.schema-snapshot.json in the migrations directory). Prints a human-readable
summary of additions, removals, and modifications.

Examples:
  ayb migrate diff
  ayb migrate diff --output json
  ayb migrate diff --sql`,
	RunE: runMigrateDiff,
}

var migrateGenerateCmd = &cobra.Command{
	Use:   "generate <name>",
	Short: "Generate up/down SQL migration files from schema diff",
	Long: `Diff the live database against the last snapshot and generate SQL migration
files. Saves the current schema as the new snapshot baseline.

Examples:
  ayb migrate generate add_users_table
  ayb migrate generate --migrations-dir ./migrations add_posts_table`,
	Args: cobra.ExactArgs(1),
	RunE: runMigrateGenerate,
}

func init() {
	migrateCmd.AddCommand(migrateUpCmd)
	migrateCmd.AddCommand(migrateCreateCmd)
	migrateCmd.AddCommand(migrateStatusCmd)
	migrateCmd.AddCommand(migrateEncryptColumnCmd)
	migrateCmd.AddCommand(migrateDiffCmd)
	migrateCmd.AddCommand(migrateGenerateCmd)

	for _, cmd := range []*cobra.Command{migrateUpCmd, migrateCreateCmd, migrateStatusCmd} {
		cmd.Flags().String("config", "", "Path to ayb.toml config file")
		cmd.Flags().String("migrations-dir", "", "Migrations directory (overrides config)")
	}
	migrateEncryptColumnCmd.Flags().String("config", "", "Path to ayb.toml config file")
	migrateEncryptColumnCmd.Flags().String("database-url", "", "PostgreSQL connection URL (overrides config)")
	migrateUpCmd.Flags().String("database-url", "", "PostgreSQL connection URL (overrides config)")
	migrateStatusCmd.Flags().String("database-url", "", "PostgreSQL connection URL (overrides config)")

	for _, cmd := range []*cobra.Command{migrateDiffCmd, migrateGenerateCmd} {
		cmd.Flags().String("config", "", "Path to ayb.toml config file")
		cmd.Flags().String("database-url", "", "PostgreSQL connection URL (overrides config)")
		cmd.Flags().String("migrations-dir", "", "Migrations directory (overrides config)")
	}
	migrateDiffCmd.Flags().String("output", "table", "Output format: table or json")
	migrateDiffCmd.Flags().Bool("sql", false, "Print generated DDL SQL instead of change summary")
}

// runMigrateCreate creates a new, empty migration file with the given name in the migrations directory.
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

// runMigrateUp applies all pending migrations to the database and reports the count of applied migrations.
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

// runMigrateStatus reports the status of all migrations, showing which have been applied with timestamps and which are pending.
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

// snapshotBaseline is the filename used to persist the schema snapshot baseline.
const snapshotBaseline = ".schema-snapshot.json"

// runMigrateDiff compares the live database schema against a baseline snapshot and displays the differences in table, JSON, or SQL format.
func runMigrateDiff(cmd *cobra.Command, args []string) error {
	pool, cleanup, dir, err := connectAndDirForDiff(cmd)
	if err != nil {
		return err
	}
	defer cleanup()

	ctx := context.Background()
	live, err := schemadiff.TakeSnapshot(ctx, pool.DB())
	if err != nil {
		return fmt.Errorf("taking snapshot: %w", err)
	}

	baseline, err := loadBaselineSnapshot(dir)
	if err != nil {
		return err
	}

	cs := schemadiff.Diff(baseline, live)

	outputFmt, _ := cmd.Flags().GetString("output")
	printSQL, _ := cmd.Flags().GetBool("sql")

	if printSQL {
		up := schemadiff.GenerateUp(cs)
		if up == "" {
			fmt.Println("-- No changes detected.")
		} else {
			fmt.Println(up)
		}
		return nil
	}

	if outputFmt == "json" {
		return json.NewEncoder(os.Stdout).Encode(cs)
	}

	// Human-readable summary.
	if len(cs) == 0 {
		fmt.Println("No schema changes detected.")
		return nil
	}

	fmt.Printf("%-30s  %-15s  %s\n", "CHANGE TYPE", "SCHEMA.TABLE", "DETAIL")
	fmt.Printf("%-30s  %-15s  %s\n", "-----------", "------------", "------")
	for _, c := range cs {
		table := c.SchemaName + "." + c.TableName
		detail := changeDetail(c)
		fmt.Printf("%-30s  %-15s  %s\n", string(c.Type), table, detail)
	}
	return nil
}

// runMigrateGenerate generates up and down SQL migration files from schema differences between the live database and the last baseline snapshot, then updates the baseline.
func runMigrateGenerate(cmd *cobra.Command, args []string) error {
	name := args[0]
	if name == "" {
		return fmt.Errorf("migration name is required")
	}

	pool, cleanup, dir, err := connectAndDirForDiff(cmd)
	if err != nil {
		return err
	}
	defer cleanup()

	ctx := context.Background()
	live, err := schemadiff.TakeSnapshot(ctx, pool.DB())
	if err != nil {
		return fmt.Errorf("taking snapshot: %w", err)
	}

	baseline, err := loadBaselineSnapshot(dir)
	if err != nil {
		return err
	}

	cs := schemadiff.Diff(baseline, live)
	if len(cs) == 0 {
		fmt.Println("No schema changes detected. Nothing to generate.")
		return nil
	}

	upSQL := schemadiff.GenerateUp(cs)
	downSQL := schemadiff.GenerateDown(cs)

	upPath, downPath, err := schemadiff.WriteMigration(dir, name, upSQL, downSQL)
	if err != nil {
		return fmt.Errorf("writing migration files: %w", err)
	}

	// Save updated snapshot as new baseline.
	baselinePath := filepath.Join(dir, snapshotBaseline)
	if err := schemadiff.SaveSnapshot(baselinePath, live); err != nil {
		return fmt.Errorf("saving snapshot baseline: %w", err)
	}

	fmt.Printf("Created: %s\n", upPath)
	fmt.Printf("Created: %s\n", downPath)
	fmt.Printf("Baseline updated: %s\n", baselinePath)
	return nil
}

// connectAndDirForDiff connects to the database and returns pool, cleanup fn, migrations dir.
func connectAndDirForDiff(cmd *cobra.Command) (*postgres.Pool, func(), string, error) {
	cfg, err := loadMigrateConfig(cmd)
	if err != nil {
		return nil, nil, "", err
	}
	dir := migrationsDir(cmd, cfg)
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	pool, cleanup, err := connectForMigrate(cmd, cfg, logger)
	if err != nil {
		return nil, nil, "", err
	}
	return pool, cleanup, dir, nil
}

// loadBaselineSnapshot loads the snapshot from dir/.schema-snapshot.json.
// Returns an empty snapshot (not an error) if the file does not exist.
func loadBaselineSnapshot(dir string) (*schemadiff.Snapshot, error) {
	path := filepath.Join(dir, snapshotBaseline)
	snap, err := schemadiff.LoadSnapshot(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &schemadiff.Snapshot{}, nil
		}
		return nil, fmt.Errorf("loading baseline snapshot: %w", err)
	}
	return snap, nil
}

// changeDetail returns a brief human-readable detail string for a change.
func changeDetail(c schemadiff.Change) string {
	switch c.Type {
	case schemadiff.ChangeAddColumn, schemadiff.ChangeDropColumn:
		return c.ColumnName + " " + c.NewTypeName + c.OldTypeName
	case schemadiff.ChangeAlterColumnType:
		return c.ColumnName + ": " + c.OldTypeName + " → " + c.NewTypeName
	case schemadiff.ChangeAlterColumnDefault:
		return c.ColumnName + " default: " + c.OldDefault + " → " + c.NewDefault
	case schemadiff.ChangeAlterColumnNullable:
		if c.NewNullable {
			return c.ColumnName + ": NOT NULL → NULL"
		}
		return c.ColumnName + ": NULL → NOT NULL"
	case schemadiff.ChangeCreateIndex, schemadiff.ChangeDropIndex:
		return c.Index.Name
	case schemadiff.ChangeAddForeignKey, schemadiff.ChangeDropForeignKey:
		return c.ForeignKey.ConstraintName
	case schemadiff.ChangeAddCheckConstraint, schemadiff.ChangeDropCheckConstraint:
		return c.CheckConstraint.Name
	case schemadiff.ChangeCreateEnum, schemadiff.ChangeAlterEnumAddValue:
		return c.EnumSchema + "." + c.EnumName
	case schemadiff.ChangeAddRLSPolicy, schemadiff.ChangeDropRLSPolicy:
		return c.RLSPolicy.Name
	case schemadiff.ChangeEnableExtension, schemadiff.ChangeDisableExtension:
		return c.ExtensionName
	}
	return ""
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

// connectForMigrate establishes a PostgreSQL connection pool using the configured or flag-provided database URL and returns a cleanup function.
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
