# Handoff 016 — Stage 5 Review

## What was done

### Bugs found and fixed: 4

1. **DRY violation: duplicate OTP validation** (`sms_auth.go`)
   - `ConfirmSMSCode` had 50+ lines of inline OTP validation logic identical to `validateSMSCodeForPhone` in `sms_mfa.go`
   - Refactored `ConfirmSMSCode` to call the shared helper `validateSMSCodeForPhone`
   - All existing tests still pass

2. **Silent SMS send failure** (`sms_mfa.go`)
   - `EnrollSMSMFA` and `ChallengeSMSMFA` swallowed SMS provider send errors (logged but returned nil)
   - For explicit user-initiated MFA flows (enrollment, challenge), this means the user thinks the code was sent but it never arrived
   - Fixed both to propagate the send error to the caller

3. **MFA bypass via OptionalAuth** (`middleware.go`)
   - `OptionalAuth` did not filter MFA pending tokens — an MFA pending token would have claims placed in context, treating it as fully authenticated on optional-auth routes
   - Fixed to skip placing claims in context when `claims.MFAPending == true`
   - Added test: `TestOptionalAuth_MFAPendingToken_TreatedAsUnauthenticated`

4. **TDD violation: implementation ahead of tests**
   - Steps 5 and 7 implementation code (EnrollSMSMFA, ConfirmSMSMFAEnrollment, ChallengeSMSMFA, VerifySMSMFA, validateSMSCodeForPhone, HasSMSMFA) was written in the previous build session without corresponding tests from Steps 4 and 6
   - Updated checklist to accurately mark what's done vs what's not
   - Steps 4 and 6 tests are still needed (integration-style tests requiring a DB)

### Tests verified passing
- `go test ./internal/auth/...` — PASS (all unit tests)
- `go test ./internal/sms/...` — PASS
- `go build ./...` — PASS

## What's next (remaining Stage 5 work)

Steps still unchecked:
- **Step 4**: Write MFA enrollment tests (integration tests with DB) — `TestEnrollSMSMFA_*`, `TestConfirmSMSMFAEnrollment_*`
- **Step 6**: Write MFA challenge/verify tests — `TestLogin_WithMFA_*`, `TestChallengeSMSMFA_*`, `TestVerifySMSMFA_*`
- **Step 7 (partial)**: Login gating — modify `Login()`, `ConfirmMagicLink()`, `ConfirmSMSCode()` to return pending token for MFA-enrolled users
- **Steps 8-9**: MFA handler endpoints and route wiring
- **Step 10**: Login response shape update
- **Step 11**: Integration tests
- **Step 12**: Build & test verification, commits

Key architectural note: The service-level functions (EnrollSMSMFA, ConfirmSMSMFAEnrollment, ChallengeSMSMFA, VerifySMSMFA, HasSMSMFA) are all implemented. What's missing is:
1. Tests for those functions
2. Login flow gating (Login/ConfirmMagicLink/ConfirmSMSCode returning pending token when MFA enrolled)
3. HTTP handler layer
4. Route wiring

## Files modified this session
- `internal/auth/sms_auth.go` — refactored `ConfirmSMSCode` to use `validateSMSCodeForPhone` (DRY fix)
- `internal/auth/sms_mfa.go` — `EnrollSMSMFA` and `ChallengeSMSMFA` now return SMS send errors
- `internal/auth/middleware.go` — `OptionalAuth` now filters MFA pending tokens
- `internal/auth/middleware_test.go` — added `TestOptionalAuth_MFAPendingToken_TreatedAsUnauthenticated`
- `.mike/sms_CHECKLIST-52c148/checklists/stage_05_checklist.md` — updated to reflect actual state, added review fixes section

## Files relevant to next work
- `internal/auth/sms_mfa.go` — all MFA service methods
- `internal/auth/sms_mfa_test.go` — needs enrollment and challenge tests added
- `internal/auth/auth.go` — `Login()` needs MFA gating
- `internal/auth/magic_link.go` — `ConfirmMagicLink()` needs MFA gating
- `internal/auth/sms_auth.go` — `ConfirmSMSCode()` needs MFA gating
- `internal/auth/handler.go` — needs MFA handler endpoints and routes
- `internal/auth/auth_integration_test.go` — MFA integration test patterns
