-- Migration 038: Add attempt tracking to MFA challenges
-- Enables per-code attempt limits: after N failed attempts, the challenge
-- is invalidated and the user must request a new code.

ALTER TABLE _ayb_mfa_challenges
    ADD COLUMN IF NOT EXISTS attempt_count INT NOT NULL DEFAULT 0;
