-- AYB auth users table.
-- Stores registered user accounts for built-in authentication.
CREATE TABLE IF NOT EXISTS _ayb_users (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email         TEXT NOT NULL,
    password_hash TEXT NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Case-insensitive unique index on email.
CREATE UNIQUE INDEX IF NOT EXISTS idx_ayb_users_email
    ON _ayb_users (LOWER(email));

-- Role used for authenticated API requests. SET LOCAL ROLE switches to this
-- role within each request transaction so RLS policies are enforced even when
-- the connection pool uses a superuser.
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'ayb_authenticated') THEN
        CREATE ROLE ayb_authenticated NOLOGIN;
    END IF;
END
$$;

-- Grant the authenticated role access to the public schema and all current
-- and future tables/sequences so user-created tables are accessible.
GRANT USAGE ON SCHEMA public TO ayb_authenticated;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO ayb_authenticated;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO ayb_authenticated;
ALTER DEFAULT PRIVILEGES IN SCHEMA public
    GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO ayb_authenticated;
ALTER DEFAULT PRIVILEGES IN SCHEMA public
    GRANT USAGE, SELECT ON SEQUENCES TO ayb_authenticated;
