package notifications

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type fakeRow struct {
	scanFn func(dest ...any) error
}

func (r *fakeRow) Scan(dest ...any) error {
	if r.scanFn == nil {
		return errors.New("scan not configured")
	}
	return r.scanFn(dest...)
}

type fakeRows struct {
	items []Notification
	idx   int
	err   error
}

func (r *fakeRows) Close()                                       {}
func (r *fakeRows) Err() error                                   { return r.err }
func (r *fakeRows) CommandTag() pgconn.CommandTag                { return pgconn.CommandTag{} }
func (r *fakeRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (r *fakeRows) Next() bool {
	if r.idx >= len(r.items) {
		return false
	}
	r.idx++
	return true
}
func (r *fakeRows) Scan(dest ...any) error {
	if r.idx == 0 || r.idx > len(r.items) {
		return errors.New("scan called out of bounds")
	}
	n := r.items[r.idx-1]
	meta, _ := json.Marshal(n.Metadata)
	*(dest[0].(*string)) = n.ID
	*(dest[1].(*string)) = n.UserID
	*(dest[2].(*string)) = n.Title
	*(dest[3].(*string)) = n.Body
	*(dest[4].(*[]byte)) = meta
	*(dest[5].(*string)) = n.Channel
	if n.ReadAt != nil {
		*(dest[6].(**time.Time)) = n.ReadAt
	} else {
		*(dest[6].(**time.Time)) = nil
	}
	*(dest[7].(*time.Time)) = n.CreatedAt
	return nil
}
func (r *fakeRows) Values() ([]any, error) { return nil, nil }
func (r *fakeRows) RawValues() [][]byte    { return nil }
func (r *fakeRows) Conn() *pgx.Conn        { return nil }

type fakeDB struct {
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

func (db *fakeDB) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	db.queryRowSQL = append(db.queryRowSQL, sql)
	db.queryRowArg = append(db.queryRowArg, args)
	if db.queryRowFn != nil {
		return db.queryRowFn(sql, args...)
	}
	return &fakeRow{scanFn: func(dest ...any) error { return errors.New("no row configured") }}
}

func (db *fakeDB) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	db.querySQL = append(db.querySQL, sql)
	db.queryArg = append(db.queryArg, args)
	if db.queryFn != nil {
		return db.queryFn(sql, args...)
	}
	return &fakeRows{}, nil
}

func (db *fakeDB) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	db.execSQL = append(db.execSQL, sql)
	db.execArg = append(db.execArg, args)
	if db.execFn != nil {
		return db.execFn(sql, args...)
	}
	return pgconn.NewCommandTag("UPDATE 0"), nil
}

func TestStoreCreate_DefaultsAndScans(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC().Round(time.Second)
	db := &fakeDB{}
	db.queryRowFn = func(sql string, args ...any) pgx.Row {
		return &fakeRow{scanFn: func(dest ...any) error {
			*(dest[0].(*string)) = "notif-1"
			*(dest[1].(*string)) = "user-1"
			*(dest[2].(*string)) = "Hello"
			*(dest[3].(*string)) = "World"
			*(dest[4].(*[]byte)) = []byte(`{"k":"v"}`)
			*(dest[5].(*string)) = "general"
			*(dest[6].(**time.Time)) = nil
			*(dest[7].(*time.Time)) = now
			return nil
		}}
	}

	s := &Store{db: db}
	n, err := s.Create(context.Background(), "user-1", "Hello", "World", nil, "")
	testutil.NoError(t, err)
	testutil.Equal(t, 1, len(db.queryRowSQL))
	testutil.Contains(t, db.queryRowSQL[0], "INSERT INTO _ayb_notifications")
	testutil.Equal(t, "notif-1", n.ID)
	testutil.Equal(t, "general", n.Channel)
	testutil.Equal(t, "v", n.Metadata["k"])
	testutil.Equal(t, now, n.CreatedAt)

	metaArg, ok := db.queryRowArg[0][3].([]byte)
	testutil.True(t, ok, "metadata arg must be []byte")
	testutil.Equal(t, "{}", string(metaArg))
}

func TestStoreListByUser_UnreadFilter(t *testing.T) {
	t.Parallel()
	created := time.Now().UTC().Round(time.Second)
	db := &fakeDB{}
	db.queryRowFn = func(sql string, args ...any) pgx.Row {
		return &fakeRow{scanFn: func(dest ...any) error {
			*(dest[0].(*int)) = 1
			return nil
		}}
	}
	db.queryFn = func(sql string, args ...any) (pgx.Rows, error) {
		return &fakeRows{items: []Notification{{
			ID: "notif-1", UserID: "user-1", Title: "T", Body: "B", Metadata: map[string]any{}, Channel: "general", CreatedAt: created,
		}}}, nil
	}

	s := &Store{db: db}
	items, total, err := s.ListByUser(context.Background(), "user-1", true, 2, 10)
	testutil.NoError(t, err)
	testutil.Equal(t, 1, total)
	testutil.Equal(t, 1, len(items))
	testutil.True(t, strings.Contains(db.queryRowSQL[0], "read_at IS NULL"), "count query should include unread filter")
	testutil.True(t, strings.Contains(db.querySQL[0], "read_at IS NULL"), "data query should include unread filter")
	testutil.Equal(t, 10, db.queryArg[0][1])
	testutil.Equal(t, 10, db.queryArg[0][2])
}

func TestStoreMarkRead_NotFound(t *testing.T) {
	t.Parallel()
	db := &fakeDB{execFn: func(sql string, args ...any) (pgconn.CommandTag, error) {
		return pgconn.NewCommandTag("UPDATE 0"), nil
	}}
	s := &Store{db: db}
	err := s.MarkRead(context.Background(), "missing", "user-1")
	testutil.True(t, errors.Is(err, ErrNotFound), "expected ErrNotFound, got %v", err)
}

func TestStoreMarkAllRead_ReturnsCount(t *testing.T) {
	t.Parallel()
	db := &fakeDB{execFn: func(sql string, args ...any) (pgconn.CommandTag, error) {
		return pgconn.NewCommandTag("UPDATE 3"), nil
	}}
	s := &Store{db: db}
	n, err := s.MarkAllRead(context.Background(), "user-1")
	testutil.NoError(t, err)
	testutil.Equal(t, int64(3), n)
}
