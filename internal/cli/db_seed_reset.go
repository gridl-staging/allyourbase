// Package cli Provides CLI commands for seeding a database and performing complete database resets for development environments.
package cli

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/allyourbase/ayb/internal/migrations"
	"github.com/allyourbase/ayb/internal/sqlutil"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/cobra"
)

// runDBSeed executes SQL statements from a seed file to populate the database. The seed file path is specified as a command argument or read from the configuration file. SQL execution occurs within a transaction.
func runDBSeed(cmd *cobra.Command, args []string) error {
	dbURL, err := resolveDBURL(cmd)
	if err != nil {
		return err
	}

	seedPath := ""
	if len(args) > 0 {
		seedPath = args[0]
	} else {
		cfg, cfgErr := loadDBConfig(cmd)
		if cfgErr != nil {
			return cfgErr
		}
		seedPath = cfg.Database.SeedFile
	}
	if seedPath == "" {
		return fmt.Errorf("no seed file specified: provide a path argument or set database.seed_file in ayb.toml")
	}

	sqlBytes, err := os.ReadFile(seedPath)
	if err != nil {
		return fmt.Errorf("reading seed file %s: %w", seedPath, err)
	}

	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	pool, cleanup, err := connectForDB(dbURL)
	if err != nil {
		return err
	}
	defer cleanup()
	_ = logger

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	tag, err := tx.Exec(ctx, string(sqlBytes))
	if err != nil {
		return fmt.Errorf("executing seed file: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing seed: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Seed applied: %s (%d rows affected)\n", seedPath, tag.RowsAffected())
	return nil
}

// runDBReset performs a destructive reset of the user database by dropping all user-created tables and enums, re-running migrations from the configured directory, and optionally re-seeding from a configured seed file. The operation requires the --yes flag as a safety confirmation.
func runDBReset(cmd *cobra.Command, args []string) error {
	yes, _ := cmd.Flags().GetBool("yes")
	if !yes {
		return fmt.Errorf("ayb db reset is destructive — pass --yes to confirm")
	}

	dbURL, err := resolveDBURL(cmd)
	if err != nil {
		return err
	}

	cfg, err := loadDBConfig(cmd)
	if err != nil {
		return err
	}

	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	pool, cleanup, err := connectForDB(dbURL)
	if err != nil {
		return err
	}
	defer cleanup()

	fmt.Fprintln(cmd.OutOrStdout(), "Dropping user tables...")
	if err := dropUserObjects(ctx, pool); err != nil {
		return fmt.Errorf("dropping user objects: %w", err)
	}

	fmt.Fprintln(cmd.OutOrStdout(), "Re-running migrations...")
	migrationRunner := migrations.NewUserRunner(pool, cfg.Database.MigrationsDir, logger)
	if err := migrationRunner.Bootstrap(ctx); err != nil {
		return fmt.Errorf("bootstrapping migrations: %w", err)
	}
	appliedMigrations, err := migrationRunner.Up(ctx)
	if err != nil {
		return fmt.Errorf("applying migrations: %w", err)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Applied %d migration(s).\n", appliedMigrations)

	if cfg.Database.SeedFile != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "Re-seeding from %s...\n", cfg.Database.SeedFile)
		if err := runDBSeed(cmd, []string{cfg.Database.SeedFile}); err != nil {
			return fmt.Errorf("re-seeding: %w", err)
		}
	}

	fmt.Fprintln(cmd.OutOrStdout(), "Reset complete.")
	return nil
}

// dropUserObjects removes all user-created tables and enums from the database, excluding system schemas and framework-internal objects. Drops are performed with CASCADE to handle table dependencies.
func dropUserObjects(ctx context.Context, pool *pgxpool.Pool) error {
	tableRows, err := pool.Query(ctx, `
		SELECT schemaname, tablename
		FROM pg_tables
		WHERE schemaname NOT IN ('pg_catalog','information_schema','pg_toast')
		  AND schemaname NOT LIKE 'pg_%'
		  AND tablename NOT LIKE '_ayb_%'
		  AND tablename != 'schema_migrations'
		ORDER BY schemaname, tablename`)
	if err != nil {
		return fmt.Errorf("querying user tables: %w", err)
	}
	defer tableRows.Close()

	quotedTableNames := make([]string, 0)
	for tableRows.Next() {
		var schemaName string
		var tableName string
		if err := tableRows.Scan(&schemaName, &tableName); err != nil {
			return err
		}
		quotedTableNames = append(quotedTableNames, sqlutil.QuoteQualifiedName(schemaName, tableName))
	}
	if err := tableRows.Err(); err != nil {
		return err
	}

	for _, quotedTableName := range quotedTableNames {
		if _, err := pool.Exec(ctx, "DROP TABLE IF EXISTS "+quotedTableName+" CASCADE"); err != nil {
			return fmt.Errorf("dropping table %s: %w", quotedTableName, err)
		}
	}

	enumRows, err := pool.Query(ctx, `
		SELECT n.nspname, t.typname
		FROM pg_type t
		  JOIN pg_namespace n ON n.oid = t.typnamespace
		WHERE t.typtype = 'e'
		  AND n.nspname NOT IN ('pg_catalog','information_schema')
		  AND n.nspname NOT LIKE 'pg_%'
		ORDER BY n.nspname, t.typname`)
	if err != nil {
		return fmt.Errorf("querying user enums: %w", err)
	}
	defer enumRows.Close()

	quotedTypeNames := make([]string, 0)
	for enumRows.Next() {
		var schemaName string
		var typeName string
		if err := enumRows.Scan(&schemaName, &typeName); err != nil {
			return err
		}
		quotedTypeNames = append(quotedTypeNames, sqlutil.QuoteQualifiedName(schemaName, typeName))
	}
	if err := enumRows.Err(); err != nil {
		return err
	}

	for _, quotedTypeName := range quotedTypeNames {
		if _, err := pool.Exec(ctx, "DROP TYPE IF EXISTS "+quotedTypeName+" CASCADE"); err != nil {
			return fmt.Errorf("dropping type %s: %w", quotedTypeName, err)
		}
	}

	return nil
}
