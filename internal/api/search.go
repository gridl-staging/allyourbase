package api

import (
	"fmt"
	"strings"

	"github.com/allyourbase/ayb/internal/schema"
)

// textColumnTypes are PostgreSQL type names that should be included in full-text search.
var textColumnTypes = map[string]bool{
	"text":              true,
	"varchar":           true,
	"character varying": true,
	"char":              true,
	"character":         true,
	"name":              true,
	"citext":            true,
}

// isTextColumn returns true if a column is a text type suitable for FTS.
func isTextColumn(col *schema.Column) bool {
	if col.IsJSON || col.IsArray || col.IsEnum {
		return false
	}
	// Normalize: strip modifiers like (255).
	base := strings.ToLower(col.TypeName)
	if idx := strings.Index(base, "("); idx > 0 {
		base = strings.TrimSpace(base[:idx])
	}
	return textColumnTypes[base]
}

// textColumns returns the names of all text columns in a table.
func textColumns(tbl *schema.Table) []string {
	var cols []string
	for _, c := range tbl.Columns {
		if isTextColumn(c) {
			cols = append(cols, c.Name)
		}
	}
	return cols
}

// buildSearchSQL generates a FTS WHERE clause and an ORDER BY expression for ranking.
// It uses websearch_to_tsquery (Postgres 11+) for user-friendly search syntax.
//
// argOffset is the starting parameter index (e.g., if filters already used $1-$3, pass 4).
//
// Returns:
//   - whereSQL: the WHERE condition, e.g. `to_tsvector('simple', ...) @@ websearch_to_tsquery('simple', $4)`
//   - rankSQL: the ORDER BY expression, e.g. `ts_rank(to_tsvector('simple', ...), websearch_to_tsquery('simple', $4))`
//   - args: the query parameter values (just the search term)
//   - error: if no searchable text columns exist
func buildSearchSQL(tbl *schema.Table, searchTerm string, argOffset int) (whereSQL, rankSQL string, args []any, err error) {
	cols := textColumns(tbl)
	if len(cols) == 0 {
		return "", "", nil, fmt.Errorf("table %q has no text columns to search", tbl.Name)
	}

	// Build: coalesce("col1", '') || ' ' || coalesce("col2", '') || ...
	parts := make([]string, len(cols))
	for i, col := range cols {
		parts[i] = fmt.Sprintf("coalesce(%s, '')", quoteIdent(col))
	}
	docExpr := strings.Join(parts, " || ' ' || ")

	paramRef := fmt.Sprintf("$%d", argOffset)
	tsvector := fmt.Sprintf("to_tsvector('simple', %s)", docExpr)
	tsquery := fmt.Sprintf("websearch_to_tsquery('simple', %s)", paramRef)

	whereSQL = fmt.Sprintf("%s @@ %s", tsvector, tsquery)
	rankSQL = fmt.Sprintf("ts_rank(%s, %s)", tsvector, tsquery)
	args = []any{searchTerm}

	return whereSQL, rankSQL, args, nil
}
