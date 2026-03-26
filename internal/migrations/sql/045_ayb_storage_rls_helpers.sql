-- Storage RLS helper functions for policy templates.
-- Policies use auth.uid() to match Supabase-compatible policy syntax.

CREATE SCHEMA IF NOT EXISTS auth;

CREATE OR REPLACE FUNCTION auth.uid()
RETURNS uuid
LANGUAGE sql
STABLE
AS $$
    SELECT NULLIF(current_setting('ayb.user_id', true), '')::uuid
$$;
