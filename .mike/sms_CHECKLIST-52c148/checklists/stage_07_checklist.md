# Stage 7: Fraud Hardening & Monitoring

## Step 1: Add nyaruka/phonenumbers Dependency

- [x] `go get github.com/nyaruka/phonenumbers`
- [x] `go mod tidy`
- [x] Verify import works: quick sanity check in a test

## Step 2: Upgrade Phone Normalization with phonenumbers (TDD)

- [x] RED: Add new test cases to `internal/auth/sms_auth_test.go`:
  - `TestNormalizePhone_LibPhoneNumber`: valid numbers formatted in various ways (national, international, with extensions) → all normalize to E.164
  - `TestNormalizePhone_RejectsInvalidForCountry`: numbers that match E.164 digit pattern but are invalid for their country (e.g., `+1999555000` — invalid US area code) → error
  - `TestNormalizePhone_AcceptsGlobalNumbers`: valid numbers from many countries (India, Japan, Brazil, Nigeria, etc.) → correct E.164
  - Existing `TestNormalizePhone` and `TestNormalizePhoneRejectsInvalid` must still pass
- [x] GREEN: Replace `normalizePhone()` in `internal/auth/sms_auth.go`:
  - Use `phonenumbers.Parse(input, "")` (empty default region — require E.164 `+` prefix)
  - Validate with `phonenumbers.IsValidNumber()`
  - Format output with `phonenumbers.Format(num, phonenumbers.E164)`
  - Remove manual stripping/digit-counting logic
- [x] `go test ./internal/auth/... -run TestNormalizePhone` — confirm PASS
- [x] Commit: `feat: upgrade phone normalization to use nyaruka/phonenumbers`

## Step 3: Upgrade Country Detection with phonenumbers (TDD)

- [x] RED: Add new test cases to `internal/auth/sms_auth_test.go`:
  - `TestPhoneCountry`: extract country code from E.164 numbers — `+14155552671` → `"US"`, `+442079460958` → `"GB"`, `+919876543210` → `"IN"`
  - `TestPhoneCountry_DistinguishesNANP`: `+14155552671` (US) vs `+16135550123` (CA) — correctly distinguish US and Canada despite shared `+1` prefix
  - `TestPhoneCountry_Caribbean`: `+18765551234` (Jamaica) is not `"US"` or `"CA"`
  - `TestIsAllowedCountry_WithPhoneNumbers`: US number allowed when `["US"]`, Canadian number blocked when only `["US"]` (was impossible with prefix matching)
- [x] GREEN: Create `phoneCountry(phone string) string` helper in `internal/auth/sms_auth.go`:
  - Uses `phonenumbers.Parse()` + `phonenumbers.GetRegionCodeForNumber()`
  - Returns ISO 3166-1 alpha-2 code (e.g., `"US"`) or `""` on failure
- [x] GREEN: Replace `isAllowedCountry()` to use `phoneCountry()` instead of `countryDialCode` prefix map
- [x] Delete `countryDialCode` map — no longer needed
- [x] Update `TestIsAllowedCountry` existing tests if behavior changes (US/CA distinction)
- [x] `go test ./internal/auth/... -run "TestPhoneCountry|TestIsAllowedCountry"` — confirm PASS
- [x] Commit: `feat: use phonenumbers for country detection, remove dial code map`

## Step 4: Migration 015 — SMS Stats Columns (TDD)

- [x] Create `internal/migrations/sql/015_ayb_sms_stats.sql`:
  - `ALTER TABLE _ayb_sms_daily_counts ADD COLUMN IF NOT EXISTS confirm_count INTEGER NOT NULL DEFAULT 0;`
  - `ALTER TABLE _ayb_sms_daily_counts ADD COLUMN IF NOT EXISTS fail_count INTEGER NOT NULL DEFAULT 0;`
- [x] Update migration runner if needed (check `internal/migrations/migrations.go` for how migrations are registered)
- [x] Commit: `feat: add migration 015 for SMS confirmation stats columns`

## Step 5: Instrument SMS Confirmation Tracking (TDD)

- [x] RED: Add integration tests in `internal/auth/auth_integration_test.go`:
  - `TestSMSStats_ConfirmIncrementsCount`: after successful `ConfirmSMSCode`, `confirm_count` for today incremented
  - `TestSMSStats_FailedConfirmIncrementsFailCount`: after failed `ConfirmSMSCode` (wrong code), `fail_count` for today incremented
- [x] GREEN: Instrument `ConfirmSMSCode()` in `internal/auth/sms_auth.go`:
  - On successful confirmation: `UPDATE _ayb_sms_daily_counts SET confirm_count = confirm_count + 1 WHERE date = CURRENT_DATE`
  - On failed confirmation (invalid/expired code): `UPDATE _ayb_sms_daily_counts SET fail_count = fail_count + 1 WHERE date = CURRENT_DATE`
  - Use upsert pattern (`INSERT ... ON CONFLICT DO UPDATE`) in case no row exists for today yet
- [x] `go test -tags=integration ./internal/auth/... -run TestSMSStats` — confirm PASS
- [x] Commit: `feat: track SMS confirmation success/failure counts`

## Step 6: Admin SMS Health Endpoint (TDD)

- [x] RED: Create tests in `internal/server/sms_health_test.go`:
  - `TestAdminSMSHealth_ReturnsStats`: mock DB with known daily_counts rows, verify JSON response includes `today`, `last_7d`, `last_30d` sections with `sent`, `confirmed`, `failed`, `conversion_rate`
  - `TestAdminSMSHealth_WarnsLowConversion`: when conversion rate < 10%, response includes `"warning": "low conversion rate"`
  - `TestAdminSMSHealth_NoData`: no rows → returns zeroes, no error
  - `TestAdminSMSHealth_RequiresAdmin`: unauthenticated request → 401
- [x] GREEN: Create `internal/server/sms_health_handler.go`:
  - `handleAdminSMSHealth(w, r)` method on `*Server`
  - Query `_ayb_sms_daily_counts` aggregated over today, 7d, 30d windows:
    - `SUM(count)` as sent, `SUM(confirm_count)` as confirmed, `SUM(fail_count)` as failed
  - Calculate `conversion_rate = confirmed / sent * 100` (0 if sent == 0)
  - Include `"warning"` field if conversion rate < 10% and sent > 0
  - Return JSON response
- [x] `go test ./internal/server/... -run TestAdminSMSHealth` — confirm PASS
- [x] Commit: `feat: add admin SMS health endpoint with conversion rate monitoring`

## Step 7: Wire SMS Health Endpoint into Server

- [x] Update `internal/server/server.go`:
  - Add `GET /api/admin/sms/health` route under admin-auth gated section (requires pool != nil)
  - Only register when SMS/auth is enabled
- [x] `go test ./internal/server/...` — confirm PASS
- [x] Commit: `feat: wire admin SMS health endpoint into server routes`

## Step 8: Build & Test Verification

- [x] Run `go build ./...` — confirm PASS
- [x] Run `go test ./internal/auth/...` — confirm all phone normalization and country tests PASS
- [x] Run `go test ./internal/sms/...` — confirm provider tests still PASS (no regression)
- [x] Run `go test ./internal/server/...` — confirm health endpoint tests PASS
- [x] Run `go test ./internal/config/...` — confirm config tests still PASS
- [x] Final commit if needed

## Master Stages
1. [x] SMS Provider Layer
2. [x] Database & Config
3. [x] Auth Service & Handlers
4. [x] Server Wiring & Integration Tests
5. [x] SMS MFA Second Factor
6. [x] Provider Expansion
7. [x] Fraud Hardening & Monitoring — phonenumbers upgrade, conversion rate tracking, admin SMS health endpoint
8. Transactional SMS API
