-- Migration 148: Tenant Maintenance Mode and Circuit Breaker
-- Stage 6: Tenant Protection Controls

-- Table for tenant maintenance mode (admin-controlled)
CREATE TABLE IF NOT EXISTS _ayb_tenant_maintenance (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES _ayb_tenants(id) ON DELETE CASCADE,
    enabled BOOLEAN NOT NULL DEFAULT false,
    reason TEXT,
    enabled_at TIMESTAMPTZ,
    enabled_by UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT _ayb_tenant_maintenance_tenant_id_key UNIQUE (tenant_id)
);

CREATE INDEX IF NOT EXISTS idx_ayb_tenant_maintenance_tenant_id ON _ayb_tenant_maintenance(tenant_id);

-- Table for tenant circuit breaker state (auto-managed)
CREATE TABLE IF NOT EXISTS _ayb_tenant_circuit_breaker (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES _ayb_tenants(id) ON DELETE CASCADE,
    state TEXT NOT NULL DEFAULT 'closed',
    consecutive_failures INTEGER NOT NULL DEFAULT 0,
    opened_at TIMESTAMPTZ,
    half_open_probes INTEGER NOT NULL DEFAULT 0,
    last_failure_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT _ayb_tenant_circuit_breaker_tenant_id_key UNIQUE (tenant_id),
    CONSTRAINT _ayb_tenant_circuit_breaker_state_check CHECK (state IN ('closed', 'open', 'half_open'))
);

CREATE INDEX IF NOT EXISTS idx_ayb_tenant_circuit_breaker_tenant_id ON _ayb_tenant_circuit_breaker(tenant_id);

-- Add comment for documentation
COMMENT ON TABLE _ayb_tenant_maintenance IS 'Stores admin-controlled maintenance mode state per tenant';
COMMENT ON TABLE _ayb_tenant_circuit_breaker IS 'Stores auto-managed circuit breaker state per tenant';
