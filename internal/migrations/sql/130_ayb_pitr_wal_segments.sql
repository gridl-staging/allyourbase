-- Migration 130: Create WAL segment archive metadata table for PITR.

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
);

CREATE INDEX IF NOT EXISTS _ayb_wal_segments_project_archived_idx
    ON _ayb_wal_segments (project_id, database_id, archived_at DESC);
