//go:build integration

package fdw_test

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"

	"github.com/allyourbase/ayb/internal/fdw"
	"github.com/allyourbase/ayb/internal/migrations"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/allyourbase/ayb/internal/vault"
)

var sharedPG *testutil.PGContainer

func TestMain(m *testing.M) {
	ctx := context.Background()
	pg, cleanup := testutil.StartPostgresForTestMain(ctx)
	sharedPG = pg
	code := m.Run()
	cleanup()
	os.Exit(code)
}

// resetAndMigrate drops/recreates the public schema and runs all migrations
// so the _ayb_fdw_servers tracking table exists.
func resetAndMigrate(t *testing.T, ctx context.Context) {
	t.Helper()
	_, err := sharedPG.Pool.Exec(ctx, "DROP SCHEMA public CASCADE; CREATE SCHEMA public")
	testutil.NoError(t, err)
	r := migrations.NewRunner(sharedPG.Pool, testutil.DiscardLogger())
	testutil.NoError(t, r.Bootstrap(ctx))
	_, err = r.Run(ctx)
	testutil.NoError(t, err)
}

// memVault is a simple in-memory vault store for testing.
type memVault struct {
	mu      sync.Mutex
	secrets map[string][]byte
}

func newMemVault() *memVault {
	return &memVault{secrets: make(map[string][]byte)}
}

func (v *memVault) SetSecret(_ context.Context, name string, value []byte) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.secrets[name] = append([]byte(nil), value...)
	return nil
}

func (v *memVault) GetSecret(_ context.Context, name string) ([]byte, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	val, ok := v.secrets[name]
	if !ok {
		return nil, fmt.Errorf("%w: %s", vault.ErrSecretNotFound, name)
	}
	return val, nil
}

func (v *memVault) DeleteSecret(_ context.Context, name string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if _, ok := v.secrets[name]; !ok {
		return fmt.Errorf("%w: %s", vault.ErrSecretNotFound, name)
	}
	delete(v.secrets, name)
	return nil
}

func TestListServersEmpty(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	svc := fdw.NewService(sharedPG.Pool, newMemVault())

	servers, err := svc.ListServers(ctx)
	testutil.NoError(t, err)
	testutil.Equal(t, 0, len(servers))
}

func TestListForeignTablesEmpty(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	svc := fdw.NewService(sharedPG.Pool, newMemVault())

	tables, err := svc.ListForeignTables(ctx)
	testutil.NoError(t, err)
	testutil.Equal(t, 0, len(tables))
}

func TestCreateServerValidation(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	svc := fdw.NewService(sharedPG.Pool, newMemVault())

	// Invalid identifier.
	err := svc.CreateServer(ctx, fdw.CreateServerOpts{
		Name:    "has space",
		FDWType: "postgres_fdw",
		Options: map[string]string{"host": "localhost", "port": "5432", "dbname": "test"},
	})
	testutil.Error(t, err)
	testutil.Contains(t, err.Error(), "invalid characters")

	// Invalid FDW type.
	err = svc.CreateServer(ctx, fdw.CreateServerOpts{
		Name:    "valid_name",
		FDWType: "invalid_fdw",
		Options: map[string]string{},
	})
	testutil.Error(t, err)
	testutil.Contains(t, err.Error(), "unsupported fdw type")

	// Missing required postgres_fdw options.
	err = svc.CreateServer(ctx, fdw.CreateServerOpts{
		Name:    "valid_name",
		FDWType: "postgres_fdw",
		Options: map[string]string{"host": "localhost"},
	})
	testutil.Error(t, err)
	testutil.Contains(t, err.Error(), "required")
}

func TestCreateServerNilDB(t *testing.T) {
	svc := fdw.NewService(nil, newMemVault())
	err := svc.CreateServer(context.Background(), fdw.CreateServerOpts{
		Name:    "test",
		FDWType: "postgres_fdw",
	})
	testutil.Error(t, err)
	testutil.Contains(t, err.Error(), "not configured")
}

func TestCreateServerNilVault(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	svc := fdw.NewService(sharedPG.Pool, nil)
	err := svc.CreateServer(ctx, fdw.CreateServerOpts{
		Name:    "test",
		FDWType: "postgres_fdw",
		Options: map[string]string{"host": "localhost", "port": "5432", "dbname": "test"},
	})
	testutil.Error(t, err)
	testutil.Contains(t, err.Error(), "not configured")
}

// TestCreateAndDropServerWithFileFDW tests the full lifecycle of a file_fdw server.
// file_fdw may not be available in all Postgres distributions, so this test
// skips if the extension cannot be created.
func TestCreateAndDropServerWithFileFDW(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	v := newMemVault()
	svc := fdw.NewService(sharedPG.Pool, v)

	// file_fdw needs a filename option.
	err := svc.CreateServer(ctx, fdw.CreateServerOpts{
		Name:    "test_file_srv",
		FDWType: "file_fdw",
		Options: map[string]string{"filename": "/dev/null"},
	})
	if err != nil {
		// If file_fdw extension is not available, skip.
		t.Skipf("file_fdw not available: %v", err)
	}

	// Server should appear in list.
	servers, err := svc.ListServers(ctx)
	testutil.NoError(t, err)
	testutil.Equal(t, 1, len(servers))
	testutil.Equal(t, "test_file_srv", servers[0].Name)
	testutil.Equal(t, "file_fdw", servers[0].FDWType)
	testutil.True(t, !servers[0].CreatedAt.IsZero(), "CreatedAt should be populated")

	// Drop the server.
	err = svc.DropServer(ctx, "test_file_srv", false)
	testutil.NoError(t, err)

	// Should be gone.
	servers, err = svc.ListServers(ctx)
	testutil.NoError(t, err)
	testutil.Equal(t, 0, len(servers))
}

// TestCreateServerPostgresFDWMissingUserMapping tests that postgres_fdw requires
// user mapping credentials.
func TestCreateServerPostgresFDWMissingUserMapping(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	svc := fdw.NewService(sharedPG.Pool, newMemVault())

	// postgres_fdw with empty user mapping should fail.
	err := svc.CreateServer(ctx, fdw.CreateServerOpts{
		Name:    "pg_srv",
		FDWType: "postgres_fdw",
		Options: map[string]string{"host": "localhost", "port": "5432", "dbname": "test"},
		UserMapping: fdw.UserMapping{
			User:     "",
			Password: "secret",
		},
	})
	// This will either fail at CreateServer validation or at the user mapping step.
	// Either way it should be an error.
	testutil.Error(t, err)
}

func TestDropServerNonexistent(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	svc := fdw.NewService(sharedPG.Pool, newMemVault())

	// DropServer uses IF EXISTS for the SQL, so it should succeed silently
	// even if the server doesn't exist — the tracking row delete just affects 0 rows.
	err := svc.DropServer(ctx, "nonexistent", false)
	testutil.NoError(t, err)
}

func TestDropServerValidation(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	svc := fdw.NewService(sharedPG.Pool, newMemVault())

	err := svc.DropServer(ctx, "has space", false)
	testutil.Error(t, err)
	testutil.Contains(t, err.Error(), "invalid characters")
}
