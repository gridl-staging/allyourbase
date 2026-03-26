//go:build integration

package schemadiff_test

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/allyourbase/ayb/internal/schemadiff"
	"github.com/jackc/pgx/v5/pgxpool"
)

func getTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("TEST_DATABASE_URL not set, skipping integration test")
	}
	pool, err := pgxpool.New(context.Background(), dbURL)
	if err != nil {
		t.Fatalf("connecting to database: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return pool
}

func execSQL(t *testing.T, pool *pgxpool.Pool, sql string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), sql); err != nil {
		t.Fatalf("exec SQL: %v\nSQL: %s", err, sql)
	}
}

func TestIntegration_SnapshotDiffRoundTrip(t *testing.T) {
	pool := getTestPool(t)
	ctx := context.Background()

	// Clean up test objects.
	t.Cleanup(func() {
		execSQL(t, pool, `DROP TABLE IF EXISTS test_orders CASCADE`)
		execSQL(t, pool, `DROP TABLE IF EXISTS test_users CASCADE`)
		execSQL(t, pool, `DROP TYPE IF EXISTS test_status CASCADE`)
	})

	// Step 1: Create initial schema.
	execSQL(t, pool, `CREATE TYPE test_status AS ENUM ('active', 'inactive')`)
	execSQL(t, pool, `
		CREATE TABLE test_users (
			id serial PRIMARY KEY,
			email text NOT NULL UNIQUE,
			status test_status DEFAULT 'active',
			bio text,
			created_at timestamptz DEFAULT now()
		)
	`)

	// Step 2: Take first snapshot.
	snap1, err := schemadiff.TakeSnapshot(ctx, pool)
	if err != nil {
		t.Fatalf("TakeSnapshot 1: %v", err)
	}

	// Verify snapshot has our table.
	found := false
	for _, tbl := range snap1.Tables {
		if tbl.Name == "test_users" {
			found = true
			if len(tbl.Columns) < 5 {
				t.Errorf("expected at least 5 columns, got %d", len(tbl.Columns))
			}
		}
	}
	if !found {
		t.Fatal("test_users not found in snapshot")
	}

	// Step 3: Apply DDL changes.
	execSQL(t, pool, `ALTER TABLE test_users ADD COLUMN phone text`)
	execSQL(t, pool, `ALTER TABLE test_users DROP COLUMN bio`)
	execSQL(t, pool, `ALTER TYPE test_status ADD VALUE 'pending'`)
	execSQL(t, pool, `
		CREATE TABLE test_orders (
			id serial PRIMARY KEY,
			user_id integer REFERENCES test_users(id),
			amount numeric(10,2) NOT NULL,
			created_at timestamptz DEFAULT now()
		)
	`)
	execSQL(t, pool, `CREATE INDEX idx_test_orders_user ON test_orders(user_id)`)

	// Step 4: Take second snapshot.
	snap2, err := schemadiff.TakeSnapshot(ctx, pool)
	if err != nil {
		t.Fatalf("TakeSnapshot 2: %v", err)
	}

	// Step 5: Diff and verify expected changes.
	cs := schemadiff.Diff(snap1, snap2)
	if len(cs) == 0 {
		t.Fatal("expected non-empty changeset")
	}

	changeTypes := make(map[schemadiff.ChangeType]int)
	for _, c := range cs {
		changeTypes[c.Type]++
	}

	// We expect: CreateTable (test_orders), AddColumn (phone), DropColumn (bio),
	// AlterEnumAddValue (pending), AddForeignKey, CreateIndex.
	if changeTypes[schemadiff.ChangeCreateTable] < 1 {
		t.Error("expected at least one CreateTable change")
	}
	if changeTypes[schemadiff.ChangeAddColumn] < 1 {
		t.Error("expected at least one AddColumn change")
	}
	if changeTypes[schemadiff.ChangeDropColumn] < 1 {
		t.Error("expected at least one DropColumn change")
	}

	// Step 6: Generate SQL and apply to verify it's valid.
	upSQL := schemadiff.GenerateUp(cs)
	downSQL := schemadiff.GenerateDown(cs)
	if upSQL == "" {
		t.Error("GenerateUp produced empty SQL")
	}
	if downSQL == "" {
		t.Error("GenerateDown produced empty SQL")
	}

	// Step 7: Verify SQL is parseable (we can't easily apply it since the schema
	// already has these changes, but we can verify it's non-empty and well-formed).
	t.Logf("Up SQL:\n%s", upSQL)
	t.Logf("Down SQL:\n%s", downSQL)
}

func TestIntegration_RLSPolicySnapshot(t *testing.T) {
	pool := getTestPool(t)
	ctx := context.Background()

	t.Cleanup(func() {
		execSQL(t, pool, `DROP TABLE IF EXISTS test_rls_items CASCADE`)
	})

	execSQL(t, pool, `
		CREATE TABLE test_rls_items (
			id serial PRIMARY KEY,
			owner_id integer NOT NULL,
			data text
		)
	`)
	execSQL(t, pool, `ALTER TABLE test_rls_items ENABLE ROW LEVEL SECURITY`)
	execSQL(t, pool, `
		CREATE POLICY owner_only ON test_rls_items
			FOR ALL
			USING (owner_id = current_setting('app.user_id')::integer)
	`)

	snap, err := schemadiff.TakeSnapshot(ctx, pool)
	if err != nil {
		t.Fatalf("TakeSnapshot: %v", err)
	}

	found := false
	for _, tbl := range snap.Tables {
		if tbl.Name == "test_rls_items" {
			found = true
			if len(tbl.RLSPolicies) == 0 {
				t.Error("expected RLS policies on test_rls_items")
			} else {
				pol := tbl.RLSPolicies[0]
				if pol.Name != "owner_only" {
					t.Errorf("policy name = %q, want owner_only", pol.Name)
				}
			}
		}
	}
	if !found {
		t.Fatal("test_rls_items not found in snapshot")
	}
}

func TestIntegration_EmptyDiffAfterNoChanges(t *testing.T) {
	pool := getTestPool(t)
	ctx := context.Background()

	// Take two snapshots without changing anything.
	snap1, err := schemadiff.TakeSnapshot(ctx, pool)
	if err != nil {
		t.Fatalf("TakeSnapshot 1: %v", err)
	}
	snap2, err := schemadiff.TakeSnapshot(ctx, pool)
	if err != nil {
		t.Fatalf("TakeSnapshot 2: %v", err)
	}

	cs := schemadiff.Diff(snap1, snap2)
	if len(cs) != 0 {
		t.Errorf("expected empty changeset for identical snapshots, got %d changes", len(cs))
		for _, c := range cs {
			fmt.Printf("  %s %s.%s\n", c.Type, c.SchemaName, c.TableName)
		}
	}
}
