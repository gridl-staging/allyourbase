# Handoff 002 — Build (Stage 2: Database & Config)

## What I did

Completed all checklist items for Stage 2 (Database & Config).

### Step 1: Migration 013
- Copied `_dev/messaging/impl/04_migration.sql` → `internal/migrations/sql/013_ayb_sms.sql`
- Reviewed: `_ayb_users.phone` column (unique partial index), `_ayb_sms_codes` (bcrypt hash), `_ayb_sms_optouts`, `_ayb_sms_daily_counts`
- Verified `go build ./...` passes (migration embedded via `//go:embed sql/*.sql`)
- Committed: `feat: add migration 013 for SMS OTP`

### Step 2: SMS Config Tests (RED)
- Added 9 tests to `internal/config/config_test.go` using existing style (`package config`, `testutil.*` helpers):
  - `TestSMSConfigDefaults` — verifies all SMS default values
  - `TestSMSConfigValidation_RequiresAuthEnabled` — sms_enabled without auth.enabled fails
  - `TestSMSConfigValidation_UnknownProvider` — "carrier_pigeon" fails
  - `TestSMSConfigValidation_TwilioRequiresCredentials` — twilio without SID fails
  - `TestSMSConfigValidation_CodeLengthBounds` — 3 fails, 9 fails, 6 passes
  - `TestSMSConfigValidation_ExpiryBounds` — 59 fails, 601 fails
  - `TestSMSConfigValidation_DailyLimitBounds` — -1 fails, 0 (unlimited) passes
  - `TestSMSConfigValidation_AllowedCountries` — "XX" fails (validates against ISO 3166-1 alpha-2 set)
  - `TestSMSConfigEnvVarOverride` — AYB_AUTH_SMS_ENABLED, TWILIO_SID/TOKEN/FROM, SMS_PROVIDER
- Added `validSMSConfig(t)` helper (includes JWT secret for auth.enabled requirement)
- Confirmed compile failure (RED phase)

### Step 3: SMS Config Implementation (GREEN)
- Added 11 SMS fields to `AuthConfig` struct (sms_enabled, sms_provider, sms_code_length, sms_code_expiry, sms_max_attempts, sms_daily_limit, sms_allowed_countries, twilio_sid, twilio_token, twilio_from)
- Added SMS defaults in `Default()`: provider="log", code_length=6, expiry=300, max_attempts=3, daily_limit=1000, allowed_countries=["US","CA"]
- Added SMS validation in `Validate()`: requires auth.enabled, provider must be twilio/log, twilio requires credentials, code length 4-8, expiry 60-600, daily_limit >= 0, country codes validated against hardcoded ISO 3166-1 alpha-2 set
- Added `validISO3166Alpha2` map with all 249 standard country codes
- Added 5 SMS env var bindings in `applyEnv()`
- Added SMS keys to `validKeys`, `GetValue()`, `coerceValue()`, and `defaultTOML`
- All 50+ config tests pass, `go build ./...` clean
- Committed: `feat: add SMS config fields, defaults, and validation`

## Test results
- `go test ./internal/config/...` — PASS (all tests)
- `go build ./...` — clean

## Commits
1. `173e863` — `feat: add migration 013 for SMS OTP`
2. `e97086a` — `feat: add SMS config fields, defaults, and validation`

## What's next
Stage 2 checklist is fully complete. Next stage (Stage 3) covers:
- Auth Service & Handlers: OTP generation, phone normalization, geo check, request/confirm flows, route wiring (Phase 1 Step 6)

## Files created or modified
- `internal/migrations/sql/013_ayb_sms.sql` — **created** (migration for SMS tables)
- `internal/config/config.go` — **modified** (AuthConfig SMS fields, defaults, validation, env vars, validKeys, GetValue, coerceValue, defaultTOML, ISO 3166-1 alpha-2 set)
- `internal/config/config_test.go` — **modified** (9 SMS config tests + validSMSConfig helper)
- `.mike/sms_CHECKLIST-52c148/checklists/stage_02_checklist.md` — **modified** (all items checked off)
