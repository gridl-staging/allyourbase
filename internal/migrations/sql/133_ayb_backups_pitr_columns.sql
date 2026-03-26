-- Migration 133: Add PITR-related columns to the existing backups table.
-- Existing logical backup records default to backup_type='logical' with NULL LSN fields.

ALTER TABLE _ayb_backups
    ADD COLUMN IF NOT EXISTS backup_type TEXT        NOT NULL DEFAULT 'logical',
    ADD COLUMN IF NOT EXISTS start_lsn   pg_lsn,
    ADD COLUMN IF NOT EXISTS end_lsn     pg_lsn,
    ADD COLUMN IF NOT EXISTS project_id  TEXT        NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS database_id TEXT        NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS _ayb_backups_pitr_idx
    ON _ayb_backups (backup_type, project_id, database_id);
