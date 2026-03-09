-- Materialized view refresh registry.
CREATE TABLE IF NOT EXISTS _ayb_matview_refreshes (
    id                        UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    schema_name               VARCHAR(63) NOT NULL DEFAULT 'public'
                                  CHECK (schema_name ~ '^[A-Za-z_][A-Za-z0-9_]*$'),
    view_name                 VARCHAR(63) NOT NULL
                                  CHECK (view_name ~ '^[A-Za-z_][A-Za-z0-9_]*$'),
    refresh_mode              VARCHAR(20) NOT NULL DEFAULT 'standard'
                                  CHECK (refresh_mode IN ('standard', 'concurrent')),
    last_refresh_at           TIMESTAMPTZ,
    last_refresh_duration_ms  INT,
    last_refresh_status       VARCHAR(20)
                                  CHECK (last_refresh_status IN ('success', 'error')),
    last_refresh_error        TEXT,
    created_at                TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (schema_name, view_name)
);
