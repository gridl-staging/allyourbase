# Handoff 029 — Stage 2 Review (OAuth recent-session hardening)

## Scope reviewed
- Recent OAuth work from the latest sessions (authorization/token/revoke handlers, middleware integration, token validation).
- Focus areas requested: logic bugs, false-positive tests, boundary error handling, checklist alignment.

## Issues found and fixed

1. **Crash on `/api/auth/revoke` when auth service has no DB pool**
- Root cause: `Service.RevokeOAuthToken` dereferenced `s.pool` unconditionally.
- Impact: panic -> 500 instead of RFC 7009 behavior when running in nil-pool test/runtime contexts.
- Fix:
  - Added nil-pool guard in `RevokeOAuthToken` that safely returns (`internal/auth/oauth_provider.go`).

2. **OAuth app rate limits were not propagated into claims (rate limiting bypass risk)**
- Root cause: `ValidateOAuthToken` did not load app rate-limit columns; `oauthTokenInfoToClaims` did not map them into `Claims`.
- Impact: OAuth tokens could miss `AppRateLimitRPS/AppRateLimitWindow` and bypass Stage 1 app limiter behavior.
- Fix:
  - `ValidateOAuthToken` now joins `_ayb_apps` and returns rate-limit fields.
  - `ValidateOAuthToken` filters out revoked OAuth clients (`c.revoked_at IS NULL`).
  - `oauthTokenInfoToClaims` now maps rate-limit fields into `Claims`.

3. **False-positive revocation test**
- Root cause: `TestOAuthRevokeAlways200OnServiceError` did not inject any error (`revokeErr: nil`), so it never exercised the intended path.
- Fix:
  - Test now injects a real error and asserts handler still returns 200 and calls provider once.

## Tests added/strengthened (red -> green)
- Added failing tests first, then fixed implementation:
  - `internal/auth/middleware_test.go`
    - `TestValidateTokenOrAPIKeyOAuthWithNilPoolReturnsError`
    - `TestOAuthTokenInfoToClaimsIncludesAppRateLimitFields`
- Strengthened existing test:
  - `internal/auth/oauth_revoke_handler_test.go`
    - `TestOAuthRevokeAlways200OnServiceError` now uses a real injected error and verifies call behavior.

## Focused tests run
- `GOCACHE=$PWD/.gocache go test ./internal/auth -run 'TestValidateTokenOrAPIKeyOAuthWithNilPoolReturnsError|TestOAuthTokenInfoToClaimsIncludesAppRateLimitFields|TestOAuthRevokeAlways200OnServiceError|TestOAuthToken|TestOAuthAuthorize|TestOAuthConsent|TestOAuthRevoke' -count=1`
- `GOCACHE=$PWD/.gocache go test ./internal/server -run 'TestAuthTokenEndpointAcceptsFormContentType|TestAuthRevokeEndpointAcceptsFormContentType|TestCORSPreflightOnOAuthTokenEndpoint|TestCORSPreflightOnOAuthRevokeEndpoint|TestRequireAdminOrUserAuthAppRateLimitEnforced|TestRequireAdminOrUserAuthAdminBypassesRateLimit' -count=1`

## Checklist and input updates
- Updated stage checklist:
  - `.mike/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST-96c8e1/checklists/stage_02_checklist.md`
  - Added review regression note.
  - Marked Token Revocation block complete.
  - Marked app-rate-limit and CORS middleware items complete.
- Updated original input file with this session’s review regression note:
  - `_dev/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST.md`

## Files modified
- `internal/auth/oauth_provider.go`
- `internal/auth/middleware.go`
- `internal/auth/middleware_test.go`
- `internal/auth/oauth_revoke_handler_test.go`
- `.mike/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST-96c8e1/checklists/stage_02_checklist.md`
- `_dev/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST.md`

## Prior handoff references reviewed
- `.mike/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST-96c8e1/session_handoffs/handoff_024_review.md`
- `.mike/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST-96c8e1/session_handoffs/handoff_026_build.md`
- `.mike/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST-96c8e1/session_handoffs/handoff_027_build.md`

## Next steps
1. Run OAuth integration tests in an environment with `TEST_DATABASE_URL` to execute full DB-backed coverage for rate-limit propagation and revoke/validate behavior.
2. Continue remaining Stage 2 middleware scope/backward-compat items and associated tests.
