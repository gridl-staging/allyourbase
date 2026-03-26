BEGIN;

CREATE TABLE IF NOT EXISTS _ayb_fdw_servers (
    name TEXT PRIMARY KEY,
    fdw_type TEXT NOT NULL,
    options JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMIT;
