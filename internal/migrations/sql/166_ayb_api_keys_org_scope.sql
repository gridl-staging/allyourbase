-- Stage 166: Add org_id scope to API keys.
-- A key can be legacy user-scoped (org_id NULL, app_id NULL),
-- app-scoped (app_id NOT NULL, org_id NULL), or
-- org-scoped (org_id NOT NULL, app_id NULL).
-- The CHECK constraint enforces mutual exclusivity of app and org scope.

ALTER TABLE _ayb_api_keys
    ADD COLUMN IF NOT EXISTS org_id UUID REFERENCES _ayb_organizations(id);

CREATE INDEX IF NOT EXISTS idx_ayb_api_keys_org_id ON _ayb_api_keys (org_id);

-- Enforce that a key cannot be both app-scoped and org-scoped.
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conrelid = '_ayb_api_keys'::regclass
          AND conname  = '_ayb_api_keys_scope_exclusivity'
    ) THEN
        ALTER TABLE _ayb_api_keys
            ADD CONSTRAINT _ayb_api_keys_scope_exclusivity
            CHECK (NOT (app_id IS NOT NULL AND org_id IS NOT NULL));
    END IF;
END $$;
