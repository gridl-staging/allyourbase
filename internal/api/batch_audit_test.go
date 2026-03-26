package api

import (
	"context"
	"io"
	"log/slog"
	"net/http/httptest"
	"testing"

	"github.com/allyourbase/ayb/internal/audit"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type fakeBatchRows struct {
	cols []string
	rows [][]any
	idx  int
	err  error
}

func (r *fakeBatchRows) Close() {}

func (r *fakeBatchRows) Err() error { return r.err }

func (r *fakeBatchRows) CommandTag() pgconn.CommandTag { return pgconn.CommandTag{} }

func (r *fakeBatchRows) FieldDescriptions() []pgconn.FieldDescription {
	desc := make([]pgconn.FieldDescription, len(r.cols))
	for i, col := range r.cols {
		desc[i] = pgconn.FieldDescription{Name: col}
	}
	return desc
}

func (r *fakeBatchRows) Next() bool {
	if r.idx >= len(r.rows) {
		return false
	}
	r.idx++
	return true
}

func (r *fakeBatchRows) Scan(dest ...any) error {
	if r.idx == 0 || r.idx > len(r.rows) {
		return io.EOF
	}
	current := r.rows[r.idx-1]
	for i := range dest {
		ptr, ok := dest[i].(*any)
		if !ok {
			return io.ErrUnexpectedEOF
		}
		*ptr = current[i]
	}
	return nil
}

func (r *fakeBatchRows) Values() ([]any, error) {
	if r.idx == 0 || r.idx > len(r.rows) {
		return nil, io.EOF
	}
	return r.rows[r.idx-1], nil
}

func (r *fakeBatchRows) RawValues() [][]byte { return nil }

func (r *fakeBatchRows) Conn() *pgx.Conn { return nil }

type fakeBatchQuerier struct {
	queryRows []pgx.Rows
	queryIdx  int
	execTag   pgconn.CommandTag
	execCalls int
	lastQuery string
}

func (q *fakeBatchQuerier) Query(context.Context, string, ...any) (pgx.Rows, error) {
	if q.queryIdx >= len(q.queryRows) {
		return &fakeBatchRows{}, nil
	}
	rows := q.queryRows[q.queryIdx]
	q.queryIdx++
	return rows, nil
}

func (q *fakeBatchQuerier) QueryRow(context.Context, string, ...any) pgx.Row {
	panic("QueryRow not used")
}

func (q *fakeBatchQuerier) Exec(_ context.Context, query string, args ...any) (pgconn.CommandTag, error) {
	q.execCalls++
	q.lastQuery = query
	_ = args
	return q.execTag, nil
}

type captureBatchAuditSink struct {
	audited map[string]bool
	entries []audit.AuditEntry
	querier []audit.Execer
}

func (s *captureBatchAuditSink) ShouldAudit(tableName string) bool {
	return s.audited[tableName]
}

func (s *captureBatchAuditSink) LogMutationWithQuerier(ctx context.Context, q audit.Execer, entry audit.AuditEntry) error {
	_ = ctx
	s.entries = append(s.entries, entry)
	s.querier = append(s.querier, q)
	return nil
}

func usersTableForBatchAudit() *schema.Table {
	return &schema.Table{
		Schema:     "public",
		Name:       "users",
		Kind:       "table",
		PrimaryKey: []string{"id"},
		Columns: []*schema.Column{
			{Name: "id", TypeName: "uuid"},
			{Name: "email", TypeName: "text"},
		},
	}
}

func TestExecBatchOpCreateLogsAuditWhenEnabled(t *testing.T) {
	t.Parallel()
	tbl := usersTableForBatchAudit()
	sink := &captureBatchAuditSink{audited: map[string]bool{"users": true}}
	h := &Handler{logger: slog.Default(), auditSink: sink}

	q := &fakeBatchQuerier{queryRows: []pgx.Rows{
		&fakeBatchRows{cols: []string{"id", "email"}, rows: [][]any{{"u1", "new@example.com"}}},
	}}
	req := httptest.NewRequest("POST", "/collections/users/batch", nil)

	result, event, err := h.execBatchOp(req, q, tbl, BatchOperation{Method: "create", Body: map[string]any{"email": "new@example.com"}})
	testutil.NoError(t, err)
	testutil.Equal(t, 201, result.Status)
	testutil.NotNil(t, event)
	testutil.Equal(t, 1, len(sink.entries))
	if len(sink.entries) == 1 {
		testutil.Equal(t, "INSERT", sink.entries[0].Operation)
		testutil.Equal(t, "users", sink.entries[0].TableName)
		testutil.Equal(t, "u1", sink.entries[0].RecordID.(map[string]any)["id"])
	}
}

func TestExecBatchOpUpdateLogsAuditWhenEnabled(t *testing.T) {
	t.Parallel()
	tbl := usersTableForBatchAudit()
	sink := &captureBatchAuditSink{audited: map[string]bool{"users": true}}
	h := &Handler{logger: slog.Default(), auditSink: sink}

	oldJSON := []byte(`{"id":"u1","email":"old@example.com"}`)
	q := &fakeBatchQuerier{queryRows: []pgx.Rows{
		&fakeBatchRows{cols: []string{"id", "email", "_audit_old_values"}, rows: [][]any{{"u1", "new@example.com", oldJSON}}},
	}}
	req := httptest.NewRequest("POST", "/collections/users/batch", nil)

	result, event, err := h.execBatchOp(req, q, tbl, BatchOperation{Method: "update", ID: "u1", Body: map[string]any{"email": "new@example.com"}})
	testutil.NoError(t, err)
	testutil.Equal(t, 200, result.Status)
	testutil.NotNil(t, event)
	testutil.Equal(t, 1, len(sink.entries))
	if len(sink.entries) == 1 {
		testutil.Equal(t, "UPDATE", sink.entries[0].Operation)
		// extractOldRecord JSON-decodes the CTE sentinel into map[string]any.
		oldValues, ok := sink.entries[0].OldValues.(map[string]any)
		testutil.True(t, ok, "expected map[string]any old values")
		testutil.Equal(t, "u1", oldValues["id"])
		testutil.Equal(t, "old@example.com", oldValues["email"])
		_, hasSentinel := sink.entries[0].NewValues.(map[string]any)["_audit_old_values"]
		testutil.Equal(t, false, hasSentinel)
	}
}

func TestExecBatchOpDeleteLogsAuditWhenEnabled(t *testing.T) {
	t.Parallel()
	tbl := usersTableForBatchAudit()
	sink := &captureBatchAuditSink{audited: map[string]bool{"users": true}}
	h := &Handler{logger: slog.Default(), auditSink: sink}

	q := &fakeBatchQuerier{queryRows: []pgx.Rows{
		&fakeBatchRows{cols: []string{"id", "email"}, rows: [][]any{{"u1", "gone@example.com"}}},
	}}
	req := httptest.NewRequest("POST", "/collections/users/batch", nil)

	result, event, err := h.execBatchOp(req, q, tbl, BatchOperation{Method: "delete", ID: "u1"})
	testutil.NoError(t, err)
	testutil.Equal(t, 204, result.Status)
	testutil.NotNil(t, event)
	testutil.Equal(t, 1, len(sink.entries))
	if len(sink.entries) == 1 {
		testutil.Equal(t, "DELETE", sink.entries[0].Operation)
		testutil.Equal(t, "u1", sink.entries[0].OldValues.(map[string]any)["id"])
	}
	testutil.Equal(t, 0, q.execCalls)
}

func TestExecBatchOpSkipsAuditForNonAuditedTable(t *testing.T) {
	t.Parallel()
	tbl := usersTableForBatchAudit()
	sink := &captureBatchAuditSink{audited: map[string]bool{"other": true}}
	h := &Handler{logger: slog.Default(), auditSink: sink}

	q := &fakeBatchQuerier{queryRows: []pgx.Rows{
		&fakeBatchRows{cols: []string{"id", "email"}, rows: [][]any{{"u1", "new@example.com"}}},
	}}
	req := httptest.NewRequest("POST", "/collections/users/batch", nil)

	_, _, err := h.execBatchOp(req, q, tbl, BatchOperation{Method: "create", Body: map[string]any{"email": "new@example.com"}})
	testutil.NoError(t, err)
	testutil.Equal(t, 0, len(sink.entries))
}

func TestExecBatchOpUpdateSetsEventOldRecordWithoutAudit(t *testing.T) {
	t.Parallel()
	tbl := usersTableForBatchAudit()
	h := &Handler{logger: slog.Default()}

	oldJSON := []byte(`{"id":"u1","email":"old@example.com"}`)
	q := &fakeBatchQuerier{queryRows: []pgx.Rows{
		&fakeBatchRows{cols: []string{"id", "email", "_audit_old_values"}, rows: [][]any{{"u1", "new@example.com", oldJSON}}},
	}}
	req := httptest.NewRequest("POST", "/collections/users/batch", nil)

	result, event, err := h.execBatchOp(req, q, tbl, BatchOperation{Method: "update", ID: "u1", Body: map[string]any{"email": "new@example.com"}})
	testutil.NoError(t, err)
	testutil.Equal(t, 200, result.Status)
	testutil.NotNil(t, event)
	if event != nil {
		testutil.Equal(t, "update", event.Action)
		testutil.Equal(t, "u1", event.Record["id"])
		testutil.Equal(t, "new@example.com", event.Record["email"])
		testutil.NotNil(t, event.OldRecord)
		testutil.Equal(t, "u1", event.OldRecord["id"])
		testutil.Equal(t, "old@example.com", event.OldRecord["email"])
		_, hasSentinel := event.Record["_audit_old_values"]
		testutil.Equal(t, false, hasSentinel)
	}
}

func TestExecBatchOpDeleteSetsEventOldRecordWithoutAudit(t *testing.T) {
	t.Parallel()
	tbl := usersTableForBatchAudit()
	h := &Handler{logger: slog.Default()}

	q := &fakeBatchQuerier{queryRows: []pgx.Rows{
		&fakeBatchRows{cols: []string{"id", "email"}, rows: [][]any{{"u1", "gone@example.com"}}},
	}}
	req := httptest.NewRequest("POST", "/collections/users/batch", nil)

	result, event, err := h.execBatchOp(req, q, tbl, BatchOperation{Method: "delete", ID: "u1"})
	testutil.NoError(t, err)
	testutil.Equal(t, 204, result.Status)
	testutil.NotNil(t, event)
	if event != nil {
		testutil.Equal(t, "delete", event.Action)
		testutil.Equal(t, "u1", event.Record["id"])
		testutil.Equal(t, 1, len(event.Record))
		testutil.NotNil(t, event.OldRecord)
		testutil.Equal(t, "u1", event.OldRecord["id"])
		testutil.Equal(t, "gone@example.com", event.OldRecord["email"])
	}
	testutil.Equal(t, 0, q.execCalls)
}
