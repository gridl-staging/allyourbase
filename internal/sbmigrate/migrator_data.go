package sbmigrate

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/allyourbase/ayb/internal/migrate"
	"github.com/jackc/pgx/v5/pgconn"
)

// migrateData streams row data from source tables to the target database, deferring tables with unmet foreign key constraints and retrying them after dependencies are resolved.
func (m *Migrator) migrateData(ctx context.Context, tx *sql.Tx, phaseIdx, totalPhases int) error {
	phase := migrate.Phase{Name: "Data", Index: phaseIdx, Total: totalPhases}

	// Re-introspect to get tables that now exist in the target.
	tables, err := introspectTables(ctx, m.source)
	if err != nil {
		return fmt.Errorf("introspecting tables for data copy: %w", err)
	}
	tables = m.filterSkippedTables(tables)

	var totalRows int64
	for _, t := range tables {
		totalRows += t.RowCount
	}

	m.progress.StartPhase(phase, int(totalRows))
	start := time.Now()

	fmt.Fprintln(m.output, "Copying data...")

	type deferredDataTable struct {
		table   TableInfo
		lastErr error
	}
	deferred := make([]deferredDataTable, 0)

	copied := 0
	for i, t := range tables {
		savepoint := fmt.Sprintf("ayb_data_table_%d", i)
		count, err := copyTableDataWithSavepoint(ctx, m.source, tx, t, savepoint, func(n int) {
			m.progress.Progress(phase, copied+n, int(totalRows))
		})
		if err != nil {
			if isRetriableDataTableError(err) {
				deferred = append(deferred, deferredDataTable{table: t, lastErr: err})
				continue
			}
			return fmt.Errorf("copying data for %s: %w", t.Name, err)
		}
		copied += count
		m.stats.Records += count
		if m.verbose {
			fmt.Fprintf(m.output, "  %s: %d rows\n", t.Name, count)
		}
	}

	// Retry deferred tables to resolve FK dependencies where parent rows are copied later.
	if len(deferred) > 0 {
		for pass := 1; pass <= len(deferred); pass++ {
			if len(deferred) == 0 {
				break
			}

			next := make([]deferredDataTable, 0, len(deferred))
			progressed := false

			for i, item := range deferred {
				savepoint := fmt.Sprintf("ayb_data_table_retry_%d_%d", pass, i)
				count, err := copyTableDataWithSavepoint(ctx, m.source, tx, item.table, savepoint, func(n int) {
					m.progress.Progress(phase, copied+n, int(totalRows))
				})
				if err != nil {
					if isRetriableDataTableError(err) {
						item.lastErr = err
						next = append(next, item)
						continue
					}
					return fmt.Errorf("copying data for %s: %w", item.table.Name, err)
				}

				progressed = true
				copied += count
				m.stats.Records += count
				if m.verbose {
					fmt.Fprintf(m.output, "  %s: %d rows\n", item.table.Name, count)
				}
			}

			if !progressed {
				for _, item := range next {
					m.markSkippedTable(item.table.Name, item.lastErr)
					m.stats.Skipped++
					m.progress.Warn(fmt.Sprintf("skipping data copy for %s due unresolved dependency: %v", item.table.Name, item.lastErr))
				}
				break
			}

			deferred = next
		}
	}

	// Reset sequences.
	seqCount, err := resetSequences(ctx, tx, tables)
	if err != nil {
		m.progress.Warn(fmt.Sprintf("sequence reset: %v", err))
	}
	m.stats.Sequences = seqCount

	m.progress.CompletePhase(phase, int(totalRows), time.Since(start))
	fmt.Fprintf(m.output, "  ✓ %d records copied across %d tables\n", m.stats.Records, len(tables))
	return nil
}

// copyTableDataWithSavepoint copies data from a source table to the target within a savepoint, rolling back and releasing the savepoint if an error occurs.
func copyTableDataWithSavepoint(
	ctx context.Context,
	source *sql.DB,
	tx *sql.Tx,
	table TableInfo,
	savepoint string,
	progressFn func(int),
) (int, error) {
	if err := execSavepointCommand(ctx, tx, "SAVEPOINT "+savepoint); err != nil {
		return 0, fmt.Errorf("creating savepoint for data copy %s: %w", table.Name, err)
	}

	count, err := copyTableData(ctx, source, tx, table, progressFn)
	if err != nil {
		if rbErr := execSavepointCommand(ctx, tx, "ROLLBACK TO SAVEPOINT "+savepoint); rbErr != nil {
			return 0, fmt.Errorf("rolling back savepoint for data copy %s after error %v: %w", table.Name, err, rbErr)
		}
		if relErr := execSavepointCommand(ctx, tx, "RELEASE SAVEPOINT "+savepoint); relErr != nil {
			return 0, fmt.Errorf("releasing savepoint for data copy %s after rollback: %w", table.Name, relErr)
		}
		return 0, err
	}

	if err := execSavepointCommand(ctx, tx, "RELEASE SAVEPOINT "+savepoint); err != nil {
		return 0, fmt.Errorf("releasing savepoint for data copy %s: %w", table.Name, err)
	}

	return count, nil
}

func isRetriableDataTableError(err error) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}

	switch pgErr.Code {
	case "23503": // foreign_key_violation
		return true
	case "42P01": // undefined_table
		return true
	default:
		return false
	}
}

func execSavepointCommand(ctx context.Context, tx *sql.Tx, stmt string) error {
	_, err := tx.ExecContext(ctx, stmt)
	return err
}
