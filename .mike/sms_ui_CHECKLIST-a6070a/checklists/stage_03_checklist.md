# Stage 3: SMSMessages + SMSSendTester Components

## Phase 5 — SMSMessages Component (TDD)

### Step 1: RED — write failing tests
- [x] Create `ui/src/components/__tests__/SMSMessages.test.tsx`
- [x] `vi.mock("../../api", ...)` with `listAdminSMSMessages: vi.fn()` — follow Webhooks.test.tsx pattern: named exports in factory, `vi.mocked()` for typed references
- [x] Add `makeSMSMessage(overrides?)` helper returning a single `SMSMessage` with overridable fields (realistic defaults: `id: "msg_1"`, `to: "+15551234567"`, `body: "Hello from test"`, `provider: "twilio"`, `message_id: "SM_abc123"`, `status: "delivered"`, `created_at: "2026-02-20T10:00:00Z"`, `updated_at: "2026-02-20T10:00:00Z"`)
- [x] Add `makeSMSMessageListResponse(overrides?)` helper returning a default `SMSMessageListResponse` with 2–3 messages using `makeSMSMessage` (include varied statuses: delivered, failed, pending)
- [x] Test: `shows loading state` — pending promise, assert loading spinner/text visible
- [x] Test: `shows empty state when no messages` — resolve with `{ items: [], page: 1, perPage: 50, totalItems: 0, totalPages: 0 }`, assert "No messages sent yet" text visible
- [x] Test: `renders message rows with correct data` — resolve with fixture containing 2+ messages, assert each row shows: `to` phone number, truncated `body` (60 chars + "…"), `provider`, `status`, `created_at` — verify actual values from fixture, not just element existence
- [x] Test: `status badge: delivered is green` — message with `status: "delivered"`, assert `data-testid="status-badge-delivered"` has green class (`bg-green-100`)
- [x] Test: `status badge: failed is red` — message with `status: "failed"`, assert `data-testid="status-badge-failed"` has red class (`bg-red-100`)
- [x] Test: `status badge: pending is yellow` — message with `status: "pending"`, assert `data-testid="status-badge-pending"` has yellow class (`bg-yellow-100`)
- [x] Test: `shows error_message text in row when present` — message with `error_message: "provider timeout"`, assert error text visible in that row
- [x] Test: `shows pagination when totalPages > 1` — fixture with `totalPages: 3, page: 2`, assert `data-testid="pagination-next"` and `data-testid="pagination-prev"` visible, assert "Page 2 of 3" text visible
- [x] Test: `hides pagination when totalPages <= 1` — fixture with `totalPages: 1, page: 1, items: [makeSMSMessage()]`, assert `data-testid="pagination-next"` NOT in document
- [x] Test: `clicking Next calls listAdminSMSMessages with page 2` — fixture with `page: 1, totalPages: 3`, click Next, assert `listAdminSMSMessages` called with `expect.objectContaining({ page: 2 })`
- [x] Test: `Prev button is disabled on page 1` — fixture with `page: 1, totalPages: 3`, assert `data-testid="pagination-prev"` is disabled
- [x] Test: `Next button is disabled on last page` — fixture with `page: 3, totalPages: 3`, assert `data-testid="pagination-next"` is disabled
- [x] Test: `shows error state with retry button` — rejected promise, assert error message text + Retry button visible; click Retry, assert `listAdminSMSMessages` called twice
- [x] Test: `Send SMS button is visible in header` — assert `data-testid="open-send-modal"` button visible with text "Send SMS"
- [x] Run `npm test -- --run SMSMessages` → all tests fail (RED confirmed)

### Step 2: GREEN — implement `SMSMessages.tsx`
- [x] Create `ui/src/components/SMSMessages.tsx`
- [x] Import `listAdminSMSMessages` from `../api`, SMS types from `../types`, `cn` from `../lib/utils`
- [x] Import `Loader2`, `AlertCircle`, `MessageSquare` from `lucide-react`
- [x] Use `useCallback` + `useEffect` fetch pattern (matches Webhooks.tsx / SMSHealth.tsx)
- [x] State: `messages: SMSMessage[]`, `loading`, `error`, `page`, `totalPages`, `totalItems`, `showSendModal`
- [x] `fetchMessages` callback takes `p: number` parameter (like DeliveryHistoryModal), calls `listAdminSMSMessages({ page: p })`
- [x] Loading state: `<Loader2>` spinner + "Loading..." text (match SMSHealth pattern)
- [x] Error state: `<AlertCircle>` + error message + Retry button (match SMSHealth pattern)
- [x] Empty state: `<MessageSquare>` icon + "No messages sent yet" text (match Webhooks empty state pattern)
- [x] Page header with title "SMS Messages" + "Send SMS" button (`data-testid="open-send-modal"`)
- [x] Table (`data-testid="sms-messages-table"`) with columns: To, Body, Provider, Status, Sent At, Error
- [x] Each row: `data-testid="sms-row-{id}"`
- [x] Body column: truncate to 60 chars + "…" when longer
- [x] Status badge: `data-testid="status-badge-{status}"`, use `cn()` for conditional classes
  - `delivered`/`sent` → green (`bg-green-100 text-green-700`)
  - `failed`/`undelivered`/`canceled` → red (`bg-red-100 text-red-700`)
  - `pending`/`queued`/`accepted`/`sending` → yellow (`bg-yellow-100 text-yellow-700`)
- [x] Error column: show `error_message` when present, otherwise empty
- [x] Pagination bar (only render when `totalPages > 1`): "Page X of Y", Prev (`data-testid="pagination-prev"`) / Next (`data-testid="pagination-next"`) buttons
  - Prev disabled when `page === 1`
  - Next disabled when `page === totalPages`
- [x] Date column: format `created_at` with `new Date(x).toLocaleString()` (matches DeliveryHistoryModal pattern)
- [x] "Send SMS" button sets `showSendModal = true` (actual modal rendering wired in Phase 6 Step 3)
- [x] Run `npm test -- --run SMSMessages` → all tests pass (GREEN)
- [x] Run `npm test -- --run` (full suite) → no regressions
- [x] Commit: `feat: add SMSMessages component with pagination`

## Phase 6 — SMSSendTester Component (TDD)

### Step 1: RED — write failing tests
- [x] Create `ui/src/components/__tests__/SMSSendTester.test.tsx`
- [x] `vi.mock("../../api", ...)` with `adminSendSMS: vi.fn()` — follow same pattern as SMSMessages tests
- [x] Add `makeSMSSendResponse(overrides?)` helper returning a default `SMSSendResponse` with realistic data (`id: "msg_123"`, `message_id: "SM_abc"`, `status: "queued"`, `to: "+15551234567"`)
- [x] Test: `renders phone input, body textarea, and Send button` — assert `input[type="tel"]` or label "To (phone number)", textarea label "Message body", and Send button all visible
- [x] Test: `Send button is disabled when both inputs are empty` — assert Send button is disabled on mount
- [x] Test: `Send button is disabled when phone is empty but body is filled` — fill body only, assert disabled
- [x] Test: `Send button is disabled when body is empty but phone is filled` — fill phone only, assert disabled
- [x] Test: `Send button is enabled when both inputs are non-empty` — fill both, assert enabled
- [x] Test: `shows Sending... while in flight` — fill both inputs, click Send, return pending promise, assert button text is "Sending..." and button is disabled
- [x] Test: `calls adminSendSMS with correct args on submit` — fill phone "+15551234567" and body "Hello test", click Send, assert `adminSendSMS` called with `("+15551234567", "Hello test")`
- [x] Test: `shows success result with id and status after send` — resolve with fixture, assert `data-testid="send-result"` visible, verify actual `id`, `status`, `to` values from response rendered
- [x] Test: `shows error message when adminSendSMS rejects` — reject with error, assert `data-testid="send-error"` visible with error message text
- [x] Test: `clears inputs after successful send` — after successful send, assert phone input and body textarea are empty
- [x] Test: `calls onClose when Cancel is clicked` — pass `onClose` spy, click Cancel, assert spy called once
- [x] Run `npm test -- --run SMSSendTester` → all tests fail (RED confirmed)

### Step 2: GREEN — implement `SMSSendTester.tsx`
- [x] Create `ui/src/components/SMSSendTester.tsx`
- [x] Props: `{ onClose: () => void; onSent?: () => void }` — `onSent` callback for parent to refresh message list after send
- [x] Modal overlay (match Webhooks modal pattern: `fixed inset-0 bg-black/40 flex items-center justify-center z-40`)
- [x] Header: "Send Test SMS" + Close (X) button
- [x] Phone input: `type="tel"`, label "To (phone number)", placeholder "+1234567890"
- [x] Body textarea: label "Message body", placeholder "Enter message text..."
- [x] Send button: disabled until both phone and body are non-empty; shows "Sending..." + disabled while in flight
- [x] On success: display result card (`data-testid="send-result"`) showing `id`, `status`, `to` from response; clear inputs; call `onSent?.()` if provided
- [x] On error: display error message (`data-testid="send-error"`) with red styling
- [x] Cancel button: calls `onClose`
- [x] NOTE: `adminSendSMS` uses user auth (`auth.RequireAuth`), not admin token. This means the admin dashboard token will NOT work with this endpoint in production. Stage 4 browser tests must address this — either add an admin send endpoint, use a real user token for testing, or mark send tests as skip with a TODO. For now (mocked tests), this doesn't matter.
- [x] Run `npm test -- --run SMSSendTester` → all tests pass (GREEN)

### Step 3: Wire SMSSendTester into SMSMessages
- [x] Import `SMSSendTester` into `SMSMessages.tsx`
- [x] Conditionally render `<SMSSendTester onClose={() => setShowSendModal(false)} onSent={() => fetchMessages(page)} />` when `showSendModal` is true
- [x] Update `SMSMessages.test.tsx` mock: add `adminSendSMS: vi.fn()` to the `vi.mock("../../api", ...)` factory (required because `SMSSendTester` imports it)
- [x] Add test to `SMSMessages.test.tsx`: `clicking Send SMS opens the send modal` — click `data-testid="open-send-modal"`, assert "Send Test SMS" heading or phone input visible
- [x] Run `npm test -- --run SMSMessages` → still all pass (including new test)
- [x] Run `npm test -- --run SMSSendTester` → still all pass
- [x] Run `npm test -- --run` (full suite) → no regressions
- [x] Commit: `feat: add SMSSendTester modal and wire into SMSMessages`


## Master Stages
1. ~~Audit + Backend endpoint~~ ✅ — Architecture/API audit (Phase 1) and Go backend for admin SMS messages endpoint (Phase 2). Complete.
2. ~~TypeScript types + SMSHealth component~~ ✅ — All SMS types and API client functions (Phase 3), TDD SMSHealth stats dashboard component (Phase 4). Complete.
3. ~~SMSMessages + SMSSendTester components~~ ✅ - TDD the messages table with pagination (Phase 5) and the send-SMS modal (Phase 6). Both are pure component work with mocked API. Complete.
4. Layout integration + Browser tests - Wire SMS views into sidebar/routing (Phase 7), then browser smoke tests (Phase 8) and full unmocked e2e tests (Phase 9).
