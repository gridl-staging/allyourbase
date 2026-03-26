-- Stage 169: Enforce one custom domain attachment per hosted site.
--
-- This must be a follow-up migration rather than a back-edit to 168 so
-- databases that already applied 168 still receive the new uniqueness guard.

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
          FROM pg_constraint
         WHERE conrelid = '_ayb_sites'::regclass
           AND conname = '_ayb_sites_custom_domain_unique'
    ) THEN
        ALTER TABLE _ayb_sites
            ADD CONSTRAINT _ayb_sites_custom_domain_unique UNIQUE (custom_domain_id);
    END IF;
END $$;
