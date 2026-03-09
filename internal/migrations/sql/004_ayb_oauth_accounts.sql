-- OAuth identity linking table.
-- Maps external OAuth provider identities to AYB users.
CREATE TABLE IF NOT EXISTS _ayb_oauth_accounts (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id          UUID NOT NULL REFERENCES _ayb_users(id) ON DELETE CASCADE,
    provider         TEXT NOT NULL,
    provider_user_id TEXT NOT NULL,
    email            TEXT,
    name             TEXT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(provider, provider_user_id)
);

CREATE INDEX IF NOT EXISTS idx_ayb_oauth_accounts_user_id ON _ayb_oauth_accounts (user_id);
