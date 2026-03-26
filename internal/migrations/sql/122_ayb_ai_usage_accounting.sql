-- Stage 6: AI usage accounting and daily rollups.

ALTER TABLE _ayb_ai_call_log
  ADD COLUMN IF NOT EXISTS cost_usd NUMERIC(20,6) NOT NULL DEFAULT 0;

CREATE INDEX IF NOT EXISTS idx_ayb_ai_call_log_provider_model_created_at
  ON _ayb_ai_call_log (provider, model, created_at DESC);

CREATE TABLE IF NOT EXISTS _ayb_ai_usage_daily (
  day             DATE NOT NULL,
  provider        TEXT NOT NULL,
  model           TEXT NOT NULL,
  calls           BIGINT NOT NULL DEFAULT 0,
  input_tokens    BIGINT NOT NULL DEFAULT 0,
  output_tokens   BIGINT NOT NULL DEFAULT 0,
  total_tokens    BIGINT NOT NULL DEFAULT 0,
  total_cost_usd  NUMERIC(20,6) NOT NULL DEFAULT 0,
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (day, provider, model)
);

CREATE INDEX IF NOT EXISTS idx_ayb_ai_usage_daily_provider_model_day
  ON _ayb_ai_usage_daily (provider, model, day DESC);
