-- Resumable upload session metadata for TUS uploads.
-- Records per-upload state and temporary backend path while chunks are being
-- received.
CREATE TABLE IF NOT EXISTS _ayb_storage_uploads (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    bucket       TEXT NOT NULL,
    name         TEXT NOT NULL,
    path         TEXT NOT NULL,
    content_type TEXT NOT NULL DEFAULT 'application/octet-stream',
    user_id      UUID REFERENCES _ayb_users(id) ON DELETE SET NULL,
    total_size   BIGINT NOT NULL CHECK (total_size > 0),
    uploaded_size BIGINT NOT NULL DEFAULT 0 CHECK (uploaded_size >= 0),
    status       TEXT NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'finalizing')),
    expires_at   TIMESTAMPTZ NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (bucket, name, path),
    CHECK (uploaded_size <= total_size)
);

CREATE INDEX IF NOT EXISTS idx_ayb_storage_uploads_expires_at ON _ayb_storage_uploads (expires_at);
CREATE INDEX IF NOT EXISTS idx_ayb_storage_uploads_bucket_name ON _ayb_storage_uploads (bucket, name);
