# Handoff 038 — Deep Review: Phase 4 Transactional SMS API

**Date:** 2026-02-20
**Session type:** Review (self-directed deep audit of sessions 034–037)
**Branch:** mac2

---

## What was reviewed

Full audit of all Phase 4 code and tests: `internal/server/messaging_handler.go`,
`internal/server/messaging_test.go`, plus all SMS providers, auth package, phone utilities,
and checklist alignment. No manual QA exists — tests are the only gate.

---

## Bugs found and fixed

### Bug 1 (production): Out-of-order webhook callbacks could regress message status

**File:** `internal/server/messaging_handler.go` — `pgMessageStore.UpdateDeliveryStatus`

**Problem:** The SQL blindly `SET status = $1` with no ordering guard. Twilio documents that
status callbacks can arrive out of order under network retries. A `delivered` callback followed
by a late `sent` callback would silently regress the message back to `sent`, losing the final
delivery confirmation.

The stage_08_checklist.md explicitly noted: *"Note: status callbacks may arrive out of order.
Only update if the new status is 'later' in the lifecycle."* This requirement was not implemented.

**Fix:** Added `deliveryStatusRank(status string) int` helper defining the Twilio lifecycle:

```
pending(0) < accepted(1) < queued(2) < sending(3) < sent(4) < {delivered,undelivered,failed,read,canceled}(5)
```

The `pgMessageStore.UpdateDeliveryStatus` SQL now includes a conditional CASE expression:

```sql
WHERE provider_message_id = $3
AND (
    CASE status WHEN 'pending' THEN 0 WHEN 'accepted' THEN 1 ... ELSE 5 END
    <=
    CASE $1::text WHEN 'pending' THEN 0 WHEN 'accepted' THEN 1 ... ELSE 5 END
)
```

The `fakeMsgStore.UpdateDeliveryStatus` in tests mirrors this logic using `deliveryStatusRank`.
Both pg and fake implementations are now consistent — the test fake accurately models real
DB behavior.

---

## Missing tests added

All 5 were genuine gaps — paths that could fail in production with no test catching them.

| Test | What it proves |
|------|----------------|
| `TestSMSDeliveryWebhook_OutOfOrderStatus` | `delivered → sent` does not regress status |
| `TestSMSDeliveryWebhook_DBError` | DB failure on `UpdateDeliveryStatus` → still returns 200 (Twilio must not retry) |
| `TestMessagingSMSSend_DBErrorOnInsert` | DB failure at insert → 500, SMS provider never called (no SMS without audit record) |
| `TestMessagingSMSList_EmptyReturnsArray` | No messages → `[]` not `null` (JSON API contract) |
| `TestMessagingSMSList_LimitClamp` | `?limit=200` clamped to 100 |

Two new error-store wrappers added to support the new tests:
- `errOnInsertMsgStore` — returns error from `InsertMessage`
- `errOnUpdateDeliveryStatusMsgStore` — returns error from `UpdateDeliveryStatus`

Both follow the identical wrapper pattern already established by `errOnGetMsgStore` and
`errOnListMsgStore`.

---

## What was NOT changed

- `handleSMSDeliveryWebhook` still returns 200 on DB error (correct — Twilio behaviour)
- Twilio webhook signature verification remains TODO (needs webhook URL + auth token in config)
- `UpdateMessageSent` / `UpdateMessageFailed` use direct status assignment (not ordering) — correct, because these are called by the server immediately after Send(), not by external webhooks
- No changes to providers, auth, config, migrations

---

## Test count

`go test ./... -count=1` — **all 27 packages PASS**

Phase 4 tests in `internal/server/messaging_test.go`:
- Send: 11 tests (was 10, +DBErrorOnInsert)
- List: 7 tests (was 5, +EmptyReturnsArray, +LimitClamp)
- Get: 4 tests (unchanged)
- Webhook: 7 tests (was 5, +OutOfOrderStatus, +DBError)

---

## Files changed

| File | What changed |
|------|-------------|
| `internal/server/messaging_handler.go` | Added `deliveryStatusRank`; updated `pgMessageStore.UpdateDeliveryStatus` SQL |
| `internal/server/messaging_test.go` | Updated `fakeMsgStore.UpdateDeliveryStatus` ordering; added 2 error wrappers; added 5 tests |
| `.mike/sms_CHECKLIST-52c148/checklists/stage_08_checklist.md` | Updated steps 4, 5, 6 to reflect new tests |
| `_dev/messaging/impl/sms_CHECKLIST.md` | Phase 4 status line updated |

---

## Known remaining limitations (not bugs, documented)

1. **Twilio webhook signature verification** — TODO comment in handler. Requires webhook URL
   in config and Twilio auth token accessible at webhook time. Not a security hole in dev/staging
   (webhook endpoint is not publicly advertised), but required before production traffic.

2. **`api_key_id` always NULL** — `Claims` has no `APIKeyID` field yet. The column exists in
   the messages table for future use. This is a known deferred item from the original checklist.

3. **Status progression between terminals** — `delivered` → `failed` is allowed by the ordering
   (both rank 5). This is intentional: if a message is marked failed but a delayed delivery
   confirmation arrives, we accept it. The alternative (locking on first terminal) would be
   incorrect for Twilio's retry behaviour.
