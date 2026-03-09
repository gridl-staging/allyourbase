# Handoff 008 — Review (Stage 1: Per-App API Key Scoping)

## What I reviewed

Reviewed Stage 1 backend/admin paths and tests with focus on boundary validation, false-positive tests, and error mapping around app-scoped API key creation.

## Bugs found and fixed

1. Admin API key creation accepted invalid `userId` format and created keys anyway.
- Fix: added UUID validation in `handleAdminCreateAPIKey`.
- Files:
  - `internal/server/apikeys_handler.go`
  - `internal/server/apikeys_handler_test.go`

2. Admin app creation accepted invalid `ownerUserId` format and passed it to service.
- Fix: added UUID validation in `handleAdminCreateApp`.
- Files:
  - `internal/server/apps_handler.go`
  - `internal/server/apps_handler_test.go`

3. API key creation DB error mapping incorrectly treated all FK violations as `ErrInvalidAppID`.
- Impact: missing users could be misreported as “app not found”.
- Fix: added FK constraint-aware mapping (`app_id` FK -> `ErrInvalidAppID`, `user_id` FK -> `ErrUserNotFound`).
- Files:
  - `internal/auth/apikeys.go`
  - `internal/auth/apikeys_test.go`

## Test updates (TDD red -> green)

Added failing tests first, then fixed code:
- `TestAdminCreateAPIKeyInvalidUserIDFormat`
- `TestAdminCreateAPIKeyUserNotFound`
- `TestAdminCreateAppInvalidOwnerIDFormat`
- `TestMapCreateAPIKeyInsertErrorAppFK`
- `TestMapCreateAPIKeyInsertErrorUserFK`
- `TestMapCreateAPIKeyInsertErrorInvalidUUID`
- `TestMapCreateAPIKeyInsertErrorPassthrough`

Adjusted existing tests to use valid UUID fixtures where they were unintentionally relying on lax validation.

## Focused test commands run

Passed:
- `GOCACHE=/tmp/ayb_gocache go test ./internal/auth -run 'TestMapCreateAPIKeyInsertError|APIKey|Scope|AppRate' -count=1`
- `GOCACHE=/tmp/ayb_gocache go test ./internal/server -run 'AdminCreateAPIKey|AdminListAPIKeys|AdminCreateApp|AdminUpdateApp|AdminDeleteApp|AdminListApps|RequireAdminOrUserAuthAppRateLimit' -count=1`
- `GOCACHE=/tmp/ayb_gocache go test ./internal/migrations -run 'Runner|Migration' -count=1`
- `GOCACHE=/tmp/ayb_gocache go test ./internal/cli -run 'TestAppsCommandRegistered' -count=1`

Blocked by sandbox bind restrictions (cannot open loopback listeners here):
- broader `internal/auth`/`internal/server` tests using `httptest.NewServer` or local listeners
- `internal/cli` tests that spin up `httptest.NewServer`

## Checklist/docs updates

- Updated stage tracker note in:
  - `_dev/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST.md`

No stage checkbox state changed this session; this was a review/fix pass.

## Files modified

- `internal/auth/apikeys.go`
- `internal/auth/apikeys_test.go`
- `internal/server/apikeys_handler.go`
- `internal/server/apikeys_handler_test.go`
- `internal/server/apps_handler.go`
- `internal/server/apps_handler_test.go`
- `_dev/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST.md`
- `.mike/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST-96c8e1/session_handoffs/handoff_008_review.md`

## What’s next

1. Run Stage 1 CLI test subset in a non-sandboxed environment (bind/listener required) to fully validate `apps` and `apikeys --app` command paths.
2. Add/confirm completion-gate negative tests explicitly labeled for app-scoped keys denied out-of-scope table/write operations if you want that gate checked.
