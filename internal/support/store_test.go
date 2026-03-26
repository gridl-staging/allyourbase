package support

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type fakeSupportRow struct {
	values []any
	err    error
}

func (r *fakeSupportRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	return scanSupportValuesIntoDest(r.values, dest...)
}

type fakeSupportRows struct {
	rows [][]any
	idx  int
	err  error
}

func (r *fakeSupportRows) Close()                                       {}
func (r *fakeSupportRows) Err() error                                   { return r.err }
func (r *fakeSupportRows) CommandTag() pgconn.CommandTag                { return pgconn.CommandTag{} }
func (r *fakeSupportRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (r *fakeSupportRows) Values() ([]any, error)                       { return nil, nil }
func (r *fakeSupportRows) RawValues() [][]byte                          { return nil }
func (r *fakeSupportRows) Conn() *pgx.Conn                              { return nil }

func (r *fakeSupportRows) Next() bool {
	if r.idx >= len(r.rows) {
		return false
	}
	r.idx++
	return true
}

func (r *fakeSupportRows) Scan(dest ...any) error {
	if r.idx == 0 || r.idx > len(r.rows) {
		return errors.New("scan called out of bounds")
	}
	return scanSupportValuesIntoDest(r.rows[r.idx-1], dest...)
}

type fakeSupportDB struct {
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

func (db *fakeSupportDB) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	db.queryRowSQL = append(db.queryRowSQL, sql)
	db.queryRowArg = append(db.queryRowArg, args)
	if db.queryRowFn != nil {
		return db.queryRowFn(sql, args...)
	}
	return &fakeSupportRow{err: errors.New("query row not configured")}
}

func (db *fakeSupportDB) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	db.querySQL = append(db.querySQL, sql)
	db.queryArg = append(db.queryArg, args)
	if db.queryFn != nil {
		return db.queryFn(sql, args...)
	}
	return &fakeSupportRows{}, nil
}

func (db *fakeSupportDB) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	db.execSQL = append(db.execSQL, sql)
	db.execArg = append(db.execArg, args)
	if db.execFn != nil {
		return db.execFn(sql, args...)
	}
	return pgconn.NewCommandTag("UPDATE 0"), nil
}

func strptr(v string) *string {
	return &v
}

func scanSupportValuesIntoDest(values []any, dest ...any) error {
	if len(dest) != len(values) {
		return fmt.Errorf("dest len %d != values len %d", len(dest), len(values))
	}
	for i, d := range dest {
		switch ptr := d.(type) {
		case *string:
			v, ok := values[i].(string)
			if !ok {
				return fmt.Errorf("value %d is not string", i)
			}
			*ptr = v
		case *time.Time:
			v, ok := values[i].(time.Time)
			if !ok {
				return fmt.Errorf("value %d is not time.Time", i)
			}
			*ptr = v
		default:
			return fmt.Errorf("unsupported scan destination type %T", d)
		}
	}
	return nil
}

func TestStoreCreateTicket(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC().Round(time.Second)
	db := &fakeSupportDB{}
	db.queryRowFn = func(sql string, args ...any) pgx.Row {
		return &fakeSupportRow{values: []any{"ticket-1", "tenant-1", "user-1", "Need help", TicketStatusOpen, TicketPriorityNormal, now, now}}
	}

	store := NewStoreWithDB(db)
	ticket, err := store.CreateTicket(context.Background(), "tenant-1", "user-1", "Need help", "first message", TicketPriorityNormal)
	testutil.NoError(t, err)
	testutil.Equal(t, "ticket-1", ticket.ID)
	testutil.Equal(t, TicketStatusOpen, ticket.Status)
	testutil.Equal(t, TicketPriorityNormal, ticket.Priority)
	testutil.SliceLen(t, db.queryRowSQL, 1)
	testutil.Contains(t, db.queryRowSQL[0], "WITH new_ticket AS")
	testutil.Contains(t, db.queryRowSQL[0], "INSERT INTO _ayb_support_tickets")
	testutil.Contains(t, db.queryRowSQL[0], "INSERT INTO _ayb_support_messages")
	testutil.Contains(t, db.queryRowSQL[0], "customer")
}

func TestStoreListTicketsWithFilters(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC().Round(time.Second)
	db := &fakeSupportDB{}
	db.queryFn = func(sql string, args ...any) (pgx.Rows, error) {
		return &fakeSupportRows{rows: [][]any{{"ticket-1", "tenant-1", "user-1", "Need help", TicketStatusOpen, TicketPriorityHigh, now, now}}}, nil
	}

	store := NewStoreWithDB(db)
	tickets, err := store.ListTickets(context.Background(), "tenant-1", TicketFilters{Status: TicketStatusOpen, Priority: TicketPriorityHigh})
	testutil.NoError(t, err)
	testutil.SliceLen(t, tickets, 1)
	testutil.Equal(t, "ticket-1", tickets[0].ID)
	testutil.SliceLen(t, db.querySQL, 1)
	testutil.Contains(t, db.querySQL[0], "FROM _ayb_support_tickets")
	testutil.Contains(t, db.querySQL[0], "WHERE")
	testutil.Contains(t, db.querySQL[0], "tenant_id")
	testutil.Contains(t, db.querySQL[0], "status")
	testutil.Contains(t, db.querySQL[0], "priority")
	testutil.Contains(t, db.querySQL[0], "ORDER BY created_at DESC")
}

func TestStoreGetTicketNotFound(t *testing.T) {
	t.Parallel()

	db := &fakeSupportDB{}
	db.queryRowFn = func(sql string, args ...any) pgx.Row {
		return &fakeSupportRow{err: pgx.ErrNoRows}
	}

	store := NewStoreWithDB(db)
	_, err := store.GetTicket(context.Background(), "missing")
	testutil.True(t, errors.Is(err, ErrTicketNotFound), "expected ErrTicketNotFound, got %v", err)
}

func TestStoreUpdateTicketPartialFields(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC().Round(time.Second)
	cases := []struct {
		name     string
		update   TicketUpdate
		contains []string
	}{
		{name: "status only", update: TicketUpdate{Status: strptr(TicketStatusInProgress)}, contains: []string{"status", "updated_at = NOW()"}},
		{name: "priority only", update: TicketUpdate{Priority: strptr(TicketPriorityUrgent)}, contains: []string{"priority", "updated_at = NOW()"}},
		{name: "status and priority", update: TicketUpdate{Status: strptr(TicketStatusResolved), Priority: strptr(TicketPriorityLow)}, contains: []string{"status", "priority", "updated_at = NOW()"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := &fakeSupportDB{}
			db.queryRowFn = func(sql string, args ...any) pgx.Row {
				return &fakeSupportRow{values: []any{"ticket-1", "tenant-1", "user-1", "Need help", TicketStatusOpen, TicketPriorityNormal, now, now}}
			}

			store := NewStoreWithDB(db)
			_, err := store.UpdateTicket(context.Background(), "ticket-1", tc.update)
			testutil.NoError(t, err)
			testutil.SliceLen(t, db.queryRowSQL, 1)
			for _, piece := range tc.contains {
				testutil.Contains(t, db.queryRowSQL[0], piece)
			}
		})
	}
}

func TestStoreUpdateTicketRejectsEmptyPatch(t *testing.T) {
	t.Parallel()

	db := &fakeSupportDB{}
	store := NewStoreWithDB(db)

	_, err := store.UpdateTicket(context.Background(), "ticket-1", TicketUpdate{})
	testutil.True(t, errors.Is(err, ErrNoTicketUpdates), "expected ErrNoTicketUpdates, got %v", err)
	testutil.SliceLen(t, db.queryRowSQL, 0)
}

func TestStoreAddMessageUpdatesParentTicket(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC().Round(time.Second)
	db := &fakeSupportDB{}
	db.queryRowFn = func(sql string, args ...any) pgx.Row {
		return &fakeSupportRow{values: []any{"msg-1", "ticket-1", SenderSupport, "working on it", now}}
	}

	store := NewStoreWithDB(db)
	msg, err := store.AddMessage(context.Background(), "ticket-1", SenderSupport, "working on it")
	testutil.NoError(t, err)
	testutil.Equal(t, "msg-1", msg.ID)
	testutil.SliceLen(t, db.queryRowSQL, 1)
	testutil.Contains(t, db.queryRowSQL[0], "WITH updated_ticket AS")
	testutil.Contains(t, db.queryRowSQL[0], "INSERT INTO _ayb_support_messages")
	testutil.Contains(t, db.queryRowSQL[0], "UPDATE _ayb_support_tickets")
	testutil.Contains(t, db.queryRowSQL[0], "updated_at = NOW()")
}

func TestStoreAddMessageTicketNotFound(t *testing.T) {
	t.Parallel()

	db := &fakeSupportDB{}
	db.queryRowFn = func(sql string, args ...any) pgx.Row {
		return &fakeSupportRow{err: pgx.ErrNoRows}
	}

	store := NewStoreWithDB(db)
	_, err := store.AddMessage(context.Background(), "missing-ticket", SenderSupport, "hello")
	testutil.True(t, errors.Is(err, ErrTicketNotFound), "expected ErrTicketNotFound, got %v", err)
}

func TestStoreListMessages(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC().Round(time.Second)
	db := &fakeSupportDB{}
	db.queryFn = func(sql string, args ...any) (pgx.Rows, error) {
		return &fakeSupportRows{rows: [][]any{
			{"msg-1", "ticket-1", SenderCustomer, "first", now},
			{"msg-2", "ticket-1", SenderSupport, "second", now.Add(time.Second)},
		}}, nil
	}

	store := NewStoreWithDB(db)
	msgs, err := store.ListMessages(context.Background(), "ticket-1")
	testutil.NoError(t, err)
	testutil.SliceLen(t, msgs, 2)
	testutil.Equal(t, "msg-1", msgs[0].ID)
	testutil.Equal(t, "msg-2", msgs[1].ID)
	testutil.SliceLen(t, db.querySQL, 1)
	testutil.Contains(t, db.querySQL[0], "WHERE ticket_id = $1")
	testutil.Contains(t, db.querySQL[0], "ORDER BY created_at ASC")
}

func TestServiceValidation(t *testing.T) {
	t.Parallel()

	db := &fakeSupportDB{}
	svc := NewService(NewStoreWithDB(db))

	_, err := svc.CreateTicket(context.Background(), "tenant-1", "user-1", "", "body", TicketPriorityNormal)
	testutil.ErrorContains(t, err, "subject")

	_, err = svc.CreateTicket(context.Background(), "tenant-1", "user-1", "subject", "", TicketPriorityNormal)
	testutil.ErrorContains(t, err, "body")

	_, err = svc.CreateTicket(context.Background(), "tenant-1", "user-1", "subject", "body", "invalid")
	testutil.ErrorContains(t, err, "priority")

	_, err = svc.AddMessage(context.Background(), "ticket-1", "invalid", "body")
	testutil.ErrorContains(t, err, "sender")

	_, err = svc.UpdateTicket(context.Background(), "ticket-1", TicketUpdate{})
	testutil.ErrorContains(t, err, "at least one")

	testutil.SliceLen(t, db.queryRowSQL, 0)
	testutil.SliceLen(t, db.querySQL, 0)
	testutil.SliceLen(t, db.execSQL, 0)
}

func TestNewNoopSupportService(t *testing.T) {
	t.Parallel()

	svc := NewNoopSupportService()

	ticket, err := svc.CreateTicket(context.Background(), "tenant-1", "user-1", "subject", "body", TicketPriorityNormal)
	testutil.NoError(t, err)
	testutil.Nil(t, ticket)

	tickets, err := svc.ListTickets(context.Background(), "tenant-1", TicketFilters{})
	testutil.NoError(t, err)
	testutil.Equal(t, 0, len(tickets))

	ticket, err = svc.GetTicket(context.Background(), "ticket-1")
	testutil.NoError(t, err)
	testutil.Nil(t, ticket)

	ticket, err = svc.UpdateTicket(context.Background(), "ticket-1", TicketUpdate{})
	testutil.NoError(t, err)
	testutil.Nil(t, ticket)

	msg, err := svc.AddMessage(context.Background(), "ticket-1", SenderCustomer, "body")
	testutil.NoError(t, err)
	testutil.Nil(t, msg)

	msgs, err := svc.ListMessages(context.Background(), "ticket-1")
	testutil.NoError(t, err)
	testutil.Equal(t, 0, len(msgs))

	// Guard against accidental string normalization differences in no-op paths.
	testutil.True(t, !strings.Contains(SenderCustomer, " "))
}
