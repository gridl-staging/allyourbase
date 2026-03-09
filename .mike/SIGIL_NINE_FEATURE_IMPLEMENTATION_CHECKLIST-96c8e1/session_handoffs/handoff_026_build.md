# Handoff 026 — Stage 2 Build (OAuth Authorization + Consent Endpoints)

## What I built

Implemented OAuth provider-mode authorization HTTP endpoints in `internal/auth`:

1. `GET /api/auth/authorize`
- Requires authenticated user session.
- Validates `response_type=code`, `client_id`, `redirect_uri`, `scope`, `state`, `code_challenge`, `code_challenge_method=S256`.
- Validates redirect URI exact match against registered client URIs.
- Validates requested scope is allowed by the client.
- Parses optional `allowed_tables` query values (supports comma-separated values).
- Checks existing consent:
  - If missing: returns consent prompt payload (`requires_consent=true` + request context fields).
  - If present: issues authorization code and redirects to client `redirect_uri` with `code` + unchanged `state`.

2. `POST /api/auth/authorize/consent`
- Requires authenticated user session.
- Accepts consent decision payload.
- Re-validates all authorization request fields.
- `decision=deny`: redirects with RFC 6749 style `error=access_denied` and `state`.
- `decision=approve`: saves consent, issues authorization code, redirects with `code` + `state`.

Also wired both routes in auth router:
- `r.With(RequireAuth(h.auth)).Get("/authorize", h.handleOAuthAuthorize)`
- `r.With(RequireAuth(h.auth)).Post("/authorize/consent", h.handleOAuthConsent)`

## TDD (red -> green)

Added failing tests first, then implemented handlers to pass.

New focused handler tests (`internal/auth/oauth_authorize_handler_test.go`) cover:
- route auth requirement
- missing state
- invalid redirect URI rejection
- PKCE plain rejection (S256-only)
- invalid scope rejection
- consent-required prompt response
- consent-skip redirect (`code` + state round-trip)
- deny flow redirect (`error=access_denied` + state)
- approve flow (save consent + code redirect)
- invalid consent decision rejection

## Checklist updates

Updated stage checklist:
- `.mike/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST-96c8e1/checklists/stage_02_checklist.md`
  - Marked Authorization Endpoint block items complete (authorization validation, PKCE/state/scope validation, consent flow, authorize+consent handlers, and failing-test coverage line).

Updated original input file with progress note:
- `_dev/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST.md`
  - Added Stage 2 authorization handler note (dated 2026-02-22).

## Tests run

Passed:
- `GOCACHE=/tmp/ayb_gocache go test ./internal/auth -run 'TestOAuthAuthorize|TestOAuthConsent' -count=1`
- `GOCACHE=$PWD/.gocache go test ./internal/auth -run 'TestOAuthAuthorize|TestOAuthConsent|TestHandle(Register|Login|Refresh|Logout|PasswordReset|VerifyEmail|ResendVerification|DeleteMe)|TestRequireAuth' -count=1`

Not runnable in this sandbox environment:
- Some existing OAuth callback tests that create local `httptest.NewServer` listeners (port bind not permitted here).

## Files created/modified

Created:
- `internal/auth/oauth_authorize_handler.go`
- `internal/auth/oauth_authorize_handler_test.go`
- `.mike/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST-96c8e1/session_handoffs/handoff_026_build.md`

Modified:
- `internal/auth/handler.go`
- `.mike/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST-96c8e1/checklists/stage_02_checklist.md`
- `_dev/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST.md`

## Next steps

1. Implement `POST /api/auth/token` handler (`grant_type=authorization_code`, `refresh_token`, `client_credentials`) with RFC 6749 form parsing and OAuth JSON error envelope.
2. Implement `POST /api/auth/revoke` handler (RFC 7009 semantics).
3. Extend middleware `validateTokenOrAPIKey` fallback path to validate opaque OAuth access tokens and map into claims for downstream enforcement.
