package fbmigrate

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/allyourbase/ayb/internal/migrate"
)

// RTDBNode represents a top-level node from a Firebase Realtime Database export.
// Each top-level key becomes a table; child keys become rows stored as JSONB.
type RTDBNode struct {
	Name     string
	Children map[string]json.RawMessage
}

// ParseRTDBExport reads a Firebase RTDB JSON export and returns top-level nodes.
// The export format is a single JSON object where each top-level key is a
// "collection" (e.g., {"users": {...}, "posts": {...}}).
func ParseRTDBExport(path string) ([]RTDBNode, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading RTDB export: %w", err)
	}

	var root map[string]json.RawMessage
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("parsing RTDB export: %w", err)
	}

	// Sort top-level keys for deterministic table creation order.
	names := make([]string, 0, len(root))
	for name := range root {
		names = append(names, name)
	}
	sort.Strings(names)

	var nodes []RTDBNode
	for _, name := range names {
		raw := root[name]
		// Each top-level value should be an object whose keys are record IDs.
		var children map[string]json.RawMessage
		if err := json.Unmarshal(raw, &children); err != nil {
			// If it's not an object (e.g., a scalar or array), store as single row.
			children = map[string]json.RawMessage{"_root": raw}
		}
		nodes = append(nodes, RTDBNode{
			Name:     name,
			Children: children,
		})
	}

	return nodes, nil
}

// NormalizeRTDBTableName converts an RTDB path to a PostgreSQL table name.
// RTDB keys can contain Unicode, slashes, and special characters.
func NormalizeRTDBTableName(name string) string {
	name = strings.ToLower(name)
	var sb strings.Builder
	for _, c := range name {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '_' {
			sb.WriteRune(c)
		} else if c == '-' || c == '/' || c == ' ' || c == '.' {
			sb.WriteRune('_')
		}
	}
	result := sb.String()
	// Ensure it starts with a letter.
	if len(result) > 0 && (result[0] >= '0' && result[0] <= '9') {
		result = "t_" + result
	}
	if result == "" {
		result = "rtdb_data"
	}
	// PostgreSQL identifier limit.
	if len(result) > 63 {
		result = result[:63]
	}
	return result
}

// createRTDBTableSQL generates a CREATE TABLE statement for an RTDB node.
// Each table has: id TEXT PK, data JSONB.
func createRTDBTableSQL(tableName string) string {
	return fmt.Sprintf(
		`CREATE TABLE IF NOT EXISTS %q ("id" text PRIMARY KEY, "data" jsonb NOT NULL)`,
		tableName,
	)
}

// createRTDBIndexSQL generates a GIN index on the data column.
func createRTDBIndexSQL(tableName string) string {
	indexName := "idx_" + tableName + "_data"
	// PostgreSQL identifier limit is 63 chars; truncate if needed.
	if len(indexName) > 63 {
		indexName = indexName[:63]
	}
	return fmt.Sprintf(
		`CREATE INDEX IF NOT EXISTS %q ON %q USING GIN ("data")`,
		indexName, tableName,
	)
}

// migrateRTDB reads an RTDB export and creates tables with JSONB data.
func (m *Migrator) migrateRTDB(ctx context.Context, tx *sql.Tx, phaseIdx, totalPhases int) error {
	phase := migrate.Phase{Name: "RTDB", Index: phaseIdx, Total: totalPhases}

	nodes, err := ParseRTDBExport(m.opts.RTDBExportPath)
	if err != nil {
		return err
	}

	var totalRecords int
	for _, n := range nodes {
		totalRecords += len(n.Children)
	}

	m.progress.StartPhase(phase, totalRecords)
	start := time.Now()

	fmt.Fprintln(m.output, "Migrating Realtime Database...")

	processed := 0
	for _, node := range nodes {
		tableName := NormalizeRTDBTableName(node.Name)

		// Create table.
		if _, err := tx.ExecContext(ctx, createRTDBTableSQL(tableName)); err != nil {
			return fmt.Errorf("creating table %s: %w", tableName, err)
		}

		// Create GIN index.
		if _, err := tx.ExecContext(ctx, createRTDBIndexSQL(tableName)); err != nil {
			m.progress.Warn(fmt.Sprintf("creating index on %s: %v", tableName, err))
		}

		m.stats.RTDBNodes++

		// Insert records.
		for childKey, childData := range node.Children {
			result, err := tx.ExecContext(ctx,
				fmt.Sprintf(`INSERT INTO %q ("id", "data") VALUES ($1, $2) ON CONFLICT ("id") DO NOTHING`, tableName),
				childKey, string(childData),
			)
			if err != nil {
				m.stats.Errors = append(m.stats.Errors,
					fmt.Sprintf("inserting %s/%s: %v", tableName, childKey, err))
				processed++
				m.progress.Progress(phase, processed, totalRecords)
				continue
			}
			if n, _ := result.RowsAffected(); n > 0 {
				m.stats.RTDBRecords++
			}
			processed++
			m.progress.Progress(phase, processed, totalRecords)
		}

		if m.verbose {
			fmt.Fprintf(m.output, "  %s: %d records\n", tableName, len(node.Children))
		}
	}

	m.progress.CompletePhase(phase, totalRecords, time.Since(start))
	fmt.Fprintf(m.output, "  %d records across %d nodes\n", m.stats.RTDBRecords, m.stats.RTDBNodes)
	return nil
}
