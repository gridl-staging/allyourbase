-- Migration 100: Create branch metadata table.

CREATE TABLE IF NOT EXISTS _ayb_branches (
    id              TEXT         NOT NULL DEFAULT gen_random_uuid()::text PRIMARY KEY,
    name            TEXT         NOT NULL,
    source_database TEXT         NOT NULL,
    branch_database TEXT         NOT NULL,
    status          TEXT         NOT NULL DEFAULT 'creating',
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    error_message   TEXT         NOT NULL DEFAULT ''
);

CREATE UNIQUE INDEX IF NOT EXISTS _ayb_branches_name_idx ON _ayb_branches (name);
CREATE INDEX IF NOT EXISTS _ayb_branches_status_idx ON _ayb_branches (status);
