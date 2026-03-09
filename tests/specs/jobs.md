# Jobs & Scheduler Test Specification (Stage 3)

## Scope

Stage 3 validates AYB's Postgres-backed job queue and in-process scheduler:

- queue state transitions
- concurrent claim safety
- crash recovery
- retry/backoff behavior
- cron schedule enqueue behavior
- built-in cleanup handlers
- admin API and CLI management surface
- admin UI Jobs/Schedules views

## Test matrix

| Area | Required behavior | Automated coverage |
|---|---|---|
| State machine | `queued -> running -> completed` | `internal/jobs/store_test.go` (`TestEnqueueClaimCompleteFlow`) |
| State machine | retry path `queued -> running -> queued` with attempts | `internal/jobs/store_test.go` (`TestFailRetryAndTerminalFailure`) |
| State machine | terminal `failed` after `max_attempts` | `internal/jobs/store_test.go` (`TestFailRetryAndTerminalFailure`) |
| State machine | cancel queued jobs only | `internal/jobs/store_test.go` (`TestCancelQueuedJob`) |
| Concurrency | two concurrent claims for one queued job; exactly one succeeds | `internal/jobs/store_test.go` (`TestClaimSkipLockedConcurrent`) |
| Crash recovery | expired lease on running job is re-queued | `internal/jobs/store_test.go` (`TestRecoverStalledJobs`) |
| Backoff | deterministic exponential growth + cap + attempt clamping | `internal/jobs/backoff_test.go` (`TestComputeBackoffWithRand_*`) |
| Lease renewal | long-running jobs renew lease while executing | `internal/jobs/service_test.go` (`TestLeaseRenewalExtendsLease`, `TestLeaseRenewalStopsOnCompletion`) |
| Scheduler tick | due schedule enqueues one job and advances `next_run_at` | `internal/jobs/service_scheduler_test.go` (`TestSchedulerTickEnqueuesAndAdvances`) |
| Scheduler de-dup | concurrent ticks only enqueue once | `internal/jobs/service_scheduler_test.go` (`TestSchedulerTickDuplicatePrevention`) |
| Scheduler disable race | transactional enqueue must skip disabled schedules | `internal/jobs/store_sql_test.go` (`TestAdvanceScheduleAndEnqueueRequiresEnabledSchedule`), `internal/jobs/store_test.go` (`TestAdvanceScheduleAndEnqueueSkipsDisabledSchedule`) |
| Schedule enable behavior | disabled -> enabled recomputes `next_run_at` | `internal/server/jobs_handler_test.go` (`TestHandleAdminUpdateScheduleEnableRecomputesNextRunAt`) |
| Idempotency | duplicate `idempotency_key` rejected | `internal/jobs/store_test.go` (`TestIdempotencyKeyUnique`) |
| Built-in handlers | cleanup SQL deletes expired target rows | `internal/jobs/handlers_test.go` (`TestStaleSessionCleanupHandler`, `TestWebhookDeliveryPruneHandler`, `TestExpiredOAuthCleanupHandler`, `TestExpiredAuthCleanupHandler`) |
| Admin API | list/get/retry/cancel jobs + stats | `internal/server/jobs_handler_test.go` (`TestHandleAdminListJobs*`, `TestHandleAdminGetJob*`, `TestHandleAdminRetryJob*`, `TestHandleAdminCancelJob*`, `TestHandleAdminJobStats`) |
| Admin API | schedules CRUD + enable/disable | `internal/server/jobs_handler_test.go` (`TestHandleAdminListSchedules`, `TestHandleAdminCreateSchedule*`, `TestHandleAdminUpdateSchedule*`, `TestHandleAdminDeleteSchedule*`, `TestHandleAdminEnableSchedule`, `TestHandleAdminDisableSchedule`) |
| CLI | jobs/schedules command behavior and input validation | `internal/cli/jobs_cli_test.go` |
| Config/runtime | `jobs.scheduler_enabled` gating and jobs config typing | `internal/jobs/service_test.go` (`TestServiceStartSchedulerRespectsConfig`), `internal/config/config_test.go` (`TestSetValueJobsTypes`) |
| Runtime compatibility | legacy webhook pruner toggle when jobs on/off | `internal/server/webhook_pruner_toggle_test.go` |
| UI component | Jobs view loading/filter/retry/cancel | `ui/src/components/__tests__/Jobs.test.tsx` |
| UI component | Schedules view toggle/create/edit/delete/validation | `ui/src/components/__tests__/Schedules.test.tsx` |
| UI navigation | Jobs/Schedules available in layout + command palette | `ui/src/components/__tests__/Layout.test.tsx`, `ui/src/components/__tests__/CommandPalette.test.tsx` |

## Focused command set

Use targeted commands for Stage 3 verification (avoid whole-project runs):

```bash
go test ./internal/jobs -count=1
go test ./internal/server -run 'TestHandleAdmin(Job|ListJobs|GetJob|RetryJob|CancelJob|ListSchedules|CreateSchedule|UpdateSchedule|DeleteSchedule|EnableSchedule|DisableSchedule)|TestNew(StartsLegacyWebhookPrunerWhenJobsDisabled|SkipsLegacyWebhookPrunerWhenJobsEnabled)' -count=1
go test ./internal/cli -run 'TestJobs|TestSchedules' -count=1
go test ./internal/migrations -run 'TestJobs' -count=1
cd ui && npm test -- src/components/__tests__/Jobs.test.tsx src/components/__tests__/Schedules.test.tsx src/components/__tests__/Layout.test.tsx src/components/__tests__/CommandPalette.test.tsx
```

## Browser 3-tier note

Stage 3 currently has component-level coverage for Jobs/Schedules views. If adding browser tests for these pages later, follow `resources/BROWSER_TESTING_STANDARDS_2.md` and implement:

1. browser-mocked tests for stable UI interactions
2. browser-unmocked tests for real admin API integration
