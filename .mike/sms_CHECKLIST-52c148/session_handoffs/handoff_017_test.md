# Handoff 017 — Stage 5 Test

## What was done

### Tests written: 13 new integration tests
All tests pass against real Postgres via `go run ./internal/testutil/cmd/testpg`.

#### Step 4: MFA Enrollment (7 tests)
1. `TestEnrollSMSMFA_Success` — enrolls user, verifies OTP sent, `_ayb_user_mfa` row created with `enabled=false`
2. `TestEnrollSMSMFA_InvalidPhone` — bad phone returns `ErrInvalidPhoneNumber`
3. `TestEnrollSMSMFA_AlreadyEnrolled` — second enrollment after confirmation returns `ErrMFAAlreadyEnrolled`
4. `TestConfirmSMSMFAEnrollment_Success` — correct code sets `enabled=true`, `enrolled_at` non-nil, `HasSMSMFA` returns true
5. `TestConfirmSMSMFAEnrollment_WrongCode` — wrong code returns `ErrInvalidSMSCode`, enrollment stays `enabled=false`
6. `TestHasSMSMFA_NotEnrolled` — user without enrollment returns false
7. `TestEnrollSMSMFA_ReEnrollAfterDisabledReset` — re-enrollment after unconfirmed attempt succeeds via upsert

#### Step 6: MFA Challenge + Login Gating (6 tests)
8. `TestChallengeSMSMFA_Success` — sends OTP to enrolled phone
9. `TestVerifySMSMFA_Success` — correct code issues full (non-pending) tokens
10. `TestVerifySMSMFA_WrongCode` — wrong code returns `ErrInvalidSMSCode`
11. `TestLogin_WithMFA_ReturnsPendingToken` — **RED→GREEN**: Login with MFA-enrolled user returns `MFAPending` token, no refresh token
12. `TestLogin_WithMFA_FullFlowEndToEnd` — **RED→GREEN**: full login→pending→challenge→verify→full tokens flow
13. `TestLogin_WithoutMFA_ReturnsNormalTokens` — non-MFA user gets normal tokens (regression guard)

### Implementation: Login MFA gating (Step 7 partial)
- Modified `Login()` in `auth.go`: after password verification, checks `HasSMSMFA(ctx, user.ID)`. If true, returns `generateMFAPendingToken(&user)` with empty refresh token instead of `issueTokens`.
- This was the RED→GREEN step for tests 11 and 12.

### Verification
- `go test ./internal/auth/...` — PASS (unit tests)
- `go test -tags=integration ./internal/auth/...` — PASS (all integration tests including 13 new ones)
- `go build ./...` — PASS

### Bugs found: 0
No bugs discovered in this session. Previous session's review fixes are all still working correctly.

## What's next (remaining Stage 5 work)

### Step 7 (remaining):
- [ ] Gate `ConfirmMagicLink` in `magic_link.go` — if user has MFA, return pending token
- [ ] Gate `ConfirmSMSCode` in `sms_auth.go` — if user has MFA, return pending token
- [ ] Add tests for magic link and SMS first-factor MFA gating

### Steps 8-9: MFA HTTP Handlers
- [ ] `handleMFAEnroll`, `handleMFAEnrollConfirm`, `handleMFAChallenge`, `handleMFAVerify`
- [ ] `RequireMFAPending` middleware
- [ ] Route wiring in `Routes()`
- [ ] Gate behind `smsEnabled`

### Step 10: Login Response Shape
- [ ] `handleLogin` returns `{mfa_pending: true, mfa_token: "..."}` for MFA users
- [ ] Same for `handleMagicLinkConfirm` and `handleSMSConfirm`

### Step 11: Full MFA Integration Tests
- [ ] End-to-end HTTP-level integration tests (handler layer)

### Step 12: Build & Test Verification + Commits

## Files modified this session
- `internal/auth/auth.go` — added MFA gating to `Login()` (lines 216-223)
- `internal/auth/auth_integration_test.go` — added 13 MFA integration tests (helpers + test functions)
- `.mike/sms_CHECKLIST-52c148/checklists/stage_05_checklist.md` — marked Steps 4, 6, and 7 (Login gating) as done

## Files relevant to next work
- `internal/auth/magic_link.go` — `ConfirmMagicLink()` needs MFA gating (Step 7 remaining)
- `internal/auth/sms_auth.go` — `ConfirmSMSCode()` needs MFA gating (Step 7 remaining)
- `internal/auth/handler.go` — needs MFA handler endpoints and routes (Steps 8-10)
- `internal/auth/middleware.go` — needs `RequireMFAPending` middleware (Step 9)
- `internal/auth/auth_integration_test.go` — add handler-level integration tests (Step 11)
