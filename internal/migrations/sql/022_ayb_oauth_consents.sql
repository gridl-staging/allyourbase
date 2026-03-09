-- OAuth 2.0 provider: remembered user consents.
-- When a user approves an OAuth client for a given scope, we store it here.
-- On re-authorization, if requested scope is a subset of the stored consent, skip the prompt.
CREATE TABLE IF NOT EXISTS _ayb_oauth_consents (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL REFERENCES _ayb_users(id) ON DELETE CASCADE,
    client_id       VARCHAR(60) NOT NULL REFERENCES _ayb_oauth_clients(client_id),
    scope           VARCHAR(20) NOT NULL CHECK (scope IN ('readonly', 'readwrite', '*')),
    allowed_tables  TEXT[],
    granted_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (user_id, client_id)
);
