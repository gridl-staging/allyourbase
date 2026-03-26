package api

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/sqlutil"
)

// buildSelectOne builds a SELECT query for a single record by primary key.
func buildSelectOne(tbl *schema.Table, fields []string, pkValues []string) (string, []any) {
	cols := buildColumnList(tbl, fields)
	where, args := buildPKWhere(tbl, pkValues)

	q := fmt.Sprintf("SELECT %s FROM %s WHERE %s LIMIT 1", cols, sqlutil.QuoteQualifiedName(tbl.Schema, tbl.Name), where)
	return q, args
}

// buildInsert builds an INSERT statement with a geometry-aware RETURNING clause.
func buildInsert(tbl *schema.Table, data map[string]any) (string, []any) {
	columns := make([]string, 0, len(data))
	placeholders := make([]string, 0, len(data))
	args := make([]any, 0, len(data))

	i := 1
	for col, val := range data {
		column := tbl.ColumnByName(col)
		if column == nil {
			continue // skip unknown columns
		}
		columns = append(columns, sqlutil.QuoteIdent(col))
		if column.IsGeometry {
			if val == nil {
				placeholders = append(placeholders, "NULL")
			} else {
				geomExpr := fmt.Sprintf("ST_GeomFromGeoJSON($%d)", i)
				if column.IsGeography {
					geomExpr += "::geography"
				}
				placeholders = append(placeholders, geomExpr)
				args = append(args, marshalGeoJSONValue(val))
				i++
			}
			continue
		}

		placeholders = append(placeholders, fmt.Sprintf("$%d", i))
		args = append(args, val)
		i++
	}

	q := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s) %s",
		sqlutil.QuoteQualifiedName(tbl.Schema, tbl.Name),
		strings.Join(columns, ", "),
		strings.Join(placeholders, ", "),
		buildReturningClause(tbl),
	)
	return q, args
}

// buildUpdate builds an UPDATE statement with geometry-aware SET and RETURNING clauses.
func buildUpdate(tbl *schema.Table, data map[string]any, pkValues []string) (string, []any) {
	setClauses := make([]string, 0, len(data))
	args := make([]any, 0, len(data)+len(tbl.PrimaryKey))

	i := 1
	for col, val := range data {
		column := tbl.ColumnByName(col)
		if column == nil {
			continue
		}

		if column.IsGeometry {
			if val == nil {
				setClauses = append(setClauses, fmt.Sprintf("%s = NULL", sqlutil.QuoteIdent(col)))
			} else {
				geomExpr := fmt.Sprintf("ST_GeomFromGeoJSON($%d)", i)
				if column.IsGeography {
					geomExpr += "::geography"
				}
				setClauses = append(setClauses, fmt.Sprintf("%s = %s", sqlutil.QuoteIdent(col), geomExpr))
				args = append(args, marshalGeoJSONValue(val))
				i++
			}
			continue
		}

		setClauses = append(setClauses, fmt.Sprintf("%s = $%d", sqlutil.QuoteIdent(col), i))
		args = append(args, val)
		i++
	}

	// Build PK where clause starting at current param index.
	whereParts := make([]string, len(tbl.PrimaryKey))
	for j, pk := range tbl.PrimaryKey {
		whereParts[j] = fmt.Sprintf("%s = $%d", sqlutil.QuoteIdent(pk), i)
		args = append(args, pkValues[j])
		i++
	}

	q := fmt.Sprintf("UPDATE %s SET %s WHERE %s %s",
		sqlutil.QuoteQualifiedName(tbl.Schema, tbl.Name),
		strings.Join(setClauses, ", "),
		strings.Join(whereParts, " AND "),
		buildReturningClause(tbl),
	)
	return q, args
}

// buildDelete builds a DELETE ... WHERE pk = ... statement.
func buildDelete(tbl *schema.Table, pkValues []string) (string, []any) {
	where, args := buildPKWhere(tbl, pkValues)
	q := fmt.Sprintf("DELETE FROM %s WHERE %s", sqlutil.QuoteQualifiedName(tbl.Schema, tbl.Name), where)
	return q, args
}

// buildDeleteReturning builds a DELETE ... WHERE pk = ... RETURNING * statement
// so callers can capture the deleted row for audit logging in a single SQL round-trip.
func buildDeleteReturning(tbl *schema.Table, pkValues []string) (string, []any) {
	where, args := buildPKWhere(tbl, pkValues)
	q := fmt.Sprintf("DELETE FROM %s WHERE %s %s", sqlutil.QuoteQualifiedName(tbl.Schema, tbl.Name), where, buildReturningClause(tbl))
	return q, args
}

// buildUpdateWithAudit builds an UPDATE that first captures the old row via a
// CTE so old and new values are returned atomically in one statement.
// The RETURNING clause includes the regular columns plus
// "_audit_old_values json" (the pre-update row serialised via row_to_json).
func buildUpdateWithAudit(tbl *schema.Table, data map[string]any, pkValues []string) (string, []any) {
	setClauses := make([]string, 0, len(data))
	args := make([]any, 0, len(data)+len(tbl.PrimaryKey))

	i := 1
	for col, val := range data {
		column := tbl.ColumnByName(col)
		if column == nil {
			continue
		}
		if column.IsGeometry {
			if val == nil {
				setClauses = append(setClauses, fmt.Sprintf("%s = NULL", sqlutil.QuoteIdent(col)))
			} else {
				geomExpr := fmt.Sprintf("ST_GeomFromGeoJSON($%d)", i)
				if column.IsGeography {
					geomExpr += "::geography"
				}
				setClauses = append(setClauses, fmt.Sprintf("%s = %s", sqlutil.QuoteIdent(col), geomExpr))
				args = append(args, marshalGeoJSONValue(val))
				i++
			}
			continue
		}
		setClauses = append(setClauses, fmt.Sprintf("%s = $%d", sqlutil.QuoteIdent(col), i))
		args = append(args, val)
		i++
	}

	whereParts := make([]string, len(tbl.PrimaryKey))
	for j, pk := range tbl.PrimaryKey {
		whereParts[j] = fmt.Sprintf("%s = $%d", sqlutil.QuoteIdent(pk), i)
		args = append(args, pkValues[j])
		i++
	}
	whereSQL := strings.Join(whereParts, " AND ")

	ref := sqlutil.QuoteQualifiedName(tbl.Schema, tbl.Name)
	returning := buildReturningClause(tbl)

	// CTE captures the old row before the update so the read and write happen
	// atomically within the same statement — no separate SELECT needed.
	q := fmt.Sprintf(
		`WITH _old AS (SELECT * FROM %s WHERE %s)
UPDATE %s SET %s WHERE %s %s, (SELECT row_to_json(_old.*) FROM _old) AS _audit_old_values`,
		ref, whereSQL,
		ref, strings.Join(setClauses, ", "), whereSQL, returning,
	)
	return q, args
}

// buildPKWhere builds the WHERE clause for primary key matching.
func buildPKWhere(tbl *schema.Table, pkValues []string) (string, []any) {
	parts := make([]string, len(tbl.PrimaryKey))
	args := make([]any, len(tbl.PrimaryKey))
	for i, pk := range tbl.PrimaryKey {
		parts[i] = fmt.Sprintf("%s = $%d", sqlutil.QuoteIdent(pk), i+1)
		args[i] = pkValues[i]
	}
	return strings.Join(parts, " AND "), args
}

func marshalGeoJSONValue(v any) string {
	switch val := v.(type) {
	case string:
		return val
	case []byte:
		return string(val)
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprintf("%v", v)
		}
		return string(b)
	}
}

func selectExprForColumn(col *schema.Column) string {
	if col.IsGeometry || col.IsGeography {
		return fmt.Sprintf("ST_AsGeoJSON(%s)::jsonb AS %s", sqlutil.QuoteIdent(col.Name), sqlutil.QuoteIdent(col.Name))
	}
	return sqlutil.QuoteIdent(col.Name)
}

func buildAllColumnsProjection(tbl *schema.Table) string {
	parts := make([]string, 0, len(tbl.Columns))
	for _, col := range tbl.Columns {
		parts = append(parts, selectExprForColumn(col))
	}
	return strings.Join(parts, ", ")
}

// buildColumnList builds the column selection for SELECT queries.
func buildColumnList(tbl *schema.Table, fields []string) string {
	if len(fields) == 0 {
		if tbl.HasGeometry() {
			return buildAllColumnsProjection(tbl)
		}
		return "*"
	}

	parts := make([]string, 0, len(fields))
	for _, f := range fields {
		col := tbl.ColumnByName(f)
		if col != nil {
			parts = append(parts, selectExprForColumn(col))
		}
	}

	if len(parts) == 0 {
		if tbl.HasGeometry() {
			return buildAllColumnsProjection(tbl)
		}
		return "*"
	}
	return strings.Join(parts, ", ")
}

func buildReturningClause(tbl *schema.Table) string {
	if !tbl.HasGeometry() {
		return "RETURNING *"
	}
	return "RETURNING " + buildAllColumnsProjection(tbl)
}

type sqlCondition struct {
	clause string
	args   []any
}

func combineSQLConditions(conditions ...sqlCondition) (string, []any) {
	whereParts := make([]string, 0, len(conditions))
	args := make([]any, 0, len(conditions))
	for _, condition := range conditions {
		if condition.clause == "" {
			continue
		}
		whereParts = append(whereParts, condition.clause)
		args = append(args, condition.args...)
	}
	return strings.Join(whereParts, " AND "), args
}

func appendSelectExprs(baseColumns string, extraExprs []string) string {
	combined := baseColumns
	for _, expr := range extraExprs {
		if expr == "" {
			continue
		}
		combined += ", " + expr
	}
	return combined
}

// buildList builds a SELECT query for listing records with pagination, sort, and optional filter/search.
func buildList(tbl *schema.Table, opts listOpts) (dataQuery string, dataArgs []any, countQuery string, countArgs []any) {
	cols := buildColumnList(tbl, opts.fields)
	ref := sqlutil.QuoteQualifiedName(tbl.Schema, tbl.Name)

	combinedPredicate, allWhereArgs := combineSQLConditions(
		sqlCondition{clause: opts.filterSQL, args: opts.filterArgs},
		sqlCondition{clause: opts.spatialSQL, args: opts.spatialArgs},
		sqlCondition{clause: opts.searchSQL, args: opts.searchArgs},
	)
	whereClause := ""
	if combinedPredicate != "" {
		whereClause = " WHERE " + combinedPredicate
	}

	// Count query (unless skipTotal).
	if !opts.skipTotal {
		countQuery = fmt.Sprintf("SELECT COUNT(*) FROM %s%s", ref, whereClause)
		countArgs = append([]any{}, allWhereArgs...)
	}

	sortFields := opts.sortFields
	distanceSelect := opts.distanceSelect
	sortArgs := opts.sortArgs

	if len(sortFields) == 0 && len(opts.sort.Terms) > 0 {
		resolved, err := resolveStructuredSort(opts.sort, len(allWhereArgs)+1)
		if err == nil {
			sortFields = resolved.Fields
			distanceSelect = resolved.DistanceSelect
			sortArgs = resolved.Args
		}
	}

	if distanceSelect != "" {
		cols = appendSelectExprs(cols, []string{distanceSelect})
	}

	// Data query — when search is active, default to relevance ordering.
	orderClause := ""
	if len(sortFields) > 0 {
		orderClause = " ORDER BY " + sortFieldsToSQL(sortFields)
	} else if opts.sortSQL != "" {
		orderClause = " ORDER BY " + opts.sortSQL
	} else if opts.searchRank != "" {
		orderClause = " ORDER BY " + opts.searchRank + " DESC"
	}

	offset := (opts.page - 1) * opts.perPage
	argsWithSort := append(append([]any{}, allWhereArgs...), sortArgs...)
	argIdx := len(argsWithSort) + 1

	dataQuery = fmt.Sprintf("SELECT %s FROM %s%s%s LIMIT $%d OFFSET $%d",
		cols, ref, whereClause, orderClause, argIdx, argIdx+1)
	dataArgs = append(argsWithSort, opts.perPage, offset)

	return
}

// listOpts holds the parsed query parameters for a list request.
type listOpts struct {
	table               *schema.Table
	page                int
	perPage             int
	skipTotal           bool
	fields              []string
	sortSQL             string
	sort                StructuredSort
	sortFields          []SortField
	sortArgs            []any
	distanceSelect      string
	cursorSelects       []string
	cursorHelperColumns []string
	filterSQL           string
	filterArgs          []any
	spatialSQL          string
	spatialArgs         []any
	searchSQL           string // FTS WHERE clause
	searchRank          string // FTS ts_rank() expression for ORDER BY
	searchArgs          []any  // search term parameter
}

// parsePKValues splits a composite primary key value from the URL.
// Single PKs return a single-element slice. Composite PKs are comma-separated.
func parsePKValues(id string, numPKCols int) []string {
	if numPKCols <= 1 {
		return []string{id}
	}
	return strings.SplitN(id, ",", numPKCols)
}
