-- DB trigger metadata table: maps edge functions to database table events.
CREATE TABLE IF NOT EXISTS _ayb_edge_db_triggers (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    function_id     UUID NOT NULL REFERENCES _ayb_edge_functions(id) ON DELETE CASCADE,
    table_name      TEXT NOT NULL,
    schema_name     TEXT NOT NULL DEFAULT 'public',
    events          TEXT[] NOT NULL,
    filter_columns  TEXT[],
    enabled         BOOLEAN NOT NULL DEFAULT true,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (function_id, table_name, schema_name)
);

CREATE INDEX IF NOT EXISTS idx_ayb_edge_db_triggers_function
    ON _ayb_edge_db_triggers (function_id);

CREATE INDEX IF NOT EXISTS idx_ayb_edge_db_triggers_table
    ON _ayb_edge_db_triggers (schema_name, table_name) WHERE enabled = true;

-- Event queue table: durable queue for DB trigger events awaiting dispatch.
CREATE TABLE IF NOT EXISTS _ayb_edge_trigger_events (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    trigger_id      UUID NOT NULL REFERENCES _ayb_edge_db_triggers(id) ON DELETE CASCADE,
    table_name      TEXT NOT NULL,
    schema_name     TEXT NOT NULL,
    operation       TEXT NOT NULL,
    row_id          TEXT,
    payload         JSONB,
    status          TEXT NOT NULL DEFAULT 'pending',
    attempts        INTEGER NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    processed_at    TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_ayb_edge_trigger_events_pending
    ON _ayb_edge_trigger_events (status, created_at) WHERE status IN ('pending', 'failed');

-- PG trigger function: inserts event into queue and sends NOTIFY for low-latency wakeup.
-- Respects ayb.trigger_depth session variable for recursion guard.
CREATE OR REPLACE FUNCTION _ayb_edge_notify() RETURNS trigger AS $$
DECLARE
    event_id UUID;
    current_depth INTEGER;
    row_pk TEXT;
    row_payload JSONB;
BEGIN
    -- Recursion guard: check session variable for trigger depth
    BEGIN
        current_depth := current_setting('ayb.trigger_depth', true)::INTEGER;
    EXCEPTION WHEN OTHERS THEN
        current_depth := 0;
    END;

    IF current_depth > 1 THEN
        RETURN COALESCE(NEW, OLD);
    END IF;

    -- Extract row primary key (best-effort: use id column if present)
    IF TG_OP = 'DELETE' THEN
        row_payload := to_jsonb(OLD);
        BEGIN
            row_pk := OLD.id::TEXT;
        EXCEPTION WHEN OTHERS THEN
            row_pk := NULL;
        END;
    ELSE
        row_payload := to_jsonb(NEW);
        BEGIN
            row_pk := NEW.id::TEXT;
        EXCEPTION WHEN OTHERS THEN
            row_pk := NULL;
        END;
    END IF;

    -- Insert event into queue
    INSERT INTO _ayb_edge_trigger_events (trigger_id, table_name, schema_name, operation, row_id, payload)
    VALUES (TG_ARGV[0]::UUID, TG_TABLE_NAME, TG_TABLE_SCHEMA, TG_OP, row_pk, row_payload)
    RETURNING id INTO event_id;

    -- Low-latency wakeup notification
    PERFORM pg_notify('ayb_edge_trigger', event_id::TEXT);

    RETURN COALESCE(NEW, OLD);
END;
$$ LANGUAGE plpgsql;
