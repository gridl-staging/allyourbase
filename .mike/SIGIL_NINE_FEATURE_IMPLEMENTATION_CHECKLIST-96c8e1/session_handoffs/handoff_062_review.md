# Handoff 062 — Stage 3 Review Hardening

## Scope
Reviewed recent Stage 3 jobs/scheduler/admin/config/CLI work for correctness gaps, false-positive risk, and checklist drift. Fixed concrete issues found and added focused regression tests.

## Bugs found and fixed

1. `jobs.scheduler_enabled` config was ignored at runtime
- Root cause: `jobs.Service.Start` always launched scheduler loop; startup wiring did not pass scheduler-enabled flag.
- Fix:
  - Added `SchedulerEnabled bool` to `internal/jobs.ServiceConfig` (default `true`).
  - `Service.Start` now starts scheduler loop only when enabled.
  - `internal/cli/start.go` now wires `cfg.Jobs.SchedulerEnabled` into `jobs.ServiceConfig`.
- Regression tests:
  - `internal/jobs/service_scheduler_test.go` (`TestStartWithSchedulerDisabledDoesNotRunSchedulerLoop`)
  - `internal/jobs/service_test.go` (`TestSchedulerDisabledDoesNotEnqueue`, integration-tagged)

2. Config `SetValue` did not type-coerce `jobs.*` keys
- Root cause: `coerceValue` omitted all `jobs.*` bool/int keys, so TOML wrote quoted strings and later `Load` failed decoding.
- Fix:
  - Added bool coercion for `jobs.enabled`, `jobs.scheduler_enabled`.
  - Added int coercion for `jobs.worker_concurrency`, `jobs.poll_interval_ms`, `jobs.lease_duration_s`, `jobs.max_retries_default`, `jobs.scheduler_tick_s`.
- Regression tests:
  - `internal/config/config_test.go` (`TestSetValueJobsTypes`)
  - Extended `TestCoerceValue` with `jobs.*` cases.

3. Schedule enable via `PUT /api/admin/schedules/:id` could leave `next_run_at` nil
- Root cause: handler recomputed `next_run_at` only when cron/timezone changed, not when enabling a disabled schedule.
- Fix:
  - `internal/server/jobs_handler.go` now recomputes `next_run_at` on disabled→enabled transition.
- Regression test:
  - `internal/server/jobs_handler_test.go` (`TestHandleAdminUpdateScheduleEnableRecomputesNextRunAt`)

4. Missing CLI boundary validation for schedule payload/update flags
- Root cause: schedule create/update ignored JSON marshal errors and accepted invalid payload/`--enabled` input.
- Fix:
  - `internal/cli/jobs_cli.go` now validates payload JSON with `json.Valid`.
  - Added strict bool parse for `--enabled` using `strconv.ParseBool`.
  - Added marshal error handling (no silent ignored errors).
- Regression tests:
  - `internal/cli/jobs_cli_test.go` (`TestSchedulesCreateInvalidPayloadJSON`, `TestSchedulesUpdateInvalidPayloadJSON`, `TestSchedulesUpdateInvalidEnabledValue`)

## Checklist updates
Updated Stage 3 checklist status in:
- `.mike/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST-96c8e1/checklists/stage_03_checklist.md`

Marked complete (implemented and verified in code/tests):
- Admin jobs/schedules API endpoints
- Queue stats endpoint
- Jobs/schedules CLI commands
- Jobs env var wiring
- Jobs config key get/set coverage
- Jobs config bounds validation
- Startup/runtime and shutdown wiring
- Admin API handler tests
- CLI tests

Also added review notes documenting the fixes.

Updated master tracker note in:
- `_dev/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST.md`

## Focused tests run
- `GOCACHE=/tmp/ayb-gocache go test ./internal/jobs -run 'Test(StartWithSchedulerDisabledDoesNotRunSchedulerLoop|ComputeBackoffWithRandDeterministic)$' -count=1`
- `GOCACHE=/tmp/ayb-gocache go test ./internal/config -run 'Test(SetValueJobsTypes|CoerceValue|SetValueBoolean|SetValue)$' -count=1`
- `GOCACHE=/tmp/ayb-gocache go test ./internal/server -run 'TestHandleAdminUpdateSchedule(EnableRecomputesNextRunAt|$)' -count=1`
- `GOCACHE=/tmp/ayb-gocache go test ./internal/cli -run 'TestSchedules(CreateInvalidPayloadJSON|UpdateInvalidPayloadJSON|UpdateInvalidEnabledValue)$' -count=1`

Note: integration-tagged jobs tests requiring `TEST_DATABASE_URL` were not run in this environment.

## Files created or modified
Created:
- `internal/jobs/service_scheduler_test.go`
- `.mike/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST-96c8e1/session_handoffs/handoff_062_review.md`

Modified:
- `internal/jobs/service.go`
- `internal/jobs/service_test.go`
- `internal/cli/start.go`
- `internal/config/config.go`
- `internal/config/config_test.go`
- `internal/server/jobs_handler.go`
- `internal/server/jobs_handler_test.go`
- `internal/cli/jobs_cli.go`
- `internal/cli/jobs_cli_test.go`
- `.mike/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST-96c8e1/checklists/stage_03_checklist.md`
- `_dev/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST.md`

## Next recommended work
- Stage 3 remaining major gap is admin dashboard Jobs/Schedules UI + component/browser tests and docs updates (`docs-site/guide/*`, `tests/specs/jobs.md`).
