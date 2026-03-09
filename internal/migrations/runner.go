package migrations

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed sql/*.sql
var embeddedMigrations embed.FS

// Runner handles system schema migrations.
type Runner struct {
	pool   *pgxpool.Pool
	logger *slog.Logger
	source fs.FS
}

// NewRunner creates a new migration runner.
func NewRunner(pool *pgxpool.Pool, logger *slog.Logger) *Runner {
	return &Runner{pool: pool, logger: logger, source: embeddedMigrations}
}

// NewRunnerWithFS creates a migration runner that loads migrations from the
// provided filesystem. If source is nil, embedded migrations are used.
func NewRunnerWithFS(pool *pgxpool.Pool, logger *slog.Logger, source fs.FS) *Runner {
	if source == nil {
		source = embeddedMigrations
	}
	return &Runner{pool: pool, logger: logger, source: source}
}

// Bootstrap creates the _ayb_migrations table if it doesn't exist.
func (r *Runner) Bootstrap(ctx context.Context) error {
	_, err := r.pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS _ayb_migrations (
			id          SERIAL PRIMARY KEY,
			name        TEXT NOT NULL UNIQUE,
			applied_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`)
	if err != nil {
		return fmt.Errorf("creating _ayb_migrations table: %w", err)
	}
	r.logger.Debug("migration table ready")
	return nil
}

// Run applies all pending embedded migrations in order.
func (r *Runner) Run(ctx context.Context) (int, error) {
	entries, err := fs.ReadDir(r.source, "sql")
	if err != nil {
		return 0, fmt.Errorf("reading embedded migrations: %w", err)
	}

	// Sort by filename to ensure order.
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	applied := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}

		name := entry.Name()

		// Check if already applied.
		var exists bool
		err := r.pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM _ayb_migrations WHERE name = $1)", name).Scan(&exists)
		if err != nil {
			return applied, fmt.Errorf("checking migration %s: %w", name, err)
		}
		if exists {
			continue
		}

		// Read and execute migration.
		sql, err := fs.ReadFile(r.source, "sql/"+name)
		if err != nil {
			return applied, fmt.Errorf("reading migration %s: %w", name, err)
		}

		tx, err := r.pool.Begin(ctx)
		if err != nil {
			return applied, fmt.Errorf("starting transaction for %s: %w", name, err)
		}
		defer tx.Rollback(ctx) // no-op after commit; safety net for panics

		if _, err := tx.Exec(ctx, string(sql)); err != nil {
			return applied, fmt.Errorf("executing migration %s: %w", name, err)
		}

		if _, err := tx.Exec(ctx, "INSERT INTO _ayb_migrations (name) VALUES ($1)", name); err != nil {
			return applied, fmt.Errorf("recording migration %s: %w", name, err)
		}

		if err := tx.Commit(ctx); err != nil {
			return applied, fmt.Errorf("committing migration %s: %w", name, err)
		}

		r.logger.Info("applied migration", "name", name)
		applied++
	}

	return applied, nil
}

// GetApplied returns the list of applied migrations.
func (r *Runner) GetApplied(ctx context.Context) ([]AppliedMigration, error) {
	rows, err := r.pool.Query(ctx, "SELECT name, applied_at FROM _ayb_migrations ORDER BY id")
	if err != nil {
		return nil, fmt.Errorf("querying applied migrations: %w", err)
	}
	defer rows.Close()

	var migrations []AppliedMigration
	for rows.Next() {
		var m AppliedMigration
		if err := rows.Scan(&m.Name, &m.AppliedAt); err != nil {
			return nil, fmt.Errorf("scanning migration row: %w", err)
		}
		migrations = append(migrations, m)
	}
	return migrations, rows.Err()
}

// AppliedMigration represents a migration that has been applied.
type AppliedMigration struct {
	Name      string
	AppliedAt time.Time
}
