# Handoff 007 — Stage Transition (Stage 3 → Stage 4)

## What was verified

- **Stage 3 checklist**: All items checked off (Steps 1-4: unit tests, service layer, handlers, route wiring)
- **Tests**: `go test ./internal/auth/...` — PASS (cached)
- **Build**: `go build ./...` — PASS
- **Commit**: `feat: add SMS OTP auth service and handlers` already committed (3bb7a01 area)
- **stages.md**: Updated to mark stage 3 as `[x]`

## What stage 4 covers

**Server Wiring & Integration Tests** (Phase 1 Steps 7-9)

1. **CLI wiring** (`internal/cli/start.go`): Add `buildSMSProvider()` function (mirrors `buildMailer`), wire SMS provider + config into auth service when `cfg.Auth.SMSEnabled` is true
2. **Server wiring** (`internal/server/server.go`): Add `authHandler.SetSMSEnabled(true)` when `cfg.Auth.SMSEnabled` (mirrors magic link pattern at line ~206)
3. **Integration tests** (`internal/auth/auth_integration_test.go`): 8 tests covering full OTP flow, code consumption, attempt limits, geo blocking, daily limits — uses `CaptureProvider`
4. **Server-level smoke test**: HTTP-level test through `server.New()` hitting `/api/auth/sms` and `/api/auth/sms/confirm`

### Key implementation notes for next session

- `ConfirmSMSCode` returns `(*User, string, string, error)` not a result struct — the reference impl (`07_integration_tests.go`) uses `result.Token` which needs adaptation to `user, token, refreshToken, err`
- Config fields already exist: `cfg.Auth.SMSEnabled`, `SMSProvider`, `SMSCodeLength`, `SMSCodeExpiry`, `SMSMaxAttempts`, `SMSDailyLimit`, `SMSAllowedCountries`, `TwilioSID`, `TwilioToken`, `TwilioFrom`
- Config validation already validates SMS fields when `SMSEnabled` is true
- `sms.Config` struct lives in `internal/sms/sms.go`, providers in `log.go`, `twilio.go`, `capture.go`
- Integration tests use `//go:build integration`, `package auth_test`, `testutil.StartPostgresForTestMain`, `resetAndMigrate()` pattern
- `setupMagicLinkServer()` at line ~1114 of integration test file is the template for `setupSMSServer()`

## Files created or modified

- Modified: `.mike/sms_CHECKLIST-52c148/stages.md` — marked stage 3 complete
- Created: `.mike/sms_CHECKLIST-52c148/checklists/stage_04_checklist.md` — new stage checklist
- Created: `.mike/sms_CHECKLIST-52c148/session_handoffs/handoff_007_stage_transition.md` — this file
