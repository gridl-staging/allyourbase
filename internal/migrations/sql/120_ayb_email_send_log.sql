-- Email send audit log for the public email API.
-- Tracks every send attempt for auditing and analytics.
CREATE TABLE IF NOT EXISTS _ayb_email_send_log (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    api_key_id  UUID,
    user_id     UUID,
    from_addr   TEXT NOT NULL,
    to_addr     TEXT NOT NULL,
    subject     TEXT NOT NULL,
    template_key TEXT,
    status      TEXT NOT NULL CHECK (status IN ('sent', 'failed')),
    error_msg   TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_email_send_log_apikey_ts
    ON _ayb_email_send_log (api_key_id, created_at);

CREATE INDEX IF NOT EXISTS idx_email_send_log_user_ts
    ON _ayb_email_send_log (user_id, created_at);
