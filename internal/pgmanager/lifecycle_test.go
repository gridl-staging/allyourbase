package pgmanager

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/testutil"
)

type fakeRow struct {
	scan func(dest ...any) error
}

func (r fakeRow) Scan(dest ...any) error {
	return r.scan(dest...)
}

type fakeResult struct{}

func (fakeResult) LastInsertId() (int64, error) { return 0, nil }
func (fakeResult) RowsAffected() (int64, error) { return 0, nil }

type fakeDatabaseClient struct {
	query            string
	queryArgs        []any
	execQuery        string
	execCalled       bool
	closeCalled      bool
	databaseExists   bool
	queryErr         error
	execErr          error
	connectionString string
}

func (db *fakeDatabaseClient) QueryRowContext(_ context.Context, query string, args ...any) rowScanner {
	db.query = query
	db.queryArgs = append([]any(nil), args...)
	return fakeRow{
		scan: func(dest ...any) error {
			if db.queryErr != nil {
				return db.queryErr
			}
			exists, ok := dest[0].(*bool)
			if !ok {
				return sql.ErrNoRows
			}
			*exists = db.databaseExists
			return nil
		},
	}
}

func (db *fakeDatabaseClient) ExecContext(_ context.Context, query string, _ ...any) (sql.Result, error) {
	db.execCalled = true
	db.execQuery = query
	if db.execErr != nil {
		return nil, db.execErr
	}
	return fakeResult{}, nil
}

func (db *fakeDatabaseClient) Close() error {
	db.closeCalled = true
	return nil
}

// --- initdb skips when already initialized ---

func TestRunInitDBSkipsWhenInitialized(t *testing.T) {
	t.Parallel()
	dataDir := t.TempDir()

	// Create PG_VERSION to simulate already-initialized data dir.
	testutil.NoError(t, os.WriteFile(filepath.Join(dataDir, "PG_VERSION"), []byte("16"), 0o644))

	// Should succeed without actually running initdb (no real binary needed).
	err := runInitDB(context.Background(), "/nonexistent/bin", dataDir, testutil.DiscardLogger())
	testutil.NoError(t, err)
}

// --- pg_ctl command argument verification ---

func TestStartPostgresArgs(t *testing.T) {
	t.Parallel()
	// We can't run pg_ctl without real binaries, but we can verify the function
	// creates the right command by checking the error message contains the binary path.
	binDir := t.TempDir()
	dataDir := t.TempDir()

	err := startPostgres(context.Background(), binDir, dataDir, 15432, testutil.DiscardLogger())
	testutil.True(t, err != nil, "expected error without real pg_ctl binary")
	testutil.Contains(t, err.Error(), "pg_ctl start failed")
}

func TestStartPostgresIncludesServerLogFile(t *testing.T) {
	t.Parallel()

	binDir := t.TempDir()
	dataDir := t.TempDir()
	argsFile := filepath.Join(t.TempDir(), "pgctl-args.txt")
	pgCtlDir := filepath.Join(binDir, "bin")
	testutil.NoError(t, os.MkdirAll(pgCtlDir, 0o755))

	script := filepath.Join(pgCtlDir, "pg_ctl")
	scriptBody := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"" + argsFile + "\"\n"
	testutil.NoError(t, os.WriteFile(script, []byte(scriptBody), 0o755))

	err := startPostgres(context.Background(), binDir, dataDir, 15432, testutil.DiscardLogger())
	testutil.NoError(t, err)

	args, readErr := os.ReadFile(argsFile)
	testutil.NoError(t, readErr)
	testutil.Contains(t, string(args), "-l")
	testutil.Contains(t, string(args), filepath.Join(dataDir, pgCtlStartLogName))
}

func TestEnsureManagedDatabaseCreatesMissingDatabase(t *testing.T) {
	originalOpen := openDatabaseClient
	defer func() { openDatabaseClient = originalOpen }()

	client := &fakeDatabaseClient{}
	openDatabaseClient = func(_ context.Context, connURL string) (databaseClient, error) {
		client.connectionString = connURL
		return client, nil
	}

	err := ensureManagedDatabase(context.Background(), 15432, testutil.DiscardLogger())
	testutil.NoError(t, err)
	testutil.Equal(t, managedConnURL(15432, "postgres"), client.connectionString)
	testutil.Contains(t, client.query, "SELECT EXISTS")
	testutil.Equal(t, dbName, client.queryArgs[0])
	testutil.True(t, client.execCalled, "expected CREATE DATABASE to run when database is missing")
	testutil.Equal(t, `CREATE DATABASE "ayb"`, client.execQuery)
	testutil.True(t, client.closeCalled, "expected admin connection to close")
}

func TestEnsureManagedDatabaseSkipsExistingDatabase(t *testing.T) {
	originalOpen := openDatabaseClient
	defer func() { openDatabaseClient = originalOpen }()

	client := &fakeDatabaseClient{databaseExists: true}
	openDatabaseClient = func(_ context.Context, _ string) (databaseClient, error) {
		return client, nil
	}

	err := ensureManagedDatabase(context.Background(), 15432, testutil.DiscardLogger())
	testutil.NoError(t, err)
	testutil.True(t, !client.execCalled, "CREATE DATABASE should be skipped when database already exists")
	testutil.True(t, client.closeCalled, "expected admin connection to close")
}

func TestStopPostgresArgs(t *testing.T) {
	t.Parallel()
	binDir := t.TempDir()
	dataDir := t.TempDir()

	err := stopPostgres(binDir, dataDir, testutil.DiscardLogger())
	testutil.True(t, err != nil, "expected error without real pg_ctl binary")
	testutil.Contains(t, err.Error(), "pg_ctl stop failed")
}

func TestRunInitDBArgs(t *testing.T) {
	t.Parallel()
	binDir := t.TempDir()
	dataDir := t.TempDir()

	// No PG_VERSION exists, so initdb should be attempted and fail (no binary).
	err := runInitDB(context.Background(), binDir, dataDir, testutil.DiscardLogger())
	testutil.True(t, err != nil, "expected error without real initdb binary")
	testutil.Contains(t, err.Error(), "initdb failed")
}

// --- extension initialization ---

func TestInitExtensionsEmptyList(t *testing.T) {
	t.Parallel()
	// Empty extension list should be a no-op (doesn't even try to connect).
	err := initExtensions(context.Background(), "postgresql://invalid:5432/db", nil, testutil.DiscardLogger())
	testutil.NoError(t, err)
}

func TestStartPostgresDoesNotHangWhenPgCtlChildKeepsLogFDOpen(t *testing.T) {
	const helperEnv = "AYB_PGMANAGER_START_HELPER"
	if os.Getenv(helperEnv) == "1" {
		binDir := os.Getenv("AYB_PGMANAGER_BIN_DIR")
		dataDir := os.Getenv("AYB_PGMANAGER_DATA_DIR")
		err := startPostgres(context.Background(), binDir, dataDir, 15432, testutil.DiscardLogger())
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		os.Exit(0)
	}

	binDir := filepath.Join(t.TempDir(), "bin")
	dataDir := t.TempDir()
	testutil.NoError(t, os.MkdirAll(filepath.Join(binDir, "bin"), 0o755))

	childPIDFile := filepath.Join(t.TempDir(), "fake-pg-child.pid")
	pgCtlScript := fmt.Sprintf(`#!/bin/sh
set -eu
if [ "$1" = "start" ]; then
  sh -c 'sleep 30' &
  child=$!
  printf '%%s\n' "$child" > %q
  printf 'server started\n'
  exit 0
fi
printf 'server stopped\n'
`, childPIDFile)
	pgCtlPath := filepath.Join(binDir, "bin", "pg_ctl")
	testutil.NoError(t, os.WriteFile(pgCtlPath, []byte(pgCtlScript), 0o755))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=TestStartPostgresDoesNotHangWhenPgCtlChildKeepsLogFDOpen")
	cmd.Env = append(os.Environ(),
		helperEnv+"=1",
		"AYB_PGMANAGER_BIN_DIR="+binDir,
		"AYB_PGMANAGER_DATA_DIR="+dataDir,
	)
	output, err := cmd.CombinedOutput()
	t.Cleanup(func() {
		data, readErr := os.ReadFile(childPIDFile)
		if readErr != nil {
			return
		}
		pid, parseErr := strconv.Atoi(strings.TrimSpace(string(data)))
		if parseErr != nil {
			return
		}
		proc, findErr := os.FindProcess(pid)
		if findErr == nil {
			_ = proc.Kill()
		}
	})
	if err != nil {
		t.Fatalf("startPostgres helper failed: %v\n%s", err, output)
	}
	testutil.True(t, ctx.Err() == nil, "startPostgres helper timed out")
	startLogPath := filepath.Join(dataDir, pgCtlStartLogName)
	logBytes, readErr := os.ReadFile(startLogPath)
	testutil.NoError(t, readErr)
	testutil.Contains(t, string(logBytes), "server started")
}
