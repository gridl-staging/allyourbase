-- Migration 036: Extend MFA tables for TOTP and email MFA
-- Makes phone nullable (was NOT NULL, blocks non-SMS methods).
-- Adds TOTP-specific and email-MFA-specific columns.

ALTER TABLE _ayb_user_mfa
    ALTER COLUMN phone DROP NOT NULL,
    ADD COLUMN IF NOT EXISTS totp_secret_enc BYTEA,
    ADD COLUMN IF NOT EXISTS totp_enrolled_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS last_used_step BIGINT DEFAULT 0;

-- MFA challenges table: challenge-based verification pattern.
-- Each MFA verification requires a prior challenge record (single-use).
CREATE TABLE IF NOT EXISTS _ayb_mfa_challenges (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    factor_id   UUID        NOT NULL REFERENCES _ayb_user_mfa(id) ON DELETE CASCADE,
    ip_address  INET,
    otp_code_hash TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    verified_at TIMESTAMPTZ,
    expires_at  TIMESTAMPTZ NOT NULL DEFAULT (NOW() + INTERVAL '5 minutes')
);

CREATE INDEX IF NOT EXISTS idx_mfa_challenges_factor_created
    ON _ayb_mfa_challenges (factor_id, created_at);

-- MFA backup codes table: separate table for atomic single-use consumption.
CREATE TABLE IF NOT EXISTS _ayb_mfa_backup_codes (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID        NOT NULL REFERENCES _ayb_users(id) ON DELETE CASCADE,
    code_hash  TEXT        NOT NULL,
    used_at    TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_mfa_backup_codes_user_id
    ON _ayb_mfa_backup_codes (user_id);
