//go:build integration

package branching

import (
	"context"
	"os"
	"os/exec"
	"testing"

	"github.com/allyourbase/ayb/internal/migrations"
	"github.com/allyourbase/ayb/internal/schemadiff"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/jackc/pgx/v5/pgxpool"
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

func setupBranchingIntegrationSchema(t *testing.T, ctx context.Context) {
	t.Helper()

	if _, err := sharedPG.Pool.Exec(ctx, "DROP SCHEMA public CASCADE; CREATE SCHEMA public"); err != nil {
		t.Fatalf("resetting schema: %v", err)
	}

	if _, err := sharedPG.Pool.Exec(ctx, "CREATE TABLE users (id SERIAL PRIMARY KEY, name TEXT NOT NULL)"); err != nil {
		t.Fatalf("creating source schema: %v", err)
	}

	runner := migrations.NewRunner(sharedPG.Pool, testutil.DiscardLogger())
	if err := runner.Bootstrap(ctx); err != nil {
		t.Fatalf("bootstrapping migrations: %v", err)
	}
	if _, err := runner.Run(ctx); err != nil {
		t.Fatalf("running migrations: %v", err)
	}
}

func requirePgDumpTools(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("pg_dump"); err != nil {
		t.Skip("pg_dump not available, skipping branching lifecycle integration test")
	}
	if _, err := exec.LookPath("psql"); err != nil {
		t.Skip("psql not available, skipping branching lifecycle integration test")
	}
}

func branchDatabaseExists(ctx context.Context, name string) bool {
	var exists bool
	if err := sharedPG.Pool.QueryRow(ctx,
		"SELECT EXISTS (SELECT 1 FROM pg_database WHERE datname = $1)", name,
	).Scan(&exists); err != nil {
		panic(err)
	}
	return exists
}

func closePool(pool *pgxpool.Pool) {
	if pool != nil {
		pool.Close()
	}
}

func TestManagerLifecycle_CreateDiffAndDelete(t *testing.T) {
	requirePgDumpTools(t)

	ctx := context.Background()
	setupBranchingIntegrationSchema(t, ctx)

	logger := testutil.DiscardLogger()
	repo := NewPgRepo(sharedPG.Pool)
	mgr := NewManager(sharedPG.Pool, repo, logger, ManagerConfig{})

	name := "full-lifecycle-test"
	rec, err := mgr.Create(ctx, name, sharedPG.ConnString)
	testutil.NoError(t, err)
	testutil.Equal(t, StatusReady, rec.Status)
	testutil.Equal(t, name, rec.Name)
	testutil.True(t, branchDatabaseExists(ctx, rec.BranchDatabase))

	sourcePool := sharedPG.Pool
	branchURL, err := ReplaceDatabaseInURL(sharedPG.ConnString, rec.BranchDatabase)
	testutil.NoError(t, err)

	branchPool, err := pgxpool.New(ctx, branchURL)
	testutil.NoError(t, err)
	defer closePool(branchPool)

	beforeSource, err := schemadiff.TakeSnapshot(ctx, sourcePool)
	testutil.NoError(t, err)
	beforeBranch, err := schemadiff.TakeSnapshot(ctx, branchPool)
	testutil.NoError(t, err)
	beforeChanges := schemadiff.Diff(beforeSource, beforeBranch)
	testutil.Equal(t, 0, len(beforeChanges))

	_, err = sharedPG.Pool.Exec(ctx, "ALTER TABLE users ADD COLUMN IF NOT EXISTS profile JSONB")
	testutil.NoError(t, err)

	afterSource, err := schemadiff.TakeSnapshot(ctx, sourcePool)
	testutil.NoError(t, err)
	afterChanges := schemadiff.Diff(afterSource, beforeBranch)
	testutil.True(t, len(afterChanges) > 0)

	err = mgr.Delete(ctx, name)
	testutil.NoError(t, err)
	testutil.False(t, branchDatabaseExists(ctx, rec.BranchDatabase))
	record, err := repo.Get(ctx, name)
	testutil.NoError(t, err)
	testutil.Nil(t, record)
}

func TestManagerCreateCloneFailureLeavesFailedMetadata(t *testing.T) {
	ctx := context.Background()
	setupBranchingIntegrationSchema(t, ctx)

	logger := testutil.DiscardLogger()
	repo := NewPgRepo(sharedPG.Pool)
	mgr := NewManager(sharedPG.Pool, repo, logger, ManagerConfig{
		PgDumpPath: "/tmp/does-not-exist",
		PsqlPath:   "/tmp/does-not-exist",
	})

	name := "failure-recovery"
	_, err := mgr.Create(ctx, name, sharedPG.ConnString)
	testutil.ErrorContains(t, err, "cloning database")

	record, err := repo.Get(ctx, name)
	testutil.NoError(t, err)
	testutil.NotNil(t, record)
	testutil.Equal(t, StatusFailed, record.Status)
	testutil.True(t, record.ErrorMessage != "")
	testutil.False(t, branchDatabaseExists(ctx, record.BranchDatabase))
}
