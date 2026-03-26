-- Execution logs for edge function invocations.
CREATE TABLE IF NOT EXISTS _ayb_edge_function_logs (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    function_id      UUID NOT NULL REFERENCES _ayb_edge_functions(id) ON DELETE CASCADE,
    invocation_id    UUID NOT NULL,
    status           TEXT NOT NULL,
    duration_ms      INT NOT NULL
                         CHECK (duration_ms >= 0),
    stdout           TEXT,
    error            TEXT,
    request_method   TEXT,
    request_path     TEXT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ayb_edge_function_logs_function_created
    ON _ayb_edge_function_logs (function_id, created_at DESC);
