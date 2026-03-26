-- Device tokens for push notifications (FCM/APNS), scoped by app and user.
CREATE TABLE IF NOT EXISTS _ayb_device_tokens (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    app_id            UUID NOT NULL REFERENCES _ayb_apps(id) ON DELETE CASCADE,
    user_id           UUID NOT NULL REFERENCES _ayb_users(id) ON DELETE CASCADE,
    provider          VARCHAR(32) NOT NULL
                          CHECK (provider IN ('fcm', 'apns')),
    platform          VARCHAR(32) NOT NULL
                          CHECK (platform IN ('android', 'ios')),
    token             TEXT NOT NULL
                          CHECK (length(token) > 0 AND length(token) <= 4096),
    device_name       VARCHAR(255),
    is_active         BOOLEAN NOT NULL DEFAULT true,
    last_used         TIMESTAMPTZ,
    last_refreshed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (app_id, provider, token),
    UNIQUE (id, app_id, user_id, provider)
);

CREATE INDEX IF NOT EXISTS idx_ayb_device_tokens_user_active
    ON _ayb_device_tokens (user_id, is_active);

CREATE INDEX IF NOT EXISTS idx_ayb_device_tokens_app_user_active
    ON _ayb_device_tokens (app_id, user_id, is_active);

CREATE INDEX IF NOT EXISTS idx_ayb_device_tokens_active_refreshed
    ON _ayb_device_tokens (is_active, last_refreshed_at);
