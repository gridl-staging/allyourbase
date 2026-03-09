# Handoff 001 — Build (Stage 1: Audit + Backend Endpoint)

**Date:** 2026-02-20
**Rotation:** build
**Stage:** 1/4

## What was done

### Phase 1 — Architecture + API Contract Audit (complete)

Audited all backend handler files and confirmed:

1. **Health response JSON shape** (`sms_health_handler.go`):
   - `today`, `last_7d`, `last_30d` each with: `sent`, `confirmed`, `failed`, `conversion_rate`
   - Optional `warning` key when `todaySent > 0 && conversionRate < 10`
   - `conversionRate()` = confirmed/sent * 100, returns 0 when sent=0
   - Pool nil guard → 404

2. **smsMessage struct** (`messaging_handler.go`):
   - `UserID` has `json:"-"` (hidden from user-scoped responses)
   - Fields: `id`, `to`, `body`, `provider`, `message_id`, `status`, `error_message`, `created_at`, `updated_at`

3. **Route registration** (`server.go` lines 167-170):
   - Only `/health` was registered under `/api/admin/sms`. `/messages` was missing — now added.

4. **User-scoped vs admin-scoped gap confirmed**:
   - `ListMessages` filters by `userID` → admin endpoint needs `ListAllMessages` without user filter

5. **UI patterns documented** (from `Webhooks.tsx` + test):
   - Fetch with `useCallback` + `useEffect`
   - Loading/error/empty states, table with action buttons
   - Modal pattern with discriminated union type
   - Test pattern: `vi.mock("../../api")`, `makeX()` helper, `vi.clearAllMocks()`

6. **Component hierarchy** confirmed:
   - `Layout.tsx` → Messaging sidebar section (MessageCircle icon)
   - `"sms-health"` view → `<SMSHealth />`
   - `"sms-messages"` view → `<SMSMessages />` (contains `<SMSSendTester />` modal)

7. **Types needed** (all confirmed against backend):
   - `SMSWindowStats`, `SMSHealthResponse`, `SMSMessage`, `SMSMessageListResponse`, `SMSSendResponse`

8. **API functions needed**:
   - `getSMSHealth()`, `listAdminSMSMessages()`, `adminSendSMS()`

### Phase 2 — Backend: Admin SMS Messages Endpoint (complete)

**TDD RED → GREEN completed.**

Added `GET /api/admin/sms/messages` endpoint with:
- `adminSMSMessage` struct exposing `user_id` in JSON
- `ListAllMessages(ctx, limit, offset)` on `messageStore` interface + `pgMessageStore` implementation
- `handleAdminSMSMessages` handler with msgStore nil guard, pagination (`?page`, `?perPage`), totalPages computation
- Route registered under existing `/api/admin/sms` subrouter (admin-auth gated)

**Tests written (4 new, all pass):**
- `TestAdminSMSMessages_NoPool_Returns404` — nil msgStore → 404
- `TestAdminSMSMessages_EmptyResult` — 200, correct empty paginated response
- `TestAdminSMSMessages_ReturnsList` — 2 messages from different users, sorted DESC, user_id exposed
- `TestAdminSMSMessages_Pagination` — 15 messages, page=2&perPage=10 → 5 items, totalPages=2

**Design decision:** Handler guards on `s.msgStore == nil` (not `s.pool == nil`) because:
- `msgStore` is the handler's actual dependency
- When pool is nil, msgStore is also nil → 404 (same effect)
- When pool is non-nil but SMS not configured, msgStore is nil → 404 (prevents nil panic)

## Test results

```
go test ./internal/server/... -run "TestAdminSMS" -v
--- PASS: TestAdminSMSMessages_NoPool_Returns404 (0.00s)
--- PASS: TestAdminSMSMessages_EmptyResult (0.00s)
--- PASS: TestAdminSMSMessages_Pagination (0.00s)
--- PASS: TestAdminSMSMessages_ReturnsList (0.00s)
--- PASS: TestAdminSMSHealth_RequiresAdmin (0.00s)
--- PASS: TestAdminSMSHealth_NoPool_Returns404 (0.00s)
PASS
```

Full server test suite: all pass.

## Files created/modified

### Created
- `internal/server/sms_admin_messages_test.go` — 4 tests + `fakeMsgStore.ListAllMessages` implementation

### Modified
- `internal/server/messaging_handler.go` — added `adminSMSMessage` struct, `ListAllMessages` to interface + `pgMessageStore`
- `internal/server/messaging_test.go` — added `ListAllMessages` delegation to all 4 error wrapper store types
- `internal/server/sms_health_handler.go` — added `handleAdminSMSMessages` handler
- `internal/server/server.go` — registered `r.Get("/messages", s.handleAdminSMSMessages)` in admin SMS subrouter
- `.mike/sms_ui_CHECKLIST-a6070a/checklists/stage_01_checklist.md` — all items checked
- `_dev/messaging/impl/sms_ui_CHECKLIST.md` — Phase 1 + Phase 2 items checked

## What's next

**Stage 2:** TypeScript types + SMSHealth component (Phases 3–4)

Phase 3: Add TS types to `ui/src/types.ts` and API functions to `ui/src/api.ts`:
- `SMSWindowStats`, `SMSHealthResponse`, `SMSMessage`, `SMSMessageListResponse`, `SMSSendResponse`
- `getSMSHealth()`, `listAdminSMSMessages()`, `adminSendSMS()`

Phase 4: TDD the `<SMSHealth />` component:
- 7 tests (loading, stats display, warning badge, 0% conversion, error+retry)
- Three-column card layout: Today / Last 7 Days / Last 30 Days

## API contract summary (for Stage 2+)

```
GET /api/admin/sms/messages?page=1&perPage=50
Response: {
  "items": [{ "id", "user_id", "to", "body", "provider", "message_id", "status", "error_message", "created_at", "updated_at" }],
  "page": 1,
  "perPage": 50,
  "totalItems": 0,
  "totalPages": 0
}

GET /api/admin/sms/health
Response: {
  "today": { "sent", "confirmed", "failed", "conversion_rate" },
  "last_7d": { ... },
  "last_30d": { ... },
  "warning": "low conversion rate" // optional
}
```
