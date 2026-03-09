# Handoff 014 — Review (Stage 1: Per-App API Key Scoping)

## What I reviewed

Performed a comprehensive code review of all Stage 1 implementation:

### Backend (Go)
- `internal/auth/apps.go` — App CRUD operations, proper error handling, transactional DeleteApp with key revocation
- `internal/auth/apps_test.go` — Validation, JSON serialization, error sentinel tests
- `internal/auth/apikeys.go` — CreateAPIKey with optional appId, ValidateAPIKey with LEFT JOIN to apps for rate limit data, `applyAppRateLimitClaims` helper
- `internal/auth/apikeys_test.go` — Comprehensive scope/table/hash/format tests plus app-related claim and FK error mapping tests
- `internal/auth/app_ratelimit.go` — Sliding window rate limiter with per-app isolation, background cleanup goroutine
- `internal/auth/app_ratelimit_test.go` — Allow/deny/expiry/isolation/middleware/default-window tests
- `internal/auth/middleware.go` — RequireAuth, OptionalAuth, ClaimsFromContext, scope checking
- `internal/server/apps_handler.go` — Admin CRUD handlers with UUID validation, error mapping
- `internal/server/apps_handler_test.go` — Full coverage with fakeAppManager: list/get/create/update/delete, pagination, error paths
- `internal/server/apikeys_handler.go` — Admin key management with appId support, UUID validation for appId
- `internal/server/apikeys_handler_test.go` — Full coverage: CRUD, scopes, app-scoped creation, FK error handling
- `internal/server/admin.go:149` — **Verified app rate limiter IS correctly wired** in `requireAdminOrUserAuth` via closure chaining: `RequireAuth → appRL.Middleware → next`
- `internal/server/admin_middleware_test.go` — Tests rate limit enforcement through middleware chain AND admin bypass
- `internal/migrations/sql/017_ayb_apps.sql` — Apps table, app_id FK on api_keys
- `internal/migrations/sql/018_ayb_apps_fk_restrict.sql` — Changed FK from SET NULL to RESTRICT

### CLI
- `internal/cli/apps.go` — apps list/create/delete with table/JSON/CSV output
- `internal/cli/apikeys.go` — Extended with `--app` flag, App column in list
- `internal/cli/apps_test.go` — 17 tests covering all commands including edge cases

### Frontend
- `ui/src/components/Apps.tsx` — Full CRUD UI with pagination, modals, user email lookup
- `ui/src/components/ApiKeys.tsx` — Extended with app scope selector, app association display, rate limit stats
- `ui/src/components/__tests__/Apps.test.tsx` — 21 tests
- `ui/src/components/__tests__/ApiKeys.test.tsx` — 46 tests (was 43, added 3 new + enriched 1)
- `ui/src/types.ts` — AppResponse, AppListResponse, APIKeyResponse with appId field
- `ui/src/api.ts` — listApps, createApp, deleteApp, createApiKey with appId

## Issues found and fixed

### Test coverage gaps (fixed)

1. **Missing test: "User-scoped" label for legacy keys** — No test verified that keys with `appId: null` show the "User-scoped" label in the API keys list. Added test.

2. **Missing test: "Rate: unlimited" for unlimited-rate-limit apps** — The `formatAppRateLimit` function's unlimited path (`rateLimitRps <= 0`) was untested. Added test.

3. **Incomplete assertion on "app metadata unavailable" test** — The existing test checked that the raw app ID appears as fallback but didn't verify the "Rate: unknown" message also appears. Enriched test to assert both.

4. **Missing test: Created modal shows app info** — When creating an app-scoped key, the created modal shows app name and rate limit. This wasn't tested. Added test.

## No bugs found in implementation

After thorough review:
- App rate limiter is correctly wired via `requireAdminOrUserAuth` (not a standalone middleware mount, which initially looked suspicious)
- Admin tokens correctly bypass app rate limits (tested in `admin_middleware_test.go`)
- DeleteApp correctly revokes all app-scoped keys AND nullifies app_id in a transaction before deleting the app row
- FK constraint changed from SET NULL to RESTRICT prevents accidental privilege escalation
- Frontend correctly handles all edge cases: null appId, missing app metadata, unlimited rate limits
- CLI properly sends/omits appId based on --app flag
- All error paths return appropriate HTTP status codes with clear messages
- UUID validation happens at handler level before hitting DB

## Test results

- **Go auth package**: All tests pass (apps, apikeys, rate limiter, middleware, scopes)
- **Go server package**: All tests pass (apps handler, apikeys handler, admin middleware)
- **Go CLI package**: All tests pass (apps commands, apikeys commands)
- **UI Apps.test.tsx**: 21/21 pass
- **UI ApiKeys.test.tsx**: 46/46 pass (was 43, added 3 new + enriched 1)
- **TypeScript**: `tsc --noEmit` clean

## Files modified

- `ui/src/components/__tests__/ApiKeys.test.tsx` — Added 3 new tests, enriched 1 existing test
- `_dev/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST.md` — Added review progress note

## What's next

Stage 1 remaining unchecked items are **SDK & Docs** and final feature-status updates:
- Update TypeScript SDK types (if needed)
- Update docs:
  - `docs-site/guide/api-reference.md`
  - `docs-site/guide/admin-dashboard.md`
  - `docs-site/guide/configuration.md` (if config impact)
  - `tests/specs/admin.md`
- Completion updates:
  - `_dev/AYB_FEATURE_CHECKLIST_from_SIJLE.md`
  - `_dev/FEATURES.md`

## Key architecture notes for next dev

- The app rate limiter is wired through `requireAdminOrUserAuth` (in `internal/server/admin.go:149`), NOT as a standalone `r.Use()` call. This is a deliberate design: admin tokens bypass rate limits entirely, while user/API-key auth goes through both auth validation and app rate limiting.
- `ValidateAPIKey` uses a LEFT JOIN with `_ayb_apps` to fetch rate limit config. When an app is deleted, its keys are revoked first (revoked_at set), then app_id nullified, then app row deleted — all in one transaction.
- The migration uses ON DELETE RESTRICT (migration 018) to prevent silent FK cascade. The application code handles the cascade explicitly in `DeleteApp`.
