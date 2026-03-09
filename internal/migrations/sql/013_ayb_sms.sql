-- Migration 013: SMS OTP support
-- File: internal/migrations/sql/013_ayb_sms.sql

-- Add phone number to users table
ALTER TABLE _ayb_users ADD COLUMN IF NOT EXISTS phone TEXT;

-- Phone numbers must be unique when present (NULL allowed)
CREATE UNIQUE INDEX IF NOT EXISTS _ayb_users_phone_unique
    ON _ayb_users (phone)
    WHERE phone IS NOT NULL;

-- OTP codes table
-- code_hash: bcrypt of the OTP (NOT SHA-256 — 6-digit codes are brute-forceable
-- with SHA-256 in microseconds; bcrypt's work factor provides real protection)
CREATE TABLE IF NOT EXISTS _ayb_sms_codes (
    id         BIGSERIAL PRIMARY KEY,
    phone      TEXT        NOT NULL,
    code_hash  TEXT        NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    attempts   INTEGER     NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS _ayb_sms_codes_phone_idx ON _ayb_sms_codes (phone);

-- Opt-outs: populated when AYB receives a STOP webhook (Phase 3+)
CREATE TABLE IF NOT EXISTS _ayb_sms_optouts (
    phone        TEXT        NOT NULL PRIMARY KEY,
    opted_out_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- SMS send counter for circuit breaker (sms_daily_limit)
-- One row per UTC day. Reset by rolling the date.
CREATE TABLE IF NOT EXISTS _ayb_sms_daily_counts (
    date  DATE    NOT NULL PRIMARY KEY,
    count INTEGER NOT NULL DEFAULT 0
);
