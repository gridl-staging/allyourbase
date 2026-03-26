-- Stage G-1: Tenant memberships — user-to-tenant associations with roles.

CREATE TABLE IF NOT EXISTS _ayb_tenant_memberships (
    id         UUID        NOT NULL DEFAULT gen_random_uuid() PRIMARY KEY,
    tenant_id  UUID        NOT NULL REFERENCES _ayb_tenants(id) ON DELETE CASCADE,
    user_id    UUID        NOT NULL REFERENCES _ayb_users(id) ON DELETE CASCADE,
    role       TEXT        NOT NULL DEFAULT 'member'
                           CHECK (role IN ('owner', 'admin', 'member')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_ayb_tenant_memberships_user_id ON _ayb_tenant_memberships (user_id);
