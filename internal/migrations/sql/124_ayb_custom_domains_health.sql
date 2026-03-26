-- Health monitoring and re-verification support for custom domains.

-- Expand the status enum to include verification_lapsed.
ALTER TABLE _ayb_custom_domains DROP CONSTRAINT IF EXISTS _ayb_custom_domains_status_check;
ALTER TABLE _ayb_custom_domains ADD CONSTRAINT _ayb_custom_domains_status_check
    CHECK (status IN ('pending_verification','verified','active','verification_failed','tombstoned','verification_lapsed'));

-- Health monitoring columns.
ALTER TABLE _ayb_custom_domains ADD COLUMN IF NOT EXISTS health_status TEXT NOT NULL DEFAULT 'unknown'
    CHECK (health_status IN ('healthy', 'unhealthy', 'unknown'));
ALTER TABLE _ayb_custom_domains ADD COLUMN IF NOT EXISTS last_health_check TIMESTAMPTZ;
ALTER TABLE _ayb_custom_domains ADD COLUMN IF NOT EXISTS reverify_failures INT NOT NULL DEFAULT 0;
