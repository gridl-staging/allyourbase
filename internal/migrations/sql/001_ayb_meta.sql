-- AYB system metadata table.
-- Tracks AYB version and instance information.
CREATE TABLE IF NOT EXISTS _ayb_meta (
    key         TEXT PRIMARY KEY,
    value       TEXT NOT NULL,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Record initial setup.
INSERT INTO _ayb_meta (key, value) VALUES ('installed_at', NOW()::TEXT)
ON CONFLICT (key) DO NOTHING;
