-- Stage G-1: Core tenant table with lifecycle state machine.

CREATE TABLE IF NOT EXISTS _ayb_tenants (
    id             UUID        NOT NULL DEFAULT gen_random_uuid() PRIMARY KEY,
    name           TEXT        NOT NULL,
    slug           TEXT        NOT NULL UNIQUE,
    isolation_mode TEXT        NOT NULL DEFAULT 'schema'
                               CHECK (isolation_mode IN ('schema', 'database')),
    plan_tier      TEXT        NOT NULL DEFAULT 'free',
    region         TEXT        NOT NULL DEFAULT 'default',
    org_metadata   JSONB       NOT NULL DEFAULT '{}',
    state          TEXT        NOT NULL DEFAULT 'provisioning'
                               CHECK (state IN ('provisioning', 'active', 'suspended', 'deleting', 'deleted')),
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ayb_tenants_state ON _ayb_tenants (state);
