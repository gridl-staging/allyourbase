-- Stage G-1: Per-tenant resource quotas with hard and soft limits.
-- NULL values represent unlimited (no constraint enforced).

CREATE TABLE IF NOT EXISTS _ayb_tenant_quotas (
    id                          UUID        NOT NULL DEFAULT gen_random_uuid() PRIMARY KEY,
    tenant_id                   UUID        NOT NULL REFERENCES _ayb_tenants(id) ON DELETE CASCADE UNIQUE,
    db_size_bytes_hard          BIGINT,
    db_size_bytes_soft          BIGINT,
    request_rate_rps_hard       INT,
    request_rate_rps_soft       INT,
    realtime_connections_hard   INT,
    realtime_connections_soft   INT,
    job_concurrency_hard        INT,
    job_concurrency_soft        INT,
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
