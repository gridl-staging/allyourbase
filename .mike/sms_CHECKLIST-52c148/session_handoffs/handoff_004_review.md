# Handoff 004 — Review (Stage 3)

## What I did

### Reviewed Stage 3 implementation (Auth Service & Handlers)

Found and fixed **6 bugs** in the previous build session's work:

#### CRITICAL bugs (code wouldn't compile):
1. **Missing `smsEnabled` field on Handler struct** (`handler.go`): The `sms_auth.go` handlers referenced `h.smsEnabled` but the field didn't exist on the Handler struct. Added it.
2. **Missing `SetSMSEnabled` method** (`handler.go`): Tests called `h.SetSMSEnabled(true)` but the method wasn't defined. Added it matching the `SetMagicLinkEnabled` pattern.
3. **SMS routes not registered** (`handler.go`): `Routes()` didn't include `/sms` or `/sms/confirm` POST routes. Tests for route registration would have failed. Added both routes.

#### Logic bugs:
4. **`normalizePhone` used `unicode.IsDigit` instead of ASCII check** (`sms_auth.go`): `unicode.IsDigit` accepts Arabic-Indic and other non-ASCII digit characters (e.g., `١٢٣`), which are not valid E.164 digits. These are also multi-byte in UTF-8, so the `len(phone)-1` digit count would be wrong. Fixed to use `r >= '0' && r <= '9'` and `WriteByte` instead of `WriteRune`. Removed unused `unicode` import. Added non-ASCII digit test case.
5. **`UserByID` didn't select `phone` column** (`auth.go`): After adding `Phone` to the `User` struct, `UserByID` still only queried `id, email, created_at, updated_at`. SMS-authenticated users calling `/me` would have no phone in the response. Fixed with `COALESCE(phone, '')`.
6. **`Login` query didn't select `phone` column** (`auth.go`): Same issue — users who had both email and phone would lose the phone field on email/password login response. Fixed.

### Added missing test coverage:
- **`TestIsAllowedCountry`**: The geo-blocking function had zero test coverage. Added tests for: empty list allows all, explicit list filters correctly, unknown country code blocks safely.
- **Non-ASCII digit rejection**: Added Arabic-Indic digit test case to `TestNormalizePhoneRejectsInvalid`.

### Updated checklist
- Marked Steps 2, 3, and 4 as complete (except final commit)
- All items verified: `go test ./internal/auth/...` PASS (all tests), `go build ./...` PASS

## Test results
- `go test ./internal/auth/...` — PASS (16 SMS tests + all existing auth tests)
- `go test ./internal/sms/...` — PASS
- `go build ./...` — PASS

## What's next
- Stage 3 Step 4: Commit the work (`feat: add SMS OTP auth service and handlers`)
- Stage 4: Server wiring & integration tests

## Files modified
- `internal/auth/handler.go` — added `smsEnabled` field, `SetSMSEnabled` method, SMS routes in `Routes()`
- `internal/auth/sms_auth.go` — fixed `normalizePhone` ASCII digit check, removed `unicode` import
- `internal/auth/auth.go` — fixed `UserByID` and `Login` to include phone column
- `internal/auth/sms_auth_test.go` — added `TestIsAllowedCountry`, added non-ASCII digit test case
- `.mike/sms_CHECKLIST-52c148/checklists/stage_03_checklist.md` — updated all items to [x]
- `.mike/sms_CHECKLIST-52c148/session_handoffs/handoff_004_review.md` — this file
