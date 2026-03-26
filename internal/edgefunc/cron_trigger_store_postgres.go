package edgefunc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Compile-time interface check.
var _ CronTriggerStore = (*CronTriggerPostgresStore)(nil)

const cronTriggerColumns = `id, function_id, schedule_id, payload, created_at, updated_at`

// CronTriggerPostgresStore implements CronTriggerStore using PostgreSQL.
type CronTriggerPostgresStore struct {
	pool *pgxpool.Pool
}

// NewCronTriggerPostgresStore creates a new Postgres-backed cron trigger store.
func NewCronTriggerPostgresStore(pool *pgxpool.Pool) *CronTriggerPostgresStore {
	return &CronTriggerPostgresStore{pool: pool}
}

// CreateCronTrigger inserts a new cron trigger record.
// If trigger.ID is set, it is used as the primary key; otherwise the DB generates one.
func (s *CronTriggerPostgresStore) CreateCronTrigger(ctx context.Context, trigger *CronTrigger) (*CronTrigger, error) {
	payload := trigger.Payload
	if payload == nil {
		payload = json.RawMessage(`{}`)
	}

	var row pgx.Row
	if trigger.ID != "" {
		row = s.pool.QueryRow(ctx,
			`INSERT INTO _ayb_edge_cron_triggers (id, function_id, schedule_id, payload)
			 VALUES ($1, $2, $3, $4)
			 RETURNING `+cronTriggerColumns,
			trigger.ID, trigger.FunctionID, trigger.ScheduleID, payload,
		)
	} else {
		row = s.pool.QueryRow(ctx,
			`INSERT INTO _ayb_edge_cron_triggers (function_id, schedule_id, payload)
			 VALUES ($1, $2, $3)
			 RETURNING `+cronTriggerColumns,
			trigger.FunctionID, trigger.ScheduleID, payload,
		)
	}

	return scanCronTrigger(row, trigger.CronExpr, trigger.Timezone, trigger.Enabled)
}

// GetCronTrigger retrieves a cron trigger by ID, joining with the schedule for cron/timezone/enabled.
func (s *CronTriggerPostgresStore) GetCronTrigger(ctx context.Context, id string) (*CronTrigger, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT ct.id, ct.function_id, ct.schedule_id, ct.payload, ct.created_at, ct.updated_at,
		        js.cron_expr, js.timezone, js.enabled
		 FROM _ayb_edge_cron_triggers ct
		 JOIN _ayb_job_schedules js ON ct.schedule_id = js.id
		 WHERE ct.id = $1`, id,
	)

	return scanCronTriggerWithSchedule(row)
}

// ListCronTriggers returns all cron triggers for a function.
func (s *CronTriggerPostgresStore) ListCronTriggers(ctx context.Context, functionID string) ([]*CronTrigger, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT ct.id, ct.function_id, ct.schedule_id, ct.payload, ct.created_at, ct.updated_at,
		        js.cron_expr, js.timezone, js.enabled
		 FROM _ayb_edge_cron_triggers ct
		 JOIN _ayb_job_schedules js ON ct.schedule_id = js.id
		 WHERE ct.function_id = $1
		 ORDER BY ct.created_at`, functionID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing cron triggers: %w", err)
	}
	defer rows.Close()

	var result []*CronTrigger
	for rows.Next() {
		t, err := scanCronTriggerWithSchedule(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning cron trigger: %w", err)
		}
		result = append(result, t)
	}
	if result == nil {
		result = []*CronTrigger{}
	}
	return result, rows.Err()
}

// UpdateCronTrigger updates a cron trigger's payload.
func (s *CronTriggerPostgresStore) UpdateCronTrigger(ctx context.Context, trigger *CronTrigger) (*CronTrigger, error) {
	payload := trigger.Payload
	if payload == nil {
		payload = json.RawMessage(`{}`)
	}

	row := s.pool.QueryRow(ctx,
		`UPDATE _ayb_edge_cron_triggers SET payload = $2, updated_at = NOW()
		 WHERE id = $1
		 RETURNING `+cronTriggerColumns,
		trigger.ID, payload,
	)

	result, err := scanCronTrigger(row, trigger.CronExpr, trigger.Timezone, trigger.Enabled)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrCronTriggerNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("updating cron trigger: %w", err)
	}
	return result, nil
}

// DeleteCronTrigger deletes a cron trigger by ID.
func (s *CronTriggerPostgresStore) DeleteCronTrigger(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM _ayb_edge_cron_triggers WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("deleting cron trigger: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrCronTriggerNotFound
	}
	return nil
}

// scanCronTrigger scans the core cron trigger columns and supplements cron/tz/enabled from caller.
func scanCronTrigger(row pgx.Row, cronExpr, timezone string, enabled bool) (*CronTrigger, error) {
	var t CronTrigger
	err := row.Scan(&t.ID, &t.FunctionID, &t.ScheduleID, &t.Payload, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrCronTriggerNotFound
		}
		return nil, fmt.Errorf("scanning cron trigger: %w", err)
	}
	t.CronExpr = cronExpr
	t.Timezone = timezone
	t.Enabled = enabled
	return &t, nil
}

// scanCronTriggerWithSchedule scans cron trigger columns + joined schedule fields.
func scanCronTriggerWithSchedule(row pgx.Row) (*CronTrigger, error) {
	var t CronTrigger
	err := row.Scan(
		&t.ID, &t.FunctionID, &t.ScheduleID, &t.Payload, &t.CreatedAt, &t.UpdatedAt,
		&t.CronExpr, &t.Timezone, &t.Enabled,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrCronTriggerNotFound
		}
		return nil, fmt.Errorf("scanning cron trigger: %w", err)
	}
	return &t, nil
}
