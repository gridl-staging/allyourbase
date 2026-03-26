-- Stage 3: Webhook event deduplication and usage meter sync checkpoints.

CREATE TABLE IF NOT EXISTS _ayb_billing_webhook_events (
    event_id      TEXT        NOT NULL UNIQUE,
    event_type    TEXT        NOT NULL,
    payload       JSONB       NOT NULL,
    processed_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ayb_billing_webhook_events_processed_at
    ON _ayb_billing_webhook_events (processed_at);

CREATE TABLE IF NOT EXISTS _ayb_billing_usage_sync (
    tenant_id            UUID        NOT NULL,
    usage_date           DATE        NOT NULL,
    metric               TEXT        NOT NULL,
    last_reported_value  BIGINT      NOT NULL,
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    
    PRIMARY KEY (tenant_id, usage_date, metric),
    
    CONSTRAINT chk_ayb_billing_usage_sync_metric
        CHECK (metric IN ('api_requests', 'storage_bytes', 'bandwidth_bytes', 'function_invocations'))
);

CREATE INDEX IF NOT EXISTS idx_ayb_billing_usage_sync_tenant_date
    ON _ayb_billing_usage_sync (tenant_id, usage_date);
