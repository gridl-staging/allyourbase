-- Push delivery audit records. Retry orchestration is handled by _ayb_jobs.
CREATE TABLE IF NOT EXISTS _ayb_push_deliveries (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    device_token_id     UUID NOT NULL,
    job_id              UUID REFERENCES _ayb_jobs(id) ON DELETE SET NULL,
    app_id              UUID NOT NULL,
    user_id             UUID NOT NULL,
    provider            VARCHAR(32) NOT NULL
                            CHECK (provider IN ('fcm', 'apns')),
    title               TEXT NOT NULL
                            CHECK (length(title) > 0 AND length(title) <= 256),
    body                TEXT NOT NULL
                            CHECK (length(body) > 0 AND length(body) <= 4096),
    data_payload        JSONB
                            CHECK (data_payload IS NULL OR length(data_payload::text) <= 8192),
    status              VARCHAR(32) NOT NULL DEFAULT 'pending'
                            CHECK (status IN ('pending', 'sent', 'failed', 'invalid_token')),
    error_code          VARCHAR(255),
    error_message       TEXT,
    provider_message_id TEXT,
    sent_at             TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (device_token_id, app_id, user_id, provider)
        REFERENCES _ayb_device_tokens(id, app_id, user_id, provider) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_ayb_push_deliveries_app_user_created
    ON _ayb_push_deliveries (app_id, user_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_ayb_push_deliveries_status
    ON _ayb_push_deliveries (status);
