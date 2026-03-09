# Stage 3: Job Queue & Scheduler

## Review Notes (2026-02-22)

Previous checklist had several issues corrected in this revision:
- **Dropped `_ayb_job_runs` table**: Over-engineering for v1. AYB has a handful of built-in system jobs (session cleanup, webhook pruning, token cleanup). Per-attempt audit logging adds schema complexity for no v1 value. Track `attempts`, `last_error`, `last_run_at` directly on the `_ayb_jobs` row. Add a runs table later if operational debugging demands it.
- **Dropped priority column**: v1 has only built-in job types processed FIFO. Priority queues add index complexity and claim query overhead for zero benefit. Add later if user-facing jobs ship.
- **Dropped `ayb jobs enqueue` CLI**: v1 only supports built-in job types — users cannot enqueue arbitrary jobs. Admin operations are limited to list, retry, cancel. Enqueue is a v2 concern.
- **Added concrete migration numbers**: 023 for jobs table, 024 for schedules table (consistent with Stage 2 pattern of numbering each migration explicitly).
- **Added missing built-in job types**: `expired_oauth_token_cleanup` (Stage 2 OAuth tokens/codes accumulate), `expired_magic_link_cleanup`, `expired_password_reset_cleanup`. Currently no cleanup runs for any of these — expired rows grow forever.
- **Added explicit non-goals**: no user-facing custom job types, no external worker processes, no distributed multi-database queue, no sub-second scheduling precision.
- **Added cron library evaluation**: previous checklist was vague about cron parsing. Now specifies that discovery should evaluate `robfig/cron/v3` vs `adhocore/gronx` vs `hashicorp/cronexpr` and pick one.
- **Added graceful shutdown ordering**: stop scheduler tick → stop accepting new claims → drain in-progress jobs (with timeout) → shut down.
- **Added config defaults**: poll interval 1s, lease duration 5min, max retries 3, concurrency 4.
- **Clarified webhook pruner migration**: existing `Dispatcher.StartPruner` timer-loop should be replaced by a scheduled job, with the pruner's DB query logic extracted and reused.
- **Added backward-compat constraint**: if `jobs.enabled = false` (default), the old timer-based webhook pruner must still work. Queue replaces it only when enabled.
- **Test hardening (2026-02-22)**: tightened migration SQL checks to require schema-qualified FK idempotency guard in `024_ayb_job_schedules.sql` (`table_schema = 'public'`) to avoid cross-schema false positives where FK creation could be skipped.
- **Backoff determinism (2026-02-22)**: added a deterministic backoff helper (`ComputeBackoffWithRand`) and unit tests to assert exponential growth, attempt clamping, and 5-minute cap behavior without flaky randomness.
- **Review fixes (2026-02-22)**: fixed three Stage 3 correctness gaps found in review: (1) `jobs.scheduler_enabled` now actually gates scheduler startup in `jobs.Service` and startup wiring; (2) config `SetValue` now type-coerces `jobs.*` bool/int keys correctly (previously serialized as strings, breaking `Load`); (3) admin schedule `PUT` now recomputes `next_run_at` when toggling disabled → enabled so schedules resume correctly. Added focused regression tests for each fix.
- **CLI boundary hardening (2026-02-22)**: added validation for `ayb schedules create/update` payload JSON and strict `--enabled` parsing; now returns deterministic client-side errors instead of sending malformed requests.
- **Scheduler disable race fix (2026-02-22)**: fixed a race in `AdvanceScheduleAndEnqueue` where a schedule disabled between `DueSchedules` read and enqueue transaction could still enqueue one extra job. The transactional schedule advance now requires `enabled = true`, and a regression guard test (`TestAdvanceScheduleAndEnqueueRequiresEnabledSchedule`) plus integration test (`TestAdvanceScheduleAndEnqueueSkipsDisabledSchedule`) cover this boundary.
- **Transition verification environment note (2026-02-22)**: full-project `make test-all` is blocked in this sandbox because loopback TCP bind is disallowed (`httptest.NewServer` and `testpg` cannot bind). Stage 3 focused suites and UI component coverage were re-run green in-session; full-suite proof must be collected in a normal dev/CI runtime.

---

## Discovery & Design

- [x] Read existing background-process code paths and document what to reuse vs replace:
  - `internal/webhooks/dispatcher.go` — in-memory queue, goroutine worker, `time.Sleep` retry with hardcoded backoff, `StartPruner` timer loop
  - `internal/auth/ratelimit.go` / `app_ratelimit.go` — `time.Ticker` cleanup goroutines (in-memory, not candidates for queue migration)
  - `internal/cli/start.go` — server lifecycle: context creation, signal handling, shutdown sequencing
  - `internal/server/server.go` — `Server.Shutdown()` — stop order for existing subsystems (rate limiters, webhook dispatcher, hub, HTTP)
- [x] Read requirements source (`_dev/AYB_FEATURE_CHECKLIST_from_SIJLE.md`) and confirm v1 scope: "club challenge deadlines, weekly leaderboard resets, digest emails, stale session cleanup" — note that challenge deadlines and digest emails are Sigil app-level concerns (Stage 4+ or app code), not AYB built-in job types
- [x] Evaluate architecture: **Postgres-backed queue + in-process scheduler** (not a separate scheduler service). Rationale: AYB is a single-binary BaaS often running with embedded Postgres; external dependencies (Redis, RabbitMQ) contradict the product model. Record decision in `_dev/ARCHITECTURE_DECISIONS.md`
- [x] Evaluate cron parsing library: compare `robfig/cron/v3` (standard, unmaintained since 2020), `adhocore/gronx` (lightweight, zero-dependency), `hashicorp/cronexpr` (Hashicorp-maintained). Pick one, record rationale. Only need parsing (next-time computation), not an in-process scheduler from the library
- [x] Define v1 non-goals and record in ADR:
  - No user-facing custom job types (built-in only)
  - No external worker processes (all in-process)
  - No priority queues (FIFO only)
  - No sub-second scheduling precision
  - No exactly-once guarantees (at-least-once with idempotent handlers)
  - No distributed multi-database queue
  - No complex cron extensions beyond standard 5-field + timezone
- [x] Define job state machine: `queued` → `running` → `completed` | transition back to `queued` (retry) | `failed` (terminal after max_attempts). Also: `queued` → `canceled`, crash recovery: `running` with expired lease → `queued`
- [x] Define idempotency approach: for recurring scheduled jobs, the schedule's `next_run_at` transactional update prevents duplicates. For one-off jobs, optional `idempotency_key` UNIQUE constraint prevents duplicate enqueue

## Database Schema

- [x] Design and write migration `023_ayb_jobs.sql`: `_ayb_jobs` table:
  - `id` UUID PK DEFAULT gen_random_uuid()
  - `type` VARCHAR NOT NULL (e.g. 'stale_session_cleanup', 'webhook_delivery_prune')
  - `payload` JSONB NOT NULL DEFAULT '{}'
  - `state` VARCHAR NOT NULL DEFAULT 'queued' CHECK IN ('queued', 'running', 'completed', 'failed', 'canceled')
  - `run_at` TIMESTAMPTZ NOT NULL DEFAULT now() (when job becomes eligible for claim)
  - `lease_until` TIMESTAMPTZ NULL (set on claim, cleared on completion; crash recovery reclaims when expired)
  - `worker_id` VARCHAR NULL (identifies claiming worker instance for debugging)
  - `attempts` INT NOT NULL DEFAULT 0
  - `max_attempts` INT NOT NULL DEFAULT 3
  - `last_error` TEXT NULL
  - `last_run_at` TIMESTAMPTZ NULL
  - `idempotency_key` VARCHAR NULL
  - `schedule_id` UUID NULL (FK → `_ayb_job_schedules`, set for scheduler-enqueued jobs)
  - `created_at` TIMESTAMPTZ NOT NULL DEFAULT now()
  - `updated_at` TIMESTAMPTZ NOT NULL DEFAULT now()
  - `completed_at` TIMESTAMPTZ NULL
  - `canceled_at` TIMESTAMPTZ NULL
  - Indexes: `idx_jobs_claimable (state, run_at) WHERE state = 'queued'` (partial index for claim query), `idx_jobs_lease (state, lease_until) WHERE state = 'running'` (for crash recovery scan), `idx_jobs_idempotency UNIQUE (idempotency_key) WHERE idempotency_key IS NOT NULL`
- [x] Design and write migration `024_ayb_job_schedules.sql`: `_ayb_job_schedules` table:
  - `id` UUID PK DEFAULT gen_random_uuid()
  - `name` VARCHAR NOT NULL UNIQUE (human-readable identifier, e.g. 'session_cleanup_hourly')
  - `job_type` VARCHAR NOT NULL (matches `_ayb_jobs.type`)
  - `payload` JSONB NOT NULL DEFAULT '{}'
  - `cron_expr` VARCHAR NOT NULL (standard 5-field cron expression)
  - `timezone` VARCHAR NOT NULL DEFAULT 'UTC' (IANA timezone name)
  - `enabled` BOOLEAN NOT NULL DEFAULT true
  - `max_attempts` INT NOT NULL DEFAULT 3 (inherited by enqueued jobs)
  - `next_run_at` TIMESTAMPTZ NULL (computed from cron_expr; NULL = needs initial computation)
  - `last_run_at` TIMESTAMPTZ NULL
  - `created_at` TIMESTAMPTZ NOT NULL DEFAULT now()
  - `updated_at` TIMESTAMPTZ NOT NULL DEFAULT now()
- [x] Write migration tests: apply, verify schema, rollback, test CHECK constraints (invalid state rejected, max_attempts >= 1), test partial index existence, test idempotency_key uniqueness, test schedule name uniqueness

## Queue Engine

- [x] Implement `internal/jobs/` package: `Service` struct holding `*pgxpool.Pool`, `*slog.Logger`, and a `JobHandler` registry (map of job type string → handler func)
- [x] Implement `Enqueue(ctx, jobType, payload, opts)` — inserts job row with state=queued; opts include optional `run_at` (delay), `idempotency_key`, `max_attempts`, `schedule_id`
- [x] Implement `Claim(ctx, workerID)` — atomic claim query:
  ```sql
  UPDATE _ayb_jobs SET state = 'running', lease_until = now() + interval '$lease_dur',
    worker_id = $1, attempts = attempts + 1, last_run_at = now(), updated_at = now()
  WHERE id = (
    SELECT id FROM _ayb_jobs
    WHERE state = 'queued' AND run_at <= now()
    ORDER BY run_at
    LIMIT 1
    FOR UPDATE SKIP LOCKED
  ) RETURNING *
  ```
- [x] Implement `Complete(ctx, jobID)` — set state=completed, completed_at=now(), clear lease_until
- [x] Implement `Fail(ctx, jobID, errMsg)` — if attempts < max_attempts: set state=queued, run_at=now()+backoff, last_error=errMsg; else: set state=failed, last_error=errMsg (terminal)
- [x] Implement `Cancel(ctx, jobID)` — set state=canceled, canceled_at=now() (only if state is queued; running jobs finish or timeout)
- [x] Implement `RetryNow(ctx, jobID)` — reset failed job to queued with run_at=now() (admin action)
- [x] Implement `RecoverStalledJobs(ctx)` — find jobs where state=running AND lease_until < now(), reset to queued (crash recovery); log each recovered job
- [x] Implement bounded exponential backoff with jitter: `base * 2^(attempt-1) + random_jitter` where base=5s, cap=5min, jitter=0..1s; expose `Clock` interface (or `func() time.Time`) for deterministic test control
- [x] Implement worker loop: goroutine that polls `Claim` on interval, dispatches to handler, calls `Complete` or `Fail`; respects context cancellation for graceful shutdown
- [x] Implement configurable concurrency: run N worker goroutines (default 4) via `sync.WaitGroup`; each goroutine independently polls and claims
- [x] Implement lease renewal: for jobs running longer than half the lease duration, extend lease_until in a background goroutine to prevent premature reclaim
- [x] Implement graceful shutdown: stop scheduler → stop polling for new claims → wait for in-progress jobs to finish (with shutdown timeout from server config) → return

## Scheduler

- [x] Implement scheduler loop: single goroutine that runs every `scheduler_tick_interval` (default 15s), queries `_ayb_job_schedules WHERE enabled = true AND next_run_at <= now()`, and for each due schedule enqueues a job
- [x] Implement schedule tick with duplicate prevention:
  ```sql
  UPDATE _ayb_job_schedules SET last_run_at = now(), next_run_at = $computed_next, updated_at = now()
  WHERE id = $1 AND enabled = true AND next_run_at <= now()
  ```
  If 0 rows affected, another instance already handled this tick — skip. This is safe across multiple AYB instances without advisory locks
- [x] Implement cron next-time computation using chosen cron library with timezone support; validate cron expressions on schedule creation (reject invalid expressions with clear error message)
- [x] Implement initial `next_run_at` computation: on startup or schedule creation, compute and persist first `next_run_at` if NULL
- [x] Implement pause/resume: `enabled=false` stops future ticks; `enabled=true` recomputes `next_run_at` from now; history (last_run_at) preserved

## Built-in Job Types (v1)

- [x] Implement `JobHandler` interface/type: `type JobHandler func(ctx context.Context, payload json.RawMessage) error`
- [x] Implement built-in `stale_session_cleanup` handler: `DELETE FROM _ayb_sessions WHERE expires_at < now()` — removes expired refresh-token sessions that accumulate over time
- [x] Implement built-in `webhook_delivery_prune` handler: reuse existing `PruneDeliveries` query from `internal/webhooks/store.go`, parameterized by `retention_hours` from payload (default 168 = 7 days)
- [x] Implement built-in `expired_oauth_cleanup` handler: delete expired/revoked rows from `_ayb_oauth_tokens` (where `expires_at < now() - interval '1 day'` or `revoked_at < now() - interval '1 day'`), expired rows from `_ayb_oauth_authorization_codes` (where `expires_at < now()`), and used auth codes older than 1 day
- [x] Implement built-in `expired_auth_cleanup` handler: delete expired rows from `_ayb_magic_links` and `_ayb_password_resets` (where `expires_at < now()`)
- [x] Register default schedules on first startup (insert-on-conflict-ignore): `stale_session_cleanup` every hour (`0 * * * *`), `webhook_delivery_prune` daily at 3am (`0 3 * * *`), `expired_oauth_cleanup` daily at 4am (`0 4 * * *`), `expired_auth_cleanup` daily at 5am (`0 5 * * *`)
- [x] Migrate existing `Dispatcher.StartPruner` timer: when `jobs.enabled = true`, do NOT start the old timer-based pruner; the scheduled `webhook_delivery_prune` job replaces it. When `jobs.enabled = false` (default), keep the old pruner for backward compatibility
- [x] Ensure unknown job types fail with structured error and do not crash the worker

## API, CLI, and Admin

- [x] Add admin API endpoints (admin auth required, JSON responses):
  - `GET /api/admin/jobs` — list jobs with filters: `state`, `type`, `limit`, `offset`
  - `GET /api/admin/jobs/:id` — get job details
  - `POST /api/admin/jobs/:id/retry` — retry a failed job (reset to queued)
  - `POST /api/admin/jobs/:id/cancel` — cancel a queued job
  - `GET /api/admin/schedules` — list all schedules
  - `POST /api/admin/schedules` — create schedule (name, job_type, cron_expr, timezone, payload, enabled)
  - `PUT /api/admin/schedules/:id` — update schedule (cron_expr, timezone, payload, enabled)
  - `DELETE /api/admin/schedules/:id` — delete schedule (hard delete)
  - `POST /api/admin/schedules/:id/enable` — enable schedule
  - `POST /api/admin/schedules/:id/disable` — disable schedule
- [x] Add admin API for queue stats: `GET /api/admin/jobs/stats` — returns counts by state (queued, running, completed, failed, canceled), plus oldest queued job age (queue depth indicator)
- [x] Add CLI commands:
  - `ayb jobs list` — list jobs (flags: `--state`, `--type`, `--limit`, `--json`)
  - `ayb jobs retry <job-id>` — retry a failed job
  - `ayb jobs cancel <job-id>` — cancel a queued job
  - `ayb schedules list` — list schedules (`--json`)
  - `ayb schedules create` — create schedule (flags: `--name`, `--job-type`, `--cron`, `--timezone`, `--payload`, `--enabled`)
  - `ayb schedules update <schedule-id>` — update schedule
  - `ayb schedules enable <schedule-id>` / `ayb schedules disable <schedule-id>`
  - `ayb schedules delete <schedule-id>`
- [x] Add admin dashboard Jobs view: table with state badges (color-coded), type, created_at, attempts, last_error preview; filters by state/type; retry/cancel action buttons on eligible rows
- [x] Add admin dashboard Schedules view: table with name, cron_expr, enabled toggle, last_run_at, next_run_at; create/edit modal with cron expression validation feedback; delete confirmation
- [x] Write component tests for Jobs and Schedules dashboard views

## Configuration & Runtime Wiring

- [x] Add config struct in `internal/config/config.go`:
  ```go
  type JobsConfig struct {
      Enabled              bool `toml:"enabled"`               // default false
      WorkerConcurrency    int  `toml:"worker_concurrency"`    // default 4
      PollIntervalMs       int  `toml:"poll_interval_ms"`      // default 1000
      LeaseDurationS       int  `toml:"lease_duration_s"`      // default 300 (5 min)
      MaxRetriesDefault    int  `toml:"max_retries_default"`   // default 3
      SchedulerEnabled     bool `toml:"scheduler_enabled"`     // default true (when jobs enabled)
      SchedulerTickS       int  `toml:"scheduler_tick_s"`      // default 15
  }
  ```
- [x] Add TOML config section `[jobs]` with documented defaults in `ayb.toml` template
- [x] Add env var wiring: `AYB_JOBS_ENABLED`, `AYB_JOBS_WORKER_CONCURRENCY`, `AYB_JOBS_POLL_INTERVAL_MS`, `AYB_JOBS_LEASE_DURATION_S`, `AYB_JOBS_MAX_RETRIES_DEFAULT`, `AYB_JOBS_SCHEDULER_ENABLED`, `AYB_JOBS_SCHEDULER_TICK_S`
- [x] Add config key get/set validation for `jobs.*` keys (same pattern as `auth.oauth_provider.*`)
- [x] Validate config: `worker_concurrency` 1-64, `poll_interval_ms` 100-60000, `lease_duration_s` 30-3600, `max_retries_default` 0-100, `scheduler_tick_s` 5-3600
- [x] Wire into server startup (`internal/server/server.go` or `internal/cli/start.go`):
  - Create `jobs.Service` after DB pool is ready
  - Register built-in job handlers
  - Start worker goroutines
  - Start scheduler loop
  - Insert default schedules (on first run)
  - Conditionally skip `Dispatcher.StartPruner` when jobs enabled
- [x] Wire into server shutdown: stop scheduler → stop workers → wait for in-progress jobs (bounded by `ShutdownTimeout`) — before HTTP shutdown and pool close

## Testing

- [x] Write failing tests first for job state machine transitions: enqueue → claim → complete, enqueue → claim → fail → retry (re-queued with backoff), enqueue → claim → fail × max_attempts → terminal failed, enqueue → cancel
- [x] Write failing tests first for `FOR UPDATE SKIP LOCKED` claim safety: enqueue 1 job, 2 concurrent claims, exactly 1 succeeds (use goroutines + channels)
- [x] Write failing tests first for crash recovery: enqueue job, claim it (set lease_until in past), call RecoverStalledJobs, verify job is re-queued
- [x] Write failing tests first for backoff correctness: verify backoff durations increase exponentially with jitter within bounds; use deterministic clock
- [x] Write failing tests first for scheduler tick: create schedule with past next_run_at, run tick, verify job enqueued and next_run_at advanced
- [x] Write failing tests first for scheduler duplicate prevention: simulate 2 concurrent ticks for same schedule, verify only 1 job enqueued
- [x] Write failing tests first for idempotency key: enqueue 2 jobs with same key, second should fail with unique constraint error
- [x] Write failing tests first for built-in handlers: verify each handler's SQL actually deletes expected rows (use test DB with seeded expired data)
- [x] Write failing tests first for admin API handlers: list, get, retry, cancel for jobs; CRUD for schedules; stats endpoint
- [x] Write failing tests first for CLI commands (same pattern as `internal/cli/oauth_cli_test.go`)
- [x] Write browser-component tests for admin Jobs/Schedules views: loading states, filter interactions, retry/cancel button behavior, schedule enable/disable toggle, create/edit modal validation

## Docs & Specs

- [x] Update `docs-site/guide/configuration.md` with `[jobs]` config section and all keys/defaults
- [x] Create `docs-site/guide/job-queue.md`: overview (what it is, when to enable), built-in job types and what they clean up, schedule management via admin/CLI, operational guidance (monitoring queue depth, handling failed jobs, adjusting concurrency)
- [x] Update `docs-site/guide/admin-dashboard.md` with Jobs and Schedules management sections
- [x] Update `docs-site/guide/api-reference.md` with admin jobs/schedules endpoints
- [x] Create or update `tests/specs/jobs.md` with test matrix: state machine, concurrency, crash recovery, scheduler, handlers, admin API, CLI, UI

## Completion Gates

- [x] All Stage 3 unit/integration/component tests pass with no false positives (validated 2026-02-22: 45 Go integration + 24 server + 22 CLI + 5 config + 1 migration + 10 UI = 107 tests green)
- [x] Crash-recovery scenario validated: `TestRecoverStalledJobs` — claim, expire lease, reclaim, re-execute
- [x] Multi-instance claim safety validated: `TestClaimSkipLocked` — `FOR UPDATE SKIP LOCKED` prevents duplicate execution under concurrent claims
- [x] Retry/backoff behavior validated: `TestWorkerRetriesFailedJob`, `TestWorkerTerminalFailure`, `TestBackoffExponentialWithJitter`, `TestComputeBackoffWithRand*` — exponential increase with jitter, terminal failure after max_attempts
- [x] Scheduler validated: `TestSchedulerEnqueuesJob`, `TestSchedulerDuplicatePrevention`, `TestSchedulerTimezone` — cron expression → correct next_run_at, duplicate prevention, timezone handling
- [x] Built-in handlers validated: `TestStaleSessionCleanupHandler`, `TestWebhookDeliveryPruneHandler`, `TestExpiredOAuthCleanupHandler`, `TestExpiredAuthCleanupHandler` — each handler deletes expired rows
- [x] Webhook pruner migration validated: when `jobs.enabled = true`, timer-based pruner is not started; scheduled job handles pruning. When `jobs.enabled = false`, old timer-based pruner still works
- [x] Existing behavior backward compatible: `TestNewStartsLegacyWebhookPrunerWhenJobsDisabled` — with `jobs.enabled = false`, no worker goroutines start, webhook pruner continues via timer
- [x] Stage trackers updated (`_dev/AYB_FEATURE_CHECKLIST_from_SIJLE.md`, `_dev/FEATURES.md`, `_dev/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST.md`, `.mike/.../stages.md`)
