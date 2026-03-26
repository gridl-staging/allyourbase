//go:build integration

package edgefunc_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/edgefunc"
	"github.com/allyourbase/ayb/internal/sqlutil"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/google/uuid"
)

func TestPostgresQueryExecutor_SelectWithFilters(t *testing.T) {
	ctx := context.Background()
	table := createQueryExecutorTestTable(t)

	_, err := testPool.Exec(ctx,
		fmt.Sprintf(`INSERT INTO %s (name, age) VALUES ('alice', 30), ('bob', 20)`, sqlutil.QuoteIdent(table)),
	)
	testutil.NoError(t, err)

	qe := edgefunc.NewPostgresQueryExecutor(testPool, nil)
	result, err := qe.Execute(ctx, edgefunc.Query{
		Table:   table,
		Action:  "select",
		Columns: "id, name, age",
		Filters: []edgefunc.Filter{
			{Column: "age", Op: "gte", Value: 25},
		},
	})
	testutil.NoError(t, err)
	testutil.SliceLen(t, result.Rows, 1)
	testutil.Equal(t, "alice", result.Rows[0]["name"])
}

func TestPostgresQueryExecutor_InsertUpdateDelete(t *testing.T) {
	ctx := context.Background()
	table := createQueryExecutorTestTable(t)
	qe := edgefunc.NewPostgresQueryExecutor(testPool, nil)

	inserted, err := qe.Execute(ctx, edgefunc.Query{
		Table:  table,
		Action: "insert",
		Data: map[string]interface{}{
			"name": "charlie",
			"age":  41,
		},
	})
	testutil.NoError(t, err)
	testutil.SliceLen(t, inserted.Rows, 1)
	rowID := inserted.Rows[0]["id"]

	updated, err := qe.Execute(ctx, edgefunc.Query{
		Table:  table,
		Action: "update",
		Data: map[string]interface{}{
			"age": 42,
		},
		Filters: []edgefunc.Filter{
			{Column: "id", Op: "eq", Value: rowID},
		},
	})
	testutil.NoError(t, err)
	testutil.SliceLen(t, updated.Rows, 1)

	deleted, err := qe.Execute(ctx, edgefunc.Query{
		Table:  table,
		Action: "delete",
		Filters: []edgefunc.Filter{
			{Column: "id", Op: "eq", Value: rowID},
		},
	})
	testutil.NoError(t, err)
	testutil.SliceLen(t, deleted.Rows, 1)
	testutil.Equal(t, "charlie", deleted.Rows[0]["name"])

	selected, err := qe.Execute(ctx, edgefunc.Query{
		Table:   table,
		Action:  "select",
		Columns: "*",
		Filters: []edgefunc.Filter{
			{Column: "id", Op: "eq", Value: rowID},
		},
	})
	testutil.NoError(t, err)
	testutil.SliceLen(t, selected.Rows, 0)
}

func TestPostgresQueryExecutor_RejectsUnsafeWritesWithoutFilters(t *testing.T) {
	ctx := context.Background()
	table := createQueryExecutorTestTable(t)
	qe := edgefunc.NewPostgresQueryExecutor(testPool, nil)

	_, err := qe.Execute(ctx, edgefunc.Query{
		Table:  table,
		Action: "update",
		Data: map[string]interface{}{
			"age": 99,
		},
	})
	testutil.ErrorContains(t, err, "at least one filter")

	_, err = qe.Execute(ctx, edgefunc.Query{
		Table:  table,
		Action: "delete",
	})
	testutil.ErrorContains(t, err, "at least one filter")
}

func TestPostgresQueryExecutor_RejectsDisallowedTable(t *testing.T) {
	ctx := context.Background()
	table := createQueryExecutorTestTable(t)
	qe := edgefunc.NewPostgresQueryExecutor(testPool, []string{"allowed_only"})

	_, err := qe.Execute(ctx, edgefunc.Query{
		Table:   table,
		Action:  "select",
		Columns: "*",
	})
	testutil.ErrorContains(t, err, "not allowed")
}

func TestPostgresQueryExecutor_UnsupportedAction(t *testing.T) {
	ctx := context.Background()
	table := createQueryExecutorTestTable(t)
	qe := edgefunc.NewPostgresQueryExecutor(testPool, nil)

	_, err := qe.Execute(ctx, edgefunc.Query{
		Table:  table,
		Action: "merge",
	})
	testutil.ErrorContains(t, err, "unsupported action")
}

func TestPostgresQueryExecutor_QueryRawExecutesParameterizedSelect(t *testing.T) {
	ctx := context.Background()
	table := createQueryExecutorTestTable(t)

	_, err := testPool.Exec(ctx,
		fmt.Sprintf(`INSERT INTO %s (name, age) VALUES ('alice', 30), ('bob', 20)`, sqlutil.QuoteIdent(table)),
	)
	testutil.NoError(t, err)

	qe := edgefunc.NewPostgresQueryExecutor(testPool, []string{table})
	sql := fmt.Sprintf(`SELECT "name" FROM %s WHERE "age" >= $1 ORDER BY "name" ASC`, sqlutil.QuoteIdent(table))

	result, err := qe.QueryRaw(ctx, sql, 25)
	testutil.NoError(t, err)
	testutil.SliceLen(t, result.Rows, 1)
	testutil.Equal(t, "alice", result.Rows[0]["name"])
}

func TestPostgresQueryExecutor_QueryRawEnforcesAllowlist(t *testing.T) {
	ctx := context.Background()
	table := createQueryExecutorTestTable(t)

	qe := edgefunc.NewPostgresQueryExecutor(testPool, []string{"different_table"})
	sql := fmt.Sprintf(`SELECT "id" FROM %s`, sqlutil.QuoteIdent(table))

	_, err := qe.QueryRaw(ctx, sql)
	testutil.ErrorContains(t, err, "not allowed")
}

func createQueryExecutorTestTable(t *testing.T) string {
	t.Helper()
	ctx := context.Background()
	name := "test_qe_" + strings.ReplaceAll(uuid.New().String()[:8], "-", "")
	_, err := testPool.Exec(ctx, fmt.Sprintf(`
		CREATE TABLE %s (
			id BIGSERIAL PRIMARY KEY,
			name TEXT NOT NULL,
			age INT NOT NULL
		)
	`, sqlutil.QuoteIdent(name)))
	testutil.NoError(t, err)

	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), fmt.Sprintf(`DROP TABLE IF EXISTS %s`, sqlutil.QuoteIdent(name)))
	})
	return name
}
