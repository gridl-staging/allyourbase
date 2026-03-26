-- Migration 131: Create backup manifest metadata table for PITR.

CREATE TABLE IF NOT EXISTS _ayb_backup_manifests (
    id          TEXT        NOT NULL DEFAULT gen_random_uuid()::text PRIMARY KEY,
    project_id  TEXT        NOT NULL,
    database_id TEXT        NOT NULL,
    backup_id   TEXT        NOT NULL REFERENCES _ayb_backups (id),
    object_key  TEXT        NOT NULL,
    start_lsn   pg_lsn      NOT NULL,
    end_lsn     pg_lsn      NOT NULL,
    checksum    TEXT        NOT NULL,
    timeline    INTEGER     NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS _ayb_backup_manifests_project_created_idx
    ON _ayb_backup_manifests (project_id, database_id, created_at DESC);
