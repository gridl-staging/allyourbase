// Package graphql Builds SQL statements for GraphQL mutations with parameter binding and constraint validation.
package graphql

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/sqlutil"
)

// buildBatchInsertStatement generates an INSERT statement for multiple rows with optional ON CONFLICT handling, returning the SQL string and parameter values.
func buildBatchInsertStatement(tbl *schema.Table, objects []map[string]interface{}, onConflict map[string]interface{}) (string, []any, error) {
	if tbl == nil {
		return "", nil, fmt.Errorf("table is nil")
	}
	if len(objects) == 0 {
		return "", nil, fmt.Errorf("objects must include at least one object")
	}

	columnSet := make(map[string]struct{})
	for _, object := range objects {
		for key := range object {
			if tbl.ColumnByName(key) == nil {
				return "", nil, fmt.Errorf("unknown column: %s", key)
			}
			columnSet[key] = struct{}{}
		}
	}

	columns := make([]string, 0, len(columnSet))
	for column := range columnSet {
		columns = append(columns, column)
	}
	sort.Strings(columns)
	if len(columns) == 0 {
		for _, col := range tbl.Columns {
			columns = append(columns, col.Name)
		}
		sort.Strings(columns)
	}

	var b strings.Builder
	b.WriteString("INSERT INTO ")
	b.WriteString(sqlutil.QuoteQualifiedName(tbl.Schema, tbl.Name))
	b.WriteString(" (")
	for idx, column := range columns {
		if idx > 0 {
			b.WriteString(", ")
		}
		b.WriteString(sqlutil.QuoteIdent(column))
	}
	b.WriteString(") VALUES ")

	args := make([]any, 0, len(objects)*len(columns))
	paramIdx := 1
	for objIdx, object := range objects {
		if objIdx > 0 {
			b.WriteString(", ")
		}
		b.WriteString("(")
		for colIdx, column := range columns {
			if colIdx > 0 {
				b.WriteString(", ")
			}
			if value, ok := object[column]; ok {
				b.WriteString(fmt.Sprintf("$%d", paramIdx))
				args = append(args, value)
				paramIdx++
				continue
			}
			b.WriteString("DEFAULT")
		}
		b.WriteString(")")
	}

	onConflictSQL, err := buildOnConflictClause(tbl, onConflict)
	if err != nil {
		return "", nil, err
	}
	b.WriteString(onConflictSQL)
	b.WriteString(" RETURNING ")
	b.WriteString(buildGraphQLProjection(tbl))
	return b.String(), args, nil
}

// buildUpdateStatement generates an UPDATE statement supporting multiple operators: _set for direct column assignment, _inc for numeric column increment, _append and _prepend for JSON column operations, returning the SQL string and all parameter values.
func buildUpdateStatement(
	tbl *schema.Table,
	where map[string]interface{},
	set map[string]interface{},
	inc map[string]interface{},
	appendJ map[string]interface{},
	prependJ map[string]interface{},
) (string, []any, error) {
	if tbl == nil {
		return "", nil, fmt.Errorf("table is nil")
	}
	if err := validateUpdateOperators(set, inc, appendJ, prependJ); err != nil {
		return "", nil, err
	}

	assignments := make([]string, 0, len(set)+len(inc)+len(appendJ)+len(prependJ))
	args := make([]any, 0, len(assignments))
	addAssignment := func(column string, assignmentSQL string, value any) {
		assignments = append(assignments, assignmentSQL)
		args = append(args, value)
	}

	for _, key := range sortedObjectKeys(set) {
		if tbl.ColumnByName(key) == nil {
			return "", nil, fmt.Errorf("unknown column: %s", key)
		}
		addAssignment(key, fmt.Sprintf("%s = $%d", sqlutil.QuoteIdent(key), len(args)+1), set[key])
	}

	for _, key := range sortedObjectKeys(inc) {
		col := tbl.ColumnByName(key)
		if col == nil {
			return "", nil, fmt.Errorf("unknown column: %s", key)
		}
		if !isNumericColumn(col) {
			return "", nil, fmt.Errorf("column '%s' is not numeric, cannot use _inc", key)
		}
		quoted := sqlutil.QuoteIdent(key)
		addAssignment(key, fmt.Sprintf("%s = %s + $%d", quoted, quoted, len(args)+1), inc[key])
	}

	for _, key := range sortedObjectKeys(appendJ) {
		col := tbl.ColumnByName(key)
		if col == nil {
			return "", nil, fmt.Errorf("unknown column: %s", key)
		}
		if !col.IsJSON {
			return "", nil, fmt.Errorf("column '%s' is not json, cannot use _append", key)
		}
		quoted := sqlutil.QuoteIdent(key)
		addAssignment(key, fmt.Sprintf("%s = %s || $%d::jsonb", quoted, quoted, len(args)+1), appendJ[key])
	}

	for _, key := range sortedObjectKeys(prependJ) {
		col := tbl.ColumnByName(key)
		if col == nil {
			return "", nil, fmt.Errorf("unknown column: %s", key)
		}
		if !col.IsJSON {
			return "", nil, fmt.Errorf("column '%s' is not json, cannot use _prepend", key)
		}
		quoted := sqlutil.QuoteIdent(key)
		addAssignment(key, fmt.Sprintf("%s = $%d::jsonb || %s", quoted, len(args)+1, quoted), prependJ[key])
	}

	var b strings.Builder
	b.WriteString("UPDATE ")
	b.WriteString(sqlutil.QuoteQualifiedName(tbl.Schema, tbl.Name))
	b.WriteString(" SET ")
	b.WriteString(strings.Join(assignments, ", "))

	whereSQL, whereArgs, err := resolveWhere(where, tbl, len(args)+1)
	if err != nil {
		return "", nil, err
	}
	if whereSQL != "" {
		b.WriteString(" WHERE ")
		b.WriteString(whereSQL)
		args = append(args, whereArgs...)
	}
	b.WriteString(" RETURNING ")
	b.WriteString(buildGraphQLProjection(tbl))
	return b.String(), args, nil
}

func isNumericColumn(col *schema.Column) bool {
	if col == nil {
		return false
	}
	switch strings.ToLower(col.TypeName) {
	case "smallint", "int2", "integer", "int4", "bigint", "int8",
		"smallserial", "serial2", "serial", "serial4", "bigserial", "serial8",
		"real", "float4", "double precision", "float8", "numeric", "decimal":
		return true
	default:
		return false
	}
}

// buildDeleteStatement generates a DELETE FROM table WHERE ... RETURNING * SQL statement with parameter placeholders, returning the SQL string and where clause argument values.
func buildDeleteStatement(tbl *schema.Table, where map[string]interface{}) (string, []any, error) {
	if tbl == nil {
		return "", nil, fmt.Errorf("table is nil")
	}

	var b strings.Builder
	b.WriteString("DELETE FROM ")
	b.WriteString(sqlutil.QuoteQualifiedName(tbl.Schema, tbl.Name))

	whereSQL, whereArgs, err := resolveWhere(where, tbl, 1)
	if err != nil {
		return "", nil, err
	}
	if whereSQL != "" {
		b.WriteString(" WHERE ")
		b.WriteString(whereSQL)
	}
	b.WriteString(" RETURNING ")
	b.WriteString(buildGraphQLProjection(tbl))
	return b.String(), whereArgs, nil
}

// buildOnConflictClause generates a PostgreSQL ON CONFLICT clause for INSERT statements, validating the constraint name and update columns against the table schema.
func buildOnConflictClause(tbl *schema.Table, onConflict map[string]interface{}) (string, error) {
	if len(onConflict) == 0 {
		return "", nil
	}

	constraintRaw, ok := onConflict["constraint"]
	if !ok || constraintRaw == nil {
		return "", fmt.Errorf("on_conflict.constraint is required")
	}
	constraint, ok := constraintRaw.(string)
	if !ok || strings.TrimSpace(constraint) == "" {
		return "", fmt.Errorf("on_conflict.constraint must be a non-empty string")
	}
	if !isAllowedConstraint(tbl, constraint) {
		return "", fmt.Errorf("unknown constraint: %s", constraint)
	}

	updateColumns, err := parseStringList(onConflict["update_columns"])
	if err != nil {
		return "", fmt.Errorf("on_conflict.update_columns: %w", err)
	}
	updateColumns = uniqueStringsPreserveOrder(updateColumns)
	for _, col := range updateColumns {
		if tbl.ColumnByName(col) == nil {
			return "", fmt.Errorf("unknown column: %s", col)
		}
	}

	var b strings.Builder
	b.WriteString(" ON CONFLICT ON CONSTRAINT ")
	b.WriteString(sqlutil.QuoteIdent(constraint))
	if len(updateColumns) == 0 {
		b.WriteString(" DO NOTHING")
		return b.String(), nil
	}

	assignments := make([]string, 0, len(updateColumns))
	for _, col := range updateColumns {
		quoted := sqlutil.QuoteIdent(col)
		assignments = append(assignments, quoted+" = EXCLUDED."+quoted)
	}
	b.WriteString(" DO UPDATE SET ")
	b.WriteString(strings.Join(assignments, ", "))
	return b.String(), nil
}

func uniqueStringsPreserveOrder(values []string) []string {
	if len(values) < 2 {
		return values
	}
	seen := make(map[string]struct{}, len(values))
	unique := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		unique = append(unique, value)
	}
	return unique
}

func isAllowedConstraint(tbl *schema.Table, constraint string) bool {
	allowed := mutationConstraintNames(tbl)
	return slices.Contains(allowed, constraint)
}

// parseStringList converts an interface value to a string slice, accepting either []interface{} with string items or []string, returning an error if the value is not a list or contains non-string items.
func parseStringList(v interface{}) ([]string, error) {
	if v == nil {
		return nil, nil
	}
	switch raw := v.(type) {
	case []interface{}:
		out := make([]string, 0, len(raw))
		for _, item := range raw {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("must be a list of strings")
			}
			out = append(out, s)
		}
		return out, nil
	case []string:
		out := make([]string, len(raw))
		copy(out, raw)
		return out, nil
	default:
		return nil, fmt.Errorf("must be a list")
	}
}

func sortedObjectKeys(m map[string]interface{}) []string {
	if len(m) == 0 {
		return nil
	}
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// buildSelectStatement generates a SELECT statement for a table with optional WHERE conditions, returning the SQL string and parameter values.
func buildSelectStatement(tbl *schema.Table, where map[string]interface{}) (string, []any, error) {
	if tbl == nil {
		return "", nil, fmt.Errorf("table is nil")
	}

	var b strings.Builder
	b.WriteString("SELECT * FROM ")
	b.WriteString(sqlutil.QuoteQualifiedName(tbl.Schema, tbl.Name))

	whereSQL, whereArgs, err := resolveWhere(where, tbl, 1)
	if err != nil {
		return "", nil, err
	}
	if whereSQL != "" {
		b.WriteString(" WHERE ")
		b.WriteString(whereSQL)
	}
	return b.String(), whereArgs, nil
}
