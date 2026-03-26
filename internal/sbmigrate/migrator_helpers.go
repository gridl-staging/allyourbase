package sbmigrate

import (
	"context"
	"errors"
	"fmt"

	"github.com/allyourbase/ayb/internal/migrate"
	"github.com/jackc/pgx/v5/pgconn"
)

// printStats outputs a summary of migration statistics to the output writer, including counts of tables, views, records, users, OAuth links, and RLS policies.
func (m *Migrator) printStats() {
	fmt.Fprintf(m.output, "\nSummary:\n")
	if m.stats.Tables > 0 {
		fmt.Fprintf(m.output, "  Tables:     %d\n", m.stats.Tables)
	}
	if m.stats.Views > 0 {
		fmt.Fprintf(m.output, "  Views:      %d\n", m.stats.Views)
	}
	if m.stats.Records > 0 {
		fmt.Fprintf(m.output, "  Records:    %d\n", m.stats.Records)
	}
	if m.stats.Sequences > 0 {
		fmt.Fprintf(m.output, "  Sequences:  %d\n", m.stats.Sequences)
	}
	fmt.Fprintf(m.output, "  Users:      %d\n", m.stats.Users)
	fmt.Fprintf(m.output, "  OAuth:      %d\n", m.stats.OAuthLinks)
	fmt.Fprintf(m.output, "  RLS:        %d\n", m.stats.Policies)
	if m.stats.StorageFiles > 0 {
		fmt.Fprintf(m.output, "  Files:      %d (%s)\n", m.stats.StorageFiles, migrate.FormatBytes(m.stats.StorageBytes))
	}
	fmt.Fprintf(m.output, "  Skipped:    %d\n", m.stats.Skipped)
	if len(m.stats.Errors) > 0 {
		fmt.Fprintf(m.output, "  Errors:     %d\n", len(m.stats.Errors))
		for _, e := range m.stats.Errors {
			fmt.Fprintf(m.output, "    - %s\n", e)
		}
	}
}

// extractString tries multiple keys in a map and returns the first non-empty string value.
func extractString(data map[string]any, keys ...string) string {
	for _, key := range keys {
		if val, ok := data[key]; ok {
			if s, ok := val.(string); ok && s != "" {
				return s
			}
		}
	}
	return ""
}

// sourceColumnExists checks whether a column exists in the source database schema, caching results for subsequent calls.
func (m *Migrator) sourceColumnExists(ctx context.Context, schema, table, column string) (bool, error) {
	if m.sourceColumnCache == nil {
		m.sourceColumnCache = make(map[string]bool)
	}

	key := schema + "." + table + "." + column
	if exists, ok := m.sourceColumnCache[key]; ok {
		return exists, nil
	}

	var exists bool
	err := m.source.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM information_schema.columns
			WHERE table_schema = $1
			  AND table_name = $2
			  AND column_name = $3
		)
	`, schema, table, column).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("checking source schema for %s: %w", key, err)
	}

	m.sourceColumnCache[key] = exists
	return exists, nil
}

func (m *Migrator) markSkippedTable(name string, err error) {
	if m.skippedTables == nil {
		m.skippedTables = make(map[string]string)
	}
	m.skippedTables[name] = err.Error()
}

func (m *Migrator) isSkippedTable(name string) bool {
	if m.skippedTables == nil {
		return false
	}
	_, ok := m.skippedTables[name]
	return ok
}

// filterSkippedTables returns a filtered list of tables, excluding those marked as skipped during schema migration due to incompatibilities.
func (m *Migrator) filterSkippedTables(tables []TableInfo) []TableInfo {
	if len(tables) == 0 || len(m.skippedTables) == 0 {
		return tables
	}

	filtered := make([]TableInfo, 0, len(tables))
	for _, t := range tables {
		if m.isSkippedTable(t.Name) {
			if m.verbose {
				fmt.Fprintf(m.output, "  skipped data copy for %s (schema incompatibility)\n", t.Name)
			}
			continue
		}
		filtered = append(filtered, t)
	}
	return filtered
}

// isSkippableSchemaTableError determines whether a table creation error represents a recoverable schema incompatibility that can be deferred, such as undefined functions or missing foreign key references.
func isSkippableSchemaTableError(err error) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}

	switch pgErr.Code {
	case "42883": // undefined_function
		return true
	case "42704": // undefined_object (often undefined type)
		return true
	case "42P01": // undefined_table (e.g. FK references skipped table)
		return true
	case "0A000": // feature_not_supported
		return true
	default:
		return false
	}
}
