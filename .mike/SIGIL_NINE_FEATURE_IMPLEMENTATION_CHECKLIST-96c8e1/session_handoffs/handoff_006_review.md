# Handoff 006 — Review (Stage 1: Per-App API Key Scoping)

## What I Reviewed

Reviewed recent Stage 1 backend/auth work with focus on app-scoped API keys, rate limiting, and test quality. Also verified checklist discovery prerequisites by reading:
- `ui/src/components/ApiKeys.tsx`
- `internal/cli/apikeys.go`
- `sdk/src/client.ts`
- `_dev/AYB_FEATURE_CHECKLIST_from_SIJLE.md`
- `_dev/session/handoffs/handoff_207_migration_sigil_supabase.md`

## Bugs Found and Fixed

1. **App rate limits were loaded from DB but never copied to claims**
- Impact: app-scoped API keys never carried `AppRateLimitRPS` / `AppRateLimitWindow`, so downstream app rate limiting could not trigger from real API key auth.
- Fix: added `applyAppRateLimitClaims(...)` and used it in `ValidateAPIKey`.
- Files:
  - `internal/auth/apikeys.go`
  - `internal/auth/apikeys_test.go`

2. **Per-app rate limiter was not wired into authenticated server request path**
- Impact: even with `AppRateLimiter` implemented, main auth middleware path did not enforce it.
- Fix: added `appRL` to `Server`, initialized in `New`, stopped in `Shutdown`, and wrapped non-admin user-auth requests in `requireAdminOrUserAuth` with `appRL.Middleware(...)`.
- Files:
  - `internal/server/server.go`
  - `internal/server/admin.go`
  - `internal/server/admin_middleware_test.go`

## Tests (TDD: red -> green)

Red first:
- `GOCACHE=/tmp/ayb_gocache go test ./internal/auth -run 'TestApplyAppRateLimitClaims' -count=1` (failed: missing function)
- `GOCACHE=/tmp/ayb_gocache go test ./internal/server -run 'TestRequireAdminOrUserAuthAppRateLimitEnforced' -count=1` (failed: server missing app rate limiter wiring)

Green after fixes:
- `GOCACHE=/tmp/ayb_gocache go test ./internal/auth -run 'TestApplyAppRateLimitClaims' -count=1`
- `GOCACHE=/tmp/ayb_gocache go test ./internal/server -run 'TestRequireAdminOrUserAuthAppRateLimitEnforced' -count=1`
- `GOCACHE=/tmp/ayb_gocache go test ./internal/auth -run 'Test(AppRateLimiter|ApplyAppRateLimitClaims|IsAPIKey|ValidScopes|ClaimsIs|CheckWriteScope|CheckTableScope)' -count=1`
- `GOCACHE=/tmp/ayb_gocache go test ./internal/server -run 'Test(Admin(List|Create|Revoke)|RequireAdminOrUserAuthAppRateLimitEnforced)' -count=1`
- `GOCACHE=/tmp/ayb_gocache go test ./internal/server -run 'Test(APIRequireAdminOrUserAuth|StorageWriteRoutesRequireAuth|RequireAdminOrUserAuthAppRateLimitEnforced|AdminAuthRateLimited)' -count=1`

Note: full package runs that require opening local listeners are sandbox-blocked in this environment, so only focused test subsets were run.

## Checklist and Input Updates

Updated stage checklist progress:
- ` .mike/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST-96c8e1/checklists/stage_01_checklist.md`
  - Marked discovery reads complete (UI/CLI/SDK + requirements/handoff_207)
  - Marked backend per-app rate limiting implementation complete
  - Marked completion gate “Rate limiting test: app exceeding limit gets 429” complete

Updated original input file with current stage progress note:
- `_dev/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST.md`

## Files Created/Modified This Session

Created:
- `internal/server/admin_middleware_test.go`
- `.mike/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST-96c8e1/session_handoffs/handoff_006_review.md`

Modified:
- `internal/auth/apikeys.go`
- `internal/auth/apikeys_test.go`
- `internal/server/admin.go`
- `internal/server/server.go`
- `.mike/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST-96c8e1/checklists/stage_01_checklist.md`
- `_dev/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST.md`

Also touched by session runtime metadata:
- `.mike/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST-96c8e1/analytics/events_v1.jsonl`
- `.mike/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST-96c8e1/state.json`

## Next Work (Stage 1)

1. Add integration coverage for app-scoped key denial on out-of-scope table/operation paths (completion gate still open).
2. Confirm CLI app commands against Stage 1 requirements and close checklist items after focused CLI tests in a non-sandboxed test environment.
3. Complete remaining docs/spec updates (`docs-site/guide/*`, `tests/specs/admin.md`, `_dev/FEATURES.md`, `_dev/AYB_FEATURE_CHECKLIST_from_SIJLE.md`).
