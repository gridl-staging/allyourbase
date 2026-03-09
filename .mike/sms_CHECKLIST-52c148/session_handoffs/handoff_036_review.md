# Handoff 036 — Stage 8 Review

## What was done

### Review of iteration 35 (build) work
Reviewed all code and tests written in the previous session for Steps 1-3 of Stage 8 (Transactional SMS API).

### Bugs Found and Fixed

**BUG 1 — BUILD BROKEN (critical):** `internal/cli/start.go:323` — Previous session renamed
local variable `p` to `smsProvider` (`p := buildSMSProvider(...)` → `smsProvider = buildSMSProvider(...)`)
without declaring `smsProvider` as a variable. This caused a compilation error:
```
internal/cli/start.go:323:4: undefined: smsProvider
```
**Fix:** Added `var smsProvider sms.Provider` declaration before the auth service block.

**BUG 2 — MISSING WIRING (critical):** `srv.SetSMSProvider(...)` was never called after
`srv := server.New(...)`. The `SetSMSProvider` method and server struct fields were added correctly,
but the wiring in `start.go` to actually connect the SMS provider to the server was missing entirely.
This would have made the entire transactional SMS API non-functional (always returning 404 "SMS disabled").
**Fix:** Added `srv.SetSMSProvider(cfg.Auth.SMSProvider, smsProvider, cfg.Auth.SMSAllowedCountries)` call
after server creation, guarded by `smsProvider != nil`.

### Code Quality Verification
- Verified `sms/phone.go` correctly exports `NormalizePhone`, `PhoneCountry`, `IsAllowedCountry`, `ErrInvalidPhoneNumber`
- Verified `auth/sms_auth.go` wrappers delegate properly to `sms.*` functions
- Verified `auth/sms_mfa.go` calls `sms.NormalizePhone()` directly (not through wrapper)
- Verified `auth.ErrInvalidPhoneNumber = sms.ErrInvalidPhoneNumber` aliasing works with `errors.Is()`
- Verified migration 016 SQL is correct (NOT NULL user_id, nullable api_key_id, compound index)
- Verified `Server.SetSMSProvider` method stores provider, name, and allowed countries correctly
- Verified no circular dependency: `auth` → `sms` import direction is correct

### Test Verification
All tests pass after fix:
- `go build ./...` — PASS
- `go test ./internal/sms/...` — PASS (phone util + all provider tests)
- `go test ./internal/auth/...` — PASS (phone wrappers, SMS handlers, MFA, all existing tests)
- `go test ./internal/server/...` — PASS
- `go test ./internal/config/...` — PASS

### Test Quality Assessment
- Phone tests in `sms/phone_test.go` properly validate: E.164 normalization, invalid input rejection,
  country detection (NANP disambiguation), country allowlist logic
- Auth tests in `auth/sms_auth_test.go` test same logic through wrappers — intentionally retained as
  regression safety net for the delegation layer
- No false positives found — all tests verify actual behavior with proper assertions

## Files Modified
- `internal/cli/start.go` — fixed undeclared `smsProvider` var, added `srv.SetSMSProvider()` call
- `.mike/sms_CHECKLIST-52c148/checklists/stage_08_checklist.md` — checked off Steps 1-3, documented bug fix
- `_dev/messaging/impl/sms_CHECKLIST.md` — added progress note for Phase 4

## Files Created
- `.mike/sms_CHECKLIST-52c148/session_handoffs/handoff_036_review.md` (this file)

## What's Next
- Stage 8 Steps 4-8 remain (the actual transactional SMS API):
  - Step 4: Send SMS endpoint (TDD) — `POST /api/messaging/sms/send`
  - Step 5: Message history endpoints (TDD) — `GET /messages`, `GET /messages/{id}`
  - Step 6: Delivery status webhook (TDD) — `POST /api/webhooks/sms/status`
  - Step 7: Wire messaging routes into server
  - Step 8: Build & test verification
- Key context for implementer:
  - Server now has `smsProvider`, `smsProviderName`, `smsAllowedCountries` fields (properly wired)
  - `auth.ClaimsFromContext(r.Context())` gets user claims, `auth.CheckWriteScope(claims)` checks write permission
  - `sms.NormalizePhone()`, `sms.IsAllowedCountry()` available for phone validation
  - Migration 016 creates `_ayb_sms_messages` table (auto-discovered by migration runner)
  - Auth middleware: `auth.RequireAuth(authSvc)` handles JWT/API key validation
