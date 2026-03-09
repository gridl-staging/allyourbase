-- Non-expiring API keys for service-to-service authentication.
-- Keys are tied to a user so RLS policies apply via the user's identity.
CREATE TABLE IF NOT EXISTS _ayb_api_keys (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES _ayb_users(id) ON DELETE CASCADE,
    name        TEXT NOT NULL DEFAULT '',
    key_hash    TEXT NOT NULL,
    key_prefix  TEXT NOT NULL,
    last_used_at TIMESTAMPTZ,
    expires_at  TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    revoked_at  TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_ayb_api_keys_key_hash
    ON _ayb_api_keys (key_hash);

CREATE INDEX IF NOT EXISTS idx_ayb_api_keys_user_id
    ON _ayb_api_keys (user_id);
