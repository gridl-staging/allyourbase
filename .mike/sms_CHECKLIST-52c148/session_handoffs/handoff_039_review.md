# Handoff 039 — Deep Review: Skeptical Audit + UI/UX Gap Analysis

**Date:** 2026-02-20
**Session type:** Review (external skeptical audit — UI/UX + test quality + production bug hunt)
**Branch:** mac2

---

## Summary

Reviewed entire SMS implementation (Phases 1–4) with a skeptical eye toward test quality,
false positives, and production correctness. Found 1 production bug, 1 dummy test that
was actively misleading, and 2 missing tests. Fixed all four. Also identified a significant
product gap: **the admin dashboard has zero SMS UI**.

`go test ./... -count=1` — **all 24 packages PASS** (all previous tests still pass, 5 new ones added).

---

## Production Bug Fixed

### Bug: `handleSMSDeliveryWebhook` silently corrupted status on empty `MessageStatus`

**File:** `internal/server/messaging_handler.go`

**Problem:** `deliveryStatusRank("")` falls into the `default` case and returns rank 5
(the highest — same as terminal statuses like `delivered`). This meant a Twilio callback
with a present `MessageSid` but absent `MessageStatus` would pass the ordering guard
and overwrite any existing status with an empty string `""`, including terminal statuses
like `delivered`. A malformed or partially-delivered webhook could silently blank out
audit records with no error logged.

The stage_08_checklist.md only mentioned `MessageSid` as the required field. It did not
note that `MessageStatus` must also be non-empty.

**Fix:** Added early-return when `messageStatus == ""`. Returns 200 (not 400) because
Twilio retries on any non-2xx response — we must not cause unnecessary retries:

```go
messageStatus := r.FormValue("MessageStatus")
if messageStatus == "" {
    // Skip update to avoid corrupting stored status. Return 200 so Twilio does not retry.
    return
}
```

**Test added:** `TestSMSDeliveryWebhook_MissingMessageStatus` — seeds a message at
`status="delivered"`, sends a callback with `MessageSid` but no `MessageStatus`,
asserts status remains `"delivered"` and response is 200.

---

## Misleading Test Fixed

### `TestAdminSMSHealth_WithAuth_Returns200` was a dummy test with the wrong name

**File:** `internal/server/sms_health_test.go`

**Problem:** The test was named "Returns200" but asserted `http.StatusNotFound` (404).
Its body contained `_ = json.Unmarshal // prevent unused import` — a dead-code smell
from a copy-paste that was never completed. It was functionally identical to
`TestAdminSMSHealth_NoPool_Returns404` (same server setup, same assertion), providing
zero additional coverage while actively misrepresenting the endpoint's behavior to
anyone reading the test output.

**Fix:** Deleted the duplicate test. Replaced it with nothing in the external test file
(the two remaining tests are sufficient for the no-pool path). Moved all new coverage
to the internal test file (see below).

---

## Missing Tests Added

### 1. `TestMessagingSMSGet_RequiresAuth`

**File:** `internal/server/messaging_test.go`

The `GET /api/messaging/sms/messages/{id}` handler has an explicit `claims == nil → 401`
check, but no test covered it. The send and list handlers both had auth tests; get did not.

**Test:** Calls `handleMessagingSMSGet` directly with no claims in context → asserts 401.

### 2. `TestSMSDeliveryWebhook_MissingMessageStatus`

Described above under "Production Bug Fixed."

---

## New Internal Test File

**File:** `internal/server/sms_health_internal_test.go` (new)

Added unit tests for two previously-untested pure functions:

| Test | What it proves |
|------|----------------|
| `TestConversionRate_ZeroSent` | Returns 0 when sent=0 (no divide-by-zero) |
| `TestConversionRate_FullConversion` | 10/10 → 100.0 |
| `TestConversionRate_PartialConversion` | 50/100 → 50.0, 1/4 → 25.0 |
| `TestDeliveryStatusRank_Ordering` | Each lifecycle step has rank >= previous |
| `TestDeliveryStatusRank_TerminalStatuses` | All terminal statuses share the highest rank |
| `TestDeliveryStatusRank_UnknownStatusIsTerminal` | Unknown and empty strings return rank 5 |

The last test (`UnknownStatusIsTerminal`) documents that empty string has rank 5 — which
is why the production bug above was a real bug (empty status would overwrite everything).

---

## Test Count

| Package | Before | After |
|---------|--------|-------|
| `internal/server` (messaging tests) | 29 | 31 (+2) |
| `internal/server` (sms health — external) | 3 | 2 (-1 dummy, replaced) |
| `internal/server` (sms health — internal) | 0 | 6 (+6 new) |

Net: +7 real tests, -1 dummy test.

---

## Files Changed

| File | What changed |
|------|-------------|
| `internal/server/messaging_handler.go` | Early-return when `MessageStatus` is empty |
| `internal/server/messaging_test.go` | Added `TestMessagingSMSGet_RequiresAuth`, `TestSMSDeliveryWebhook_MissingMessageStatus` |
| `internal/server/sms_health_test.go` | Removed `TestAdminSMSHealth_WithAuth_Returns200` (dummy test) |
| `internal/server/sms_health_internal_test.go` | New file: `conversionRate` and `deliveryStatusRank` unit tests |

---

## Major Product Gap: No SMS UI in Admin Dashboard

The admin dashboard (`ui/`) has **zero SMS components**. The sidebar has:
Tables, SQL Editor, Functions, RLS Policies, Storage, Webhooks, Users, API Keys, API Explorer.

There is no section for:
- **SMS configuration status** — is SMS enabled? which provider? which countries allowed?
- **Message history** — view sent messages, status, errors per user or globally
- **SMS health/conversion stats** — the `/api/admin/sms/health` endpoint exists but
  no UI surfaces it
- **Send tester** — no way to test-send an SMS from the dashboard without using curl

This means a developer using the dashboard cannot:
1. Confirm SMS is enabled and wired correctly without checking config files
2. Diagnose delivery failures without querying the DB directly
3. See conversion rates without hitting the API endpoint manually

**The backend APIs are fully implemented and tested. The dashboard gap is a UI build task.**
Suggested scope for a follow-on session:

```
Services section → add "Messaging" group:
  - SMS Health: card showing today/7d/30d stats + conversion rate + warning badge
  - SMS Messages: paginated table of sent messages with status, to, body, error
  - (Phase 2) SMS Config: read-only panel showing current provider, allowed countries
```

This is work that belongs to the dashboard team, not the backend SMS sessions. But until
it exists, the CEO cannot verify SMS works from the dashboard — they'd need to use curl
or check logs.

---

## Known Remaining Limitations (unchanged from handoff_038)

1. **Twilio webhook signature verification** — TODO in `handleSMSDeliveryWebhook`.
   Required before production traffic. Needs webhook URL + Twilio auth token in config.

2. **`api_key_id` always NULL** — `Claims` has no `APIKeyID` field yet. Column exists
   in `_ayb_sms_messages` for future use.

3. **No SMS UI in dashboard** — described above. Requires a separate UI build session.
