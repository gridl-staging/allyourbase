// Package backup Repository implements database-backed backup metadata storage with CRUD operations and filtering for both logical and physical backups.
package backup

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// BackupRecord represents a row in _ayb_backups.
// BackupRecord holds backup operation metadata from the _ayb_backups table, including status, artifact details, timing, and LSN boundaries for point-in-time recovery.
type BackupRecord struct {
	ID              string     `json:"id"`
	DBName          string     `json:"db_name"`
	ObjectKey       string     `json:"object_key"`
	SizeBytes       int64      `json:"size_bytes"`
	Checksum        string     `json:"checksum"`
	StartedAt       time.Time  `json:"started_at"`
	CompletedAt     *time.Time `json:"completed_at,omitempty"`
	Status          string     `json:"status"`
	ErrorMessage    string     `json:"error_message,omitempty"`
	TriggeredBy     string     `json:"triggered_by"`
	RestoreSourceID string     `json:"restore_source_id,omitempty"`
	// PITR fields — populated for physical backups.
	BackupType string  `json:"backup_type"`         // "logical" | "physical"
	StartLSN   *string `json:"start_lsn,omitempty"` // nullable — only physical
	EndLSN     *string `json:"end_lsn,omitempty"`   // nullable — only physical
	ProjectID  string  `json:"project_id,omitempty"`
	DatabaseID string  `json:"database_id,omitempty"`
}

// Repo is the backup metadata repository interface.
type Repo interface {
	Create(ctx context.Context, dbName, triggeredBy string) (*BackupRecord, error)
	// CreatePhysical inserts a new physical backup record for the given project/database.
	CreatePhysical(ctx context.Context, projectID, databaseID, triggeredBy string) (*BackupRecord, error)
	UpdateStatus(ctx context.Context, id, status, errMsg string) error
	MarkCompleted(ctx context.Context, id, objectKey string, sizeBytes int64, checksum string, completedAt time.Time) error
	// MarkPhysicalCompleted transitions a physical backup to completed, recording LSN boundaries.
	MarkPhysicalCompleted(ctx context.Context, id, objectKey string, sizeBytes int64, checksum, startLSN, endLSN string, completedAt time.Time) error
	Get(ctx context.Context, id string) (*BackupRecord, error)
	List(ctx context.Context, f ListFilter) ([]BackupRecord, int, error)
	CompletedByDB(ctx context.Context, dbName string) ([]BackupRecord, error)
	RecordRestore(ctx context.Context, dbName, sourceID, triggeredBy string) (string, error)
	// ListPhysicalCompleted returns all completed physical backups for a project/database, newest first.
	ListPhysicalCompleted(ctx context.Context, projectID, databaseID string) ([]BackupRecord, error)
}

// ListFilter configures backup list queries.
type ListFilter struct {
	Status string
	DBName string
	Limit  int
	Offset int
}

// PgRepo is a PostgreSQL-backed Repo.
type PgRepo struct {
	pool *pgxpool.Pool
}

// NewPgRepo creates a PgRepo.
func NewPgRepo(pool *pgxpool.Pool) Repo {
	return &PgRepo{pool: pool}
}

// NewRepository is an alias for NewPgRepo used by CLI wiring.
var NewRepository = NewPgRepo

// Create inserts a new logical backup record with status "running" and returns it.
func (r *PgRepo) Create(ctx context.Context, dbName, triggeredBy string) (*BackupRecord, error) {
	row := r.pool.QueryRow(ctx,
		"INSERT INTO _ayb_backups (db_name, status, triggered_by, started_at, backup_type)"+
			" VALUES ($1, $2, $3, NOW(), 'logical')"+
			" RETURNING "+backupColumns,
		dbName, StatusRunning, triggeredBy,
	)
	return scanBackupRecord(row)
}

// CreatePhysical inserts a new physical backup record for the given project/database.
func (r *PgRepo) CreatePhysical(ctx context.Context, projectID, databaseID, triggeredBy string) (*BackupRecord, error) {
	row := r.pool.QueryRow(ctx,
		"INSERT INTO _ayb_backups (db_name, project_id, database_id, status, triggered_by, started_at, backup_type)"+
			" VALUES ($1, $2, $3, $4, $5, NOW(), 'physical')"+
			" RETURNING "+backupColumns,
		databaseID, projectID, databaseID, StatusRunning, triggeredBy,
	)
	return scanBackupRecord(row)
}

// UpdateStatus sets status and error_message for a backup record.
func (r *PgRepo) UpdateStatus(ctx context.Context, id, status, errMsg string) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE _ayb_backups SET status = $2, error_message = $3, completed_at = NOW()
		 WHERE id = $1`,
		id, status, errMsg,
	)
	if err != nil {
		return fmt.Errorf("updating backup status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("backup record %q not found", id)
	}
	return nil
}

// MarkCompleted transitions a backup to completed with all artifact metadata.
func (r *PgRepo) MarkCompleted(ctx context.Context, id, objectKey string, sizeBytes int64, checksum string, completedAt time.Time) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE _ayb_backups
		 SET status = $2, object_key = $3, size_bytes = $4, checksum = $5, completed_at = $6, error_message = ''
		 WHERE id = $1`,
		id, StatusCompleted, objectKey, sizeBytes, checksum, completedAt,
	)
	if err != nil {
		return fmt.Errorf("marking backup completed: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("backup record %q not found", id)
	}
	return nil
}

// MarkPhysicalCompleted transitions a physical backup to completed, recording LSN boundaries.
func (r *PgRepo) MarkPhysicalCompleted(ctx context.Context, id, objectKey string, sizeBytes int64, checksum, startLSN, endLSN string, completedAt time.Time) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE _ayb_backups
		 SET status = $2, object_key = $3, size_bytes = $4, checksum = $5,
		     start_lsn = $6::pg_lsn, end_lsn = $7::pg_lsn,
		     completed_at = $8, error_message = ''
		 WHERE id = $1`,
		id, StatusCompleted, objectKey, sizeBytes, checksum,
		startLSN, endLSN, completedAt,
	)
	if err != nil {
		return fmt.Errorf("marking physical backup completed: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("backup record %q not found", id)
	}
	return nil
}

// Get retrieves a single backup record by ID.
func (r *PgRepo) Get(ctx context.Context, id string) (*BackupRecord, error) {
	row := r.pool.QueryRow(ctx,
		"SELECT "+backupColumns+" FROM _ayb_backups WHERE id = $1", id,
	)
	return scanBackupRecord(row)
}

// List returns backup records matching the filter, plus total count.
func (r *PgRepo) List(ctx context.Context, f ListFilter) ([]BackupRecord, int, error) {
	cond, args := buildWhere(f)

	var total int
	if err := r.pool.QueryRow(ctx, "SELECT COUNT(*) FROM _ayb_backups"+cond, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting backups: %w", err)
	}

	limit := f.Limit
	if limit <= 0 {
		limit = 50
	}
	limitArgs := append(args, limit, f.Offset)
	query := fmt.Sprintf(
		"SELECT %s FROM _ayb_backups%s ORDER BY started_at DESC LIMIT $%d OFFSET $%d",
		backupColumns, cond, len(limitArgs)-1, len(limitArgs),
	)

	rows, err := r.pool.Query(ctx, query, limitArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("listing backups: %w", err)
	}
	defer rows.Close()

	var records []BackupRecord
	for rows.Next() {
		rec, err := scanBackupRecord(rows)
		if err != nil {
			return nil, 0, err
		}
		records = append(records, *rec)
	}
	return records, total, rows.Err()
}

// CompletedByDB returns all completed backups for a database, newest first.
func (r *PgRepo) CompletedByDB(ctx context.Context, dbName string) ([]BackupRecord, error) {
	rows, err := r.pool.Query(ctx,
		"SELECT "+backupColumns+" FROM _ayb_backups"+
			" WHERE db_name = $1 AND status = $2"+
			" ORDER BY started_at DESC",
		dbName, StatusCompleted,
	)
	if err != nil {
		return nil, fmt.Errorf("querying completed backups: %w", err)
	}
	defer rows.Close()

	var records []BackupRecord
	for rows.Next() {
		rec, err := scanBackupRecord(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, *rec)
	}
	return records, rows.Err()
}

// RecordRestore inserts a record for a restore operation.
func (r *PgRepo) RecordRestore(ctx context.Context, dbName, sourceID, triggeredBy string) (string, error) {
	var id string
	err := r.pool.QueryRow(ctx,
		`INSERT INTO _ayb_backups (db_name, status, triggered_by, restore_source_id, started_at)
		 VALUES ($1, $2, $3, $4, NOW())
		 RETURNING id`,
		dbName, StatusRunning, triggeredBy, sourceID,
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("recording restore: %w", err)
	}
	return id, nil
}

// ListPhysicalCompleted returns all completed physical backups for a project/database, newest first.
func (r *PgRepo) ListPhysicalCompleted(ctx context.Context, projectID, databaseID string) ([]BackupRecord, error) {
	rows, err := r.pool.Query(ctx,
		"SELECT "+backupColumns+" FROM _ayb_backups"+
			" WHERE project_id = $1 AND database_id = $2 AND backup_type = 'physical' AND status = $3"+
			" ORDER BY completed_at DESC",
		projectID, databaseID, StatusCompleted,
	)
	if err != nil {
		return nil, fmt.Errorf("listing completed physical backups: %w", err)
	}
	defer rows.Close()

	var records []BackupRecord
	for rows.Next() {
		rec, err := scanBackupRecord(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, *rec)
	}
	return records, rows.Err()
}

// --- internal helpers ---

const backupColumns = "id, db_name, object_key, size_bytes, checksum," +
	" started_at, completed_at, status, error_message, triggered_by, restore_source_id," +
	" backup_type, start_lsn::text, end_lsn::text, project_id, database_id"

// scanBackupRecord scans a single BackupRecord from any pgx row/rows source.
func scanBackupRecord(row interface{ Scan(dest ...any) error }) (*BackupRecord, error) {
	var rec BackupRecord
	err := row.Scan(
		&rec.ID, &rec.DBName, &rec.ObjectKey, &rec.SizeBytes, &rec.Checksum,
		&rec.StartedAt, &rec.CompletedAt, &rec.Status, &rec.ErrorMessage,
		&rec.TriggeredBy, &rec.RestoreSourceID,
		&rec.BackupType, &rec.StartLSN, &rec.EndLSN, &rec.ProjectID, &rec.DatabaseID,
	)
	if err != nil {
		return nil, fmt.Errorf("scanning backup record: %w", err)
	}
	return &rec, nil
}

func buildWhere(f ListFilter) (string, []any) {
	cond := " WHERE 1=1"
	var args []any
	n := 1
	if f.Status != "" {
		cond += fmt.Sprintf(" AND status = $%d", n)
		args = append(args, f.Status)
		n++
	}
	if f.DBName != "" {
		cond += fmt.Sprintf(" AND db_name = $%d", n)
		args = append(args, f.DBName)
	}
	return cond, args
}
