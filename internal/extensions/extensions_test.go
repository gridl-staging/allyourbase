package extensions

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
)

// --- Validation tests ---

func TestValidateExtensionName(t *testing.T) {
	valid := []string{
		"pgvector",
		"pg_trgm",
		"pg_cron",
		"pg_stat_statements",
		"uuid-ossp",
		"hstore",
		"postgis",
		"plpgsql",
	}
	for _, name := range valid {
		if err := ValidateExtensionName(name); err != nil {
			t.Errorf("ValidateExtensionName(%q) = %v; want nil", name, err)
		}
	}
}

func TestValidateExtensionNameInvalid(t *testing.T) {
	invalid := []string{
		"",
		"DROP TABLE",
		"pg_trgm; DROP TABLE users",
		"ext\"name",
		"ext'name",
		"ext\nname",
		"ext\x00name",
		".hidden",
		"a/b",
		"ext name",
	}
	for _, name := range invalid {
		if err := ValidateExtensionName(name); err == nil {
			t.Errorf("ValidateExtensionName(%q) = nil; want error", name)
		}
	}
}

func TestValidateExtensionNameTooLong(t *testing.T) {
	long := make([]byte, 64)
	for i := range long {
		long[i] = 'a'
	}
	if err := ValidateExtensionName(string(long)); err != nil {
		t.Errorf("ValidateExtensionName(64 chars) = %v; want nil", err)
	}

	tooLong := make([]byte, 65)
	for i := range tooLong {
		tooLong[i] = 'a'
	}
	if err := ValidateExtensionName(string(tooLong)); err == nil {
		t.Error("ValidateExtensionName(65 chars) = nil; want error")
	}
}

// --- Mock DB for unit tests ---

type mockDB struct {
	queryFunc func(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	execFunc  func(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func (m *mockDB) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return m.queryFunc(ctx, query, args...)
}

func (m *mockDB) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return m.execFunc(ctx, query, args...)
}

// --- Service constructor tests ---

func TestNewService(t *testing.T) {
	db := &mockDB{}
	svc := NewService(db)
	if svc == nil {
		t.Fatal("NewService returned nil")
	}
}

// --- EnableSQL / DisableSQL generation tests ---

func TestEnableSQL(t *testing.T) {
	got := enableSQL("pgvector")
	want := `CREATE EXTENSION IF NOT EXISTS "pgvector"`
	if got != want {
		t.Errorf("enableSQL(%q) = %q; want %q", "pgvector", got, want)
	}
}

func TestEnableSQLQuoting(t *testing.T) {
	got := enableSQL("uuid-ossp")
	want := `CREATE EXTENSION IF NOT EXISTS "uuid-ossp"`
	if got != want {
		t.Errorf("enableSQL(%q) = %q; want %q", "uuid-ossp", got, want)
	}
}

func TestDisableSQL(t *testing.T) {
	got := disableSQL("pgvector")
	want := `DROP EXTENSION IF EXISTS "pgvector"`
	if got != want {
		t.Errorf("disableSQL(%q) = %q; want %q", "pgvector", got, want)
	}
}

func TestDisableSQLCascade(t *testing.T) {
	got := disableSQLCascade("pgvector")
	want := `DROP EXTENSION IF EXISTS "pgvector" CASCADE`
	if got != want {
		t.Errorf("disableSQLCascade(%q) = %q; want %q", "pgvector", got, want)
	}
}

// mockResult satisfies sql.Result for ExecContext tests.
type mockResult struct{}

func (mockResult) LastInsertId() (int64, error) { return 0, nil }
func (mockResult) RowsAffected() (int64, error) { return 1, nil }

// availableDB returns a mockDB that reports a single extension as available
// (COUNT(*)=1), and succeeds on exec. queryOverride can replace the query func.
func availableDB(available bool) *mockDB {
	count := 0
	if available {
		count = 1
	}
	return &mockDB{
		queryFunc: fakeCountQuery(count),
		execFunc: func(ctx context.Context, query string, args ...any) (sql.Result, error) {
			return mockResult{}, nil
		},
	}
}

// fakeCountQuery returns a QueryContext func that opens a real sql.Rows
// containing a single integer via the fakedriver.
func fakeCountQuery(count int) func(context.Context, string, ...any) (*sql.Rows, error) {
	return func(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
		db, err := sql.Open("fakeext", fmt.Sprintf("%d", count))
		if err != nil {
			return nil, err
		}
		return db.QueryContext(ctx, "SELECT 1")
	}
}

// --- Enable behavior tests ---

func TestEnableSuccess(t *testing.T) {
	var execSQL string
	db := availableDB(true)
	db.execFunc = func(ctx context.Context, query string, args ...any) (sql.Result, error) {
		execSQL = query
		return mockResult{}, nil
	}

	svc := NewService(db)
	if err := svc.Enable(context.Background(), "pgvector"); err != nil {
		t.Fatalf("Enable: %v", err)
	}
	if !strings.Contains(execSQL, `"pgvector"`) {
		t.Errorf("expected quoted extension name in SQL, got %q", execSQL)
	}
}

func TestEnableInvalidName(t *testing.T) {
	svc := NewService(availableDB(true))
	err := svc.Enable(context.Background(), "bad name!")
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "invalid characters") {
		t.Errorf("error = %q; want validation error", err)
	}
}

func TestEnableNotAvailable(t *testing.T) {
	svc := NewService(availableDB(false))
	err := svc.Enable(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected not-available error")
	}
	if !strings.Contains(err.Error(), "not available") {
		t.Errorf("error = %q; want not-available error", err)
	}
}

func TestEnableExecError(t *testing.T) {
	db := availableDB(true)
	db.execFunc = func(ctx context.Context, query string, args ...any) (sql.Result, error) {
		return nil, fmt.Errorf("permission denied")
	}

	svc := NewService(db)
	err := svc.Enable(context.Background(), "pgvector")
	if err == nil {
		t.Fatal("expected exec error")
	}
	if !strings.Contains(err.Error(), "permission denied") {
		t.Errorf("error = %q; want permission denied", err)
	}
}

// --- Disable behavior tests ---

func TestDisableSuccess(t *testing.T) {
	var execSQL string
	db := &mockDB{
		execFunc: func(ctx context.Context, query string, args ...any) (sql.Result, error) {
			execSQL = query
			return mockResult{}, nil
		},
	}

	svc := NewService(db)
	if err := svc.Disable(context.Background(), "pgvector"); err != nil {
		t.Fatalf("Disable: %v", err)
	}
	if !strings.Contains(execSQL, "DROP EXTENSION") {
		t.Errorf("expected DROP EXTENSION, got %q", execSQL)
	}
}

func TestDisableInvalidName(t *testing.T) {
	db := &mockDB{}
	svc := NewService(db)
	err := svc.Disable(context.Background(), "")
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestDisableDependencyError(t *testing.T) {
	db := &mockDB{
		execFunc: func(ctx context.Context, query string, args ...any) (sql.Result, error) {
			return nil, fmt.Errorf(`cannot drop extension "pgvector" because other objects depends on it`)
		},
	}

	svc := NewService(db)
	err := svc.Disable(context.Background(), "pgvector")
	if err == nil {
		t.Fatal("expected dependency error")
	}
	if !strings.Contains(err.Error(), "dependent objects") {
		t.Errorf("error = %q; want dependency message", err)
	}
	if !strings.Contains(err.Error(), "cascade") {
		t.Errorf("error = %q; want cascade suggestion", err)
	}
}

func TestDisableExecError(t *testing.T) {
	db := &mockDB{
		execFunc: func(ctx context.Context, query string, args ...any) (sql.Result, error) {
			return nil, fmt.Errorf("connection refused")
		},
	}

	svc := NewService(db)
	err := svc.Disable(context.Background(), "pgvector")
	if err == nil {
		t.Fatal("expected exec error")
	}
	if !strings.Contains(err.Error(), "connection refused") {
		t.Errorf("error = %q; want connection refused", err)
	}
}

// --- isDependencyError tests ---

func TestIsDependencyError(t *testing.T) {
	tests := []struct {
		msg  string
		want bool
	}{
		{"cannot drop extension because other objects depends on it", true},
		{"dependency check failed", true},
		{"permission denied", false},
		{"connection refused", false},
	}
	for _, tc := range tests {
		got := isDependencyError(fmt.Errorf("%s", tc.msg))
		if got != tc.want {
			t.Errorf("isDependencyError(%q) = %v; want %v", tc.msg, got, tc.want)
		}
	}
}

// --- Service.List tests ---

func TestServiceListQueryError(t *testing.T) {
	db := &mockDB{
		queryFunc: func(_ context.Context, _ string, _ ...any) (*sql.Rows, error) {
			return nil, fmt.Errorf("connection refused")
		},
	}
	_, err := NewService(db).List(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "querying extensions") {
		t.Errorf("error = %q; want 'querying extensions'", err)
	}
}

func TestServiceListRowMapping(t *testing.T) {
	rows, err := openFakeListRows([][]driver.Value{
		{"pgvector", "0.5.1", "vector similarity search", "0.5.1"}, // installed
		{"pg_trgm", "1.6", "trigram text search", nil},             // available, not installed
	})
	if err != nil {
		t.Fatalf("fakeListRows: %v", err)
	}

	db := &mockDB{
		queryFunc: func(_ context.Context, _ string, _ ...any) (*sql.Rows, error) {
			return rows, nil
		},
	}

	exts, err := NewService(db).List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(exts) != 2 {
		t.Fatalf("expected 2 extensions, got %d", len(exts))
	}

	// pgvector — installed
	pg := exts[0]
	if pg.Name != "pgvector" || !pg.Installed || !pg.Available || pg.InstalledVersion != "0.5.1" {
		t.Errorf("pgvector = %+v", pg)
	}

	// pg_trgm — available but not installed
	trgm := exts[1]
	if trgm.Name != "pg_trgm" || trgm.Installed || trgm.InstalledVersion != "" {
		t.Errorf("pg_trgm = %+v", trgm)
	}
}

// --- Fake database/sql driver for returning *sql.Rows with a count ---

func init() {
	sql.Register("fakeext", &fakeDriver{})
	sql.Register("fakeextlist", &fakeListDriver{})
}

type fakeDriver struct{}

func (d *fakeDriver) Open(dsn string) (driver.Conn, error) {
	var count int
	fmt.Sscanf(dsn, "%d", &count)
	return &fakeConn{count: count}, nil
}

type fakeConn struct{ count int }

func (c *fakeConn) Prepare(query string) (driver.Stmt, error) {
	return &fakeStmt{count: c.count}, nil
}
func (c *fakeConn) Close() error              { return nil }
func (c *fakeConn) Begin() (driver.Tx, error) { return nil, fmt.Errorf("not supported") }

type fakeStmt struct{ count int }

func (s *fakeStmt) Close() error                                    { return nil }
func (s *fakeStmt) NumInput() int                                   { return 0 }
func (s *fakeStmt) Exec(args []driver.Value) (driver.Result, error) { return mockResult{}, nil }
func (s *fakeStmt) Query(args []driver.Value) (driver.Rows, error) {
	return &fakeRows{count: s.count, done: false}, nil
}

type fakeRows struct {
	count int
	done  bool
}

func (r *fakeRows) Columns() []string { return []string{"count"} }
func (r *fakeRows) Close() error      { return nil }
func (r *fakeRows) Next(dest []driver.Value) error {
	if r.done {
		return fmt.Errorf("EOF")
	}
	r.done = true
	dest[0] = int64(r.count)
	return nil
}

// fakeListDriver backs TestServiceListRowMapping: it serves pre-loaded multi-column rows.

var (
	fakeListMu   sync.Mutex
	fakeListData [][]driver.Value
)

type fakeListDriver struct{}

func (d *fakeListDriver) Open(_ string) (driver.Conn, error) { return &fakeListConn{}, nil }

type fakeListConn struct{}

func (c *fakeListConn) Prepare(_ string) (driver.Stmt, error) { return &fakeListStmt{}, nil }
func (c *fakeListConn) Close() error                          { return nil }
func (c *fakeListConn) Begin() (driver.Tx, error)             { return nil, fmt.Errorf("not supported") }

type fakeListStmt struct{}

func (s *fakeListStmt) Close() error                                 { return nil }
func (s *fakeListStmt) NumInput() int                                { return 0 }
func (s *fakeListStmt) Exec(_ []driver.Value) (driver.Result, error) { return mockResult{}, nil }
func (s *fakeListStmt) Query(_ []driver.Value) (driver.Rows, error) {
	fakeListMu.Lock()
	data := fakeListData
	fakeListData = nil
	fakeListMu.Unlock()
	return &fakeListDriverRows{rows: data}, nil
}

type fakeListDriverRows struct {
	rows [][]driver.Value
	pos  int
}

func (r *fakeListDriverRows) Columns() []string {
	return []string{"name", "default_version", "comment", "extversion"}
}
func (r *fakeListDriverRows) Close() error { return nil }
func (r *fakeListDriverRows) Next(dest []driver.Value) error {
	if r.pos >= len(r.rows) {
		return io.EOF
	}
	copy(dest, r.rows[r.pos])
	r.pos++
	return nil
}

// openFakeListRows opens a *sql.DB via "fakeextlist" pre-loaded with the given rows,
// then queries it to obtain a *sql.Rows value for injection into mockDB.queryFunc.
func openFakeListRows(data [][]driver.Value) (*sql.Rows, error) {
	fakeListMu.Lock()
	fakeListData = data
	fakeListMu.Unlock()

	db, err := sql.Open("fakeextlist", "")
	if err != nil {
		return nil, err
	}
	return db.QueryContext(context.Background(), "SELECT 1")
}
