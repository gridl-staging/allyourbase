# Handoff 003 — Stage Transition (Stage 2 → Stage 3)

## What I did

### Verified Stage 2 Completion
- All checklist items in `stage_02_checklist.md` are checked off (3 steps, all items complete)
- `go test ./internal/config/...` — PASS
- `go test ./internal/sms/...` — PASS
- `go build ./...` — clean

### Updated stages.md
- Marked stage 2 as `[x]` (complete)

### Generated Stage 3 Checklist
Created `checklists/stage_03_checklist.md` with 4 steps:

1. **Step 1: Unit Tests (RED)** — Create `internal/auth/sms_auth_test.go` with tests for OTP generation, phone normalization, handler behavior (disabled/missing fields/invalid format/valid input), route registration, and SetSMSEnabled. Uses `package auth`, `testutil.*` helpers, `newSMSHandler()` pattern mirroring `newMagicLinkHandler`.

2. **Step 2: Service Layer Implementation (GREEN)** — Add `Phone` to `User` struct, SMS fields to `Service` struct (`smsProvider`, `smsConfig`), sentinel errors, and create `internal/auth/sms_auth.go` with `generateOTP`, `normalizePhone`, `isAllowedCountry`, `RequestSMSCode`, `ConfirmSMSCode`.

3. **Step 3: Handler Implementation (GREEN)** — Add `smsEnabled` to `Handler`, `SetSMSEnabled`, request/response types, `handleSMSRequest` (anti-enumeration: 200 for valid format, 400 for invalid), `handleSMSConfirm` (bcrypt verify, issue tokens).

4. **Step 4: Route Wiring** — Add `/sms` and `/sms/confirm` POST routes in `handler.go` `Routes()`.

### Key Design Decisions Captured in Checklist
- **bcrypt for OTP hashing** (not SHA-256) — matches migration 013 comments
- **Anti-enumeration**: geo-blocked phones get 200 + no error, invalid format gets 400
- **`package auth`** (not `auth_test`) — matches all existing test files
- **`testutil.*`** helpers — matches existing test style (no testify assert/require)
- **Router-based tests**: requests go through `h.Routes()` + `router.ServeHTTP`, same as magic link tests
- **Daily limit circuit breaker**: uses `_ayb_sms_daily_counts` table
- **Find-or-create user by phone**: mirrors magic link's find-or-create pattern with race handling

## What's next
Stage 3 (Auth Service & Handlers) — build session should:
1. Write failing tests first (RED) in `internal/auth/sms_auth_test.go`
2. Implement service methods and handlers (GREEN) in `internal/auth/sms_auth.go`
3. Wire routes in `handler.go`
4. All auth unit tests should pass

## Files created or modified
- `.mike/sms_CHECKLIST-52c148/stages.md` — **modified** (stage 2 marked [x])
- `.mike/sms_CHECKLIST-52c148/checklists/stage_03_checklist.md` — **created**
- `.mike/sms_CHECKLIST-52c148/session_handoffs/handoff_003_stage_transition.md` — **created**
