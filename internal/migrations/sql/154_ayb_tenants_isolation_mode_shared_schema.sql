BEGIN;

ALTER TABLE _ayb_tenants
  DROP CONSTRAINT _ayb_tenants_isolation_mode_check;

UPDATE _ayb_tenants
SET isolation_mode = 'shared'
WHERE isolation_mode = 'database';

ALTER TABLE _ayb_tenants
  ADD CONSTRAINT _ayb_tenants_isolation_mode_check
  CHECK (isolation_mode IN ('shared', 'schema'));

ALTER TABLE _ayb_tenants
  ALTER COLUMN isolation_mode SET DEFAULT 'shared';

COMMIT;
