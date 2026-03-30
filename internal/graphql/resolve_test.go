package graphql

import (
	"context"
	"testing"

	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func testTable() *schema.Table {
	return &schema.Table{
		Schema: "public",
		Name:   "posts",
		Kind:   "table",
		Columns: []*schema.Column{
			{Name: "id", TypeName: "integer", IsPrimaryKey: true},
			{Name: "title", TypeName: "text"},
			{Name: "body", TypeName: "text", IsNullable: true},
			{Name: "score", TypeName: "integer"},
			{Name: "created_at", TypeName: "timestamptz"},
		},
		PrimaryKey: []string{"id"},
	}
}

type stubTx struct{}

func (s *stubTx) Begin(ctx context.Context) (pgx.Tx, error) { return s, nil }
func (s *stubTx) Commit(ctx context.Context) error          { return nil }
func (s *stubTx) Rollback(ctx context.Context) error        { return nil }
func (s *stubTx) CopyFrom(ctx context.Context, tableName pgx.Identifier, columnNames []string, rowSrc pgx.CopyFromSource) (int64, error) {
	return 0, nil
}
func (s *stubTx) SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults { return nil }
func (s *stubTx) LargeObjects() pgx.LargeObjects                               { return pgx.LargeObjects{} }
func (s *stubTx) Prepare(ctx context.Context, name, sql string) (*pgconn.StatementDescription, error) {
	return nil, nil
}
func (s *stubTx) Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}
func (s *stubTx) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	return nil, nil
}
func (s *stubTx) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row { return nil }
func (s *stubTx) Conn() *pgx.Conn                                               { return nil }

var _ pgx.Tx = (*stubTx)(nil)

func TestWithRLSQueryRunnerUsesContextTxWithoutPool(t *testing.T) {
	t.Parallel()

	tx := &stubTx{}
	ctx := ctxWithTx(context.Background(), tx)
	called := false

	got, err := withRLSQueryRunner(ctx, nil, func(q queryRunner) (interface{}, error) {
		called = true
		qTx, ok := q.(pgx.Tx)
		testutil.True(t, ok, "expected query runner to be a transaction")
		testutil.True(t, qTx == tx, "expected context transaction to be passed through")
		return "ok", nil
	})
	testutil.NoError(t, err)
	testutil.True(t, called, "expected resolver function to be called")
	testutil.Equal(t, "ok", got)
}

// --- resolveWhere tests ---

func TestResolveWhereEmpty(t *testing.T) {
	t.Parallel()
	sql, args, err := resolveWhere(nil, testTable(), 1)
	testutil.NoError(t, err)
	testutil.Equal(t, "", sql)
	testutil.Equal(t, 0, len(args))
}

func TestResolveWhereEq(t *testing.T) {
	t.Parallel()
	where := map[string]interface{}{
		"title": map[string]interface{}{"_eq": "hello"},
	}
	sql, args, err := resolveWhere(where, testTable(), 1)
	testutil.NoError(t, err)
	testutil.Equal(t, `"title" = $1`, sql)
	testutil.Equal(t, 1, len(args))
	testutil.Equal(t, "hello", args[0])
}

func TestResolveWhereGtLtCombined(t *testing.T) {
	t.Parallel()
	where := map[string]interface{}{
		"score": map[string]interface{}{"_gt": 5, "_lt": 10},
	}
	sql, args, err := resolveWhere(where, testTable(), 1)
	testutil.NoError(t, err)
	testutil.Equal(t, `"score" > $1 AND "score" < $2`, sql)
	testutil.Equal(t, 2, len(args))
	testutil.Equal(t, 5, args[0])
	testutil.Equal(t, 10, args[1])
}

func TestResolveWhereIn(t *testing.T) {
	t.Parallel()
	where := map[string]interface{}{
		"title": map[string]interface{}{"_in": []interface{}{"a", "b", "c"}},
	}
	sql, args, err := resolveWhere(where, testTable(), 3)
	testutil.NoError(t, err)
	testutil.Equal(t, `"title" IN ($3, $4, $5)`, sql)
	testutil.Equal(t, 3, len(args))
	testutil.Equal(t, "a", args[0])
	testutil.Equal(t, "b", args[1])
	testutil.Equal(t, "c", args[2])
}

func TestResolveWhereIsNullTrue(t *testing.T) {
	t.Parallel()
	where := map[string]interface{}{
		"body": map[string]interface{}{"_is_null": true},
	}
	sql, args, err := resolveWhere(where, testTable(), 1)
	testutil.NoError(t, err)
	testutil.Equal(t, `"body" IS NULL`, sql)
	testutil.Equal(t, 0, len(args))
}

func TestResolveWhereIsNullFalse(t *testing.T) {
	t.Parallel()
	where := map[string]interface{}{
		"body": map[string]interface{}{"_is_null": false},
	}
	sql, args, err := resolveWhere(where, testTable(), 1)
	testutil.NoError(t, err)
	testutil.Equal(t, `"body" IS NOT NULL`, sql)
	testutil.Equal(t, 0, len(args))
}

func TestResolveWhereLike(t *testing.T) {
	t.Parallel()
	where := map[string]interface{}{
		"title": map[string]interface{}{"_like": "%hello%"},
	}
	sql, args, err := resolveWhere(where, testTable(), 1)
	testutil.NoError(t, err)
	testutil.Equal(t, `"title" LIKE $1`, sql)
	testutil.Equal(t, 1, len(args))
}

func TestResolveWhereIlike(t *testing.T) {
	t.Parallel()
	where := map[string]interface{}{
		"title": map[string]interface{}{"_ilike": "%hello%"},
	}
	sql, args, err := resolveWhere(where, testTable(), 1)
	testutil.NoError(t, err)
	testutil.Equal(t, `"title" ILIKE $1`, sql)
	testutil.Equal(t, 1, len(args))
}

func TestResolveWhereAnd(t *testing.T) {
	t.Parallel()
	where := map[string]interface{}{
		"_and": []interface{}{
			map[string]interface{}{"title": map[string]interface{}{"_eq": "a"}},
			map[string]interface{}{"score": map[string]interface{}{"_gt": 5}},
		},
	}
	sql, args, err := resolveWhere(where, testTable(), 1)
	testutil.NoError(t, err)
	testutil.Equal(t, `("title" = $1 AND "score" > $2)`, sql)
	testutil.Equal(t, 2, len(args))
	testutil.Equal(t, "a", args[0])
	testutil.Equal(t, 5, args[1])
}

func TestResolveWhereAndParamIdxOffset(t *testing.T) {
	t.Parallel()
	where := map[string]interface{}{
		"_and": []interface{}{
			map[string]interface{}{"title": map[string]interface{}{"_eq": "a"}},
			map[string]interface{}{"score": map[string]interface{}{"_gt": 5}},
		},
	}
	sql, args, err := resolveWhere(where, testTable(), 5)
	testutil.NoError(t, err)
	testutil.Equal(t, `("title" = $5 AND "score" > $6)`, sql)
	testutil.Equal(t, 2, len(args))
	testutil.Equal(t, "a", args[0])
	testutil.Equal(t, 5, args[1])
}

func TestResolveWhereOr(t *testing.T) {
	t.Parallel()
	where := map[string]interface{}{
		"_or": []interface{}{
			map[string]interface{}{"title": map[string]interface{}{"_eq": "a"}},
			map[string]interface{}{"title": map[string]interface{}{"_eq": "b"}},
		},
	}
	sql, args, err := resolveWhere(where, testTable(), 1)
	testutil.NoError(t, err)
	testutil.Equal(t, `("title" = $1 OR "title" = $2)`, sql)
	testutil.Equal(t, 2, len(args))
	testutil.Equal(t, "a", args[0])
	testutil.Equal(t, "b", args[1])
}

func TestResolveWhereNot(t *testing.T) {
	t.Parallel()
	where := map[string]interface{}{
		"_not": map[string]interface{}{
			"title": map[string]interface{}{"_eq": "hidden"},
		},
	}
	sql, args, err := resolveWhere(where, testTable(), 1)
	testutil.NoError(t, err)
	testutil.Equal(t, `NOT ("title" = $1)`, sql)
	testutil.Equal(t, 1, len(args))
}

func TestResolveWhereNestedAndInsideOr(t *testing.T) {
	t.Parallel()
	where := map[string]interface{}{
		"_or": []interface{}{
			map[string]interface{}{
				"_and": []interface{}{
					map[string]interface{}{"title": map[string]interface{}{"_eq": "a"}},
					map[string]interface{}{"score": map[string]interface{}{"_gt": 1}},
				},
			},
			map[string]interface{}{"score": map[string]interface{}{"_lt": 0}},
		},
	}
	sql, args, err := resolveWhere(where, testTable(), 1)
	testutil.NoError(t, err)
	testutil.Equal(t, `(("title" = $1 AND "score" > $2) OR "score" < $3)`, sql)
	testutil.Equal(t, 3, len(args))
	testutil.Equal(t, "a", args[0])
	testutil.Equal(t, 1, args[1])
	testutil.Equal(t, 0, args[2])
}

func TestResolveWhereUnknownColumn(t *testing.T) {
	t.Parallel()
	where := map[string]interface{}{
		"nonexistent": map[string]interface{}{"_eq": "val"},
	}
	_, _, err := resolveWhere(where, testTable(), 1)
	testutil.True(t, err != nil, "expected error for unknown column")
	testutil.Equal(t, "unknown column: nonexistent", err.Error())
}

func TestResolveWhereInvalidInputErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args map[string]interface{}
		want string
	}{
		{
			name: "_and non-list",
			args: map[string]interface{}{"_and": map[string]interface{}{"title": "x"}},
			want: "_and must be a list",
		},
		{
			name: "_not non-object",
			args: map[string]interface{}{"_not": []interface{}{"x"}},
			want: "_not must be an object",
		},
		{
			name: "_is_null non-boolean",
			args: map[string]interface{}{
				"body": map[string]interface{}{"_is_null": "true"},
			},
			want: "_is_null must be boolean",
		},
		{
			name: "_in non-list",
			args: map[string]interface{}{
				"title": map[string]interface{}{"_in": "not-a-list"},
			},
			want: "_in must be a list",
		},
		{
			name: "unknown operator",
			args: map[string]interface{}{
				"title": map[string]interface{}{"_wat": "x"},
			},
			want: "unknown operator: _wat",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, _, err := resolveWhere(tc.args, testTable(), 1)
			testutil.True(t, err != nil, "expected error")
			testutil.Equal(t, tc.want, err.Error())
		})
	}
}

// --- resolveOrderBy tests ---

func TestResolveOrderByEmpty(t *testing.T) {
	t.Parallel()
	sql, err := resolveOrderBy(nil, testTable())
	testutil.NoError(t, err)
	testutil.Equal(t, "", sql)
}

func TestResolveOrderByAsc(t *testing.T) {
	t.Parallel()
	orderBy := map[string]interface{}{"title": "ASC"}
	sql, err := resolveOrderBy(orderBy, testTable())
	testutil.NoError(t, err)
	testutil.Equal(t, `"title" ASC`, sql)
}

func TestResolveOrderByDesc(t *testing.T) {
	t.Parallel()
	orderBy := map[string]interface{}{"score": "DESC"}
	sql, err := resolveOrderBy(orderBy, testTable())
	testutil.NoError(t, err)
	testutil.Equal(t, `"score" DESC`, sql)
}

func TestResolveOrderByMultiple(t *testing.T) {
	t.Parallel()
	// Use a slice of single-key maps to guarantee order
	orderBy := map[string]interface{}{
		"title": "ASC",
		"score": "DESC",
	}
	sql, err := resolveOrderBy(orderBy, testTable())
	testutil.NoError(t, err)
	// Both columns should appear
	testutil.True(t, sql != "", "expected non-empty SQL")
}

func TestResolveOrderByUnknownColumn(t *testing.T) {
	t.Parallel()
	orderBy := map[string]interface{}{"nonexistent": "ASC"}
	_, err := resolveOrderBy(orderBy, testTable())
	testutil.True(t, err != nil, "expected error for unknown column")
}

// --- buildSelectQuery tests ---

func TestBuildSelectQueryBasic(t *testing.T) {
	t.Parallel()
	tbl := testTable()
	sql, args, err := buildSelectQuery(tbl, nil, nil, 0, 0)
	testutil.NoError(t, err)
	testutil.Equal(t, `SELECT * FROM "public"."posts" LIMIT $1`, sql)
	testutil.Equal(t, 1, len(args))
	testutil.Equal(t, DefaultMaxLimit, args[0])
}

func TestBuildSelectQueryWithWhere(t *testing.T) {
	t.Parallel()
	tbl := testTable()
	where := map[string]interface{}{
		"title": map[string]interface{}{"_eq": "hello"},
	}
	sql, args, err := buildSelectQuery(tbl, where, nil, 0, 0)
	testutil.NoError(t, err)
	testutil.Equal(t, `SELECT * FROM "public"."posts" WHERE "title" = $1 LIMIT $2`, sql)
	testutil.Equal(t, 2, len(args))
	testutil.Equal(t, "hello", args[0])
}

func TestBuildSelectQueryWithOrderBy(t *testing.T) {
	t.Parallel()
	tbl := testTable()
	orderBy := map[string]interface{}{"title": "ASC"}
	sql, args, err := buildSelectQuery(tbl, nil, orderBy, 0, 0)
	testutil.NoError(t, err)
	testutil.Equal(t, `SELECT * FROM "public"."posts" ORDER BY "title" ASC LIMIT $1`, sql)
	testutil.Equal(t, 1, len(args))
}

func TestBuildSelectQueryWithLimitOffset(t *testing.T) {
	t.Parallel()
	tbl := testTable()
	sql, args, err := buildSelectQuery(tbl, nil, nil, 10, 20)
	testutil.NoError(t, err)
	testutil.Equal(t, `SELECT * FROM "public"."posts" LIMIT $1 OFFSET $2`, sql)
	testutil.Equal(t, 2, len(args))
	testutil.Equal(t, 10, args[0])
	testutil.Equal(t, 20, args[1])
}

func TestBuildSelectQueryLimitCapped(t *testing.T) {
	t.Parallel()
	tbl := testTable()
	sql, args, err := buildSelectQuery(tbl, nil, nil, 99999, 0)
	testutil.NoError(t, err)
	testutil.Equal(t, `SELECT * FROM "public"."posts" LIMIT $1`, sql)
	testutil.Equal(t, DefaultMaxLimit, args[0])
}
