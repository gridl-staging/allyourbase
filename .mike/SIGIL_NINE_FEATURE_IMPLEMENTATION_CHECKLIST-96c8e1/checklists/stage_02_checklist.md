# Stage 2: OAuth 2.0 Provider Mode

## Review Notes (2026-02-21)

Previous checklist had several issues corrected in this revision:
- **Token format**: Changed from JWT to opaque tokens. The previous plan extended JWT Claims with OAuth fields then added per-request revocation checks to ValidateToken — this defeats the purpose of JWTs (no DB lookup). Opaque tokens are simpler, cleanly revocable, and provide better separation from session tokens.
- **PKCE**: Now required for ALL clients (per RFC 9700 / OAuth 2.1 BCP), not just public clients. Dropped `plain` challenge method — only S256 supported.
- **Scope model**: Aligned with existing API key scopes (`readonly`, `readwrite`, `*`) + `AllowedTables` rather than inventing new `api:read`/`api:write` strings.
- **Refresh token reuse detection**: Added per RFC 9700 §4.14.2 — if a refresh token is used twice, revoke all tokens for that grant (indicates compromise).
- **Client authentication**: Explicitly specified (HTTP Basic or POST body per RFC 6749 §2.3).
- **Error responses**: Must follow RFC 6749 §5.2 format (`error`, `error_description`).
- **State parameter**: Now explicitly required (CSRF protection).
- **Client ID format**: Defined as prefixed random string (`ayb_cid_` + 24 hex bytes).
- **CORS**: OAuth endpoints must allow cross-origin requests for SPA clients.
- **Removed token introspection**: Moved to non-goals for v1. AYB is a single-server BaaS; introspection is for external resource servers which is out of scope.
- **Tokens table redesign**: Replaced paired access+refresh hash columns with single token_hash + token_type per row, linked by grant_id. Simpler, enables independent lifecycle.
- **Review hardening (2026-02-22)**: Fixed OAuth code/refresh single-use race windows using transactional `FOR UPDATE` + conditional updates; fixed admin OAuth client handlers to return 5xx for internal failures (and tightened false-positive tests); tightened `IsOAuthClientID` to enforce full `ayb_cid_` + 48 lowercase hex format.
- **Review regression hardening (2026-02-22)**: Fixed OAuth revocation crash path when DB pool is unavailable (return-safe per RFC 7009 behavior), fixed missing OAuth app rate-limit propagation (`ValidateOAuthToken` + claims mapping) that bypassed app limiter enforcement, and fixed revocation handler false-positive test that did not inject service errors.
- **Test hardening (2026-02-22)**: Fixed API-key validation nil-DB panic path (`ValidateAPIKey` now returns a safe error when pool is unavailable) discovered via new middleware tests, and reduced JWT expiry-boundary test runtime from ~2s wait to ~300ms while keeping stable expiry margins.
- **Config hardening (2026-02-22)**: Completed OAuth provider mode config wiring: env vars (`AYB_AUTH_OAUTH_PROVIDER_*`), config-key exposure (`auth.oauth_provider.*` in get/set validation), default TOML `[auth.oauth_provider]` section with documented defaults + PKCE note, positive-duration validation, and startup runtime mapping of configured durations into auth service provider-mode settings.
- **Review hardening (2026-02-22)**: Fixed CORS missing PUT method (SPA clients blocked from updating apps/OAuth clients), fixed authorization endpoint not rejecting revoked OAuth clients (security bypass), removed false-positive `TestOAuthTokenResponseFormat`, added revoked-client regression tests for authorize + consent endpoints.
- **Test review (2026-02-22)**: Full test suite review (12 files, ~6100 lines). No false positives, no redundancy, all suites fast. Added 11 handler-level parameter validation tests covering: missing grant_type, missing auth-code params (code/redirect_uri/code_verifier), missing refresh_token, missing/invalid scope for client_credentials, unknown client_id, invalid response_type, missing code_challenge/client_id/redirect_uri at authorize endpoint.
- **UI auth-flow hardening (2026-02-22)**: Fixed consent/login `return_to` interoperability bug (consent redirect now sends relative path; admin login return handler normalizes same-origin paths), closed protocol-relative open-redirect bypass (`//host`), and tightened regression tests to cover real 401 consent redirect behavior plus bypass attempts.
- **Test pass (2026-02-22)**: Full test suite run and review. Found and fixed consent approve handler missing `wantsJSON` check (SPA clients got 302 instead of JSON response on approve). All Go tests (auth ~85 OAuth, server ~40, CLI ~28, config ~4, migrations ~1) and UI tests (63 component tests across 3 files) pass. No false positives, no redundancy, no slow setups found.
- **Consent hardening (2026-02-22)**: Fixed consent reuse scope drift for `allowed_tables` (prior restricted consent could be reused for expanded table access without re-consent). `HasConsent` now checks requested `allowed_tables` are covered by stored consent and authorize handler forwards requested tables into consent checks. Added regressions in `oauth_provider_integration_test.go` + `oauth_authorize_handler_test.go`. Also removed React `act(...)` warning in OAuthClients loading-state test to keep UI suite warning-free.

---

## Discovery & Design

- [x] Read existing OAuth consumer implementation (`internal/auth/oauth.go`, `handler.go` OAuth routes) and understand state/callback flow
- [x] Read existing auth token system (`auth.go` generateToken/ValidateToken, Claims struct, refresh token flow) to understand token patterns
- [x] Read Stage 1 app identity model (`internal/auth/apps.go`) — OAuth clients will build on the apps table
- [x] Read requirements source: `_dev/AYB_FEATURE_CHECKLIST_from_SIJLE.md` (OAuth 2.0 provider mode section) and `_dev/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST.md` (section 1)
- [x] Research OAuth 2.0 RFC 6749 grant types and decide which to support in v1: **authorization_code + client_credentials** (implicit and ROPC are deprecated per RFC 9700 — do not implement)
- [x] Research PKCE (RFC 7636): require for ALL clients (public and confidential), S256 only (no plain), per OAuth 2.1 BCP and RFC 9700
- [x] Decide token format: **opaque tokens** (random bytes, SHA-256 hash stored in DB) for OAuth access/refresh tokens — NOT JWTs. Rationale: revocation requires DB lookup anyway; opaque is simpler and cleanly separated from session JWTs
- [x] Record chosen approach and rejected alternatives in `_dev/ARCHITECTURE_DECISIONS.md` (ADR for OAuth provider mode)
- [x] Define explicit non-goals for v1: no dynamic client registration (RFC 7591), no device authorization grant (RFC 8628), no OpenID Connect id_token, no DPoP/mTLS sender-constraining, no token introspection endpoint (RFC 7662 — single-server BaaS doesn't need it)

## Database Schema

- [x] Design and write migration SQL `019_ayb_oauth_clients.sql`: `_ayb_oauth_clients` table (id UUID PK, app_id FK→_ayb_apps NOT NULL, client_id VARCHAR UNIQUE with `ayb_cid_` prefix + 24 random hex, client_secret_hash VARCHAR, name VARCHAR NOT NULL, redirect_uris TEXT[] NOT NULL, scopes TEXT[] NOT NULL, client_type VARCHAR CHECK IN ('confidential','public') NOT NULL, created_at TIMESTAMPTZ DEFAULT now(), updated_at TIMESTAMPTZ DEFAULT now(), revoked_at TIMESTAMPTZ NULL)
- [x] Design and write migration SQL `020_ayb_oauth_authorization_codes.sql`: `_ayb_oauth_authorization_codes` table (id UUID PK, code_hash VARCHAR UNIQUE NOT NULL, client_id VARCHAR FK→_ayb_oauth_clients(client_id), user_id FK→_ayb_users, redirect_uri TEXT NOT NULL, scope VARCHAR NOT NULL, allowed_tables TEXT[], code_challenge VARCHAR NOT NULL, code_challenge_method VARCHAR NOT NULL DEFAULT 'S256', state VARCHAR NOT NULL, expires_at TIMESTAMPTZ NOT NULL, used_at TIMESTAMPTZ NULL, created_at TIMESTAMPTZ DEFAULT now())
- [x] Design and write migration SQL `021_ayb_oauth_tokens.sql`: `_ayb_oauth_tokens` table (id UUID PK, token_hash VARCHAR UNIQUE NOT NULL, token_type VARCHAR CHECK IN ('access','refresh') NOT NULL, client_id VARCHAR FK→_ayb_oauth_clients(client_id), user_id FK→_ayb_users NULL (null for client_credentials grants), scope VARCHAR NOT NULL, allowed_tables TEXT[], grant_id UUID NOT NULL, expires_at TIMESTAMPTZ NOT NULL, revoked_at TIMESTAMPTZ NULL, created_at TIMESTAMPTZ DEFAULT now()); CREATE INDEX idx_oauth_tokens_grant_id ON _ayb_oauth_tokens(grant_id) for reuse detection; CREATE INDEX idx_oauth_tokens_client_id ON _ayb_oauth_tokens(client_id) for per-client stats
- [x] Design and write migration SQL `022_ayb_oauth_consents.sql`: `_ayb_oauth_consents` table (id UUID PK, user_id FK→_ayb_users, client_id VARCHAR FK→_ayb_oauth_clients(client_id), scope VARCHAR NOT NULL, allowed_tables TEXT[], granted_at TIMESTAMPTZ DEFAULT now(), UNIQUE(user_id, client_id))
- [x] Write tests for migrations: apply, verify schema, rollback

## Backend Implementation — Client Registration

- [x] Implement `internal/auth/oauth_clients.go`: OAuthClient struct, RegisterClient, GetClient, ListClients, UpdateClient, RevokeClient, RegenerateClientSecret
- [x] Link OAuth clients to Stage 1 apps: each OAuth client belongs to an `_ayb_apps` record (app_id FK NOT NULL); inherits app-level rate limits
- [x] Implement client_secret hashing using existing `hashToken` (SHA-256) — appropriate because secrets are server-generated high-entropy random strings, same pattern as API keys
- [x] Implement client_id generation: `ayb_cid_` prefix + 24 random hex bytes (consistent with API key prefix pattern `ayb_`)
- [x] Implement redirect_uri validation: exact-match only (no wildcards, no query params), require HTTPS in production, allow `http://localhost:*` and `http://127.0.0.1:*` for development (port wildcard for native apps per RFC 8252 §7.3)
- [x] Implement `internal/auth/oauth_clients_handler.go`: admin HTTP handlers for client CRUD (POST/GET/PUT/DELETE `/api/admin/oauth/clients`)
- [x] Wire OAuth client admin routes into server router (`internal/server/server.go`) under `/api/admin/oauth/clients`
- [x] Write failing tests first for: client registration, client_id format validation, client secret hashing/verification, redirect_uri validation (HTTPS enforcement, exact match, localhost exception), client CRUD, revoked client rejection

## Backend Implementation — Authorization Endpoint

- [x] Implement `internal/auth/oauth_authorize.go`: authorization endpoint logic — validate client_id, redirect_uri (exact match against registered URIs), response_type=code, scope, state (required — reject if missing)
- [x] Implement authorization code generation: 32 random bytes hex-encoded, store SHA-256 hash in DB, 10-minute TTL, single-use (set `used_at` on exchange, reject if already set)
- [x] Implement PKCE: require `code_challenge` + `code_challenge_method=S256` on ALL authorization requests (public and confidential clients); store with code; reject requests with `plain` method or missing challenge
- [x] Implement scope validation: map OAuth scopes to existing API key scope model — `readonly` (GET only), `readwrite` (GET+POST+PUT+DELETE), `*` (full access); support optional `allowed_tables` restriction; validate requested scope is subset of client's registered scopes
- [x] Implement consent flow: check `_ayb_oauth_consents` for prior matching grant (same user + client + scope subset); if none, return consent prompt data; on user approval, store consent and issue code
- [x] Implement `state` parameter: require on all authorization requests, return unchanged in redirect; reject requests without state (CSRF protection)
- [x] Add handler `handleOAuthAuthorize` at `GET /api/auth/authorize` — validates params, checks consent, redirects with code+state or serves consent data
- [x] Add handler `handleOAuthConsent` at `POST /api/auth/authorize/consent` — processes user's grant/deny decision (requires authenticated user session); on deny, redirect with `error=access_denied` per RFC 6749 §4.1.2.1
- [x] Write failing tests for: authorization request validation, missing/invalid redirect_uri rejection, code generation + single-use enforcement, PKCE S256-only enforcement (plain rejected), scope validation against client registered scopes, consent persistence + consent skip on re-auth, state parameter round-trip, deny flow returns access_denied error, RFC 6749 §4.1.2.1 error response format

## Backend Implementation — Token Endpoint

- [x] Implement `internal/auth/oauth_token.go`: token endpoint logic for `grant_type=authorization_code` — validate code (lookup by hash, check TTL, check single-use via used_at), validate client credentials, validate redirect_uri matches original request, verify PKCE code_verifier (SHA-256 of verifier must equal stored code_challenge)
- [x] Implement `grant_type=client_credentials` — validate client_id + client_secret (confidential clients only), issue opaque access token (no user context, app-level access), no refresh token issued
- [x] Implement `grant_type=refresh_token` — validate refresh token (lookup by hash, check not revoked/expired); implement refresh token rotation with reuse detection: on successful use, revoke old refresh token and issue new pair; if token already has revoked_at set (reuse attempt), revoke ALL tokens with same grant_id (compromise indicator per RFC 9700 §4.14.2)
- [x] Implement opaque access token generation: 32 random bytes hex-encoded with `ayb_at_` prefix, store SHA-256 hash in `_ayb_oauth_tokens` with token_type='access', default 1-hour TTL (configurable)
- [x] Implement opaque refresh token generation: 48 random bytes hex-encoded with `ayb_rt_` prefix, store SHA-256 hash in `_ayb_oauth_tokens` with token_type='refresh', default 30-day TTL (configurable), linked to access token via shared grant_id
- [x] Implement client authentication at token endpoint: support HTTP Basic auth (`Authorization: Basic base64(client_id:client_secret)`) and POST body params (`client_id` + `client_secret` fields); public clients send only `client_id` (no secret required)
- [x] Implement RFC 6749 §5.1 compliant success response: `{ "access_token": "...", "token_type": "Bearer", "expires_in": 3600, "refresh_token": "...", "scope": "..." }`
- [x] Implement RFC 6749 §5.2 compliant error responses: `{ "error": "...", "error_description": "..." }`; supported error codes: `invalid_request`, `invalid_client`, `invalid_grant`, `unauthorized_client`, `unsupported_grant_type`, `invalid_scope`
- [x] Add handler `handleOAuthToken` at `POST /api/auth/token` — accept `application/x-www-form-urlencoded` per RFC 6749; respond with `application/json`
- [x] Write failing tests for: auth code exchange (happy path), PKCE S256 verification, code replay rejection (used_at already set), client_credentials grant (happy path), refresh token rotation (old invalidated, new pair issued), refresh token reuse detection (all grant tokens revoked), expired code rejection, invalid client credentials rejection, redirect_uri mismatch rejection, token response format compliance (token_type, expires_in present)

## Backend Implementation — Token Revocation

- [x] Implement token revocation endpoint `POST /api/auth/revoke` per RFC 7009: accept `token` + optional `token_type_hint` (access_token/refresh_token), look up by hash, set revoked_at; on refresh token revocation also revoke all tokens with same grant_id; return 200 OK regardless (per RFC 7009 — don't leak token existence)
- [x] Implement `ValidateOAuthToken` method: look up opaque token by hash in `_ayb_oauth_tokens`, verify not revoked and not expired, return scope + user_id + client_id + allowed_tables; keep completely separate from `ValidateToken` (JWT path)
- [x] Write failing tests for: revocation of access token, revocation of refresh token (cascading grant revocation), validation of valid/revoked/expired opaque tokens, revocation endpoint returns 200 for unknown tokens (per RFC 7009 spec)

## Backend Implementation — Middleware & Enforcement

- [x] Extend `validateTokenOrAPIKey` in middleware to try opaque OAuth token lookup (via `ValidateOAuthToken`) when JWT parsing fails and token doesn't start with `ayb_` API key prefix; construct Claims-compatible struct from OAuth token data for downstream handlers
- [x] Implement scope enforcement for OAuth tokens: `readonly` allows GET only, `readwrite` allows GET/POST/PUT/DELETE, `*` allows all; enforce `allowed_tables` restrictions same as existing API key code path
- [x] Ensure OAuth tokens respect app-level rate limits from the linked app (resolve client_id → app_id → rate limits; reuse existing AppRateLimiter from Stage 1)
- [x] Ensure backward compatibility: existing JWT tokens (login/register) and API keys continue to work unchanged; OAuth token path is purely additive
- [x] Ensure CORS headers allow cross-origin POST to `/api/auth/token` and `/api/auth/revoke` for SPA OAuth clients
- [x] Write failing tests for: scope-restricted access (readonly token denied POST), allowed_tables enforcement, rate limiting via OAuth client's app, mixed auth coexistence (OAuth + API key + session JWT all work), CORS preflight on token endpoint

## CLI

- [x] Add `ayb oauth clients create <app-id>` command with `--name`, `--redirect-uris` (comma-separated), `--scopes` (comma-separated from: readonly, readwrite, *), `--type` (confidential/public, default confidential) flags; display client_id and client_secret (secret shown once only)
- [x] Add `ayb oauth clients list` command with `--json` flag for JSON output
- [x] Add `ayb oauth clients delete <client-id>` command (soft-delete via revoked_at)
- [x] Add `ayb oauth clients rotate-secret <client-id>` command — regenerates client_secret, displays new secret once
- [x] Write tests for all new CLI commands
- [x] Fix CLI test pflag StringSlice cross-test pollution (2026-02-22): replaced `fl.Value.Set("")` with `pflag.SliceValue.Replace([]string{})` in resetOAuthCreateFlags to properly bypass pflag's internal append-vs-replace state

## Admin Dashboard

- [x] Add OAuth Clients management section: list clients (client_id, name, linked app, type, active/revoked status), register client button, revoke/rotate-secret actions
- [x] Show client details: client_id (always visible), client_secret (shown once on create/rotate in modal), redirect URIs, scopes, linked app name, client type
- [x] Add OAuth client registration form: app selector dropdown (existing apps), name input, redirect URIs (multi-value input), scope checkboxes (readonly/readwrite/*), client type radio (confidential/public)
- [x] Show OAuth token stats per client: active access token count, active refresh token count, total grants, last token issued timestamp
- [x] Write component tests for new UI elements

## Consent UI

- [x] Implement consent page (server-rendered HTML or SPA route at `/oauth/consent`): display requesting app name + description (from linked _ayb_apps record), requested scope in human-readable form, approve/deny buttons
- [x] Consent page must require authenticated user session; if not authenticated, redirect to login with `return_to` parameter preserving full authorize URL, then redirect back to consent after login (2026-02-22: implemented 401 detection in OAuthConsent + return_to redirect in App.tsx with open-redirect prevention)
- [x] Consent page scope descriptions: `readonly` → "Read your data", `readwrite` → "Read and modify your data", `*` → "Full access to your account"; if allowed_tables specified, list table names
- [x] Write component tests for consent page rendering, scope display, approve/deny interaction, unauthenticated redirect-to-login flow

## SDK & Docs

- [x] Update TypeScript SDK type definitions for OAuth client management responses (OAuthClient type, OAuthClientCreateResponse with one-time secret field)
- [x] Update `docs-site/guide/authentication.md` with OAuth provider mode overview (what it enables, when to use it, link to detailed guide)
- [x] Create `docs-site/guide/oauth-provider.md`: detailed guide — client registration, authorization code flow with PKCE walkthrough, client credentials flow, token lifecycle (refresh rotation, revocation), scope model aligned with API key scopes, consent behavior
- [x] Update `docs-site/guide/admin-dashboard.md` with OAuth client management UI documentation
- [x] Update `docs-site/guide/configuration.md` with OAuth provider config options (token durations, enable/disable)
- [x] Update `docs-site/guide/api-reference.md` with OAuth endpoints: `/api/auth/authorize`, `/api/auth/token`, `/api/auth/revoke`, `/api/admin/oauth/clients`
- [x] Create or update `tests/specs/oauth.md` with OAuth provider test cases (auth code flow, client credentials, PKCE, refresh rotation, reuse detection, revocation, scope enforcement, consent)

## Configuration

- [x] Add OAuth provider config section to `internal/config/config.go`: `OAuthProvider` struct with `Enabled` bool (default false), `AccessTokenDuration` int (seconds, default 3600), `RefreshTokenDuration` int (seconds, default 2592000 = 30 days), `AuthCodeDuration` int (seconds, default 600)
- [x] Add config validation: OAuth provider requires `auth.enabled = true` and `auth.jwt_secret` to be set (jwt_secret not used for OAuth tokens themselves but needed for session auth that consent flow depends on)
- [x] Add TOML config section: `[auth.oauth_provider]` with `enabled`, `access_token_duration`, `refresh_token_duration`, `auth_code_duration`
- [x] Document defaults in config comments and docs: access token 1h, refresh token 30d, auth code 10min, PKCE always required (S256 only, not configurable off)

## Completion Gates

- [x] All new tests green (unit + integration) — verified 2026-02-22: auth 0.67s, server 0.29s, CLI 0.56s, config 0.18s, migrations 0.19s, UI 447 tests 2.9s
- [x] Existing auth tests still pass (backward compatibility: login, register, API keys, OAuth consumer flow unchanged)
- [x] Authorization code flow end-to-end: register client → authorize (with PKCE S256 + state) → consent → exchange code → access API with opaque token → refresh → revoke — covered in `oauth_provider_integration_test.go`
- [x] Client credentials flow end-to-end: register confidential client → token request with client_id+secret → access API (app-level, no user context) — covered in `oauth_provider_integration_test.go`
- [x] PKCE enforcement: both public and confidential clients must provide code_challenge/code_verifier (S256 only) — covered in integration + handler tests
- [x] Refresh token rotation: old refresh token invalidated after use, new access+refresh pair issued — covered in `oauth_provider_integration_test.go`
- [x] Refresh token reuse detection: using an already-rotated refresh token revokes ALL tokens for that grant_id — covered in `oauth_provider_integration_test.go`
- [x] Negative tests: invalid redirect_uri rejected, expired code rejected, replayed code rejected (used_at set), revoked token rejected, scope violation denied, missing state rejected, plain PKCE method rejected, unknown token revocation returns 200 — all covered across handler + integration tests
- [x] Rate limiting: OAuth client inherits app rate limits via app_id FK, exceeding limit returns 429 — covered in middleware + integration tests
- [x] Update `_dev/AYB_FEATURE_CHECKLIST_from_SIJLE.md` to mark OAuth 2.0 provider mode complete
- [x] Update `_dev/FEATURES.md` to reflect OAuth provider mode
