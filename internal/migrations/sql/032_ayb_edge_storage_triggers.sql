-- Storage trigger table: maps edge functions to storage bucket events.
CREATE TABLE IF NOT EXISTS _ayb_edge_storage_triggers (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    function_id     UUID NOT NULL REFERENCES _ayb_edge_functions(id) ON DELETE CASCADE,
    bucket          TEXT NOT NULL,
    event_types     TEXT[] NOT NULL,
    prefix_filter   TEXT,
    suffix_filter   TEXT,
    enabled         BOOLEAN NOT NULL DEFAULT true,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ayb_edge_storage_triggers_function
    ON _ayb_edge_storage_triggers (function_id);

CREATE INDEX IF NOT EXISTS idx_ayb_edge_storage_triggers_bucket
    ON _ayb_edge_storage_triggers (bucket) WHERE enabled = true;
