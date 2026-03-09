-- OAuth 2.0 provider: opaque access and refresh tokens.
-- One row per token (not paired). Access + refresh tokens share a grant_id.
-- Reuse detection: if a revoked refresh token is presented, revoke all tokens with same grant_id.
CREATE TABLE IF NOT EXISTS _ayb_oauth_tokens (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    token_hash      VARCHAR(64) UNIQUE NOT NULL,
    token_type      VARCHAR(10) NOT NULL CHECK (token_type IN ('access', 'refresh')),
    client_id       VARCHAR(60) NOT NULL REFERENCES _ayb_oauth_clients(client_id),
    user_id         UUID REFERENCES _ayb_users(id) ON DELETE CASCADE, -- NULL for client_credentials grants
    scope           VARCHAR(20) NOT NULL CHECK (scope IN ('readonly', 'readwrite', '*')),
    allowed_tables  TEXT[],
    grant_id        UUID NOT NULL,
    expires_at      TIMESTAMPTZ NOT NULL,
    revoked_at      TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ayb_oauth_tokens_grant_id ON _ayb_oauth_tokens (grant_id);
CREATE INDEX IF NOT EXISTS idx_ayb_oauth_tokens_client_id ON _ayb_oauth_tokens (client_id);
