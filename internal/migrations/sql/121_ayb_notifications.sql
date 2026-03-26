-- In-app notifications for authenticated users.
-- Each notification belongs to a single user and tracks read state.
CREATE TABLE IF NOT EXISTS _ayb_notifications (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID NOT NULL,
    title      TEXT NOT NULL,
    body       TEXT,
    metadata   JSONB NOT NULL DEFAULT '{}',
    channel    TEXT NOT NULL DEFAULT 'general',
    read_at    TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Paginated list queries: newest first, scoped to user.
CREATE INDEX IF NOT EXISTS idx_notifications_user_created
    ON _ayb_notifications (user_id, created_at DESC);

-- Unread filter queries.
CREATE INDEX IF NOT EXISTS idx_notifications_user_readat
    ON _ayb_notifications (user_id, read_at);

-- Row-level security: users see only their own notifications.
ALTER TABLE _ayb_notifications ENABLE ROW LEVEL SECURITY;

CREATE POLICY notif_user_owns ON _ayb_notifications
    FOR ALL
    TO ayb_authenticated
    USING (user_id::text = current_setting('ayb.user_id', true));
