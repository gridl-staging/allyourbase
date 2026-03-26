-- Migration 043: Create audit log table for tracking data mutations.

CREATE TABLE IF NOT EXISTS _ayb_audit_log (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    timestamp  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    user_id    UUID,
    api_key_id UUID,
    table_name TEXT        NOT NULL,
    record_id  JSONB,
    operation  TEXT        NOT NULL CHECK (operation IN ('INSERT', 'UPDATE', 'DELETE')),
    old_values JSONB,
    new_values JSONB,
    ip_address INET
);

CREATE INDEX IF NOT EXISTS idx_ayb_audit_log_table_ts
    ON _ayb_audit_log (table_name, timestamp);

CREATE INDEX IF NOT EXISTS idx_ayb_audit_log_user_ts
    ON _ayb_audit_log (user_id, timestamp);
