# Handoff 024 — Stage 2 Review Hardening (OAuth recent sessions)

## What I reviewed

Focused review scope: recent Stage 2 OAuth changes from the last four sessions/commits (OAuth clients, OAuth provider logic/tests, OAuth migrations/tests, admin handlers/tests).

## Issues found and fixed

1. **Authorization code consumed before full validation + race window**
- File: `internal/auth/oauth_provider.go`
- Problem: `ExchangeAuthorizationCode` marked codes as used before redirect/client/PKCE validation, and did not use row locking/conditional single-use update, allowing one-shot denial and replay race risk.
- Fix: moved exchange path into a transaction with `FOR UPDATE`, performed all validations first, then consumed code with `WHERE used_at IS NULL`, and issued tokens in the same transaction.

2. **Refresh token rotation race window**
- File: `internal/auth/oauth_provider.go`
- Problem: refresh token revocation update was unconditional (`WHERE id = $1`), so concurrent refreshes could both succeed.
- Fix: moved refresh flow into a transaction with `FOR UPDATE`; revoke old refresh with `WHERE revoked_at IS NULL`; on race/reuse detection revoke all tokens for the grant and return `invalid_grant`.

3. **Admin handler error boundary mapping produced false positives**
- Files: `internal/server/oauth_clients_handler.go`, `internal/server/oauth_clients_handler_test.go`
- Problem: internal create/rotate failures returned 400, and tests allowed weak assertions (`>= 400`).
- Fix: pre-validate create payload in handler, return 4xx for validation/domain errors and 5xx for internal errors; tightened tests to assert exact 500 + message.

4. **`IsOAuthClientID` too permissive + weak tests**
- Files: `internal/auth/oauth_clients.go`, `internal/auth/oauth_clients_test.go`
- Problem: helper only checked prefix; tests treated `ayb_cid_x` as valid.
- Fix: enforce exact format (`ayb_cid_` + 48 lowercase hex chars) and tightened tests for length/charset negatives.

5. **Integration test API mismatch**
- File: `internal/auth/oauth_provider_integration_test.go`
- Problem: used `SetOAuthProviderConfig` (non-existent) instead of `SetOAuthProviderModeConfig`.
- Fix: corrected method call and added PKCE retry regression test (failed PKCE must not consume the code).

## Tests (focused)

Passed:
- `GOCACHE=$PWD/.gocache go test ./internal/auth -run 'Test(IsOAuthClientID|GenerateClientID|GenerateClientSecret|HashAndVerifyClientSecret|ValidateRedirectURIs|MatchRedirectURI|ValidateOAuthScopes|IsScopeSubset|ValidateClientType|IsOAuthAccessToken|IsOAuthRefreshToken|IsOAuthToken|LocalhostRedirectURIPortMatching|PKCEVerifyS256|GeneratePKCEChallenge|PKCEChallengeIsBase64URL|OAuthErrorCodes|NewOAuthError|OAuthAccessTokenFormat|OAuthRefreshTokenFormat|OAuthProviderModeConfigDefaults|OAuthProviderModeConfigCustom|CreateAuthorizationCodeValidation|OAuthTokenResponseFormat)' -count=1`
- `GOCACHE=$PWD/.gocache go test ./internal/server -run 'TestAdmin(ListOAuthClients|GetOAuthClient|CreateOAuthClient|RevokeOAuthClient|RotateOAuthClientSecret|UpdateOAuthClient)' -count=1`
- `GOCACHE=$PWD/.gocache go test ./internal/migrations -run 'TestOAuthMigrationSQLConstraints' -count=1`
- `GOCACHE=$PWD/.gocache go test -c -tags=integration ./internal/auth` (integration compile check)

Not runnable in this sandbox:
- Integration execution requiring DB bootstrap (`TEST_DATABASE_URL`) and local port binding.

## Checklist/docs updates

Updated:
- `.mike/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST-96c8e1/checklists/stage_02_checklist.md`
- `_dev/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST.md`

## Files modified

- `internal/auth/oauth_clients.go`
- `internal/auth/oauth_clients_test.go`
- `internal/auth/oauth_provider.go`
- `internal/auth/oauth_provider_integration_test.go`
- `internal/server/oauth_clients_handler.go`
- `internal/server/oauth_clients_handler_test.go`
- `.mike/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST-96c8e1/checklists/stage_02_checklist.md`
- `_dev/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST.md`

## What’s next

1. Run OAuth integration tests in an environment with `TEST_DATABASE_URL` configured, especially:
   - `TestOAuthPKCEFailureDoesNotConsumeAuthorizationCode`
   - refresh reuse/race paths in `internal/auth/oauth_provider_integration_test.go`
2. Continue Stage 2 implementation from authorization/token endpoint handlers (currently still unchecked in stage checklist).
