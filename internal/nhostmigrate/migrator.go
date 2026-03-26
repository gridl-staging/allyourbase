// Package nhostmigrate implements schema and data migration from NHost (a Hasura-based platform) to AYB by parsing pg_dump files and overlaying Hasura v3 metadata to build and execute a migration plan.
package nhostmigrate

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/allyourbase/ayb/internal/migrate"
	_ "github.com/jackc/pgx/v5/stdlib"
	"strings"
)

const (
	nhostSourceLabel = "NHost (source)"
	aybTargetLabel   = "AYB (target)"
)

// Migrator imports schema/data from pg_dump and overlays Hasura metadata.
type Migrator struct {
	db       *sql.DB
	opts     MigrationOptions
	stats    MigrationStats
	output   io.Writer
	verbose  bool
	progress migrate.ProgressReporter
	beginTx  func(context.Context) (txLike, error)
}

type txLike interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	Commit() error
	Rollback() error
}

// NewMigrator creates a new NHost migrator.
func NewMigrator(opts MigrationOptions) (*Migrator, error) {
	if strings.TrimSpace(opts.HasuraMetadataPath) == "" {
		return nil, fmt.Errorf("hasura metadata path is required")
	}
	if strings.TrimSpace(opts.PgDumpPath) == "" {
		return nil, fmt.Errorf("pg_dump path is required")
	}
	if strings.TrimSpace(opts.DatabaseURL) == "" {
		return nil, fmt.Errorf("database URL is required")
	}

	metaInfo, err := os.Stat(opts.HasuraMetadataPath)
	if err != nil {
		return nil, fmt.Errorf("hasura metadata path: %w", err)
	}
	if !metaInfo.IsDir() {
		return nil, fmt.Errorf("hasura metadata path must be a directory")
	}

	dumpInfo, err := os.Stat(opts.PgDumpPath)
	if err != nil {
		return nil, fmt.Errorf("pg_dump path: %w", err)
	}
	if dumpInfo.IsDir() {
		return nil, fmt.Errorf("pg_dump path must be a file")
	}

	db, err := sql.Open("pgx", opts.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("connecting to database: %w", err)
	}
	if err := db.PingContext(context.Background()); err != nil {
		db.Close()
		return nil, fmt.Errorf("pinging database: %w", err)
	}

	output := io.Writer(os.Stdout)
	if opts.DryRun && !opts.Verbose {
		output = io.Discard
	}

	progress := opts.Progress
	if progress == nil {
		progress = migrate.NopReporter{}
	}

	m := &Migrator{
		db:       db,
		opts:     opts,
		output:   output,
		verbose:  opts.Verbose,
		progress: progress,
	}
	m.beginTx = func(ctx context.Context) (txLike, error) {
		return m.db.BeginTx(ctx, nil)
	}

	return m, nil
}

// Close releases the database connection.
func (m *Migrator) Close() error {
	if m.db != nil {
		return m.db.Close()
	}
	return nil
}

// Analyze reads pg_dump + metadata and returns an analysis report.
func (m *Migrator) Analyze(ctx context.Context) (*migrate.AnalysisReport, error) {
	_, report, err := m.buildPlan(ctx)
	if err != nil {
		return nil, err
	}
	return report, nil
}

// Migrate applies schema/data and metadata overlays inside a transaction.
func (m *Migrator) Migrate(ctx context.Context) (*MigrationStats, error) {
	plan, _, err := m.buildPlan(ctx)
	if err != nil {
		return nil, err
	}

	tx, err := m.beginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	phase := migrate.Phase{Name: "Replay SQL", Index: 1, Total: 1}
	m.progress.StartPhase(phase, len(plan.Statements))
	started := time.Now()

	for i, stmt := range plan.Statements {
		if _, err := tx.ExecContext(ctx, stmt.SQL); err != nil {
			plan.stats.Errors = append(plan.stats.Errors, err.Error())
			return nil, fmt.Errorf("executing %s statement %d: %w", stmt.Kind, i+1, err)
		}
		m.progress.Progress(phase, i+1, len(plan.Statements))
	}

	if m.opts.DryRun {
		if err := tx.Rollback(); err != nil {
			return nil, fmt.Errorf("rolling back dry-run: %w", err)
		}
	} else {
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("committing transaction: %w", err)
		}
	}

	m.progress.CompletePhase(phase, len(plan.Statements), time.Since(started))
	m.stats = plan.stats
	return &m.stats, nil
}
