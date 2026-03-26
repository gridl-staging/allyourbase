-- Stage G-1: Immutable audit event log for tenant lifecycle actions.
-- Immutability is enforced by the Go repository layer (insert/query only).

CREATE TABLE IF NOT EXISTS _ayb_tenant_audit_events (
    id         UUID        NOT NULL DEFAULT gen_random_uuid() PRIMARY KEY,
    tenant_id  UUID        NOT NULL REFERENCES _ayb_tenants(id) ON DELETE CASCADE,
    actor_id   UUID,
    action     TEXT        NOT NULL,
    result     TEXT        NOT NULL DEFAULT 'success',
    metadata   JSONB       NOT NULL DEFAULT '{}',
    ip_address INET,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ayb_tenant_audit_events_tenant_created
    ON _ayb_tenant_audit_events (tenant_id, created_at);
