# Handoff 022 — Stage 6 Review (Pre-Implementation)

## What was done

### Full code review of stages 1-5 SMS infrastructure
Reviewed all files in `internal/sms/`, `internal/auth/sms_*.go`, `internal/auth/sms_mfa*.go`, `internal/config/config.go`, and `internal/cli/start.go`.

### Bugs found and fixed: 2

1. **DRY violation: RequestSMSCode duplicated sendOTPToPhone** (`internal/auth/sms_auth.go`)
   - `RequestSMSCode` had ~35 lines of inline OTP generation, bcrypt hashing, DB insertion, and SMS sending — identical logic to the `sendOTPToPhone` helper in `sms_mfa.go`
   - Previous review (016) fixed a similar DRY violation in `ConfirmSMSCode` but missed this one
   - Refactored `RequestSMSCode` to call `sendOTPToPhone`, keeping only the daily-limit and country-check logic that's unique to it
   - Removed unused `bcrypt` and `time` imports

2. **Missing Twilio network error test** (`internal/sms/twilio_test.go`)
   - Only success and HTTP error cases were tested; no test for unreachable server
   - Added `TestTwilioSendNetworkError`: points at `127.0.0.1:1`, verifies wrapped error message

### Tests verified passing
- `go test ./internal/sms/...` — PASS
- `go test ./internal/auth/...` — PASS (all unit tests)
- `go build ./...` — PASS

### Other observations (no fix needed)
- `LogProvider.Send` returns empty `MessageID` — this is fine because callers discard the result on the send path
- `RequestSMSCode` returns nil even on SMS send failure — intentional anti-enumeration behavior for the primary auth flow
- `CaptureProvider` properly uses mutex for thread safety
- Config validation correctly constrains SMS settings (code length 4-8, expiry 60-600s, etc.)
- MFA pending token is properly rejected by `RequireAuth` and `OptionalAuth` (fixed in review 016)

## What's next
Stage 6 implementation: all 10 steps are unchecked. Start with Step 1 (Plivo Provider TDD).

## Files modified
- `internal/auth/sms_auth.go` — DRY refactor: `RequestSMSCode` now calls `sendOTPToPhone`
- `internal/sms/twilio_test.go` — added `TestTwilioSendNetworkError`

## Checklist paths
- Stage 6 checklist: `.mike/sms_CHECKLIST-52c148/checklists/stage_06_checklist.md`
- Previous handoff: `.mike/sms_CHECKLIST-52c148/session_handoffs/handoff_021_stage_transition.md`
- This handoff: `.mike/sms_CHECKLIST-52c148/session_handoffs/handoff_022_review.md`
