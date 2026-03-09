# Stage 5: SMS MFA Second Factor

## Step 1: Migration 014 — MFA Enrollment Table

- [x] Create `internal/migrations/sql/014_ayb_sms_mfa.sql`:
  - `_ayb_user_mfa (id UUID PK DEFAULT gen_random_uuid(), user_id UUID NOT NULL REFERENCES _ayb_users(id) ON DELETE CASCADE, method TEXT NOT NULL DEFAULT 'sms', phone TEXT NOT NULL, enabled BOOLEAN NOT NULL DEFAULT false, enrolled_at TIMESTAMPTZ, created_at TIMESTAMPTZ NOT NULL DEFAULT now())`
  - Unique constraint on `(user_id, method)` — one MFA method per type per user
  - Index on `user_id`
- [x] Verify migration applies cleanly against test DB
- [x] Commit: `feat: add migration 014 for SMS MFA enrollment`

## Step 2: MFA Claims & Pending Token — Unit Tests (RED)

- [x] Add `MFAPending bool` field to `Claims` struct in `auth.go` (`json:"mfa_pending,omitempty"`)
- [x] Add `generateMFAPendingToken(user *User) (string, error)` to `Service` — issues a short-lived JWT (5 min) with `mfa_pending: true`
- [x] Add test in `sms_mfa_test.go` (package `auth`):
  - `TestGenerateMFAPendingToken`: verify token parses, has `mfa_pending: true`, expires in ≤5 min, subject = user ID
  - `TestMFAPendingToken_RejectedByRequireAuth`: middleware should reject tokens with `mfa_pending: true` on normal routes (401)
- [x] Run `go test ./internal/auth/...` — confirm FAIL

## Step 3: MFA Claims & Pending Token — Implementation (GREEN)

- [x] Implement `generateMFAPendingToken` in `sms_mfa.go`: same as `generateToken` but sets `MFAPending: true` and `ExpiresAt: now + 5min`
- [x] Update `RequireAuth` middleware in `middleware.go`: after validating token, if `claims.MFAPending == true`, return 401 with error `"MFA verification required"`
- [x] Run `go test ./internal/auth/...` — confirm PASS

## Step 4: MFA Enrollment — Integration Tests

- [x] Add tests in `auth_integration_test.go` (integration tests, require real DB):
  - `TestEnrollSMSMFA_Success`: authenticated user → `EnrollSMSMFA(ctx, userID, phone)` → sends OTP to phone, creates `_ayb_user_mfa` row with `enabled=false`
  - `TestEnrollSMSMFA_InvalidPhone`: bad phone → error
  - `TestEnrollSMSMFA_AlreadyEnrolled`: user already has enabled SMS MFA → error `"SMS MFA already enrolled"`
  - `TestConfirmSMSMFAEnrollment_Success`: correct code → `_ayb_user_mfa.enabled = true`, `enrolled_at` set
  - `TestConfirmSMSMFAEnrollment_WrongCode`: wrong code → error, enrollment stays `enabled=false`
  - `TestHasSMSMFA_NotEnrolled`: user without MFA → false
  - `TestEnrollSMSMFA_ReEnrollAfterDisabledReset`: re-enroll after unconfirmed attempt
- [x] Run `go test -tags=integration ./internal/auth/...` — confirm PASS

## Step 5: MFA Enrollment — Implementation (GREEN)

- [x] Create `internal/auth/sms_mfa.go` with:
  - `EnrollSMSMFA(ctx context.Context, userID, phone string) error`:
    - Normalize phone
    - Check no existing enabled enrollment for this user+method
    - Insert `_ayb_user_mfa` row with `enabled=false`
    - Generate OTP, hash with bcrypt, store in `_ayb_sms_codes` (reuses existing OTP infra)
    - Send OTP via SMS provider
  - `ConfirmSMSMFAEnrollment(ctx context.Context, userID, phone, code string) error`:
    - Validate OTP against `_ayb_sms_codes` (reuse `validateSMSCodeForPhone` shared helper)
    - On success: `UPDATE _ayb_user_mfa SET enabled = true, enrolled_at = now() WHERE user_id = $1 AND method = 'sms'`
  - `HasSMSMFA(ctx context.Context, userID string) (bool, error)`:
    - Query `_ayb_user_mfa` for `user_id = $1 AND method = 'sms' AND enabled = true`
  - `var ErrMFAAlreadyEnrolled = errors.New("SMS MFA already enrolled")`
  - `validateSMSCodeForPhone` shared helper (also used by `ConfirmSMSCode` in `sms_auth.go`)
- [x] Run `go test ./internal/auth/...` — confirm PASS

## Step 6: MFA Challenge on Login — Integration Tests

- [x] Add tests in `auth_integration_test.go`:
  - `TestLogin_WithMFA_ReturnsPendingToken`: user with MFA enrolled → `Login()` returns `mfa_pending` token instead of full token, no refresh token
  - `TestLogin_WithMFA_FullFlowEndToEnd`: login → pending token → challenge → verify → full tokens
  - `TestLogin_WithoutMFA_ReturnsNormalTokens`: user without MFA → normal tokens
  - `TestChallengeSMSMFA_Success`: `ChallengeSMSMFA(ctx, userID)` sends OTP to enrolled phone
  - `TestVerifySMSMFA_Success`: correct code → issues full tokens (access + refresh)
  - `TestVerifySMSMFA_WrongCode`: wrong code → error, no tokens issued
- [x] Run `go test -tags=integration ./internal/auth/...` — confirm PASS

## Step 7: MFA Challenge on Login — Implementation (GREEN)

- [x] Implement `ChallengeSMSMFA(ctx context.Context, userID string) error`:
    - Look up enrolled phone from `_ayb_user_mfa`
    - Generate OTP → hash → store in `_ayb_sms_codes` → send via provider
- [x] Implement `VerifySMSMFA(ctx context.Context, userID, code string) (*User, string, string, error)`:
    - Look up enrolled phone from `_ayb_user_mfa`
    - Validate OTP from `_ayb_sms_codes`
    - On success: `issueTokens()` → return full access + refresh tokens
- [x] Modify `Login()` in `auth.go`: after password validation, check `HasSMSMFA(ctx, user.ID)` — if true, return `generateMFAPendingToken(user)` instead of `issueTokens(user)`. Return a distinct response shape: `{mfa_pending: true, mfa_token: "..."}` with no refresh token.
- [x] Also gate magic link and SMS OTP first-factor login: if user has MFA enrolled, `ConfirmMagicLink` and `ConfirmSMSCode` should return pending token instead of full tokens
- [x] Run `go test ./internal/auth/...` — confirm PASS

## Step 8: MFA Handlers — Tests

- [x] Add handler integration tests in `auth_integration_test.go`:
  - `TestHandleMFAEnroll_Success`: POST `/api/auth/mfa/sms/enroll` with `{"phone": "+14155552671"}` + valid auth token → 200
  - `TestHandleMFAEnroll_Unauthenticated`: no token → 401
  - `TestHandleMFAEnrollConfirm_Success`: POST `/api/auth/mfa/sms/enroll/confirm` with `{"phone": "+14155552671", "code": "123456"}` + auth token → 200
  - `TestHandleMFAChallenge_Success`: POST `/api/auth/mfa/sms/challenge` with valid MFA pending token → 200 (sends OTP)
  - `TestHandleMFAChallenge_NotPendingToken`: regular token → 401 `"no MFA challenge pending"`
  - `TestHandleMFAVerify_Success`: POST `/api/auth/mfa/sms/verify` with `{"code": "123456"}` + pending token → 200 with full tokens
  - `TestHandleMFAVerify_WrongCode`: wrong code → 401
  - `TestHandleMFA_DisabledReturns404`: all MFA endpoints return 404 when SMS not enabled
- [x] All tests PASS

## Step 9: MFA Handlers — Implementation (GREEN)

- [x] Handler methods in `sms_mfa_handlers.go`:
  - `handleMFAEnroll(w, r)` — requires auth, calls `EnrollSMSMFA`
  - `handleMFAEnrollConfirm(w, r)` — requires auth, calls `ConfirmSMSMFAEnrollment`
  - `handleMFAChallenge(w, r)` — accepts MFA pending token (special middleware), calls `ChallengeSMSMFA`
  - `handleMFAVerify(w, r)` — accepts MFA pending token, calls `VerifySMSMFA`, returns full tokens
- [x] `RequireMFAPending` middleware in `middleware.go`: validates token, requires `mfa_pending: true`
- [x] Routes wired in `Routes()` with `requireSMSEnabled` middleware gating
- [x] All tests PASS

## Step 10: Login Response Shape Update

- [x] `handleLogin` returns `{"mfa_pending": true, "mfa_token": "..."}` for MFA users
- [x] `handleMagicLinkConfirm` and `handleSMSConfirm` return same shape for MFA users
- [x] `handleOAuthCallback` returns same shape for MFA users (all 3 paths: SSE, redirect, JSON)
- [x] Tests: `TestHandleLogin_WithMFA_ReturnsMFAResponse`, `TestHandleLogin_WithoutMFA_ReturnsNormalResponse`
- [x] All tests PASS

## Step 11: Integration Tests

- [x] MFA integration tests in `auth_integration_test.go`:
  - `TestEnrollSMSMFA_*` (4 tests): enroll success, invalid phone, already enrolled, re-enroll
  - `TestConfirmSMSMFAEnrollment_*` (2 tests): success, wrong code
  - `TestHasSMSMFA_NotEnrolled`: not enrolled returns false
  - `TestChallengeSMSMFA_Success`: challenge sends OTP
  - `TestVerifySMSMFA_*` (2 tests): success, wrong code
  - `TestLogin_WithMFA_*` (2 tests): pending token, full E2E flow
  - `TestLogin_WithoutMFA_ReturnsNormalTokens`: regression guard
  - `TestConfirmMagicLink_WithMFA_ReturnsPendingToken`: magic link MFA gating
  - `TestConfirmSMSCode_WithMFA_ReturnsPendingToken`: SMS first-factor MFA gating
  - `TestHandleMFA*` (8 tests): handler-level tests for all MFA endpoints
- [x] All tests PASS

## Step 12: Build & Test Verification

- [x] Run `go build ./...` — confirm PASS
- [x] Run `go test ./internal/auth/...` — confirm unit tests PASS
- [x] Run `go test ./internal/sms/...` — confirm SMS package tests PASS
- [x] Run `go test ./internal/cli/...` — confirm CLI tests PASS
- [x] Commit: `feat: add SMS MFA second factor enrollment and challenge`
- [x] Commit: `test: add SMS MFA integration tests`


## Review 019 Fixes

- [x] **BUG: OAuth login bypasses MFA (SECURITY)** — `loginByID()` in `oauth.go` called `issueTokens()` directly without checking `HasSMSMFA()`. Any MFA-enrolled user logging in via OAuth (Google, GitHub, etc.) got full tokens without MFA challenge. Fixed by adding MFA gating to `loginByID()`. Also updated `OAuthEvent` struct and `handleOAuthCallback` handler to return MFA pending response shape for SSE, redirect, and JSON paths.
- [x] **BUG: HasSMSMFA errors silently swallowed (SECURITY)** — In `Login()`, `ConfirmSMSCode()`, and `ConfirmMagicLink()`, the pattern `if hasMFA, err := ...; err == nil && hasMFA` silently ignores DB errors, allowing login to proceed without MFA during transient DB failures. Fixed to propagate errors (return error instead of silently proceeding).
- [x] **BUG: DRY violation in OTP generation** — `EnrollSMSMFA()` and `ChallengeSMSMFA()` each had ~20 lines of duplicated OTP generate/hash/store/send logic. Extracted shared helpers `sendOTPToPhone()` and `mfaEnrolledPhone()`.

## Review 016 Fixes

- [x] **BUG: DRY violation** — `ConfirmSMSCode` in `sms_auth.go` had inline OTP validation logic (50+ lines) duplicated from `validateSMSCodeForPhone` in `sms_mfa.go`. Refactored `ConfirmSMSCode` to call the shared helper.
- [x] **BUG: Silent SMS send failure** — `EnrollSMSMFA` and `ChallengeSMSMFA` swallowed SMS send errors (logged but returned nil). For explicit user-initiated MFA actions, this means user thinks code was sent but it wasn't. Fixed to propagate errors.
- [x] **BUG: MFA bypass via OptionalAuth** — `OptionalAuth` middleware did not filter MFA pending tokens. An MFA pending token would be treated as fully authenticated on optional-auth routes. Fixed to skip placing claims in context for MFA pending tokens. Added test `TestOptionalAuth_MFAPendingToken_TreatedAsUnauthenticated`.
- [x] **NOTE: TDD violation** — Steps 5 and 7 implementation was written ahead of Steps 4 and 6 tests. Checklist updated to reflect actual state. Steps 4 and 6 tests still need to be written (integration-style, require DB).

## Master Stages
1. [x] SMS Provider Layer - Provider interface, LogProvider, TwilioProvider, CaptureProvider (Phase 1 Steps 1-3)
2. [x] Database & Config - Migration 013, SMS config fields with validation, startup wiring (Phase 1 Steps 4-5)
3. [x] Auth Service & Handlers - OTP generation, phone normalization, geo check, request/confirm flows, route wiring (Phase 1 Step 6)
4. [x] Server Wiring & Integration Tests - Server/CLI wiring, integration tests, smoke test (Phase 1 Steps 7-9)
5. SMS MFA Second Factor - Migration 014, enroll/challenge endpoints, MFA pending token gating (Phase 2)
6. Provider Expansion - Plivo, Telnyx, Prelude, MSG91, AWS SNS, Vonage, webhook provider, test phone numbers (Phase 3 first half)
7. Fraud Hardening & Monitoring - libphonenumber upgrade, conversion rate monitoring, admin health endpoint (Phase 3 second half)
8. Transactional SMS API - Migration 015, messaging endpoint, delivery status webhook (Phase 4)
