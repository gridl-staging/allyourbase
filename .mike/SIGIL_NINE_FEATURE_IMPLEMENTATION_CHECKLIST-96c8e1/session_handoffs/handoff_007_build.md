# Handoff 007 — Build (Stage 1: Per-App API Key Scoping)

## What I Built

Completed the next unchecked Stage 1 item:
- ✅ Wrote migration tests covering apply/schema/rollback behavior.

Implementation details:
- Added migration-runner filesystem injection support so tests can run deterministic failing migrations:
  - `internal/migrations/runner.go`
  - New `Runner.source fs.FS` field
  - New constructor: `NewRunnerWithFS(pool, logger, source)` (falls back to embedded migrations when `source == nil`)
  - `Run` now reads migrations via `fs.ReadDir` / `fs.ReadFile` from the configured source
- Added failing-first integration test for rollback semantics:
  - `internal/migrations/runner_test.go`
  - `TestRunMigrationsRollsBackFailedMigration`
  - Uses `fstest.MapFS` with a deliberately broken migration containing `_ayb_apps` + `_ayb_api_keys` DDL and an invalid SQL call
  - Verifies migration transaction rollback leaves no partial tables and no recorded migration row

Checklist and tracking updates:
- Marked Stage 1 DB migration test item complete:
  - `.mike/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST-96c8e1/checklists/stage_01_checklist.md`
- Updated original input tracker progress note:
  - `_dev/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST.md`

## TDD Evidence (Red → Green)

Red (before implementation):
- `GOCACHE=/tmp/ayb_gocache go test ./internal/migrations -tags=integration -run TestRunMigrationsRollsBackFailedMigration -count=1`
- Failure: `undefined: migrations.NewRunnerWithFS`

Green (after implementation, environment-limited):
- Integration test execution is blocked in this sandbox because `TEST_DATABASE_URL` is not set (package integration `TestMain` panics before running tests).
- Focused non-integration verification run:
  - `GOCACHE=/tmp/ayb_gocache go test ./internal/migrations -count=1`
  - Result: `ok`

## What’s Next

Next unchecked Stage 1 work candidates:
1. CLI commands:
   - `ayb apps create <name> --description`
   - `ayb apps list` (JSON output)
   - `ayb apps delete <id>`
   - `ayb apikeys create --app <id>`
2. CLI test coverage for all new commands.
3. Completion gate test: negative case where app-scoped key is denied out-of-scope table/operation.

## Files Created/Modified

Created:
- `.mike/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST-96c8e1/session_handoffs/handoff_007_build.md`

Modified:
- `internal/migrations/runner.go`
- `internal/migrations/runner_test.go`
- `.mike/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST-96c8e1/checklists/stage_01_checklist.md`
- `_dev/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST.md`
- `.mike/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST-96c8e1/analytics/events_v1.jsonl`
- `.mike/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST-96c8e1/state.json`
