//go:build integration

package billing_test

import (
	"context"
	"os"
	"testing"

	"github.com/allyourbase/ayb/internal/billing"
	"github.com/allyourbase/ayb/internal/migrations"
	"github.com/allyourbase/ayb/internal/testutil"
)

var sharedPG *testutil.PGContainer

// testTenantID is a fixed UUID for the test tenant created in resetAndMigrate.
const testTenantID = "00000000-0000-0000-0000-000000000001"
const testTenantID2 = "00000000-0000-0000-0000-000000000002"

func TestMain(m *testing.M) {
	ctx := context.Background()
	pg, cleanup := testutil.StartPostgresForTestMain(ctx)
	sharedPG = pg
	code := m.Run()
	cleanup()
	os.Exit(code)
}

// resetAndMigrate drops and recreates the public schema, runs all migrations,
// and inserts test tenants so that FK constraints on _ayb_billing are satisfied.
func resetAndMigrate(t *testing.T, ctx context.Context) {
	t.Helper()
	_, err := sharedPG.Pool.Exec(ctx, "DROP SCHEMA public CASCADE; CREATE SCHEMA public")
	testutil.NoError(t, err)
	r := migrations.NewRunner(sharedPG.Pool, testutil.DiscardLogger())
	testutil.NoError(t, r.Bootstrap(ctx))
	_, err = r.Run(ctx)
	testutil.NoError(t, err)

	// Insert test tenants for FK references (unique slugs required).
	tenants := []struct{ id, slug string }{
		{testTenantID, "billing-test-tenant-1"},
		{testTenantID2, "billing-test-tenant-2"},
	}
	for _, tenant := range tenants {
		_, err = sharedPG.Pool.Exec(ctx,
			`INSERT INTO _ayb_tenants (id, name, slug, state) VALUES ($1, 'Test Tenant', $2, 'active')`,
			tenant.id, tenant.slug)
		testutil.NoError(t, err)
	}
}

func TestBillingCreateAndGet(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	store := billing.NewStore(sharedPG.Pool)

	// Create a billing record for the tenant.
	rec, err := store.Create(ctx, testTenantID)
	testutil.NoError(t, err)
	testutil.Equal(t, testTenantID, rec.TenantID)
	testutil.Equal(t, billing.PlanFree, rec.Plan)
	testutil.Equal(t, billing.PaymentStatusUnpaid, rec.PaymentStatus)
	testutil.True(t, !rec.CreatedAt.IsZero(), "CreatedAt should be set")
	testutil.Equal(t, "", rec.StripeCustomerID)
	testutil.Equal(t, "", rec.StripeSubscriptionID)

	// Get should return the same record.
	got, err := store.Get(ctx, testTenantID)
	testutil.NoError(t, err)
	testutil.Equal(t, testTenantID, got.TenantID)
	testutil.Equal(t, billing.PlanFree, got.Plan)
}

func TestBillingCreateDuplicateFails(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	store := billing.NewStore(sharedPG.Pool)

	_, err := store.Create(ctx, testTenantID)
	testutil.NoError(t, err)

	// Duplicate create should return ErrBillingConflict.
	_, err = store.Create(ctx, testTenantID)
	testutil.Error(t, err)
	testutil.ErrorContains(t, err, "already exists")
}

func TestBillingGetNotFound(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	store := billing.NewStore(sharedPG.Pool)

	_, err := store.Get(ctx, "00000000-0000-0000-0000-999999999999")
	testutil.Error(t, err)
	testutil.ErrorContains(t, err, "not found")
}

func TestBillingUpsert(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	store := billing.NewStore(sharedPG.Pool)

	// Upsert a record (insert path).
	rec := &billing.BillingRecord{
		TenantID:             testTenantID,
		StripeCustomerID:     "cus_test123",
		StripeSubscriptionID: "sub_test456",
		Plan:                 billing.PlanPro,
		PaymentStatus:        billing.PaymentStatusActive,
	}
	err := store.Upsert(ctx, rec)
	testutil.NoError(t, err)

	// Verify the insert.
	got, err := store.Get(ctx, testTenantID)
	testutil.NoError(t, err)
	testutil.Equal(t, "cus_test123", got.StripeCustomerID)
	testutil.Equal(t, "sub_test456", got.StripeSubscriptionID)
	testutil.Equal(t, billing.PlanPro, got.Plan)
	testutil.Equal(t, billing.PaymentStatusActive, got.PaymentStatus)

	// Upsert again (update path) — change plan.
	rec.Plan = billing.PlanEnterprise
	err = store.Upsert(ctx, rec)
	testutil.NoError(t, err)

	got, err = store.Get(ctx, testTenantID)
	testutil.NoError(t, err)
	testutil.Equal(t, billing.PlanEnterprise, got.Plan)
}

func TestBillingUpsertNilFails(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	store := billing.NewStore(sharedPG.Pool)

	err := store.Upsert(ctx, nil)
	testutil.Error(t, err)
	testutil.ErrorContains(t, err, "required")
}

func TestBillingUpdatePlanAndPayment(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	store := billing.NewStore(sharedPG.Pool)

	_, err := store.Create(ctx, testTenantID)
	testutil.NoError(t, err)

	// Update plan and payment status.
	err = store.UpdatePlanAndPayment(ctx, testTenantID, billing.PlanStarter, billing.PaymentStatusActive)
	testutil.NoError(t, err)

	got, err := store.Get(ctx, testTenantID)
	testutil.NoError(t, err)
	testutil.Equal(t, billing.PlanStarter, got.Plan)
	testutil.Equal(t, billing.PaymentStatusActive, got.PaymentStatus)

	// Update nonexistent tenant should return not found.
	err = store.UpdatePlanAndPayment(ctx, "00000000-0000-0000-0000-999999999999", billing.PlanPro, billing.PaymentStatusActive)
	testutil.Error(t, err)
	testutil.ErrorContains(t, err, "not found")
}

func TestBillingUpdateStripeState(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	store := billing.NewStore(sharedPG.Pool)

	_, err := store.Create(ctx, testTenantID)
	testutil.NoError(t, err)

	// Set Stripe customer and subscription IDs.
	err = store.UpdateStripeState(ctx, testTenantID, "cus_abc", "sub_xyz")
	testutil.NoError(t, err)

	got, err := store.Get(ctx, testTenantID)
	testutil.NoError(t, err)
	testutil.Equal(t, "cus_abc", got.StripeCustomerID)
	testutil.Equal(t, "sub_xyz", got.StripeSubscriptionID)

	// Empty strings should not overwrite existing values (COALESCE/NULLIF logic).
	err = store.UpdateStripeState(ctx, testTenantID, "", "sub_new")
	testutil.NoError(t, err)

	got, err = store.Get(ctx, testTenantID)
	testutil.NoError(t, err)
	testutil.Equal(t, "cus_abc", got.StripeCustomerID) // unchanged
	testutil.Equal(t, "sub_new", got.StripeSubscriptionID)
}

func TestBillingGetBySubscriptionID(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	store := billing.NewStore(sharedPG.Pool)

	// Create and set subscription ID.
	_, err := store.Create(ctx, testTenantID)
	testutil.NoError(t, err)
	err = store.UpdateStripeState(ctx, testTenantID, "cus_sub", "sub_lookup")
	testutil.NoError(t, err)

	// Lookup by subscription ID.
	got, err := store.GetBySubscriptionID(ctx, "sub_lookup")
	testutil.NoError(t, err)
	testutil.Equal(t, testTenantID, got.TenantID)

	// Nonexistent subscription ID.
	_, err = store.GetBySubscriptionID(ctx, "sub_nonexistent")
	testutil.Error(t, err)
	testutil.ErrorContains(t, err, "not found")
}

func TestBillingWebhookEventDeduplication(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	store := billing.NewStore(sharedPG.Pool)

	// Event not yet processed.
	exists, err := store.HasProcessedEvent(ctx, "evt_test_001")
	testutil.NoError(t, err)
	testutil.False(t, exists, "event should not exist before recording")

	// Record the event.
	err = store.RecordWebhookEvent(ctx, "evt_test_001", "invoice.paid", []byte(`{"total": 2000}`))
	testutil.NoError(t, err)

	// Now it should be marked as processed.
	exists, err = store.HasProcessedEvent(ctx, "evt_test_001")
	testutil.NoError(t, err)
	testutil.True(t, exists, "event should exist after recording")

	// Recording the same event again should not error (ON CONFLICT DO NOTHING).
	err = store.RecordWebhookEvent(ctx, "evt_test_001", "invoice.paid", []byte(`{"total": 2000}`))
	testutil.NoError(t, err)
}

func TestBillingUsageSyncCheckpoint(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	store := billing.NewStore(sharedPG.Pool)

	// No checkpoint yet — should return 0.
	val, err := store.GetUsageSyncCheckpoint(ctx, testTenantID, "2026-03-28", "api_requests")
	testutil.NoError(t, err)
	testutil.Equal(t, int64(0), val)

	// Upsert a checkpoint.
	err = store.UpsertUsageSyncCheckpoint(ctx, testTenantID, "2026-03-28", "api_requests", 42000)
	testutil.NoError(t, err)

	// Should now return the saved value.
	val, err = store.GetUsageSyncCheckpoint(ctx, testTenantID, "2026-03-28", "api_requests")
	testutil.NoError(t, err)
	testutil.Equal(t, int64(42000), val)

	// Upsert again with higher value — should update.
	err = store.UpsertUsageSyncCheckpoint(ctx, testTenantID, "2026-03-28", "api_requests", 85000)
	testutil.NoError(t, err)

	val, err = store.GetUsageSyncCheckpoint(ctx, testTenantID, "2026-03-28", "api_requests")
	testutil.NoError(t, err)
	testutil.Equal(t, int64(85000), val)

	// Different metric for same tenant/date should be independent.
	val, err = store.GetUsageSyncCheckpoint(ctx, testTenantID, "2026-03-28", "bandwidth_bytes")
	testutil.NoError(t, err)
	testutil.Equal(t, int64(0), val) // not yet set
}
