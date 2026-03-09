-- Webhook delivery logs — persists delivery attempts for debugging.
CREATE TABLE IF NOT EXISTS _ayb_webhook_deliveries (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    webhook_id    UUID NOT NULL REFERENCES _ayb_webhooks(id) ON DELETE CASCADE,
    event_action  TEXT NOT NULL,
    event_table   TEXT NOT NULL,
    success       BOOLEAN NOT NULL,
    status_code   INTEGER,
    attempt       INTEGER NOT NULL DEFAULT 1,
    duration_ms   INTEGER NOT NULL DEFAULT 0,
    error         TEXT NOT NULL DEFAULT '',
    request_body  TEXT NOT NULL DEFAULT '',
    response_body TEXT NOT NULL DEFAULT '',
    delivered_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_webhook_id
    ON _ayb_webhook_deliveries (webhook_id, delivered_at DESC);
