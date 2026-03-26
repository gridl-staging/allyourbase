-- 048: async request logging table
CREATE TABLE IF NOT EXISTS _ayb_request_logs (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    timestamp     TIMESTAMPTZ NOT NULL DEFAULT now(),
    method        TEXT,
    path          TEXT,
    status_code   INT,
    duration_ms   BIGINT,
    user_id       UUID,
    api_key_id    UUID,
    request_size  BIGINT,
    response_size BIGINT,
    ip_address    INET,
    request_id    TEXT
);

CREATE INDEX IF NOT EXISTS idx_ayb_request_logs_timestamp
    ON _ayb_request_logs (timestamp);

CREATE INDEX IF NOT EXISTS idx_ayb_request_logs_path_ts
    ON _ayb_request_logs (path, timestamp);
