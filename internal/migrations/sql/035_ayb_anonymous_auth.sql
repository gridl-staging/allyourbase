-- Migration 035: Anonymous auth + account linking
-- Adds is_anonymous flag and linked_at timestamp to users table.
-- Makes email and password_hash nullable for anonymous users.

ALTER TABLE _ayb_users
    ALTER COLUMN email DROP NOT NULL,
    ALTER COLUMN password_hash DROP NOT NULL,
    ADD COLUMN IF NOT EXISTS is_anonymous BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS linked_at TIMESTAMPTZ;

-- Allow the unique index to handle NULL emails (anonymous users have no email).
-- Drop the old index and recreate as a partial index on non-null emails only.
DROP INDEX IF EXISTS idx_ayb_users_email;
CREATE UNIQUE INDEX idx_ayb_users_email ON _ayb_users (LOWER(email)) WHERE email IS NOT NULL;
