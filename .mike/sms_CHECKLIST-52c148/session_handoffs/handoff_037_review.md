# Handoff 037 — Post-Session 036 Review

## What was done

### Full codebase review of SMS implementation (Phases 1–4)

Reviewed all code, tests, and docs written across sessions 001–036. Two bugs and one latent
defect were found and fixed. All tests now pass.

---

## Bugs Found and Fixed

### BUG 1 — SILENT ERROR SWALLOWING (critical): `pgMessageStore.GetMessage`

**File:** `internal/server/messaging_handler.go:88`

**Problem:** The `GetMessage` implementation returned `(nil, nil)` for **any** error from
the database, not just `pgx.ErrNoRows`. This meant real DB failures (connection loss,
query errors, malformed UUIDs causing PostgreSQL cast errors) were silently treated as
"message not found" and returned a 404 to the caller. In production, a DB outage on
the message GET endpoint would be completely invisible — no error log, no 500, just a
steady stream of 404s.

```go
// BEFORE (bug):
if err != nil {
    return nil, nil // not found   ← swallows ALL errors
}

// AFTER (fixed):
if err != nil {
    if errors.Is(err, pgx.ErrNoRows) {
        return nil, nil   // genuinely not found
    }
    return nil, err       // real DB error — surfaces as 500
}
```

**Fix:** Added `"errors"` and `"github.com/jackc/pgx/v5"` imports; updated `GetMessage`
to distinguish `pgx.ErrNoRows` (not found = nil, nil) from all other errors (return err).
This follows the established pattern used throughout the codebase (e.g. `sms_auth.go`,
`magic_link.go`).

### BUG 2 — NIL LOGGER PANIC (latent): `newMessagingTestServer` missing logger

**File:** `internal/server/messaging_test.go:197`

**Problem:** The test helper `newMessagingTestServer` created a `*Server` without
initializing `s.logger`. The existing tests never hit a `s.logger.Error(...)` call because
the mock provider and fake store always succeeded, so the nil pointer was dormant. The new
DB-error tests (added to cover Bug 1's fix path) triggered the panic immediately:

```
panic: runtime error: invalid memory address or nil pointer dereference
    log/slog.(*Logger).Error(...)
    internal/server.(*Server).handleMessagingSMSList
```

**Fix:** Added `logger: testutil.DiscardLogger()` to the server struct in
`newMessagingTestServer`. Any future error-path test now works correctly without a
separate fix.

---

## Tests Added

Two tests added to `internal/server/messaging_test.go` to cover the previously untested
DB-error code paths:

- `TestMessagingSMSGet_DBError` — `GetMessage` returns an error → handler returns 500
- `TestMessagingSMSList_DBError` — `ListMessages` returns an error → handler returns 500

Two new error-injecting store types support these tests:

- `errOnGetMsgStore` — delegates everything to `fakeMsgStore` except `GetMessage`, which
  always returns `errors.New("db connection lost")`
- `errOnListMsgStore` — same pattern for `ListMessages`

Both implement the full `messageStore` interface without duplication.

---

## Test Counts

| Package | Before | After |
|---|---|---|
| `internal/server` (messaging) | 25 | 27 |
| All other packages | unchanged | unchanged |

---

## Test Verification

```
go build ./...                    PASS
go test ./internal/sms/...        PASS
go test ./internal/auth/...       PASS
go test ./internal/server/...     PASS (27 messaging tests, all green)
go test ./internal/config/...     PASS
go test ./...                     PASS (all packages)
```

---

## What was NOT changed

- No production handler logic changes (only error propagation fix in `pgMessageStore`)
- No schema or migration changes
- No config changes
- All Phase 1–3 code reviewed and confirmed correct
- Phone utility delegation (`sms_auth.go` → `sms/phone.go`) verified correct
- `ErrInvalidPhoneNumber` aliasing (`auth.ErrInvalidPhoneNumber = sms.ErrInvalidPhoneNumber`)
  verified to work correctly with `errors.Is`
- `start.go` SMS provider wiring (to both `authSvc` and `srv`) confirmed correct

---

## Files Modified

- `internal/server/messaging_handler.go` — fixed `GetMessage` to return real errors
  for non-ErrNoRows failures; added `"errors"` and `"github.com/jackc/pgx/v5"` imports
- `internal/server/messaging_test.go` — fixed nil logger in `newMessagingTestServer`;
  added `errOnGetMsgStore` and `errOnListMsgStore` fakes; added two DB-error tests

## Files Created

- `.mike/sms_CHECKLIST-52c148/session_handoffs/handoff_037_review.md` (this file)

---

## Current Status

**All 8 stages complete. All tests pass. No known bugs remain.**

Stage 8 (Transactional SMS API) is fully implemented and tested:
- Phone utility extracted to `internal/sms/phone.go`
- Migration 016 (`_ayb_sms_messages` table)
- `POST /api/messaging/sms/send` — auth + write-scope check + phone validation + DB persistence
- `GET /api/messaging/sms/messages` — paginated, user-scoped
- `GET /api/messaging/sms/messages/{id}` — user-scoped, 404 on not-found or wrong user, 500 on DB error
- `POST /api/webhooks/sms/status` — Twilio delivery callback (form-encoded, idempotent)
- All error paths (provider failure, DB failure) tested and returning correct HTTP status codes

## Open TODOs (not blocking)

- Twilio webhook request signature verification (marked `TODO` in `handleSMSDeliveryWebhook`)
- Populate `api_key_id` in `_ayb_sms_messages` — requires adding `APIKeyID` field to
  `auth.Claims` (column is already in the schema, currently always NULL for JWT auth)
