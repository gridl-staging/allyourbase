package sbmigrate

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/allyourbase/ayb/internal/migrate"
)

// migrateSchema creates tables and views in the target database from the source schema, deferring tables with unmet dependencies and retrying them after dependent tables are created.
func (m *Migrator) migrateSchema(ctx context.Context, tx *sql.Tx, phaseIdx, totalPhases int) error {
	phase := migrate.Phase{Name: "Schema", Index: phaseIdx, Total: totalPhases}

	tables, err := introspectTables(ctx, m.source)
	if err != nil {
		return fmt.Errorf("introspecting tables: %w", err)
	}

	views, err := introspectViews(ctx, m.source)
	if err != nil {
		return fmt.Errorf("introspecting views: %w", err)
	}

	totalItems := len(tables) + len(views)
	m.progress.StartPhase(phase, totalItems)
	start := time.Now()

	fmt.Fprintln(m.output, "Creating schema...")

	type deferredTable struct {
		table   TableInfo
		lastErr error
	}
	deferred := make([]deferredTable, 0)

	for i, t := range tables {
		savepoint := fmt.Sprintf("ayb_schema_table_%d", i)
		if err := createTableWithSavepoint(ctx, tx, t, savepoint); err != nil {
			if isSkippableSchemaTableError(err) {
				deferred = append(deferred, deferredTable{table: t, lastErr: err})
				continue
			}
			return fmt.Errorf("creating table %s: %w", t.Name, err)
		}
		m.stats.Tables++
		m.progress.Progress(phase, i+1, totalItems)
		if m.verbose {
			fmt.Fprintf(m.output, "  CREATE TABLE %s (%d columns)\n", t.Name, len(t.Columns))
		}
	}

	// Retry deferred tables to handle valid FK dependencies created later in this phase.
	if len(deferred) > 0 {
		for pass := 1; pass <= len(deferred); pass++ {
			if len(deferred) == 0 {
				break
			}

			next := make([]deferredTable, 0, len(deferred))
			progressed := false

			for i, item := range deferred {
				savepoint := fmt.Sprintf("ayb_schema_table_retry_%d_%d", pass, i)
				if err := createTableWithSavepoint(ctx, tx, item.table, savepoint); err != nil {
					if isSkippableSchemaTableError(err) {
						item.lastErr = err
						next = append(next, item)
						continue
					}
					return fmt.Errorf("creating table %s: %w", item.table.Name, err)
				}

				progressed = true
				m.stats.Tables++
				if m.verbose {
					fmt.Fprintf(m.output, "  CREATE TABLE %s (%d columns)\n", item.table.Name, len(item.table.Columns))
				}
			}

			if !progressed {
				for _, item := range next {
					m.markSkippedTable(item.table.Name, item.lastErr)
					m.stats.Skipped++
					m.progress.Warn(fmt.Sprintf("skipping table %s due source/target schema incompatibility: %v", item.table.Name, item.lastErr))
				}
				break
			}

			deferred = next
		}
	}

	for i, v := range views {
		ddl := createViewSQL(v)
		savepoint := fmt.Sprintf("ayb_schema_view_%d", i)
		if err := execSavepointCommand(ctx, tx, "SAVEPOINT "+savepoint); err != nil {
			return fmt.Errorf("creating savepoint for view %s: %w", v.Name, err)
		}
		if _, err := tx.ExecContext(ctx, ddl); err != nil {
			if rbErr := execSavepointCommand(ctx, tx, "ROLLBACK TO SAVEPOINT "+savepoint); rbErr != nil {
				return fmt.Errorf("rolling back savepoint for view %s after error %v: %w", v.Name, err, rbErr)
			}
			if relErr := execSavepointCommand(ctx, tx, "RELEASE SAVEPOINT "+savepoint); relErr != nil {
				return fmt.Errorf("releasing savepoint for view %s after rollback: %w", v.Name, relErr)
			}
			// Views may depend on tables that don't exist in the target yet.
			// Log a warning instead of failing.
			m.progress.Warn(fmt.Sprintf("skipping view %s: %v", v.Name, err))
			continue
		}
		if err := execSavepointCommand(ctx, tx, "RELEASE SAVEPOINT "+savepoint); err != nil {
			return fmt.Errorf("releasing savepoint for view %s: %w", v.Name, err)
		}
		m.stats.Views++
		if m.verbose {
			fmt.Fprintf(m.output, "  CREATE VIEW %s\n", v.Name)
		}
	}

	m.progress.CompletePhase(phase, totalItems, time.Since(start))
	fmt.Fprintf(m.output, "  ✓ %d tables, %d views created\n", m.stats.Tables, m.stats.Views)
	return nil
}

// createTableWithSavepoint creates a table within a database savepoint, rolling back and releasing the savepoint if the creation fails.
func createTableWithSavepoint(ctx context.Context, tx *sql.Tx, table TableInfo, savepoint string) error {
	ddl := createTableSQL(table)
	if err := execSavepointCommand(ctx, tx, "SAVEPOINT "+savepoint); err != nil {
		return fmt.Errorf("creating savepoint for table %s: %w", table.Name, err)
	}
	if _, err := tx.ExecContext(ctx, ddl); err != nil {
		if rbErr := execSavepointCommand(ctx, tx, "ROLLBACK TO SAVEPOINT "+savepoint); rbErr != nil {
			return fmt.Errorf("rolling back savepoint for table %s after error %v: %w", table.Name, err, rbErr)
		}
		if relErr := execSavepointCommand(ctx, tx, "RELEASE SAVEPOINT "+savepoint); relErr != nil {
			return fmt.Errorf("releasing savepoint for table %s after rollback: %w", table.Name, relErr)
		}
		return err
	}
	if err := execSavepointCommand(ctx, tx, "RELEASE SAVEPOINT "+savepoint); err != nil {
		return fmt.Errorf("releasing savepoint for table %s: %w", table.Name, err)
	}
	return nil
}
