// Package backup RestoreJob models a database row for restore operations, and PgRestoreJobRepo provides SQL-backed persistence for managing restore job state transitions.
package backup

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	RestorePhasePending         = "pending"
	RestorePhaseValidating      = "validating"
	RestorePhaseRestoring       = "restoring"
	RestorePhaseVerifying       = "verifying"
	RestorePhaseReadyForCutover = "ready_for_cutover"
	RestorePhaseCompleted       = "completed"
	RestorePhaseFailed          = "failed"
)

const (
	RestoreStatusPending   = "pending"
	RestoreStatusRunning   = "running"
	RestoreStatusCompleted = "completed"
	RestoreStatusFailed    = "failed"
)

// RestoreJob models a row in _ayb_restore_jobs.
// RestoreJob represents a database row for a restore operation, including its phase, status, base backup information, WAL segments, and verification results.
type RestoreJob struct {
	ID                 string          `json:"id"`
	ProjectID          string          `json:"project_id"`
	DatabaseID         string          `json:"database_id"`
	Environment        string          `json:"environment"`
	TargetTime         time.Time       `json:"target_time"`
	Phase              string          `json:"phase"`
	Status             string          `json:"status"`
	BaseBackupID       string          `json:"base_backup_id"`
	WALSegmentsNeeded  int             `json:"wal_segments_needed"`
	VerificationResult json.RawMessage `json:"verification_result,omitempty"`
	Logs               string          `json:"logs"`
	ErrorMessage       string          `json:"error_message"`
	RequestedBy        string          `json:"requested_by"`
	StartedAt          time.Time       `json:"started_at"`
	CompletedAt        *time.Time      `json:"completed_at,omitempty"`
}

// RestoreJobRepo persists restore-job state transitions.
type RestoreJobRepo interface {
	Create(ctx context.Context, job RestoreJob) (*RestoreJob, error)
	Get(ctx context.Context, id string) (*RestoreJob, error)
	UpdatePhase(ctx context.Context, id, phase, status string) error
	SetBaseBackup(ctx context.Context, id, baseBackupID string, walSegmentsNeeded int) error
	MarkCompleted(ctx context.Context, id string, verificationResult json.RawMessage) error
	MarkFailed(ctx context.Context, id, errorMessage string) error
	AppendLog(ctx context.Context, id, line string) error
	ListByProject(ctx context.Context, projectID, databaseID string) ([]RestoreJob, error)
}

type restoreJobDB interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// PgRestoreJobRepo is a PostgreSQL implementation of RestoreJobRepo.
type PgRestoreJobRepo struct {
	db restoreJobDB
}

// NewPgRestoreJobRepo creates a pgx-backed restore-job repository.
func NewPgRestoreJobRepo(pool *pgxpool.Pool) RestoreJobRepo {
	return &PgRestoreJobRepo{db: pool}
}

const restoreJobColumns = "id, project_id, database_id, environment, target_time, phase, status," +
	" base_backup_id, wal_segments_needed, verification_result, logs, error_message, requested_by, started_at, completed_at"

// Create inserts a new restore job with the provided parameters, defaulting phase and status to pending if not specified, and returns the created job with its assigned ID.
func (r *PgRestoreJobRepo) Create(ctx context.Context, job RestoreJob) (*RestoreJob, error) {
	phase := job.Phase
	if phase == "" {
		phase = RestorePhasePending
	}
	status := job.Status
	if status == "" {
		status = RestoreStatusPending
	}
	row := r.db.QueryRow(ctx,
		`INSERT INTO _ayb_restore_jobs (project_id, database_id, environment, target_time, phase, status, requested_by)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 RETURNING `+restoreJobColumns,
		job.ProjectID, job.DatabaseID, job.Environment, job.TargetTime, phase, status, job.RequestedBy,
	)
	created, err := scanRestoreJob(row)
	if err != nil {
		return nil, fmt.Errorf("creating restore job: %w", err)
	}
	return created, nil
}

func (r *PgRestoreJobRepo) Get(ctx context.Context, id string) (*RestoreJob, error) {
	row := r.db.QueryRow(ctx,
		"SELECT "+restoreJobColumns+" FROM _ayb_restore_jobs WHERE id = $1",
		id,
	)
	job, err := scanRestoreJob(row)
	if err != nil {
		return nil, fmt.Errorf("getting restore job %q: %w", id, err)
	}
	return job, nil
}

func (r *PgRestoreJobRepo) UpdatePhase(ctx context.Context, id, phase, status string) error {
	tag, err := r.db.Exec(ctx,
		`UPDATE _ayb_restore_jobs SET phase = $2, status = $3 WHERE id = $1`,
		id, phase, status,
	)
	if err != nil {
		return fmt.Errorf("updating restore job phase: %w", err)
	}
	return requireRestoreJobUpdated(tag, id)
}

func (r *PgRestoreJobRepo) SetBaseBackup(ctx context.Context, id, baseBackupID string, walSegmentsNeeded int) error {
	tag, err := r.db.Exec(ctx,
		`UPDATE _ayb_restore_jobs SET base_backup_id = $2, wal_segments_needed = $3 WHERE id = $1`,
		id, baseBackupID, walSegmentsNeeded,
	)
	if err != nil {
		return fmt.Errorf("setting restore job base backup: %w", err)
	}
	return requireRestoreJobUpdated(tag, id)
}

func (r *PgRestoreJobRepo) MarkCompleted(ctx context.Context, id string, verificationResult json.RawMessage) error {
	tag, err := r.db.Exec(ctx,
		`UPDATE _ayb_restore_jobs
		 SET phase = 'completed', status = 'completed', verification_result = $2, completed_at = NOW()
		 WHERE id = $1`,
		id, verificationResult,
	)
	if err != nil {
		return fmt.Errorf("marking restore job completed: %w", err)
	}
	return requireRestoreJobUpdated(tag, id)
}

func (r *PgRestoreJobRepo) MarkFailed(ctx context.Context, id, errorMessage string) error {
	tag, err := r.db.Exec(ctx,
		`UPDATE _ayb_restore_jobs
		 SET phase = 'failed', status = 'failed', error_message = $2, completed_at = NOW()
		 WHERE id = $1`,
		id, errorMessage,
	)
	if err != nil {
		return fmt.Errorf("marking restore job failed: %w", err)
	}
	return requireRestoreJobUpdated(tag, id)
}

func (r *PgRestoreJobRepo) AppendLog(ctx context.Context, id, line string) error {
	tag, err := r.db.Exec(ctx,
		`UPDATE _ayb_restore_jobs SET logs = logs || $2 WHERE id = $1`,
		id, line,
	)
	if err != nil {
		return fmt.Errorf("appending restore job log: %w", err)
	}
	return requireRestoreJobUpdated(tag, id)
}

// ListByProject returns all restore jobs for the specified project and database, ordered by start time in descending order.
func (r *PgRestoreJobRepo) ListByProject(ctx context.Context, projectID, databaseID string) ([]RestoreJob, error) {
	rows, err := r.db.Query(ctx,
		"SELECT "+restoreJobColumns+" FROM _ayb_restore_jobs WHERE project_id = $1 AND database_id = $2 ORDER BY started_at DESC",
		projectID, databaseID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing restore jobs: %w", err)
	}
	defer rows.Close()

	jobs := make([]RestoreJob, 0)
	for rows.Next() {
		job, scanErr := scanRestoreJob(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		jobs = append(jobs, *job)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating restore jobs: %w", err)
	}
	return jobs, nil
}

func requireRestoreJobUpdated(tag pgconn.CommandTag, id string) error {
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("restore job %q not found", id)
	}
	return nil
}

// scanRestoreJob decodes a database row into a RestoreJob struct, handling the verification result as a JSON byte slice.
func scanRestoreJob(row interface{ Scan(dest ...any) error }) (*RestoreJob, error) {
	var job RestoreJob
	var verification []byte
	err := row.Scan(
		&job.ID,
		&job.ProjectID,
		&job.DatabaseID,
		&job.Environment,
		&job.TargetTime,
		&job.Phase,
		&job.Status,
		&job.BaseBackupID,
		&job.WALSegmentsNeeded,
		&verification,
		&job.Logs,
		&job.ErrorMessage,
		&job.RequestedBy,
		&job.StartedAt,
		&job.CompletedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("scanning restore job: %w", err)
	}
	if len(verification) > 0 {
		job.VerificationResult = append(json.RawMessage(nil), verification...)
	}
	return &job, nil
}
