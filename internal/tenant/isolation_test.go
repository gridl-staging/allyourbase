//go:build integration

package tenant

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/testutil"
)

func schemaExists(t *testing.T, ctx context.Context, name string) bool {
	t.Helper()
	var exists bool
	err := sharedPG.Pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM pg_namespace WHERE nspname = $1)`,
		name,
	).Scan(&exists)
	testutil.NoError(t, err)
	return exists
}

func TestSchemaProvisionerLifecycle(t *testing.T) {
	setupTenantTestDB(t)
	ctx := context.Background()
	provisioner := NewSchemaProvisioner(sharedPG.Pool, testutil.DiscardLogger())
	slug := fmt.Sprintf("tenant-schema-%d", time.Now().UnixNano())

	testutil.NoError(t, provisioner.ProvisionSchema(ctx, slug))
	testutil.True(t, schemaExists(t, ctx, slug), "schema should exist after provision")

	// idempotent
	testutil.NoError(t, provisioner.ProvisionSchema(ctx, slug))
	testutil.True(t, schemaExists(t, ctx, slug), "schema should still exist after second provision")

	// Verify RLS role can access schema.
	var hasUsage bool
	err := sharedPG.Pool.QueryRow(ctx,
		`SELECT has_schema_privilege('ayb_authenticated', $1, 'USAGE')`,
		slug,
	).Scan(&hasUsage)
	testutil.NoError(t, err)
	testutil.True(t, hasUsage, "ayb_authenticated should have usage on schema")

	testutil.NoError(t, provisioner.DropSchema(ctx, slug))
	testutil.False(t, schemaExists(t, ctx, slug), "schema should be removed after drop")

	// idempotent drop
	testutil.NoError(t, provisioner.DropSchema(ctx, slug))
}
