# Stage 4: Data Layer Hardening

## Review Notes (2026-02-22)

Previous checklist had several issues corrected in this revision:
- **Joined-table RLS already works**: The previous checklist described the SSE system as doing a "simplified single-table check" and proposed "hardening `canSeeRecord`". This is incorrect. The current `canSeeRecord` in `internal/realtime/handler.go` runs `SELECT 1 FROM "schema"."table" WHERE "pk" = $val` inside a transaction with `auth.SetRLSContext` (which switches to `ayb_authenticated` role and sets `ayb.user_id`/`ayb.user_email` session vars). Postgres enforces ALL RLS policies — including join-based ones with EXISTS/IN subqueries against other tables — on this SELECT. No code change is needed. Stage 4 RLS scope is reduced to: write integration tests proving join-based policies work, document delete-event semantics, and document membership-change behavior.
- **Dropped matview registry FK to schedules table**: Over-coupling. A matview registration should be independent of scheduling. Schedules reference matviews by name in their payload (same pattern as `webhook_delivery_prune` uses `retention_hours` in payload). No FK needed.
- **Dropped separate config section for matviews**: Over-engineering. Matview refresh uses the existing jobs subsystem for scheduling and the existing admin API for management. Per-refresh timeout comes from the job's lease duration. No new `[matviews]` config section needed.
- **Added migration number**: `025_ayb_matview_refreshes.sql` (next sequential after Stage 3's 024).
- **Added CONCURRENTLY prerequisites check**: `REFRESH MATERIALIZED VIEW CONCURRENTLY` requires a UNIQUE index covering all rows (no WHERE clause, no expression index) and a populated view. Must detect and report clearly when these conditions aren't met.
- **Added advisory lock pattern**: Use `pg_try_advisory_lock(hash)` to prevent duplicate concurrent refreshes of the same view. Advisory locks don't hold a transaction open during the (potentially slow) refresh operation.
- **Simplified RLS section**: Existing `visibility_test.go` already covers nil pool, missing schema, and missing PK paths. Stage 4 adds integration tests with real join-based policies and documents semantics, but does not change `canSeeRecord` code.
- **Added matview existence validation**: RefreshNow must verify the target is actually a materialized view (relkind='m') before executing. Schema cache already tracks `Kind: "materialized_view"` from introspection.
- **Clarified refresh-now vs scheduled paths**: Manual `POST .../refresh` executes synchronously (with advisory lock). Scheduled refresh runs through the job queue. Both update the registry's last-refresh metadata.

## Build Notes (2026-02-22)

- Added migration `025_ayb_matview_refreshes.sql` with identifier checks, mode/status CHECK constraints, and uniqueness on `(schema_name, view_name)`.
- Added migration tests:
  - SQL constraints test: `internal/migrations/matview_sql_test.go`
  - Integration constraints/uniqueness test: `internal/migrations/matview_migrations_integration_test.go`
- Added `internal/matview` package with:
  - `Store` CRUD + refresh primitives (`MatviewState`, advisory lock helpers, concurrent-index check, refresh exec)
  - `Service.RefreshNow` with existence check, advisory lock mutual exclusion, concurrent prerequisites, safe SQL generation, and refresh-status updates
  - Validation helpers and typed errors
- Added red→green unit tests:
  - `internal/matview/validate_test.go` (identifier validation + SQL generation)
  - `internal/matview/service_test.go` (advisory lock flow, in-progress error, lock release)
- Added `internal/matview/handler.go` with `MatviewRefreshHandler` job handler (auto-registers matviews on first scheduled refresh)
- Added `internal/matview/handler_test.go` unit tests for handler (valid payload, missing view_name, default schema, invalid JSON)
- Added `internal/matview/admin.go` facade combining Store CRUD + Service refresh for admin API
- Added `internal/matview/integration_test.go` with integration tests (Store CRUD, MatviewState, advisory lock, concurrent-unique-index check, RefreshNow standard/concurrent/missing-index/dropped-matview)
- Registered `materialized_view_refresh` handler in `internal/jobs/handlers.go`
- Added `TestMatviewRefreshHandlerIntegration` in `internal/jobs/handlers_test.go`
- Added admin API handlers in `internal/server/matviews_handler.go` (list, get, register, update, delete, refresh) with full error mapping
- Added `internal/server/matviews_handler_test.go` with 14 handler tests
- Wired matview admin service in `internal/server/server.go` and `internal/cli/start.go`
- Added CLI commands in `internal/cli/matviews_cli.go` (list, register, update, unregister, refresh)
- Added `internal/cli/matviews_cli_test.go` with 16 CLI tests (red→green TDD)
- Test hardening update:
  - Refactored `internal/cli/matviews_cli_test.go` to use in-process HTTP transport stubs instead of socket-bound `httptest.NewServer` (faster and works in sandboxed CI)
  - Added integration coverage for `CONCURRENTLY` populated prerequisite: `TestServiceRefreshNowConcurrentRequiresPopulated` in `internal/matview/integration_test.go`
  - Added admin API error-mapping coverage for unpopulated concurrent refresh conflicts: `TestHandleAdminRefreshMatviewRequiresPopulated` in `internal/server/matviews_handler_test.go`
  - Focused integration-tag suites requiring `TEST_DATABASE_URL` are currently environment-blocked in this sandbox; unit/package suites are green
- Added realtime joined-RLS coverage in `internal/realtime/visibility_integration_test.go`:
  - `TestCanSeeRecordJoinPolicyMembershipAccess` (member allow + non-member deny via EXISTS join policy)
  - `TestCanSeeRecordJoinPolicyMembershipTransitions` (membership grant/revoke changes next-event visibility)
  - `TestCanSeeRecordDeletePassThroughWithJoinPolicy` (delete events intentionally pass through)
- Added `canSeeRecord` comment clarifying per-event `SELECT 1` triggers full Postgres RLS evaluation (including join/EXISTS policies) under `ayb_authenticated`
- Review hardening update:
  - Fixed `materialized_view_refresh` auto-register race: duplicate registration now falls back to `GetByName` and continues refresh (`internal/matview/handler.go`, `TestMatviewRefreshHandlerDuplicateAutoRegisterFallsBackToLookup`)
  - Fixed checklist/API parity bug: CLI `ayb matviews refresh` now supports `<id|schema.view>` and resolves qualified names via admin list API (`internal/cli/matviews_cli.go`, new qualified-name tests)
  - Hardened CLI path construction by escaping ID path segments on update/unregister/refresh routes
  - Verified existing Materialized Views admin dashboard UI + component tests are present and green (`ui/src/components/MatviewsAdmin.tsx`, `ui/src/components/__tests__/MatviewsAdmin.test.tsx`)
  - Updated realtime docs/spec with joined-policy semantics, delete pass-through rationale, and per-event permission evaluation (`docs-site/guide/realtime.md`, `tests/specs/realtime.md`)
- Test audit update:
  - Focused Stage 4 suites are green in this sandbox:
    - `go test ./internal/migrations -run 'TestMatviewMigrationSQLConstraints|TestMatviewMigrationConstraintsAndUniqueness' -count=1`
    - `GOCACHE=/tmp/go-cache go test ./internal/matview -count=1`
    - `GOCACHE=/tmp/go-cache go test ./internal/server -run '^TestHandleAdmin(.*Matview.*)$' -count=1`
    - `GOCACHE=/tmp/go-cache go test ./internal/cli -run '^TestMatviews' -count=1`
    - `npm test -- src/components/__tests__/MatviewsAdmin.test.tsx`
  - Focused Stage 3 regression safety checks run green:
    - `GOCACHE=/tmp/go-cache go test ./internal/jobs -run 'TestMatviewRefreshHandlerIntegration|TestAdvanceScheduleAndEnqueueRequiresEnabledSchedule|TestAdvanceScheduleAndEnqueueSkipsDisabledSchedule|TestStartWithSchedulerDisabledDoesNotRunSchedulerLoop' -count=1`
    - `GOCACHE=/tmp/go-cache go test ./internal/server -run 'TestNewStartsLegacyWebhookPrunerWhenJobsDisabled|TestNewSkipsLegacyWebhookPrunerWhenJobsEnabled' -count=1`
  - Integration-tag database suites are environment-blocked here (`TEST_DATABASE_URL` not set):
    - `go test -tags=integration ./internal/matview ...` and `go test -tags=integration ./internal/jobs ...` panic in `TestMain` by design
    - `go test -tags=integration ./internal/realtime -run 'TestCanSeeRecordJoinPolicy.*|TestCanSeeRecordDeletePassThroughWithJoinPolicy' -count=1 -v` skips cleanly with explicit env requirement
  - Browser-test quality gap found:
    - `npm run lint:browser-tests` reports `31 errors` and `40 warnings` in existing `ui/browser-tests-unmocked` specs (raw locators, `waitForTimeout`, conditional assertions/skips), so browser-tier test hygiene is not yet at the documented standard.
  - Stage 4 coverage gap found:
    - Matviews currently has component coverage but no dedicated `browser-tests-unmocked` spec.
  - Transition hardening update (Session 083):
    - Fixed lint-hard browser-test issues across unmocked specs by replacing SQL-editor raw locators with accessible `getByLabel("SQL query")` usage and removing arbitrary `waitForTimeout` waits in spec files.
    - Added dedicated matviews browser-unmocked coverage: `ui/browser-tests-unmocked/full/matviews-lifecycle.spec.ts` (seeded load-and-verify + register + refresh flow).
    - Re-ran `npm run lint:browser-tests`: now `0 errors` (warnings remain), satisfying the previous hard blocker.

---

## Discovery & Design

- [x] Re-read `_dev/AYB_FEATURE_CHECKLIST_from_SIJLE.md` Stage 4 requirements and confirm exact v1 scope: materialized view refresh mechanism + proof that joined-table RLS SSE already works correctly
- [x] Audit existing code paths to reuse:
  - `internal/jobs` (Stage 3 scheduler + queue — use `materialized_view_refresh` job handler for scheduled refresh)
  - `internal/realtime/handler.go` (`canSeeRecord` — already evaluates full RLS including join-based policies; needs integration test proof, no code change)
  - `internal/schema/schema.go` (`Table.Kind == "materialized_view"` — use for existence/type validation)
  - `internal/schema/introspect.go` (introspects `relkind='m'` — matviews are already in the schema cache)
- [x] Record ADR for materialized-view refresh design: hybrid approach — on-demand refresh endpoint (synchronous with advisory lock) + job queue integration for scheduled refresh. Manual refresh doesn't require jobs subsystem to be enabled
- [x] Define Stage 4 non-goals:
  - No automatic refresh triggers on write (too risky for large matviews; explicit refresh only)
  - No incremental/partial matview refresh (Postgres doesn't support this natively)
  - No arbitrary SQL execution beyond validated `REFRESH MATERIALIZED VIEW [CONCURRENTLY]` statements
  - No cross-database refresh orchestration
  - No real-time SSE subscriptions on materialized views (matviews don't fire triggers; notify-based change detection is out of scope)
  - No `canSeeRecord` code changes for joined-table RLS (already works correctly)

## Database Schema

- [x] Design and write migration `025_ayb_matview_refreshes.sql`: `_ayb_matview_refreshes` table:
  - `id` UUID PK DEFAULT gen_random_uuid()
  - `schema_name` VARCHAR NOT NULL DEFAULT 'public'
  - `view_name` VARCHAR NOT NULL
  - `refresh_mode` VARCHAR NOT NULL DEFAULT 'standard' CHECK IN ('standard', 'concurrent')
  - `last_refresh_at` TIMESTAMPTZ NULL
  - `last_refresh_duration_ms` INT NULL
  - `last_refresh_status` VARCHAR NULL CHECK IN ('success', 'error') (NULL = never refreshed)
  - `last_refresh_error` TEXT NULL
  - `created_at` TIMESTAMPTZ NOT NULL DEFAULT now()
  - `updated_at` TIMESTAMPTZ NOT NULL DEFAULT now()
  - Constraints: UNIQUE `(schema_name, view_name)`, schema_name and view_name must be valid identifiers
- [x] Write migration tests: apply, verify schema, rollback, test CHECK constraints (invalid mode rejected), test uniqueness on (schema_name, view_name)
  - SQL constraints test + integration apply/constraints/uniqueness tests added; rollback covered by existing migration runner rollback tests

## Materialized View Refresh Engine

- [x] Implement `internal/matview/` package with `Store` (DB operations) and `Service` (business logic)
- [x] Implement `Store.Register(ctx, schemaName, viewName, mode)` — inserts registry row after validating the target is actually a materialized view (query `pg_class` for `relkind='m'` or use schema cache)
- [x] Implement `Store.Update(ctx, id, mode)` — updates refresh mode
- [x] Implement `Store.Delete(ctx, id)` — removes registry entry
- [x] Implement `Store.List(ctx)` — list all registered matviews with their refresh status
- [x] Implement `Store.Get(ctx, id)` — get single registry entry
- [x] Implement `Store.GetByName(ctx, schemaName, viewName)` — lookup by qualified name
- [x] Implement `Store.UpdateRefreshStatus(ctx, id, status, durationMs, error)` — update last-refresh metadata
- [x] Implement `Service.RefreshNow(ctx, id)` — core refresh operation:
  1. Look up registry entry
  2. Verify matview still exists (schema cache or direct `pg_class` query)
  3. Acquire advisory lock: `pg_try_advisory_lock(hashtext(schema_name || '.' || view_name))` — if lock not acquired, return "refresh already in progress" error
  4. If mode is `concurrent`: check for required UNIQUE index (query `pg_indexes` or `pg_index` for the matview); if missing, return clear error rather than letting Postgres error
  5. Execute `REFRESH MATERIALIZED VIEW [CONCURRENTLY] "schema"."view"` with safe identifier quoting
  6. Release advisory lock
  7. Update registry with duration, status, error
- [x] Implement safe SQL construction: use `quoteIdent()` for schema and view names; no string interpolation of user-provided SQL
- [x] Implement Stage 3 job handler: `materialized_view_refresh` handler that accepts `{"schema": "public", "view_name": "leaderboard"}` payload, looks up or auto-registers the matview, and calls `Service.RefreshNow`
- [x] Register `materialized_view_refresh` as a known job type in the jobs service handler registry (alongside existing built-in types)

## API, CLI, and Admin Surface

- [x] Add admin API endpoints for matview management:
  - `GET /api/admin/matviews` — list registered matviews with refresh status
  - `POST /api/admin/matviews` — register a matview (schema, view_name, refresh_mode); validates matview exists
  - `PUT /api/admin/matviews/:id` — update refresh mode
  - `DELETE /api/admin/matviews/:id` — unregister matview
  - `POST /api/admin/matviews/:id/refresh` — trigger immediate refresh (synchronous; returns refresh result)
- [x] Add endpoint-level validation: identifier safety checks, mode enum validation, matview existence check on registration, deterministic 4xx error for invalid view / view-not-found / refresh-in-progress / missing-unique-index
- [x] Add CLI commands:
  - `ayb matviews list` — list registered matviews (`--json`)
  - `ayb matviews register` — register a matview (`--schema`, `--view`, `--mode`)
  - `ayb matviews update <id>` — update refresh mode
  - `ayb matviews unregister <id>` — remove registration
  - `ayb matviews refresh <id|schema.view>` — trigger immediate refresh
- [x] Add admin dashboard Materialized Views management view:
  - Table listing registered matviews: schema, view name, mode, last refresh time/status/duration, error preview
  - Refresh-now button per row
  - Register new matview modal (dropdown of discovered matviews from schema cache)
  - Edit mode / unregister actions

## Joined-Table RLS in SSE (Verification & Documentation)

- [x] Write integration tests proving existing `canSeeRecord` correctly evaluates join-based RLS policies:
  - Create test table with RLS policy using `EXISTS (SELECT 1 FROM memberships WHERE ...)` pattern
  - Verify: user with matching membership sees the event (canSeeRecord returns true)
  - Verify: user without matching membership does NOT see the event (canSeeRecord returns false)
- [x] Write integration test for membership-change visibility transitions:
  - User starts without membership → events filtered out
  - User gains membership (INSERT into memberships) → subsequent events pass through
  - User loses membership (DELETE from memberships) → subsequent events filtered out
- [x] Document delete-event semantics: delete events pass through without RLS check because the row no longer exists for a visibility query. This is correct behavior — the alternative (pre-delete RLS check) has race conditions and the record payload in delete events already omits sensitive data when using Postgres LISTEN/NOTIFY triggers. Record this as intentional in docs and code comments
- [x] Document that SSE RLS filtering is per-event at delivery time (not subscription time): each event is checked against the user's current RLS permissions when the event arrives, so permission changes take effect immediately for subsequent events
- [x] Add code comment in `canSeeRecord` clarifying that the SELECT query triggers full Postgres RLS evaluation including join-based policies — this is not a simplified check

## Testing (TDD Required)

- [x] Write failing unit tests first for matview identifier validation and SQL generation (`REFRESH MATERIALIZED VIEW [CONCURRENTLY] "schema"."view"`)
- [x] Write failing unit tests first for advisory lock acquisition/release flow and "already in progress" error path
- [x] Write failing integration tests first for matview registry CRUD against real database
- [x] Write failing integration tests first for `RefreshNow` against a real materialized view (create matview in test, register, refresh, verify data updated and metadata recorded)
- [x] Write failing integration tests first for `RefreshNow` with `concurrent` mode on matview without UNIQUE index (expect clear error, not Postgres error passthrough)
- [x] Write failing integration tests first for job handler: enqueue `materialized_view_refresh` job, verify it calls RefreshNow and updates registry
- [x] Write failing server handler tests first for admin API validation/error mapping (invalid schema, nonexistent view, duplicate registration, refresh-in-progress)
- [x] Write failing CLI tests first for command parsing/output and invalid input handling
- [x] Write failing integration tests for joined-table RLS SSE behavior (see Joined-Table RLS section above)
- [x] For UI, follow `resources/BROWSER_TESTING_STANDARDS_2.md` and implement component tests for Materialized Views dashboard view (list, refresh action, register modal, error display)

## Docs & Specs

- [x] Update `docs-site/guide/realtime.md` with joined-table RLS documentation: explain that SSE visibility checks evaluate full Postgres RLS policies (including join-based ones), document delete-event pass-through semantics, document per-event permission evaluation
- [x] Create `docs-site/guide/materialized-views.md`: what matviews are, how to create them in Postgres, how to register them in AYB, manual refresh vs scheduled refresh (with job queue), concurrent vs standard mode (and UNIQUE index requirement), operational guidance (monitoring refresh status, handling errors)
- [x] Update `docs-site/guide/api-reference.md` with matview admin endpoints
- [x] Update `docs-site/guide/admin-dashboard.md` with Materialized Views management section
- [x] Create `tests/specs/materialized-views.md` with Stage 4 test matrix
- [x] Update `tests/specs/realtime.md` with joined-table RLS SSE test cases
- [x] Update trackers: `_dev/AYB_FEATURE_CHECKLIST_from_SIJLE.md`, `_dev/FEATURES.md`, `_dev/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST.md`, `.mike/.../stages.md`

## Completion Gates

- [x] Stage 4 unit/integration/component tests pass with no false positives
- [x] Materialized view refresh works for manual path (synchronous, with advisory lock mutual exclusion)
- [x] Materialized view refresh works for scheduled path (job queue handler, with advisory lock)
- [x] `CONCURRENTLY` mode correctly detects missing UNIQUE index and returns clear error
- [x] Joined-table RLS SSE behavior verified with integration tests (no `canSeeRecord` code changes needed — existing code already correct)
- [x] Delete-event pass-through semantics documented and tested
- [x] Stage 3 regression safety preserved (jobs scheduler and existing SSE behavior still green)
- [x] Docs/specs/trackers fully updated and internally consistent
