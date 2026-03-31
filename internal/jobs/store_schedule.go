// Package jobs Store provides database operations for schedule lifecycle and execution coordination.
package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5"
	"time"
)

const scheduleColumns = `id, name, job_type, payload, cron_expr, timezone, enabled,
	max_attempts, next_run_at, last_run_at, created_at, updated_at`

func scanSchedule(row pgx.Row) (*Schedule, error) {
	var s Schedule
	err := row.Scan(
		&s.ID, &s.Name, &s.JobType, &s.Payload, &s.CronExpr, &s.Timezone,
		&s.Enabled, &s.MaxAttempts, &s.NextRunAt, &s.LastRunAt, &s.CreatedAt, &s.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// CreateSchedule inserts a new schedule.
func (s *Store) CreateSchedule(ctx context.Context, sched *Schedule) (*Schedule, error) {
	if sched.Payload == nil {
		sched.Payload = json.RawMessage("{}")
	}
	row := s.pool.QueryRow(ctx,
		`INSERT INTO _ayb_job_schedules (name, job_type, payload, cron_expr, timezone, enabled, max_attempts, next_run_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 RETURNING `+scheduleColumns,
		sched.Name, sched.JobType, sched.Payload, sched.CronExpr, sched.Timezone,
		sched.Enabled, sched.MaxAttempts, sched.NextRunAt,
	)
	return scanSchedule(row)
}

// UpsertSchedule inserts or updates a schedule by name (for default schedule registration).
func (s *Store) UpsertSchedule(ctx context.Context, sched *Schedule) (*Schedule, error) {
	if sched.Payload == nil {
		sched.Payload = json.RawMessage("{}")
	}
	row := s.pool.QueryRow(ctx,
		`INSERT INTO _ayb_job_schedules (name, job_type, payload, cron_expr, timezone, enabled, max_attempts, next_run_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 ON CONFLICT (name) DO NOTHING
		 RETURNING `+scheduleColumns,
		sched.Name, sched.JobType, sched.Payload, sched.CronExpr, sched.Timezone,
		sched.Enabled, sched.MaxAttempts, sched.NextRunAt,
	)
	result, err := scanSchedule(row)
	if err == pgx.ErrNoRows {
		// Already existed, fetch it.
		return s.GetScheduleByName(ctx, sched.Name)
	}
	return result, err
}

// GetSchedule returns a schedule by ID.
func (s *Store) GetSchedule(ctx context.Context, id string) (*Schedule, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT `+scheduleColumns+` FROM _ayb_job_schedules WHERE id = $1`, id,
	)
	sched, err := scanSchedule(row)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("schedule %s not found", id)
	}
	return sched, err
}

// GetScheduleByName returns a schedule by name.
func (s *Store) GetScheduleByName(ctx context.Context, name string) (*Schedule, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT `+scheduleColumns+` FROM _ayb_job_schedules WHERE name = $1`, name,
	)
	sched, err := scanSchedule(row)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("schedule %q not found", name)
	}
	return sched, err
}

// ListSchedules returns all schedules.
func (s *Store) ListSchedules(ctx context.Context) ([]Schedule, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+scheduleColumns+` FROM _ayb_job_schedules ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []Schedule
	for rows.Next() {
		var sched Schedule
		if err := rows.Scan(
			&sched.ID, &sched.Name, &sched.JobType, &sched.Payload, &sched.CronExpr,
			&sched.Timezone, &sched.Enabled, &sched.MaxAttempts, &sched.NextRunAt,
			&sched.LastRunAt, &sched.CreatedAt, &sched.UpdatedAt,
		); err != nil {
			return nil, err
		}
		result = append(result, sched)
	}
	if result == nil {
		result = []Schedule{}
	}
	return result, rows.Err()
}

// UpdateSchedule updates a schedule's mutable fields.
func (s *Store) UpdateSchedule(ctx context.Context, id string, cronExpr, timezone string, payload json.RawMessage, enabled bool, nextRunAt *time.Time) (*Schedule, error) {
	if payload == nil {
		payload = json.RawMessage("{}")
	}
	row := s.pool.QueryRow(ctx,
		`UPDATE _ayb_job_schedules SET
			cron_expr = $2, timezone = $3, payload = $4, enabled = $5, next_run_at = $6, updated_at = NOW()
		WHERE id = $1
		RETURNING `+scheduleColumns,
		id, cronExpr, timezone, payload, enabled, nextRunAt,
	)
	sched, err := scanSchedule(row)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("schedule %s not found", id)
	}
	return sched, err
}

// DeleteSchedule hard-deletes a schedule.
func (s *Store) DeleteSchedule(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM _ayb_job_schedules WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("schedule %s not found", id)
	}
	return nil
}

// DueSchedules returns enabled schedules where next_run_at <= now.
func (s *Store) DueSchedules(ctx context.Context) ([]Schedule, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+scheduleColumns+` FROM _ayb_job_schedules
		 WHERE enabled = true AND next_run_at IS NOT NULL AND next_run_at <= NOW()`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []Schedule
	for rows.Next() {
		var sched Schedule
		if err := rows.Scan(
			&sched.ID, &sched.Name, &sched.JobType, &sched.Payload, &sched.CronExpr,
			&sched.Timezone, &sched.Enabled, &sched.MaxAttempts, &sched.NextRunAt,
			&sched.LastRunAt, &sched.CreatedAt, &sched.UpdatedAt,
		); err != nil {
			return nil, err
		}
		result = append(result, sched)
	}
	return result, rows.Err()
}

// AdvanceScheduleAndEnqueue atomically advances a schedule's next_run_at
// and enqueues the corresponding job in a single transaction. This prevents
// the case where AdvanceSchedule succeeds but Enqueue fails (or vice versa),
// which would silently skip a scheduled tick.
// Returns false if another instance already advanced this tick (0 rows affected).
func (s *Store) AdvanceScheduleAndEnqueue(ctx context.Context, scheduleID string, nextRunAt time.Time, jobType string, payload json.RawMessage, maxAttempts int) (bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	tag, err := tx.Exec(ctx,
		`UPDATE _ayb_job_schedules SET
			last_run_at = NOW(),
			next_run_at = $2,
			updated_at = NOW()
		WHERE id = $1 AND enabled = true AND next_run_at <= NOW()`,
		scheduleID, nextRunAt,
	)
	if err != nil {
		return false, err
	}
	if tag.RowsAffected() == 0 {
		return false, nil // another instance handled this tick
	}
	if payload == nil {
		payload = json.RawMessage("{}")
	}
	if maxAttempts <= 0 {
		maxAttempts = 3
	}
	_, err = tx.Exec(ctx,
		`INSERT INTO _ayb_jobs (type, payload, max_attempts, schedule_id)
		 VALUES ($1, $2, $3, $4)`,
		jobType, payload, maxAttempts, scheduleID,
	)
	if err != nil {
		return false, fmt.Errorf("enqueue job: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit tx: %w", err)
	}
	return true, nil
}

// SetScheduleEnabled sets the enabled flag and optionally recomputes next_run_at.
func (s *Store) SetScheduleEnabled(ctx context.Context, id string, enabled bool, nextRunAt *time.Time) (*Schedule, error) {
	row := s.pool.QueryRow(ctx,
		`UPDATE _ayb_job_schedules SET
			enabled = $2, next_run_at = $3, updated_at = NOW()
		WHERE id = $1
		RETURNING `+scheduleColumns,
		id, enabled, nextRunAt,
	)
	sched, err := scanSchedule(row)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("schedule %s not found", id)
	}
	return sched, err
}
