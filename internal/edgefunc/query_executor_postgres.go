// Package edgefunc This file provides SQL builder functions and row scanning for executing CRUD queries against PostgreSQL. It translates a Query struct into parameterized SQL statements with proper identifier quoting and parameter indexing.
package edgefunc

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/allyourbase/ayb/internal/sqlutil"
)

// Compile-time interface check.
var _ QueryExecutor = (*PostgresQueryExecutor)(nil)
var _ SpatialQueryExecutor = (*PostgresQueryExecutor)(nil)

// PostgresQueryExecutor executes ayb.db queries against PostgreSQL.
type PostgresQueryExecutor struct {
	pool          *pgxpool.Pool
	allowedTables map[string]struct{}
}

// NewPostgresQueryExecutor creates a query executor backed by pgxpool.
// If allowedTables is non-empty, queries are restricted to those table names.
func NewPostgresQueryExecutor(pool *pgxpool.Pool, allowedTables []string) *PostgresQueryExecutor {
	allow := make(map[string]struct{}, len(allowedTables))
	for _, table := range allowedTables {
		name := strings.TrimSpace(table)
		if name == "" {
			continue
		}
		allow[name] = struct{}{}
	}
	if len(allow) == 0 {
		allow = nil
	}
	return &PostgresQueryExecutor{
		pool:          pool,
		allowedTables: allow,
	}
}

// Execute translates Query into parameterized SQL and executes it.
func (e *PostgresQueryExecutor) Execute(ctx context.Context, query Query) (QueryResult, error) {
	query.Table = strings.TrimSpace(query.Table)
	query.Action = strings.ToLower(strings.TrimSpace(query.Action))
	if query.Table == "" {
		return QueryResult{}, fmt.Errorf("query table is required")
	}

	if len(e.allowedTables) > 0 {
		if !isTableAllowed(e.allowedTables, query.Table) {
			return QueryResult{}, fmt.Errorf("table %q is not allowed", query.Table)
		}
	}

	sql, args, err := buildQuerySQL(query)
	if err != nil {
		return QueryResult{}, err
	}

	return e.queryRows(ctx, sql, "executing query", args...)
}

// QueryRaw executes parameterized SQL with allowlist enforcement across every
// directly-referenced table. When an allowlist is configured, multiple
// statements and derived table sources are rejected because they cannot be
// safely validated by the lightweight SQL scanner.
func (e *PostgresQueryExecutor) QueryRaw(ctx context.Context, sql string, args ...any) (QueryResult, error) {
	trimmedSQL := strings.TrimSpace(sql)
	if trimmedSQL == "" {
		return QueryResult{}, fmt.Errorf("sql is required")
	}

	if len(e.allowedTables) > 0 {
		targetTables, err := extractReferencedTablesFromSQL(trimmedSQL)
		if err != nil {
			return QueryResult{}, err
		}
		for _, targetTable := range targetTables {
			if !isTableAllowed(e.allowedTables, targetTable) {
				return QueryResult{}, fmt.Errorf("table %q is not allowed", targetTable)
			}
		}
	}

	return e.queryRows(ctx, trimmedSQL, "executing raw query", args...)
}

// TODO: Document PostgresQueryExecutor.queryRows.
func (e *PostgresQueryExecutor) queryRows(ctx context.Context, sql, queryErrorContext string, args ...any) (QueryResult, error) {
	if e.pool == nil {
		return QueryResult{}, fmt.Errorf("postgres query executor requires a pool")
	}

	rows, err := e.pool.Query(ctx, sql, args...)
	if err != nil {
		return QueryResult{}, fmt.Errorf("%s: %w", queryErrorContext, err)
	}
	defer rows.Close()

	resultRows, err := scanQueryRows(rows)
	if err != nil {
		return QueryResult{}, fmt.Errorf("scanning query rows: %w", err)
	}
	return QueryResult{Rows: resultRows}, nil
}

// buildQuerySQL dispatches to the appropriate builder function (select, insert, update, or delete) based on the query action, returning parameterized SQL and arguments.
func buildQuerySQL(query Query) (string, []any, error) {
	table := strings.TrimSpace(query.Table)
	if table == "" {
		return "", nil, fmt.Errorf("query table is required")
	}

	switch strings.ToLower(strings.TrimSpace(query.Action)) {
	case "select":
		return buildSelectSQL(table, query.Columns, query.Filters)
	case "insert":
		return buildInsertSQL(table, query.Data)
	case "update":
		return buildUpdateSQL(table, query.Data, query.Filters)
	case "delete":
		return buildDeleteSQL(table, query.Filters)
	default:
		return "", nil, fmt.Errorf("unsupported action %q", query.Action)
	}
}

// buildSelectSQL constructs a SELECT statement with optional WHERE filtering. It quotes column identifiers, builds parameterized where conditions starting at parameter index 1, and returns the SQL string with argument values.
func buildSelectSQL(table, rawColumns string, filters []Filter) (string, []any, error) {
	columns, err := buildSelectColumns(rawColumns)
	if err != nil {
		return "", nil, err
	}

	whereClause, args, err := buildWhereClause(filters, 1)
	if err != nil {
		return "", nil, err
	}

	sql := fmt.Sprintf("SELECT %s FROM %s", columns, sqlutil.QuoteIdent(table))
	if whereClause != "" {
		sql += " WHERE " + whereClause
	}
	return sql, args, nil
}

// buildInsertSQL constructs an INSERT statement with a RETURNING * clause, sorting column keys for deterministic output and using parameterized placeholders for values.
func buildInsertSQL(table string, data map[string]interface{}) (string, []any, error) {
	if len(data) == 0 {
		return "", nil, fmt.Errorf("insert requires data")
	}

	keys := sortedKeys(data)
	columns := make([]string, 0, len(keys))
	placeholders := make([]string, 0, len(keys))
	args := make([]any, 0, len(keys))

	for i, key := range keys {
		column := strings.TrimSpace(key)
		if column == "" {
			return "", nil, fmt.Errorf("insert contains an empty column name")
		}
		columns = append(columns, sqlutil.QuoteIdent(column))
		placeholders = append(placeholders, fmt.Sprintf("$%d", i+1))
		args = append(args, data[key])
	}

	sql := fmt.Sprintf(
		"INSERT INTO %s (%s) VALUES (%s) RETURNING *",
		sqlutil.QuoteIdent(table),
		strings.Join(columns, ", "),
		strings.Join(placeholders, ", "),
	)
	return sql, args, nil
}

// buildUpdateSQL constructs an UPDATE statement with SET clauses for each data field and a WHERE clause from filters, using parameterized placeholders and returning all updated rows.
func buildUpdateSQL(table string, data map[string]interface{}, filters []Filter) (string, []any, error) {
	if len(data) == 0 {
		return "", nil, fmt.Errorf("update requires data")
	}
	if len(filters) == 0 {
		return "", nil, fmt.Errorf("update requires at least one filter")
	}

	keys := sortedKeys(data)
	setClauses := make([]string, 0, len(keys))
	args := make([]any, 0, len(keys)+len(filters))

	for i, key := range keys {
		column := strings.TrimSpace(key)
		if column == "" {
			return "", nil, fmt.Errorf("update contains an empty column name")
		}
		setClauses = append(setClauses, fmt.Sprintf("%s = $%d", sqlutil.QuoteIdent(column), i+1))
		args = append(args, data[key])
	}

	whereClause, whereArgs, err := buildWhereClause(filters, len(args)+1)
	if err != nil {
		return "", nil, err
	}
	args = append(args, whereArgs...)

	sql := fmt.Sprintf(
		"UPDATE %s SET %s WHERE %s RETURNING *",
		sqlutil.QuoteIdent(table),
		strings.Join(setClauses, ", "),
		whereClause,
	)
	return sql, args, nil
}

func buildDeleteSQL(table string, filters []Filter) (string, []any, error) {
	if len(filters) == 0 {
		return "", nil, fmt.Errorf("delete requires at least one filter")
	}

	whereClause, args, err := buildWhereClause(filters, 1)
	if err != nil {
		return "", nil, err
	}

	sql := fmt.Sprintf("DELETE FROM %s WHERE %s RETURNING *", sqlutil.QuoteIdent(table), whereClause)
	return sql, args, nil
}

// buildSelectColumns parses a comma-separated column list, quotes each identifier, and returns the formatted column clause or '*' for all columns; returns an error if '*' is mixed with explicit columns.
func buildSelectColumns(raw string) (string, error) {
	columns := strings.TrimSpace(raw)
	if columns == "" || columns == "*" {
		return "*", nil
	}

	parts := strings.Split(columns, ",")
	quoted := make([]string, 0, len(parts))
	for _, part := range parts {
		col := strings.TrimSpace(part)
		if col == "" {
			continue
		}
		if col == "*" {
			return "", fmt.Errorf("'*' cannot be mixed with explicit columns")
		}
		quoted = append(quoted, sqlutil.QuoteIdent(col))
	}

	if len(quoted) == 0 {
		return "*", nil
	}
	return strings.Join(quoted, ", "), nil
}

// buildWhereClause builds AND-joined WHERE conditions from filters, using parameterized placeholders starting at startIndex. It handles null filter values specially by converting eq/neq to IS NULL/IS NOT NULL checks.
func buildWhereClause(filters []Filter, startIndex int) (string, []any, error) {
	if len(filters) == 0 {
		return "", nil, nil
	}

	clauses := make([]string, 0, len(filters))
	args := make([]any, 0, len(filters))
	argIndex := startIndex
	for _, f := range filters {
		column := strings.TrimSpace(f.Column)
		if column == "" {
			return "", nil, fmt.Errorf("filter column is required")
		}

		opName := strings.ToLower(strings.TrimSpace(f.Op))
		if f.Value == nil {
			switch opName {
			case "eq":
				clauses = append(clauses, fmt.Sprintf("%s IS NULL", sqlutil.QuoteIdent(column)))
			case "neq":
				clauses = append(clauses, fmt.Sprintf("%s IS NOT NULL", sqlutil.QuoteIdent(column)))
			default:
				return "", nil, fmt.Errorf("nil filter value only supports eq/neq operators")
			}
			continue
		}

		op, err := sqlOperator(f.Op)
		if err != nil {
			return "", nil, err
		}

		clauses = append(clauses, fmt.Sprintf("%s %s $%d", sqlutil.QuoteIdent(column), op, argIndex))
		args = append(args, f.Value)
		argIndex++
	}
	return strings.Join(clauses, " AND "), args, nil
}

// sqlOperator maps filter operator names (eq, neq, gt, lt, gte, lte) to their SQL equivalents, returning an error for unsupported operators.
func sqlOperator(op string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(op)) {
	case "eq":
		return "=", nil
	case "neq":
		return "!=", nil
	case "gt":
		return ">", nil
	case "lt":
		return "<", nil
	case "gte":
		return ">=", nil
	case "lte":
		return "<=", nil
	default:
		return "", fmt.Errorf("unsupported filter operator %q", op)
	}
}

func sortedKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// scanQueryRows iterates through query result rows, scanning each into a map with column names as keys, and returns a slice of these maps or an empty slice if no rows matched.
func scanQueryRows(rows pgx.Rows) ([]map[string]interface{}, error) {
	var out []map[string]interface{}
	for rows.Next() {
		record, err := scanCurrentQueryRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, record)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if out == nil {
		out = []map[string]interface{}{}
	}
	return out, nil
}

// scanCurrentQueryRow reads the current row from pgx.Rows into a map keyed by column name, normalizing UUID values to their string representation.
func scanCurrentQueryRow(rows pgx.Rows) (map[string]interface{}, error) {
	descs := rows.FieldDescriptions()
	values := make([]any, len(descs))
	ptrs := make([]any, len(descs))
	for i := range values {
		ptrs[i] = &values[i]
	}

	if err := rows.Scan(ptrs...); err != nil {
		return nil, err
	}

	record := make(map[string]interface{}, len(descs))
	for i, desc := range descs {
		record[desc.Name] = normalizeQueryValue(values[i])
	}
	return record, nil
}

func normalizeQueryValue(v any) any {
	switch val := v.(type) {
	case [16]byte:
		return fmt.Sprintf("%x-%x-%x-%x-%x", val[0:4], val[4:6], val[6:8], val[8:10], val[10:16])
	default:
		return v
	}
}

func isTableAllowed(allowed map[string]struct{}, targetTable string) bool {
	if _, ok := allowed[targetTable]; ok {
		return true
	}

	targetUnqualified := unqualifiedTableName(targetTable)
	if _, ok := allowed[targetUnqualified]; !ok {
		return false
	}

	targetSchema := qualifiedTableSchema(targetTable)
	return targetSchema == "" || targetSchema == "public"
}

func unqualifiedTableName(table string) string {
	lastDot := strings.LastIndex(table, ".")
	if lastDot == -1 {
		return table
	}
	return table[lastDot+1:]
}

func qualifiedTableSchema(table string) string {
	lastDot := strings.LastIndex(table, ".")
	if lastDot == -1 {
		return ""
	}
	return table[:lastDot]
}
