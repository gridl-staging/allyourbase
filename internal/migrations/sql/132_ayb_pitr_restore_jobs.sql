-- Migration 132: Create PITR restore job tracking table.

CREATE TABLE IF NOT EXISTS _ayb_restore_jobs (
    id                   TEXT        NOT NULL DEFAULT gen_random_uuid()::text PRIMARY KEY,
    project_id           TEXT        NOT NULL,
    database_id          TEXT        NOT NULL,
    environment          TEXT        NOT NULL DEFAULT '',
    target_time          TIMESTAMPTZ NOT NULL,
    phase                TEXT        NOT NULL DEFAULT 'pending',
    status               TEXT        NOT NULL DEFAULT 'pending',
    base_backup_id       TEXT        NOT NULL DEFAULT '',
    wal_segments_needed  INTEGER     NOT NULL DEFAULT 0,
    verification_result  JSONB,
    logs                 TEXT        NOT NULL DEFAULT '',
    error_message        TEXT        NOT NULL DEFAULT '',
    requested_by         TEXT        NOT NULL DEFAULT '',
    started_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at         TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS _ayb_restore_jobs_project_started_idx
    ON _ayb_restore_jobs (project_id, database_id, started_at DESC);
