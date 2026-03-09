# Handoff 002 — Review (Stage 1: Per-App API Key Scoping)

## What Was Done

Reviewed all code produced by Session 001 (build). Found and fixed 6 bugs — including 2 critical issues that broke the entire build and would have caused runtime crashes.

### Bugs Found & Fixed

1. **BUILD BROKEN: `Claims` struct missing `AppID` field** (Critical)
   - `internal/auth/apikeys.go:255` set `claims.AppID = *appID` but `Claims` in `auth.go` had no `AppID` field
   - `go build ./...` failed with compile error
   - **Fix:** Added `AppID string \`json:"appId,omitempty"\`` to Claims struct in `internal/auth/auth.go`

2. **RUNTIME CRASH: `scanAPIKeys` missing `app_id` column scan** (Critical)
   - Query selects 11 columns (id, user_id, name, key_prefix, scope, allowed_tables, **app_id**, last_used_at, expires_at, created_at, revoked_at) but `scanAPIKeys` only scanned 10 (skipped app_id)
   - Would cause pgx column mismatch error on every `ListAPIKeys` and `ListAllAPIKeys` call
   - **Fix:** Added `&k.AppID` to Scan call in `internal/auth/apikeys.go:270`

3. **MISSING FEATURE: Admin API key handler can't create app-scoped keys**
   - `adminCreateAPIKeyRequest` in `apikeys_handler.go` had no `AppID` field
   - No way to create app-scoped keys via the admin endpoint
   - **Fix:** Added `AppID *string \`json:"appId"\`` to request struct and wired to `CreateAPIKeyOptions`

4. **FALSE-POSITIVE TESTS in `apps_test.go`**
   - `TestAppStruct` tested Go struct assignment (not our code)
   - `TestAppListResultDefaults` tested Go struct zero values (not our code)
   - **Fix:** Replaced with meaningful tests: JSON serialization contract tests for `App`, `AppListResult`, `Claims.AppID`, `APIKey.AppID`, and `CreateAPIKeyOptions.AppID`

5. **SHALLOW TEST COVERAGE: `apps_test.go`**
   - Only tested empty-name validation, never tested any CRUD behavior
   - **Fix:** Tests now verify JSON field names match API contract (catches rename regressions), verify `omitempty` behavior for optional fields, and verify `nil` vs set `AppID` serialization

6. **`newTestService()` missing logger — nil pointer risk**
   - `Service` methods call `s.logger.Info()` but `newTestService()` didn't set logger
   - Any test that reached past validation into DB code would nil-pointer panic
   - **Fix:** Added `logger: testutil.DiscardLogger()` to `newTestService()` in `middleware_test.go`

### New Tests Added

- `TestAdminCreateAPIKeyWithAppID` — verifies app-scoped key creation via handler
- `TestAdminCreateAPIKeyWithoutAppID` — verifies legacy key creation (nil AppID)
- `TestClaimsAppIDField` — verifies Claims JSON serialization with/without AppID
- `TestAPIKeyAppIDField` — verifies APIKey JSON serialization with nil/set AppID
- `TestCreateAPIKeyOptionsAppID` — verifies options struct carries AppID
- `TestAppJSONSerialization` — verifies App struct JSON field names match API contract
- `TestAppListResultJSONSerialization` — verifies list result JSON field names

### Test Results

- `go build ./...` — passes (was failing before)
- `go test ./internal/auth/` — all pass (2.3s)
- `go test ./internal/server/` — all pass (0.5s)
- No regressions in existing tests

## Files Modified

- `internal/auth/auth.go` — Added `AppID` field to `Claims` struct
- `internal/auth/apikeys.go` — Fixed `scanAPIKeys` to scan `app_id` column
- `internal/auth/apps_test.go` — Rewrote: removed false-positive tests, added JSON contract tests
- `internal/auth/middleware_test.go` — Added logger to `newTestService()`
- `internal/server/apikeys_handler.go` — Added `AppID` to create request struct, wired to options
- `internal/server/apikeys_handler_test.go` — Updated fake to track AppID, added app-scoped key tests
- `.mike/.../checklists/stage_01_checklist.md` — Updated completed items

## What's Next

Stage 1 remaining work (in priority order):
1. **Migration tests** — Write integration test for `017_ayb_apps.sql` (apply, verify schema, rollback)
2. **Per-app rate limiting** — Implement request counting per app_id in middleware (the rate limit fields exist on the App struct but enforcement logic is not yet implemented)
3. **CLI commands** — `ayb apps create/list/delete`, extend `ayb apikeys create --app <id>`
4. **Admin Dashboard** — Apps management page, app selector in API key creation
5. **SDK & Docs** — TypeScript SDK type updates, API reference docs

### Known Gaps

- No integration tests exist for the apps CRUD operations (require real Postgres via `//go:build integration`)
- Per-app rate limiting is not yet implemented (middleware enforcement)
- The `ValidateAPIKey` function now loads `app_id` and sets `claims.AppID`, but no middleware reads `claims.AppID` to enforce app-level restrictions yet
- CLI and dashboard not started
