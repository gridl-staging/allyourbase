-- Migration 014: SMS MFA enrollment
-- File: internal/migrations/sql/014_ayb_sms_mfa.sql

-- MFA enrollment table: tracks which users have enrolled a second factor.
-- Currently supports 'sms' method only; the unique constraint on (user_id, method)
-- allows future expansion to other MFA types (TOTP, WebAuthn, etc.).
CREATE TABLE IF NOT EXISTS _ayb_user_mfa (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID        NOT NULL REFERENCES _ayb_users(id) ON DELETE CASCADE,
    method      TEXT        NOT NULL DEFAULT 'sms',
    phone       TEXT        NOT NULL,
    enabled     BOOLEAN     NOT NULL DEFAULT false,
    enrolled_at TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, method)
);

CREATE INDEX IF NOT EXISTS _ayb_user_mfa_user_id_idx ON _ayb_user_mfa (user_id);
