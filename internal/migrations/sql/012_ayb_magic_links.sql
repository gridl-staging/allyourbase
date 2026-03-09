CREATE TABLE IF NOT EXISTS _ayb_magic_links (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email TEXT NOT NULL,
    token_hash TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_magic_links_token ON _ayb_magic_links(token_hash);
CREATE INDEX IF NOT EXISTS idx_magic_links_email ON _ayb_magic_links(email);
