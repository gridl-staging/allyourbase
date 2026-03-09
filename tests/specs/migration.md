# Migration Tools Test Specification (Tier 2)

**Purpose:** Detailed test cases for migrating from PocketBase, Supabase, and Firebase to AYB (BDD Tier 1: B-MIG-001 through B-MIG-004)

**Related BDD Stories:**
- [B-MIG-001: Migrate from PocketBase](../../docs/BDD_SPECIFICATIONS.md#b-mig-001-migrate-from-pocketbase)
- [B-MIG-002: Migrate from Supabase](../../docs/BDD_SPECIFICATIONS.md#b-mig-002-migrate-from-supabase)
- [B-MIG-003: Migrate from Firebase](../../docs/BDD_SPECIFICATIONS.md#b-mig-003-migrate-from-firebase)
- [B-MIG-004: Auto-Detect Migration Source](../../docs/BDD_SPECIFICATIONS.md#b-mig-004-auto-detect-migration-source)

---

## PocketBase Migration Tests

### TC-MIG-PB-001: Migrate PocketBase — Happy Path

**Story:** B-MIG-001
**Type:** CLI + Integration
**Fixture:** `tests/fixtures/migration/pocketbase/happy-path/`

**Fixture Structure:**
```
pocketbase/happy-path/
├── pb_data/
│   └── data.db (SQLite with users, collections, settings)
└── expected/
    ├── users.json (expected ayb_users records)
    ├── posts.json (expected posts records)
    └── summary.json (expected migration summary)
```

**Command:**
```bash
ayb migrate pocketbase --source ./tests/fixtures/migration/pocketbase/happy-path/pb_data --database-url $DB_URL
```

**Expected Output:**
```
✓ Analyzing PocketBase data...
  - Found 2 auth users
  - Found 1 collection: posts (5 records)

✓ Phase 1: Migrating schema...
  - Created table: posts

✓ Phase 2: Migrating auth users...
  - Migrated 2 users (bcrypt hashes preserved)

✓ Phase 3: Migrating collection data...
  - Migrated posts: 5 records

✓ Migration complete!
  - Users: 2
  - Collections: 1 (5 total records)
```

**Assertions:**
- Command exits with status 0
- `ayb_users` table contains 2 users with preserved IDs
- Password hashes are bcrypt format (`$2a$`)
- `posts` table exists with 5 records
- Foreign keys (author_id) reference correct user IDs
- Created/updated timestamps preserved

---

### TC-MIG-PB-002: Migrate PocketBase — Idempotent Re-Run

**Story:** B-MIG-001
**Type:** CLI + Integration
**Fixture:** `tests/fixtures/migration/pocketbase/idempotent/`

**Steps:**
1. Run migration command first time
2. Verify data migrated successfully
3. Run same migration command again
4. Verify no duplicate records created

**Expected Behavior:**
- First run: all data migrated
- Second run: detects existing data, skips duplicates
- Final record counts match expected (no duplicates)

**Assertions:**
- `ayb_users` has exactly 2 records (not 4)
- `posts` has exactly 5 records (not 10)
- Idempotency via primary key constraints or ON CONFLICT

---

### TC-MIG-PB-003: Migrate PocketBase — Empty Database

**Story:** B-MIG-001
**Type:** CLI
**Fixture:** `tests/fixtures/migration/pocketbase/empty/`

**Command:**
```bash
ayb migrate pocketbase --source ./tests/fixtures/migration/pocketbase/empty/pb_data --database-url $DB_URL
```

**Expected Output:**
```
✓ Analyzing PocketBase data...
  - Found 0 auth users
  - Found 0 collections

⚠ Nothing to migrate (empty database)
```

**Assertions:**
- Command exits with status 0
- No errors thrown
- Graceful handling of empty source

---

### TC-MIG-PB-004: Migrate PocketBase — Invalid Path

**Story:** B-MIG-001
**Type:** CLI

**Command:**
```bash
ayb migrate pocketbase --source ./non_existent_path --database-url $DB_URL
```

**Expected Output:**
```
Error: PocketBase data directory not found: ./non_existent_path
```

**Assertions:**
- Command exits with non-zero status
- Clear error message about missing directory

---

## Supabase Migration Tests

### TC-MIG-SB-001: Migrate Supabase — Full Migration (5 Phases)

**Story:** B-MIG-002
**Type:** CLI + Integration
**Fixture:** `tests/fixtures/migration/supabase/full/`

**Fixture Structure:**
```
supabase/full/
├── source_dump.sql (PostgreSQL dump with schema, data, RLS)
└── expected/
    ├── users.json (expected ayb_users records)
    ├── oauth_accounts.json (expected ayb_oauth_accounts)
    ├── rls_policies.json (expected ayb_rls_policies)
    └── summary.json
```

**Command:**
```bash
ayb migrate supabase --source-url $SUPABASE_DB_URL --database-url $AYB_DB_URL
```

**Expected Output:**
```
✓ Analyzing Supabase database...
  - Tables: 3 (users, posts, comments)
  - Auth users: 10
  - OAuth accounts: 5
  - RLS policies: 8

✓ Phase 1: Migrating schema...
  - Created table: posts (3 columns, 1 FK)
  - Created table: comments (4 columns, 2 FKs)

✓ Phase 2: Migrating data...
  - posts: 50 records
  - comments: 120 records

✓ Phase 3: Migrating auth users...
  - Migrated 10 users (bcrypt hashes preserved)

✓ Phase 4: Migrating OAuth accounts...
  - Migrated 5 OAuth accounts (3 Google, 2 GitHub)

✓ Phase 5: Migrating RLS policies...
  - Migrated 8 RLS policies

✓ Migration complete!
  - Tables: 2
  - Records: 170
  - Users: 10
  - OAuth accounts: 5
  - RLS policies: 8
```

**Assertions:**
- All 5 phases complete successfully
- Schema, data, auth, OAuth, RLS all migrated
- Bcrypt password hashes preserved
- RLS policies active (can be queried via `ayb_rls_policies`)

---

### TC-MIG-SB-002: Migrate Supabase — ON CONFLICT Handling

**Story:** B-MIG-002
**Type:** CLI + Integration

**Steps:**
1. Run migration with 10 posts
2. Re-run migration with 15 posts (5 new, 10 existing)
3. Verify final count is 15 (not 25)

**Assertions:**
- ON CONFLICT clauses prevent duplicates
- Existing records preserved
- New records added
- No primary key violations

---

### TC-MIG-SB-003: Migrate Supabase — Invalid Source URL

**Story:** B-MIG-002
**Type:** CLI

**Command:**
```bash
ayb migrate supabase --source-url postgres://invalid:5432/db --database-url $AYB_DB_URL
```

**Expected Output:**
```
Error: Failed to connect to Supabase database: connection refused
```

**Assertions:**
- Command exits with non-zero status
- Clear error about connection failure

---

## Firebase Migration Tests

### TC-MIG-FB-001: Migrate Firebase — Auth + Firestore

**Story:** B-MIG-003
**Type:** CLI + Integration
**Fixture:** `tests/fixtures/migration/firebase/full/`

**Fixture Structure:**
```
firebase/full/
├── firebase-export.json (auth users + Firestore collections)
└── expected/
    ├── users.json (expected ayb_users with firebase-scrypt hashes)
    ├── posts.json (expected posts with JSONB metadata)
    └── summary.json
```

**Command:**
```bash
ayb migrate firebase --source ./tests/fixtures/migration/firebase/full/firebase-export.json --database-url $DB_URL
```

**Expected Output:**
```
✓ Analyzing Firebase export...
  - Auth users: 8
  - Firestore collections: 2 (posts, comments)

✓ Phase 1: Migrating auth users...
  - Migrated 8 users (firebase-scrypt hashes preserved)

✓ Phase 2: Migrating Firestore collections...
  - posts: 30 documents → 30 records
  - comments: 75 documents → 75 records

✓ Migration complete!
  - Users: 8
  - Collections: 2 (105 total records)
```

**Assertions:**
- Firebase scrypt password hashes stored as `$firebase-scrypt$...`
- Firestore nested JSON → JSONB columns
- Document IDs preserved
- Timestamps converted to PostgreSQL format

---

### TC-MIG-FB-002: Migrate Firebase — Scrypt Parameters Embedded

**Story:** B-MIG-003
**Type:** Integration
**Fixture:** `tests/fixtures/migration/firebase/scrypt/`

**Setup:**
Firebase export with user:
```json
{
  "users": [
    {
      "localId": "user123",
      "email": "test@example.com",
      "passwordHash": "base64hash",
      "salt": "base64salt",
      "createdAt": "1609459200000"
    }
  ],
  "config": {
    "signerKey": "base64key",
    "saltSeparator": "base64sep",
    "rounds": 8,
    "memCost": 14
  }
}
```

**Expected `ayb_users` record:**
```sql
INSERT INTO ayb_users (id, email, password_hash) VALUES (
  'user123',
  'test@example.com',
  '$firebase-scrypt$<signerKey>$<saltSep>$<salt>$8$14$<hash>'
);
```

**Assertions:**
- All scrypt parameters embedded in password hash
- Progressive re-hashing works on first login (bcrypt/firebase → argon2id)

---

### TC-MIG-FB-003: Migrate Firebase — via firebase:// URL

**Story:** B-MIG-003
**Type:** CLI
**Fixture:** Requires real Firebase project (or mock Firebase Admin SDK)

**Command:**
```bash
ayb migrate firebase --source firebase://my-project-id --database-url $DB_URL
```

**Expected Behavior:**
- Uses Firebase Admin SDK to fetch auth users and Firestore data
- Same migration flow as JSON export

**Assertions:**
- Admin SDK authentication works
- Data fetched directly from Firebase
- Migration completes successfully

---

### TC-MIG-FB-004: Migrate Firebase — Invalid Export Format

**Story:** B-MIG-003
**Type:** CLI

**Command:**
```bash
ayb migrate firebase --source ./invalid.json --database-url $DB_URL
```

**File Contents:**
```json
{
  "not": "a valid firebase export"
}
```

**Expected Output:**
```
Error: Invalid Firebase export format: missing 'users' or 'collections' field
```

**Assertions:**
- Command exits with non-zero status
- Clear error about invalid format

---

## Auto-Detection Tests

### TC-MIG-AUTO-001: Auto-Detect PocketBase

**Story:** B-MIG-004
**Type:** CLI

**Command:**
```bash
ayb start --from ./tests/fixtures/migration/pocketbase/happy-path/pb_data
```

**Expected Behavior:**
- Detects PocketBase from directory structure (contains `data.db`)
- Auto-selects `pbmigrate.Migrate()`
- Migration proceeds as PocketBase migration

**Assertions:**
- Correct migrator selected
- No manual `--type` flag required

---

### TC-MIG-AUTO-002: Auto-Detect Supabase

**Story:** B-MIG-004
**Type:** CLI

**Command:**
```bash
ayb start --from postgres://user:pass@db.supabase.co:5432/postgres
```

**Expected Behavior:**
- Detects Supabase from connection string (contains `.supabase.` hostname)
- Auto-selects `sbmigrate.Migrate()`

**Assertions:**
- Correct migrator selected

---

### TC-MIG-AUTO-003: Auto-Detect Firebase (JSON)

**Story:** B-MIG-004
**Type:** CLI

**Command:**
```bash
ayb start --from ./tests/fixtures/migration/firebase/full/firebase-export.json
```

**Expected Behavior:**
- Detects Firebase from `.json` file extension + contents
- Auto-selects `fbmigrate.Migrate()`

**Assertions:**
- Correct migrator selected

---

### TC-MIG-AUTO-004: Auto-Detect Firebase (URL)

**Story:** B-MIG-004
**Type:** CLI

**Command:**
```bash
ayb start --from firebase://my-project-id
```

**Expected Behavior:**
- Detects Firebase from `firebase://` scheme
- Auto-selects `fbmigrate.Migrate()`

**Assertions:**
- Correct migrator selected

---

### TC-MIG-AUTO-005: Auto-Detect Unknown Source

**Story:** B-MIG-004
**Type:** CLI

**Command:**
```bash
ayb start --from ./random_directory
```

**Expected Output:**
```
Error: Unable to detect migration source type from: ./random_directory
Supported sources:
  - PocketBase: path to pb_data directory
  - Supabase: postgres://...supabase...
  - Firebase: firebase://PROJECT_ID or path to .json export
```

**Assertions:**
- Command exits with non-zero status
- Helpful error message with supported sources

---

## Browser Test Coverage (Unmocked)

### Implemented Browser Tests

None (migration is CLI-only).

### Missing Browser Tests

N/A — Migration tools are CLI-only, no UI tests needed.

---

## Fixture Requirements

### PocketBase Fixtures
1. `tests/fixtures/migration/pocketbase/happy-path/` — Full PocketBase migration
2. `tests/fixtures/migration/pocketbase/idempotent/` — Re-run test
3. `tests/fixtures/migration/pocketbase/empty/` — Empty database

### Supabase Fixtures
4. `tests/fixtures/migration/supabase/full/` — 5-phase migration
5. `tests/fixtures/migration/supabase/schema-only/` — Schema without data
6. `tests/fixtures/migration/supabase/rls-policies/` — RLS policy migration

### Firebase Fixtures
7. `tests/fixtures/migration/firebase/full/` — Auth + Firestore
8. `tests/fixtures/migration/firebase/scrypt/` — Scrypt parameter embedding
9. `tests/fixtures/migration/firebase/auth-only/` — Auth without Firestore

---

## Test Tags

- `migration`
- `pocketbase`
- `supabase`
- `firebase`
- `cli`
- `idempotency`
- `password-hashing`
- `rls`

---

**Last Updated:** Session 079 (2026-02-13)
