-- Explicit bucket metadata for storage ACL.
-- Previously buckets were implicit (just a column on _ayb_storage_objects).
-- This table enables per-bucket public/private access control.
CREATE TABLE IF NOT EXISTS _ayb_storage_buckets (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name       TEXT NOT NULL UNIQUE,
    public     BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ayb_storage_buckets_name ON _ayb_storage_buckets (name);

-- Add foreign key from storage objects to buckets.
-- Existing rows with bucket values not in _ayb_storage_buckets are left as-is;
-- the FK is added only for new inserts after buckets are explicitly created.
-- We do NOT add the FK constraint here because existing objects may reference
-- implicit buckets that haven't been migrated. Bucket enforcement is done at
-- the application layer.
