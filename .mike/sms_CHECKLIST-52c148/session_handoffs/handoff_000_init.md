# Handoff 000 — Init

## What I did

Read the SMS OTP implementation checklist (`_dev/messaging/impl/sms_CHECKLIST.md`) and broke the full scope into 8 stages. Created `stages.md` and the Stage 1 detailed checklist.

## Stage breakdown rationale

The input document has 5 phases. Phase 1 (SMS OTP Auth) is the largest with 9 steps and pre-written impl files — I split it across stages 1-4 by natural boundaries:

1. **Provider Layer** (Steps 1-3) — Pure library code, no DB or config dependencies. ~3 sessions.
2. **Database & Config** (Steps 4-5) — Migration + config validation. ~3 sessions.
3. **Auth Service & Handlers** (Step 6) — The largest single step: OTP logic, phone normalization, geo check, request/confirm flows, route wiring. ~5-8 sessions.
4. **Server Wiring & Integration** (Steps 7-9) — Wiring, integration tests, smoke test. ~3-5 sessions.
5. **SMS MFA** (Phase 2) — New migration, enroll/challenge endpoints, pending token. ~5-8 sessions.
6. **Provider Expansion** (Phase 3 first half) — 7 additional providers + test phone numbers. ~6-10 sessions.
7. **Fraud Hardening** (Phase 3 second half) — libphonenumber, monitoring. ~3-5 sessions.
8. **Transactional SMS API** (Phase 4) — Migration 015, messaging endpoint. ~3-5 sessions.

Phase 5 (Managed SaaS SMS) is explicitly deferred ("do not build until revenue threshold is defined") so it's excluded.

## Files created

- `/Users/stuart/repos/allyourbase_root/allyourbase_dev/.mike/sms_CHECKLIST-52c148/stages.md`
- `/Users/stuart/repos/allyourbase_root/allyourbase_dev/.mike/sms_CHECKLIST-52c148/checklists/stage_01_checklist.md`

## What's next

Stage 1, Session 1: Start the Provider Layer. Create `internal/sms/` directory, copy RED tests from `impl/01_provider_tests.go`, watch them fail, then implement `sms.go` (Provider interface, SendResult, Config) and `log.go` (LogProvider) to make them pass.

## Key context for next session

- Pre-written impl files live in `_dev/messaging/impl/` — copy and adapt, don't write from scratch
- Module path: `github.com/allyourbase/ayb`
- Go 1.25 — `t.Context()` is available
- The `internal/sms/` directory doesn't exist yet
- `sms.Config` struct goes in `sms.go` alongside Provider/SendResult (not in config package)
- Test package is `sms_test` (external test package)
