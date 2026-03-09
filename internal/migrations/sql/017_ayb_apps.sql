-- Application identity model for per-app API key scoping.
-- Apps are first-class entities with rate limits. API keys can optionally
-- belong to an app (null app_id = legacy user-scoped key).
CREATE TABLE IF NOT EXISTS _ayb_apps (
    id                       UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name                     TEXT NOT NULL,
    description              TEXT NOT NULL DEFAULT '',
    owner_user_id            UUID NOT NULL REFERENCES _ayb_users(id) ON DELETE CASCADE,
    rate_limit_rps           INT NOT NULL DEFAULT 0,          -- 0 = no limit
    rate_limit_window_seconds INT NOT NULL DEFAULT 60,
    created_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at               TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ayb_apps_owner ON _ayb_apps (owner_user_id);

-- Add nullable app_id FK to api_keys. Null = legacy user-scoped key.
ALTER TABLE _ayb_api_keys ADD COLUMN IF NOT EXISTS app_id UUID REFERENCES _ayb_apps(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_ayb_api_keys_app_id ON _ayb_api_keys (app_id) WHERE app_id IS NOT NULL;
