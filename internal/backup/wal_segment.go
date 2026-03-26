// Package backup This file defines the WALSegmentRepo interface for managing WAL segment archive metadata and provides a PostgreSQL implementation.
package backup

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// WALSegment represents a row in _ayb_wal_segments.
type WALSegment struct {
	ID          string    `json:"id"`
	ProjectID   string    `json:"project_id"`
	DatabaseID  string    `json:"database_id"`
	Timeline    int       `json:"timeline"`
	SegmentName string    `json:"segment_name"`
	StartLSN    string    `json:"start_lsn"`
	EndLSN      string    `json:"end_lsn"`
	Checksum    string    `json:"checksum"`
	SizeBytes   int64     `json:"size_bytes"`
	ArchivedAt  time.Time `json:"archived_at"`
}

// WALSegmentRepo manages WAL segment archive metadata.
// WALSegmentRepo manages WAL segment archive metadata.
type WALSegmentRepo interface {
	// Record inserts a WAL segment record. Idempotent: re-inserting the same
	// (project_id, database_id, timeline, segment_name) is a no-op.
	Record(ctx context.Context, seg WALSegment) error

	// GetByName retrieves a single segment by its identifying coordinates.
	GetByName(ctx context.Context, projectID, databaseID string, timeline int, segmentName string) (*WALSegment, error)

	// ListRange returns all segments whose start_lsn falls within [startLSN, endLSN].
	ListRange(ctx context.Context, projectID, databaseID string, startLSN, endLSN string) ([]WALSegment, error)

	// ListOlderThan returns all segments archived before the provided cutoff.
	// Results are ordered by archived_at ASC.
	ListOlderThan(ctx context.Context, projectID, databaseID string, before time.Time) ([]WALSegment, error)

	// SumSizeBytes returns the total size of archived WAL segments for project/database.
	SumSizeBytes(ctx context.Context, projectID, databaseID string) (int64, error)

	// Delete removes a WAL segment row by ID.
	Delete(ctx context.Context, id string) error

	// LatestByProject returns the most recently archived segment for the given project/database.
	LatestByProject(ctx context.Context, projectID, databaseID string) (*WALSegment, error)

	// CoveringSegment returns the WAL segment whose LSN range contains the given LSN
	// (start_lsn <= lsn < end_lsn). Used by ManifestWriter to determine timeline for
	// the manifest and by IntegrityVerifier to find the starting segment for WAL chain verification.
	CoveringSegment(ctx context.Context, projectID, databaseID, lsn string) (*WALSegment, error)
}

// walSegmentColumns is the canonical column list for _ayb_wal_segments queries.
// Mirrors the pattern used by backupColumns in repository.go.
const walSegmentColumns = "id, project_id, database_id, timeline, segment_name," +
	" start_lsn::text, end_lsn::text, checksum, size_bytes, archived_at"

// PgWALSegmentRepo is a PostgreSQL-backed WALSegmentRepo.
type PgWALSegmentRepo struct {
	pool *pgxpool.Pool
}

// NewPgWALSegmentRepo creates a PgWALSegmentRepo.
func NewPgWALSegmentRepo(pool *pgxpool.Pool) WALSegmentRepo {
	return &PgWALSegmentRepo{pool: pool}
}

// Record inserts a WAL segment record. Duplicate (project_id, database_id, timeline,
// segment_name) tuples are silently ignored to ensure idempotency.
func (r *PgWALSegmentRepo) Record(ctx context.Context, seg WALSegment) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO _ayb_wal_segments
			(project_id, database_id, timeline, segment_name, start_lsn, end_lsn, checksum, size_bytes, archived_at)
		VALUES ($1, $2, $3, $4, $5::pg_lsn, $6::pg_lsn, $7, $8, $9)
		ON CONFLICT (project_id, database_id, timeline, segment_name) DO NOTHING`,
		seg.ProjectID, seg.DatabaseID, seg.Timeline, seg.SegmentName,
		seg.StartLSN, seg.EndLSN, seg.Checksum, seg.SizeBytes, seg.ArchivedAt,
	)
	if err != nil {
		return fmt.Errorf("recording WAL segment %q: %w", seg.SegmentName, err)
	}
	return nil
}

// GetByName retrieves a WAL segment by its unique coordinates.
func (r *PgWALSegmentRepo) GetByName(ctx context.Context, projectID, databaseID string, timeline int, segmentName string) (*WALSegment, error) {
	row := r.pool.QueryRow(ctx,
		"SELECT "+walSegmentColumns+" FROM _ayb_wal_segments"+
			" WHERE project_id = $1 AND database_id = $2 AND timeline = $3 AND segment_name = $4",
		projectID, databaseID, timeline, segmentName,
	)
	return scanWALSegment(row)
}

// ListRange returns segments whose start_lsn falls within [startLSN, endLSN].
func (r *PgWALSegmentRepo) ListRange(ctx context.Context, projectID, databaseID string, startLSN, endLSN string) ([]WALSegment, error) {
	rows, err := r.pool.Query(ctx,
		"SELECT "+walSegmentColumns+" FROM _ayb_wal_segments"+
			" WHERE project_id = $1 AND database_id = $2"+
			" AND start_lsn >= $3::pg_lsn AND start_lsn <= $4::pg_lsn"+
			" ORDER BY start_lsn ASC",
		projectID, databaseID, startLSN, endLSN,
	)
	if err != nil {
		return nil, fmt.Errorf("listing WAL segments for range [%s, %s]: %w", startLSN, endLSN, err)
	}
	defer rows.Close()

	var segs []WALSegment
	for rows.Next() {
		seg, err := scanWALSegment(rows)
		if err != nil {
			return nil, err
		}
		segs = append(segs, *seg)
	}
	return segs, rows.Err()
}

// ListOlderThan returns all segments archived before cutoff, ordered by archived_at ASC.
func (r *PgWALSegmentRepo) ListOlderThan(ctx context.Context, projectID, databaseID string, before time.Time) ([]WALSegment, error) {
	rows, err := r.pool.Query(ctx,
		"SELECT "+walSegmentColumns+" FROM _ayb_wal_segments"+
			" WHERE project_id = $1 AND database_id = $2 AND archived_at < $3"+
			" ORDER BY archived_at ASC",
		projectID, databaseID, before.UTC(),
	)
	if err != nil {
		return nil, fmt.Errorf("listing WAL segments older than %s: %w", before.UTC(), err)
	}
	defer rows.Close()

	var segs []WALSegment
	for rows.Next() {
		seg, err := scanWALSegment(rows)
		if err != nil {
			return nil, err
		}
		segs = append(segs, *seg)
	}
	return segs, rows.Err()
}

// SumSizeBytes returns the total WAL archived size for project/database.
func (r *PgWALSegmentRepo) SumSizeBytes(ctx context.Context, projectID, databaseID string) (int64, error) {
	var total int64
	if err := r.pool.QueryRow(ctx,
		"SELECT COALESCE(SUM(size_bytes), 0) FROM _ayb_wal_segments"+
			" WHERE project_id = $1 AND database_id = $2",
		projectID, databaseID,
	).Scan(&total); err != nil {
		return 0, fmt.Errorf("summing WAL segment bytes: %w", err)
	}
	return total, nil
}

// Delete removes a WAL segment row by ID.
func (r *PgWALSegmentRepo) Delete(ctx context.Context, id string) error {
	tag, err := r.pool.Exec(ctx, "DELETE FROM _ayb_wal_segments WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("deleting WAL segment row %q: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("WAL segment row %q not found", id)
	}
	return nil
}

// LatestByProject returns the most recently archived segment for the given project/database.
func (r *PgWALSegmentRepo) LatestByProject(ctx context.Context, projectID, databaseID string) (*WALSegment, error) {
	row := r.pool.QueryRow(ctx,
		"SELECT "+walSegmentColumns+" FROM _ayb_wal_segments"+
			" WHERE project_id = $1 AND database_id = $2"+
			" ORDER BY archived_at DESC LIMIT 1",
		projectID, databaseID,
	)
	return scanWALSegment(row)
}

// CoveringSegment returns the WAL segment whose LSN range contains the given LSN
// (start_lsn <= lsn < end_lsn). Returns nil if no segment covers the given LSN.
func (r *PgWALSegmentRepo) CoveringSegment(ctx context.Context, projectID, databaseID, lsn string) (*WALSegment, error) {
	row := r.pool.QueryRow(ctx,
		"SELECT "+walSegmentColumns+" FROM _ayb_wal_segments"+
			" WHERE project_id = $1 AND database_id = $2"+
			" AND start_lsn <= $3::pg_lsn AND end_lsn > $3::pg_lsn"+
			" ORDER BY start_lsn ASC LIMIT 1",
		projectID, databaseID, lsn,
	)
	return scanWALSegment(row)
}

// scanWALSegment scans a single WALSegment from a pgx.Row.
func scanWALSegment(row interface {
	Scan(dest ...any) error
}) (*WALSegment, error) {
	var seg WALSegment
	err := row.Scan(
		&seg.ID, &seg.ProjectID, &seg.DatabaseID, &seg.Timeline, &seg.SegmentName,
		&seg.StartLSN, &seg.EndLSN, &seg.Checksum, &seg.SizeBytes, &seg.ArchivedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("scanning WAL segment: %w", err)
	}
	return &seg, nil
}
