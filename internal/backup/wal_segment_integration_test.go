//go:build integration

package backup

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func setupWALSegmentTable(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	_, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS _ayb_wal_segments (
			id           TEXT        NOT NULL DEFAULT gen_random_uuid()::text PRIMARY KEY,
			project_id   TEXT        NOT NULL,
			database_id  TEXT        NOT NULL,
			timeline     INTEGER     NOT NULL,
			segment_name TEXT        NOT NULL,
			start_lsn    pg_lsn      NOT NULL,
			end_lsn      pg_lsn      NOT NULL,
			checksum     TEXT        NOT NULL,
			size_bytes   BIGINT      NOT NULL,
			archived_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			CONSTRAINT _ayb_wal_segments_unique UNIQUE (project_id, database_id, timeline, segment_name)
		)`)
	if err != nil {
		t.Fatalf("creating _ayb_wal_segments table: %v", err)
	}
	_, _ = pool.Exec(ctx, "DELETE FROM _ayb_wal_segments")
}

func makeTestSegment(projectID, databaseID string, timeline int, name, startLSN, endLSN string) WALSegment {
	return WALSegment{
		ProjectID:   projectID,
		DatabaseID:  databaseID,
		Timeline:    timeline,
		SegmentName: name,
		StartLSN:    startLSN,
		EndLSN:      endLSN,
		Checksum:    "sha256:" + name,
		SizeBytes:   16 * 1024 * 1024, // 16 MiB — standard WAL segment size
		ArchivedAt:  time.Now().UTC().Truncate(time.Millisecond),
	}
}

func TestIntegrationWALSegmentRecord(t *testing.T) {
	dbURL := testDBURL(t)
	ctx := context.Background()

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("connecting to DB: %v", err)
	}
	defer pool.Close()
	setupWALSegmentTable(t, pool)

	repo := NewPgWALSegmentRepo(pool)
	seg := makeTestSegment("proj1", "db1", 1, "000000010000000000000001", "0/1000000", "0/2000000")

	if err := repo.Record(ctx, seg); err != nil {
		t.Fatalf("Record: %v", err)
	}

	got, err := repo.GetByName(ctx, "proj1", "db1", 1, "000000010000000000000001")
	if err != nil {
		t.Fatalf("GetByName: %v", err)
	}
	if got.SegmentName != seg.SegmentName {
		t.Errorf("segment_name = %q; want %q", got.SegmentName, seg.SegmentName)
	}
	if got.StartLSN != seg.StartLSN {
		t.Errorf("start_lsn = %q; want %q", got.StartLSN, seg.StartLSN)
	}
	if got.Checksum != seg.Checksum {
		t.Errorf("checksum = %q; want %q", got.Checksum, seg.Checksum)
	}
	if got.SizeBytes != seg.SizeBytes {
		t.Errorf("size_bytes = %d; want %d", got.SizeBytes, seg.SizeBytes)
	}
}

func TestIntegrationWALSegmentRecordIdempotent(t *testing.T) {
	dbURL := testDBURL(t)
	ctx := context.Background()

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("connecting to DB: %v", err)
	}
	defer pool.Close()
	setupWALSegmentTable(t, pool)

	repo := NewPgWALSegmentRepo(pool)
	seg := makeTestSegment("proj1", "db1", 1, "000000010000000000000002", "0/2000000", "0/3000000")

	// Record twice — must not error.
	if err := repo.Record(ctx, seg); err != nil {
		t.Fatalf("first Record: %v", err)
	}
	if err := repo.Record(ctx, seg); err != nil {
		t.Fatalf("second Record (idempotent): %v", err)
	}

	// Verify only one row exists.
	var count int
	if err := pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM _ayb_wal_segments WHERE project_id = $1 AND segment_name = $2",
		"proj1", "000000010000000000000002",
	).Scan(&count); err != nil {
		t.Fatalf("count query: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 row after idempotent insert, got %d", count)
	}
}

func TestIntegrationWALSegmentListRange(t *testing.T) {
	dbURL := testDBURL(t)
	ctx := context.Background()

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("connecting to DB: %v", err)
	}
	defer pool.Close()
	setupWALSegmentTable(t, pool)

	repo := NewPgWALSegmentRepo(pool)

	// Insert three consecutive segments.
	segs := []WALSegment{
		makeTestSegment("proj2", "db2", 1, "000000010000000000000001", "0/1000000", "0/2000000"),
		makeTestSegment("proj2", "db2", 1, "000000010000000000000002", "0/2000000", "0/3000000"),
		makeTestSegment("proj2", "db2", 1, "000000010000000000000003", "0/3000000", "0/4000000"),
	}
	for _, s := range segs {
		if err := repo.Record(ctx, s); err != nil {
			t.Fatalf("Record %q: %v", s.SegmentName, err)
		}
	}

	// Query the middle two segments by LSN range.
	got, err := repo.ListRange(ctx, "proj2", "db2", "0/1000000", "0/2000000")
	if err != nil {
		t.Fatalf("ListRange: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("ListRange returned %d segments; want 2", len(got))
	}

	// Verify order: ascending by start_lsn.
	if len(got) == 2 && got[0].SegmentName > got[1].SegmentName {
		t.Errorf("segments not in ascending LSN order: %q, %q", got[0].SegmentName, got[1].SegmentName)
	}
}

func TestIntegrationWALSegmentLatestByProject(t *testing.T) {
	dbURL := testDBURL(t)
	ctx := context.Background()

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("connecting to DB: %v", err)
	}
	defer pool.Close()
	setupWALSegmentTable(t, pool)

	repo := NewPgWALSegmentRepo(pool)

	now := time.Now().UTC()
	earlier := makeTestSegment("proj3", "db3", 1, "000000010000000000000001", "0/1000000", "0/2000000")
	earlier.ArchivedAt = now.Add(-5 * time.Minute).Truncate(time.Millisecond)

	later := makeTestSegment("proj3", "db3", 1, "000000010000000000000002", "0/2000000", "0/3000000")
	later.ArchivedAt = now.Truncate(time.Millisecond)

	if err := repo.Record(ctx, earlier); err != nil {
		t.Fatalf("Record earlier: %v", err)
	}
	if err := repo.Record(ctx, later); err != nil {
		t.Fatalf("Record later: %v", err)
	}

	got, err := repo.LatestByProject(ctx, "proj3", "db3")
	if err != nil {
		t.Fatalf("LatestByProject: %v", err)
	}
	if got.SegmentName != later.SegmentName {
		t.Errorf("latest segment = %q; want %q", got.SegmentName, later.SegmentName)
	}
}
