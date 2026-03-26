-- Stage 5: Incident parent records for public/admin status surfaces.

CREATE TABLE IF NOT EXISTS _ayb_incidents (
    id                UUID        NOT NULL DEFAULT gen_random_uuid() PRIMARY KEY,
    title             TEXT        NOT NULL,
    status            TEXT        NOT NULL DEFAULT 'investigating',
    affected_services TEXT[]      NOT NULL DEFAULT '{}',
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    resolved_at       TIMESTAMPTZ NULL,

    CONSTRAINT chk_ayb_incidents_status
        CHECK (status IN ('investigating', 'identified', 'monitoring', 'resolved'))
);

CREATE INDEX IF NOT EXISTS idx_ayb_incidents_status_created_at
    ON _ayb_incidents (status, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_ayb_incidents_created_at
    ON _ayb_incidents (created_at DESC);
