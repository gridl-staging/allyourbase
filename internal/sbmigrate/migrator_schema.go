package sbmigrate

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/allyourbase/ayb/internal/migrate"
)

type deferredSchemaTable struct {
	table   TableInfo
	lastErr error
}

func (m *Migrator) migrateSchema(ctx context.Context, tx *sql.Tx, phaseIdx, totalPhases int) error {
	phase := migrate.Phase{Name: "Schema", Index: phaseIdx, Total: totalPhases}
	tables, views, err := m.loadSchemaPhaseItems(ctx)
	if err != nil {
		return err
	}

	totalItems := len(tables) + len(views)
	start := m.startSchemaPhase(phase, totalItems)

	deferred, err := m.createInitialSchemaTables(ctx, tx, phase, totalItems, tables)
	if err != nil {
		return err
	}
	if err := m.retryDeferredSchemaTables(ctx, tx, deferred); err != nil {
		return err
	}
	if err := m.createSchemaViews(ctx, tx, views); err != nil {
		return err
	}

	m.progress.CompletePhase(phase, totalItems, time.Since(start))
	fmt.Fprintf(m.output, "  ✓ %d tables, %d views created\n", m.stats.Tables, m.stats.Views)
	return nil
}

func (m *Migrator) loadSchemaPhaseItems(ctx context.Context) ([]TableInfo, []ViewInfo, error) {
	tables, err := introspectTables(ctx, m.source)
	if err != nil {
		return nil, nil, fmt.Errorf("introspecting tables: %w", err)
	}
	views, err := introspectViews(ctx, m.source)
	if err != nil {
		return nil, nil, fmt.Errorf("introspecting views: %w", err)
	}
	return tables, views, nil
}

func (m *Migrator) startSchemaPhase(phase migrate.Phase, totalItems int) time.Time {
	m.progress.StartPhase(phase, totalItems)
	fmt.Fprintln(m.output, "Creating schema...")
	return time.Now()
}

func (m *Migrator) createInitialSchemaTables(
	ctx context.Context,
	tx *sql.Tx,
	phase migrate.Phase,
	totalItems int,
	tables []TableInfo,
) ([]deferredSchemaTable, error) {
	deferred := make([]deferredSchemaTable, 0)
	for idx, table := range tables {
		savepoint := fmt.Sprintf("ayb_schema_table_%d", idx)
		if err := createTableWithSavepoint(ctx, tx, table, savepoint); err != nil {
			if isSkippableSchemaTableError(err) {
				deferred = append(deferred, deferredSchemaTable{table: table, lastErr: err})
				continue
			}
			return nil, fmt.Errorf("creating table %s: %w", table.Name, err)
		}

		m.stats.Tables++
		m.progress.Progress(phase, idx+1, totalItems)
		m.logSchemaTableCreated(table)
	}

	return deferred, nil
}

func (m *Migrator) retryDeferredSchemaTables(
	ctx context.Context,
	tx *sql.Tx,
	deferred []deferredSchemaTable,
) error {
	if len(deferred) == 0 {
		return nil
	}

	for pass := 1; pass <= len(deferred); pass++ {
		if len(deferred) == 0 {
			break
		}

		next := make([]deferredSchemaTable, 0, len(deferred))
		progressed := false
		for idx, item := range deferred {
			savepoint := fmt.Sprintf("ayb_schema_table_retry_%d_%d", pass, idx)
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
			m.logSchemaTableCreated(item.table)
		}
		if !progressed {
			m.skipDeferredSchemaTables(next)
			break
		}
		deferred = next
	}
	return nil
}

func (m *Migrator) skipDeferredSchemaTables(deferred []deferredSchemaTable) {
	for _, item := range deferred {
		m.markSkippedTable(item.table.Name, item.lastErr)
		m.stats.Skipped++
		m.progress.Warn(fmt.Sprintf("skipping table %s due source/target schema incompatibility: %v", item.table.Name, item.lastErr))
	}
}

func (m *Migrator) createSchemaViews(ctx context.Context, tx *sql.Tx, views []ViewInfo) error {
	for idx, view := range views {
		savepoint := fmt.Sprintf("ayb_schema_view_%d", idx)
		if err := execSavepointCommand(ctx, tx, "SAVEPOINT "+savepoint); err != nil {
			return fmt.Errorf("creating savepoint for view %s: %w", view.Name, err)
		}
		if _, err := tx.ExecContext(ctx, createViewSQL(view)); err != nil {
			if rbErr := execSavepointCommand(ctx, tx, "ROLLBACK TO SAVEPOINT "+savepoint); rbErr != nil {
				return fmt.Errorf("rolling back savepoint for view %s after error %v: %w", view.Name, err, rbErr)
			}
			if relErr := execSavepointCommand(ctx, tx, "RELEASE SAVEPOINT "+savepoint); relErr != nil {
				return fmt.Errorf("releasing savepoint for view %s after rollback: %w", view.Name, relErr)
			}
			// Views may depend on tables that don't exist in the target yet.
			// Log a warning instead of failing.
			m.progress.Warn(fmt.Sprintf("skipping view %s: %v", view.Name, err))
			continue
		}
		if err := execSavepointCommand(ctx, tx, "RELEASE SAVEPOINT "+savepoint); err != nil {
			return fmt.Errorf("releasing savepoint for view %s: %w", view.Name, err)
		}
		m.stats.Views++
		m.logSchemaViewCreated(view)
	}
	return nil
}

func (m *Migrator) logSchemaTableCreated(table TableInfo) {
	if m.verbose {
		fmt.Fprintf(m.output, "  CREATE TABLE %s (%d columns)\n", table.Name, len(table.Columns))
	}
}

func (m *Migrator) logSchemaViewCreated(view ViewInfo) {
	if m.verbose {
		fmt.Fprintf(m.output, "  CREATE VIEW %s\n", view.Name)
	}
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
