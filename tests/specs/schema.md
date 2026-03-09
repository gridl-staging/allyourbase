# Schema Introspection Test Specification (Tier 2)

**Purpose:** Detailed test cases for schema introspection and TypeScript type generation (BDD Tier 1: B-SCHEMA-001, B-SCHEMA-002)

**Related BDD Stories:**
- [B-SCHEMA-001: Get Full Schema](../../docs/BDD_SPECIFICATIONS.md#b-schema-001-get-full-schema)
- [B-SCHEMA-002: Generate TypeScript Types](../../docs/BDD_SPECIFICATIONS.md#b-schema-002-generate-typescript-types)

---

## Test Cases

### TC-SCHEMA-001: Get Full Schema — Happy Path

**Story:** B-SCHEMA-001
**Type:** Integration
**Fixture:** `tests/fixtures/schema/get-schema.json`

**Setup:**
```sql
CREATE TABLE users (
  id SERIAL PRIMARY KEY,
  email TEXT NOT NULL UNIQUE,
  name TEXT,
  created_at TIMESTAMP DEFAULT NOW()
);

CREATE TABLE posts (
  id SERIAL PRIMARY KEY,
  title TEXT NOT NULL,
  author_id INTEGER REFERENCES users(id),
  status TEXT CHECK (status IN ('draft', 'published')),
  metadata JSONB
);
```

**Request:**
```
GET /api/schema
```

**Expected Response (200 OK):**
```json
{
  "tables": {
    "users": {
      "columns": {
        "id": {
          "type": "integer",
          "nullable": false,
          "default": "nextval('users_id_seq'::regclass)",
          "primaryKey": true
        },
        "email": {
          "type": "text",
          "nullable": false,
          "unique": true
        },
        "name": {
          "type": "text",
          "nullable": true
        },
        "created_at": {
          "type": "timestamp without time zone",
          "nullable": true,
          "default": "now()"
        }
      },
      "foreignKeys": {},
      "isView": false
    },
    "posts": {
      "columns": {
        "id": {
          "type": "integer",
          "nullable": false,
          "primaryKey": true
        },
        "title": {
          "type": "text",
          "nullable": false
        },
        "author_id": {
          "type": "integer",
          "nullable": true
        },
        "status": {
          "type": "text",
          "nullable": true
        },
        "metadata": {
          "type": "jsonb",
          "nullable": true
        }
      },
      "foreignKeys": {
        "author_id": {
          "table": "users",
          "column": "id"
        }
      },
      "isView": false
    }
  }
}
```

**Assertions:**
- Response status = 200
- Response contains `tables` object
- Each table has `columns`, `foreignKeys`, `isView` fields
- Column metadata includes type, nullable, default, primaryKey, unique
- Foreign keys reference correct tables and columns

**Cleanup:**
```sql
DROP TABLE posts;
DROP TABLE users;
```

---

### TC-SCHEMA-002: Get Schema — Includes Views

**Story:** B-SCHEMA-001
**Type:** Integration
**Fixture:** `tests/fixtures/schema/get-schema-views.json`

**Setup:**
```sql
CREATE TABLE users (id SERIAL PRIMARY KEY, email TEXT);
CREATE VIEW active_users AS SELECT * FROM users WHERE email IS NOT NULL;
```

**Request:**
```
GET /api/schema
```

**Assertions:**
- Response contains both `users` (table) and `active_users` (view)
- `users.isView` = false
- `active_users.isView` = true

**Cleanup:**
```sql
DROP VIEW active_users;
DROP TABLE users;
```

---

### TC-SCHEMA-003: Get Schema — Excludes System Tables

**Story:** B-SCHEMA-001
**Type:** Integration

**Request:**
```
GET /api/schema
```

**Assertions:**
- Response does NOT include `pg_catalog` tables
- Response does NOT include `information_schema` tables
- Response does NOT include `ayb_*` internal tables (e.g., `ayb_users`, `ayb_sessions`)

---

### TC-SCHEMA-004: Get Schema — No Authentication Required

**Story:** B-SCHEMA-001
**Type:** Integration

**Steps:**
1. GET `/api/schema` without Authorization header
2. Expect 200 OK with schema data

**Assertions:**
- Request succeeds without authentication
- Schema is publicly accessible

---

### TC-SCHEMA-005: Generate TypeScript Types — Happy Path

**Story:** B-SCHEMA-002
**Type:** CLI
**Fixture:** `tests/fixtures/schema/typegen-basic.json`

**Setup:**
```sql
CREATE TABLE users (
  id SERIAL PRIMARY KEY,
  email TEXT NOT NULL,
  name TEXT,
  age INTEGER,
  active BOOLEAN DEFAULT true,
  metadata JSONB,
  created_at TIMESTAMP
);
```

**Command:**
```bash
ayb types typescript --database-url $DB_URL -o /tmp/types.d.ts
```

**Expected Output File (`/tmp/types.d.ts`):**
```typescript
export interface Users {
  id: number;
  email: string;
  name?: string | null;
  age?: number | null;
  active?: boolean | null;
  metadata?: any | null;
  created_at?: string | null;
}

export interface Database {
  users: Users;
}
```

**Assertions:**
- Command exits with status 0
- Output file created at specified path
- Non-nullable columns have required type (e.g., `email: string`)
- Nullable columns have optional + null type (e.g., `name?: string | null`)
- PostgreSQL types mapped correctly:
  - text → string
  - integer → number
  - boolean → boolean
  - jsonb → any
  - timestamp → string

**Cleanup:**
```sql
DROP TABLE users;
```
```bash
rm /tmp/types.d.ts
```

---

### TC-SCHEMA-006: Generate TypeScript Types — Multiple Tables

**Story:** B-SCHEMA-002
**Type:** CLI
**Fixture:** `tests/fixtures/schema/typegen-multiple.json`

**Setup:**
```sql
CREATE TABLE users (id SERIAL PRIMARY KEY, email TEXT NOT NULL);
CREATE TABLE posts (id SERIAL PRIMARY KEY, title TEXT NOT NULL, author_id INTEGER);
CREATE TABLE comments (id SERIAL PRIMARY KEY, content TEXT, post_id INTEGER);
```

**Command:**
```bash
ayb types typescript --database-url $DB_URL -o /tmp/types.d.ts
```

**Expected Output:**
```typescript
export interface Users {
  id: number;
  email: string;
}

export interface Posts {
  id: number;
  title: string;
  author_id?: number | null;
}

export interface Comments {
  id: number;
  content?: string | null;
  post_id?: number | null;
}

export interface Database {
  users: Users;
  posts: Posts;
  comments: Comments;
}
```

**Assertions:**
- All 3 tables have TypeScript interfaces
- Database interface aggregates all tables
- Table names converted to PascalCase for interface names

**Cleanup:**
```sql
DROP TABLE comments;
DROP TABLE posts;
DROP TABLE users;
```

---

### TC-SCHEMA-007: Generate TypeScript Types — Invalid Database URL

**Story:** B-SCHEMA-002
**Type:** CLI

**Command:**
```bash
ayb types typescript --database-url "postgres://invalid:5432/db" -o /tmp/types.d.ts
```

**Expected Output:**
```
Error: Failed to connect to database: connection refused
```

**Assertions:**
- Command exits with non-zero status
- Error message indicates connection failure
- No output file created

---

### TC-SCHEMA-008: Generate TypeScript Types — Output to Stdout

**Story:** B-SCHEMA-002
**Type:** CLI

**Setup:**
```sql
CREATE TABLE users (id SERIAL PRIMARY KEY, email TEXT NOT NULL);
```

**Command:**
```bash
ayb types typescript --database-url $DB_URL
```

**Expected Output (stdout):**
```typescript
export interface Users {
  id: number;
  email: string;
}

export interface Database {
  users: Users;
}
```

**Assertions:**
- When `-o` flag is omitted, output goes to stdout
- No file created

**Cleanup:**
```sql
DROP TABLE users;
```

---

### TC-SCHEMA-009: Schema Caching

**Story:** B-SCHEMA-001
**Type:** Integration
**Fixture:** `tests/fixtures/schema/caching.json`

**Steps:**
1. GET `/api/schema` (first request, cache miss)
2. Measure response time (slow, schema introspection)
3. GET `/api/schema` again (second request, cache hit)
4. Measure response time (fast, cached)
5. Verify both responses are identical

**Assertions:**
- First request takes longer (e.g., 50-200ms)
- Second request is faster (e.g., <10ms)
- Cached response matches fresh response

---

### TC-SCHEMA-010: Schema Cache Refresh

**Story:** B-SCHEMA-001
**Type:** Integration

**Steps:**
1. GET `/api/schema` (cache populated)
2. Create new table via SQL
3. Wait for cache refresh (configured interval, e.g., 60 seconds) OR restart server
4. GET `/api/schema` again
5. Verify new table appears in schema

**Assertions:**
- Schema cache refreshes periodically
- New tables appear after refresh

---

## Browser Test Coverage (Unmocked)

### Implemented Browser Tests

None currently implemented.

### Missing Browser Tests

1. **Schema Browser** (covers TC-SCHEMA-001)
   - Login to admin dashboard
   - Navigate to "Schema" tab
   - Verify all tables listed
   - Click on a table
   - Verify columns, types, constraints displayed

2. **Generate Types from UI** (covers TC-SCHEMA-005)
   - Navigate to Schema tab
   - Click "Generate Types"
   - Select "TypeScript"
   - Download generated file
   - Verify file contains correct interfaces

---

## Fixture Requirements

1. `tests/fixtures/schema/get-schema.json` — Basic schema introspection
2. `tests/fixtures/schema/get-schema-views.json` — Schema with views
3. `tests/fixtures/schema/typegen-basic.json` — Basic TypeScript generation
4. `tests/fixtures/schema/typegen-multiple.json` — Multiple tables
5. `tests/fixtures/schema/caching.json` — Schema caching test data

---

## Test Tags

- `schema`
- `introspection`
- `typegen`
- `typescript`
- `caching`
- `metadata`

---

**Last Updated:** Session 079 (2026-02-13)
