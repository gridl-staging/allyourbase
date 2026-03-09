# Webhooks Test Specification (Tier 2)

**Purpose:** Detailed test cases for webhook event notification functionality (BDD Tier 1: B-HOOK-001 through B-HOOK-005)

**Related BDD Stories:**
- [B-HOOK-001: Create Webhook](../../docs/BDD_SPECIFICATIONS.md#b-hook-001-create-webhook)
- [B-HOOK-002: List Webhooks](../../docs/BDD_SPECIFICATIONS.md#b-hook-002-list-webhooks)
- [B-HOOK-003: Update Webhook](../../docs/BDD_SPECIFICATIONS.md#b-hook-003-update-webhook)
- [B-HOOK-004: Delete Webhook](../../docs/BDD_SPECIFICATIONS.md#b-hook-004-delete-webhook)
- [B-HOOK-005: Webhook Delivery](../../docs/BDD_SPECIFICATIONS.md#b-hook-005-webhook-delivery)

---

## Test Cases

### TC-HOOK-001: Create Webhook — Happy Path

**Story:** B-HOOK-001
**Type:** Integration
**Fixture:** `tests/fixtures/webhooks/create-valid.json`

**Request:**
```json
{
  "url": "https://example.com/webhook",
  "secret": "my-secret-key",
  "events": ["INSERT", "UPDATE"],
  "tables": ["posts", "comments"],
  "enabled": true
}
```

**Expected Response (201 Created):**
```json
{
  "id": 1,
  "url": "https://example.com/webhook",
  "secret": "my-secret-key",
  "events": ["INSERT", "UPDATE"],
  "tables": ["posts", "comments"],
  "enabled": true,
  "created_at": "2026-02-13T18:00:00Z",
  "updated_at": "2026-02-13T18:00:00Z"
}
```

**Assertions:**
- Response status = 201
- Response contains webhook object with id
- All provided fields match request
- `created_at` and `updated_at` are set

---

### TC-HOOK-002: Create Webhook — Invalid URL

**Story:** B-HOOK-001
**Type:** Integration
**Fixture:** `tests/fixtures/webhooks/create-invalid-url.json`

**Request:**
```json
{
  "url": "not-a-valid-url",
  "enabled": true
}
```

**Expected Response (400 Bad Request):**
```json
{
  "error": "Invalid URL format"
}
```

**Assertions:**
- Response status = 400
- Error message indicates invalid URL

---

### TC-HOOK-003: Create Webhook — Non-Admin User

**Story:** B-HOOK-001
**Type:** Integration
**Fixture:** `tests/fixtures/webhooks/create-non-admin.json`

**Steps:**
1. Register regular user
2. POST `/api/webhooks` with user token (non-admin)
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

### TC-HOOK-004: List Webhooks — Happy Path

**Story:** B-HOOK-002
**Type:** Integration
**Fixture:** `tests/fixtures/webhooks/list-all.json`

**Setup:**
```sql
-- Create 3 test webhooks
INSERT INTO webhooks (url, enabled) VALUES
  ('https://example.com/webhook1', true),
  ('https://example.com/webhook2', false),
  ('https://example.com/webhook3', true);
```

**Request:**
```
GET /api/webhooks
```

**Expected Response (200 OK):**
```json
[
  {
    "id": 1,
    "url": "https://example.com/webhook1",
    "enabled": true,
    "created_at": "...",
    "updated_at": "..."
  },
  {
    "id": 2,
    "url": "https://example.com/webhook2",
    "enabled": false,
    "created_at": "...",
    "updated_at": "..."
  },
  {
    "id": 3,
    "url": "https://example.com/webhook3",
    "enabled": true,
    "created_at": "...",
    "updated_at": "..."
  }
]
```

**Assertions:**
- Response status = 200
- Response is array with 3 webhooks
- Each webhook has id, url, enabled, created_at, updated_at

**Cleanup:**
```sql
DELETE FROM webhooks WHERE url LIKE 'https://example.com/webhook%';
```

---

### TC-HOOK-005: List Webhooks — Non-Admin User

**Story:** B-HOOK-002
**Type:** Integration

**Steps:**
1. Register regular user
2. GET `/api/webhooks` with user token (non-admin)
3. Expect 401 Unauthorized

**Assertions:**
- Response status = 401

---

### TC-HOOK-006: Update Webhook — Happy Path

**Story:** B-HOOK-003
**Type:** Integration
**Fixture:** `tests/fixtures/webhooks/update-valid.json`

**Setup:**
```sql
INSERT INTO webhooks (id, url, enabled) VALUES (99, 'https://example.com/old', true);
```

**Request:**
```
PATCH /api/webhooks/99
```
```json
{
  "url": "https://example.com/new",
  "enabled": false
}
```

**Expected Response (200 OK):**
```json
{
  "id": 99,
  "url": "https://example.com/new",
  "enabled": false,
  "created_at": "...",
  "updated_at": "..."
}
```

**Assertions:**
- Response status = 200
- `url` updated to new value
- `enabled` updated to false
- `updated_at` timestamp updated

**Cleanup:**
```sql
DELETE FROM webhooks WHERE id = 99;
```

---

### TC-HOOK-007: Update Webhook — Non-Existent

**Story:** B-HOOK-003
**Type:** Integration

**Request:**
```
PATCH /api/webhooks/99999
```
```json
{
  "enabled": false
}
```

**Expected Response (404 Not Found):**
```json
{
  "error": "Webhook not found"
}
```

**Assertions:**
- Response status = 404

---

### TC-HOOK-008: Delete Webhook — Happy Path

**Story:** B-HOOK-004
**Type:** Integration
**Fixture:** `tests/fixtures/webhooks/delete-valid.json`

**Setup:**
```sql
INSERT INTO webhooks (id, url) VALUES (88, 'https://example.com/to-delete');
```

**Request:**
```
DELETE /api/webhooks/88
```

**Expected Response (204 No Content):**
- Empty body

**Assertions:**
- Response status = 204
- Webhook with id=88 no longer exists in database

---

### TC-HOOK-009: Delete Webhook — Non-Existent

**Story:** B-HOOK-004
**Type:** Integration

**Request:**
```
DELETE /api/webhooks/88888
```

**Expected Response (404 Not Found):**
```json
{
  "error": "Webhook not found"
}
```

**Assertions:**
- Response status = 404

---

### TC-HOOK-010: Webhook Delivery — INSERT Event

**Story:** B-HOOK-005
**Type:** Integration
**Fixture:** `tests/fixtures/webhooks/delivery-insert.json`

**Setup:**
1. Create webhook: `{url: "http://localhost:9999/webhook", events: ["INSERT"], tables: ["posts"], enabled: true, secret: "test-secret"}`
2. Start mock HTTP server on port 9999

**Steps:**
1. Insert a post record
2. Wait for webhook delivery (max 5 seconds)
3. Verify webhook received POST request

**Expected Webhook Payload:**
```json
{
  "event": "INSERT",
  "table": "posts",
  "record": {
    "id": 123,
    "title": "Test Post",
    "created_at": "2026-02-13T18:00:00Z"
  },
  "timestamp": "2026-02-13T18:00:01Z"
}
```

**Expected Headers:**
```
Content-Type: application/json
X-Webhook-Signature: sha256=<hmac-signature>
```

**Assertions:**
- Webhook received POST request
- Payload contains correct event, table, record
- `X-Webhook-Signature` header present
- Signature validates with HMAC-SHA256(payload, secret)

---

### TC-HOOK-011: Webhook Delivery — Event Filtering

**Story:** B-HOOK-005
**Type:** Integration
**Fixture:** `tests/fixtures/webhooks/delivery-filter-events.json`

**Setup:**
1. Create webhook: `{url: "http://localhost:9999/webhook", events: ["INSERT"], enabled: true}`

**Steps:**
1. Insert a post (should trigger webhook)
2. Update a post (should NOT trigger webhook)
3. Verify webhook received only 1 request (INSERT)

**Assertions:**
- Webhook received exactly 1 POST request
- Only INSERT event delivered
- UPDATE event filtered out

---

### TC-HOOK-012: Webhook Delivery — Table Filtering

**Story:** B-HOOK-005
**Type:** Integration
**Fixture:** `tests/fixtures/webhooks/delivery-filter-tables.json`

**Setup:**
1. Create webhook: `{url: "http://localhost:9999/webhook", tables: ["posts"], enabled: true}`

**Steps:**
1. Insert into posts table (should trigger webhook)
2. Insert into comments table (should NOT trigger webhook)
3. Verify webhook received only 1 request (posts)

**Assertions:**
- Webhook received exactly 1 POST request
- Only posts table event delivered
- comments table event filtered out

---

### TC-HOOK-013: Webhook Delivery — Retry on Failure

**Story:** B-HOOK-005
**Type:** Integration
**Fixture:** `tests/fixtures/webhooks/delivery-retry.json`

**Setup:**
1. Create webhook: `{url: "http://localhost:9999/webhook", enabled: true}`
2. Configure mock server to return 500 Internal Server Error on first 2 requests, then 200 OK

**Steps:**
1. Insert a post record
2. Wait for webhook delivery with retries
3. Verify webhook received 3 POST requests (2 failures + 1 success)

**Expected Retry Behavior:**
- Attempt 1: 500 error → retry after 1s
- Attempt 2: 500 error → retry after 2s (exponential backoff)
- Attempt 3: 200 success → stop retrying

**Assertions:**
- Mock server received 3 requests
- Exponential backoff applied (1s, 2s, 4s, 8s, 16s intervals)
- Delivery marked as success after attempt 3

---

### TC-HOOK-014: Webhook Delivery — Max Retries

**Story:** B-HOOK-005
**Type:** Integration
**Fixture:** `tests/fixtures/webhooks/delivery-max-retries.json`

**Setup:**
1. Create webhook: `{url: "http://localhost:9999/webhook", enabled: true}`
2. Configure mock server to always return 500 error

**Steps:**
1. Insert a post record
2. Wait for all retry attempts to complete
3. Verify webhook delivery marked as failed after 5 attempts

**Assertions:**
- Mock server received 5 POST requests (initial + 4 retries)
- Delivery marked as failed in logs/database
- No further retries after max attempts reached

---

### TC-HOOK-015: Webhook Delivery — Disabled Webhook

**Story:** B-HOOK-005
**Type:** Integration
**Fixture:** `tests/fixtures/webhooks/delivery-disabled.json`

**Setup:**
1. Create webhook: `{url: "http://localhost:9999/webhook", enabled: false}`

**Steps:**
1. Insert a post record
2. Wait 5 seconds
3. Verify webhook received NO requests

**Assertions:**
- Mock server received 0 requests
- Disabled webhooks are skipped

---

## Browser Test Coverage (Unmocked)

### Implemented Browser Tests

None currently implemented.

### Missing Browser Tests

1. **Create Webhook via UI** (covers TC-HOOK-001)
   - Login as admin
   - Navigate to Webhooks page
   - Click "Add Webhook"
   - Fill in URL, events, tables
   - Submit form
   - Verify webhook appears in list

2. **Update Webhook via UI** (covers TC-HOOK-006)
   - Navigate to existing webhook
   - Click "Edit"
   - Change URL or enabled status
   - Save changes
   - Verify updates reflected

3. **Delete Webhook via UI** (covers TC-HOOK-008)
   - Navigate to webhook list
   - Click "Delete" on webhook
   - Confirm deletion
   - Verify webhook removed from list

4. **View Webhook Delivery History** (covers TC-HOOK-010)
   - Navigate to webhook details
   - View delivery history
   - Verify successful/failed deliveries shown

---

## Fixture Requirements

1. `tests/fixtures/webhooks/create-valid.json` — Valid webhook creation
2. `tests/fixtures/webhooks/create-invalid-url.json` — Invalid URL format
3. `tests/fixtures/webhooks/create-non-admin.json` — Non-admin attempt
4. `tests/fixtures/webhooks/list-all.json` — List all webhooks
5. `tests/fixtures/webhooks/update-valid.json` — Update webhook fields
6. `tests/fixtures/webhooks/delete-valid.json` — Delete webhook
7. `tests/fixtures/webhooks/delivery-insert.json` — Webhook delivery on INSERT
8. `tests/fixtures/webhooks/delivery-filter-events.json` — Event filtering
9. `tests/fixtures/webhooks/delivery-filter-tables.json` — Table filtering
10. `tests/fixtures/webhooks/delivery-retry.json` — Retry on failure
11. `tests/fixtures/webhooks/delivery-max-retries.json` — Max retries exceeded
12. `tests/fixtures/webhooks/delivery-disabled.json` — Disabled webhook skipped

---

## Test Tags

- `webhooks`
- `admin`
- `event-delivery`
- `retry`
- `hmac`
- `filtering`

---

**Last Updated:** Session 079 (2026-02-13)
