// Package graphql resolve.go contains GraphQL resolver functions that execute SQL queries with Row-Level Security support, including utilities for building parameterized WHERE and ORDER BY clauses and managing RLS context from authentication claims.
package graphql

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/spatial"
	"github.com/allyourbase/ayb/internal/sqlutil"
)

// DefaultMaxLimit is the maximum number of rows a GraphQL query can return.
// Applied when no limit is specified or when the requested limit exceeds this value.
const DefaultMaxLimit = 1000

// operatorSQL maps WhereInput operator keys to SQL operators.
var operatorSQL = map[string]string{
	"_eq":    "=",
	"_neq":   "!=",
	"_gt":    ">",
	"_gte":   ">=",
	"_lt":    "<",
	"_lte":   "<=",
	"_like":  "LIKE",
	"_ilike": "ILIKE",
}

type queryRunner interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

type txContextKey struct{}

func ctxWithTx(ctx context.Context, tx pgx.Tx) context.Context {
	if tx == nil {
		return ctx
	}
	return context.WithValue(ctx, txContextKey{}, tx)
}

func txFromContext(ctx context.Context) pgx.Tx {
	if ctx == nil {
		return nil
	}
	tx, _ := ctx.Value(txContextKey{}).(pgx.Tx)
	return tx
}

// withRLSQueryRunner executes fn with RLS (Row-Level Security) support based on the authenticated user's claims from the context. If a transaction is already present in ctx, it is reused. If no claims are found, the function runs directly against the pool. Otherwise, a new transaction is created, configured with RLS context, and committed after the function completes.
func withRLSQueryRunner(ctx context.Context, pool *pgxpool.Pool, fn func(q queryRunner) (interface{}, error)) (interface{}, error) {
	if tx := txFromContext(ctx); tx != nil {
		return fn(tx)
	}

	if pool == nil {
		return nil, fmt.Errorf("database pool is nil")
	}

	claims := auth.ClaimsFromContext(ctx)
	if claims == nil {
		return fn(pool)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if err := auth.SetRLSContext(ctx, tx, claims); err != nil {
		return nil, fmt.Errorf("set RLS context: %w", err)
	}

	result, err := fn(tx)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}
	return result, nil
}

func queryAndScanRows(ctx context.Context, q queryRunner, sql string, args ...any) ([]map[string]any, int64, error) {
	rows, err := q.Query(ctx, sql, args...)
	if err != nil {
		return nil, 0, err
	}

	result, scanErr := scanRows(rows)
	affected := rows.CommandTag().RowsAffected()
	rows.Close()
	if scanErr != nil {
		return nil, 0, scanErr
	}
	return result, affected, nil
}

// resolveWhere recursively walks a WhereInput argument map and produces
// a parameterized SQL WHERE clause fragment. paramIdx is the starting $N index.
// Returns the SQL fragment, accumulated args, and any error.
func resolveWhere(args map[string]interface{}, tbl *schema.Table, paramIdx int) (string, []any, error) {
	if len(args) == 0 {
		return "", nil, nil
	}

	var parts []string
	var allArgs []any
	idx := paramIdx

	// Process keys in sorted order for deterministic output
	keys := make([]string, 0, len(args))
	for k := range args {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, key := range keys {
		val := args[key]

		switch key {
		case "_and":
			list, ok := val.([]interface{})
			if !ok {
				return "", nil, fmt.Errorf("_and must be a list")
			}
			var andParts []string
			for _, item := range list {
				sub, ok := item.(map[string]interface{})
				if !ok {
					return "", nil, fmt.Errorf("_and items must be objects")
				}
				sql, subArgs, err := resolveWhere(sub, tbl, idx)
				if err != nil {
					return "", nil, err
				}
				if sql != "" {
					andParts = append(andParts, sql)
					allArgs = append(allArgs, subArgs...)
					idx += len(subArgs)
				}
			}
			if len(andParts) > 0 {
				parts = append(parts, "("+strings.Join(andParts, " AND ")+")")
			}

		case "_or":
			list, ok := val.([]interface{})
			if !ok {
				return "", nil, fmt.Errorf("_or must be a list")
			}
			var orParts []string
			for _, item := range list {
				sub, ok := item.(map[string]interface{})
				if !ok {
					return "", nil, fmt.Errorf("_or items must be objects")
				}
				sql, subArgs, err := resolveWhere(sub, tbl, idx)
				if err != nil {
					return "", nil, err
				}
				if sql != "" {
					orParts = append(orParts, sql)
					allArgs = append(allArgs, subArgs...)
					idx += len(subArgs)
				}
			}
			if len(orParts) > 0 {
				parts = append(parts, "("+strings.Join(orParts, " OR ")+")")
			}

		case "_not":
			sub, ok := val.(map[string]interface{})
			if !ok {
				return "", nil, fmt.Errorf("_not must be an object")
			}
			sql, subArgs, err := resolveWhere(sub, tbl, idx)
			if err != nil {
				return "", nil, err
			}
			if sql != "" {
				parts = append(parts, "NOT ("+sql+")")
				allArgs = append(allArgs, subArgs...)
				idx += len(subArgs)
			}

		default:
			// Column-level comparison
			col := tbl.ColumnByName(key)
			if col == nil {
				return "", nil, fmt.Errorf("unknown column: %s", key)
			}

			ops, ok := val.(map[string]interface{})
			if !ok {
				return "", nil, fmt.Errorf("column %s filter must be an object", key)
			}

			// Sort operator keys for deterministic output
			opKeys := make([]string, 0, len(ops))
			for k := range ops {
				opKeys = append(opKeys, k)
			}
			sort.Strings(opKeys)

			for _, opKey := range opKeys {
				opVal := ops[opKey]

				if opKey == "_is_null" {
					boolVal, ok := opVal.(bool)
					if !ok {
						return "", nil, fmt.Errorf("_is_null must be boolean")
					}
					if boolVal {
						parts = append(parts, sqlutil.QuoteIdent(key)+" IS NULL")
					} else {
						parts = append(parts, sqlutil.QuoteIdent(key)+" IS NOT NULL")
					}
					continue
				}

				if opKey == "_in" {
					list, ok := opVal.([]interface{})
					if !ok {
						return "", nil, fmt.Errorf("_in must be a list")
					}
					placeholders := make([]string, len(list))
					for i, v := range list {
						placeholders[i] = fmt.Sprintf("$%d", idx)
						allArgs = append(allArgs, v)
						idx++
					}
					parts = append(parts, sqlutil.QuoteIdent(key)+" IN ("+strings.Join(placeholders, ", ")+")")
					continue
				}

				sqlOp, ok := operatorSQL[opKey]
				if !ok {
					return "", nil, fmt.Errorf("unknown operator: %s", opKey)
				}
				parts = append(parts, fmt.Sprintf("%s %s $%d", sqlutil.QuoteIdent(key), sqlOp, idx))
				allArgs = append(allArgs, opVal)
				idx++
			}
		}
	}

	return strings.Join(parts, " AND "), allArgs, nil
}

// resolveOrderBy walks an OrderByInput argument map and produces an ORDER BY clause.
func resolveOrderBy(args map[string]interface{}, tbl *schema.Table) (string, error) {
	if len(args) == 0 {
		return "", nil
	}

	// Sort keys for deterministic output
	keys := make([]string, 0, len(args))
	for k := range args {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var parts []string
	for _, key := range keys {
		col := tbl.ColumnByName(key)
		if col == nil {
			return "", fmt.Errorf("unknown column: %s", key)
		}

		dir, ok := args[key].(string)
		if !ok {
			return "", fmt.Errorf("order_by value for %s must be ASC or DESC", key)
		}
		dirUpper := strings.ToUpper(dir)
		if dirUpper != "ASC" && dirUpper != "DESC" {
			return "", fmt.Errorf("order_by value for %s must be ASC or DESC, got %s", key, dir)
		}
		parts = append(parts, sqlutil.QuoteIdent(key)+" "+dirUpper)
	}

	return strings.Join(parts, ", "), nil
}

// buildSelectQuery constructs a SELECT query from table metadata and GraphQL arguments.
// Exported for testing. Returns the SQL string, args, and any error.
func buildSelectQuery(tbl *schema.Table, where map[string]interface{}, orderBy map[string]interface{}, limit, offset int) (string, []any, error) {
	return buildSelectQueryWithSpatial(tbl, where, nil, orderBy, limit, offset)
}

// TODO: Document buildSelectQueryWithSpatial.
func buildSelectQueryWithSpatial(
	tbl *schema.Table,
	where map[string]interface{},
	spatialFilters []spatial.Filter,
	orderBy map[string]interface{},
	limit, offset int,
) (string, []any, error) {
	var b strings.Builder
	var allArgs []any
	paramIdx := 1

	b.WriteString("SELECT ")
	b.WriteString(buildGraphQLProjection(tbl))
	b.WriteString(" FROM ")
	b.WriteString(sqlutil.QuoteQualifiedName(tbl.Schema, tbl.Name))

	whereParts := make([]string, 0, 1+len(spatialFilters))
	if len(where) > 0 {
		whereSQL, whereArgs, err := resolveWhere(where, tbl, paramIdx)
		if err != nil {
			return "", nil, err
		}
		if whereSQL != "" {
			whereParts = append(whereParts, whereSQL)
			allArgs = append(allArgs, whereArgs...)
			paramIdx += len(whereArgs)
		}
	}

	for _, filter := range spatialFilters {
		if filter == nil {
			continue
		}
		filterSQL, filterArgs, err := filter.WhereClause(paramIdx)
		if err != nil {
			return "", nil, err
		}
		if filterSQL == "" {
			continue
		}
		whereParts = append(whereParts, filterSQL)
		allArgs = append(allArgs, filterArgs...)
		paramIdx += len(filterArgs)
	}

	if len(whereParts) > 0 {
		b.WriteString(" WHERE ")
		b.WriteString(strings.Join(whereParts, " AND "))
	}

	if len(orderBy) > 0 {
		orderSQL, err := resolveOrderBy(orderBy, tbl)
		if err != nil {
			return "", nil, err
		}
		if orderSQL != "" {
			b.WriteString(" ORDER BY ")
			b.WriteString(orderSQL)
		}
	}

	// Apply limit — cap at DefaultMaxLimit
	effectiveLimit := DefaultMaxLimit
	if limit > 0 && limit < DefaultMaxLimit {
		effectiveLimit = limit
	}
	b.WriteString(fmt.Sprintf(" LIMIT $%d", paramIdx))
	allArgs = append(allArgs, effectiveLimit)
	paramIdx++

	if offset > 0 {
		b.WriteString(fmt.Sprintf(" OFFSET $%d", paramIdx))
		allArgs = append(allArgs, offset)
	}

	return b.String(), allArgs, nil
}

// TODO: Document buildGraphQLProjection.
func buildGraphQLProjection(tbl *schema.Table) string {
	if tbl == nil || !tbl.HasGeometry() {
		return "*"
	}

	selectExprs := make([]string, 0, len(tbl.Columns))
	for _, col := range tbl.Columns {
		if col.IsGeometry || col.IsGeography {
			selectExprs = append(selectExprs, fmt.Sprintf("ST_AsGeoJSON(%s)::jsonb AS %s", sqlutil.QuoteIdent(col.Name), sqlutil.QuoteIdent(col.Name)))
			continue
		}
		selectExprs = append(selectExprs, sqlutil.QuoteIdent(col.Name))
	}
	if len(selectExprs) == 0 {
		return "*"
	}
	return strings.Join(selectExprs, ", ")
}

// scanRows scans pgx rows into a slice of maps.
func scanRows(rows pgx.Rows) ([]map[string]any, error) {
	var result []map[string]any
	descs := rows.FieldDescriptions()
	for rows.Next() {
		values := make([]any, len(descs))
		ptrs := make([]any, len(descs))
		for i := range values {
			ptrs[i] = &values[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		record := make(map[string]any, len(descs))
		for i, desc := range descs {
			record[desc.Name] = normalizeValue(values[i])
		}
		result = append(result, record)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if result == nil {
		result = []map[string]any{}
	}
	return result, nil
}

// normalizeValue converts certain pgx return types to JSON-friendly Go types.
func normalizeValue(v any) any {
	switch val := v.(type) {
	case [16]byte:
		return fmt.Sprintf("%x-%x-%x-%x-%x", val[0:4], val[4:6], val[6:8], val[8:10], val[10:16])
	default:
		return v
	}
}

// resolveTable is the root-level resolver for a single table's query field.
// It builds and executes a SELECT query with RLS enforcement.
func resolveTable(ctx context.Context, tbl *schema.Table, pool *pgxpool.Pool, cache *schema.SchemaCache, args map[string]interface{}) (interface{}, error) {
	// Extract typed arguments
	var whereArg map[string]interface{}
	if w, ok := args["where"]; ok && w != nil {
		whereArg, _ = w.(map[string]interface{})
	}
	var orderByArg map[string]interface{}
	if o, ok := args["order_by"]; ok && o != nil {
		orderByArg, _ = o.(map[string]interface{})
	}
	var limit int
	if l, ok := args["limit"]; ok && l != nil {
		switch v := l.(type) {
		case int:
			limit = v
		case float64:
			limit = int(v)
		}
	}
	var offset int
	if o, ok := args["offset"]; ok && o != nil {
		switch v := o.(type) {
		case int:
			offset = v
		case float64:
			offset = int(v)
		}
	}

	spatialFilters, err := parseSpatialArgs(tbl, cache, args)
	if err != nil {
		return nil, err
	}

	sql, sqlArgs, err := buildSelectQueryWithSpatial(tbl, whereArg, spatialFilters, orderByArg, limit, offset)
	if err != nil {
		return nil, err
	}

	result, err := withRLSQueryRunner(ctx, pool, func(q queryRunner) (interface{}, error) {
		records, _, queryErr := queryAndScanRows(ctx, q, sql, sqlArgs...)
		if queryErr != nil {
			return nil, fmt.Errorf("query: %w", queryErr)
		}
		if dl := dataloaderFromCtx(ctx); dl != nil {
			primeRowsForTableRelationships(dl, tbl, records)
		}
		return records, nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}
