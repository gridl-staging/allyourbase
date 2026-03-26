-- Stage 3: Add tenant_id to _ayb_apps for legacy migration.
-- Apps start with NULL tenant_id and are backfilled by MigrateLegacyApps.

ALTER TABLE _ayb_apps
    ADD COLUMN IF NOT EXISTS tenant_id UUID REFERENCES _ayb_tenants(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_ayb_apps_tenant_id ON _ayb_apps (tenant_id);
