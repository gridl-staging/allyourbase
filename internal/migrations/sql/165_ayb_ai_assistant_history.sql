-- Stage 165: Dashboard AI assistant query history.

CREATE TABLE IF NOT EXISTS _ayb_ai_assistant_history (
    id            UUID        NOT NULL DEFAULT gen_random_uuid() PRIMARY KEY,
    mode          TEXT        NOT NULL CHECK (mode IN ('sql', 'rls', 'migration', 'general')),
    query_text    TEXT        NOT NULL,
    response_text TEXT        NOT NULL DEFAULT '',
    sql_text      TEXT        NOT NULL DEFAULT '',
    explanation   TEXT        NOT NULL DEFAULT '',
    warning       TEXT        NOT NULL DEFAULT '',
    provider      TEXT        NOT NULL,
    model         TEXT        NOT NULL,
    status        TEXT        NOT NULL CHECK (status IN ('success', 'error', 'cancelled')),
    duration_ms   INT         NOT NULL DEFAULT 0,
    input_tokens  INT         NOT NULL DEFAULT 0,
    output_tokens INT         NOT NULL DEFAULT 0,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ayb_ai_assistant_history_created_at
    ON _ayb_ai_assistant_history (created_at DESC);

CREATE INDEX IF NOT EXISTS idx_ayb_ai_assistant_history_mode_created_at
    ON _ayb_ai_assistant_history (mode, created_at DESC);
