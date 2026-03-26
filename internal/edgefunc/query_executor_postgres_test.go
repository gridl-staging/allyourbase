package edgefunc

import (
	"context"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestBuildQuerySQL_SelectWithFilters(t *testing.T) {
	query := Query{
		Table:   "users",
		Action:  "select",
		Columns: "id, name",
		Filters: []Filter{
			{Column: "age", Op: "gte", Value: 21},
			{Column: "name", Op: "neq", Value: "bob"},
		},
	}

	sql, args, err := buildQuerySQL(query)
	testutil.NoError(t, err)
	testutil.Equal(t, `SELECT "id", "name" FROM "users" WHERE "age" >= $1 AND "name" != $2`, sql)
	testutil.Equal(t, 2, len(args))
	testutil.Equal(t, 21, args[0].(int))
	testutil.Equal(t, "bob", args[1].(string))
}

func TestBuildQuerySQL_InsertSortsColumnsForStableSQL(t *testing.T) {
	query := Query{
		Table:  "users",
		Action: "insert",
		Data: map[string]interface{}{
			"name": "alice",
			"age":  30,
		},
	}

	sql, args, err := buildQuerySQL(query)
	testutil.NoError(t, err)
	testutil.Equal(t, `INSERT INTO "users" ("age", "name") VALUES ($1, $2) RETURNING *`, sql)
	testutil.Equal(t, 2, len(args))
	testutil.Equal(t, 30, args[0].(int))
	testutil.Equal(t, "alice", args[1].(string))
}

func TestBuildQuerySQL_UpdateRequiresFilter(t *testing.T) {
	_, _, err := buildQuerySQL(Query{
		Table:  "users",
		Action: "update",
		Data: map[string]interface{}{
			"name": "alice",
		},
	})
	testutil.ErrorContains(t, err, "at least one filter")
}

func TestBuildQuerySQL_DeleteRequiresFilter(t *testing.T) {
	_, _, err := buildQuerySQL(Query{
		Table:  "users",
		Action: "delete",
	})
	testutil.ErrorContains(t, err, "at least one filter")
}

func TestBuildQuerySQL_UnsupportedFilterOperator(t *testing.T) {
	_, _, err := buildQuerySQL(Query{
		Table:  "users",
		Action: "select",
		Filters: []Filter{
			{Column: "age", Op: "contains", Value: 10},
		},
	})
	testutil.ErrorContains(t, err, "unsupported filter operator")
}

func TestBuildQuerySQL_UnsupportedAction(t *testing.T) {
	_, _, err := buildQuerySQL(Query{
		Table:  "users",
		Action: "merge",
	})
	testutil.ErrorContains(t, err, "unsupported action")
}

func TestBuildQuerySQL_NullFiltersUseSQLNullSemantics(t *testing.T) {
	sql, args, err := buildQuerySQL(Query{
		Table:   "users",
		Action:  "select",
		Columns: "id",
		Filters: []Filter{
			{Column: "deleted_at", Op: "eq", Value: nil},
			{Column: "age", Op: "gte", Value: 21},
			{Column: "email_verified_at", Op: "neq", Value: nil},
		},
	})

	testutil.NoError(t, err)
	testutil.Equal(
		t,
		`SELECT "id" FROM "users" WHERE "deleted_at" IS NULL AND "age" >= $1 AND "email_verified_at" IS NOT NULL`,
		sql,
	)
	testutil.SliceLen(t, args, 1)
	testutil.Equal(t, 21, args[0])
}

func TestBuildQuerySQL_NullFilterRejectsNonNullOperator(t *testing.T) {
	_, _, err := buildQuerySQL(Query{
		Table:   "users",
		Action:  "select",
		Columns: "id",
		Filters: []Filter{
			{Column: "deleted_at", Op: "gt", Value: nil},
		},
	})
	testutil.ErrorContains(t, err, "nil filter value only supports eq/neq")
}

func TestBuildQuerySQL_UpdateHappyPath(t *testing.T) {
	sql, args, err := buildQuerySQL(Query{
		Table:  "users",
		Action: "update",
		Data: map[string]interface{}{
			"name": "alice",
			"age":  31,
		},
		Filters: []Filter{
			{Column: "id", Op: "eq", Value: 1},
		},
	})
	testutil.NoError(t, err)
	testutil.Equal(t, `UPDATE "users" SET "age" = $1, "name" = $2 WHERE "id" = $3 RETURNING *`, sql)
	testutil.SliceLen(t, args, 3)
	testutil.Equal(t, 31, args[0].(int))
	testutil.Equal(t, "alice", args[1].(string))
	testutil.Equal(t, 1, args[2].(int))
}

func TestBuildQuerySQL_UpdateEmptyData(t *testing.T) {
	_, _, err := buildQuerySQL(Query{
		Table:  "users",
		Action: "update",
		Data:   map[string]interface{}{},
		Filters: []Filter{
			{Column: "id", Op: "eq", Value: 1},
		},
	})
	testutil.ErrorContains(t, err, "update requires data")
}

func TestBuildQuerySQL_DeleteHappyPath(t *testing.T) {
	sql, args, err := buildQuerySQL(Query{
		Table:  "users",
		Action: "delete",
		Filters: []Filter{
			{Column: "id", Op: "eq", Value: 42},
		},
	})
	testutil.NoError(t, err)
	testutil.Equal(t, `DELETE FROM "users" WHERE "id" = $1 RETURNING *`, sql)
	testutil.SliceLen(t, args, 1)
	testutil.Equal(t, 42, args[0].(int))
}

func TestPostgresQueryExecutorQueryRawRejectsEmptySQL(t *testing.T) {
	t.Parallel()

	executor := NewPostgresQueryExecutor(nil, nil)
	_, err := executor.QueryRaw(context.Background(), "")
	testutil.ErrorContains(t, err, "sql is required")
}

func TestPostgresQueryExecutorQueryRawRejectsDisallowedTableBeforeExecution(t *testing.T) {
	t.Parallel()

	executor := NewPostgresQueryExecutor(nil, []string{"allowed_table"})
	_, err := executor.QueryRaw(context.Background(), `SELECT "id" FROM "blocked_table"`)
	testutil.ErrorContains(t, err, "not allowed")
}

func TestPostgresQueryExecutorQueryRawAllowsSchemaQualifiedTable(t *testing.T) {
	t.Parallel()

	executor := NewPostgresQueryExecutor(nil, []string{"allowed_table"})
	_, err := executor.QueryRaw(context.Background(), `SELECT "id" FROM "public"."allowed_table"`)
	testutil.ErrorContains(t, err, "requires a pool")
}

func TestPostgresQueryExecutorQueryRawAllowsQuotedIdentifierTable(t *testing.T) {
	t.Parallel()

	executor := NewPostgresQueryExecutor(nil, []string{`odd"name`})
	_, err := executor.QueryRaw(context.Background(), `SELECT "id" FROM "public"."odd""name"`)
	testutil.ErrorContains(t, err, "requires a pool")
}

func TestPostgresQueryExecutorQueryRawRejectsCrossSchemaMatchForUnqualifiedAllowlist(t *testing.T) {
	t.Parallel()

	executor := NewPostgresQueryExecutor(nil, []string{"allowed_table"})
	_, err := executor.QueryRaw(context.Background(), `SELECT "id" FROM "private"."allowed_table"`)
	testutil.ErrorContains(t, err, "not allowed")
}

func TestPostgresQueryExecutorQueryRawRejectsDisallowedJoinedTableBeforeExecution(t *testing.T) {
	t.Parallel()

	executor := NewPostgresQueryExecutor(nil, []string{"allowed_table"})
	_, err := executor.QueryRaw(
		context.Background(),
		`SELECT a."id" FROM "allowed_table" a JOIN "blocked_table" b ON b."id" = a."id"`,
	)
	testutil.ErrorContains(t, err, `"blocked_table" is not allowed`)
}

func TestPostgresQueryExecutorQueryRawRejectsDisallowedSubqueryTableBeforeExecution(t *testing.T) {
	t.Parallel()

	executor := NewPostgresQueryExecutor(nil, []string{"allowed_table"})
	_, err := executor.QueryRaw(
		context.Background(),
		`SELECT "id" FROM "allowed_table" WHERE EXISTS (SELECT 1 FROM "blocked_table")`,
	)
	testutil.ErrorContains(t, err, `"blocked_table" is not allowed`)
}

func TestPostgresQueryExecutorQueryRawRejectsMultipleStatementsWhenAllowlistEnabled(t *testing.T) {
	t.Parallel()

	executor := NewPostgresQueryExecutor(nil, []string{"allowed_table"})
	_, err := executor.QueryRaw(
		context.Background(),
		`SELECT "id" FROM "allowed_table"; SELECT "id" FROM "blocked_table"`,
	)
	testutil.ErrorContains(t, err, "multiple statements")
}

func TestPostgresQueryExecutorQueryRawIgnoresKeywordsInsideStringLiterals(t *testing.T) {
	t.Parallel()

	executor := NewPostgresQueryExecutor(nil, []string{"allowed_table"})
	_, err := executor.QueryRaw(context.Background(), `SELECT 'from allowed_table' AS note FROM "blocked_table"`)
	testutil.ErrorContains(t, err, `"blocked_table" is not allowed`)
}

func TestPostgresQueryExecutorQueryRawIgnoresKeywordsInsideNestedBlockComments(t *testing.T) {
	t.Parallel()

	executor := NewPostgresQueryExecutor(nil, []string{"allowed_table"})
	_, err := executor.QueryRaw(
		context.Background(),
		`SELECT 1 /* outer /* nested */ FROM "allowed_table" */ FROM "blocked_table"`,
	)
	testutil.ErrorContains(t, err, `"blocked_table" is not allowed`)
}
