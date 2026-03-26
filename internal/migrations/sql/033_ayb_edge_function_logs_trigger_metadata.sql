-- Add trigger source metadata to edge function execution logs.
ALTER TABLE _ayb_edge_function_logs
    ADD COLUMN IF NOT EXISTS trigger_type          TEXT,
    ADD COLUMN IF NOT EXISTS trigger_id            TEXT,
    ADD COLUMN IF NOT EXISTS parent_invocation_id  TEXT;
