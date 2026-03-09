# Handoff 008 — Review (Stage 4)

## What was reviewed

Full review of all SMS OTP code from stages 1-3:

- **Stage 1 (SMS Provider Layer)**: `internal/sms/sms.go`, `log.go`, `twilio.go`, `capture.go` — clean, no issues
- **Stage 2 (Database & Config)**: `internal/migrations/sql/013_ayb_sms.sql`, `internal/config/config.go` config fields + validation — clean
- **Stage 3 (Auth Service & Handlers)**: `internal/auth/sms_auth.go`, `handler.go`, `handler_test.go`, `sms_auth_test.go` — **2 bugs found and fixed**

## Bugs found and fixed

### Bug 1: Off-by-one in max attempts deletion (CRITICAL)
**File**: `internal/auth/sms_auth.go:207-214`
**Problem**: After `maxAttempts` (default 3) failed bcrypt compares, the code row was only updated (attempts incremented) but never deleted. The `attempts >= maxAttempts` guard at line 202 only triggered on attempt N+1, meaning the code row lingered in the DB after exhausting all attempts.
**Fix**: After a failed bcrypt compare, check `attempts+1 >= maxAttempts`. If true, DELETE the row instead of updating. This ensures cleanup happens on the last allowed attempt.
**Impact**: Without this fix, the integration test `TestSMSCode_MaxAttemptsDeletesCode` (stage 4) would fail. Stale code rows could accumulate in production.

### Bug 2: Dead code in handleSMSConfirm
**File**: `internal/auth/sms_auth.go:333-336`
**Problem**: `handleSMSConfirm` checked for `ErrDailyLimitExceeded`, but `ConfirmSMSCode` never returns that error (it only returns `ErrInvalidSMSCode` or wrapped DB errors). Dead code path could never execute.
**Fix**: Removed the unreachable `ErrDailyLimitExceeded` check from the confirm handler.

## Design notes (not bugs, flagged for awareness)

1. **Country dial code coverage**: `countryDialCode` map in `sms_auth.go` covers ~35 countries, but config validation accepts all ~250 ISO 3166-1 codes. A user could configure `sms_allowed_countries = ["VN"]` (valid ISO code, passes config validation) but Vietnamese numbers would be silently geo-blocked because "VN" isn't in the dial code map. Consider adding a config-time validation against supported codes, or expanding the dial code map.

2. **TOCTOU in daily limit check**: `RequestSMSCode` reads the daily count, then later increments it. Concurrent requests could both pass the check. Acceptable for a circuit breaker but worth noting.

## Tests run

- `go test ./internal/sms/...` — PASS
- `go test ./internal/auth/...` — PASS
- `go build ./...` — PASS

## Commit

- `a9df5e1` — `fix: SMS OTP max-attempts off-by-one and dead code cleanup`

## Stage 4 checklist status

All items remain unchecked — stage 4 work (CLI wiring, server wiring, integration tests) has not started yet. The review was of stages 1-3 code.

## Files modified

- `internal/auth/sms_auth.go` — fixed off-by-one in max attempts logic, removed dead code in confirm handler

## Files reviewed (no changes needed)

- `internal/sms/sms.go`, `log.go`, `twilio.go`, `capture.go`, `sms_test.go`, `twilio_test.go`
- `internal/auth/auth.go` (SMS-related fields and methods)
- `internal/auth/handler.go` (SMS route registration, handler struct)
- `internal/auth/handler_test.go` (existing handler tests)
- `internal/auth/sms_auth_test.go` (unit tests for OTP gen, phone normalization, geo check, handlers)
- `internal/config/config.go` (SMS config fields, defaults, validation)
- `internal/migrations/sql/013_ayb_sms.sql` (migration schema)

## Checklist and handoff file paths

- Checklist: `.mike/sms_CHECKLIST-52c148/checklists/stage_04_checklist.md`
- This handoff: `.mike/sms_CHECKLIST-52c148/session_handoffs/handoff_008_review.md`
- Previous handoff: `.mike/sms_CHECKLIST-52c148/session_handoffs/handoff_007_stage_transition.md`
