//go:build integration

package auth_test

import (
	"context"
	"testing"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/golang-jwt/jwt/v5"
)

// TestRLSTenantIsolation verifies that RLS policies referencing
// current_setting('ayb.tenant_id', true) correctly isolate data
// between tenants.
func TestRLSTenantIsolation(t *testing.T) {
	ctx := context.Background()
	pool := sharedPG.Pool

	// Reset schema.
	_, err := pool.Exec(ctx, "DROP SCHEMA public CASCADE; CREATE SCHEMA public")
	if err != nil {
		t.Fatalf("resetting schema: %v", err)
	}

	// Create the authenticated role if it doesn't exist.
	_, _ = pool.Exec(ctx, `DO $$ BEGIN
		CREATE ROLE ayb_authenticated NOLOGIN;
		EXCEPTION WHEN duplicate_object THEN NULL;
	END $$`)

	// Grant the authenticated role access to the public schema.
	_, err = pool.Exec(ctx, `GRANT USAGE ON SCHEMA public TO ayb_authenticated`)
	if err != nil {
		t.Fatalf("granting schema usage: %v", err)
	}

	// Create a tenant-scoped table.
	_, err = pool.Exec(ctx, `
		CREATE TABLE items (
			id SERIAL PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			name TEXT NOT NULL
		)
	`)
	if err != nil {
		t.Fatalf("creating table: %v", err)
	}

	// Grant table access to the authenticated role.
	_, err = pool.Exec(ctx, `GRANT SELECT, INSERT ON items TO ayb_authenticated`)
	if err != nil {
		t.Fatalf("granting table access: %v", err)
	}
	_, err = pool.Exec(ctx, `GRANT USAGE, SELECT ON SEQUENCE items_id_seq TO ayb_authenticated`)
	if err != nil {
		t.Fatalf("granting sequence access: %v", err)
	}

	// Enable RLS and create a tenant isolation policy.
	_, err = pool.Exec(ctx, `ALTER TABLE items ENABLE ROW LEVEL SECURITY`)
	if err != nil {
		t.Fatalf("enabling RLS: %v", err)
	}
	_, err = pool.Exec(ctx, `
		CREATE POLICY tenant_isolation ON items
			USING (tenant_id = current_setting('ayb.tenant_id', true))
			WITH CHECK (tenant_id = current_setting('ayb.tenant_id', true))
	`)
	if err != nil {
		t.Fatalf("creating policy: %v", err)
	}

	// Insert data as superuser (bypasses RLS).
	_, err = pool.Exec(ctx, `
		INSERT INTO items (tenant_id, name) VALUES
			('tenant-a', 'Item A1'),
			('tenant-a', 'Item A2'),
			('tenant-b', 'Item B1')
	`)
	if err != nil {
		t.Fatalf("inserting test data: %v", err)
	}

	t.Run("tenant_a_sees_only_own_rows", func(t *testing.T) {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin tx: %v", err)
		}
		defer tx.Rollback(ctx) //nolint:errcheck

		claims := &auth.Claims{
			RegisteredClaims: jwt.RegisteredClaims{Subject: "user-1"},
			Email:            "a@test.com",
			TenantID:         "tenant-a",
		}
		if err := auth.SetRLSContext(ctx, tx, claims); err != nil {
			t.Fatalf("SetRLSContext: %v", err)
		}

		rows, err := tx.Query(ctx, "SELECT name FROM items ORDER BY name")
		if err != nil {
			t.Fatalf("query: %v", err)
		}
		defer rows.Close()

		var names []string
		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err != nil {
				t.Fatalf("scan: %v", err)
			}
			names = append(names, name)
		}

		if len(names) != 2 {
			t.Fatalf("expected 2 rows for tenant-a, got %d: %v", len(names), names)
		}
		if names[0] != "Item A1" || names[1] != "Item A2" {
			t.Fatalf("unexpected rows: %v", names)
		}
	})

	t.Run("tenant_b_sees_only_own_rows", func(t *testing.T) {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin tx: %v", err)
		}
		defer tx.Rollback(ctx) //nolint:errcheck

		claims := &auth.Claims{
			RegisteredClaims: jwt.RegisteredClaims{Subject: "user-2"},
			Email:            "b@test.com",
			TenantID:         "tenant-b",
		}
		if err := auth.SetRLSContext(ctx, tx, claims); err != nil {
			t.Fatalf("SetRLSContext: %v", err)
		}

		var count int
		if err := tx.QueryRow(ctx, "SELECT COUNT(*) FROM items").Scan(&count); err != nil {
			t.Fatalf("count query: %v", err)
		}
		if count != 1 {
			t.Fatalf("expected 1 row for tenant-b, got %d", count)
		}
	})

	t.Run("tenant_a_cannot_insert_as_tenant_b", func(t *testing.T) {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin tx: %v", err)
		}
		defer tx.Rollback(ctx) //nolint:errcheck

		claims := &auth.Claims{
			RegisteredClaims: jwt.RegisteredClaims{Subject: "user-1"},
			Email:            "a@test.com",
			TenantID:         "tenant-a",
		}
		if err := auth.SetRLSContext(ctx, tx, claims); err != nil {
			t.Fatalf("SetRLSContext: %v", err)
		}

		// Try to insert a row with tenant_id = tenant-b while authenticated as tenant-a.
		_, err = tx.Exec(ctx, "INSERT INTO items (tenant_id, name) VALUES ('tenant-b', 'Sneaky')")
		if err == nil {
			t.Fatal("expected RLS violation inserting row for tenant-b as tenant-a, but insert succeeded")
		}
	})

	t.Run("no_tenant_id_sees_no_rows", func(t *testing.T) {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin tx: %v", err)
		}
		defer tx.Rollback(ctx) //nolint:errcheck

		// Claims with empty TenantID — backward compatible, sees nothing
		// (RLS policy requires tenant_id match, empty setting matches nothing).
		claims := &auth.Claims{
			RegisteredClaims: jwt.RegisteredClaims{Subject: "user-1"},
			Email:            "a@test.com",
			TenantID:         "",
		}
		if err := auth.SetRLSContext(ctx, tx, claims); err != nil {
			t.Fatalf("SetRLSContext: %v", err)
		}

		var count int
		if err := tx.QueryRow(ctx, "SELECT COUNT(*) FROM items").Scan(&count); err != nil {
			t.Fatalf("count query: %v", err)
		}
		if count != 0 {
			t.Fatalf("expected 0 rows with empty tenant_id, got %d", count)
		}
	})
}
