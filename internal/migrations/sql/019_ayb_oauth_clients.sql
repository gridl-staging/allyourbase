-- OAuth 2.0 provider: client registration.
-- Each OAuth client belongs to an app (Stage 1 _ayb_apps) and inherits its rate limits.
-- Supports confidential (server-side) and public (SPA/native) client types.
CREATE TABLE IF NOT EXISTS _ayb_oauth_clients (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    app_id            UUID NOT NULL REFERENCES _ayb_apps(id) ON DELETE RESTRICT,
    client_id         VARCHAR(56) UNIQUE NOT NULL CHECK (client_id ~ '^ayb_cid_[0-9a-f]{48}$'), -- ayb_cid_ + 24 hex bytes
    client_secret_hash VARCHAR(64),                   -- SHA-256 hex; NULL for public clients
    name              TEXT NOT NULL,
    redirect_uris     TEXT[] NOT NULL,
    scopes            TEXT[] NOT NULL,                -- subset of: readonly, readwrite, *
    client_type       VARCHAR(20) NOT NULL CHECK (client_type IN ('confidential', 'public')),
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    revoked_at        TIMESTAMPTZ,
    CHECK ((client_type = 'confidential' AND client_secret_hash IS NOT NULL) OR (client_type = 'public' AND client_secret_hash IS NULL)),
    CHECK (scopes <@ ARRAY['readonly', 'readwrite', '*']::TEXT[]),
    CHECK (cardinality(scopes) >= 1),
    CHECK (cardinality(redirect_uris) >= 1)
);

CREATE INDEX IF NOT EXISTS idx_ayb_oauth_clients_app_id ON _ayb_oauth_clients (app_id);
