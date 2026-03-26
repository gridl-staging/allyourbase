//go:build integration

package tenant

import (
	"context"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

// newMigrationTestService creates a MigrationService backed by the shared test DB.
func newMigrationTestService() *MigrationService {
	return NewMigrationService(sharedPG.Pool, testutil.DiscardLogger())
}

// insertTestUser inserts a minimal user row and returns its UUID string.
func insertTestUser(t *testing.T, email string) string {
	t.Helper()
	ctx := context.Background()
	var id string
	err := sharedPG.Pool.QueryRow(ctx,
		`INSERT INTO _ayb_users (email, password_hash) VALUES ($1, 'x') RETURNING id`,
		email,
	).Scan(&id)
	testutil.NoError(t, err)
	return id
}

// insertTestApp inserts a minimal _ayb_apps row (no tenant_id) and returns its UUID.
func insertTestApp(t *testing.T, name, ownerID string) string {
	t.Helper()
	ctx := context.Background()
	var id string
	err := sharedPG.Pool.QueryRow(ctx,
		`INSERT INTO _ayb_apps (name, description, owner_user_id) VALUES ($1, '', $2) RETURNING id`,
		name, ownerID,
	).Scan(&id)
	testutil.NoError(t, err)
	return id
}

// insertOrphanApp drops the owner FK temporarily in the isolated test schema
// so migration behavior for legacy-orphaned owner ids can be exercised.
func insertOrphanApp(t *testing.T, name, ownerID string) {
	t.Helper()
	ctx := context.Background()

	_, err := sharedPG.Pool.Exec(ctx, `
DO $$
BEGIN
	IF EXISTS (
		SELECT 1
		FROM pg_constraint
		WHERE conrelid = '_ayb_apps'::regclass
		  AND conname = '_ayb_apps_owner_user_id_fkey'
	) THEN
		ALTER TABLE _ayb_apps DROP CONSTRAINT _ayb_apps_owner_user_id_fkey;
	END IF;
END $$;`)
	testutil.NoError(t, err)

	_, err = sharedPG.Pool.Exec(ctx,
		`INSERT INTO _ayb_apps (name, description, owner_user_id) VALUES ($1, '', $2)`,
		name,
		ownerID,
	)
	testutil.NoError(t, err)
}

// setAppTenantID directly sets the tenant_id on an app (simulates partial migration).
func setAppTenantID(t *testing.T, appID, tenantID string) {
	t.Helper()
	ctx := context.Background()
	_, err := sharedPG.Pool.Exec(ctx,
		`UPDATE _ayb_apps SET tenant_id = $1::uuid WHERE id = $2::uuid`,
		tenantID, appID,
	)
	testutil.NoError(t, err)
}

// --- Full lifecycle: dry-run → apply → consistency check ---

func TestMigration_DryRunThenApplyThenConsistency(t *testing.T) {
	setupTenantTestDB(t)
	ctx := context.Background()
	svc := newMigrationTestService()
	opts := MigrationOpts{BatchSize: 10}

	// Seed: two users each with two apps.
	alice := insertTestUser(t, "alice@example.com")
	bob := insertTestUser(t, "bob@example.com")
	insertTestApp(t, "alice-app-1", alice)
	insertTestApp(t, "alice-app-2", alice)
	insertTestApp(t, "bob-app-1", bob)
	insertTestApp(t, "bob-app-2", bob)

	// Dry-run must preview 2 create actions and 4 app assignments.
	report, err := svc.MigrationDryRun(ctx, opts)
	testutil.NoError(t, err)
	testutil.Equal(t, 2, report.Summary.ExaminedGroups)
	testutil.Equal(t, 2, report.Summary.CreatedTenants)
	testutil.Equal(t, 4, report.Summary.AssignedApps)
	testutil.Equal(t, 0, report.Summary.SkippedGroups)

	// Verify actions are "create".
	for _, g := range report.Groups {
		testutil.Equal(t, dryRunActionCreate, g.Action)
	}

	// Apply migration.
	result, err := svc.MigrateLegacyApps(ctx, opts)
	testutil.NoError(t, err)
	testutil.Equal(t, 2, result.ExaminedGroups)
	testutil.Equal(t, 2, result.CreatedTenants)
	testutil.Equal(t, 4, result.AssignedApps)
	testutil.Equal(t, 2, result.CreatedMemberships)
	testutil.Equal(t, 0, result.ErroredGroups)

	// Post-apply consistency check must pass.
	consistency, err := svc.CheckMigrationConsistency(ctx)
	testutil.NoError(t, err)
	testutil.True(t, consistency.Clean, "expected clean consistency after migration")
	testutil.Equal(t, 0, consistency.NullTenantIDApps)
	testutil.Equal(t, 4, consistency.MigratedApps)
	testutil.Equal(t, 4, consistency.TotalApps)
}

// --- Idempotent apply: second run must not duplicate tenants/memberships ---

func TestMigration_IdempotentApply(t *testing.T) {
	setupTenantTestDB(t)
	ctx := context.Background()
	svc := newMigrationTestService()
	opts := MigrationOpts{BatchSize: 10}

	alice := insertTestUser(t, "alice-idem@example.com")
	insertTestApp(t, "idem-app-1", alice)
	insertTestApp(t, "idem-app-2", alice)

	// First apply.
	r1, err := svc.MigrateLegacyApps(ctx, opts)
	testutil.NoError(t, err)
	testutil.Equal(t, 1, r1.CreatedTenants)
	testutil.Equal(t, 2, r1.AssignedApps)

	// Second apply: must reuse tenant, assign 0 new apps (already assigned).
	r2, err := svc.MigrateLegacyApps(ctx, opts)
	testutil.NoError(t, err)
	testutil.Equal(t, 0, r2.CreatedTenants)
	testutil.Equal(t, 1, r2.ReusedTenants)
	testutil.Equal(t, 0, r2.AssignedApps) // already set
	testutil.Equal(t, 0, r2.CreatedMemberships)
	testutil.Equal(t, 0, r2.ErroredGroups)
}

// --- Skip group: owner missing from _ayb_users ---

func TestMigration_SkipsAppWithMissingOwner(t *testing.T) {
	setupTenantTestDB(t)
	ctx := context.Background()
	svc := newMigrationTestService()
	opts := MigrationOpts{}

	// Simulate legacy data drift where an app row exists for a non-existent owner.
	ghostOwner := "00000000-0000-0000-0000-000000000001"
	insertOrphanApp(t, "ghost-app", ghostOwner)

	result, err := svc.MigrateLegacyApps(ctx, opts)
	testutil.NoError(t, err)
	testutil.Equal(t, 1, result.ExaminedGroups)
	testutil.Equal(t, 1, result.SkippedGroups)
	testutil.Equal(t, 0, result.CreatedTenants)
}

// --- Partially migrated dataset: dual-read compatibility ---

func TestMigration_PartiallyMigratedDataset(t *testing.T) {
	setupTenantTestDB(t)
	ctx := context.Background()
	svc := newMigrationTestService()
	opts := MigrationOpts{}

	alice := insertTestUser(t, "alice-partial@example.com")
	appA := insertTestApp(t, "partial-app-a", alice) // will be migrated
	_ = insertTestApp(t, "partial-app-b", alice)     // will also be migrated

	// Manually assign appA to a tenant (simulate partial prior migration).
	tenantSvc := newTestService()
	existingTenant, err := tenantSvc.CreateTenant(ctx, "alice-partial", "alice-partial", "schema", "free", "default", nil, "")
	testutil.NoError(t, err)
	setAppTenantID(t, appA, existingTenant.ID)

	// Consistency check must show 1 null (partial) before full migration.
	c1, err := svc.CheckMigrationConsistency(ctx)
	testutil.NoError(t, err)
	testutil.Equal(t, 1, c1.NullTenantIDApps)
	testutil.True(t, !c1.Clean, "should not be clean before full migration")

	// Apply migration for the remaining unmigrated app.
	result, err := svc.MigrateLegacyApps(ctx, opts)
	testutil.NoError(t, err)
	testutil.Equal(t, 0, result.ErroredGroups)

	// After full migration, consistency must pass.
	c2, err := svc.CheckMigrationConsistency(ctx)
	testutil.NoError(t, err)
	testutil.Equal(t, 0, c2.NullTenantIDApps)
	testutil.True(t, c2.Clean, "should be clean after full migration")
}

// --- Consistency checker detects each violation type independently ---

func TestConsistencyChecker_DetectsNullTenantID(t *testing.T) {
	setupTenantTestDB(t)
	ctx := context.Background()
	svc := newMigrationTestService()

	alice := insertTestUser(t, "alice-nullcheck@example.com")
	insertTestApp(t, "null-check-app", alice) // no tenant_id

	r, err := svc.CheckMigrationConsistency(ctx)
	testutil.NoError(t, err)
	testutil.Equal(t, 1, r.NullTenantIDApps)
	testutil.True(t, !r.Clean)
}

func TestConsistencyChecker_CleanAfterMigration(t *testing.T) {
	setupTenantTestDB(t)
	ctx := context.Background()
	svc := newMigrationTestService()

	alice := insertTestUser(t, "alice-cleancheck@example.com")
	insertTestApp(t, "clean-check-app", alice)

	_, err := svc.MigrateLegacyApps(ctx, MigrationOpts{})
	testutil.NoError(t, err)

	r, err := svc.CheckMigrationConsistency(ctx)
	testutil.NoError(t, err)
	testutil.True(t, r.Clean)
	testutil.Equal(t, 0, r.NullTenantIDApps)
	testutil.Equal(t, 0, r.DanglingTenantIDApps)
	testutil.Equal(t, 0, r.OrphanTenants)
}

// --- BatchSize and MaxItems controls ---

func TestMigration_MaxItemsLimit(t *testing.T) {
	setupTenantTestDB(t)
	ctx := context.Background()
	svc := newMigrationTestService()

	// Insert 3 owners each with 1 app.
	for i, email := range []string{"u1@x.com", "u2@x.com", "u3@x.com"} {
		uid := insertTestUser(t, email)
		insertTestApp(t, "limited-app-"+string(rune('a'+i)), uid)
	}

	// Only process 2 groups.
	result, err := svc.MigrateLegacyApps(ctx, MigrationOpts{MaxItems: 2})
	testutil.NoError(t, err)
	testutil.Equal(t, 2, result.ExaminedGroups)
	testutil.Equal(t, 2, result.CreatedTenants)
}
