package api

import (
	"fmt"
	"strings"

	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/sqlutil"
)

func buildImportSQL(tbl *schema.Table, cols []string, onConflict string) string {
	switch onConflict {
	case "skip":
		return buildImportSkipSQL(tbl, cols)
	case "update":
		return buildImportUpdateSQL(tbl, cols)
	default:
		return buildImportInsertSQL(tbl, cols)
	}
}

// buildImportInsertSQL builds a plain INSERT statement.
func buildImportInsertSQL(tbl *schema.Table, cols []string) string {
	quoted := make([]string, len(cols))
	placeholders := make([]string, len(cols))
	for i, col := range cols {
		quoted[i] = sqlutil.QuoteIdent(col)
		placeholders[i] = fmt.Sprintf("$%d", i+1)
	}
	return fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
		sqlutil.QuoteQualifiedName(tbl.Schema, tbl.Name),
		strings.Join(quoted, ", "),
		strings.Join(placeholders, ", "),
	)
}

// buildImportSkipSQL builds an INSERT ... ON CONFLICT DO NOTHING statement.
func buildImportSkipSQL(tbl *schema.Table, cols []string) string {
	base := buildImportInsertSQL(tbl, cols)
	pkCols := make([]string, len(tbl.PrimaryKey))
	for i, pk := range tbl.PrimaryKey {
		pkCols[i] = sqlutil.QuoteIdent(pk)
	}
	return fmt.Sprintf("%s ON CONFLICT (%s) DO NOTHING",
		base, strings.Join(pkCols, ", "))
}

// buildImportUpdateSQL builds an INSERT ... ON CONFLICT DO UPDATE SET ... statement.
// PK columns are excluded from the SET clause.
func buildImportUpdateSQL(tbl *schema.Table, cols []string) string {
	base := buildImportInsertSQL(tbl, cols)
	pkCols := make([]string, len(tbl.PrimaryKey))
	pkSet := make(map[string]bool, len(tbl.PrimaryKey))
	for i, pk := range tbl.PrimaryKey {
		pkCols[i] = sqlutil.QuoteIdent(pk)
		pkSet[pk] = true
	}

	var setClauses []string
	for _, col := range cols {
		if pkSet[col] {
			continue
		}
		setClauses = append(setClauses, fmt.Sprintf("%s = EXCLUDED.%s", sqlutil.QuoteIdent(col), sqlutil.QuoteIdent(col)))
	}

	if len(setClauses) == 0 {
		// All columns are PKs; fall back to DO NOTHING.
		return fmt.Sprintf("%s ON CONFLICT (%s) DO NOTHING",
			base, strings.Join(pkCols, ", "))
	}

	return fmt.Sprintf("%s ON CONFLICT (%s) DO UPDATE SET %s",
		base, strings.Join(pkCols, ", "), strings.Join(setClauses, ", "))
}
