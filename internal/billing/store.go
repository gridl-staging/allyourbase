// Package billing Store persists billing data and webhook events in PostgreSQL, providing CRUD operations and usage checkpoint tracking.
package billing

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	// ErrBillingRecordNotFound indicates no _ayb_billing row exists for a tenant.
	ErrBillingRecordNotFound = errors.New("billing record not found")
	// ErrBillingConflict indicates an insert attempted to overwrite an existing row.
	ErrBillingConflict = errors.New("billing record already exists")
)

type billingDB interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// Store persists billing lifecycle state in _ayb_billing.
type Store struct {
	db billingDB
}

const billingColumns = `tenant_id, stripe_customer_id, stripe_subscription_id, plan, payment_status, trial_start_at, trial_end_at, created_at, updated_at`

// NewStore builds a billing store backed by a PGX pool.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{db: pool}
}

// NewStoreWithDB allows unit tests to inject a lightweight DB facade.
func NewStoreWithDB(db billingDB) *Store {
	return &Store{db: db}
}

// Create inserts a fresh billing row for tenantID and returns the persisted record.
func (s *Store) Create(ctx context.Context, tenantID string) (*BillingRecord, error) {
	row := s.db.QueryRow(ctx,
		`INSERT INTO _ayb_billing (tenant_id)
		 VALUES ($1)
		 RETURNING `+billingColumns,
		tenantID,
	)
	rec, err := scanBillingRecord(row)
	if err == nil {
		return rec, nil
	}
	if isUniqueViolation(err) {
		return nil, ErrBillingConflict
	}
	return nil, fmt.Errorf("create billing record: %w", err)
}

// Get loads one billing record for the tenant.
func (s *Store) Get(ctx context.Context, tenantID string) (*BillingRecord, error) {
	row := s.db.QueryRow(ctx, `SELECT `+billingColumns+` FROM _ayb_billing WHERE tenant_id = $1`, tenantID)
	rec, err := scanBillingRecord(row)
	if err == nil {
		return rec, nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrBillingRecordNotFound
	}
	return nil, fmt.Errorf("get billing record: %w", err)
}

// Upsert inserts or updates one billing row.
func (s *Store) Upsert(ctx context.Context, rec *BillingRecord) error {
	if rec == nil {
		return fmt.Errorf("billing record is required")
	}
	_, err := s.db.Exec(ctx,
		`INSERT INTO _ayb_billing (
			tenant_id, stripe_customer_id, stripe_subscription_id, plan, payment_status, trial_start_at, trial_end_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
		 ON CONFLICT (tenant_id) DO UPDATE SET
			stripe_customer_id = EXCLUDED.stripe_customer_id,
			stripe_subscription_id = EXCLUDED.stripe_subscription_id,
			plan = EXCLUDED.plan,
			payment_status = EXCLUDED.payment_status,
			trial_start_at = EXCLUDED.trial_start_at,
			trial_end_at = EXCLUDED.trial_end_at,
			updated_at = NOW()`,
		rec.TenantID,
		nullString(rec.StripeCustomerID),
		nullString(rec.StripeSubscriptionID),
		string(rec.Plan),
		string(rec.PaymentStatus),
		rec.TrialStartAt,
		rec.TrialEndAt,
	)
	if err != nil {
		return fmt.Errorf("upsert billing record: %w", err)
	}
	return nil
}

// UpdatePlanAndPayment updates only plan and payment_status for tenantID.
func (s *Store) UpdatePlanAndPayment(ctx context.Context, tenantID string, plan Plan, status PaymentStatus) error {
	tag, err := s.db.Exec(ctx,
		`UPDATE _ayb_billing
		   SET plan = $2, payment_status = $3, updated_at = NOW()
		 WHERE tenant_id = $1`,
		tenantID, string(plan), string(status),
	)
	if err != nil {
		return fmt.Errorf("update billing plan/payment: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrBillingRecordNotFound
	}
	return nil
}

// UpdateStripeState updates Stripe identifiers for tenantID.
func (s *Store) UpdateStripeState(
	ctx context.Context,
	tenantID, customerID, subscriptionID string,
) error {
	tag, err := s.db.Exec(ctx,
		`UPDATE _ayb_billing
		   SET stripe_customer_id = COALESCE(NULLIF($2, ''), stripe_customer_id),
			   stripe_subscription_id = COALESCE(NULLIF($3, ''), stripe_subscription_id),
			   updated_at = NOW()
		 WHERE tenant_id = $1`,
		tenantID, customerID, subscriptionID,
	)
	if err != nil {
		return fmt.Errorf("update stripe state: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrBillingRecordNotFound
	}
	return nil
}

// GetBySubscriptionID loads a billing record by Stripe subscription ID.
func (s *Store) GetBySubscriptionID(ctx context.Context, subscriptionID string) (*BillingRecord, error) {
	row := s.db.QueryRow(ctx, `SELECT `+billingColumns+` FROM _ayb_billing WHERE stripe_subscription_id = $1`, subscriptionID)
	rec, err := scanBillingRecord(row)
	if err == nil {
		return rec, nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrBillingRecordNotFound
	}
	return nil, fmt.Errorf("get billing record by subscription id: %w", err)
}

// HasProcessedEvent checks if a webhook event has already been processed.
func (s *Store) HasProcessedEvent(ctx context.Context, eventID string) (bool, error) {
	var exists bool
	err := s.db.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM _ayb_billing_webhook_events WHERE event_id = $1)`,
		eventID,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check webhook event: %w", err)
	}
	return exists, nil
}

// RecordWebhookEvent records a webhook event for deduplication (idempotent via ON CONFLICT).
func (s *Store) RecordWebhookEvent(ctx context.Context, eventID, eventType string, payload []byte) error {
	_, err := s.db.Exec(ctx,
		`INSERT INTO _ayb_billing_webhook_events (event_id, event_type, payload)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (event_id) DO NOTHING`,
		eventID, eventType, payload,
	)
	if err != nil {
		return fmt.Errorf("record webhook event: %w", err)
	}
	return nil
}

// GetUsageSyncCheckpoint retrieves the last reported value for a tenant/metric/date.
func (s *Store) GetUsageSyncCheckpoint(ctx context.Context, tenantID string, usageDate string, metric string) (int64, error) {
	var lastReportedValue int64
	err := s.db.QueryRow(ctx,
		`SELECT COALESCE(last_reported_value, 0)
		   FROM _ayb_billing_usage_sync
		  WHERE tenant_id = $1 AND usage_date = $2 AND metric = $3`,
		tenantID, usageDate, metric,
	).Scan(&lastReportedValue)
	if err == nil {
		return lastReportedValue, nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	return 0, fmt.Errorf("get usage sync checkpoint: %w", err)
}

// UpsertUsageSyncCheckpoint updates or inserts a usage sync checkpoint.
func (s *Store) UpsertUsageSyncCheckpoint(ctx context.Context, tenantID string, usageDate string, metric string, lastReportedValue int64) error {
	_, err := s.db.Exec(ctx,
		`INSERT INTO _ayb_billing_usage_sync (tenant_id, usage_date, metric, last_reported_value)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (tenant_id, usage_date, metric) DO UPDATE SET
		   last_reported_value = EXCLUDED.last_reported_value,
		   updated_at = NOW()`,
		tenantID, usageDate, metric, lastReportedValue,
	)
	if err != nil {
		return fmt.Errorf("upsert usage sync checkpoint: %w", err)
	}
	return nil
}

func nullString(v string) any {
	if v == "" {
		return nil
	}
	return v
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return true
	}
	return false
}

// scanBillingRecord unmarshals a database row into a BillingRecord, converting nullable string fields and string enum columns to their typed equivalents.
func scanBillingRecord(row pgx.Row) (*BillingRecord, error) {
	var rec BillingRecord
	var stripeCustomerID sql.NullString
	var stripeSubscriptionID sql.NullString
	var createdAt, updatedAt time.Time
	var plan string
	var status string
	if err := row.Scan(
		&rec.TenantID,
		&stripeCustomerID,
		&stripeSubscriptionID,
		&plan,
		&status,
		&rec.TrialStartAt,
		&rec.TrialEndAt,
		&createdAt,
		&updatedAt,
	); err != nil {
		return nil, err
	}

	if stripeCustomerID.Valid {
		rec.StripeCustomerID = stripeCustomerID.String
	}
	if stripeSubscriptionID.Valid {
		rec.StripeSubscriptionID = stripeSubscriptionID.String
	}
	rec.Plan = Plan(plan)
	rec.PaymentStatus = PaymentStatus(status)
	rec.CreatedAt = createdAt
	rec.UpdatedAt = updatedAt
	return &rec, nil
}
