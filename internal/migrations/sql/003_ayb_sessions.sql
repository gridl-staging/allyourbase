-- Refresh token sessions table.
-- Stores SHA-256 hashed refresh tokens for token rotation.
CREATE TABLE IF NOT EXISTS _ayb_sessions (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID NOT NULL REFERENCES _ayb_users(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ayb_sessions_token_hash ON _ayb_sessions (token_hash);
CREATE INDEX IF NOT EXISTS idx_ayb_sessions_user_id ON _ayb_sessions (user_id);
