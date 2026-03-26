-- Stage 5: Incident timeline updates linked to parent incidents.

CREATE TABLE IF NOT EXISTS _ayb_incident_updates (
    id          UUID        NOT NULL DEFAULT gen_random_uuid() PRIMARY KEY,
    incident_id UUID        NOT NULL REFERENCES _ayb_incidents(id) ON DELETE CASCADE,
    message     TEXT        NOT NULL,
    status      TEXT        NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_ayb_incident_updates_status
        CHECK (status IN ('investigating', 'identified', 'monitoring', 'resolved'))
);

CREATE INDEX IF NOT EXISTS idx_ayb_incident_updates_incident_created_at
    ON _ayb_incident_updates (incident_id, created_at ASC);
