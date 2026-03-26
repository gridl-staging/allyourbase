-- Enable pgvector extension for vector similarity search.
-- Uses IF NOT EXISTS for idempotency. If the extension binary is unavailable
-- (e.g. non-AYB-managed Postgres without pgvector), this statement is a no-op
-- wrapped in a DO block that catches the error gracefully.
DO $$
BEGIN
    CREATE EXTENSION IF NOT EXISTS vector;
EXCEPTION WHEN OTHERS THEN
    RAISE NOTICE 'pgvector extension not available: %. Vector features will be disabled.', SQLERRM;
END;
$$;
