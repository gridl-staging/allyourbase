-- Job schedules: recurring job definitions with cron expressions.
CREATE TABLE IF NOT EXISTS _ayb_job_schedules (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name            VARCHAR(100) NOT NULL UNIQUE,
    job_type        VARCHAR(100) NOT NULL,
    payload         JSONB NOT NULL DEFAULT '{}',
    cron_expr       VARCHAR(100) NOT NULL,
    timezone        VARCHAR(50) NOT NULL DEFAULT 'UTC',
    enabled         BOOLEAN NOT NULL DEFAULT true,
    max_attempts    INT NOT NULL DEFAULT 3 CHECK (max_attempts >= 1),
    next_run_at     TIMESTAMPTZ,
    last_run_at     TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Add FK from _ayb_jobs.schedule_id → _ayb_job_schedules.id.
-- Uses DO block for idempotency (IF NOT EXISTS for FK constraints not supported directly).
DO $$ BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.table_constraints
        WHERE constraint_name = 'fk_ayb_jobs_schedule'
        AND table_name = '_ayb_jobs'
        AND table_schema = 'public'
    ) THEN
        ALTER TABLE _ayb_jobs
            ADD CONSTRAINT fk_ayb_jobs_schedule
            FOREIGN KEY (schedule_id) REFERENCES _ayb_job_schedules(id) ON DELETE SET NULL;
    END IF;
END $$;
