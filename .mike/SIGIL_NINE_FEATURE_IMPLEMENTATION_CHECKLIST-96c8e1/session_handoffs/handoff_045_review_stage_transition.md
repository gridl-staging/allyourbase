# Handoff 045 — Stage 2→3 Transition Review

## What I did

1. **Verified Stage 2 completion** — all checklist items in `stage_02_checklist.md` are checked. Stage 2 (OAuth 2.0 Provider Mode) is complete.

2. **Reviewed and rewrote Stage 3 checklist** — the previous session generated `stage_03_checklist.md` with several issues. Researched Postgres-backed job queue best practices (FOR UPDATE SKIP LOCKED pattern), Go cron libraries (robfig/cron v3, adhocore/gronx, hashicorp/cronexpr), and build-vs-buy tradeoffs (River vs custom). Key corrections made:

   - **Dropped `_ayb_job_runs` table** — over-engineering for v1. AYB has a handful of built-in system jobs. Per-attempt audit logging adds schema complexity for no v1 value. Track `attempts`, `last_error`, `last_run_at` directly on the `_ayb_jobs` row.
   - **Dropped priority column** — v1 has only built-in job types processed FIFO. Priority queues add index complexity for zero benefit.
   - **Removed `ayb jobs enqueue` CLI** — v1 only supports built-in job types. No user-facing job creation.
   - **Added concrete migration numbers** — 023 for jobs, 024 for schedules.
   - **Added 4 built-in cleanup handlers** — stale sessions, webhook deliveries, expired OAuth tokens/codes, expired magic links/password resets. Currently expired rows grow forever with no cleanup for any of these.
   - **Added backward-compat constraint** — when `jobs.enabled=false` (default), old timer-based webhook pruner still runs.
   - **Specified config defaults** — concurrency=4, poll=1s, lease=5min, retries=3, scheduler tick=15s.
   - **Added graceful shutdown ordering** — stop scheduler → stop polling → drain in-progress jobs → shut down.
   - **Specified partial indexes** — `WHERE state = 'queued'` for claim performance, `WHERE state = 'running'` for crash recovery scan.
   - **Added explicit non-goals** — no user-facing custom jobs, no external workers, no priority queues, no sub-second precision, no exactly-once guarantees.
   - **Added cron library evaluation** — discovery phase should compare robfig/cron v3, adhocore/gronx, hashicorp/cronexpr.

3. **Updated tracking files** — added Stage 3 review note to `_dev/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST.md`, advanced `state.json` to stage 3.

## Architecture decision: build custom vs use River

Recommendation recorded in checklist: **build custom Postgres-backed queue**. Rationale:
- AYB is a single-binary BaaS with embedded Postgres and its own migration system
- River brings its own migrations, patterns, and UI — unnecessary weight
- The job types are simple built-in system tasks (cleanup), not arbitrary user workloads
- FOR UPDATE SKIP LOCKED is straightforward to implement and well-documented
- Fewer external dependencies = more control over the product

## Files modified

- `.mike/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST-96c8e1/checklists/stage_03_checklist.md` — full rewrite
- `.mike/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST-96c8e1/state.json` — stage 2→3
- `_dev/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST.md` — added Stage 3 review note

## Files created

- `.mike/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST-96c8e1/session_handoffs/handoff_045_review_stage_transition.md`

## Previous handoff reference

- `.mike/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST-96c8e1/session_handoffs/handoff_044_stage_transition.md`

## What's next

Stage 3 implementation should begin with the Discovery & Design phase:
1. Read existing background-process code paths (webhook dispatcher, rate limiter cleanup goroutines, server lifecycle)
2. Choose cron parsing library
3. Record architecture decision (custom Postgres queue with FOR UPDATE SKIP LOCKED)
4. Write migration SQL for `_ayb_jobs` (023) and `_ayb_job_schedules` (024)
5. TDD the queue engine: state machine, claim safety, crash recovery, backoff
6. Implement scheduler loop with cron-based next_run_at computation
7. Implement built-in job handlers and wire default schedules
8. Admin API + CLI + dashboard
9. Config wiring and server lifecycle integration
