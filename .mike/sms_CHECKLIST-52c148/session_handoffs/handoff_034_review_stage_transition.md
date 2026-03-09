# Handoff 034 — Stage Transition Review (7 → 8)

## What was done

### Stage 7 Verification
- Confirmed all 8 steps in stage 7 checklist are checked off
- Ran full test suite — all pass:
  - `go build ./...` — PASS
  - `go test ./internal/auth/...` — PASS (cached)
  - `go test ./internal/sms/...` — PASS (cached)
  - `go test ./internal/server/...` — PASS (cached)
  - `go test ./internal/config/...` — PASS (cached)

### Stage 8 Checklist Review & Corrections

Reviewed the previous session's stage 8 checklist against the actual codebase and Twilio documentation.
Found and fixed 6 issues:

1. **`api_key_id` vs missing `APIKeyID` in Claims**: `auth.Claims` has no `APIKeyID` field. `ValidateAPIKey()`
   scans `keyID` but discards it. Made `api_key_id` explicitly nullable in migration, documented as future
   follow-up when `APIKeyID` is added to Claims. Added context notes to checklist header.

2. **Missing compound index**: Original had separate `user_id` and `created_at DESC` indexes. List query does
   `WHERE user_id = $1 ORDER BY created_at DESC`. Replaced with compound index `(user_id, created_at DESC)`
   which serves both filter and sort in a single index scan.

3. **`user_id` should be NOT NULL**: Original migration had `user_id UUID REFERENCES _ayb_users(id)` without
   NOT NULL. Every message must have an owner — added NOT NULL.

4. **Twilio status values incomplete**: Original listed 5 statuses. Added full set from Twilio docs:
   `accepted`, `queued`, `sending`, `sent`, `delivered`, `undelivered`, `failed`, `read`, `canceled`.

5. **Webhook route placement ambiguous**: Original showed `r.Post("/api/webhooks/sms/status", ...)`
   which appears to be root-level. Clarified it should be inside the `/api` route group but OUTSIDE
   the JSON content-type group: `r.Post("/webhooks/sms/status", ...)`.

6. **Missing body-too-long test**: Added `TestMessagingSMSSend_BodyTooLong` test case for body > 1600 chars.

### Additional improvements
- Added detailed context notes about `auth.Claims` structure, `auth.CheckWriteScope()`, and
  `auth.ClaimsFromContext()` to checklist header — gives implementer everything needed without digging
- Added explicit note about auth → sms import direction (no circular dependency risk)
- Added Twilio webhook field documentation with all status values at top of Step 6
- Clarified Step 1 is a pure move-and-export refactor with exact source line references
- Clarified Step 3 to use existing variable `p` from `buildSMSProvider()` in start.go
- Updated `sms_CHECKLIST.md` (input file) Phase 4 description with fuller scope

## Files modified
- `.mike/sms_CHECKLIST-52c148/checklists/stage_08_checklist.md` — rewrote with 6 fixes
- `.mike/sms_CHECKLIST-52c148/stages.md` — updated stage 8 description
- `.mike/sms_CHECKLIST-52c148/state.json` — updated stage to 8
- `_dev/messaging/impl/sms_CHECKLIST.md` — updated Phase 4 description

## Files created
- `.mike/sms_CHECKLIST-52c148/session_handoffs/handoff_034_review_stage_transition.md`

## What's next
- Stage 8 implementation (Transactional SMS API) — the final stage
- Start with Step 1: phone utility refactor — extract `normalizePhone`, `phoneCountry`, `isAllowedCountry`
  from `internal/auth/sms_auth.go` to `internal/sms/phone.go` as exported functions
- Key codebase context for implementer:
  - Phone functions: `internal/auth/sms_auth.go` lines 33-87
  - Server struct: `internal/server/server.go` line 27
  - Auth Claims: `internal/auth/auth.go` line 81 (Subject=userID, APIKeyScope, no APIKeyID)
  - Route registration: `internal/server/server.go` New() function
  - SMS Provider wiring: `internal/cli/start.go` line 323
  - Migration runner: `internal/migrations/runner.go` — uses `//go:embed sql/*.sql`, auto-discovers by filename sort
