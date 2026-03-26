-- Migration 041: Store SAML provider configuration for Auth SSO.
CREATE TABLE IF NOT EXISTS _ayb_saml_providers (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name             TEXT NOT NULL UNIQUE,
    entity_id        TEXT NOT NULL,
    idp_metadata     TEXT NOT NULL,
    sp_cert          TEXT,
    sp_key_enc       BYTEA,
    attribute_mapping JSONB,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
