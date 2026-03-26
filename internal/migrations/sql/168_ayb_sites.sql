-- Stage 168: Add sites and deploys tables for static site hosting MVP.
--
-- _ayb_sites: site settings (slug, SPA mode, optional custom domain link).
-- _ayb_deploys: deploy records with lifecycle states and a partial unique
--   index enforcing at most one live deploy per site.

CREATE TABLE IF NOT EXISTS _ayb_sites (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name            TEXT NOT NULL,
    slug            TEXT NOT NULL,
    spa_mode        BOOLEAN NOT NULL DEFAULT true,
    custom_domain_id UUID REFERENCES _ayb_custom_domains(id),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT _ayb_sites_slug_unique UNIQUE (slug)
);

CREATE TABLE IF NOT EXISTS _ayb_deploys (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    site_id         UUID NOT NULL REFERENCES _ayb_sites(id) ON DELETE CASCADE,
    status          TEXT NOT NULL DEFAULT 'uploading'
                    CHECK (status IN ('uploading', 'live', 'superseded', 'failed')),
    file_count      INTEGER NOT NULL DEFAULT 0,
    total_bytes     BIGINT NOT NULL DEFAULT 0,
    error_message   TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Enforce at most one live deploy per site at the database level.
CREATE UNIQUE INDEX IF NOT EXISTS _ayb_deploys_one_live_per_site
    ON _ayb_deploys (site_id) WHERE status = 'live';

CREATE INDEX IF NOT EXISTS idx_ayb_deploys_site_status
    ON _ayb_deploys (site_id, status);
