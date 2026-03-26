-- Migration 037: Add AAL and AMR to session table
-- Enables refresh tokens to preserve authentication assurance level.
-- Sessions created by MFA verification store aal='aal2' and amr (e.g. 'password,totp').
-- RefreshToken reads these back to issue tokens at the correct assurance level.

ALTER TABLE _ayb_sessions
    ADD COLUMN IF NOT EXISTS aal TEXT NOT NULL DEFAULT 'aal1',
    ADD COLUMN IF NOT EXISTS amr TEXT NOT NULL DEFAULT '';
