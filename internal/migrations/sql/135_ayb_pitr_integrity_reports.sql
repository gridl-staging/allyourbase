-- Migration 135: Create PITR integrity report storage table.

CREATE TABLE IF NOT EXISTS _ayb_pitr_integrity_reports (
    id          TEXT        NOT NULL DEFAULT gen_random_uuid()::text PRIMARY KEY,
    project_id  TEXT        NOT NULL,
    database_id TEXT        NOT NULL,
    status      TEXT        NOT NULL,
    checks      JSONB       NOT NULL,
    verified_at TIMESTAMPTZ NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS _ayb_pitr_integrity_reports_project_verified_idx
    ON _ayb_pitr_integrity_reports (project_id, database_id, verified_at DESC);
