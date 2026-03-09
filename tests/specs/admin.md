# Admin Test Specification (Tier 2)

**Purpose:** Detailed test cases for admin dashboard functionality (BDD Tier 1: B-ADMIN-001, B-ADMIN-002)

**Related BDD Stories:**
- [B-ADMIN-001: Admin Login](../../docs/BDD_SPECIFICATIONS.md#b-admin-001-admin-login)
- [B-ADMIN-002: Execute SQL Query](../../docs/BDD_SPECIFICATIONS.md#b-admin-002-execute-sql-query)

---

## Test Cases

### TC-ADMIN-001: Admin Login — Happy Path

**Story:** B-ADMIN-001
**Type:** Integration
**Fixture:** `tests/fixtures/admin/login-valid.json`

**Steps:**
1. POST `/api/admin/login` with valid admin password
2. Expect 200 OK
3. Expect response to contain JWT token
4. Expect token payload to contain `{ role: "admin" }`

**Expected Response:**
```json
{
  "token": "<jwt-token>"
}
```

**Assertions:**
- Response status = 200
- Response contains `token` field
- Token is valid JWT
- Token payload contains `role: "admin"`

---

### TC-ADMIN-002: Admin Login — Invalid Password

**Story:** B-ADMIN-001
**Type:** Integration
**Fixture:** `tests/fixtures/admin/login-invalid.json`

**Steps:**
1. POST `/api/admin/login` with incorrect password
2. Expect 401 Unauthorized

**Expected Response:**
```json
{
  "error": "Invalid password"
}
```

**Assertions:**
- Response status = 401
- Response contains error message

---

### TC-ADMIN-003: Admin Login — Rate Limiting

**Story:** B-ADMIN-001
**Type:** Integration
**Fixture:** `tests/fixtures/admin/login-rate-limit.json`

**Steps:**
1. POST `/api/admin/login` with invalid password 10 times
2. On 11th attempt, expect 429 Too Many Requests

**Expected Response:**
```json
{
  "error": "Too many login attempts. Try again later."
}
```

**Assertions:**
- First 10 requests return 401
- 11th request returns 429
- Error message indicates rate limit

---

### TC-ADMIN-004: Execute SQL — SELECT Query

**Story:** B-ADMIN-002
**Type:** Integration
**Fixture:** `tests/fixtures/admin/sql-select.json`

**Steps:**
1. POST `/api/admin/sql` with admin token and SELECT query
2. Expect 200 OK with query results

**Request:**
```json
{
  "query": "SELECT 1 AS test_column, 'hello' AS message"
}
```

**Expected Response:**
```json
{
  "columns": ["test_column", "message"],
  "rows": [
    {"test_column": 1, "message": "hello"}
  ],
  "rowCount": 1,
  "durationMs": 5
}
```

**Assertions:**
- Response status = 200
- Response contains `columns`, `rows`, `rowCount`, `durationMs`
- `columns` = `["test_column", "message"]`
- `rows[0].test_column` = 1
- `rows[0].message` = "hello"
- `rowCount` = 1

---

### TC-ADMIN-005: Execute SQL — INSERT Query

**Story:** B-ADMIN-002
**Type:** Integration
**Fixture:** `tests/fixtures/admin/sql-insert.json`

**Setup:**
```sql
CREATE TABLE test_table (id SERIAL PRIMARY KEY, name TEXT);
```

**Request:**
```json
{
  "query": "INSERT INTO test_table (name) VALUES ('Alice'), ('Bob')"
}
```

**Expected Response:**
```json
{
  "columns": [],
  "rows": [],
  "rowCount": 2,
  "durationMs": 8
}
```

**Assertions:**
- Response status = 200
- `rowCount` = 2
- `columns` and `rows` are empty

**Cleanup:**
```sql
DROP TABLE test_table;
```

---

### TC-ADMIN-006: Execute SQL — SQL Error

**Story:** B-ADMIN-002
**Type:** Integration
**Fixture:** `tests/fixtures/admin/sql-error.json`

**Request:**
```json
{
  "query": "SELECT * FROM non_existent_table"
}
```

**Expected Response:**
```json
{
  "error": "relation \"non_existent_table\" does not exist"
}
```

**Assertions:**
- Response status = 400
- Response contains error message about non-existent table

---

### TC-ADMIN-007: Execute SQL — Non-Admin User

**Story:** B-ADMIN-002
**Type:** Integration
**Fixture:** `tests/fixtures/admin/sql-non-admin.json`

**Steps:**
1. Register a regular user
2. POST `/api/admin/sql` with user token (non-admin)
3. Expect 401 Unauthorized

**Expected Response:**
```json
{
  "error": "Admin authentication required"
}
```

**Assertions:**
- Response status = 401
- Error message indicates admin required

---

### TC-ADMIN-008: Execute SQL — No Token

**Story:** B-ADMIN-002
**Type:** Integration

**Steps:**
1. POST `/api/admin/sql` without Authorization header
2. Expect 401 Unauthorized

**Expected Response:**
```json
{
  "error": "Missing admin token"
}
```

**Assertions:**
- Response status = 401
- Error message indicates missing token

---

## Browser tests (unmocked) Test Coverage

### Implemented Browser tests (unmocked) Tests

1. **Admin SQL Query** (`ui/browser-tests-unmocked/smoke/admin-sql-query.spec.ts`)
   - Covers TC-ADMIN-004 (SELECT query)
   - Fixed in session 078 (strict mode violation)

### Missing Browser tests (unmocked) Tests

1. **Admin Login Flow** (covers TC-ADMIN-001, TC-ADMIN-002)
   - Navigate to admin dashboard
   - Enter admin password
   - Click "Login"
   - Verify redirect to dashboard

2. **SQL Query Execution Flow** (covers TC-ADMIN-005)
   - Login as admin
   - Navigate to SQL tab
   - Enter INSERT query
   - Execute and verify rowCount displayed

3. **SQL Error Handling** (covers TC-ADMIN-006)
   - Login as admin
   - Navigate to SQL tab
   - Enter invalid SQL
   - Verify error message displayed

---

## Stage 1 Additions: Per-App API Key Scoping

These test cases define required coverage for app-scoped API keys and app-level
rate limiting introduced in Stage 1.

### TC-ADMIN-APP-001: Apps CRUD via Admin API

**Type:** Integration  
**Coverage target:** `internal/server/apps_handler_test.go`, `internal/auth/apps_test.go`

**Steps:**
1. Create app with owner user id and optional description.
2. List apps with pagination params.
3. Fetch created app by id.
4. Update app name/description/rate-limit fields.
5. Delete app and verify subsequent get returns not found.

**Assertions:**
- CRUD status codes and payloads are correct.
- Invalid UUIDs return 400.
- Missing owner user returns 404.
- Negative rate-limit values return validation error.

### TC-ADMIN-APP-002: Create App-Scoped API Key

**Type:** Integration + Component + Browser (unmocked)  
**Coverage target:** `internal/server/apikeys_handler_test.go`, `ui/src/components/__tests__/ApiKeys.test.tsx`, `ui/browser-tests-unmocked/full/api-keys-lifecycle.spec.ts`

**Steps:**
1. Create app and user fixtures.
2. Create API key with `appId`.
3. Verify response includes key and `apiKey.appId`.
4. Verify admin UI create modal allows selecting app scope.
5. Verify created modal and list row show app name and rate limit.

**Assertions:**
- `appId` is persisted and returned.
- App metadata is rendered in UI.
- User-scoped option remains available for legacy behavior.

### TC-ADMIN-APP-003: Scope Enforcement for App-Scoped Keys

**Type:** Integration  
**Coverage target:** `internal/server/admin_middleware_test.go`, `internal/auth/apikeys_test.go`

**Steps:**
1. Create app-scoped key with restricted table/write permissions.
2. Attempt allowed operation.
3. Attempt out-of-scope table/operation.

**Assertions:**
- Allowed operations succeed.
- Out-of-scope operations are denied.
- Legacy keys with `appId = null` keep backward-compatible behavior.

### TC-ADMIN-APP-004: App Rate-Limit Enforcement

**Type:** Integration  
**Coverage target:** `internal/auth/app_ratelimit_test.go`, `internal/server/admin_middleware_test.go`

**Steps:**
1. Create app with finite rate-limit settings.
2. Send requests with app-scoped key until threshold exceeded.
3. Send request with admin token for same route.

**Assertions:**
- Over-limit request returns 429 and retry metadata.
- Rate limiting is isolated per app id.
- Admin-authenticated requests bypass app limiter.

### TC-ADMIN-APP-005: Browser-Unmocked Lifecycle (App-Scoped Key)

**Type:** Browser (unmocked)  
**Coverage target:** `ui/browser-tests-unmocked/full/api-keys-lifecycle.spec.ts`

**Steps:**
1. Seed user/app/key via fixture helper APIs.
2. Navigate to API Keys view and verify seeded row renders.
3. Create key through UI selecting app scope.
4. Revoke key through UI.

**Assertions:**
- No direct API shortcuts are used in spec act/assert phases.
- App name and app rate-limit render in modal and list row.
- Revoked status is visible after confirmation.

---

## Fixture Requirements

1. `tests/fixtures/admin/login-valid.json` — Valid admin password
2. `tests/fixtures/admin/login-invalid.json` — Incorrect password
3. `tests/fixtures/admin/login-rate-limit.json` — Multiple login attempts
4. `tests/fixtures/admin/sql-select.json` — SELECT query
5. `tests/fixtures/admin/sql-insert.json` — INSERT query with setup/cleanup
6. `tests/fixtures/admin/sql-error.json` — Invalid SQL query
7. `tests/fixtures/admin/sql-non-admin.json` — Regular user token

---

## Test Tags

- `admin`
- `authentication`
- `sql`
- `rate-limiting`
- `authorization`

---

**Last Updated:** Session 079 (2026-02-13)
