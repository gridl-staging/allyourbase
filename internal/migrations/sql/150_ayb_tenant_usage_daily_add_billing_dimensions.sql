-- Stage 2: Add columns for billed usage dimensions.

ALTER TABLE _ayb_tenant_usage_daily
  ADD COLUMN IF NOT EXISTS bandwidth_bytes BIGINT NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS function_invocations BIGINT NOT NULL DEFAULT 0;

CREATE INDEX IF NOT EXISTS idx_ayb_tenant_usage_daily_bandwidth_bytes
    ON _ayb_tenant_usage_daily (date, bandwidth_bytes);

CREATE INDEX IF NOT EXISTS idx_ayb_tenant_usage_daily_function_invocations
    ON _ayb_tenant_usage_daily (date, function_invocations);
