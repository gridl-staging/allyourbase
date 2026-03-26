-- AI call log: tracks every LLM invocation for observability and usage billing.
CREATE TABLE IF NOT EXISTS _ayb_ai_call_log (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    provider        TEXT NOT NULL,
    model           TEXT NOT NULL,
    input_tokens    INT,
    output_tokens   INT,
    duration_ms     INT,
    status          TEXT NOT NULL,  -- 'success' or 'error'
    error_message   TEXT,
    edge_function_id UUID,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ayb_ai_call_log_created_at ON _ayb_ai_call_log (created_at DESC);
