-- Stage 164: Organizations and team foundation.

CREATE TABLE IF NOT EXISTS _ayb_organizations (
    id             UUID        NOT NULL DEFAULT gen_random_uuid() PRIMARY KEY,
    name           TEXT        NOT NULL,
    slug           TEXT        NOT NULL UNIQUE,
    parent_org_id  UUID        REFERENCES _ayb_organizations(id) ON DELETE CASCADE,
    plan_tier      TEXT        NOT NULL DEFAULT 'free',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ayb_organizations_parent_org_id ON _ayb_organizations (parent_org_id);

CREATE TABLE IF NOT EXISTS _ayb_teams (
    id         UUID    NOT NULL DEFAULT gen_random_uuid() PRIMARY KEY,
    org_id     UUID    NOT NULL REFERENCES _ayb_organizations(id) ON DELETE CASCADE,
    name       TEXT    NOT NULL,
    slug       TEXT    NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(org_id, slug)
);

CREATE TABLE IF NOT EXISTS _ayb_org_memberships (
    id        UUID    NOT NULL DEFAULT gen_random_uuid() PRIMARY KEY,
    org_id    UUID    NOT NULL REFERENCES _ayb_organizations(id) ON DELETE CASCADE,
    user_id   UUID    NOT NULL REFERENCES _ayb_users(id) ON DELETE CASCADE,
    role      TEXT    NOT NULL DEFAULT 'member'
                        CHECK (role IN ('owner', 'admin', 'member', 'viewer')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (org_id, user_id)
);

CREATE TABLE IF NOT EXISTS _ayb_team_memberships (
    id        UUID    NOT NULL DEFAULT gen_random_uuid() PRIMARY KEY,
    team_id   UUID    NOT NULL REFERENCES _ayb_teams(id) ON DELETE CASCADE,
    user_id   UUID    NOT NULL REFERENCES _ayb_users(id) ON DELETE CASCADE,
    role      TEXT    NOT NULL DEFAULT 'member'
                        CHECK (role IN ('lead', 'member')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (team_id, user_id)
);

ALTER TABLE _ayb_tenants
    ADD COLUMN IF NOT EXISTS org_id UUID REFERENCES _ayb_organizations(id);

DO $$
DECLARE
    existing_constraint_name text;
BEGIN
    SELECT conname
    INTO existing_constraint_name
    FROM pg_constraint
    WHERE conrelid = '_ayb_tenant_memberships'::regclass
      AND contype = 'c'
      AND conname = '_ayb_tenant_memberships_role_check';

    IF existing_constraint_name IS NOT NULL THEN
        EXECUTE format('ALTER TABLE _ayb_tenant_memberships DROP CONSTRAINT %I', existing_constraint_name);
    END IF;

    EXECUTE 'ALTER TABLE _ayb_tenant_memberships
        ADD CONSTRAINT _ayb_tenant_memberships_role_check
        CHECK (role IN (''owner'', ''admin'', ''member'', ''viewer''))';
END $$;
