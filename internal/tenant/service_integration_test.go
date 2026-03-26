//go:build integration

package tenant

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/migrations"
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

func setupTenantTestDB(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	_, err := sharedPG.Pool.Exec(ctx, "DROP SCHEMA public CASCADE; CREATE SCHEMA public")
	testutil.NoError(t, err)
	runner := migrations.NewRunner(sharedPG.Pool, testutil.DiscardLogger())
	testutil.NoError(t, runner.Bootstrap(ctx))
	_, err = runner.Run(ctx)
	testutil.NoError(t, err)
}

func newTestService() *Service {
	return NewService(sharedPG.Pool, testutil.DiscardLogger())
}

func TestCreateAndGetTenant(t *testing.T) {
	setupTenantTestDB(t)
	ctx := context.Background()
	svc := newTestService()

	orgMeta := json.RawMessage(`{"plan": "startup", "contact": "admin@acme.com"}`)

	created, err := svc.CreateTenant(ctx, "Acme Corp", "acme", "schema", "pro", "us-east-1", orgMeta, "")
	testutil.NoError(t, err)
	testutil.NotNil(t, created)
	testutil.NotEqual(t, "", created.ID)
	testutil.Equal(t, "Acme Corp", created.Name)
	testutil.Equal(t, "acme", created.Slug)
	testutil.Equal(t, "schema", created.IsolationMode)
	testutil.Equal(t, "pro", created.PlanTier)
	testutil.Equal(t, "us-east-1", created.Region)
	testutil.Equal(t, TenantStateProvisioning, created.State)
	testutil.True(t, created.CreatedAt.IsZero() == false, "CreatedAt should be set")
	testutil.True(t, created.UpdatedAt.IsZero() == false, "UpdatedAt should be set")

	// Verify OrgMetadata round-trips as valid JSON.
	var gotMeta map[string]string
	testutil.NoError(t, json.Unmarshal(created.OrgMetadata, &gotMeta))
	testutil.Equal(t, "startup", gotMeta["plan"])
	testutil.Equal(t, "admin@acme.com", gotMeta["contact"])

	// GetTenant by ID.
	byID, err := svc.GetTenant(ctx, created.ID)
	testutil.NoError(t, err)
	testutil.Equal(t, created.ID, byID.ID)
	testutil.Equal(t, created.Name, byID.Name)
	testutil.Equal(t, created.State, byID.State)

	// GetTenantBySlug.
	bySlug, err := svc.GetTenantBySlug(ctx, "acme")
	testutil.NoError(t, err)
	testutil.Equal(t, created.ID, bySlug.ID)
	testutil.Equal(t, "acme", bySlug.Slug)
}

func TestCreateTenantDuplicateSlug(t *testing.T) {
	setupTenantTestDB(t)
	ctx := context.Background()
	svc := newTestService()

	_, err := svc.CreateTenant(ctx, "First Corp", "duplicate-slug", "schema", "free", "default", nil, "")
	testutil.NoError(t, err)

	_, err = svc.CreateTenant(ctx, "Second Corp", "duplicate-slug", "schema", "free", "default", nil, "")
	if !errors.Is(err, ErrTenantSlugTaken) {
		t.Errorf("expected ErrTenantSlugTaken, got %v", err)
	}
}

func TestCreateTenantIdempotencyKeyReturnsExistingTenant(t *testing.T) {
	setupTenantTestDB(t)
	ctx := context.Background()
	svc := newTestService()

	const idemKey = "idem-create-tenant-1"
	first, err := svc.CreateTenant(ctx, "Acme Corp", "acme", "schema", "pro", "us-east-1", nil, idemKey)
	testutil.NoError(t, err)
	testutil.NotNil(t, first)
	testutil.NotNil(t, first.IdempotencyKey)
	testutil.Equal(t, idemKey, *first.IdempotencyKey)

	second, err := svc.CreateTenant(ctx, "Different Name", "different-slug", "database", "enterprise", "eu-west-1", nil, idemKey)
	testutil.NoError(t, err)
	testutil.NotNil(t, second)
	testutil.Equal(t, first.ID, second.ID)
	testutil.Equal(t, first.Slug, second.Slug)
	testutil.NotNil(t, second.IdempotencyKey)
	testutil.Equal(t, idemKey, *second.IdempotencyKey)

	var count int
	err = sharedPG.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM _ayb_tenants`).Scan(&count)
	testutil.NoError(t, err)
	testutil.Equal(t, 1, count)
}

func TestCreateTenantNameRequired(t *testing.T) {
	setupTenantTestDB(t)
	ctx := context.Background()
	svc := newTestService()

	_, err := svc.CreateTenant(ctx, "", "some-slug", "schema", "free", "default", nil, "")
	if !errors.Is(err, ErrTenantNameRequired) {
		t.Errorf("expected ErrTenantNameRequired, got %v", err)
	}
}

func TestTransitionStateAtomic(t *testing.T) {
	setupTenantTestDB(t)
	ctx := context.Background()
	svc := newTestService()

	created, err := svc.CreateTenant(ctx, "StateMachine Corp", "statemachine", "schema", "free", "default", nil, "")
	testutil.NoError(t, err)
	testutil.Equal(t, TenantStateProvisioning, created.State)

	// Valid: provisioning -> active.
	activated, err := svc.TransitionState(ctx, created.ID, TenantStateProvisioning, TenantStateActive)
	testutil.NoError(t, err)
	testutil.Equal(t, TenantStateActive, activated.State)
	testutil.True(t, activated.UpdatedAt.After(created.UpdatedAt) || activated.UpdatedAt.Equal(created.UpdatedAt),
		"updated_at should be >= original")

	// Confirm persisted.
	reloaded, err := svc.GetTenant(ctx, created.ID)
	testutil.NoError(t, err)
	testutil.Equal(t, TenantStateActive, reloaded.State)

	// Invalid: active -> provisioning (rejected by state machine before DB hit).
	_, err = svc.TransitionState(ctx, created.ID, TenantStateActive, TenantStateProvisioning)
	if !errors.Is(err, ErrInvalidStateTransition) {
		t.Errorf("expected ErrInvalidStateTransition, got %v", err)
	}

	// Stale fromState: supply provisioning as fromState when tenant is now active.
	// The UPDATE WHERE state = 'provisioning' will match 0 rows; service should
	// return ErrInvalidStateTransition (not ErrTenantNotFound).
	_, err = svc.TransitionState(ctx, created.ID, TenantStateProvisioning, TenantStateActive)
	if !errors.Is(err, ErrInvalidStateTransition) {
		t.Errorf("expected ErrInvalidStateTransition for stale fromState, got %v", err)
	}

	// Not found.
	_, err = svc.TransitionState(ctx, "00000000-0000-0000-0000-000000000000", TenantStateActive, TenantStateSuspended)
	if !errors.Is(err, ErrTenantNotFound) {
		t.Errorf("expected ErrTenantNotFound for missing tenant, got %v", err)
	}
}

func TestListTenants(t *testing.T) {
	setupTenantTestDB(t)
	ctx := context.Background()
	svc := newTestService()

	// Empty list.
	result, err := svc.ListTenants(ctx, 1, 20)
	testutil.NoError(t, err)
	testutil.Equal(t, 0, result.TotalItems)
	testutil.SliceLen(t, result.Items, 0)

	// Create three tenants.
	for _, slug := range []string{"alpha", "beta", "gamma"} {
		_, err := svc.CreateTenant(ctx, slug+" Inc", slug, "schema", "free", "default", nil, "")
		testutil.NoError(t, err)
	}

	result, err = svc.ListTenants(ctx, 1, 20)
	testutil.NoError(t, err)
	testutil.Equal(t, 3, result.TotalItems)
	testutil.SliceLen(t, result.Items, 3)
	testutil.Equal(t, 1, result.TotalPages)

	// Pagination: page size 2.
	page1, err := svc.ListTenants(ctx, 1, 2)
	testutil.NoError(t, err)
	testutil.Equal(t, 3, page1.TotalItems)
	testutil.Equal(t, 2, page1.TotalPages)
	testutil.SliceLen(t, page1.Items, 2)

	page2, err := svc.ListTenants(ctx, 2, 2)
	testutil.NoError(t, err)
	testutil.SliceLen(t, page2.Items, 1)
}

func TestInsertAuditEvent(t *testing.T) {
	setupTenantTestDB(t)
	ctx := context.Background()
	svc := newTestService()

	tenant, err := svc.CreateTenant(ctx, "Audit Corp", "audit-corp", "schema", "free", "default", nil, "")
	testutil.NoError(t, err)

	actorID := "11111111-1111-1111-1111-111111111111"
	ip := "10.0.0.1"
	meta := json.RawMessage(`{"reason": "test"}`)

	err = svc.InsertAuditEvent(ctx, tenant.ID, &actorID, "tenant.created", "success", meta, &ip)
	testutil.NoError(t, err)

	// System-initiated event: actorID nil.
	err = svc.InsertAuditEvent(ctx, tenant.ID, nil, "tenant.provisioned", "success", nil, nil)
	testutil.NoError(t, err)

	var count int
	err = sharedPG.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM _ayb_tenant_audit_events WHERE tenant_id = $1`,
		tenant.ID,
	).Scan(&count)
	testutil.NoError(t, err)
	testutil.Equal(t, 2, count)
}

func TestSetQuotasAndGetQuotas(t *testing.T) {
	setupTenantTestDB(t)
	ctx := context.Background()
	svc := newTestService()

	tenant, err := svc.CreateTenant(ctx, "QuotaTest Corp", "quota-test", "schema", "free", "default", nil, "")
	testutil.NoError(t, err)

	hardRPS := 1000
	softRPS := 800
	dbHard := int64(1000000000)
	dbSoft := int64(800000000)
	connHard := 50
	connSoft := 40
	jobHard := 10
	jobSoft := 8

	quotas := TenantQuotas{
		TenantID:                tenant.ID,
		RequestRateRPSHard:      &hardRPS,
		RequestRateRPSSoft:      &softRPS,
		DBSizeBytesHard:         &dbHard,
		DBSizeBytesSoft:         &dbSoft,
		RealtimeConnectionsHard: &connHard,
		RealtimeConnectionsSoft: &connSoft,
		JobConcurrencyHard:      &jobHard,
		JobConcurrencySoft:      &jobSoft,
	}

	set, err := svc.SetQuotas(ctx, tenant.ID, quotas)
	testutil.NoError(t, err)
	testutil.NotNil(t, set)
	testutil.NotEqual(t, "", set.ID)
	testutil.Equal(t, tenant.ID, set.TenantID)
	testutil.Equal(t, hardRPS, *set.RequestRateRPSHard)
	testutil.Equal(t, softRPS, *set.RequestRateRPSSoft)

	get, err := svc.GetQuotas(ctx, tenant.ID)
	testutil.NoError(t, err)
	testutil.NotNil(t, get)
	testutil.Equal(t, set.ID, get.ID)
	testutil.Equal(t, *set.RequestRateRPSHard, *get.RequestRateRPSHard)
	testutil.Equal(t, *set.DBSizeBytesHard, *get.DBSizeBytesHard)
}

func TestSetQuotas_UpdatesExisting(t *testing.T) {
	setupTenantTestDB(t)
	ctx := context.Background()
	svc := newTestService()

	tenant, err := svc.CreateTenant(ctx, "UpdateQuota Corp", "update-quota", "schema", "free", "default", nil, "")
	testutil.NoError(t, err)

	hardRPS1 := 100
	quotas1 := TenantQuotas{TenantID: tenant.ID, RequestRateRPSHard: &hardRPS1}
	set1, err := svc.SetQuotas(ctx, tenant.ID, quotas1)
	testutil.NoError(t, err)

	hardRPS2 := 200
	quotas2 := TenantQuotas{TenantID: tenant.ID, RequestRateRPSHard: &hardRPS2}
	set2, err := svc.SetQuotas(ctx, tenant.ID, quotas2)
	testutil.NoError(t, err)

	testutil.Equal(t, set1.ID, set2.ID)
	testutil.Equal(t, 200, *set2.RequestRateRPSHard)
}

func TestGetQuotas_ReturnsNilForNonExistent(t *testing.T) {
	setupTenantTestDB(t)
	ctx := context.Background()
	svc := newTestService()

	tenant, err := svc.CreateTenant(ctx, "NoQuota Corp", "no-quota", "schema", "free", "default", nil, "")
	testutil.NoError(t, err)

	get, err := svc.GetQuotas(ctx, tenant.ID)
	testutil.NoError(t, err)
	testutil.Nil(t, get)
}

func TestUsageAccumulator_FlushWritesToDB(t *testing.T) {
	setupTenantTestDB(t)
	ctx := context.Background()
	svc := newTestService()

	tenant, err := svc.CreateTenant(ctx, "FlushTest Corp", "flush-test", "schema", "free", "default", nil, "")
	testutil.NoError(t, err)

	acc := NewUsageAccumulator(sharedPG.Pool, testutil.DiscardLogger())
	acc.Record(tenant.ID, ResourceTypeRequestRate, 100)
	acc.Record(tenant.ID, ResourceTypeDBSizeBytes, 5000)
	acc.Record(tenant.ID, ResourceTypeJobConcurrency, 25)
	acc.RecordPeak(tenant.ID, ResourceTypeRealtimeConns, 30)

	err = acc.Flush(ctx)
	testutil.NoError(t, err)

	daily, err := svc.GetDailyUsage(ctx, tenant.ID, time.Now().UTC().Truncate(24*time.Hour))
	testutil.NoError(t, err)
	testutil.NotNil(t, daily)
	testutil.Equal(t, int64(100), daily.RequestCount)
	testutil.Equal(t, int64(5000), daily.DBBytesUsed)
	testutil.Equal(t, 25, daily.JobRuns)
	testutil.Equal(t, 30, daily.RealtimePeakConnections)
}

func TestUsageAccumulator_FlushIsAdditive(t *testing.T) {
	setupTenantTestDB(t)
	ctx := context.Background()
	svc := newTestService()

	tenant, err := svc.CreateTenant(ctx, "AdditiveTest Corp", "additive-test", "schema", "free", "default", nil, "")
	testutil.NoError(t, err)

	acc := NewUsageAccumulator(sharedPG.Pool, testutil.DiscardLogger())
	acc.Record(tenant.ID, ResourceTypeRequestRate, 50)
	err = acc.Flush(ctx)
	testutil.NoError(t, err)

	acc.Record(tenant.ID, ResourceTypeRequestRate, 30)
	err = acc.Flush(ctx)
	testutil.NoError(t, err)

	daily, err := svc.GetDailyUsage(ctx, tenant.ID, time.Now().UTC().Truncate(24*time.Hour))
	testutil.NoError(t, err)
	testutil.NotNil(t, daily)
	testutil.Equal(t, int64(80), daily.RequestCount)
}

func TestGetUsageRange(t *testing.T) {
	setupTenantTestDB(t)
	ctx := context.Background()
	svc := newTestService()

	tenant, err := svc.CreateTenant(ctx, "RangeTest Corp", "range-test", "schema", "free", "default", nil, "")
	testutil.NoError(t, err)

	acc := NewUsageAccumulator(sharedPG.Pool, testutil.DiscardLogger())

	yesterday := time.Now().UTC().AddDate(0, 0, -1).Truncate(24 * time.Hour)
	_, err = sharedPG.Pool.Exec(ctx,
		`INSERT INTO _ayb_tenant_usage_daily (tenant_id, date, request_count) VALUES ($1, $2, $3)`,
		tenant.ID, yesterday, 100,
	)
	testutil.NoError(t, err)

	today := time.Now().UTC().Truncate(24 * time.Hour)
	acc.Record(tenant.ID, ResourceTypeRequestRate, 50)
	err = acc.Flush(ctx)
	testutil.NoError(t, err)

	rangeResult, err := svc.GetUsageRange(ctx, tenant.ID, yesterday, today)
	testutil.NoError(t, err)
	testutil.SliceLen(t, rangeResult, 2)

	testutil.Equal(t, yesterday, rangeResult[0].Date)
	testutil.Equal(t, int64(100), rangeResult[0].RequestCount)
	testutil.Equal(t, today, rangeResult[1].Date)
	testutil.Equal(t, int64(50), rangeResult[1].RequestCount)
}

func TestGetCurrentUsage_MergesPersistedAndUnflushed(t *testing.T) {
	setupTenantTestDB(t)
	ctx := context.Background()
	svc := newTestService()

	tenant, err := svc.CreateTenant(ctx, "CurrentUsageTest Corp", "current-usage-test", "schema", "free", "default", nil, "")
	testutil.NoError(t, err)

	acc := NewUsageAccumulator(sharedPG.Pool, testutil.DiscardLogger())
	acc.Record(tenant.ID, ResourceTypeRequestRate, 50)
	err = acc.Flush(ctx)
	testutil.NoError(t, err)

	acc.Record(tenant.ID, ResourceTypeRequestRate, 25)

	current, err := acc.GetCurrentUsage(ctx, tenant.ID, ResourceTypeRequestRate)
	testutil.NoError(t, err)
	testutil.Equal(t, int64(75), current)
}

func TestUsageAccumulator_FlushPeakOnlyWritesToDB(t *testing.T) {
	setupTenantTestDB(t)
	ctx := context.Background()
	svc := newTestService()

	tenant, err := svc.CreateTenant(ctx, "PeakOnlyFlush Corp", "peak-only-flush", "schema", "free", "default", nil, "")
	testutil.NoError(t, err)

	acc := NewUsageAccumulator(sharedPG.Pool, testutil.DiscardLogger())
	acc.RecordPeak(tenant.ID, ResourceTypeRealtimeConns, 17)

	err = acc.Flush(ctx)
	testutil.NoError(t, err)

	daily, err := svc.GetDailyUsage(ctx, tenant.ID, time.Now().UTC().Truncate(24*time.Hour))
	testutil.NoError(t, err)
	testutil.NotNil(t, daily)
	testutil.Equal(t, 17, daily.RealtimePeakConnections)
}

func TestGetCurrentUsage_PeakUsesMaxOfPersistedAndUnflushed(t *testing.T) {
	setupTenantTestDB(t)
	ctx := context.Background()
	svc := newTestService()

	tenant, err := svc.CreateTenant(ctx, "PeakCurrentUsage Corp", "peak-current-usage", "schema", "free", "default", nil, "")
	testutil.NoError(t, err)

	acc := NewUsageAccumulator(sharedPG.Pool, testutil.DiscardLogger())
	acc.RecordPeak(tenant.ID, ResourceTypeRealtimeConns, 10)
	err = acc.Flush(ctx)
	testutil.NoError(t, err)

	// Persisted peak (10) should win over lower unflushed peak.
	acc.RecordPeak(tenant.ID, ResourceTypeRealtimeConns, 7)
	current, err := acc.GetCurrentUsage(ctx, tenant.ID, ResourceTypeRealtimeConns)
	testutil.NoError(t, err)
	testutil.Equal(t, int64(10), current)

	// Higher unflushed peak should override persisted peak.
	acc.RecordPeak(tenant.ID, ResourceTypeRealtimeConns, 12)
	current, err = acc.GetCurrentUsage(ctx, tenant.ID, ResourceTypeRealtimeConns)
	testutil.NoError(t, err)
	testutil.Equal(t, int64(12), current)
}

func TestGetDailyUsage_NormalizesNonMidnightInput(t *testing.T) {
	setupTenantTestDB(t)
	ctx := context.Background()
	svc := newTestService()

	tenant, err := svc.CreateTenant(ctx, "DailyNormalize Corp", "daily-normalize", "schema", "free", "default", nil, "")
	testutil.NoError(t, err)

	day := time.Now().UTC().Truncate(24 * time.Hour)
	_, err = sharedPG.Pool.Exec(ctx,
		`INSERT INTO _ayb_tenant_usage_daily (tenant_id, date, request_count) VALUES ($1, $2, $3)`,
		tenant.ID, day, 123,
	)
	testutil.NoError(t, err)

	daily, err := svc.GetDailyUsage(ctx, tenant.ID, day.Add(13*time.Hour))
	testutil.NoError(t, err)
	testutil.NotNil(t, daily)
	testutil.Equal(t, int64(123), daily.RequestCount)
}

func TestGetUsageRange_NormalizesDateBoundaries(t *testing.T) {
	setupTenantTestDB(t)
	ctx := context.Background()
	svc := newTestService()

	tenant, err := svc.CreateTenant(ctx, "RangeNormalize Corp", "range-normalize", "schema", "free", "default", nil, "")
	testutil.NoError(t, err)

	startDay := time.Now().UTC().AddDate(0, 0, -1).Truncate(24 * time.Hour)
	endDay := time.Now().UTC().Truncate(24 * time.Hour)

	_, err = sharedPG.Pool.Exec(ctx,
		`INSERT INTO _ayb_tenant_usage_daily (tenant_id, date, request_count) VALUES ($1, $2, $3), ($1, $4, $5)`,
		tenant.ID, startDay, 100, endDay, 200,
	)
	testutil.NoError(t, err)

	rangeResult, err := svc.GetUsageRange(ctx, tenant.ID, startDay.Add(11*time.Hour), endDay.Add(18*time.Hour))
	testutil.NoError(t, err)
	testutil.SliceLen(t, rangeResult, 2)
	testutil.Equal(t, int64(100), rangeResult[0].RequestCount)
	testutil.Equal(t, int64(200), rangeResult[1].RequestCount)
}

func TestQueryAuditEvents(t *testing.T) {
	setupTenantTestDB(t)
	ctx := context.Background()
	svc := newTestService()

	tenant1, err := svc.CreateTenant(ctx, "AuditQuery1", "audit-query-1", "schema", "free", "default", nil, "")
	testutil.NoError(t, err)

	actorID := "11111111-1111-1111-1111-111111111111"

	for i := 0; i < 5; i++ {
		action := "tenant.event_" + string(rune('a'+i))
		err = svc.InsertAuditEvent(ctx, tenant1.ID, &actorID, action, "success", nil, nil)
		testutil.NoError(t, err)
		time.Sleep(10 * time.Millisecond)
	}

	events, err := svc.QueryAuditEvents(ctx, AuditQuery{
		TenantID: tenant1.ID,
		Limit:    10,
	})
	testutil.NoError(t, err)
	testutil.SliceLen(t, events, 5)

	for i := 1; i < len(events); i++ {
		if events[i-1].CreatedAt.Before(events[i].CreatedAt) {
			t.Errorf("events not ordered by created_at DESC: [%d] %v < [%d] %v", i-1, events[i-1].CreatedAt, i, events[i].CreatedAt)
		}
	}
}

func TestAuditEventsImmutable_UpdateDeleteBlocked(t *testing.T) {
	setupTenantTestDB(t)
	ctx := context.Background()
	svc := newTestService()

	tenant1, err := svc.CreateTenant(ctx, "AuditImmutable", "audit-immutable", "schema", "free", "default", nil, "")
	testutil.NoError(t, err)

	actorID := "11111111-1111-1111-1111-111111111111"
	testutil.NoError(t, svc.InsertAuditEvent(ctx, tenant1.ID, &actorID, "tenant.created", "success", nil, nil))

	_, err = sharedPG.Pool.Exec(ctx, `UPDATE _ayb_tenant_audit_events SET action = 'tampered' WHERE tenant_id = $1::uuid`, tenant1.ID)
	testutil.ErrorContains(t, err, "audit events are immutable")

	_, err = sharedPG.Pool.Exec(ctx, `DELETE FROM _ayb_tenant_audit_events WHERE tenant_id = $1::uuid`, tenant1.ID)
	testutil.ErrorContains(t, err, "audit events are immutable")
}

func TestQueryAuditEvents_WithFilters(t *testing.T) {
	setupTenantTestDB(t)
	ctx := context.Background()
	svc := newTestService()

	tenant1, err := svc.CreateTenant(ctx, "AuditFilter", "audit-filter", "schema", "free", "default", nil, "")
	testutil.NoError(t, err)

	actorID := "11111111-1111-1111-1111-111111111111"
	otherActor := "22222222-2222-2222-2222-222222222222"

	err = svc.InsertAuditEvent(ctx, tenant1.ID, &actorID, "tenant.created", "success", nil, nil)
	testutil.NoError(t, err)
	time.Sleep(10 * time.Millisecond)

	err = svc.InsertAuditEvent(ctx, tenant1.ID, &actorID, "membership.added", "success", nil, nil)
	testutil.NoError(t, err)
	time.Sleep(10 * time.Millisecond)

	err = svc.InsertAuditEvent(ctx, tenant1.ID, &otherActor, "tenant.created", "success", nil, nil)
	testutil.NoError(t, err)

	filteredByAction, err := svc.QueryAuditEvents(ctx, AuditQuery{
		TenantID: tenant1.ID,
		Action:   "tenant.created",
		Limit:    10,
	})
	testutil.NoError(t, err)
	testutil.SliceLen(t, filteredByAction, 2)

	filteredByActor, err := svc.QueryAuditEvents(ctx, AuditQuery{
		TenantID: tenant1.ID,
		ActorID:  actorID,
		Limit:    10,
	})
	testutil.NoError(t, err)
	testutil.SliceLen(t, filteredByActor, 2)
}

func TestQueryAuditEvents_Pagination(t *testing.T) {
	setupTenantTestDB(t)
	ctx := context.Background()
	svc := newTestService()

	tenant1, err := svc.CreateTenant(ctx, "AuditPage", "audit-page", "schema", "free", "default", nil, "")
	testutil.NoError(t, err)

	for i := 0; i < 10; i++ {
		err = svc.InsertAuditEvent(ctx, tenant1.ID, nil, "tenant.event", "success", nil, nil)
		testutil.NoError(t, err)
		time.Sleep(5 * time.Millisecond)
	}

	page1, err := svc.QueryAuditEvents(ctx, AuditQuery{
		TenantID: tenant1.ID,
		Limit:    5,
		Offset:   0,
	})
	testutil.NoError(t, err)
	testutil.SliceLen(t, page1, 5)

	page2, err := svc.QueryAuditEvents(ctx, AuditQuery{
		TenantID: tenant1.ID,
		Limit:    5,
		Offset:   5,
	})
	testutil.NoError(t, err)
	testutil.SliceLen(t, page2, 5)
}
