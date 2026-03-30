package api

import (
	"context"
	"testing"

	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type queryBatchRecordQuerier struct {
	rows  pgx.Rows
	qErr  error
	query string
	args  []any
}

func (q *queryBatchRecordQuerier) Query(_ context.Context, query string, args ...any) (pgx.Rows, error) {
	q.query = query
	q.args = args
	if q.qErr != nil {
		return nil, q.qErr
	}
	return q.rows, nil
}

func (q *queryBatchRecordQuerier) QueryRow(context.Context, string, ...any) pgx.Row {
	panic("QueryRow not used")
}

func (q *queryBatchRecordQuerier) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	panic("Exec not used")
}

type closeTrackingRows struct {
	fakeBatchRows
	closed bool
}

func (r *closeTrackingRows) Close() {
	r.closed = true
}

func TestQueryBatchRecordClosesRowsAndScansRecord(t *testing.T) {
	t.Parallel()

	rows := &closeTrackingRows{
		fakeBatchRows: fakeBatchRows{
			cols: []string{"id", "email"},
			rows: [][]any{{"u1", "u1@example.com"}},
		},
	}
	q := &queryBatchRecordQuerier{rows: rows}

	record, err := queryBatchRecord(context.Background(), q, "SELECT * FROM users WHERE id = $1", "u1")
	testutil.NoError(t, err)
	testutil.Equal(t, true, rows.closed)
	testutil.Equal(t, "u1", record["id"])
	testutil.Equal(t, "u1@example.com", record["email"])
	testutil.Equal(t, "SELECT * FROM users WHERE id = $1", q.query)
	testutil.Equal(t, 1, len(q.args))
	testutil.Equal(t, "u1", q.args[0])
}

func TestQueryBatchRecordReturnsNilAndClosesRowsWhenNoRows(t *testing.T) {
	t.Parallel()

	rows := &closeTrackingRows{
		fakeBatchRows: fakeBatchRows{
			cols: []string{"id"},
			rows: [][]any{},
		},
	}
	q := &queryBatchRecordQuerier{rows: rows}

	record, err := queryBatchRecord(context.Background(), q, "SELECT * FROM users WHERE id = $1", "missing")
	testutil.NoError(t, err)
	testutil.Equal(t, true, rows.closed)
	testutil.True(t, record == nil)
}

func TestBuildBatchDeleteEventRecordUsesPrimaryKeyOrder(t *testing.T) {
	t.Parallel()

	tbl := &schema.Table{
		Schema:     "public",
		Name:       "membership",
		Kind:       "table",
		PrimaryKey: []string{"org_id", "user_id"},
	}

	record := buildBatchDeleteEventRecord(tbl, []string{"o-1", "u-2"})
	testutil.Equal(t, 2, len(record))
	testutil.Equal(t, "o-1", record["org_id"])
	testutil.Equal(t, "u-2", record["user_id"])
}
