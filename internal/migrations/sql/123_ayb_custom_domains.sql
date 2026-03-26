-- Custom domain bindings: tracks hostname → AYB instance bindings through their lifecycle.
CREATE TABLE _ayb_custom_domains (
    id                UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    hostname          TEXT        NOT NULL,
    environment       TEXT        NOT NULL DEFAULT 'production',
    status            TEXT        NOT NULL DEFAULT 'pending_verification'
                                  CHECK (status IN ('pending_verification','verified','active','verification_failed','tombstoned')),
    verification_token TEXT        NOT NULL,
    cert_ref          TEXT,
    cert_expiry       TIMESTAMPTZ,
    redirect_mode     TEXT        CHECK (redirect_mode IN ('permanent', 'temporary')),
    last_error        TEXT,
    tombstoned_at     TIMESTAMPTZ,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Partial unique index: a hostname may only be active once (tombstoned rows are excluded
-- so a hostname can be re-registered after the tombstone window expires).
CREATE UNIQUE INDEX idx_ayb_custom_domains_hostname_active
    ON _ayb_custom_domains (hostname)
    WHERE status != 'tombstoned';

-- Index on status to support efficient worker queries that scan for a specific status.
CREATE INDEX idx_ayb_custom_domains_status
    ON _ayb_custom_domains (status);
