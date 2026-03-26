-- Per-user storage quota tracking and overrides.
-- _ayb_storage_usage tracks cumulative bytes used per user.
-- _ayb_users.storage_quota_mb allows per-user quota overrides (NULL = use system default).

CREATE TABLE IF NOT EXISTS _ayb_storage_usage (
    user_id    UUID PRIMARY KEY REFERENCES _ayb_users(id) ON DELETE CASCADE,
    bytes_used BIGINT NOT NULL DEFAULT 0 CHECK (bytes_used >= 0),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE _ayb_users ADD COLUMN IF NOT EXISTS storage_quota_mb INT;
