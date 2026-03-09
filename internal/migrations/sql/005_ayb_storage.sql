-- File storage metadata table.
-- Tracks uploaded files with bucket/name uniqueness.
CREATE TABLE IF NOT EXISTS _ayb_storage_objects (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    bucket       TEXT NOT NULL,
    name         TEXT NOT NULL,
    size         BIGINT NOT NULL,
    content_type TEXT NOT NULL DEFAULT 'application/octet-stream',
    user_id      UUID REFERENCES _ayb_users(id) ON DELETE SET NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(bucket, name)
);

CREATE INDEX IF NOT EXISTS idx_ayb_storage_objects_bucket ON _ayb_storage_objects (bucket);
CREATE INDEX IF NOT EXISTS idx_ayb_storage_objects_user_id ON _ayb_storage_objects (user_id);
