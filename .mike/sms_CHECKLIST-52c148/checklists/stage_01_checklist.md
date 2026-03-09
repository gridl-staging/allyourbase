# Stage 1: SMS Provider Layer

## Step 1: Provider Interface + LogProvider

- [x] Create `internal/sms/` directory
- [x] RED: Copy `impl/01_provider_tests.go` → `internal/sms/sms_test.go` (tests for SendResult fields, LogProvider.Send, LogProvider implements Provider interface)
- [x] Run `go test ./internal/sms/...` — confirm FAIL (package doesn't exist)
- [x] GREEN: Create `internal/sms/sms.go` with `SendResult` struct (MessageID, Status), `Provider` interface (`Send(ctx, to, body) (*SendResult, error)`), and `Config` struct (CodeLength, Expiry, MaxAttempts, DailyLimit, AllowedCountries)
- [x] GREEN: Create `internal/sms/log.go` with `LogProvider` struct, `NewLogProvider(logger *slog.Logger)` constructor (nil-safe default), `Send()` returning `&SendResult{Status: "logged"}`
- [x] Run `go test ./internal/sms/...` — confirm PASS
- [x] Commit: `feat: add sms.Provider interface (with SendResult) and LogProvider`

## Step 2: TwilioProvider

- [x] RED: Copy `impl/02_twilio_tests.go` → `internal/sms/twilio_test.go` (tests for Send success with httptest mock validating auth/method/path/params, Send HTTP error, TwilioProvider implements Provider interface)
- [x] Run `go test ./internal/sms/...` — confirm FAIL (TwilioProvider doesn't exist)
- [x] GREEN: Create `internal/sms/twilio.go` with `TwilioProvider` struct (accountSID, authToken, fromNumber, baseURL, http.Client), `NewTwilioProvider()` constructor (defaults baseURL to Twilio API), `Send()` using form-encoded POST with Basic auth, JSON response parsing for sid/status/code/message, error on status >= 300
- [x] Run `go test ./internal/sms/...` — confirm PASS
- [x] Commit: `feat: add TwilioProvider`

## Step 3: CaptureProvider (Test Helper)

- [x] Copy `impl/03_capture_provider.go` → `internal/sms/capture.go` with `CaptureProvider` struct (mutex-protected Calls slice), `CaptureCall` struct (To, Body), `Send()` appending to Calls, `LastCode()` extracting 4-8 digit OTP via regex, `Reset()` clearing calls
- [x] Run `go test ./internal/sms/...` — confirm still PASS (no regressions)
- [x] Commit: `feat: add sms.CaptureProvider for integration tests`
