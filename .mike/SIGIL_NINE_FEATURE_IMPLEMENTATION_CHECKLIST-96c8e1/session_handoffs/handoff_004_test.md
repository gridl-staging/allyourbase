# Handoff 004 — Test (Stage 1: Per-App API Key Scoping)

## What Was Done

Reviewed all existing tests for Stage 1 app scoping feature. All tests pass. Found no bugs. Identified and filled coverage gaps. Removed redundant tests.

### Tests Added

- `TestAdminListAppsWithPagination` — verifies pagination returns correct page sizes and page 2 remainder
- `TestAdminListAppsPaginationDefaults` — verifies page=0/perPage=0 default to 1/20
- `TestAdminListAppsPaginationClamp` — verifies perPage>100 is clamped to 100
- `TestAdminListAppsBeyondLastPage` — verifies requesting page 999 returns 0 items but correct totalItems
- `TestAdminListAppsNonNumericParams` — verifies non-numeric page/perPage silently default
- `TestAdminUpdateAppInvalidJSON` — verifies malformed JSON body on update returns 400

### Tests Removed (Redundant)

- `TestAdminCreateAPIKeyResponseFormat` — 100% subset of `TestAdminCreateAPIKeySuccess` (same request, fewer assertions)
- `TestAPIKeyConstants` — checks `len(plaintext)==52` and `len(APIKeyPrefix)==4`, both already checked in `TestAPIKeyFormat`
- `TestIsAPIKeyLengthBoundary` — checks `ayb_x` valid and `ayb_` invalid, exact duplicates of `TestIsAPIKey` table entries

### Test Results

- `go test ./internal/auth/` — all pass (2.3s)
- `go test ./internal/server/` — all pass (0.3s)
- No regressions

### Speed Analysis

- Auth tests: 2.3s total, dominated by `TestValidateTokenBoundaryConditions/after_expiry_-_expired` (2.0s, uses time.Sleep for JWT expiry). Pre-existing, unrelated to app scoping.
- Server tests: 0.3s total. All handler tests are <1ms. Fast.
- No slow or wasteful setups found.

## Files Modified

- `internal/server/apps_handler_test.go` — Added 6 pagination/validation tests
- `internal/server/apikeys_handler_test.go` — Removed redundant `TestAdminCreateAPIKeyResponseFormat`
- `internal/auth/apikeys_test.go` — Removed redundant `TestAPIKeyConstants` and `TestIsAPIKeyLengthBoundary`

## Coverage Assessment

### Well Covered (Handler Layer)
- Apps CRUD: list (8 tests), get (4 tests), create (5 tests), update (7 tests), delete (4 tests)
- API keys: list (8 tests), revoke (5 tests), create (12 tests including app scoping)

### Well Covered (Service Layer)
- Validation: name required, negative rate limits, error sentinels
- Scope enforcement: read/write/table checks, fail-closed on unknown scope
- JSON serialization: App, AppListResult, Claims, APIKey round-trips

### Not Yet Testable (Require Real DB)
- `CreateApp`, `GetApp`, `ListApps`, `UpdateApp`, `DeleteApp` actual DB operations
- `CreateAPIKey` with app_id FK constraint enforcement
- `ValidateAPIKey` loading app_id from DB and setting claims.AppID
- `DeleteApp` transactional key revocation (race condition fix from handoff_003)
- Migration 017_ayb_apps.sql apply/verify/rollback

## What's Next

Stage 1 remaining work (in priority order):
1. **Per-app rate limiting** — Implement request counting per app_id in middleware (rate limit fields exist on App struct but enforcement logic is not implemented)
2. **CLI commands** — `ayb apps create/list/delete`, extend `ayb apikeys create --app <id>`
3. **Admin Dashboard** — Apps management page, app selector in API key creation
4. **SDK & Docs** — TypeScript SDK type updates, API reference docs
5. **Migration tests** — Integration tests for 017_ayb_apps.sql (require real Postgres)
6. **Negative scope integration tests** — App-scoped key denied access to out-of-scope tables end-to-end
