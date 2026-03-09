# Materialized Views Test Specification (Stage 4)

**Purpose:** Test matrix for materialized view refresh mechanism and joined-table RLS SSE verification.

---

## Unit Tests

### TC-MV-001: Identifier Validation

**File:** `internal/matview/validate_test.go`
**Type:** Unit

**Cases:**
1. Valid identifiers pass (`public`, `leaderboard`, `_internal1`, `A1`)
2. Invalid identifiers rejected (empty, starts with digit, hyphen, dot, space, double-quote)

### TC-MV-002: Refresh SQL Generation

**File:** `internal/matview/validate_test.go`
**Type:** Unit

**Cases:**
1. Standard mode produces `REFRESH MATERIALIZED VIEW "schema"."view"`
2. Concurrent mode produces `REFRESH MATERIALIZED VIEW CONCURRENTLY "schema"."view"`

### TC-MV-003: Service RefreshNow — Advisory Lock Flow

**File:** `internal/matview/service_test.go`
**Type:** Unit (fake store)

**Cases:**
1. Lock not acquired → returns `ErrRefreshInProgress`, records error status
2. Lock acquired, refresh succeeds → records success status, returns duration
3. Lock acquired, refresh fails → records error status with message
4. Concurrent mode without unique index → returns `ErrConcurrentRefreshRequiresIndex`
5. Matview dropped after registration → returns `ErrNotMaterializedView`

### TC-MV-004: Job Handler Payload Parsing

**File:** `internal/matview/handler_test.go`
**Type:** Unit (fake store)

**Cases:**
1. Valid payload `{"schema":"public","view_name":"leaderboard"}` → calls RefreshNow
2. Missing `view_name` → error containing "view_name"
3. Schema omitted → defaults to `"public"`
4. Invalid JSON → error
5. View not registered → auto-registers with standard mode, then refreshes
6. Duplicate auto-register race → falls back to GetByName lookup
7. DB error from GetByName → propagated without auto-register attempt

---

## Server Handler Tests

### TC-MV-005: Admin API Error Mapping

**File:** `internal/server/matviews_handler_test.go`
**Type:** Unit (fake admin)

**Cases:**

| Handler | Test | Expected |
|---|---|---|
| List | 2 registrations | 200 with count=2 |
| List | empty | 200 with count=0 |
| Get | valid ID | 200 with registration |
| Get | not found | 404 |
| Get | invalid UUID | 400 |
| Register | valid | 201 with registration |
| Register | missing viewName | 400 |
| Register | schema defaults to public | 201 |
| Register | invalid refreshMode | 400 |
| Register | duplicate | 409 |
| Register | not a matview | 404 |
| Update | valid | 200 |
| Update | not found | 404 |
| Update | invalid mode | 400 |
| Delete | valid | 204 |
| Delete | not found | 404 |
| Refresh | valid | 200 with duration |
| Refresh | in progress | 409 |
| Refresh | missing unique index | 409 |
| Refresh | requires populated | 409 |
| Refresh | not found | 404 |

---

## CLI Tests

### TC-MV-006: CLI Command Parsing and Output

**File:** `internal/cli/matviews_cli_test.go`
**Type:** Unit (in-process HTTP transport stubs)

**Cases:**

| Command | Test | Assertion |
|---|---|---|
| `matviews` | registered | subcommand exists on root |
| `matviews list` | table output | contains view names, modes |
| `matviews list --json` | JSON output | parses as JSON array |
| `matviews list` | empty | "No materialized views" message |
| `matviews register` | success | "registered" message, correct payload |
| `matviews register --schema analytics` | custom schema | payload schema=analytics |
| `matviews register` (no --view) | missing view | error containing "view" |
| `matviews register --mode invalid` | invalid mode | error containing "mode" |
| `matviews update <id> --mode concurrent` | success | "updated" message |
| `matviews update <id>` | not found | error |
| `matviews update --mode bogus` | invalid mode | error containing "mode" |
| `matviews unregister <id>` | success | "unregistered" message |
| `matviews unregister <id>` | not found | error |
| `matviews refresh <id>` | success | view name + duration |
| `matviews refresh <id>` | in progress | error |
| `matviews refresh <id>` | not found | error |
| `matviews refresh public.leaderboard` | qualified name | resolves via list API, then refreshes |
| `matviews refresh public.missing` | not registered | "not registered" error |

---

## Integration Tests (require TEST_DATABASE_URL)

### TC-MV-007: Store CRUD Against Real Database

**File:** `internal/matview/integration_test.go`
**Type:** Integration (`-tags integration`)

**Cases:**
1. Register + Get by ID + Get by name
2. Register rejects non-matview (regular table)
3. Register rejects duplicate (schema_name, view_name)
4. Update refresh mode
5. Update not found
6. Delete
7. Delete not found
8. List ordered by schema/view_name
9. MatviewState: populated matview → (true, true)
10. MatviewState: nonexistent → (false, false)

### TC-MV-008: Advisory Lock Mutual Exclusion

**File:** `internal/matview/integration_test.go`
**Type:** Integration (`-tags integration`)

**Cases:**
1. External connection holds advisory lock → `LockedRefresh` returns `ErrRefreshInProgress`
2. After external lock released → `LockedRefresh` succeeds

### TC-MV-009: Concurrent Unique Index Check

**File:** `internal/matview/integration_test.go`
**Type:** Integration (`-tags integration`)

**Cases:**
1. Matview without unique index → `HasConcurrentUniqueIndex` returns false
2. Matview with unique index → returns true

### TC-MV-010: Service RefreshNow Against Real Database

**File:** `internal/matview/integration_test.go`
**Type:** Integration (`-tags integration`)

**Cases:**
1. Standard refresh: insert more data, refresh, verify total updated
2. Concurrent refresh with unique index: succeeds, records success status
3. Concurrent refresh without unique index: returns `ErrConcurrentRefreshRequiresIndex`
4. Concurrent refresh on unpopulated view: returns `ErrConcurrentRefreshRequiresPopulated`
5. Matview dropped after registration: returns `ErrNotMaterializedView`
6. Registration not found: returns `ErrRegistrationNotFound`

### TC-MV-011: Job Handler Integration

**File:** `internal/jobs/handlers_test.go`
**Type:** Integration (`-tags integration`)

**Cases:**
1. Enqueue `materialized_view_refresh` job → handler executes RefreshNow, updates registry metadata

---

## Migration Tests

### TC-MV-012: Migration 025 Schema Validation

**File:** `internal/migrations/matview_sql_test.go`
**Type:** Unit (SQL parsing)

**Cases:**
1. SQL contains identifier CHECK constraints
2. SQL contains refresh_mode CHECK constraint (`standard`, `concurrent`)
3. SQL contains refresh_status CHECK constraint (`success`, `error`)
4. SQL contains UNIQUE constraint on `(schema_name, view_name)`

### TC-MV-013: Migration Integration

**File:** `internal/migrations/matview_migrations_integration_test.go`
**Type:** Integration (`-tags integration`)

**Cases:**
1. Apply migration, verify table exists
2. CHECK constraint rejects invalid mode
3. UNIQUE constraint rejects duplicate (schema_name, view_name)

---

## Realtime SSE Integration Tests (require TEST_DATABASE_URL)

### TC-MV-014: Joined-Table RLS Membership Allow/Deny

**File:** `internal/realtime/visibility_integration_test.go`
**Type:** Integration (`-tags integration`)

**Setup:** `secure_docs` table with `EXISTS (... project_memberships ...)` RLS policy

**Cases:**
1. User with matching membership → `canSeeRecord` returns true
2. User without membership → `canSeeRecord` returns false

### TC-MV-015: Membership Transition Semantics

**File:** `internal/realtime/visibility_integration_test.go`
**Type:** Integration (`-tags integration`)

**Cases (same subscription context):**
1. Before membership insert → event filtered (false)
2. After membership insert → event visible (true)
3. After membership delete → event filtered again (false)

### TC-MV-016: Delete Event Pass-Through

**File:** `internal/realtime/visibility_integration_test.go`
**Type:** Integration (`-tags integration`)

**Cases:**
1. Delete event passes through even when user does not satisfy join-policy membership

---

## Component Tests (UI)

### TC-MV-017: Materialized Views Admin Dashboard

**File:** `ui/src/components/__tests__/MatviewsAdmin.test.tsx`
**Type:** Component (vitest + testing-library)

**Cases:**
1. Renders list of registered matviews
2. Shows refresh status badges
3. Refresh button triggers API call
4. Register modal validation
5. Error display for failed operations

## Browser Tests (Unmocked UI)

### TC-MV-018: Materialized Views Browser Lifecycle

**File:** `ui/browser-tests-unmocked/full/matviews-lifecycle.spec.ts`
**Type:** Browser-unmocked (Playwright, real server/DB)

**Cases:**
1. Load-and-verify: seeded matview registration appears in default list view
2. Register flow: register unregistered matview through modal
3. Refresh flow: refresh action succeeds and row status updates to success

---

## Coverage Summary

| Layer | File | Count | Type |
|---|---|---|---|
| Validation | `internal/matview/validate_test.go` | 2 tests | Unit |
| Service | `internal/matview/service_test.go` | 5 tests | Unit |
| Handler | `internal/matview/handler_test.go` | 7 tests | Unit |
| Server | `internal/server/matviews_handler_test.go` | 21 tests | Unit |
| CLI | `internal/cli/matviews_cli_test.go` | 18 tests | Unit |
| Migration | `internal/migrations/matview_sql_test.go` | 1 test | Unit |
| Store | `internal/matview/integration_test.go` | 16 tests | Integration |
| Jobs | `internal/jobs/handlers_test.go` | 1 test | Integration |
| Realtime | `internal/realtime/visibility_integration_test.go` | 3 tests | Integration |
| Migration | `internal/migrations/matview_migrations_integration_test.go` | 1 test | Integration |
| UI | `ui/src/components/__tests__/MatviewsAdmin.test.tsx` | 11 tests | Component |
| UI | `ui/browser-tests-unmocked/full/matviews-lifecycle.spec.ts` | 2 tests | Browser-unmocked |

---

**Last Updated:** Session 083 (2026-02-22)
