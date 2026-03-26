// Package backup Defines the BackupManifest type and implements the ManifestRepo interface for PostgreSQL, managing backup metadata storage and retrieval.
package backup

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type BackupManifest struct {
	BackupID   string    `json:"backup_id"`
	ProjectID  string    `json:"project_id"`
	DatabaseID string    `json:"database_id"`
	ObjectKey  string    `json:"object_key"`
	StartLSN   string    `json:"start_lsn"`
	EndLSN     string    `json:"end_lsn"`
	Checksum   string    `json:"checksum"`
	Timeline   int       `json:"timeline"`
	CreatedAt  time.Time `json:"created_at"`
}

type ManifestRepo interface {
	Upsert(ctx context.Context, m BackupManifest) error
	GetByBackupID(ctx context.Context, backupID string) (*BackupManifest, error)
	ListByProjectDatabase(ctx context.Context, projectID, databaseID string) ([]BackupManifest, error)
}

type PgManifestRepo struct {
	pool *pgxpool.Pool
}

func NewPgManifestRepo(pool *pgxpool.Pool) ManifestRepo {
	return &PgManifestRepo{pool: pool}
}

// Upsert inserts a new backup manifest into the database or updates an existing one if a manifest with the same backup_id already exists, updating only when one or more fields differ from the existing record.
func (r *PgManifestRepo) Upsert(ctx context.Context, m BackupManifest) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO _ayb_backup_manifests
			(project_id, database_id, backup_id, object_key, start_lsn, end_lsn, checksum, timeline, created_at)
		VALUES ($1, $2, $3, $4, $5::pg_lsn, $6::pg_lsn, $7, $8, $9)
		ON CONFLICT (backup_id) DO UPDATE SET
			object_key = EXCLUDED.object_key,
			start_lsn = EXCLUDED.start_lsn,
			end_lsn = EXCLUDED.end_lsn,
			checksum = EXCLUDED.checksum,
			timeline = EXCLUDED.timeline,
			created_at = EXCLUDED.created_at
		WHERE
			_ayb_backup_manifests.object_key != EXCLUDED.object_key OR
			_ayb_backup_manifests.start_lsn != EXCLUDED.start_lsn OR
			_ayb_backup_manifests.end_lsn != EXCLUDED.end_lsn OR
			_ayb_backup_manifests.checksum != EXCLUDED.checksum OR
			_ayb_backup_manifests.timeline != EXCLUDED.timeline`,
		m.ProjectID, m.DatabaseID, m.BackupID, m.ObjectKey,
		m.StartLSN, m.EndLSN, m.Checksum, m.Timeline, m.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("upserting backup manifest: %w", err)
	}
	return nil
}

// GetByBackupID retrieves a backup manifest from the database by its backup_id, returning nil if no manifest with the given backup_id is found.
func (r *PgManifestRepo) GetByBackupID(ctx context.Context, backupID string) (*BackupManifest, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT backup_id, project_id, database_id, object_key,
		       start_lsn::text, end_lsn::text, checksum, timeline, created_at
		FROM _ayb_backup_manifests WHERE backup_id = $1`, backupID,
	)
	var m BackupManifest
	err := row.Scan(
		&m.BackupID, &m.ProjectID, &m.DatabaseID, &m.ObjectKey,
		&m.StartLSN, &m.EndLSN, &m.Checksum, &m.Timeline, &m.CreatedAt,
	)
	if err != nil {
		if isNoRowsError(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("getting backup manifest by backup_id: %w", err)
	}
	return &m, nil
}

// ListByProjectDatabase retrieves all backup manifests for a given project and database, ordered by creation time in descending order.
func (r *PgManifestRepo) ListByProjectDatabase(ctx context.Context, projectID, databaseID string) ([]BackupManifest, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT backup_id, project_id, database_id, object_key,
		       start_lsn::text, end_lsn::text, checksum, timeline, created_at
		FROM _ayb_backup_manifests
		WHERE project_id = $1 AND database_id = $2
		ORDER BY created_at DESC`,
		projectID, databaseID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing backup manifests: %w", err)
	}
	defer rows.Close()

	var manifests []BackupManifest
	for rows.Next() {
		var m BackupManifest
		err := rows.Scan(
			&m.BackupID, &m.ProjectID, &m.DatabaseID, &m.ObjectKey,
			&m.StartLSN, &m.EndLSN, &m.Checksum, &m.Timeline, &m.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scanning backup manifest: %w", err)
		}
		manifests = append(manifests, m)
	}
	return manifests, rows.Err()
}

func isNoRowsError(err error) bool {
	return errors.Is(err, pgx.ErrNoRows)
}
