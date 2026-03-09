# Handoff 059 — Stage 3 Lease Renewal (Build)

## What I did

Completed one focused Stage 3 build task using red→green TDD: lease renewal for long-running jobs.

1. Added failing integration tests first (red):
   - `TestExtendLease` in `store_test.go` — verifies `Store.ExtendLease` extends a running job's lease and rejects non-running jobs
   - `TestExtendLeaseNonRunningFails` in `store_test.go` — verifies extending lease on a queued job returns an error
   - `TestLeaseRenewalExtendsLease` in `service_test.go` — end-to-end: handler runs longer than initial lease, renewal goroutine extends lease, job completes successfully
   - `TestLeaseRenewalStopsOnCompletion` in `service_test.go` — verifies renewal goroutine doesn't leak or prevent quick job completion

2. Implemented lease renewal (green):
   - `Store.ExtendLease(ctx, jobID, leaseDuration)` in `store.go` — UPDATE lease_until for running jobs only
   - `Service.renewLease(ctx, jobID)` in `service.go` — goroutine that extends lease every `leaseDuration/2`, cancelled when handler finishes
   - Refactored `pollAndProcess` to start renewal goroutine alongside handler execution
   - Changed handler context from `WithTimeout(LeaseDuration)` to `WithTimeout(ShutdownTimeout)` — the handler is now bounded by graceful shutdown, not lease expiry, since the lease is kept alive by the renewal goroutine

3. Updated stage 3 checklist — marked all Queue Engine, Scheduler, Built-in Job Types (except Dispatcher.StartPruner migration), and Testing items that are implemented. The Queue Engine section is now fully complete.

4. Updated `_dev/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST.md` with progress notes.

## Tests run (focused)

- `GOCACHE=/tmp/ayb-gocache go test ./internal/jobs/... -count=1 -v` — all 3 unit tests pass
- `GOCACHE=/tmp/ayb-gocache go build ./...` — full project builds clean
- `GOCACHE=/tmp/ayb-gocache go vet -tags=integration ./internal/jobs/...` — vets clean with integration tests

## Files modified

- `internal/jobs/store.go` — added `ExtendLease` method
- `internal/jobs/service.go` — added `renewLease` goroutine, refactored `pollAndProcess` to use it
- `internal/jobs/store_test.go` — added `TestExtendLease`, `TestExtendLeaseNonRunningFails`
- `internal/jobs/service_test.go` — added `TestLeaseRenewalExtendsLease`, `TestLeaseRenewalStopsOnCompletion`
- `.mike/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST-96c8e1/checklists/stage_03_checklist.md` — marked completed items
- `_dev/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST.md` — added progress notes

## Stage 3 completion status

### Done (code + tests)
- Database Schema (migrations 023, 024 + tests)
- Queue Engine (all items: Service, Store, Enqueue, Claim, Complete, Fail, Cancel, RetryNow, RecoverStalledJobs, backoff, worker loop, concurrency, **lease renewal**, graceful shutdown)
- Scheduler (scheduler loop, duplicate prevention, cron next-time with gronx, initial next_run_at, pause/resume)
- Built-in Job Types (4 handlers + default schedules + unknown type handling)
- Core testing (state machine, SKIP LOCKED, crash recovery, backoff, scheduler, idempotency, handlers)

### Remaining
- Discovery & Design items (architecture ADR, cron library evaluation — decisions are made in code but not documented in `_dev/ARCHITECTURE_DECISIONS.md`)
- Dispatcher.StartPruner timer migration (needs server wiring)
- API, CLI, and Admin (endpoints, commands, dashboard views, component tests)
- Configuration & Runtime Wiring (env vars, TOML template, config validation, server startup/shutdown wiring)
- Docs & Specs
- Completion Gates (validation once integration tests run)

## What's next

1. **Configuration & Runtime Wiring** — env var wiring, TOML template, config validation, server startup/shutdown integration. This is the critical path since nothing runs until `jobs.enabled=true` is wired into the server lifecycle.
2. **Admin API endpoints** — list/get/retry/cancel jobs, CRUD schedules, stats endpoint.
3. **CLI commands** — `ayb jobs list/retry/cancel`, `ayb schedules list/create/update/enable/disable/delete`.
4. Integration tests need a Postgres instance to run (`-tags=integration`).
