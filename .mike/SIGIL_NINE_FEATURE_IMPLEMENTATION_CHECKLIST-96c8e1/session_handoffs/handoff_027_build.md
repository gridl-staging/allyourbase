# Handoff 027 — Stage 2 Build (OAuth Token Endpoint)

## What I did

Implemented OAuth provider token endpoint support in `internal/auth` and wired server routing so form-encoded token requests work end-to-end.

### 1. Added `POST /api/auth/token` handler
- Added `handleOAuthToken` and grant dispatch in `internal/auth/oauth_token_handler.go`.
- Supports:
  - `grant_type=authorization_code`
  - `grant_type=client_credentials`
  - `grant_type=refresh_token`
- Enforces `Content-Type: application/x-www-form-urlencoded`.
- Parses client authentication via:
  - HTTP Basic (`Authorization: Basic ...`)
  - Form body (`client_id`, `client_secret`)
- Rejects mixed Basic+body auth methods.
- Uses RFC-style OAuth JSON error envelope:
  - `{ "error": "...", "error_description": "..." }`
- Returns RFC token JSON shape from service responses.

### 2. Hooked handler into auth router
- Added `r.Post("/token", h.handleOAuthToken)` in `internal/auth/handler.go`.
- Added handler dependency interface field `oauthToken` to `Handler` for isolated tests.

### 3. Updated server routing/middleware for form token requests
- Moved `/api/auth/*` mount outside the JSON-only API middleware group in `internal/server/server.go`.
- Applied auth-route content-type allowlist:
  - `application/json`
  - `application/x-www-form-urlencoded`
- Keeps existing auth rate limiting.

### 4. TDD (red -> green) with focused tests
- Added new handler tests first in `internal/auth/oauth_token_handler_test.go`:
  - auth code happy path
  - Basic auth support
  - mixed auth method rejection
  - invalid client credentials -> `invalid_client`
  - redirect mismatch mapping -> `invalid_grant`
  - expired/replayed code mapping -> `invalid_grant`
  - client credentials happy path
  - public client rejected for client credentials -> `unauthorized_client`
  - refresh grant happy path
  - unsupported grant type
  - form content-type requirement
- Added server test in `internal/server/server_test.go`:
  - `/api/auth/token` accepts form content-type (verifies 400 `unsupported_grant_type`, not 415 middleware rejection).

### 5. Checklist + input updates
- Updated Stage 2 token endpoint checklist items to complete:
  - `.mike/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST-96c8e1/checklists/stage_02_checklist.md`
- Updated original input file with progress note:
  - `_dev/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST.md`

## Tests run

Passed:
- `GOCACHE=$PWD/.gocache go test ./internal/auth -run 'TestOAuthToken|TestOAuthAuthorize|TestOAuthConsent' -count=1`
- `GOCACHE=$PWD/.gocache go test ./internal/server -run 'TestAuthTokenEndpointAcceptsFormContentType' -count=1`

## Files created or modified

Created:
- `internal/auth/oauth_token_handler.go`
- `internal/auth/oauth_token_handler_test.go`
- `.mike/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST-96c8e1/session_handoffs/handoff_027_build.md`

Modified:
- `internal/auth/handler.go`
- `internal/server/server.go`
- `internal/server/server_test.go`
- `.mike/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST-96c8e1/checklists/stage_02_checklist.md`
- `_dev/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST.md`
- `.mike/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST-96c8e1/analytics/events_v1.jsonl` (session telemetry state file)
- `.mike/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST-96c8e1/state.json` (session state file)

## Next steps

1. Implement OAuth revocation HTTP endpoint `POST /api/auth/revoke` in `internal/auth` (RFC 7009 behavior: always 200).
2. Extend auth middleware `validateTokenOrAPIKey` to validate opaque OAuth access tokens and map into `Claims`.
3. Add middleware/scope enforcement tests for OAuth bearer tokens (`readonly` write denial, `allowed_tables`, and mixed auth coexistence).
