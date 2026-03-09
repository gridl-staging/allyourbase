# Realtime (SSE) Test Specification (Tier 2)

**Purpose:** Detailed test cases for realtime Server-Sent Events functionality (BDD Tier 1: B-REAL-001, B-REAL-002)

**Related BDD Stories:**
- [B-REAL-001: Subscribe to Table Changes](../../docs/BDD_SPECIFICATIONS.md#b-real-001-subscribe-to-table-changes)
- [B-REAL-002: OAuth SSE Channel](../../docs/BDD_SPECIFICATIONS.md#b-real-002-oauth-sse-channel)

---

## Test Cases

## Stage 4 Additions (2026-02-22)

These cases are implemented in `internal/realtime/visibility_integration_test.go`.

### TC-REAL-S4-001: Joined-Table RLS Membership Allow/Deny

**Type:** Integration (`-tags integration`)

**Setup:**
1. Create `secure_docs` with RLS enabled.
2. Add `USING (EXISTS (... project_memberships ... current_setting('ayb.user_id', true)))` policy.
3. Insert a secured doc row.

**Assertions:**
1. User with matching membership returns `true` from `canSeeRecord`.
2. User without membership returns `false` from `canSeeRecord`.

### TC-REAL-S4-002: Membership Transition Semantics

**Type:** Integration (`-tags integration`)

**Assertions (same subscription context):**
1. Before membership insert: event filtered (`false`).
2. After membership insert: subsequent event visible (`true`).
3. After membership delete: subsequent event filtered again (`false`).

This verifies evaluation is per-event at delivery time.

### TC-REAL-S4-003: Delete Event Pass-Through

**Type:** Integration (`-tags integration`)

**Assertion:**
1. `delete` events return `true` from `canSeeRecord` even when user does not currently satisfy join-policy membership.

Rationale: deleted rows cannot be checked via `SELECT ... WHERE pk = ...` and delete payloads do not include full sensitive row data.

### TC-REAL-001: Subscribe to Table — INSERT Event

**Story:** B-REAL-001
**Type:** Integration
**Fixture:** `tests/fixtures/realtime/subscribe-insert.json`

**Steps:**
1. Register and login user
2. Connect to `/api/realtime?tables=posts` with SSE (EventSource)
3. Expect `event: connected` with `{message: "Connected to realtime"}`
4. Insert a new post record
5. Expect `event: change` with `{action: "INSERT", table: "posts", record: {...}}`

**Expected SSE Event:**
```
event: change
data: {"action":"INSERT","table":"posts","record":{"id":123,"title":"Test Post","author_id":456,"created_at":"2026-02-13T18:00:00Z"}}
```

**Assertions:**
- SSE connection established
- Received `connected` event
- Received `change` event after INSERT
- Event data contains `action: "INSERT"`
- Event data contains `table: "posts"`
- Event data contains full record with inserted values

---

### TC-REAL-002: Subscribe to Table — UPDATE Event

**Story:** B-REAL-001
**Type:** Integration
**Fixture:** `tests/fixtures/realtime/subscribe-update.json`

**Setup:**
```sql
INSERT INTO posts (id, title, author_id) VALUES (999, 'Original Title', 1);
```

**Steps:**
1. Connect to `/api/realtime?tables=posts` with SSE
2. Update post with id=999
3. Expect `event: change` with `{action: "UPDATE", table: "posts", record: {...}}`

**Expected SSE Event:**
```
event: change
data: {"action":"UPDATE","table":"posts","record":{"id":999,"title":"Updated Title","author_id":1,"updated_at":"2026-02-13T18:01:00Z"}}
```

**Assertions:**
- Received `change` event after UPDATE
- Event data contains `action: "UPDATE"`
- Record reflects updated values

**Cleanup:**
```sql
DELETE FROM posts WHERE id = 999;
```

---

### TC-REAL-003: Subscribe to Table — DELETE Event

**Story:** B-REAL-001
**Type:** Integration
**Fixture:** `tests/fixtures/realtime/subscribe-delete.json`

**Setup:**
```sql
INSERT INTO posts (id, title, author_id) VALUES (888, 'To Be Deleted', 1);
```

**Steps:**
1. Connect to `/api/realtime?tables=posts` with SSE
2. Delete post with id=888
3. Expect `event: change` with `{action: "DELETE", table: "posts", record: {id: 888}}`

**Expected SSE Event:**
```
event: change
data: {"action":"DELETE","table":"posts","record":{"id":888}}
```

**Assertions:**
- Received `change` event after DELETE
- Event data contains `action: "DELETE"`
- Record contains at least the `id` field

---

### TC-REAL-004: Subscribe to Multiple Tables

**Story:** B-REAL-001
**Type:** Integration
**Fixture:** `tests/fixtures/realtime/subscribe-multiple.json`

**Steps:**
1. Connect to `/api/realtime?tables=posts,comments` with SSE
2. Insert a post
3. Insert a comment
4. Expect 2 `change` events (one for posts, one for comments)

**Assertions:**
- Received change event for `table: "posts"`
- Received change event for `table: "comments"`
- Events arrive in correct order

---

### TC-REAL-005: RLS Filtering on Realtime Events

**Story:** B-REAL-001
**Type:** Integration
**Fixture:** `tests/fixtures/realtime/subscribe-rls.json`

**Setup:**
```sql
-- Enable RLS on posts table
ALTER TABLE posts ENABLE ROW LEVEL SECURITY;

-- Policy: users can only see their own posts
CREATE POLICY posts_select_own ON posts FOR SELECT USING (author_id = current_user_id());
```

**Steps:**
1. Register User A (id=100)
2. Register User B (id=200)
3. User A connects to `/api/realtime?tables=posts` with their token
4. Insert post by User A (author_id=100)
5. Insert post by User B (author_id=200)
6. Expect User A to receive ONLY the event for their own post

**Expected Behavior:**
- User A receives `change` event for post with author_id=100
- User A does NOT receive `change` event for post with author_id=200

**Assertions:**
- RLS policies enforced on realtime events
- User only sees events for authorized records

**Cleanup:**
```sql
DROP POLICY posts_select_own ON posts;
ALTER TABLE posts DISABLE ROW LEVEL SECURITY;
```

---

### TC-REAL-006: Realtime — Invalid Token

**Story:** B-REAL-001
**Type:** Integration

**Steps:**
1. Connect to `/api/realtime?tables=posts` with invalid/expired token
2. Expect 401 Unauthorized (connection rejected)

**Expected Response:**
- HTTP 401 status
- Error message: `"Invalid or expired token"`

**Assertions:**
- Connection is rejected
- No SSE events received

---

### TC-REAL-007: Realtime — Non-Existent Table

**Story:** B-REAL-001
**Type:** Integration

**Steps:**
1. Connect to `/api/realtime?tables=non_existent_table` with valid token
2. Expect connection to succeed (no error)
3. Insert into another table (e.g., posts)
4. Expect NO events received (table filter doesn't match)

**Assertions:**
- Connection succeeds
- No events for non-matching table

---

### TC-REAL-008: OAuth SSE — Happy Path

**Story:** B-REAL-002
**Type:** Integration + Contract
**Fixture:** `tests/fixtures/oauth/sse-happy-path.json`

**Steps:**
1. Connect to `/api/realtime?oauth=true` (no auth token required)
2. Expect `event: connected` with `{clientId: "abc123"}`
3. Simulate OAuth callback: POST `/api/auth/oauth/google/callback?state=abc123&code=auth_code`
4. Expect `event: oauth` with `{token, refreshToken, user}`
5. SSE connection closes automatically

**Expected SSE Events:**
```
event: connected
data: {"clientId":"abc123"}

event: oauth
data: {"token":"<jwt>","refreshToken":"<refresh-jwt>","user":{"id":"u123","email":"test@example.com"}}
```

**Assertions:**
- Received `connected` event with unique clientId
- Received `oauth` event with token, refreshToken, user
- Connection closes after oauth event

---

### TC-REAL-009: OAuth SSE — Provider Error

**Story:** B-REAL-002
**Type:** Integration + Contract
**Fixture:** `tests/fixtures/oauth/sse-error.json`

**Steps:**
1. Connect to `/api/realtime?oauth=true`
2. Expect `event: connected` with `{clientId: "xyz789"}`
3. Simulate OAuth error: POST `/api/auth/oauth/google/callback?state=xyz789&error=access_denied`
4. Expect `event: oauth` with `{error: "oauth/provider-error", message: "..."}`

**Expected SSE Event:**
```
event: oauth
data: {"error":"oauth/provider-error","message":"User denied access"}
```

**Assertions:**
- Received `oauth` event with error
- Connection closes after error event

---

### TC-REAL-010: OAuth SSE — Expired ClientID

**Story:** B-REAL-002
**Type:** Integration
**Fixture:** `tests/fixtures/oauth/sse-expired-client.json`

**Steps:**
1. Connect to `/api/realtime?oauth=true`
2. Receive clientId
3. Wait 10 minutes (clientId expiration timeout)
4. Simulate OAuth callback with expired clientId
5. Expect `event: oauth` with `{error: "oauth/timeout"}`

**Assertions:**
- Expired clientId returns error via SSE
- Error code = `oauth/timeout`

---

## Browser Test Coverage (Unmocked)

### Implemented Browser Tests

None currently implemented.

### Missing Browser Tests

1. **Realtime Table Subscription** (covers TC-REAL-001, TC-REAL-002, TC-REAL-003)
   - Create a post via UI
   - Subscribe to realtime updates
   - Update/delete post in another browser tab
   - Verify UI updates in real-time

2. **OAuth Popup Flow** (covers TC-REAL-008, TC-REAL-009)
   - Click "Sign in with Google"
   - Popup opens, SSE connection established
   - User completes OAuth flow
   - Verify token received via SSE
   - Verify popup closes automatically

---

## Fixture Requirements

1. `tests/fixtures/realtime/subscribe-insert.json` — Insert event test
2. `tests/fixtures/realtime/subscribe-update.json` — Update event test
3. `tests/fixtures/realtime/subscribe-delete.json` — Delete event test
4. `tests/fixtures/realtime/subscribe-multiple.json` — Multiple table subscription
5. `tests/fixtures/realtime/subscribe-rls.json` — RLS policy enforcement
6. `tests/fixtures/oauth/sse-happy-path.json` — Successful OAuth via SSE
7. `tests/fixtures/oauth/sse-error.json` — OAuth error via SSE
8. `tests/fixtures/oauth/sse-expired-client.json` — Expired clientId

---

## Test Tags

- `realtime`
- `sse`
- `oauth`
- `rls`
- `websockets-alternative`

---

**Last Updated:** Session 079 (2026-02-13)
