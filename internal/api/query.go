package api

import (
	"fmt"
	"strings"

	"github.com/allyourbase/ayb/internal/schema"
)

// quoteIdent safely quotes a SQL identifier to prevent injection.
// Only identifiers that have been validated against the schema cache should reach here.
func quoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

// tableRef returns the fully-qualified, quoted "schema"."table" reference.
func tableRef(tbl *schema.Table) string {
	return quoteIdent(tbl.Schema) + "." + quoteIdent(tbl.Name)
}

// buildSelectOne builds a SELECT query for a single record by primary key.
func buildSelectOne(tbl *schema.Table, fields []string, pkValues []string) (string, []any) {
	cols := buildColumnList(tbl, fields)
	where, args := buildPKWhere(tbl, pkValues)

	q := fmt.Sprintf("SELECT %s FROM %s WHERE %s LIMIT 1", cols, tableRef(tbl), where)
	return q, args
}

// buildInsert builds an INSERT ... RETURNING * statement.
func buildInsert(tbl *schema.Table, data map[string]any) (string, []any) {
	columns := make([]string, 0, len(data))
	placeholders := make([]string, 0, len(data))
	args := make([]any, 0, len(data))

	i := 1
	for col, val := range data {
		if tbl.ColumnByName(col) == nil {
			continue // skip unknown columns
		}
		columns = append(columns, quoteIdent(col))
		placeholders = append(placeholders, fmt.Sprintf("$%d", i))
		args = append(args, val)
		i++
	}

	q := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s) RETURNING *",
		tableRef(tbl),
		strings.Join(columns, ", "),
		strings.Join(placeholders, ", "),
	)
	return q, args
}

// buildUpdate builds an UPDATE ... SET ... WHERE pk = ... RETURNING * statement.
func buildUpdate(tbl *schema.Table, data map[string]any, pkValues []string) (string, []any) {
	setClauses := make([]string, 0, len(data))
	args := make([]any, 0, len(data)+len(tbl.PrimaryKey))

	i := 1
	for col, val := range data {
		if tbl.ColumnByName(col) == nil {
			continue
		}
		setClauses = append(setClauses, fmt.Sprintf("%s = $%d", quoteIdent(col), i))
		args = append(args, val)
		i++
	}

	// Build PK where clause starting at current param index.
	whereParts := make([]string, len(tbl.PrimaryKey))
	for j, pk := range tbl.PrimaryKey {
		whereParts[j] = fmt.Sprintf("%s = $%d", quoteIdent(pk), i)
		args = append(args, pkValues[j])
		i++
	}

	q := fmt.Sprintf("UPDATE %s SET %s WHERE %s RETURNING *",
		tableRef(tbl),
		strings.Join(setClauses, ", "),
		strings.Join(whereParts, " AND "),
	)
	return q, args
}

// buildDelete builds a DELETE ... WHERE pk = ... statement.
func buildDelete(tbl *schema.Table, pkValues []string) (string, []any) {
	where, args := buildPKWhere(tbl, pkValues)
	q := fmt.Sprintf("DELETE FROM %s WHERE %s", tableRef(tbl), where)
	return q, args
}

// buildPKWhere builds the WHERE clause for primary key matching.
func buildPKWhere(tbl *schema.Table, pkValues []string) (string, []any) {
	parts := make([]string, len(tbl.PrimaryKey))
	args := make([]any, len(tbl.PrimaryKey))
	for i, pk := range tbl.PrimaryKey {
		parts[i] = fmt.Sprintf("%s = $%d", quoteIdent(pk), i+1)
		args[i] = pkValues[i]
	}
	return strings.Join(parts, " AND "), args
}

// buildColumnList builds the column selection for SELECT queries.
// If fields is empty, returns "*".
func buildColumnList(tbl *schema.Table, fields []string) string {
	if len(fields) == 0 {
		return "*"
	}
	quoted := make([]string, 0, len(fields))
	for _, f := range fields {
		if tbl.ColumnByName(f) != nil {
			quoted = append(quoted, quoteIdent(f))
		}
	}
	if len(quoted) == 0 {
		return "*"
	}
	return strings.Join(quoted, ", ")
}

// buildList builds a SELECT query for listing records with pagination, sort, and optional filter/search.
func buildList(tbl *schema.Table, opts listOpts) (dataQuery string, dataArgs []any, countQuery string, countArgs []any) {
	cols := buildColumnList(tbl, opts.fields)
	ref := tableRef(tbl)

	// Collect WHERE parts and args from filter and search.
	var whereParts []string
	var allWhereArgs []any

	if opts.filterSQL != "" {
		whereParts = append(whereParts, opts.filterSQL)
		allWhereArgs = append(allWhereArgs, opts.filterArgs...)
	}

	if opts.searchSQL != "" {
		whereParts = append(whereParts, opts.searchSQL)
		allWhereArgs = append(allWhereArgs, opts.searchArgs...)
	}

	whereClause := ""
	if len(whereParts) > 0 {
		whereClause = " WHERE " + strings.Join(whereParts, " AND ")
	}

	// Count query (unless skipTotal).
	if !opts.skipTotal {
		countQuery = fmt.Sprintf("SELECT COUNT(*) FROM %s%s", ref, whereClause)
		countArgs = append([]any{}, allWhereArgs...)
	}

	// Data query — when search is active, default to relevance ordering.
	orderClause := ""
	if opts.sortSQL != "" {
		orderClause = " ORDER BY " + opts.sortSQL
	} else if opts.searchRank != "" {
		orderClause = " ORDER BY " + opts.searchRank + " DESC"
	}

	offset := (opts.page - 1) * opts.perPage
	argIdx := len(allWhereArgs) + 1

	dataQuery = fmt.Sprintf("SELECT %s FROM %s%s%s LIMIT $%d OFFSET $%d",
		cols, ref, whereClause, orderClause, argIdx, argIdx+1)
	dataArgs = append(append([]any{}, allWhereArgs...), opts.perPage, offset)

	return
}

// listOpts holds the parsed query parameters for a list request.
type listOpts struct {
	page       int
	perPage    int
	skipTotal  bool
	fields     []string
	sortSQL    string
	filterSQL  string
	filterArgs []any
	searchSQL  string // FTS WHERE clause
	searchRank string // FTS ts_rank() expression for ORDER BY
	searchArgs []any  // search term parameter
}

// parsePKValues splits a composite primary key value from the URL.
// Single PKs return a single-element slice. Composite PKs are comma-separated.
func parsePKValues(id string, numPKCols int) []string {
	if numPKCols <= 1 {
		return []string{id}
	}
	return strings.SplitN(id, ",", numPKCols)
}
