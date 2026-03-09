# Admin UI E2E Test Specifications (Tier 2)

**BDD References:** B-ADMIN-001 through B-ADMIN-011
**Test Location:** `ui/browser-tests-unmocked/smoke/` and `ui/browser-tests-unmocked/full/`
**Framework:** Playwright
**Principle:** UI-ONLY — no direct API calls in E2E tests

---

## Navigation Pattern

All admin UI pages follow this navigation pattern:
1. Navigate to `/admin/`
2. Verify "AYB Admin" text is visible
3. Click sidebar button for the desired section

**Sidebar Admin Buttons:**
- `Webhooks` — Webhooks management
- `Storage` — Storage browser
- `Users` — User management
- `Functions` — Function browser
- `API Keys` — API key management
- `API Explorer` — Interactive API explorer
- `RLS Policies` — Row-Level Security management

**Table View Tabs (when table selected):**
- `Data` — Table browser
- `Schema` — Schema view
- `SQL` — SQL editor

---

## Admin Login

### SMOKE TEST: Admin login with password
**BDD:** B-ADMIN-001
**File:** `ui/browser-tests-unmocked/smoke/admin-login.spec.ts`
**Precondition:** Server started with `admin.password` configured

**Steps:**
1. Navigate to `/admin/`
2. Verify login form shows "Enter the admin password"
3. Verify "Sign in" button is visible
4. Enter admin password in password input
5. Click "Sign in"
6. Verify "AYB Admin" sidebar text visible (dashboard loaded)

**Verify:** Dashboard loads after correct password

### SMOKE TEST: Admin rejects wrong password
**Precondition:** Server started with `admin.password` configured

**Steps:**
1. Navigate to `/admin/`
2. Enter "wrongpassword" in password input
3. Click "Sign in"
4. Verify "invalid password" error text visible

**Verify:** Error message displayed, not logged in

---

## Webhooks Management

### SMOKE TEST: Webhooks CRUD
**BDD:** B-ADMIN-003
**File:** `ui/browser-tests-unmocked/smoke/webhooks-crud.spec.ts`

**Steps:**
1. Navigate to admin → Click "Webhooks" sidebar button
2. Verify webhooks view loads (heading "Webhooks" visible)
3. Click "Create Webhook" or "+" button
4. Fill URL: `https://httpbin.org/post`
5. Click "Create" / "Save"
6. Verify webhook appears in list with URL text
7. Click delete icon on webhook → Confirm deletion
8. Verify webhook removed from list

**Verify:** Full create-delete cycle works

### FULL TEST: Webhooks lifecycle
**BDD:** B-ADMIN-003
**File:** `ui/browser-tests-unmocked/full/webhooks-lifecycle.spec.ts`

**Steps:**
1. Navigate to Webhooks
2. Create webhook with URL `https://httpbin.org/post`, events: `create`, tables: (empty = all)
3. Verify webhook in list
4. Toggle enabled/disabled switch → Verify toast confirmation
5. Click edit icon → Change URL to `https://httpbin.org/anything` → Save
6. Verify updated URL in list
7. Click test icon → Verify test result toast (success or failure)
8. Click history icon → Verify delivery history modal opens
9. Close history modal
10. Delete webhook → Confirm → Verify removed

**Verify:** Create, edit, toggle, test, history, delete all work

---

## Storage Management

### SMOKE TEST: Storage upload + download + delete
**BDD:** B-ADMIN-004
**File:** `ui/browser-tests-unmocked/smoke/storage-upload.spec.ts` (expanded)

**Steps:**
1. Navigate to admin → Click "Storage" sidebar button
2. Upload file via hidden file input: `smoke-test-{timestamp}.txt`
3. Verify file appears in list OR success toast
4. Click download icon on uploaded file (verify no error)
5. Click delete icon → Confirm deletion
6. Verify file removed from list

**Verify:** Upload, download, delete cycle works

### FULL TEST: Storage lifecycle
**BDD:** B-ADMIN-004
**File:** `ui/browser-tests-unmocked/full/storage-lifecycle.spec.ts`

**Steps:**
1. Navigate to Storage
2. Upload text file → Verify in list
3. Upload image file (PNG) → Verify in list
4. Click preview (Eye) icon on image → Verify preview modal opens → Close
5. Click signed URL icon → Verify "Signed URL copied" toast
6. Click download icon → Verify no error
7. Delete first file → Confirm → Verify removed
8. Delete second file → Confirm → Verify removed / empty state

**Verify:** Upload, list, preview, signed URL, download, delete all work

---

## User Management

### SMOKE TEST: Users list
**BDD:** B-ADMIN-005
**File:** `ui/browser-tests-unmocked/smoke/users-list.spec.ts`

**Steps:**
1. Navigate to admin → Click "Users" sidebar button
2. Verify Users view loads (heading "Users" visible)
3. Verify either user list OR empty state ("No users" or similar) is visible
4. If users exist: verify table shows email column

**Verify:** Users section loads and displays correctly

---

## API Key Management

### FULL TEST: API Keys lifecycle
**BDD:** B-ADMIN-006
**File:** `ui/browser-tests-unmocked/full/api-keys-lifecycle.spec.ts`

**Precondition:** At least one user exists (create via SQL if needed)

**Steps:**
1. Navigate to admin → Click "API Keys" sidebar button
2. Verify API Keys view loads
3. Click "Create Key" button
4. Fill name: `test-key-{timestamp}`
5. Select user from dropdown
6. Select scope: "full access" (*)
7. Click "Create"
8. Verify "created" modal shows with the new key
9. Close modal
10. Verify key appears in list with name and "Active" badge
11. Click revoke (trash) icon → Confirm revocation
12. Verify key shows "Revoked" badge OR is removed from list

**Verify:** Create, view, revoke lifecycle works

---

## Functions Browser

### FULL TEST: Functions browser
**BDD:** B-ADMIN-007
**File:** `ui/browser-tests-unmocked/full/functions-browser.spec.ts`

**Precondition:** Create a test function via SQL

**Steps:**
1. Create function via SQL: `CREATE FUNCTION test_add(a int, b int) RETURNS int AS 'SELECT a + b' LANGUAGE SQL`
2. Navigate to admin → Click "Functions" sidebar button
3. Verify Functions view loads with function count
4. Verify `test_add` appears in function list
5. Click to expand `test_add`
6. Verify parameter inputs visible (a, b)
7. Fill a=3, b=5
8. Click Execute (Play icon)
9. Verify result shows `8` (or status 200)
10. Clean up: Drop function via SQL

**Verify:** Browse, expand, execute, results display all work

---

## API Explorer

### FULL TEST: API Explorer
**BDD:** B-ADMIN-008
**File:** `ui/browser-tests-unmocked/full/api-explorer.spec.ts`

**Steps:**
1. Navigate to admin → Click "API Explorer" sidebar button
2. Verify API Explorer loads (method selector, path input visible)
3. Verify GET is selected by default
4. Enter path: `/api/schema`
5. Click Execute (Play icon)
6. Verify response section shows status 200
7. Verify response body contains "tables"
8. Verify cURL tab shows generated cURL command
9. Verify JS SDK tab shows generated SDK code

**Verify:** Send request, view response, code generation all work

---

## RLS Policy Management

### FULL TEST: RLS Policies
**BDD:** B-ADMIN-009
**File:** `ui/browser-tests-unmocked/full/rls-policies.spec.ts`

**Precondition:** Create a test table via SQL

**Steps:**
1. Create table via SQL: `CREATE TABLE rls_test_items (id serial primary key, name text, user_id uuid)`
2. Navigate to admin → Click "RLS Policies" sidebar button
3. Verify RLS view loads
4. Select `rls_test_items` from table dropdown
5. Verify RLS is currently disabled
6. Click enable RLS toggle → Verify enabled state
7. Click "Create Policy" or "+" button
8. Fill policy name: `test_owner_policy`
9. Select command: ALL
10. Fill USING clause: `(user_id = current_setting('app.user_id')::uuid)`
11. Click "Create"
12. Verify policy appears in list
13. Delete policy → Confirm
14. Verify policy removed
15. Disable RLS → Verify disabled state
16. Clean up: Drop table via SQL

**Verify:** Enable/disable RLS, create policy, delete policy all work

---

## Table Browser Advanced

### FULL TEST: Table browser advanced features
**BDD:** B-ADMIN-010
**File:** `ui/browser-tests-unmocked/full/table-browser-advanced.spec.ts`

**Precondition:** Create test table with sample data via SQL

**Steps:**
1. Create table and insert 5 test records via SQL
2. Navigate to table in sidebar
3. **Search:** Enter search term → Verify filtered results → Clear search
4. **Filter:** Apply advanced filter (e.g., `status='active'`) → Verify results → Clear
5. **Sort:** Click column header → Verify sort applied (header shows sort indicator)
6. **Export CSV:** Click export → Select CSV → Verify download initiated or toast
7. **Export JSON:** Click export → Select JSON → Verify download initiated or toast
8. **Row Detail:** Click a row → Verify detail drawer opens with all fields → Close
9. **Batch Delete:** Select multiple checkboxes → Click batch delete → Confirm → Verify deleted
10. Clean up: Drop table via SQL

**Verify:** Search, filter, sort, export, row detail, batch delete all work

---

## Schema Browser

### Covered by: `full/blog-platform-journey.spec.ts` (Step 11)
**BDD:** B-ADMIN-011

The blog platform journey test already validates:
- Switch to Schema view via tab
- Column listing (author_id visible)
- FK relationship display (references authors)
- FK in comments (post_id references posts)

No additional standalone test needed — blog journey provides sufficient coverage.

---

## Selector Strategy

**Priority (most to least resilient):**
1. `getByRole("button", { name: "..." })` — Buttons by accessible name
2. `getByText("...")` — Visible text content
3. `locator("aside").getByRole("button", { name: /^Storage$/i })` — Scoped sidebar buttons
4. `getByPlaceholder(/.../)` — Form inputs
5. `locator('input[name="..."]')` — Named form fields
6. `locator('label:has-text("...")').locator('..').locator('input')` — Label-based fallback

**Common Patterns:**
- Sidebar nav: `page.locator("aside").getByRole("button", { name: /^ButtonText$/i })`
- Tab switching: `page.getByRole("button", { name: /^TabName$/i })`
- Confirmation dialog: `page.getByRole("button", { name: /confirm|yes|delete/i })`
- Success toast: `page.getByText(/success|created|deleted|copied/i)`
- Unique data: Use `Date.now()` suffix for all test data names

---

**Document Version:** 1.0
**Last Updated:** 2026-02-13 (Session 081)
