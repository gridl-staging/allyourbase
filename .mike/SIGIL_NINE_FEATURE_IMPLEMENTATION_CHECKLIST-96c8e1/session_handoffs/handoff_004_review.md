# Handoff 004 — Review (Stage 1: Per-App API Key Scoping)

## What Was Done

Reviewed all code and tests from previous sessions (000 init, 002 review). Found and fixed 4 bugs — including a race condition that could lead to privilege escalation.

### Bugs Found & Fixed

1. **RACE CONDITION: `DeleteApp` not transactional — potential privilege escalation** (Critical)
   - `DeleteApp` performed two operations (revoke keys, delete app) without a transaction
   - Between the revoke and delete, a new key could be created for the app
   - The `ON DELETE SET NULL` FK would then silently convert that new key from app-scoped to unrestricted user-scoped
   - **Fix:** Wrapped both operations in `pool.Begin(ctx)` / `tx.Commit(ctx)` with `defer tx.Rollback(ctx)`
   - File: `internal/auth/apps.go:178-209`

2. **MISSING SERVICE-LAYER VALIDATION: `UpdateApp` accepts negative rate limits** (Medium)
   - `ErrAppInvalidRateLimit` sentinel existed but was never returned by any code
   - Only the HTTP handler validated rate limits — direct callers (CLI, internal code) could set negative values
   - **Fix:** Added `rateLimitRPS < 0 || rateLimitWindowSeconds < 0` check in `UpdateApp` before the DB query
   - File: `internal/auth/apps.go:154-156`

3. **500 INSTEAD OF 400: Non-UUID `appId` in `CreateAPIKey` causes unhandled Postgres error** (Medium)
   - If `appId` is `"not-a-uuid"`, PostgreSQL returns error code `22P02` (invalid text representation)
   - Code only caught `23503` (FK violation), so non-UUID appId fell through to generic 500
   - **Fix:** Added `case "22P02"` to the pgErr switch in `CreateAPIKey`
   - File: `internal/auth/apikeys.go:108-116`

4. **MISSING INPUT VALIDATION: Admin handler passes unvalidated `appId` to service** (Medium)
   - `handleAdminCreateAPIKey` passed `req.AppID` directly to the service without UUID format check
   - Combined with bug #3, this meant admin API returned 500 for malformed appId
   - **Fix:** Added `httputil.IsValidUUID(*req.AppID)` check before calling service
   - File: `internal/server/apikeys_handler.go:92-95`

### Tests Added

- `TestUpdateAppNegativeRateLimitRPS` — verifies service rejects negative RPS (bug #2)
- `TestUpdateAppNegativeRateLimitWindow` — verifies service rejects negative window (bug #2)
- `TestAdminCreateAPIKeyNonUUIDAppID` — verifies handler rejects non-UUID appId with 400 (bug #4)

### Tests Modified

- `TestAdminCreateAPIKeyInvalidAppID` — updated test appId from `nonexistent00` (not valid hex) to `aaaaaaaaa099` (valid UUID format that doesn't exist) so it tests the service-layer FK validation rather than being intercepted by the new handler-level UUID check

### Test Results

- `go build ./...` — passes
- `go test ./internal/auth/` — all pass (2.3s)
- `go test ./internal/server/` — all pass (0.3s)
- No regressions in existing tests

## Files Modified

- `internal/auth/apps.go` — Wrapped `DeleteApp` in transaction; added rate limit validation to `UpdateApp`
- `internal/auth/apikeys.go` — Added `22P02` error code handling for non-UUID appId
- `internal/auth/apps_test.go` — Added `TestUpdateAppNegativeRateLimitRPS`, `TestUpdateAppNegativeRateLimitWindow`
- `internal/server/apikeys_handler.go` — Added `appId` UUID format validation in `handleAdminCreateAPIKey`
- `internal/server/apikeys_handler_test.go` — Added `TestAdminCreateAPIKeyNonUUIDAppID`; fixed `TestAdminCreateAPIKeyInvalidAppID` appId to use valid UUID format

## What's Next

Stage 1 remaining work (in priority order):
1. **Migration tests** — Write integration test for `017_ayb_apps.sql` (apply, verify schema, rollback)
2. **Per-app rate limiting** — Implement request counting per app_id in middleware (rate limit fields exist on App struct but enforcement logic is not yet implemented)
3. **CLI commands** — `ayb apps create/list/delete`, extend `ayb apikeys create --app <id>`
4. **Admin Dashboard** — Apps management page, app selector in API key creation
5. **SDK & Docs** — TypeScript SDK type updates, API reference docs
6. **Negative scope tests** — Add integration tests for app-scoped key denied access to out-of-scope tables (unit-level scope checks are tested, but end-to-end not yet)

### Known Gaps (Unchanged from Previous Reviews)

- No integration tests exist for the apps CRUD operations (require real Postgres via `//go:build integration`)
- Per-app rate limiting is not yet implemented (middleware enforcement)
- The `ValidateAPIKey` function sets `claims.AppID`, but no middleware reads `claims.AppID` to enforce app-level restrictions yet
- CLI and dashboard not started
