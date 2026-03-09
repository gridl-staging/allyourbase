# Handoff 028 — Stage 6→7 Transition

## What was done

1. **Verified Stage 6 completion**: All 10 steps in `stage_06_checklist.md` are checked off (`[x]`). No items remain unchecked.

2. **Ran all relevant test suites** — all pass:
   - `go build ./...` — PASS
   - `go test ./internal/sms/...` — PASS
   - `go test ./internal/config/...` — PASS
   - `go test ./internal/cli/...` — PASS

3. **Updated `stages.md`**: Marked stage 6 as `[x]` (done).

4. **Generated stage 7 checklist** at `.mike/sms_CHECKLIST-52c148/checklists/stage_07_checklist.md`.

5. **Advanced `state.json`**: current_stage → 7, rotation → build.

## Stage 7 Overview: Fraud Hardening & Monitoring

8 steps covering:

- **Steps 1–3**: Replace manual `normalizePhone()` and `countryDialCode` map with `nyaruka/phonenumbers`. This enables true phone validation (reject invalid area codes), proper country extraction (distinguish US vs Canada despite shared +1), and eliminates the hardcoded 40-country dial code map.
- **Step 4**: Migration 015 — add `confirm_count` and `fail_count` columns to `_ayb_sms_daily_counts`.
- **Step 5**: Instrument `ConfirmSMSCode()` to track confirmation success/failure in the DB.
- **Steps 6–7**: New `GET /api/admin/sms/health` endpoint returning send counts, confirmation counts, conversion rate, and low-rate warnings. Admin-auth gated.
- **Step 8**: Build & test verification.

## Key context for the next session

### Current phone normalization (`internal/auth/sms_auth.go:33-61`)
- Manual character stripping + E.164 regex validation
- `countryDialCode` map (lines 79-92): 40 countries, prefix-based matching
- `isAllowedCountry` (lines 65-76): checks if phone prefix matches any allowed country's dial code
- **Limitation**: Cannot distinguish US vs Canada (both +1), no validation beyond digit count

### Existing tests (`internal/auth/sms_auth_test.go`)
- `TestNormalizePhone`: 5 valid cases (must continue passing)
- `TestNormalizePhoneRejectsInvalid`: 9 invalid cases (must continue passing)
- `TestIsAllowedCountry`: tests empty list, US/GB allowed, IN/DE blocked, unknown code
- Note: the US/CA distinction test will need updating — current implementation treats both as +1

### Database (`internal/migrations/sql/013_ayb_sms.sql`)
- `_ayb_sms_daily_counts`: columns `date DATE`, `count INTEGER` — tracks sends only
- Migration 015 will add `confirm_count` and `fail_count`

### Admin endpoints (`internal/server/server.go:96-159`)
- Existing pattern: `/api/admin/*` routes under `s.requireAdminToken` middleware
- Stats handler at `internal/server/logs_stats_handler.go` — returns runtime stats, no SMS metrics
- New SMS health endpoint follows same pattern

### No existing phonenumbers dependency
- `go.mod` does not include `nyaruka/phonenumbers` — must be added in Step 1

## Files modified
- `.mike/sms_CHECKLIST-52c148/stages.md` — stage 6 marked done
- `.mike/sms_CHECKLIST-52c148/state.json` — advanced to stage 7
- `.mike/sms_CHECKLIST-52c148/checklists/stage_07_checklist.md` — created

## Checklist path
- `.mike/sms_CHECKLIST-52c148/checklists/stage_07_checklist.md`
