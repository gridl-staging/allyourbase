# Stage 4: Layout Integration + Browser Tests

## Phase 7 — Layout Integration

### Step 0: Add heading to `SMSHealth.tsx`
- [x] `SMSHealth.tsx` currently renders stat cards directly with no `<h2>` heading. Add one for consistency with `SMSMessages.tsx` and for browser test heading assertions.
- [x] Wrap the main content return (after loading/error guards) in a container with heading:
  ```tsx
  <div className="space-y-4">
    <h2 className="text-lg font-semibold">SMS Health</h2>
    {data.warning && ( ... )}
    <div className="grid ...">
  ```
  The `<div className="space-y-4">` already exists — just insert the `<h2>` as the first child.
- [x] Update `SMSHealth.test.tsx`: add a test `renders heading` that asserts `screen.getByRole("heading", { name: /SMS Health/i })` is visible. This should pass immediately with the new heading.
- [x] Run `npm test -- --run SMSHealth` → all tests pass (including new heading test)

### Step 1: Update `Layout.tsx`
- [x] Extend `View` type union: add `"sms-health" | "sms-messages"`
- [x] Import `SMSHealth` from `./SMSHealth`, `SMSMessages` from `./SMSMessages`
- [x] Import `MessageCircle`, `MessageSquare` from `lucide-react`
- [x] Update `handleAdminView` callback parameter type: add `| "sms-health" | "sms-messages"` to the union
- [x] Update `isAdminView` boolean: add `|| view === "sms-health" || view === "sms-messages"`
- [x] Add "Messaging" sidebar section between Services and Admin sections
- [x] Add SMS cases to main content routing (inside the `isAdminView` ternary, BEFORE the `Users` fallback)
- [x] Verify: `npm run build` passes (type-check)

### Step 2: Update `CommandPalette.tsx`
- [x] Import `MessageCircle`, `MessageSquare` from `lucide-react`
- [x] Add two items to `NAV_ITEMS` array
- [x] Verify: `npm run build` still passes

### Step 3: Update `Layout.test.tsx`
- [x] Add mocks for new components at top of file (next to existing `vi.mock` blocks)
- [x] Test: `renders Messaging section in sidebar` — assert "Messaging" section label visible, "SMS Health" and "SMS Messages" button text visible
- [x] Test: `clicking SMS Health renders SMSHealth component` — click "SMS Health" in sidebar, assert `data-testid="sms-health-view"` visible, assert tab bar (Data/Schema/SQL) not visible
- [x] Test: `clicking SMS Messages renders SMSMessages component` — click "SMS Messages" in sidebar, assert `data-testid="sms-messages-view"` visible, assert tab bar hidden
- [x] Test: `clicking a table from SMS view returns to data view` — go to SMS Health, click a table, assert `data-testid="table-browser"` visible
- [x] Run `npm test -- --run Layout` → all tests pass (including existing tests)
- [x] Run `npm test -- --run` (full suite) → no regressions (4 pre-existing StorageBrowser failures unrelated to SMS)
- [x] Commit: `feat: wire SMS views into Layout sidebar and routing`

### Review fixes (session 011)
- [x] **FIX**: SMSHealth.test.tsx — replaced `toHaveTextContent("8")` and `toHaveTextContent("30")` with `within(card).getByText()` to eliminate false positives (substring matching on "80.0%" and "300"/"Last 30 Days")
- [x] **FIX**: SMSSendTester.test.tsx — added `calls onSent after successful send` and `does not call onSent on failed send` tests (coverage gap for critical integration callback)
- [x] **FIX**: Layout.test.tsx — added tab bar hidden assertion to SMS Messages test (consistency with SMS Health test)

## Phase 8 — Browser Unmocked Smoke Tests

**Read `_dev/BROWSER_TESTING_STANDARDS_2.md` before writing any spec file.**

All shortcuts (API calls, SQL seeding) go in `fixtures.ts`. Spec files contain only human-like Act + Assert. Import `test` and `expect` from `"../fixtures"`, NOT from `@playwright/test`.

### Step 0: Add SMS fixtures to `fixtures.ts`
- [x] Add `ensureSMSTestUser` helper (internal, used by seedSMSMessage)
- [x] Add `seedSMSMessage` helper (exported)
- [x] Add `cleanupSMSMessages` helper (exported)
- [x] Add `seedSMSDailyCounts` helper (exported)
- [x] Add `cleanupSMSDailyCounts` helper (exported)

### Step 1: SMS Health smoke test
- [x] Create `ui/browser-tests-unmocked/smoke/sms-health.spec.ts`
- [x] Test: `admin can navigate to SMS Health page and see stat cards` — navigates, clicks sidebar, asserts heading + all 3 stat card labels + stat row labels

### Step 2: SMS Messages smoke test
- [x] Create `ui/browser-tests-unmocked/smoke/sms-messages.spec.ts`
- [x] Test: `seeded message renders in messages list` — seeds via fixture, navigates, asserts phone + body visible, cleans up
- [x] Test: `Send SMS modal opens and closes` — clicks Send SMS, asserts modal fields, cancels, asserts hidden
- [x] Commit: `test: add SMS smoke browser tests` (included in iteration 12 commit)

## Phase 9 — Browser Unmocked Full Tests

Follow `webhooks-lifecycle.spec.ts` pattern: use `pendingCleanup` array + `test.afterEach` for reliable cleanup even on failure.

### Step 1: Full SMS dashboard test
- [x] Create `ui/browser-tests-unmocked/full/sms-dashboard.spec.ts`
- [x] Set up `pendingCleanup` pattern with `test.afterEach`
- [x] Test: `seeded messages render with correct status badges` — seeds 3 messages (delivered/failed/pending), asserts phone + status per row using `tr` filter
- [x] Test: `Send SMS modal validates inputs` — checks disabled/enabled states with progressive form fill
- [x] Test: `send test SMS and verify result` — `test.skip` with auth gap TODO documented
- [x] Test: `SMS Health stats display with seeded daily counts` — cleans first for determinism, seeds via fixture, asserts values in Today card via `getByTestId`
- [x] Commit: `test: add SMS full browser tests` (included in iteration 12 commit)

### Review fixes (session 013)
- [x] **FIX**: SMSHealth.test.tsx — "shows 0% conversion rate" test used `toHaveTextContent("0.0%")` (substring match on whole card) instead of `within().getByText("0.0%")` (exact element match). Fixed for consistency with false-positive fixes from session 011.

## Final Verification
- [x] Run all component tests: `cd ui && npm test -- --run` → 74 pass (SMS: 8+16+13, Layout: 21, CommandPalette: 16)
- [x] Run all smoke browser tests: `cd ui && npx playwright test --project=smoke` → 4 passed
- [x] Run all full browser tests: `cd ui && npx playwright test --project=full` → 4 passed, 1 skipped (auth gap)
- [x] Update `stage_04_checklist.md`: all Phase 7/8/9 items checked off
- [x] Update input file: mark progress
- [x] Final commit: `feat: complete SMS dashboard UI — all phases done`

## Post-Completion Review (session 017)

Critical e2e coverage gaps found and fixed:

### Bugs found in previous implementation
- [x] **BUG**: `seedSMSDailyCounts` fixture used additive `ON CONFLICT DO UPDATE SET count = existing + new` instead of idempotent `EXCLUDED.count`. If cleanup failed silently, values would be non-deterministic. Fixed to use `EXCLUDED`.
- [x] **BUG**: Full e2e tests ran in parallel (`fullyParallel: true`) but the SMS Health stats test and warning badge test both modify `_ayb_sms_daily_counts` for CURRENT_DATE. Race condition caused test failures. Fixed with `test.describe.configure({ mode: "serial" })`.
- [x] **BUG**: `getByText("3")` in "Last 30 Days" card caused strict mode violation — matched both `<h3>Last 30 Days</h3>` (contains "3" as substring of "30") and `<span>3</span>` (failed count). Fixed with `{ exact: true }` on all numeric assertions.

### Coverage gaps filled
- [x] **GAP**: SMS Health test only verified 2 of 4 stats (sent, confirmed). Now verifies all 4: sent, confirmed, failed, conversion rate.
- [x] **GAP**: Last 7 Days and Last 30 Days cards had zero e2e verification. Now both cards are fully verified (all 4 values each).
- [x] **GAP**: SMS Health warning badge had zero e2e coverage. Added test: seeds low conversion rate data (5%), verifies badge visible with correct text.
- [x] **GAP**: `error_message` in message rows had zero e2e coverage. Added test: seeds failed message with error, verifies error text renders in row.
- [x] **GAP**: Pagination had zero e2e coverage. Added test: seeds 55 messages, verifies Next/Prev buttons, navigates pages, verifies Prev disabled on page 1.
- [x] **GAP**: No negative test for warning badge absence. Stats display test now asserts `sms-warning-badge` is hidden when conversion rate is healthy.

### New fixture functions
- [x] `cleanupSMSDailyCountsAll` — deletes all daily counts within 30-day window (for deterministic 7d/30d card testing)
- [x] `seedSMSMessageBatch` — seeds N messages via `generate_series` in a single SQL call (for pagination testing)

### Updated test results
| Suite | Result |
|---|---|
| Component tests | 74 pass |
| Smoke browser tests | 4 pass (3 SMS + auth setup) |
| Full browser tests | 7 pass, 1 skip (auth gap) |


## Master Stages
1. ~~Audit + Backend endpoint~~ ✅ — Architecture/API audit (Phase 1) and Go backend for admin SMS messages endpoint (Phase 2). Complete.
2. ~~TypeScript types + SMSHealth component~~ ✅ — All SMS types and API client functions (Phase 3), TDD SMSHealth stats dashboard component (Phase 4). Complete.
3. ~~SMSMessages + SMSSendTester components~~ ✅ — TDD the messages table with pagination (Phase 5) and the send-SMS modal (Phase 6). Both are pure component work with mocked API. Complete.
4. Layout integration + Browser tests — Wire SMS views into sidebar/routing (Phase 7), then browser smoke tests (Phase 8) and full unmocked e2e tests (Phase 9).


## Key Notes for Implementers

### DB schema reference (verified against migrations)
```
Table: _ayb_sms_messages
  id                  UUID PK (auto)
  user_id             UUID NOT NULL FK → _ayb_users(id)
  to_phone            TEXT NOT NULL        ← JSON: "to"
  body                TEXT NOT NULL
  provider            TEXT NOT NULL
  provider_message_id TEXT DEFAULT ''      ← JSON: "message_id"
  status              TEXT NOT NULL DEFAULT 'pending'
  error_message       TEXT DEFAULT ''      ← JSON: omitempty
  created_at          TIMESTAMPTZ
  updated_at          TIMESTAMPTZ

Table: _ayb_sms_daily_counts  (used by SMS Health stats)
  date           DATE PK
  count          INTEGER (total sends)
  confirm_count  INTEGER
  fail_count     INTEGER
```

### Auth gap for SMS send
`POST /api/messaging/sms/send` requires user auth (`auth.RequireAuth`), not admin token. The admin dashboard token stored in `ayb_admin_token` localStorage will get a 401 from this endpoint. Component tests (mocked) work fine. Browser tests that actually send SMS must either:
- Add a `POST /api/admin/sms/send` endpoint (Go backend change)
- Create and auth as a real user in the test
- Be marked as `test.skip` with a TODO

### Existing components and data-testids
- `SMSHealth.tsx`: stat cards use `data-testid="sms-stats-{windowKey}"` (today, last_7d, last_30d), warning uses `data-testid="sms-warning-badge"`. Heading `<h2>SMS Health</h2>` added in Phase 7 Step 0.
- `SMSMessages.tsx`: heading `<h2>SMS Messages</h2>`, table `data-testid="sms-messages-table"`, rows `data-testid="sms-row-{id}"`, badges `data-testid="status-badge-{status}"`, pagination `data-testid="pagination-prev/next"`, send button `data-testid="open-send-modal"`
- `SMSSendTester.tsx`: heading `<h3>Send Test SMS</h3>`, phone label "To (phone number)", body label "Message body", result `data-testid="send-result"`, error `data-testid="send-error"`

### Browser test patterns to follow
- Smoke tests: `webhooks-crud.spec.ts`, `users-list.spec.ts` — load-and-verify first, then CRUD
- Full tests: `webhooks-lifecycle.spec.ts` — comprehensive lifecycle with `pendingCleanup` + `test.afterEach` pattern
- Fixtures: `fixtures.ts` — use `execSQL` for seeding, `adminToken` fixture for auth
- ESLint: `eslint.config.mjs` — no API calls in spec files, no raw locators, no force clicks
- All spec files import `test` and `expect` from `"../fixtures"`, not from `@playwright/test`
- Allowed raw locators in ESLint config: `aside`, `tr`, `input[type="file"]`, `main`, `option`
- Use `page.locator("aside").getByRole("button", ...)` for sidebar navigation (matches existing patterns)

### Playwright run commands
- Individual smoke test: `cd ui && npx playwright test --project=smoke browser-tests-unmocked/smoke/sms-health.spec.ts`
- All smoke tests: `cd ui && npx playwright test --project=smoke`
- Individual full test: `cd ui && npx playwright test --project=full browser-tests-unmocked/full/sms-dashboard.spec.ts`
- All full tests: `cd ui && npx playwright test --project=full`
- With headed browser (debug): append `--headed`
