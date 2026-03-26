-- Migration 090: Create backup metadata table.

CREATE TABLE IF NOT EXISTS _ayb_backups (
    id                TEXT         NOT NULL DEFAULT gen_random_uuid()::text PRIMARY KEY,
    db_name           TEXT         NOT NULL,
    object_key        TEXT         NOT NULL DEFAULT '',
    size_bytes        BIGINT       NOT NULL DEFAULT 0,
    checksum          TEXT         NOT NULL DEFAULT '',
    started_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    completed_at      TIMESTAMPTZ,
    status            TEXT         NOT NULL DEFAULT 'pending',
    error_message     TEXT         NOT NULL DEFAULT '',
    triggered_by      TEXT         NOT NULL DEFAULT '',
    restore_source_id TEXT         NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS _ayb_backups_status_idx      ON _ayb_backups (status);
CREATE INDEX IF NOT EXISTS _ayb_backups_started_at_idx  ON _ayb_backups (started_at DESC);
CREATE INDEX IF NOT EXISTS _ayb_backups_db_name_idx     ON _ayb_backups (db_name);
