# Stage 2: Database & Config

## Step 1: Database Migration 013

- [x] Copy `_dev/messaging/impl/04_migration.sql` → `internal/migrations/sql/013_ayb_sms.sql`
- [x] Review migration SQL: `_ayb_users.phone` column, `_ayb_sms_codes` table (bcrypt hash, not SHA-256), `_ayb_sms_optouts`, `_ayb_sms_daily_counts`
- [x] Verify migration embeds correctly: `go build ./...` passes (migration runner uses `//go:embed sql/*.sql`)
- [x] Commit: `feat: add migration 013 for SMS OTP`

## Step 2: SMS Config Fields

- [x] RED: Add SMS config tests to `internal/config/config_test.go`
  - Adapt `_dev/messaging/impl/05_config_tests.go` to match existing test style:
    - Use `package config` (not `config_test`) — matches all existing tests in the file
    - Use `testutil.*` helpers (not testify `assert`) — matches existing test patterns
    - `Load("")` in impl becomes `Load("", nil)` (Load takes configPath + flags)
  - Tests to add:
    - `TestSMSConfigDefaults`: verify `SMSEnabled=false`, `SMSProvider="log"`, `SMSCodeLength=6`, `SMSCodeExpiry=300`, `SMSMaxAttempts=3`, `SMSDailyLimit=1000`, `SMSAllowedCountries=["US","CA"]`
    - `TestSMSConfigValidation_RequiresAuthEnabled`: `sms_enabled=true` + `auth.enabled=false` → error containing "sms_enabled requires auth.enabled"
    - `TestSMSConfigValidation_UnknownProvider`: provider `"carrier_pigeon"` → error containing "sms_provider"
    - `TestSMSConfigValidation_TwilioRequiresCredentials`: `twilio` provider without SID → error containing "twilio_sid"
    - `TestSMSConfigValidation_CodeLengthBounds`: 3 → error, 9 → error, 6 → no error
    - `TestSMSConfigValidation_ExpiryBounds`: 59 → error, 601 → error
    - `TestSMSConfigValidation_DailyLimitBounds`: -1 → error, 0 (unlimited) → no error
    - `TestSMSConfigValidation_AllowedCountries`: `["XX"]` → error (must validate against real ISO 3166-1 alpha-2, not just `len==2`)
    - `TestSMSConfigEnvVarOverride`: env vars `AYB_AUTH_SMS_ENABLED`, `AYB_AUTH_TWILIO_SID`, `AYB_AUTH_TWILIO_TOKEN`, `AYB_AUTH_TWILIO_FROM`, `AYB_AUTH_SMS_PROVIDER` override config
    - `validSMSConfig(t)` helper: returns `Default()` with `Auth.Enabled=true`, `SMSEnabled=true`, `SMSProvider="log"`
- [x] Run `go test ./internal/config/...` — confirm FAIL (SMS fields don't exist yet)

## Step 3: SMS Config Implementation

- [x] GREEN: Add SMS fields to `AuthConfig` struct in `internal/config/config.go`:
  - `SMSEnabled bool` with `toml:"sms_enabled"`
  - `SMSProvider string` with `toml:"sms_provider"`
  - `SMSCodeLength int` with `toml:"sms_code_length"`
  - `SMSCodeExpiry int` with `toml:"sms_code_expiry"` (seconds)
  - `SMSMaxAttempts int` with `toml:"sms_max_attempts"`
  - `SMSDailyLimit int` with `toml:"sms_daily_limit"` (0 = unlimited)
  - `SMSAllowedCountries []string` with `toml:"sms_allowed_countries"`
  - `TwilioSID string` with `toml:"twilio_sid"`
  - `TwilioToken string` with `toml:"twilio_token"`
  - `TwilioFrom string` with `toml:"twilio_from"`
- [x] GREEN: Add SMS defaults to `Default()`:
  - `SMSProvider: "log"`, `SMSCodeLength: 6`, `SMSCodeExpiry: 300`, `SMSMaxAttempts: 3`, `SMSDailyLimit: 1000`, `SMSAllowedCountries: []string{"US", "CA"}`
- [x] GREEN: Add SMS validation to `Validate()`:
  - `SMSEnabled && !Enabled` → error
  - `SMSProvider` not in `["twilio", "log"]` → error
  - `SMSProvider == "twilio"` requires `TwilioSID`, `TwilioToken`, `TwilioFrom`
  - `SMSCodeLength` not in 4-8 → error
  - `SMSCodeExpiry` not in 60-600 → error
  - `SMSDailyLimit < 0` → error
  - `SMSAllowedCountries`: each must be a valid ISO 3166-1 alpha-2 code (hardcoded set — `len==2` alone is insufficient, `"XX"` must fail)
- [x] GREEN: Add SMS env var bindings to `applyEnv()`:
  - `AYB_AUTH_SMS_ENABLED` → bool
  - `AYB_AUTH_SMS_PROVIDER` → string
  - `AYB_AUTH_TWILIO_SID` → string
  - `AYB_AUTH_TWILIO_TOKEN` → string
  - `AYB_AUTH_TWILIO_FROM` → string
- [x] GREEN: Add SMS keys to `validKeys` map, `GetValue()`, `coerceValue()`, and `defaultTOML` template
- [x] Run `go test ./internal/config/...` — confirm PASS
- [x] Run `go build ./...` — confirm PASS
- [x] Commit: `feat: add SMS config fields, defaults, and validation`


## Master Stages
1. [x] SMS Provider Layer - Provider interface, LogProvider, TwilioProvider, CaptureProvider (Phase 1 Steps 1-3)
2. Database & Config - Migration 013, SMS config fields with validation, startup wiring (Phase 1 Steps 4-5)
3. Auth Service & Handlers - OTP generation, phone normalization, geo check, request/confirm flows, route wiring (Phase 1 Step 6)
4. Server Wiring & Integration Tests - Server/CLI wiring, integration tests, smoke test (Phase 1 Steps 7-9)
5. SMS MFA Second Factor - Migration 014, enroll/challenge endpoints, MFA pending token gating (Phase 2)
6. Provider Expansion - Plivo, Telnyx, Prelude, MSG91, AWS SNS, Vonage, webhook provider, test phone numbers (Phase 3 first half)
7. Fraud Hardening & Monitoring - libphonenumber upgrade, conversion rate monitoring, admin health endpoint (Phase 3 second half)
8. Transactional SMS API - Migration 015, messaging endpoint, delivery status webhook (Phase 4)
