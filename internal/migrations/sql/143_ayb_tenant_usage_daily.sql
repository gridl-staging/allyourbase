-- Stage G-1: Daily usage aggregates per tenant for billing and quota tracking.

CREATE TABLE IF NOT EXISTS _ayb_tenant_usage_daily (
    id                       UUID        NOT NULL DEFAULT gen_random_uuid() PRIMARY KEY,
    tenant_id                UUID        NOT NULL REFERENCES _ayb_tenants(id) ON DELETE CASCADE,
    date                     DATE        NOT NULL,
    request_count            BIGINT      NOT NULL DEFAULT 0,
    db_bytes_used            BIGINT      NOT NULL DEFAULT 0,
    realtime_peak_connections INT         NOT NULL DEFAULT 0,
    job_runs                 INT         NOT NULL DEFAULT 0,
    created_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, date)
);

CREATE INDEX IF NOT EXISTS idx_ayb_tenant_usage_daily_date ON _ayb_tenant_usage_daily (date);
