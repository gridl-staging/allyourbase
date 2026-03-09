# Stage 1: Audit + Backend Endpoint

## Phase 1 — Architecture + API Contract Audit

- [x] Read `internal/server/sms_health_handler.go` — confirm health response JSON shape and `conversionRate()` logic
- [x] Read `internal/server/messaging_handler.go` — confirm `smsMessage` struct fields and handler patterns
- [x] Read `internal/server/server.go` (~lines 160-170) — confirm admin route registration pattern under `/api/admin/sms`
- [x] Read `ui/src/components/Webhooks.tsx` + `ui/src/components/__tests__/Webhooks.test.tsx` — document the list+modal UI pattern and test patterns to follow
- [x] Confirm exact JSON field names for health response and message list response match checklist spec
- [x] Confirm route registration for admin SMS endpoints (health already exists, messages missing)
- [x] Document user-scoped vs admin-scoped message list gap: `GET /api/messaging/sms/messages` filters by `claims.Subject`, admin endpoint needs to list all messages with `user_id` field
- [x] Design component hierarchy and document in session handoff:
  - `Layout.tsx` → Messaging sidebar section (MessageCircle icon)
  - `"sms-health"` view → `<SMSHealth />`
  - `"sms-messages"` view → `<SMSMessages />` (contains `<SMSSendTester />` modal)
- [x] Map every new `types.ts` type needed: `SMSWindowStats`, `SMSHealthResponse`, `SMSMessage`, `SMSMessageListResponse`, `SMSSendResponse`
- [x] Map every new `api.ts` function needed: `getSMSHealth()`, `listAdminSMSMessages()`, `adminSendSMS()`
- [x] No code written in this phase — audit and design only

## Phase 2 — Backend: Admin SMS Messages Endpoint

### RED — write failing tests
- [x] Create `internal/server/sms_admin_messages_test.go` with test: `TestAdminSMSMessages_NoPool_Returns404`
- [x] Add test: `TestAdminSMSMessages_EmptyResult` — expects 200, `{"items":[],"page":1,"perPage":50,"totalItems":0,"totalPages":0}`
- [x] Add test: `TestAdminSMSMessages_ReturnsList` — N messages sorted by `created_at` DESC, `user_id` in JSON
- [x] Add test: `TestAdminSMSMessages_Pagination` — `?page=2&perPage=10` applied correctly, `totalPages` computed
- [x] Run `go test ./internal/server/...` — all 4 new tests fail (RED)

### GREEN — implement
- [x] Add `adminSMSMessage` struct with `UserID string` json tag `"user_id"` in handler file
- [x] Add `ListAllMessages(ctx, limit, offset int) ([]adminSMSMessage, int, error)` to `messageStore` interface
- [x] Implement `ListAllMessages` on `pgMessageStore` with COUNT(*) for total
- [x] Implement `handleAdminSMSMessages` handler: pool nil guard (404), parse `?page`/`?perPage` (defaults 1/50, max 200), compute offset and totalPages, return paginated JSON
- [x] Register route `r.Get("/messages", s.handleAdminSMSMessages)` under `/api/admin/sms` subrouter in `server.go`
- [x] Run `go test ./internal/server/...` — all new tests pass (GREEN)
- [x] Run `go test ./...` — full suite passes
- [x] Commit: `feat: add GET /api/admin/sms/messages endpoint`
