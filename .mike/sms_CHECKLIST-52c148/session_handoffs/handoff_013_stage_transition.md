# Handoff 013 — Stage 4→5 Transition

## What was done

### Stage 4 verification
- All checklist items in `stage_04_checklist.md` confirmed checked off (Steps 1–6 + review fixes)
- Test suites confirmed passing:
  - `go test ./internal/sms/...` — PASS
  - `go test ./internal/cli/...` — PASS
  - `go test ./internal/auth/...` — PASS (unit tests)
  - `go build ./...` — PASS
- `stages.md` updated: stage 4 marked `[x]`

### Stage 5 checklist generated
- Created `checklists/stage_05_checklist.md` with 12 steps covering SMS MFA Second Factor
- Follows TDD RED→GREEN pattern throughout

## What the next stage covers (Stage 5: SMS MFA Second Factor)

**Scope:** Allow users to enroll SMS as a second factor. After first-factor login (password, magic link, or SMS OTP), MFA-enrolled users receive a short-lived "MFA pending" token instead of full access. They must complete an SMS challenge to get full tokens.

**Key deliverables:**
1. Migration 014: `_ayb_user_mfa` table (enrollment tracking)
2. `MFAPending` claim in JWT — short-lived token for MFA gating
3. `RequireAuth` middleware updated to reject MFA pending tokens on normal routes
4. Enrollment flow: `EnrollSMSMFA` → send OTP → `ConfirmSMSMFAEnrollment` → enable
5. Challenge flow: `ChallengeSMSMFA` → send OTP → `VerifySMSMFA` → issue full tokens
6. Login modification: `Login`, `ConfirmMagicLink`, `ConfirmSMSCode` all check for MFA enrollment and return pending token if enrolled
7. Four new handler endpoints: `/mfa/sms/enroll`, `/mfa/sms/enroll/confirm`, `/mfa/sms/challenge`, `/mfa/sms/verify`
8. Integration tests for full MFA flows

**Architecture notes:**
- Reuses existing OTP infrastructure (`_ayb_sms_codes`, bcrypt hashing, `CaptureProvider`)
- MFA pending token: same JWT structure with `mfa_pending: true`, 5-minute expiry
- `RequireMFAPending` middleware (inverse of normal auth) gates challenge/verify endpoints
- Enrollment requires normal auth token; challenge/verify accept only pending tokens

## Files modified this session
- `.mike/sms_CHECKLIST-52c148/stages.md` — marked stage 4 `[x]`
- `.mike/sms_CHECKLIST-52c148/checklists/stage_05_checklist.md` — created

## Files relevant to next stage
- `internal/auth/auth.go` — `Claims` struct (add `MFAPending`), `Service` struct
- `internal/auth/sms_auth.go` — existing OTP helpers to reuse/refactor
- `internal/auth/handler.go` — route wiring, handler enablement
- `internal/auth/middleware.go` — `RequireAuth` (reject pending tokens)
- `internal/auth/oauth.go` — `issueTokens()` pattern
- `internal/auth/magic_link.go` — `ConfirmMagicLink` (gate with MFA check)
- `internal/migrations/sql/` — next is `014_ayb_sms_mfa.sql`
- `internal/auth/auth_integration_test.go` — integration test patterns
