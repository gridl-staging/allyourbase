# Handoff 080 (Review) - Stage 4 Data Layer Hardening

## What I reviewed

Focused review of recent Stage 4 backend/CLI/realtime/UI test work:
- `internal/matview/*` (service/store/job handler + tests)
- `internal/cli/matviews_cli.go` + tests
- `internal/server/matviews_handler.go` + tests
- Realtime Stage 4 integration/docs/spec alignment
- Stage checklist + input tracker consistency

## Issues found and fixed

1. **Scheduled refresh auto-register race (real bug)**
- Problem: `materialized_view_refresh` handler could fail when two workers race to auto-register the same matview. One worker would create the row; the other would get duplicate-registration error and fail the job.
- Fix: on duplicate-registration during auto-register, handler now retries `GetByName` and proceeds with refresh.
- Files:
  - `internal/matview/handler.go`
  - `internal/matview/handler_test.go` (new regression test: `TestMatviewRefreshHandlerDuplicateAutoRegisterFallsBackToLookup`)

2. **CLI spec/behavior mismatch (real bug + checklist deviation)**
- Problem: checklist requires `ayb matviews refresh <id|schema.view>`, but CLI only supported UUID-style ID.
- Fix: added qualified-name resolution (`schema.view` -> lookup via `GET /api/admin/matviews` -> resolved ID -> refresh).
- Also hardened path construction by escaping ID path segments for update/unregister/refresh routes.
- Files:
  - `internal/cli/matviews_cli.go`
  - `internal/cli/matviews_cli_test.go` (new tests: `TestMatviewsRefreshByQualifiedName`, `TestMatviewsRefreshByQualifiedNameNotFound`)

3. **Stage 4 checklist/docs/spec drift (process quality issue)**
- Problem: Stage checklist had stale unchecked items despite existing Materialized Views admin UI/component tests; realtime docs/spec lacked explicit Stage 4 joined-table/per-event semantics language.
- Fix: verified UI component coverage is present and passing; updated realtime docs/spec and checklist statuses accordingly.
- Files:
  - `.mike/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST-96c8e1/checklists/stage_04_checklist.md`
  - `_dev/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST.md`
  - `docs-site/guide/realtime.md`
  - `tests/specs/realtime.md`

## Tests run (focused)

Go:
- `go test ./internal/matview -run TestMatviewRefreshHandlerDuplicateAutoRegisterFallsBackToLookup -count=1`
- `go test ./internal/cli -run 'TestMatviewsRefreshByQualifiedName|TestMatviewsRefreshByQualifiedNameNotFound|TestMatviewsRefreshSuccess' -count=1`
- `go test ./internal/matview -count=1`
- `go test ./internal/cli -run 'TestMatviews' -count=1`
- `go test ./internal/server -run 'TestHandleAdmin(Matviews|RefreshMatview|RegisterMatview|UpdateMatview|DeleteMatview|GetMatview|ListMatviews)' -count=1`

UI:
- `cd ui && npm test -- src/components/__tests__/MatviewsAdmin.test.tsx` (11/11 passed)

Note:
- A broader `go test ./internal/cli ./internal/server` run is sandbox-blocked here due loopback bind restrictions in unrelated tests. Focused suites above are green.

## Checklist updates made

Updated Stage 4 checklist:
- Marked complete:
  - Admin dashboard matviews management view
  - UI component tests item
  - Realtime docs item
  - Realtime spec item
  - Delete-event semantics docs/per-event semantics items
  - Delete-event completion gate
- Added review hardening build note with concrete bug fixes and file references.

Updated input tracker (`_dev/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST.md`):
- Added Stage 4 review hardening note
- Marked Joined-table RLS docs/tracker items complete in section 5

## Modified files

- `internal/matview/handler.go`
- `internal/matview/handler_test.go`
- `internal/cli/matviews_cli.go`
- `internal/cli/matviews_cli_test.go`
- `docs-site/guide/realtime.md`
- `tests/specs/realtime.md`
- `.mike/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST-96c8e1/checklists/stage_04_checklist.md`
- `_dev/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST.md`

## Next steps

1. Finish remaining Stage 4 docs/spec deliverables:
   - `docs-site/guide/materialized-views.md`
   - `tests/specs/materialized-views.md`
   - `docs-site/guide/api-reference.md` matview endpoints
   - `docs-site/guide/admin-dashboard.md` matview section
2. Complete tracker updates listed in Stage 4 docs/spec section (`_dev/AYB_FEATURE_CHECKLIST_from_SIJLE.md`, `_dev/FEATURES.md`, `.mike/.../stages.md`).
3. Run integration-tag Stage 4 suites in an environment with `TEST_DATABASE_URL` available for final gate evidence.
