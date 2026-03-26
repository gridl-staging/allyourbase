-- Stage G-3: Add idempotency key to tenants for duplicate-safe creation.

ALTER TABLE _ayb_tenants ADD COLUMN IF NOT EXISTS idempotency_key TEXT;

CREATE UNIQUE INDEX IF NOT EXISTS idx_ayb_tenants_idempotency_key
    ON _ayb_tenants (idempotency_key) WHERE idempotency_key IS NOT NULL;
