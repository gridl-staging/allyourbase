-- Add stdout size and response status metadata to edge function logs.
ALTER TABLE _ayb_edge_function_logs
    ADD COLUMN IF NOT EXISTS stdout_bytes         INT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS response_status_code INT NOT NULL DEFAULT 0;
