# Handoff 010 — Review (Stage 4)

## What was reviewed

Full review of all Stage 4 work (iteration 9): CLI wiring, server wiring, integration tests, and smoke tests.

### Files reviewed
- `internal/cli/start.go` — `buildSMSProvider` function and SMS wiring block (lines 322-333)
- `internal/cli/start_sms_test.go` — 3 CLI tests for provider selection
- `internal/server/server.go` — `SetSMSEnabled` call (lines 208-209)
- `internal/auth/auth_integration_test.go` — 8 SMS integration tests + 2 server-level smoke tests + helpers
- `internal/auth/sms_auth.go` — OTP request/confirm service logic + HTTP handlers
- `internal/auth/handler.go` — route registration, `SetSMSEnabled` method
- `internal/auth/auth.go` — `SetSMSProvider`, `SetSMSConfig`, `DB()` methods
- `internal/sms/sms.go`, `capture.go` — Provider interface, CaptureProvider
- `internal/config/config.go` — SMS config fields, defaults, validation
- `internal/migrations/sql/002_ayb_users.sql` — users table schema (`email TEXT NOT NULL`)
- `internal/migrations/sql/013_ayb_sms.sql` — SMS tables

## Bugs found and fixed

### Bug 1: SMS user creation missing email NOT NULL constraint (CRITICAL)
**File**: `internal/auth/sms_auth.go:237-241`
**Problem**: `ConfirmSMSCode` inserted into `_ayb_users` with only `(phone, password_hash)`, but the `email` column has a `NOT NULL` constraint (from migration 002). Every new SMS-only user creation would fail at runtime with:
```
ERROR: null value in column "email" of relation "_ayb_users" violates not-null constraint
```
**Root cause**: The SMS INSERT was modeled after the user creation pattern but forgot to include a placeholder email, unlike the OAuth flow which generates `{provider}+{id}@oauth.local`.
**Fix**: Generate placeholder email `{phone}@sms.local` (e.g. `+14155552671@sms.local`) and include it in the INSERT, mirroring the OAuth pattern.
**Test**: Added assertion in `TestSMSFullFlow_NewUser` verifying `user.Email == "+14155552671@sms.local"`.
**Impact**: Without this fix, no SMS-only user could ever be created. The integration tests added in iteration 9 would have failed if actually run against a database, but they were added in a "build" iteration that only compiled the code.

## Design notes (not bugs, flagged for awareness)

1. **Country dial code coverage** (carried from handoff 008): `countryDialCode` map covers ~35 countries. Config validation accepts all ISO 3166-1 codes. A configured but unsupported country code would silently geo-block all its numbers.

2. **TOCTOU in daily limit** (carried from handoff 008): `RequestSMSCode` reads the daily count then increments non-atomically. Acceptable for a circuit breaker but concurrent requests could exceed the limit by 1-2.

3. **Placeholder email for SMS users**: Using `phone@sms.local` works but means SMS-only users have a non-real email in the `email` column. This is consistent with the OAuth pattern (`provider+id@oauth.local`). Future stages should consider making `email` nullable or adding a `has_real_email` flag if needed.

## Tests run

- `go build ./...` — PASS
- `go test ./internal/sms/...` — PASS (6 tests)
- `go test ./internal/auth/...` — PASS (unit tests, ~30 SMS-related)
- `go test ./internal/cli/...` — PASS (3 SMS provider tests)
- `go test -tags=integration ./internal/auth/... -run TestSMS` — PASS (11 tests)
- `go test -tags=integration ./internal/auth/...` — PASS (full suite, all existing integration tests)

## Commits

- `70d2f8d` — `fix: SMS user creation missing email NOT NULL constraint`

## Stage 4 checklist status

All items checked complete. Stage 4 is done — CLI wiring, server wiring, integration tests, and smoke tests are all implemented and verified.

## Files modified

- `internal/auth/sms_auth.go` — added placeholder email in SMS user INSERT
- `internal/auth/auth_integration_test.go` — added placeholder email assertion in `TestSMSFullFlow_NewUser`
- `.mike/sms_CHECKLIST-52c148/checklists/stage_04_checklist.md` — marked all items complete

## Files reviewed (no changes needed)

- `internal/cli/start.go` (`buildSMSProvider`, SMS wiring block)
- `internal/cli/start_sms_test.go` (3 unit tests)
- `internal/server/server.go` (`SetSMSEnabled` call)
- `internal/auth/handler.go` (route registration, handler struct)
- `internal/auth/auth.go` (service struct, setter methods)
- `internal/sms/sms.go`, `capture.go` (provider interface)
- `internal/config/config.go` (SMS config fields)

## Checklist and handoff file paths

- Checklist: `.mike/sms_CHECKLIST-52c148/checklists/stage_04_checklist.md`
- This handoff: `.mike/sms_CHECKLIST-52c148/session_handoffs/handoff_010_review.md`
- Previous handoff: `.mike/sms_CHECKLIST-52c148/session_handoffs/handoff_008_review.md`
