-- Cron trigger linking table: maps edge functions to job schedules for periodic execution.
CREATE TABLE IF NOT EXISTS _ayb_edge_cron_triggers (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    function_id     UUID NOT NULL REFERENCES _ayb_edge_functions(id) ON DELETE CASCADE,
    schedule_id     UUID NOT NULL REFERENCES _ayb_job_schedules(id) ON DELETE CASCADE,
    payload         JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ayb_edge_cron_triggers_function
    ON _ayb_edge_cron_triggers (function_id);
