package fdw

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/vault"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestValidateIdentifier(t *testing.T) {
	t.Parallel()

	valid := []string{"foo", "foo_bar", "A1", "_x", "abc123"}
	for _, id := range valid {
		if err := ValidateIdentifier(id); err != nil {
			t.Fatalf("ValidateIdentifier(%q) unexpected error: %v", id, err)
		}
	}

	tooLong := strings.Repeat("a", 64)
	invalid := []string{"", "1abc", "abc-def", "abc.def", "abc$", tooLong}
	for _, id := range invalid {
		if err := ValidateIdentifier(id); err == nil {
			t.Fatalf("ValidateIdentifier(%q) expected error", id)
		}
	}
}

func TestValidateFDWType(t *testing.T) {
	t.Parallel()

	for _, typ := range []string{"postgres_fdw", "file_fdw"} {
		if err := ValidateFDWType(typ); err != nil {
			t.Fatalf("ValidateFDWType(%q) unexpected error: %v", typ, err)
		}
	}

	for _, typ := range []string{"", "mysql_fdw", "postgres"} {
		if err := ValidateFDWType(typ); err == nil {
			t.Fatalf("ValidateFDWType(%q) expected error", typ)
		}
	}
}

func TestCreateServerPostgresFDWSQLSequence(t *testing.T) {
	t.Parallel()

	tx := &mockTx{}
	db := &mockDB{beginTx: tx}
	vs := &mockVaultStore{}
	svc := NewService(db, vs)

	err := svc.CreateServer(context.Background(), CreateServerOpts{
		Name:    "analytics_fdw",
		FDWType: "postgres_fdw",
		Options: map[string]string{
			"host":   "localhost",
			"port":   "5432",
			"dbname": "analytics",
		},
		UserMapping: UserMapping{
			User:     "report_user",
			Password: "secret123",
		},
	})
	if err != nil {
		t.Fatalf("CreateServer unexpected error: %v", err)
	}

	if !tx.committed {
		t.Fatal("expected transaction commit")
	}
	if len(tx.execSQL) != 4 {
		t.Fatalf("expected 4 exec statements, got %d", len(tx.execSQL))
	}
	mustContain(t, tx.execSQL[0], `CREATE EXTENSION IF NOT EXISTS "postgres_fdw"`)
	mustContain(t, tx.execSQL[1], `CREATE SERVER "analytics_fdw" FOREIGN DATA WRAPPER "postgres_fdw" OPTIONS (`)
	mustContain(t, tx.execSQL[1], `host 'localhost'`)
	mustContain(t, tx.execSQL[1], `port '5432'`)
	mustContain(t, tx.execSQL[1], `dbname 'analytics'`)
	mustContain(t, tx.execSQL[2], `CREATE USER MAPPING FOR CURRENT_USER SERVER "analytics_fdw" OPTIONS (user 'report_user', password 'secret123')`)
	mustContain(t, tx.execSQL[3], `INSERT INTO _ayb_fdw_servers`)

	if len(vs.setCalls) != 1 {
		t.Fatalf("expected one vault SetSecret call, got %d", len(vs.setCalls))
	}
	if vs.setCalls[0].key != "fdw.analytics_fdw.password" {
		t.Fatalf("vault key = %q, want %q", vs.setCalls[0].key, "fdw.analytics_fdw.password")
	}
	if string(vs.setCalls[0].value) != "secret123" {
		t.Fatalf("vault value = %q, want secret123", string(vs.setCalls[0].value))
	}
}

func TestCreateServerFileFDWSQLSequence(t *testing.T) {
	t.Parallel()

	tx := &mockTx{}
	db := &mockDB{beginTx: tx}
	vs := &mockVaultStore{}
	svc := NewService(db, vs)

	err := svc.CreateServer(context.Background(), CreateServerOpts{
		Name:    "csv_fdw",
		FDWType: "file_fdw",
		Options: map[string]string{
			"filename": "/tmp/data.csv",
		},
	})
	if err != nil {
		t.Fatalf("CreateServer unexpected error: %v", err)
	}

	if len(tx.execSQL) != 3 {
		t.Fatalf("expected 3 exec statements, got %d", len(tx.execSQL))
	}
	mustContain(t, tx.execSQL[0], `CREATE EXTENSION IF NOT EXISTS "file_fdw"`)
	mustContain(t, tx.execSQL[1], `CREATE SERVER "csv_fdw" FOREIGN DATA WRAPPER "file_fdw" OPTIONS (filename '/tmp/data.csv')`)
	mustContain(t, tx.execSQL[2], `INSERT INTO _ayb_fdw_servers`)
	if len(vs.setCalls) != 0 {
		t.Fatalf("expected no vault SetSecret calls, got %d", len(vs.setCalls))
	}
}

func TestCreateServerEscapesSingleQuotes(t *testing.T) {
	t.Parallel()

	tx := &mockTx{}
	db := &mockDB{beginTx: tx}
	vs := &mockVaultStore{}
	svc := NewService(db, vs)

	err := svc.CreateServer(context.Background(), CreateServerOpts{
		Name:    "quotes_fdw",
		FDWType: "postgres_fdw",
		Options: map[string]string{
			"host":   "db'o",
			"port":   "5432",
			"dbname": "ana'lytics",
		},
		UserMapping: UserMapping{
			User:     "rep'orter",
			Password: "pa'ss",
		},
	})
	if err != nil {
		t.Fatalf("CreateServer unexpected error: %v", err)
	}

	mustContain(t, tx.execSQL[1], `host 'db''o'`)
	mustContain(t, tx.execSQL[1], `dbname 'ana''lytics'`)
	mustContain(t, tx.execSQL[2], `user 'rep''orter'`)
	mustContain(t, tx.execSQL[2], `password 'pa''ss'`)
}

func TestCreateServerCleansUpSecretWhenTransactionFails(t *testing.T) {
	t.Parallel()

	tx := &mockTx{
		execErrAt: map[int]error{
			3: errors.New("tracking insert failed"),
		},
	}
	db := &mockDB{beginTx: tx}
	vs := &mockVaultStore{}
	svc := NewService(db, vs)

	err := svc.CreateServer(context.Background(), CreateServerOpts{
		Name:    "analytics_fdw",
		FDWType: "postgres_fdw",
		Options: map[string]string{
			"host":   "localhost",
			"port":   "5432",
			"dbname": "analytics",
		},
		UserMapping: UserMapping{
			User:     "report_user",
			Password: "secret123",
		},
	})
	if err == nil {
		t.Fatal("expected CreateServer error")
	}
	if tx.committed {
		t.Fatal("expected transaction not to commit on error")
	}
	if !tx.rolled {
		t.Fatal("expected transaction rollback on error")
	}
	if len(vs.setCalls) != 1 {
		t.Fatalf("expected one SetSecret call, got %d", len(vs.setCalls))
	}
	if len(vs.deleteCalls) != 1 {
		t.Fatalf("expected one DeleteSecret cleanup call, got %d", len(vs.deleteCalls))
	}
	if vs.deleteCalls[0] != "fdw.analytics_fdw.password" {
		t.Fatalf("unexpected deleted key %q", vs.deleteCalls[0])
	}
}

func TestImportTablesGeneratesSQLWithAndWithoutFilter(t *testing.T) {
	t.Parallel()

	rows := &mockRows{
		data: [][]any{
			{"public", "events", "analytics_fdw", "id", "int8"},
			{"public", "events", "analytics_fdw", "name", "text"},
		},
	}
	db := &mockDB{
		queryRows: rows,
	}
	svc := NewService(db, &mockVaultStore{})

	out, err := svc.ImportTables(context.Background(), "analytics_fdw", ImportOpts{})
	if err != nil {
		t.Fatalf("ImportTables unexpected error: %v", err)
	}
	if len(out) != 1 || len(out[0].Columns) != 2 {
		t.Fatalf("unexpected imported table result: %+v", out)
	}
	mustContain(t, db.execSQL[0], `IMPORT FOREIGN SCHEMA "public" FROM SERVER "analytics_fdw" INTO "public"`)

	db.execSQL = nil
	_, err = svc.ImportTables(context.Background(), "analytics_fdw", ImportOpts{
		RemoteSchema: "public",
		LocalSchema:  "local",
		TableNames:   []string{"events", "users"},
	})
	if err != nil {
		t.Fatalf("ImportTables with filter unexpected error: %v", err)
	}
	mustContain(t, db.execSQL[0], `LIMIT TO ("events", "users")`)
	mustContain(t, db.execSQL[0], `INTO "local"`)
}

func TestDropServerCascadeAndCleanup(t *testing.T) {
	t.Parallel()

	tx := &mockTx{}
	db := &mockDB{beginTx: tx}
	vs := &mockVaultStore{}
	svc := NewService(db, vs)

	err := svc.DropServer(context.Background(), "analytics_fdw", true)
	if err != nil {
		t.Fatalf("DropServer unexpected error: %v", err)
	}

	if len(tx.execSQL) != 3 {
		t.Fatalf("expected 3 SQL statements, got %d", len(tx.execSQL))
	}
	mustContain(t, tx.execSQL[0], `DROP USER MAPPING IF EXISTS FOR CURRENT_USER SERVER "analytics_fdw"`)
	mustContain(t, tx.execSQL[1], `DROP SERVER IF EXISTS "analytics_fdw" CASCADE`)
	mustContain(t, tx.execSQL[2], `DELETE FROM _ayb_fdw_servers WHERE name = $1`)
	if len(vs.deleteCalls) != 1 || vs.deleteCalls[0] != "fdw.analytics_fdw.password" {
		t.Fatalf("vault delete calls = %#v, want fdw.analytics_fdw.password", vs.deleteCalls)
	}

	tx.execSQL = nil
	vs.deleteCalls = nil
	err = svc.DropServer(context.Background(), "analytics_fdw", false)
	if err != nil {
		t.Fatalf("DropServer non-cascade unexpected error: %v", err)
	}
	if strings.Contains(tx.execSQL[1], "CASCADE") {
		t.Fatalf("expected non-cascade DROP SERVER, got %q", tx.execSQL[1])
	}
}

func TestDropServerIgnoresMissingVaultSecret(t *testing.T) {
	t.Parallel()

	tx := &mockTx{}
	db := &mockDB{beginTx: tx}
	vs := &mockVaultStore{
		deleteErr: fmt.Errorf("%w: %s", vault.ErrSecretNotFound, "fdw.csv_fdw.password"),
	}
	svc := NewService(db, vs)

	err := svc.DropServer(context.Background(), "csv_fdw", false)
	if err != nil {
		t.Fatalf("DropServer should ignore missing vault secret, got error: %v", err)
	}
	if len(vs.deleteCalls) != 1 || vs.deleteCalls[0] != "fdw.csv_fdw.password" {
		t.Fatalf("vault delete calls = %#v, want fdw.csv_fdw.password", vs.deleteCalls)
	}
}

func TestDropForeignTableQuotesQualifiedName(t *testing.T) {
	t.Parallel()

	db := &mockDB{}
	svc := NewService(db, &mockVaultStore{})

	err := svc.DropForeignTable(context.Background(), "Analytics", "Events")
	if err != nil {
		t.Fatalf("DropForeignTable unexpected error: %v", err)
	}
	if len(db.execSQL) != 1 {
		t.Fatalf("expected 1 SQL statement, got %d", len(db.execSQL))
	}
	mustContain(t, db.execSQL[0], `DROP FOREIGN TABLE IF EXISTS "Analytics"."Events"`)
}

func TestNilGuards(t *testing.T) {
	t.Parallel()

	noDB := NewService(nil, &mockVaultStore{})
	if _, err := noDB.ListServers(context.Background()); err == nil {
		t.Fatal("expected ListServers error with nil db")
	}

	noVault := NewService(&mockDB{queryRows: &mockRows{}}, nil)
	err := noVault.CreateServer(context.Background(), CreateServerOpts{
		Name:    "analytics_fdw",
		FDWType: "file_fdw",
		Options: map[string]string{"filename": "/tmp/file.csv"},
	})
	if err == nil {
		t.Fatal("expected CreateServer error with nil vault store")
	}
}

func mustContain(t *testing.T, s, want string) {
	t.Helper()
	if !strings.Contains(s, want) {
		t.Fatalf("expected %q to contain %q", s, want)
	}
}

type mockDB struct {
	beginTx   pgx.Tx
	beginErr  error
	execSQL   []string
	execArgs  [][]any
	querySQL  []string
	queryRows pgx.Rows
}

func (m *mockDB) Begin(_ context.Context) (pgx.Tx, error) {
	if m.beginErr != nil {
		return nil, m.beginErr
	}
	if m.beginTx == nil {
		m.beginTx = &mockTx{}
	}
	return m.beginTx, nil
}

func (m *mockDB) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	m.execSQL = append(m.execSQL, sql)
	m.execArgs = append(m.execArgs, cloneArgs(args))
	return pgconn.NewCommandTag("OK"), nil
}

func (m *mockDB) Query(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
	m.querySQL = append(m.querySQL, sql)
	if m.queryRows == nil {
		return &mockRows{}, nil
	}
	return m.queryRows, nil
}

func (m *mockDB) QueryRow(_ context.Context, _ string, _ ...any) pgx.Row {
	return mockRow{err: errors.New("not implemented")}
}

type mockTx struct {
	execSQL   []string
	execArgs  [][]any
	committed bool
	rolled    bool
	execErrAt map[int]error
}

func (m *mockTx) Begin(_ context.Context) (pgx.Tx, error) { return m, nil }
func (m *mockTx) Commit(_ context.Context) error {
	m.committed = true
	return nil
}
func (m *mockTx) Rollback(_ context.Context) error {
	m.rolled = true
	return nil
}
func (m *mockTx) CopyFrom(_ context.Context, _ pgx.Identifier, _ []string, _ pgx.CopyFromSource) (int64, error) {
	return 0, nil
}
func (m *mockTx) SendBatch(_ context.Context, _ *pgx.Batch) pgx.BatchResults { return nil }
func (m *mockTx) LargeObjects() pgx.LargeObjects                             { return pgx.LargeObjects{} }
func (m *mockTx) Prepare(_ context.Context, _, _ string) (*pgconn.StatementDescription, error) {
	return nil, nil
}
func (m *mockTx) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	current := len(m.execSQL)
	m.execSQL = append(m.execSQL, sql)
	m.execArgs = append(m.execArgs, cloneArgs(args))
	if err := m.execErrAt[current]; err != nil {
		return pgconn.CommandTag{}, err
	}
	return pgconn.NewCommandTag("OK"), nil
}
func (m *mockTx) Query(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
	return &mockRows{}, nil
}
func (m *mockTx) QueryRow(_ context.Context, _ string, _ ...any) pgx.Row {
	return mockRow{err: errors.New("not implemented")}
}
func (m *mockTx) Conn() *pgx.Conn { return nil }

type mockVaultStore struct {
	setCalls    []setCall
	deleteCalls []string
	deleteErr   error
}

type setCall struct {
	key   string
	value []byte
}

func (m *mockVaultStore) SetSecret(_ context.Context, key string, value []byte) error {
	m.setCalls = append(m.setCalls, setCall{key: key, value: append([]byte(nil), value...)})
	return nil
}

func (m *mockVaultStore) GetSecret(_ context.Context, _ string) ([]byte, error) {
	return nil, errors.New("not implemented")
}

func (m *mockVaultStore) DeleteSecret(_ context.Context, key string) error {
	m.deleteCalls = append(m.deleteCalls, key)
	return m.deleteErr
}

type mockRows struct {
	data [][]any
	idx  int
	err  error
}

func (m *mockRows) Close()                                       {}
func (m *mockRows) Err() error                                   { return m.err }
func (m *mockRows) CommandTag() pgconn.CommandTag                { return pgconn.NewCommandTag("SELECT 0") }
func (m *mockRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (m *mockRows) Next() bool {
	if m.idx >= len(m.data) {
		return false
	}
	m.idx++
	return true
}
func (m *mockRows) Scan(dest ...any) error {
	if m.idx == 0 || m.idx > len(m.data) {
		return fmt.Errorf("scan called out of bounds")
	}
	row := m.data[m.idx-1]
	if len(dest) != len(row) {
		return fmt.Errorf("scan len mismatch: got %d want %d", len(dest), len(row))
	}
	for i := range dest {
		if err := assignScan(dest[i], row[i]); err != nil {
			return err
		}
	}
	return nil
}
func (m *mockRows) Values() ([]any, error) {
	if m.idx == 0 || m.idx > len(m.data) {
		return nil, fmt.Errorf("values called out of bounds")
	}
	return m.data[m.idx-1], nil
}
func (m *mockRows) RawValues() [][]byte { return nil }
func (m *mockRows) Conn() *pgx.Conn     { return nil }

type mockRow struct {
	err error
}

func (m mockRow) Scan(_ ...any) error { return m.err }

func assignScan(dest any, value any) error {
	dv := reflect.ValueOf(dest)
	if dv.Kind() != reflect.Ptr || dv.IsNil() {
		return fmt.Errorf("destination must be a non-nil pointer")
	}
	vv := reflect.ValueOf(value)
	if !vv.IsValid() {
		dv.Elem().Set(reflect.Zero(dv.Elem().Type()))
		return nil
	}
	if vv.Type().AssignableTo(dv.Elem().Type()) {
		dv.Elem().Set(vv)
		return nil
	}
	if vv.Type().ConvertibleTo(dv.Elem().Type()) {
		dv.Elem().Set(vv.Convert(dv.Elem().Type()))
		return nil
	}
	return fmt.Errorf("cannot assign %T to %T", value, dest)
}

func cloneArgs(args []any) []any {
	out := make([]any, len(args))
	copy(out, args)
	return out
}

var _ = time.Now
