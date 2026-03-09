# Stage 2: TypeScript Types + SMSHealth Component

## Phase 3 — TypeScript Types + API Client

### Step 1: Add types to `ui/src/types.ts`
- [x] Add `SMSWindowStats` interface: `sent`, `confirmed`, `failed`, `conversion_rate` (all `number`)
- [x] Add `SMSHealthResponse` interface: `today`, `last_7d`, `last_30d` (each `SMSWindowStats`), optional `warning?: string`
- [x] Add `SMSMessage` interface: `id`, `to`, `body`, `provider`, `message_id`, `status`, `created_at`, `updated_at` (all `string`), `error_message?: string` (optional — Go uses `omitempty`, absent when empty), `user_id?: string` (admin endpoint only)
- [x] Add `SMSMessageListResponse` interface: `items: SMSMessage[]`, `page`, `perPage`, `totalItems`, `totalPages` (all `number`)
- [x] Add `SMSSendResponse` interface: `id`, `message_id`, `status`, `to` (all `string`)

### Step 2: Add API functions to `ui/src/api.ts`
- [x] Import new SMS types in the `import type` block
- [x] Add `getSMSHealth(): Promise<SMSHealthResponse>` → `GET /api/admin/sms/health` (admin token)
- [x] Add `listAdminSMSMessages(params?: { page?: number; perPage?: number }): Promise<SMSMessageListResponse>` → `GET /api/admin/sms/messages` (admin token, query params)
- [x] Add `adminSendSMS(to: string, body: string): Promise<SMSSendResponse>` → `POST /api/messaging/sms/send` with JSON body `{ to, body }` — NOTE: this endpoint uses user auth (`auth.RequireAuth`), not admin token. The admin dashboard may need a dedicated admin send endpoint or auth bridge in Stage 3. For now, define the function to match the existing API.
- [x] `npm run build` passes with no type errors

## Phase 4 — SMSHealth Component (TDD)

### Step 1: RED — write failing tests
- [x] Create `ui/src/components/__tests__/SMSHealth.test.tsx`
- [x] `vi.mock("../../api", ...)` with `getSMSHealth: vi.fn()` — follow Webhooks.test.tsx pattern: named exports in factory, vi.mocked() for typed references
- [x] Add `makeSMSHealthResponse()` helper returning a default `SMSHealthResponse` with realistic numbers (e.g., today: sent=10, confirmed=8, failed=2, conversion_rate=80.0)
- [x] Test: `shows loading state` — pending promise, assert loading spinner/text visible
- [x] Test: `displays stats for all three windows` — resolve with fixture, assert actual numeric values rendered for Today/Last 7 Days/Last 30 Days (sent, confirmed, failed, conversion rate %)
- [x] Test: `shows warning badge when warning key present` — fixture with `warning: "low conversion rate"`, assert `data-testid="sms-warning-badge"` visible, assert text content matches
- [x] Test: `hides warning badge when no warning key` — fixture without `warning` field, assert `data-testid="sms-warning-badge"` not in document
- [x] Test: `shows 0% conversion rate when sent is 0` — all-zero stats fixture, renders "0%" without NaN/crash
- [x] Test: `shows error state with retry button` — rejected promise, assert error message text + Retry button visible
- [x] Test: `clicking Retry re-fetches` — click Retry, assert `getSMSHealth` called twice total
- [x] Run `npm test -- --run SMSHealth` → all 7 tests fail (RED confirmed)

### Step 2: GREEN — implement `SMSHealth.tsx`
- [x] Create `ui/src/components/SMSHealth.tsx`
- [x] Fetch data with `useCallback` + `useEffect` pattern (matches Webhooks.tsx: `fetchX` callback, loading/error/data states)
- [x] Loading state: `<Loader2>` spinner + "Loading..." text (match Webhooks loading style)
- [x] Error state: `<AlertCircle>` + error message + Retry button that re-calls fetch (match Webhooks error style)
- [x] Three-column card layout: "Today" / "Last 7 Days" / "Last 30 Days"
- [x] Each card shows: Sent count, Confirmed count, Failed count, Conversion Rate % (formatted with `toFixed(1)`)
- [x] Warning banner with `data-testid="sms-warning-badge"` when `response.warning` is set
- [x] All `data-testid` attributes referenced in tests are present
- [x] Run `npm test -- --run SMSHealth` → all 7 tests pass (GREEN)
- [x] Run `npm test -- --run` (full suite) → all tests pass (4 pre-existing failures in StorageBrowser/etc unrelated to SMS)
- [x] Commit: `feat: add SMS types, API client, and SMSHealth component`
