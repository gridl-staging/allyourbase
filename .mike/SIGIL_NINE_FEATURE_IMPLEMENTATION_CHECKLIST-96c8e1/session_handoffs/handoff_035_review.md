# Handoff 035 — Stage 2 Review Session

## What I did

Comprehensive review of all Stage 2 OAuth 2.0 Provider Mode code and tests from the previous four sessions. Found and fixed 2 bugs and 1 false-positive test, added 2 new regression tests.

### Bug 1: CORS missing PUT method (severity: high)
- **File**: `internal/server/middleware.go:115`
- **Issue**: `Access-Control-Allow-Methods` was `"GET, POST, PATCH, DELETE, OPTIONS"` but the server uses `PUT` for `handleAdminUpdateOAuthClient` and `handleAdminUpdateApp`. Browser SPA clients would get CORS failures on all PUT requests.
- **Fix**: Added `PUT` to the allowed methods list.
- **Test**: Added PUT assertion to `TestCORSHeaders` in `internal/server/middleware_test.go`.

### Bug 2: Authorization endpoint accepts revoked OAuth clients (severity: security)
- **File**: `internal/auth/oauth_authorize_handler.go` in `validateOAuthAuthorizeRequest`
- **Issue**: `GetOAuthClient` returns the client record regardless of revocation status. The handler checked `errors.Is(err, ErrOAuthClientRevoked)` but `GetOAuthClient` never returns that error — it returns the struct with `RevokedAt` set. Result: revoked clients could initiate authorization flows and receive consent prompts. While tokens issued through this path would fail at `ValidateOAuthToken` (which does check client revocation), auth codes should never be issued for revoked clients.
- **Fix**: Replaced dead-code `ErrOAuthClientRevoked` error check with explicit `client.RevokedAt != nil` check after getting the client.
- **Tests**: Added `TestOAuthAuthorizeRevokedClientRejected` and `TestOAuthConsentRevokedClientRejected`.

### False positive removed: TestOAuthTokenResponseFormat
- **File**: `internal/auth/oauth_provider_test.go`
- **Issue**: Test created an `OAuthTokenResponse` struct literal, then asserted the fields it just set. This tested Go struct assignment, not any production code behavior. It would pass regardless of whether the token endpoint was implemented.
- **Fix**: Deleted the test.

## What I reviewed (no issues found)
- OAuth client registration (`oauth_clients.go`): ID generation, secret hashing (SHA-256 + constant-time compare), redirect URI validation (HTTPS enforcement, localhost exception, exact match), scope validation, client type validation — all correct.
- Authorization code flow (`oauth_provider.go`): Code generation, transactional single-use with `FOR UPDATE` + conditional update, PKCE S256 verification before code consumption (PKCE failure doesn't consume the code — confirmed by integration test), token pair issuance within same transaction — all correct.
- Token endpoint (`oauth_token_handler.go`): Content-type validation, grant_type dispatch, client auth extraction (Basic + POST body, rejects dual auth), OAuth error propagation — all correct.
- Refresh token rotation + reuse detection: Transactional rotation with `FOR UPDATE`, reuse detection revokes all grant tokens, concurrent refresh race handling — all correct per RFC 9700 §4.14.2.
- Token revocation (`oauth_revoke_handler.go`): RFC 7009 compliant (200 for unknown tokens, cascade on refresh revocation, nil pool handled safely) — all correct.
- Token validation (`ValidateOAuthToken`): Joins with clients + apps tables, checks client revocation, token revocation, expiry, returns rate limit info from app — all correct.
- Middleware (`validateTokenOrAPIKey`): OAuth token prefix routed before API key prefix (since `ayb_at_` starts with `ayb_`), claims conversion includes all rate limit fields — all correct.
- CLI commands and tests: 4 commands + 26 tests, pflag reset fix verified — all correct.
- Admin handler tests: Comprehensive fakes, proper 4xx/5xx mapping, service error injection — no false positives found.
- CORS tests: OAuth-specific preflight tests for `/token` and `/revoke` — valid.
- Integration tests: Auth code E2E, code replay, PKCE verification, PKCE retry after failure, client credentials, refresh rotation, reuse detection, revocation cascade, consent, redirect URI mismatch, client ID mismatch, allowed tables, expired token, rate limit propagation — comprehensive.

## Tests run
- `go test ./internal/auth -count=1 -v` → all pass (0.578s)
- `go test ./internal/server -count=1 -v` → all pass (0.309s)
- `go test ./internal/cli -run 'TestOAuth' -count=1 -v` → 26/26 pass

## Files modified
- `internal/server/middleware.go` — added PUT to CORS allowed methods
- `internal/server/middleware_test.go` — added PUT assertion to CORS test
- `internal/auth/oauth_authorize_handler.go` — fixed revoked client bypass (explicit `RevokedAt` check)
- `internal/auth/oauth_authorize_handler_test.go` — added 2 revoked client regression tests
- `internal/auth/oauth_provider_test.go` — removed false-positive `TestOAuthTokenResponseFormat`
- `_dev/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST.md` — added review note
- `.mike/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST-96c8e1/checklists/stage_02_checklist.md` — added review note

## Remaining Stage 2 unchecked items

### Admin Dashboard (UI)
- [ ] OAuth Clients management section
- [ ] Client details display
- [ ] Registration form
- [ ] Token stats per client
- [ ] Component tests

### Consent UI
- [ ] Consent page
- [ ] Auth redirect flow
- [ ] Scope descriptions
- [ ] Component tests

### SDK & Docs
- [ ] TypeScript SDK type definitions
- [ ] Authentication guide update
- [ ] OAuth provider guide
- [ ] Admin dashboard docs
- [ ] Configuration docs
- [ ] API reference
- [ ] Test specs

### Configuration
- [ ] Document defaults in config comments and docs

### Completion Gates
- [ ] All new tests green
- [ ] Existing auth tests still pass
- [ ] E2E flows verified
- [ ] Update feature checklists

## Next steps
1. **Admin Dashboard UI** — read `_dev/BROWSER_TESTING_STANDARDS_2.md` first, implement OAuth clients management section with 3-tier testing.
2. **Consent UI** — server-rendered or SPA consent page at `/oauth/consent`.
3. **SDK & Docs** — TypeScript types, guide updates.
4. **Completion gates** — integration test verification, feature checklist updates.
