# RPC (Remote Procedure Call) Test Specification (Tier 2)

**Purpose:** Detailed test cases for calling PostgreSQL functions via REST API (BDD Tier 1: B-RPC-001)

**Related BDD Stories:**
- [B-RPC-001: Call PostgreSQL Function](../../docs/BDD_SPECIFICATIONS.md#b-rpc-001-call-postgresql-function)

---

## Test Cases

### TC-RPC-001: Call Function — Void Function

**Story:** B-RPC-001
**Type:** Integration
**Fixture:** `tests/fixtures/rpc/call-void.json`

**Setup:**
```sql
CREATE OR REPLACE FUNCTION increment_counter(counter_name TEXT)
RETURNS VOID AS $$
BEGIN
  UPDATE counters SET value = value + 1 WHERE name = counter_name;
END;
$$ LANGUAGE plpgsql;

CREATE TABLE counters (name TEXT PRIMARY KEY, value INTEGER);
INSERT INTO counters VALUES ('test_counter', 0);
```

**Request:**
```
POST /api/rpc/increment_counter
```
```json
{
  "counter_name": "test_counter"
}
```

**Expected Response (204 No Content):**
- Empty body

**Assertions:**
- Response status = 204
- No response body
- Database updated: `SELECT value FROM counters WHERE name='test_counter'` returns 1

**Cleanup:**
```sql
DROP FUNCTION increment_counter;
DROP TABLE counters;
```

---

### TC-RPC-002: Call Function — Scalar Return

**Story:** B-RPC-001
**Type:** Integration
**Fixture:** `tests/fixtures/rpc/call-scalar.json`

**Setup:**
```sql
CREATE OR REPLACE FUNCTION add_numbers(a INTEGER, b INTEGER)
RETURNS INTEGER AS $$
BEGIN
  RETURN a + b;
END;
$$ LANGUAGE plpgsql;
```

**Request:**
```
POST /api/rpc/add_numbers
```
```json
{
  "a": 5,
  "b": 7
}
```

**Expected Response (200 OK):**
```json
12
```

**Assertions:**
- Response status = 200
- Response body is unwrapped scalar value (not `{result: 12}`)
- Response = 12 (number type)

**Cleanup:**
```sql
DROP FUNCTION add_numbers;
```

---

### TC-RPC-003: Call Function — Set-Returning Function

**Story:** B-RPC-001
**Type:** Integration
**Fixture:** `tests/fixtures/rpc/call-setof.json`

**Setup:**
```sql
CREATE OR REPLACE FUNCTION get_active_users()
RETURNS TABLE(id INTEGER, email TEXT, created_at TIMESTAMP) AS $$
BEGIN
  RETURN QUERY SELECT u.id, u.email, u.created_at FROM ayb_users u WHERE u.email_verified = true;
END;
$$ LANGUAGE plpgsql;

-- Insert test data
INSERT INTO ayb_users (id, email, email_verified, password_hash, created_at) VALUES
  (1, 'alice@example.com', true, 'hash1', '2026-01-01 10:00:00'),
  (2, 'bob@example.com', false, 'hash2', '2026-01-02 11:00:00'),
  (3, 'charlie@example.com', true, 'hash3', '2026-01-03 12:00:00');
```

**Request:**
```
POST /api/rpc/get_active_users
```
```json
{}
```

**Expected Response (200 OK):**
```json
[
  {
    "id": 1,
    "email": "alice@example.com",
    "created_at": "2026-01-01T10:00:00Z"
  },
  {
    "id": 3,
    "email": "charlie@example.com",
    "created_at": "2026-01-03T12:00:00Z"
  }
]
```

**Assertions:**
- Response status = 200
- Response is array of records
- Only verified users (alice, charlie) returned
- bob (email_verified=false) excluded

**Cleanup:**
```sql
DROP FUNCTION get_active_users;
DELETE FROM ayb_users WHERE id IN (1, 2, 3);
```

---

### TC-RPC-004: Call Function — Non-Existent Function

**Story:** B-RPC-001
**Type:** Integration

**Request:**
```
POST /api/rpc/non_existent_function
```
```json
{
  "arg": "value"
}
```

**Expected Response (404 Not Found):**
```json
{
  "error": "Function not found: non_existent_function"
}
```

**Assertions:**
- Response status = 404
- Error message indicates function not found

---

### TC-RPC-005: Call Function — Wrong Argument Types

**Story:** B-RPC-001
**Type:** Integration
**Fixture:** `tests/fixtures/rpc/call-wrong-types.json`

**Setup:**
```sql
CREATE OR REPLACE FUNCTION calculate_age(birth_year INTEGER)
RETURNS INTEGER AS $$
BEGIN
  RETURN EXTRACT(YEAR FROM NOW()) - birth_year;
END;
$$ LANGUAGE plpgsql;
```

**Request:**
```
POST /api/rpc/calculate_age
```
```json
{
  "birth_year": "not-a-number"
}
```

**Expected Response (400 Bad Request):**
```json
{
  "error": "Invalid argument type for parameter 'birth_year': expected integer, got string"
}
```

**Assertions:**
- Response status = 400
- Error message indicates type mismatch

**Cleanup:**
```sql
DROP FUNCTION calculate_age;
```

---

### TC-RPC-006: Call Function — Missing Required Argument

**Story:** B-RPC-001
**Type:** Integration
**Fixture:** `tests/fixtures/rpc/call-missing-arg.json`

**Setup:**
```sql
CREATE OR REPLACE FUNCTION greet(name TEXT)
RETURNS TEXT AS $$
BEGIN
  RETURN 'Hello, ' || name || '!';
END;
$$ LANGUAGE plpgsql;
```

**Request:**
```
POST /api/rpc/greet
```
```json
{}
```

**Expected Response (400 Bad Request):**
```json
{
  "error": "Missing required argument: name"
}
```

**Assertions:**
- Response status = 400
- Error message indicates missing argument

**Cleanup:**
```sql
DROP FUNCTION greet;
```

---

### TC-RPC-007: Call Function — Optional Arguments (Defaults)

**Story:** B-RPC-001
**Type:** Integration
**Fixture:** `tests/fixtures/rpc/call-default-args.json`

**Setup:**
```sql
CREATE OR REPLACE FUNCTION power(base INTEGER, exponent INTEGER DEFAULT 2)
RETURNS INTEGER AS $$
BEGIN
  RETURN base ^ exponent;
END;
$$ LANGUAGE plpgsql;
```

**Request 1 (with default):**
```
POST /api/rpc/power
```
```json
{
  "base": 5
}
```

**Expected Response (200 OK):**
```json
25
```

**Request 2 (override default):**
```
POST /api/rpc/power
```
```json
{
  "base": 2,
  "exponent": 8
}
```

**Expected Response (200 OK):**
```json
256
```

**Assertions:**
- When `exponent` omitted, default value (2) is used
- When `exponent` provided, custom value is used

**Cleanup:**
```sql
DROP FUNCTION power;
```

---

### TC-RPC-008: Call Function — RLS Policy Enforcement

**Story:** B-RPC-001
**Type:** Integration
**Fixture:** `tests/fixtures/rpc/call-rls.json`

**Setup:**
```sql
-- Enable RLS on posts table
ALTER TABLE posts ENABLE ROW LEVEL SECURITY;

-- Policy: users can only see their own posts
CREATE POLICY posts_select_own ON posts FOR SELECT USING (author_id = current_user_id());

-- Create function that queries posts
CREATE OR REPLACE FUNCTION get_my_posts()
RETURNS TABLE(id INTEGER, title TEXT) AS $$
BEGIN
  RETURN QUERY SELECT p.id, p.title FROM posts p;
END;
$$ LANGUAGE plpgsql;

-- Insert test data
INSERT INTO posts (id, title, author_id) VALUES
  (1, 'Post by User 100', 100),
  (2, 'Post by User 200', 200);
```

**Steps:**
1. Register User A (id=100)
2. Login as User A
3. POST `/api/rpc/get_my_posts` with User A's token

**Expected Response (200 OK):**
```json
[
  {
    "id": 1,
    "title": "Post by User 100"
  }
]
```

**Assertions:**
- User A receives only their own posts (author_id=100)
- User A does NOT see posts by User 200
- RLS policies enforced in function context

**Cleanup:**
```sql
DROP FUNCTION get_my_posts;
DROP POLICY posts_select_own ON posts;
ALTER TABLE posts DISABLE ROW LEVEL SECURITY;
DELETE FROM posts WHERE id IN (1, 2);
```

---

### TC-RPC-009: Call Function — SQL Injection Protection

**Story:** B-RPC-001
**Type:** Integration
**Fixture:** `tests/fixtures/rpc/call-sql-injection.json`

**Setup:**
```sql
CREATE OR REPLACE FUNCTION search_users(query TEXT)
RETURNS TABLE(id INTEGER, email TEXT) AS $$
BEGIN
  -- Vulnerable if not properly parameterized
  RETURN QUERY EXECUTE 'SELECT id, email FROM ayb_users WHERE email LIKE ''%' || query || '%''';
END;
$$ LANGUAGE plpgsql;
```

**Request:**
```
POST /api/rpc/search_users
```
```json
{
  "query": "'; DROP TABLE ayb_users; --"
}
```

**Expected Behavior:**
- Function should safely handle malicious input
- No tables should be dropped
- Query should be parameterized or escaped

**Assertions:**
- Response status = 200 or 400 (not 500)
- `ayb_users` table still exists after request
- No SQL injection vulnerability

**Cleanup:**
```sql
DROP FUNCTION search_users;
```

---

### TC-RPC-010: Call Function — Complex Return Type (JSONB)

**Story:** B-RPC-001
**Type:** Integration
**Fixture:** `tests/fixtures/rpc/call-jsonb.json`

**Setup:**
```sql
CREATE OR REPLACE FUNCTION get_user_stats(user_id INTEGER)
RETURNS JSONB AS $$
BEGIN
  RETURN jsonb_build_object(
    'total_posts', (SELECT COUNT(*) FROM posts WHERE author_id = user_id),
    'total_comments', (SELECT COUNT(*) FROM comments WHERE user_id = user_id),
    'joined_at', (SELECT created_at FROM ayb_users WHERE id = user_id)
  );
END;
$$ LANGUAGE plpgsql;

-- Insert test data
INSERT INTO ayb_users (id, email, password_hash, created_at) VALUES
  (999, 'testuser@example.com', 'hash', '2026-01-01 10:00:00');
INSERT INTO posts (author_id) VALUES (999), (999);
INSERT INTO comments (user_id) VALUES (999);
```

**Request:**
```
POST /api/rpc/get_user_stats
```
```json
{
  "user_id": 999
}
```

**Expected Response (200 OK):**
```json
{
  "total_posts": 2,
  "total_comments": 1,
  "joined_at": "2026-01-01T10:00:00Z"
}
```

**Assertions:**
- Response status = 200
- JSONB response is unwrapped (not `{"result": {...}}`)
- Response contains correct stats

**Cleanup:**
```sql
DROP FUNCTION get_user_stats;
DELETE FROM comments WHERE user_id = 999;
DELETE FROM posts WHERE author_id = 999;
DELETE FROM ayb_users WHERE id = 999;
```

---

## Browser tests (unmocked) Test Coverage

### Implemented Browser tests (unmocked) Tests

1. **Function Browser** (`ui/browser-tests-unmocked/smoke/admin-sql-query.spec.ts`)
   - Partially covers RPC functionality (admin UI can query functions)

### Missing Browser tests (unmocked) Tests

1. **Call Function via UI** (covers TC-RPC-002, TC-RPC-003)
   - Navigate to "Functions" tab
   - Click on function name
   - Enter argument values in form
   - Click "Execute"
   - Verify result displayed

2. **Function with Error Handling** (covers TC-RPC-005, TC-RPC-006)
   - Navigate to function
   - Enter invalid argument types
   - Click "Execute"
   - Verify error message displayed

---

## Fixture Requirements

1. `tests/fixtures/rpc/call-void.json` — Void function call
2. `tests/fixtures/rpc/call-scalar.json` — Scalar return value
3. `tests/fixtures/rpc/call-setof.json` — Set-returning function
4. `tests/fixtures/rpc/call-wrong-types.json` — Type mismatch error
5. `tests/fixtures/rpc/call-missing-arg.json` — Missing required argument
6. `tests/fixtures/rpc/call-default-args.json` — Optional arguments with defaults
7. `tests/fixtures/rpc/call-rls.json` — RLS policy enforcement
8. `tests/fixtures/rpc/call-sql-injection.json` — SQL injection protection
9. `tests/fixtures/rpc/call-jsonb.json` — JSONB return type

---

## Test Tags

- `rpc`
- `functions`
- `postgresql`
- `stored-procedures`
- `rls`
- `security`

---

**Last Updated:** Session 079 (2026-02-13)
