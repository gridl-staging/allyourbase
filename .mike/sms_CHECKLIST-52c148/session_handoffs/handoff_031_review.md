# Handoff 031 — Stage 7 Review (Steps 1–4)

## What was reviewed

Thorough review of all code and tests from Steps 1–4 of Stage 7 (phonenumbers upgrade + migration 015).

### Files reviewed
- `internal/auth/sms_auth.go` — normalizePhone, phoneCountry, isAllowedCountry, RequestSMSCode, ConfirmSMSCode, handlers
- `internal/auth/sms_auth_test.go` — all unit tests for the above
- `internal/auth/sms_mfa.go` — storeOTPCode, sendOTPToPhone, validateSMSCodeForPhone, MFA enrollment/challenge/verify
- `internal/auth/auth.go` — Service struct, imports, error sentinels
- `internal/migrations/sql/015_ayb_sms_stats.sql` — new migration
- `internal/migrations/sql/013_ayb_sms.sql` — original SMS tables
- `internal/migrations/runner.go` — migration runner (auto-discovers embedded SQL files)

## Bugs found and fixed (3)

### 1. Weak test assertions (false positive risk)
`TestNormalizePhoneRejectsInvalid` and `TestNormalizePhone_RejectsInvalidForCountry` used `err != nil` assertions instead of `errors.Is(err, ErrInvalidPhoneNumber)`. If the function ever returned a different error type (e.g., a wrapped internal error), these tests would still pass — hiding a regression.

**Fix:** Replaced with `errors.Is(err, ErrInvalidPhoneNumber)` checks. Added `"errors"` import.

### 2. Test case tests wrong thing
`TestNormalizePhone_RejectsInvalidForCountry` had `+1999555000` (only 9 digits after +1). This fails for wrong digit count, not for the invalid area code 999 that the comment claims. The test name and comment are misleading.

**Fix:** Changed to `+19995551234` (proper 10-digit US format with unassigned area code 999). Verified with phonenumbers library that this is rejected specifically for invalid area code.

### 3. Missing edge case test coverage
No tests existed for `phoneCountry` with invalid inputs (empty string, garbage, too-short) or `isAllowedCountry` with unparseable phone numbers. These paths work correctly but were untested — a refactor could introduce a panic with no test to catch it.

**Fix:** Added `TestPhoneCountry_InvalidInputs` (4 cases) and `TestIsAllowedCountry_UnparseablePhone` (2 cases).

## Design observations (no changes needed)

- **Double test-phone check** in `RequestSMSCode` + `sendOTPToPhone`: Not a bug. The `RequestSMSCode` check correctly skips daily count increment for test phones. The `sendOTPToPhone` check serves MFA callers (`EnrollSMSMFA`, `ChallengeSMSMFA`) that don't have their own pre-check.
- **Daily count incremented before send**: Intentional abuse-prevention design. Comment documents the tradeoff.
- **Handler pre-validates phone format (400) before anti-enumeration (200)**: Standard practice — format validation is not enumeration; the 200 is about hiding whether an account exists.
- **confirm_count/fail_count columns unused**: Expected — Step 5 implements the instrumentation code.
- **Migration 015**: Correct and idempotent (`IF NOT EXISTS`). Table name matches 013.

## Test results

All test suites pass:
- `go build ./...` — PASS
- `go test ./internal/auth/...` — PASS (2.28s)
- `go test ./internal/sms/...` — PASS
- `go test ./internal/config/...` — PASS
- `go test ./internal/server/...` — PASS

## Commit
`2f73976` — `fix: strengthen phone validation test assertions and add edge case coverage`

## Files modified
- `internal/auth/sms_auth_test.go` — strengthened assertions, fixed test case, added 2 new test functions

## What's next
- Steps 5–7: Implement SMS confirmation tracking (`confirm_count`/`fail_count` instrumentation), admin SMS health endpoint, and server wiring
- Step 8: Final build & test verification

## Checklist path
- `.mike/sms_CHECKLIST-52c148/checklists/stage_07_checklist.md`

## Previous handoff
- `.mike/sms_CHECKLIST-52c148/session_handoffs/handoff_028_stage_transition.md`
