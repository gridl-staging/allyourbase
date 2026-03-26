// Package migrations provides a UserRunner for executing and tracking user-managed SQL migration files loaded from disk.
package migrations

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// UserRunner handles user-managed SQL migrations from a directory on disk.
// Separate from the system Runner which uses embedded migrations.
type UserRunner struct {
	pool       *pgxpool.Pool
	dir        string
	logger     *slog.Logger
	schemaName string
}

var errUserRunnerPoolRequired = errors.New("user runner pool is required for database operations")

// NewUserRunner creates a runner for user migrations in the given directory.
func NewUserRunner(pool *pgxpool.Pool, dir string, logger *slog.Logger) *UserRunner {
	return &UserRunner{pool: pool, dir: dir, logger: normalizeRunnerLogger(logger)}
}

// NewUserRunnerWithSchema creates a runner scoped to a specific tenant schema.
func NewUserRunnerWithSchema(pool *pgxpool.Pool, dir, schemaName string, logger *slog.Logger) *UserRunner {
	return &UserRunner{pool: pool, dir: dir, logger: normalizeRunnerLogger(logger), schemaName: schemaName}
}

// Bootstrap creates the _ayb_user_migrations tracking table if it doesn't exist.
func (r *UserRunner) Bootstrap(ctx context.Context) error {
	if err := r.requirePool(); err != nil {
		return err
	}

	_, err := r.pool.Exec(ctx, fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			id          SERIAL PRIMARY KEY,
			name        TEXT NOT NULL UNIQUE,
			applied_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`, r.trackingTableExpr()))
	if err != nil {
		return fmt.Errorf("creating _ayb_user_migrations table: %w", err)
	}
	return nil
}

// Up applies all pending user migrations in filename order.
// Returns the number of migrations applied.
func (r *UserRunner) Up(ctx context.Context) (int, error) {
	if err := r.requirePool(); err != nil {
		return 0, err
	}

	files, err := r.listFiles()
	if err != nil {
		return 0, err
	}
	if len(files) == 0 {
		return 0, nil
	}

	applied := 0
	for _, name := range files {
		var exists bool
		err := r.pool.QueryRow(ctx, fmt.Sprintf(
			"SELECT EXISTS(SELECT 1 FROM %s WHERE name = $1)",
			r.trackingTableExpr(),
		), name,
		).Scan(&exists)
		if err != nil {
			return applied, fmt.Errorf("checking migration %s: %w", name, err)
		}
		if exists {
			continue
		}

		sql, err := os.ReadFile(filepath.Join(r.dir, name))
		if err != nil {
			return applied, fmt.Errorf("reading migration %s: %w", name, err)
		}

		if err := r.applyMigration(ctx, name, sql); err != nil {
			return applied, err
		}
		applied++
	}

	return applied, nil
}

// MigrationStatus represents a migration file and whether it has been applied.
type MigrationStatus struct {
	Name      string
	AppliedAt *time.Time // nil if pending
}

// Status returns all migration files with their applied/pending state.
func (r *UserRunner) Status(ctx context.Context) ([]MigrationStatus, error) {
	if err := r.requirePool(); err != nil {
		return nil, err
	}

	files, err := r.listFiles()
	if err != nil {
		return nil, err
	}

	// Load applied set.
	applied, err := r.getApplied(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]MigrationStatus, len(files))
	for i, name := range files {
		result[i] = MigrationStatus{Name: name}
		if t, ok := applied[name]; ok {
			result[i].AppliedAt = &t
		}
	}
	return result, nil
}

// CreateFile generates a new timestamped migration SQL file in the migrations directory.
// Returns the path to the created file.
func (r *UserRunner) CreateFile(name string) (string, error) {
	if err := os.MkdirAll(r.dir, 0o755); err != nil {
		return "", fmt.Errorf("creating migrations directory: %w", err)
	}

	ts := time.Now().UTC().Format("20060102150405")
	filename := fmt.Sprintf("%s_%s.sql", ts, sanitizeName(name))
	path := filepath.Join(r.dir, filename)

	content := fmt.Sprintf("-- Migration: %s\n-- Created: %s\n\n",
		name, time.Now().UTC().Format(time.RFC3339))

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("writing migration file: %w", err)
	}
	return path, nil
}

// listFiles returns sorted .sql filenames from the migrations directory.
func (r *UserRunner) listFiles() ([]string, error) {
	entries, err := os.ReadDir(r.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading migrations directory %s: %w", r.dir, err)
	}

	var files []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		files = append(files, e.Name())
	}
	sort.Strings(files)
	return files, nil
}

// getApplied returns a map of migration name → applied_at for all applied migrations.
func (r *UserRunner) getApplied(ctx context.Context) (map[string]time.Time, error) {
	rows, err := r.pool.Query(ctx, fmt.Sprintf(
		"SELECT name, applied_at FROM %s ORDER BY id",
		r.trackingTableExpr(),
	))
	if err != nil {
		return nil, fmt.Errorf("querying applied user migrations: %w", err)
	}
	defer rows.Close()

	applied := make(map[string]time.Time)
	for rows.Next() {
		var name string
		var t time.Time
		if err := rows.Scan(&name, &t); err != nil {
			return nil, fmt.Errorf("scanning user migration row: %w", err)
		}
		applied[name] = t
	}
	return applied, rows.Err()
}

func (r *UserRunner) trackingTableExpr() string {
	if r.schemaName == "" {
		return "_ayb_user_migrations"
	}
	return fmt.Sprintf(`%s._ayb_user_migrations`, pgx.Identifier{r.schemaName}.Sanitize())
}

func (r *UserRunner) searchPathSQL() string {
	return fmt.Sprintf(`SET search_path TO %s, public`, pgx.Identifier{r.schemaName}.Sanitize())
}

// applyMigration executes a migration SQL script in a transaction, optionally setting search_path for schema-scoped execution, and records the migration in the tracking table.
func (r *UserRunner) applyMigration(ctx context.Context, name string, sql []byte) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("starting transaction for %s: %w", name, err)
	}
	defer tx.Rollback(ctx) // no-op after commit; safety net for panics

	if r.schemaName != "" {
		if _, err := tx.Exec(ctx, r.searchPathSQL()); err != nil {
			return fmt.Errorf("setting search_path for %s: %w", name, err)
		}
	}

	if _, err := tx.Exec(ctx, string(sql)); err != nil {
		return fmt.Errorf("executing migration %s: %w", name, err)
	}

	if _, err := tx.Exec(ctx, fmt.Sprintf(
		"INSERT INTO %s (name) VALUES ($1)",
		r.trackingTableExpr(),
	), name,
	); err != nil {
		return fmt.Errorf("recording migration %s: %w", name, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing migration %s: %w", name, err)
	}

	r.logger.Info("applied user migration", "name", name)
	return nil
}

func (r *UserRunner) requirePool() error {
	if r == nil || r.pool == nil {
		return errUserRunnerPoolRequired
	}
	return nil
}

func normalizeRunnerLogger(logger *slog.Logger) *slog.Logger {
	if logger != nil {
		return logger
	}
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// sanitizeName replaces non-alphanumeric characters with underscores for filenames.
func sanitizeName(name string) string {
	var b strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	return b.String()
}
