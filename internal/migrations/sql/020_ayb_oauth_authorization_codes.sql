-- OAuth 2.0 provider: authorization codes (short-lived, single-use).
-- Codes are stored as SHA-256 hashes. PKCE is mandatory (S256 only).
CREATE TABLE IF NOT EXISTS _ayb_oauth_authorization_codes (
    id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code_hash             VARCHAR(64) UNIQUE NOT NULL,
    client_id             VARCHAR(60) NOT NULL REFERENCES _ayb_oauth_clients(client_id),
    user_id               UUID NOT NULL REFERENCES _ayb_users(id) ON DELETE CASCADE,
    redirect_uri          TEXT NOT NULL,
    scope                 VARCHAR(20) NOT NULL CHECK (scope IN ('readonly', 'readwrite', '*')),
    allowed_tables        TEXT[],
    code_challenge        VARCHAR(128) NOT NULL,
    code_challenge_method VARCHAR(10) NOT NULL DEFAULT 'S256' CHECK (code_challenge_method = 'S256'),
    state                 VARCHAR(256) NOT NULL,
    expires_at            TIMESTAMPTZ NOT NULL,
    used_at               TIMESTAMPTZ,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ayb_oauth_auth_codes_client_id ON _ayb_oauth_authorization_codes (client_id);
