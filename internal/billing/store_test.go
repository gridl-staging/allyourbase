package billing

import (
	"bytes"
	"context"
	stdsql "database/sql"
	"errors"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type fakeBillingRow struct {
	scanFn func(dest ...any) error
}

func (r *fakeBillingRow) Scan(dest ...any) error {
	if r.scanFn == nil {
		return errors.New("scan not configured")
	}
	return r.scanFn(dest...)
}

type fakeBillingDB struct {
	queryRowFn func(sql string, args ...any) pgx.Row
	execFn     func(sql string, args ...any) (pgconn.CommandTag, error)

	queryRowSQL []string
	queryRowArg [][]any
	execSQL     []string
	execArg     [][]any
}

func (db *fakeBillingDB) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	db.queryRowSQL = append(db.queryRowSQL, sql)
	db.queryRowArg = append(db.queryRowArg, args)
	if db.queryRowFn != nil {
		return db.queryRowFn(sql, args...)
	}
	return &fakeBillingRow{scanFn: func(dest ...any) error {
		return pgx.ErrNoRows
	}}
}

func (db *fakeBillingDB) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	db.execSQL = append(db.execSQL, sql)
	db.execArg = append(db.execArg, args)
	if db.execFn != nil {
		return db.execFn(sql, args...)
	}
	return pgconn.NewCommandTag("UPDATE 0"), nil
}

func TestStoreCreate_DefaultsAndConflict(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC().Round(time.Second)
	db := &fakeBillingDB{}
	db.queryRowFn = func(sql string, args ...any) pgx.Row {
		return &fakeBillingRow{
			scanFn: func(dest ...any) error {
				*(dest[0].(*string)) = "tenant-1"
				*(dest[1].(*stdsql.NullString)) = stdsql.NullString{}
				*(dest[2].(*stdsql.NullString)) = stdsql.NullString{}
				*(dest[3].(*string)) = string(PlanFree)
				*(dest[4].(*string)) = string(PaymentStatusUnpaid)
				*(dest[5].(**time.Time)) = nil
				*(dest[6].(**time.Time)) = nil
				*(dest[7].(*time.Time)) = now
				*(dest[8].(*time.Time)) = now
				return nil
			},
		}
	}

	s := NewStoreWithDB(db)
	rec, err := s.Create(context.Background(), "tenant-1")
	testutil.NoError(t, err)
	testutil.Equal(t, 1, len(db.queryRowSQL))
	testutil.Contains(t, db.queryRowSQL[0], "INSERT INTO _ayb_billing")
	testutil.Equal(t, "tenant-1", rec.TenantID)
	testutil.Equal(t, PlanFree, rec.Plan)
	testutil.Equal(t, PaymentStatusUnpaid, rec.PaymentStatus)
	testutil.Equal(t, now, rec.CreatedAt)

	db.queryRowFn = func(sql string, args ...any) pgx.Row {
		return &fakeBillingRow{
			scanFn: func(dest ...any) error {
				return &pgconn.PgError{Code: "23505"}
			},
		}
	}

	_, err = s.Create(context.Background(), "tenant-1")
	testutil.ErrorContains(t, err, "billing record already exists")
}

func TestStoreGet_NotFound(t *testing.T) {
	t.Parallel()

	db := &fakeBillingDB{
		queryRowFn: func(sql string, args ...any) pgx.Row {
			return &fakeBillingRow{scanFn: func(dest ...any) error { return pgx.ErrNoRows }}
		},
	}
	s := NewStoreWithDB(db)
	_, err := s.Get(context.Background(), "tenant-missing")
	testutil.ErrorContains(t, err, "billing record not found")
}

func TestStoreUpsert_PersistsPlanAndStripeIDs(t *testing.T) {
	t.Parallel()

	db := &fakeBillingDB{
		execFn: func(sql string, args ...any) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag("INSERT 1"), nil
		},
	}
	s := NewStoreWithDB(db)
	err := s.Upsert(context.Background(), &BillingRecord{
		TenantID:             "tenant-1",
		StripeCustomerID:     "cus_123",
		StripeSubscriptionID: "sub_123",
		Plan:                 PlanPro,
		PaymentStatus:        PaymentStatusActive,
	})
	testutil.NoError(t, err)

	testutil.Contains(t, db.execSQL[0], "INSERT INTO _ayb_billing")
	testutil.Equal(t, "tenant-1", db.execArg[0][0].(string))
	testutil.Equal(t, "cus_123", db.execArg[0][1].(string))
	testutil.Equal(t, "sub_123", db.execArg[0][2].(string))
	testutil.Equal(t, string(PlanPro), db.execArg[0][3].(string))
	testutil.Equal(t, string(PaymentStatusActive), db.execArg[0][4].(string))
}

func TestStoreUpdatePlanAndPayment_NotFound(t *testing.T) {
	t.Parallel()

	db := &fakeBillingDB{
		execFn: func(sql string, args ...any) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag("UPDATE 0"), nil
		},
	}
	s := NewStoreWithDB(db)
	err := s.UpdatePlanAndPayment(context.Background(), "missing", PlanFree, PaymentStatusUnpaid)
	testutil.ErrorContains(t, err, "billing record not found")
}

func TestStoreUpdateStripeState_UpdatesProvidedValues(t *testing.T) {
	t.Parallel()

	db := &fakeBillingDB{
		execFn: func(sql string, args ...any) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag("UPDATE 1"), nil
		},
	}
	s := NewStoreWithDB(db)
	err := s.UpdateStripeState(context.Background(), "tenant-1", "cus_123", "")
	testutil.NoError(t, err)
	testutil.Contains(t, db.execSQL[0], "UPDATE _ayb_billing")
	testutil.Equal(t, "tenant-1", db.execArg[0][0].(string))
	testutil.Equal(t, "cus_123", db.execArg[0][1].(string))
	testutil.Equal(t, "", db.execArg[0][2].(string))
}

func TestScanBillingRecord_AllowsNullStripeIDs(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC().Round(time.Second)
	row := &fakeBillingRow{
		scanFn: func(dest ...any) error {
			*(dest[0].(*string)) = "tenant-1"
			*(dest[1].(*stdsql.NullString)) = stdsql.NullString{}
			*(dest[2].(*stdsql.NullString)) = stdsql.NullString{}
			*(dest[3].(*string)) = string(PlanFree)
			*(dest[4].(*string)) = string(PaymentStatusUnpaid)
			*(dest[5].(**time.Time)) = nil
			*(dest[6].(**time.Time)) = nil
			*(dest[7].(*time.Time)) = now
			*(dest[8].(*time.Time)) = now
			return nil
		},
	}

	rec, err := scanBillingRecord(row)
	testutil.NoError(t, err)
	testutil.Equal(t, "tenant-1", rec.TenantID)
	testutil.Equal(t, "", rec.StripeCustomerID)
	testutil.Equal(t, "", rec.StripeSubscriptionID)
	testutil.Equal(t, PlanFree, rec.Plan)
	testutil.Equal(t, PaymentStatusUnpaid, rec.PaymentStatus)
}

func TestStoreGetBySubscriptionID_FindsRecord(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC().Round(time.Second)
	db := &fakeBillingDB{
		queryRowFn: func(sql string, args ...any) pgx.Row {
			return &fakeBillingRow{
				scanFn: func(dest ...any) error {
					*(dest[0].(*string)) = "tenant-1"
					*(dest[1].(*stdsql.NullString)) = stdsql.NullString{String: "cus_123", Valid: true}
					*(dest[2].(*stdsql.NullString)) = stdsql.NullString{String: "sub_123", Valid: true}
					*(dest[3].(*string)) = string(PlanPro)
					*(dest[4].(*string)) = string(PaymentStatusActive)
					*(dest[5].(**time.Time)) = nil
					*(dest[6].(**time.Time)) = nil
					*(dest[7].(*time.Time)) = now
					*(dest[8].(*time.Time)) = now
					return nil
				},
			}
		},
	}
	s := NewStoreWithDB(db)
	rec, err := s.GetBySubscriptionID(context.Background(), "sub_123")
	testutil.NoError(t, err)
	testutil.Contains(t, db.queryRowSQL[0], "WHERE stripe_subscription_id = $1")
	testutil.Equal(t, "sub_123", db.queryRowArg[0][0].(string))
	testutil.Equal(t, "tenant-1", rec.TenantID)
	testutil.Equal(t, "cus_123", rec.StripeCustomerID)
	testutil.Equal(t, "sub_123", rec.StripeSubscriptionID)
}

func TestStoreGetBySubscriptionID_NotFound(t *testing.T) {
	t.Parallel()

	db := &fakeBillingDB{
		queryRowFn: func(sql string, args ...any) pgx.Row {
			return &fakeBillingRow{scanFn: func(dest ...any) error { return pgx.ErrNoRows }}
		},
	}
	s := NewStoreWithDB(db)
	_, err := s.GetBySubscriptionID(context.Background(), "sub_missing")
	testutil.ErrorContains(t, err, "billing record not found")
}

func TestStoreHasProcessedEvent(t *testing.T) {
	t.Parallel()

	db := &fakeBillingDB{
		queryRowFn: func(sql string, args ...any) pgx.Row {
			return &fakeBillingRow{
				scanFn: func(dest ...any) error {
					*(dest[0].(*bool)) = true
					return nil
				},
			}
		},
	}
	s := NewStoreWithDB(db)
	ok, err := s.HasProcessedEvent(context.Background(), "evt_123")
	testutil.NoError(t, err)
	testutil.True(t, ok, "event should be marked processed")
	testutil.Contains(t, db.queryRowSQL[0], "FROM _ayb_billing_webhook_events")
	testutil.Equal(t, "evt_123", db.queryRowArg[0][0].(string))
}

func TestStoreRecordWebhookEvent_UsesConflictDedup(t *testing.T) {
	t.Parallel()

	db := &fakeBillingDB{
		execFn: func(sql string, args ...any) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag("INSERT 0"), nil
		},
	}
	s := NewStoreWithDB(db)
	err := s.RecordWebhookEvent(context.Background(), "evt_123", "invoice.paid", []byte(`{"id":"evt_123"}`))
	testutil.NoError(t, err)
	testutil.Contains(t, db.execSQL[0], "INSERT INTO _ayb_billing_webhook_events")
	testutil.Contains(t, db.execSQL[0], "ON CONFLICT (event_id) DO NOTHING")
	testutil.Equal(t, "evt_123", db.execArg[0][0].(string))
	testutil.Equal(t, "invoice.paid", db.execArg[0][1].(string))
	testutil.True(t, bytes.Equal([]byte(`{"id":"evt_123"}`), db.execArg[0][2].([]byte)), "payload should match")
}

func TestStoreGetUsageSyncCheckpoint_NotFoundReturnsZero(t *testing.T) {
	t.Parallel()

	db := &fakeBillingDB{
		queryRowFn: func(sql string, args ...any) pgx.Row {
			return &fakeBillingRow{scanFn: func(dest ...any) error { return pgx.ErrNoRows }}
		},
	}
	s := NewStoreWithDB(db)
	value, err := s.GetUsageSyncCheckpoint(context.Background(), "tenant-1", "2026-03-03", "api_requests")
	testutil.NoError(t, err)
	testutil.Equal(t, int64(0), value)
}

func TestStoreGetUsageSyncCheckpoint_ReturnsValue(t *testing.T) {
	t.Parallel()

	db := &fakeBillingDB{
		queryRowFn: func(sql string, args ...any) pgx.Row {
			return &fakeBillingRow{
				scanFn: func(dest ...any) error {
					*(dest[0].(*int64)) = 123
					return nil
				},
			}
		},
	}
	s := NewStoreWithDB(db)
	value, err := s.GetUsageSyncCheckpoint(context.Background(), "tenant-1", "2026-03-03", "api_requests")
	testutil.NoError(t, err)
	testutil.Equal(t, int64(123), value)
}

func TestStoreUpsertUsageSyncCheckpoint_UsesConflictUpdate(t *testing.T) {
	t.Parallel()

	db := &fakeBillingDB{
		execFn: func(sql string, args ...any) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag("INSERT 1"), nil
		},
	}
	s := NewStoreWithDB(db)
	err := s.UpsertUsageSyncCheckpoint(context.Background(), "tenant-1", "2026-03-03", "api_requests", 321)
	testutil.NoError(t, err)
	testutil.Contains(t, db.execSQL[0], "INSERT INTO _ayb_billing_usage_sync")
	testutil.Contains(t, db.execSQL[0], "ON CONFLICT (tenant_id, usage_date, metric) DO UPDATE")
	testutil.Equal(t, "tenant-1", db.execArg[0][0].(string))
	testutil.Equal(t, "2026-03-03", db.execArg[0][1].(string))
	testutil.Equal(t, "api_requests", db.execArg[0][2].(string))
	testutil.Equal(t, int64(321), db.execArg[0][3].(int64))
}
