CREATE TABLE IF NOT EXISTS _ayb_password_resets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES _ayb_users(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_password_resets_token ON _ayb_password_resets(token_hash);
CREATE INDEX IF NOT EXISTS idx_password_resets_user ON _ayb_password_resets(user_id);
