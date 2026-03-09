//go:build integration

package schema_test

import (
	"context"
	"os"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
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

// resetDB drops and recreates the public schema (and app schema if present)
// so each test starts with a clean database.
func resetDB(t *testing.T, ctx context.Context) {
	t.Helper()
	_, err := sharedPG.Pool.Exec(ctx, "DROP SCHEMA public CASCADE; CREATE SCHEMA public; DROP SCHEMA IF EXISTS app CASCADE")
	if err != nil {
		t.Fatalf("resetting schema: %v", err)
	}
}
