-- Stage 167: Add org_id to tenant audit events for org-scoped queries.
-- tenant_id stays NOT NULL — org-level-only events are NOT a requirement.
-- This enriches existing tenant events so they can be queried by org.

ALTER TABLE _ayb_tenant_audit_events
    ADD COLUMN IF NOT EXISTS org_id UUID REFERENCES _ayb_organizations(id);

CREATE INDEX IF NOT EXISTS idx_ayb_tenant_audit_events_org
    ON _ayb_tenant_audit_events (org_id, created_at);
