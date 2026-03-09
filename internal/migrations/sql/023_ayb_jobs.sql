-- Job queue: persistent background jobs with claim-based processing.
CREATE TABLE IF NOT EXISTS _ayb_jobs (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    type            VARCHAR(100) NOT NULL,
    payload         JSONB NOT NULL DEFAULT '{}',
    state           VARCHAR(20) NOT NULL DEFAULT 'queued'
                        CHECK (state IN ('queued', 'running', 'completed', 'failed', 'canceled')),
    run_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    lease_until     TIMESTAMPTZ,
    worker_id       VARCHAR(100),
    attempts        INT NOT NULL DEFAULT 0,
    max_attempts    INT NOT NULL DEFAULT 3 CHECK (max_attempts >= 1),
    last_error      TEXT,
    last_run_at     TIMESTAMPTZ,
    idempotency_key VARCHAR(255),
    schedule_id     UUID,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at    TIMESTAMPTZ,
    canceled_at     TIMESTAMPTZ
);

-- Partial index for efficient claim queries: only queued jobs eligible for pickup.
CREATE INDEX IF NOT EXISTS idx_ayb_jobs_claimable
    ON _ayb_jobs (state, run_at) WHERE state = 'queued';

-- Partial index for crash recovery: find running jobs with expired leases.
CREATE INDEX IF NOT EXISTS idx_ayb_jobs_lease
    ON _ayb_jobs (state, lease_until) WHERE state = 'running';

-- Unique partial index for idempotency: prevent duplicate enqueue by key.
CREATE UNIQUE INDEX IF NOT EXISTS idx_ayb_jobs_idempotency
    ON _ayb_jobs (idempotency_key) WHERE idempotency_key IS NOT NULL;
