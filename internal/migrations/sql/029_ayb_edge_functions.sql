-- Edge function definitions and runtime configuration.
CREATE TABLE IF NOT EXISTS _ayb_edge_functions (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name            TEXT NOT NULL UNIQUE,
    entry_point     TEXT NOT NULL DEFAULT 'handler',
    source          TEXT NOT NULL,
    compiled_js     TEXT NOT NULL,
    timeout_ms      INT NOT NULL DEFAULT 5000
                        CHECK (timeout_ms >= 0),
    env_vars        JSONB NOT NULL DEFAULT '{}'::jsonb
                        CHECK (jsonb_typeof(env_vars) = 'object'),
    public          BOOLEAN NOT NULL DEFAULT false,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
