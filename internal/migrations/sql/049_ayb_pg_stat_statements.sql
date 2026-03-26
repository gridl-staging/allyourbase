-- Enable pg_stat_statements when supported by the running Postgres.
-- This is best-effort so environments without extension support keep working.
DO $$
BEGIN
    CREATE EXTENSION IF NOT EXISTS pg_stat_statements;
EXCEPTION
    WHEN insufficient_privilege THEN
        RAISE NOTICE 'insufficient privilege to enable pg_stat_statements; continuing';
    WHEN undefined_file THEN
        RAISE NOTICE 'pg_stat_statements extension files not available; continuing';
END
$$;
