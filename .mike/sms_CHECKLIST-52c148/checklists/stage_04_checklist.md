# Stage 4: Server Wiring & Integration Tests

## Step 1: CLI Wiring — Unit Tests (RED)

- [x] Add test for `buildSMSProvider` in `internal/cli/start_test.go` (or a new `start_sms_test.go`):
  - `TestBuildSMSProvider_Log`: `sms_provider = "log"` → returns `*sms.LogProvider`
  - `TestBuildSMSProvider_Twilio`: `sms_provider = "twilio"` with SID/token/from → returns `*sms.TwilioProvider`
  - `TestBuildSMSProvider_Default`: empty/unknown → returns `*sms.LogProvider`
- [x] Run `go test ./internal/cli/...` — confirm FAIL (function doesn't exist yet)

## Step 2: CLI Wiring — Implementation (GREEN)

- [x] Add `buildSMSProvider(cfg *config.Config, logger *slog.Logger) sms.Provider` in `internal/cli/start.go`:
  - `case "twilio"`: `sms.NewTwilioProvider(cfg.Auth.TwilioSID, cfg.Auth.TwilioToken, cfg.Auth.TwilioFrom, "")`
  - `default`: `sms.NewLogProvider(logger)`
- [x] Wire SMS into auth service creation block in `start.go` (after mailer wiring, mirrors that pattern)
- [x] Run `go test ./internal/cli/...` — confirm PASS

## Step 3: Server Wiring

- [x] Add SMS handler enablement in `server.go` after the magic link block
- [x] Run `go build ./...` — confirm PASS

## Step 4: Integration Tests (RED)

- [x] Add SMS integration tests to `internal/auth/auth_integration_test.go` (build tag `integration`, package `auth_test`)
- [x] Add `setupSMSService(t *testing.T) (*auth.Service, *sms.CaptureProvider)` helper
- [x] Add import for `"github.com/allyourbase/ayb/internal/sms"` to integration test file
- [x] Tests added:
  - **`TestSMSFullFlow_NewUser`**: request code → capture.LastCode() → confirm → verify user.Phone, placeholder email, tokens non-empty
  - **`TestSMSFullFlow_ExistingUser`**: request+confirm twice same phone → same user ID
  - **`TestSMSCode_ConsumedAfterUse`**: confirm once → second confirm with same code → error
  - **`TestSMSCode_InvalidCodeIncrementsAttempts`**: wrong code → query `_ayb_sms_codes` → attempts = 1
  - **`TestSMSCode_MaxAttemptsDeletesCode`**: 3 wrong codes → code row deleted
  - **`TestSMSCode_NewRequestDeletesOldCode`**: request twice → only 1 code row exists
  - **`TestSMS_GeoBlock`**: UK number `+442079460958` with `AllowedCountries: ["US","CA"]` → no SMS sent, no error
  - **`TestSMS_DailyLimitCircuitBreaker`**: set `DailyLimit: 2` → 3rd request → `ErrDailyLimitExceeded`
- [x] Each test calls `resetAndMigrate(t, t.Context())` to start with clean schema
- [x] Run `go test -tags=integration ./internal/auth/...` — confirm PASS

## Step 5: Server-Level SMS Smoke Test

- [x] Add `TestSMSEndpoints_ServerLevel` in `auth_integration_test.go`:
  - `setupSMSServer(t)` helper mirrors `setupMagicLinkServer` pattern
  - POST `/api/auth/sms` with `{"phone": "+14155552671"}` → 200
  - POST `/api/auth/sms/confirm` with `{"phone": "+14155552671", "code": "<captured>"}` → 200 with token
- [x] Add `TestSMSEndpoints_DisabledReturns404`:
  - POST `/api/auth/sms` when disabled → 404
- [x] Run `go test -tags=integration ./internal/auth/...` — confirm PASS

## Step 6: Build & Test Verification

- [x] Run `go build ./...` — confirm PASS
- [x] Run `go test ./internal/auth/...` — confirm unit tests PASS
- [x] Run `go test ./internal/cli/...` — confirm CLI tests PASS
- [x] Run `go test ./internal/sms/...` — confirm SMS package tests PASS
- [x] Commit: `feat: wire SMS provider into server and CLI startup` (in iteration 9)
- [x] Commit: `test: add SMS OTP integration tests` (in iteration 9)

## Review Fixes (iteration 10)

- [x] **Bug fix**: `ConfirmSMSCode` INSERT missing `email` column — `_ayb_users.email` is NOT NULL, so SMS-only users would fail with constraint violation. Fixed by generating placeholder email `{phone}@sms.local` (mirrors OAuth pattern).
- [x] Added assertion in `TestSMSFullFlow_NewUser` verifying placeholder email.
- [x] Commit: `fix: SMS user creation missing email NOT NULL constraint`


## Master Stages
1. [x] SMS Provider Layer - Provider interface, LogProvider, TwilioProvider, CaptureProvider (Phase 1 Steps 1-3)
2. [x] Database & Config - Migration 013, SMS config fields with validation, startup wiring (Phase 1 Steps 4-5)
3. [x] Auth Service & Handlers - OTP generation, phone normalization, geo check, request/confirm flows, route wiring (Phase 1 Step 6)
4. [x] Server Wiring & Integration Tests - Server/CLI wiring, integration tests, smoke test (Phase 1 Steps 7-9)
5. SMS MFA Second Factor - Migration 014, enroll/challenge endpoints, MFA pending token gating (Phase 2)
6. Provider Expansion - Plivo, Telnyx, Prelude, MSG91, AWS SNS, Vonage, webhook provider, test phone numbers (Phase 3 first half)
7. Fraud Hardening & Monitoring - libphonenumber upgrade, conversion rate monitoring, admin health endpoint (Phase 3 second half)
8. Transactional SMS API - Migration 015, messaging endpoint, delivery status webhook (Phase 4)
