package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// intervalSec formats a time.Duration as a Postgres-compatible interval string.
// Go's Duration.String() produces "5m0s" which Postgres cannot parse;
// this produces "300 seconds" which is unambiguous.
func intervalSec(d time.Duration) string {
	return fmt.Sprintf("%d seconds", int64(d.Seconds()))
}

// Store handles database operations for the job queue.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore creates a new job Store.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

const jobColumns = `id, type, payload, state, run_at, lease_until, worker_id,
	attempts, max_attempts, last_error, last_run_at, idempotency_key,
	schedule_id, created_at, updated_at, completed_at, canceled_at`

func scanJob(row pgx.Row) (*Job, error) {
	var j Job
	err := row.Scan(
		&j.ID, &j.Type, &j.Payload, &j.State, &j.RunAt, &j.LeaseUntil,
		&j.WorkerID, &j.Attempts, &j.MaxAttempts, &j.LastError, &j.LastRunAt,
		&j.IdempotencyKey, &j.ScheduleID, &j.CreatedAt, &j.UpdatedAt,
		&j.CompletedAt, &j.CanceledAt,
	)
	if err != nil {
		return nil, err
	}
	return &j, nil
}

func scanJobs(rows pgx.Rows) ([]Job, error) {
	var result []Job
	for rows.Next() {
		var j Job
		if err := rows.Scan(
			&j.ID, &j.Type, &j.Payload, &j.State, &j.RunAt, &j.LeaseUntil,
			&j.WorkerID, &j.Attempts, &j.MaxAttempts, &j.LastError, &j.LastRunAt,
			&j.IdempotencyKey, &j.ScheduleID, &j.CreatedAt, &j.UpdatedAt,
			&j.CompletedAt, &j.CanceledAt,
		); err != nil {
			return nil, err
		}
		result = append(result, j)
	}
	return result, rows.Err()
}

// Enqueue inserts a new job with state=queued.
func (s *Store) Enqueue(ctx context.Context, jobType string, payload json.RawMessage, opts EnqueueOpts) (*Job, error) {
	if payload == nil {
		payload = json.RawMessage("{}")
	}
	runAt := time.Now()
	if opts.RunAt != nil {
		runAt = *opts.RunAt
	}
	maxAttempts := 3
	if opts.MaxAttempts > 0 {
		maxAttempts = opts.MaxAttempts
	}

	var idempotencyKey *string
	if opts.IdempotencyKey != "" {
		idempotencyKey = &opts.IdempotencyKey
	}
	var scheduleID *string
	if opts.ScheduleID != "" {
		scheduleID = &opts.ScheduleID
	}

	row := s.pool.QueryRow(ctx,
		`INSERT INTO _ayb_jobs (type, payload, run_at, max_attempts, idempotency_key, schedule_id)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING `+jobColumns,
		jobType, payload, runAt, maxAttempts, idempotencyKey, scheduleID,
	)
	return scanJob(row)
}

// Claim atomically claims the next eligible queued job using FOR UPDATE SKIP LOCKED.
// Returns nil, nil if no job is available.
func (s *Store) Claim(ctx context.Context, workerID string, leaseDuration time.Duration) (*Job, error) {
	row := s.pool.QueryRow(ctx,
		`UPDATE _ayb_jobs SET
			state = 'running',
			lease_until = NOW() + $1::interval,
			worker_id = $2,
			attempts = attempts + 1,
			last_run_at = NOW(),
			updated_at = NOW()
		WHERE id = (
			SELECT id FROM _ayb_jobs
			WHERE state = 'queued' AND run_at <= NOW()
			ORDER BY run_at
			LIMIT 1
			FOR UPDATE SKIP LOCKED
		)
		RETURNING `+jobColumns,
		intervalSec(leaseDuration), workerID,
	)
	j, err := scanJob(row)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return j, err
}

// Complete marks a running job as completed.
func (s *Store) Complete(ctx context.Context, jobID string) (*Job, error) {
	row := s.pool.QueryRow(ctx,
		`UPDATE _ayb_jobs SET
			state = 'completed',
			completed_at = NOW(),
			lease_until = NULL,
			worker_id = NULL,
			updated_at = NOW()
		WHERE id = $1 AND state = 'running'
		RETURNING `+jobColumns,
		jobID,
	)
	j, err := scanJob(row)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("job %s not found or not in running state", jobID)
	}
	return j, err
}

// Fail handles a job failure. If retries remain, re-queues with backoff.
// Otherwise marks as permanently failed.
func (s *Store) Fail(ctx context.Context, jobID string, errMsg string, backoff time.Duration) (*Job, error) {
	// First try re-queue (attempts < max_attempts).
	row := s.pool.QueryRow(ctx,
		`UPDATE _ayb_jobs SET
			state = 'queued',
			run_at = NOW() + $2::interval,
			last_error = $3,
			lease_until = NULL,
			worker_id = NULL,
			updated_at = NOW()
		WHERE id = $1 AND state = 'running' AND attempts < max_attempts
		RETURNING `+jobColumns,
		jobID, intervalSec(backoff), errMsg,
	)
	j, err := scanJob(row)
	if err == nil {
		return j, nil
	}
	if err != pgx.ErrNoRows {
		return nil, err
	}

	// Terminal failure: attempts >= max_attempts.
	row = s.pool.QueryRow(ctx,
		`UPDATE _ayb_jobs SET
			state = 'failed',
			last_error = $2,
			lease_until = NULL,
			worker_id = NULL,
			updated_at = NOW()
		WHERE id = $1 AND state = 'running'
		RETURNING `+jobColumns,
		jobID, errMsg,
	)
	j, err = scanJob(row)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("job %s not found or not in running state", jobID)
	}
	return j, err
}

// Cancel cancels a queued job.
func (s *Store) Cancel(ctx context.Context, jobID string) (*Job, error) {
	row := s.pool.QueryRow(ctx,
		`UPDATE _ayb_jobs SET
			state = 'canceled',
			canceled_at = NOW(),
			updated_at = NOW()
		WHERE id = $1 AND state = 'queued'
		RETURNING `+jobColumns,
		jobID,
	)
	j, err := scanJob(row)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("job %s not found or not in queued state", jobID)
	}
	return j, err
}

// RetryNow resets a failed job to queued with run_at=now.
func (s *Store) RetryNow(ctx context.Context, jobID string) (*Job, error) {
	row := s.pool.QueryRow(ctx,
		`UPDATE _ayb_jobs SET
			state = 'queued',
			run_at = NOW(),
			last_error = NULL,
			attempts = 0,
			lease_until = NULL,
			worker_id = NULL,
			updated_at = NOW()
		WHERE id = $1 AND state = 'failed'
		RETURNING `+jobColumns,
		jobID,
	)
	j, err := scanJob(row)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("job %s not found or not in failed state", jobID)
	}
	return j, err
}

// ExtendLease extends the lease of a running job by the given duration from now.
// Returns an error if the job is not in the running state.
func (s *Store) ExtendLease(ctx context.Context, jobID string, leaseDuration time.Duration) (*Job, error) {
	row := s.pool.QueryRow(ctx,
		`UPDATE _ayb_jobs SET
			lease_until = NOW() + $2::interval,
			updated_at = NOW()
		WHERE id = $1 AND state = 'running'
		RETURNING `+jobColumns,
		jobID, intervalSec(leaseDuration),
	)
	j, err := scanJob(row)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("job %s not found or not in running state", jobID)
	}
	return j, err
}

// RecoverStalledJobs finds running jobs with expired leases and re-queues them.
// Returns the number of recovered jobs.
func (s *Store) RecoverStalledJobs(ctx context.Context) (int64, error) {
	tag, err := s.pool.Exec(ctx,
		`UPDATE _ayb_jobs SET
			state = 'queued',
			lease_until = NULL,
			worker_id = NULL,
			updated_at = NOW()
		WHERE state = 'running' AND lease_until < NOW()`,
	)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// Get returns a job by ID.
func (s *Store) Get(ctx context.Context, jobID string) (*Job, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT `+jobColumns+` FROM _ayb_jobs WHERE id = $1`,
		jobID,
	)
	j, err := scanJob(row)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("job %s not found", jobID)
	}
	return j, err
}

// List returns jobs with optional filters.
func (s *Store) List(ctx context.Context, state string, jobType string, limit, offset int) ([]Job, error) {
	query := `SELECT ` + jobColumns + ` FROM _ayb_jobs WHERE 1=1`
	args := []any{}
	argN := 1

	if state != "" {
		query += fmt.Sprintf(" AND state = $%d", argN)
		args = append(args, state)
		argN++
	}
	if jobType != "" {
		query += fmt.Sprintf(" AND type = $%d", argN)
		args = append(args, jobType)
		argN++
	}
	query += " ORDER BY created_at DESC"
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT $%d", argN)
		args = append(args, limit)
		argN++
	}
	if offset > 0 {
		query += fmt.Sprintf(" OFFSET $%d", argN)
		args = append(args, offset)
	}

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	jobs, err := scanJobs(rows)
	if jobs == nil {
		jobs = []Job{}
	}
	return jobs, err
}

// Stats returns aggregate counts by state.
func (s *Store) Stats(ctx context.Context) (*QueueStats, error) {
	var stats QueueStats
	err := s.pool.QueryRow(ctx, `
		SELECT
			COALESCE(SUM(CASE WHEN state = 'queued' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN state = 'running' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN state = 'completed' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN state = 'failed' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN state = 'canceled' THEN 1 ELSE 0 END), 0)
		FROM _ayb_jobs
	`).Scan(&stats.Queued, &stats.Running, &stats.Completed, &stats.Failed, &stats.Canceled)
	if err != nil {
		return nil, err
	}

	// Oldest queued job age.
	var age *float64
	err = s.pool.QueryRow(ctx,
		`SELECT EXTRACT(EPOCH FROM NOW() - MIN(run_at))
		 FROM _ayb_jobs WHERE state = 'queued'`,
	).Scan(&age)
	if err != nil && err != pgx.ErrNoRows {
		return nil, err
	}
	stats.OldestAge = age

	return &stats, nil
}

// --- Schedule operations ---

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
