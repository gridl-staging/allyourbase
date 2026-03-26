package backup

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type fakeRestoreJobRow struct {
	scanFn func(dest ...any) error
}

func (r *fakeRestoreJobRow) Scan(dest ...any) error {
	if r.scanFn == nil {
		return errors.New("scan not configured")
	}
	return r.scanFn(dest...)
}

type fakeRestoreJobRows struct {
	items []RestoreJob
	idx   int
	err   error
}

func (r *fakeRestoreJobRows) Close()                                       {}
func (r *fakeRestoreJobRows) Err() error                                   { return r.err }
func (r *fakeRestoreJobRows) CommandTag() pgconn.CommandTag                { return pgconn.CommandTag{} }
func (r *fakeRestoreJobRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (r *fakeRestoreJobRows) Values() ([]any, error)                       { return nil, nil }
func (r *fakeRestoreJobRows) RawValues() [][]byte                          { return nil }
func (r *fakeRestoreJobRows) Conn() *pgx.Conn                              { return nil }

func (r *fakeRestoreJobRows) Next() bool {
	if r.idx >= len(r.items) {
		return false
	}
	r.idx++
	return true
}

func (r *fakeRestoreJobRows) Scan(dest ...any) error {
	if r.idx == 0 || r.idx > len(r.items) {
		return errors.New("scan called out of bounds")
	}
	populateRestoreJobDest(r.items[r.idx-1], dest)
	return nil
}

type fakeRestoreJobDB struct {
	queryRowSQL []string
	queryRowArg [][]any
	querySQL    []string
	queryArg    [][]any
	execSQL     []string
	execArg     [][]any

	queryRowFn func(sql string, args ...any) pgx.Row
	queryFn    func(sql string, args ...any) (pgx.Rows, error)
	execFn     func(sql string, args ...any) (pgconn.CommandTag, error)
}

func (db *fakeRestoreJobDB) QueryRow(_ context.Context, sql string, args ...any) pgx.Row {
	db.queryRowSQL = append(db.queryRowSQL, sql)
	db.queryRowArg = append(db.queryRowArg, args)
	if db.queryRowFn != nil {
		return db.queryRowFn(sql, args...)
	}
	return &fakeRestoreJobRow{scanFn: func(dest ...any) error { return errors.New("no row configured") }}
}

func (db *fakeRestoreJobDB) Query(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
	db.querySQL = append(db.querySQL, sql)
	db.queryArg = append(db.queryArg, args)
	if db.queryFn != nil {
		return db.queryFn(sql, args...)
	}
	return &fakeRestoreJobRows{}, nil
}

func (db *fakeRestoreJobDB) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	db.execSQL = append(db.execSQL, sql)
	db.execArg = append(db.execArg, args)
	if db.execFn != nil {
		return db.execFn(sql, args...)
	}
	return pgconn.NewCommandTag("UPDATE 1"), nil
}

func restoreJobFixture(id string) RestoreJob {
	now := time.Now().UTC().Round(time.Second)
	return RestoreJob{
		ID:                 id,
		ProjectID:          "proj1",
		DatabaseID:         "db1",
		Environment:        "prod",
		TargetTime:         now.Add(-10 * time.Minute),
		Phase:              RestorePhasePending,
		Status:             RestoreStatusPending,
		BaseBackupID:       "",
		WALSegmentsNeeded:  0,
		VerificationResult: json.RawMessage(`{"passed":true}`),
		Logs:               "",
		ErrorMessage:       "",
		RequestedBy:        "user@example.com",
		StartedAt:          now,
		CompletedAt:        nil,
	}
}

func populateRestoreJobDest(item RestoreJob, dest []any) {
	verification := []byte(nil)
	if len(item.VerificationResult) > 0 {
		verification = append([]byte(nil), item.VerificationResult...)
	}
	*(dest[0].(*string)) = item.ID
	*(dest[1].(*string)) = item.ProjectID
	*(dest[2].(*string)) = item.DatabaseID
	*(dest[3].(*string)) = item.Environment
	*(dest[4].(*time.Time)) = item.TargetTime
	*(dest[5].(*string)) = item.Phase
	*(dest[6].(*string)) = item.Status
	*(dest[7].(*string)) = item.BaseBackupID
	*(dest[8].(*int)) = item.WALSegmentsNeeded
	*(dest[9].(*[]byte)) = verification
	*(dest[10].(*string)) = item.Logs
	*(dest[11].(*string)) = item.ErrorMessage
	*(dest[12].(*string)) = item.RequestedBy
	*(dest[13].(*time.Time)) = item.StartedAt
	*(dest[14].(**time.Time)) = item.CompletedAt
}

func scanRowWithJob(item RestoreJob) pgx.Row {
	return &fakeRestoreJobRow{scanFn: func(dest ...any) error {
		populateRestoreJobDest(item, dest)
		return nil
	}}
}

func TestRestoreJobCreate(t *testing.T) {
	t.Parallel()

	item := restoreJobFixture("job-1")
	db := &fakeRestoreJobDB{queryRowFn: func(sql string, args ...any) pgx.Row {
		return scanRowWithJob(item)
	}}
	repo := &PgRestoreJobRepo{db: db}

	created, err := repo.Create(context.Background(), item)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.ID != "job-1" {
		t.Fatalf("ID = %q; want job-1", created.ID)
	}
	if len(db.queryRowSQL) != 1 || !strings.Contains(db.queryRowSQL[0], "INSERT INTO _ayb_restore_jobs") {
		t.Fatalf("expected INSERT, got %q", strings.Join(db.queryRowSQL, "\n"))
	}
}

func TestRestoreJobGet(t *testing.T) {
	t.Parallel()

	item := restoreJobFixture("job-get")
	db := &fakeRestoreJobDB{queryRowFn: func(sql string, args ...any) pgx.Row {
		return scanRowWithJob(item)
	}}
	repo := &PgRestoreJobRepo{db: db}

	got, err := repo.Get(context.Background(), "job-get")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ID != "job-get" {
		t.Fatalf("ID = %q; want job-get", got.ID)
	}
	if len(db.queryRowSQL) != 1 || !strings.Contains(db.queryRowSQL[0], "FROM _ayb_restore_jobs") {
		t.Fatalf("expected SELECT, got %q", strings.Join(db.queryRowSQL, "\n"))
	}
}

func TestRestoreJobUpdatePhase(t *testing.T) {
	t.Parallel()

	db := &fakeRestoreJobDB{}
	repo := &PgRestoreJobRepo{db: db}

	err := repo.UpdatePhase(context.Background(), "job-1", RestorePhaseValidating, RestoreStatusRunning)
	if err != nil {
		t.Fatalf("UpdatePhase: %v", err)
	}
	if len(db.execSQL) != 1 || !strings.Contains(db.execSQL[0], "phase = $2") || !strings.Contains(db.execSQL[0], "status = $3") {
		t.Fatalf("expected phase+status update, got %q", strings.Join(db.execSQL, "\n"))
	}
}

func TestRestoreJobMarkCompleted(t *testing.T) {
	t.Parallel()

	db := &fakeRestoreJobDB{}
	repo := &PgRestoreJobRepo{db: db}

	vr := json.RawMessage(`{"checks":[{"name":"schema","passed":true}]}`)
	if err := repo.MarkCompleted(context.Background(), "job-1", vr); err != nil {
		t.Fatalf("MarkCompleted: %v", err)
	}
	if len(db.execSQL) != 1 || !strings.Contains(db.execSQL[0], "phase = 'completed'") || !strings.Contains(db.execSQL[0], "verification_result = $2") {
		t.Fatalf("unexpected SQL: %q", strings.Join(db.execSQL, "\n"))
	}
}

func TestRestoreJobMarkFailed(t *testing.T) {
	t.Parallel()

	db := &fakeRestoreJobDB{}
	repo := &PgRestoreJobRepo{db: db}

	if err := repo.MarkFailed(context.Background(), "job-1", "boom"); err != nil {
		t.Fatalf("MarkFailed: %v", err)
	}
	if len(db.execSQL) != 1 || !strings.Contains(db.execSQL[0], "phase = 'failed'") || !strings.Contains(db.execSQL[0], "error_message = $2") {
		t.Fatalf("unexpected SQL: %q", strings.Join(db.execSQL, "\n"))
	}
}

func TestRestoreJobAppendLog(t *testing.T) {
	t.Parallel()

	db := &fakeRestoreJobDB{}
	repo := &PgRestoreJobRepo{db: db}

	if err := repo.AppendLog(context.Background(), "job-1", "[phase] validating\n"); err != nil {
		t.Fatalf("AppendLog: %v", err)
	}
	if len(db.execSQL) != 1 || !strings.Contains(db.execSQL[0], "logs = logs || $2") {
		t.Fatalf("unexpected SQL: %q", strings.Join(db.execSQL, "\n"))
	}
}

func TestRestoreJobListByProject(t *testing.T) {
	t.Parallel()

	rows := []RestoreJob{restoreJobFixture("job-1"), restoreJobFixture("job-2")}
	db := &fakeRestoreJobDB{queryFn: func(sql string, args ...any) (pgx.Rows, error) {
		return &fakeRestoreJobRows{items: rows}, nil
	}}
	repo := &PgRestoreJobRepo{db: db}

	items, err := repo.ListByProject(context.Background(), "proj1", "db1")
	if err != nil {
		t.Fatalf("ListByProject: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("len(items) = %d; want 2", len(items))
	}
	if len(db.querySQL) != 1 || !strings.Contains(db.querySQL[0], "ORDER BY started_at DESC") {
		t.Fatalf("unexpected SQL: %q", strings.Join(db.querySQL, "\n"))
	}
}
