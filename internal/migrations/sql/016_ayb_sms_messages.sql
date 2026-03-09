CREATE TABLE IF NOT EXISTS _ayb_sms_messages (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES _ayb_users(id),
    api_key_id UUID REFERENCES _ayb_api_keys(id),
    to_phone TEXT NOT NULL,
    body TEXT NOT NULL,
    provider TEXT NOT NULL DEFAULT '',
    provider_message_id TEXT DEFAULT '',
    status TEXT NOT NULL DEFAULT 'pending',
    error_message TEXT DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_sms_messages_user_created
    ON _ayb_sms_messages(user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_sms_messages_status
    ON _ayb_sms_messages(status);
CREATE INDEX IF NOT EXISTS idx_sms_messages_provider_msg_id
    ON _ayb_sms_messages(provider_message_id)
    WHERE provider_message_id != '';
