-- OAuth provider tokens (vault-encrypted) for server-side API access and refresh.
CREATE TABLE IF NOT EXISTS _ayb_oauth_provider_tokens (
    id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id               UUID NOT NULL REFERENCES _ayb_users(id) ON DELETE CASCADE,
    provider              TEXT NOT NULL,
    access_token_enc      BYTEA,
    refresh_token_enc     BYTEA,
    token_type            TEXT,
    scopes                TEXT,
    expires_at            TIMESTAMPTZ,
    refresh_failure_count INT NOT NULL DEFAULT 0,
    last_refresh_error    TEXT,
    last_refreshed_at     TIMESTAMPTZ,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_ayb_oauth_provider_tokens_user_provider
    ON _ayb_oauth_provider_tokens (user_id, provider);
