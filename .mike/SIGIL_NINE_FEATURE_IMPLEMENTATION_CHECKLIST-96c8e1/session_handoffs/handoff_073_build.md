# Handoff 073 — Stage 4 Matview CLI Commands (TDD)

## What I did

### 1) Read current state
- Reviewed handoff_070 and traced changes from sessions 71-72 (no handoff files for those).
- Sessions 71-72 had already completed:
  - `internal/matview/handler.go` + `handler_test.go` — job handler with auto-registration
  - `internal/matview/admin.go` — facade combining Store CRUD + Service refresh
  - `internal/matview/integration_test.go` — integration tests for Store CRUD, RefreshNow, concurrent mode, dropped matview
  - `internal/server/matviews_handler.go` + `matviews_handler_test.go` — admin API endpoints with full error mapping (14 tests)
  - `internal/server/server.go` — route registration for `/admin/matviews`
  - `internal/cli/start.go` — wiring matview admin service at startup
  - `internal/jobs/handlers.go` — registered `materialized_view_refresh` handler
  - `internal/jobs/handlers_test.go` — added `TestMatviewRefreshHandlerIntegration` end-to-end test

### 2) TDD: CLI commands (red → green)
- Wrote failing tests first: `internal/cli/matviews_cli_test.go` (16 tests)
  - Command registration
  - `matviews list` — table output, JSON output, empty state
  - `matviews register` — success, custom schema/mode, missing view, invalid mode
  - `matviews update` — success, not found, invalid mode
  - `matviews unregister` — success, not found
  - `matviews refresh` — success (shows duration), in-progress error, not found
- Verified red (all 16 tests fail with "unknown command matviews")
- Implemented: `internal/cli/matviews_cli.go`
  - `ayb matviews list` — table/JSON output with schema, view, mode, last refresh, status
  - `ayb matviews register --schema --view --mode` — defaults to public/standard
  - `ayb matviews update <id> --mode` — requires mode flag
  - `ayb matviews unregister <id>` — deletes registration
  - `ayb matviews refresh <id>` — triggers synchronous refresh, shows duration
- Verified green (all 16 tests pass)

### 3) Updated checklist
- Marked completed: job handler, job type registration, admin API endpoints, endpoint validation, CLI commands, integration tests (CRUD, RefreshNow, concurrent, job handler, server handlers, CLI)
- Updated build notes with sessions 71-73 work summary

### 4) Regression check
- All matview unit tests pass: `go test ./internal/matview`
- All server handler tests pass: `go test ./internal/server -run Matview`
- All migration tests pass: `go test ./internal/migrations -run TestMatviewMigrationSQLConstraints`
- All CLI tests pass: `go test ./internal/cli -run TestMatviews`
- Jobs/schedules CLI tests still green (no regressions)

## Focused tests run

All green:
- `GOCACHE=$(pwd)/.gocache go test ./internal/cli -run TestMatviews -count=1` (16 tests)
- `GOCACHE=$(pwd)/.gocache go test ./internal/matview -count=1`
- `GOCACHE=$(pwd)/.gocache go test ./internal/server -run Matview -count=1` (14 tests)
- `GOCACHE=$(pwd)/.gocache go test ./internal/migrations -run TestMatviewMigrationSQLConstraints -count=1`
- `GOCACHE=$(pwd)/.gocache go test ./internal/cli -run 'Jobs|Schedules' -count=1` (regression check)

## What's next

Remaining Stage 4 items:
1. **Admin dashboard UI** — Materialized Views management view (table listing, refresh button, register modal)
2. **Joined-table RLS SSE integration tests** — prove `canSeeRecord` evaluates join-based policies, membership-change visibility transitions
3. **RLS documentation** — delete-event pass-through semantics, per-event permission evaluation, code comment in `canSeeRecord`
4. **Docs** — `docs-site/guide/materialized-views.md`, update `api-reference.md`, `admin-dashboard.md`, `realtime.md`
5. **Test specs** — `tests/specs/materialized-views.md`, update `tests/specs/realtime.md`
6. **Tracker updates** — `_dev/AYB_FEATURE_CHECKLIST_from_SIJLE.md`, `_dev/FEATURES.md`, stages.md

## Files created or modified

Created:
- `internal/cli/matviews_cli.go`
- `internal/cli/matviews_cli_test.go`
- `.mike/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST-96c8e1/session_handoffs/handoff_073_build.md`

Modified:
- `.mike/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST-96c8e1/checklists/stage_04_checklist.md`
- `_dev/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST.md`
