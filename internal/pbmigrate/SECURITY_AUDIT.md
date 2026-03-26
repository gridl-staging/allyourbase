# Security Audit: internal/pbmigrate

**Date:** 2026-03-22
**Scope:** `internal/pbmigrate/` package — PocketBase to PostgreSQL migration
**Auditor:** Security review (Stage 5 checklist, session s21)
**Baseline commit:** `d085dec4` (HEAD of `mattman/mar22_pm_03_pbmigrate_hardening`)

## Executive Summary

The `pbmigrate` package converts PocketBase (SQLite) databases to AYB-managed PostgreSQL. The audit examined SQL injection, RLS policy generation safety, path traversal, auth data validation, and data integrity. Stages 2-5 of the hardening sprint have addressed the most critical gaps. No unresolved critical vulnerabilities remain in this package at HEAD.

## Findings

### F1: SQL Injection — Well Protected [PASS]

**Severity:** N/A (no vulnerability found)
**Files:** `typemap.go`, `migrator.go`, `reader.go`, `auth.go`, `sqlutil/sqlutil.go`

All user-derived identifiers (table names, column names, index names) pass through `SanitizeIdentifier()` which delegates to `sqlutil.QuoteIdent()`. The `QuoteIdent` implementation:
- Wraps identifiers in double quotes
- Escapes embedded `"` as `""`
- Strips null bytes

All data values in INSERT statements use parameterized queries (`$1`, `$2`, etc.) via `insertBatch()` (migrator.go:309) and `insertAuthUser()` (auth.go:211).

**Evidence:**
- `insertBatch` builds `$N` placeholders and passes values as `args` to `tx.Exec`
- `insertAuthUser` uses `$1`-`$5` for all auth fields
- `insertUserProfile` iterates custom fields with parameterized values
- `ReadRecords` (reader.go:187) quotes the table name via `SanitizeIdentifier`

### F2: RLS Policy Generation — Hardened, Fail-Closed [PASS after Stage 5]

**Severity:** N/A (previously critical, now remediated)
**Files:** `rls.go`, `migrator.go`

**Pre-hardening state:** Unsupported PocketBase tokens (e.g., `@collection.*`, `@request.data.*`) were silently passed through into PostgreSQL RLS policy SQL, potentially creating policies that were syntactically valid but semantically wrong — granting unintended access.

**Current state (post-Stage 5):**
- Three compiled regexes gate token processing:
  - `nestedAuthFieldRegex` — rejects `@request.auth.X.Y` (ambiguous nested traversals)
  - `authFieldRegex` — converts `@request.auth.id` and `@request.auth.{field}` to PostgreSQL equivalents
  - `unsupportedTokenRegex` — catches any remaining `@...` tokens after known conversions
- Processing order in `convertRuleExpression()` is deliberate: check nested first, convert known tokens, then reject remaining
- `@collection.*` references are no longer translated (previously emitted a semantically unsafe user-id-based predicate)
- `migrateRLS()` records conversion failures in `MigrationStats.Errors` before returning the error
- Safe operators converted: `&&`→AND, `||`→OR, `!=`→`<>`
- Regex `~` operator passed through unchanged (valid PostgreSQL syntax)

**Test coverage:** `TestConvertRuleExpression` (9 cases), `TestGenerateRLSPolicies_UnsupportedRuleReturnsError`, `TestMigrateRLS_UnsupportedRuleRecordsDiagnostic`

### F3: Path Traversal in File Migration — Well Protected [PASS]

**Severity:** N/A (no vulnerability found)
**Files:** `files.go`

File migration has defense-in-depth path validation:
- `validateCollectionStorageKey()` (line 269) rejects empty, `.`, `..`, and paths containing `/` or `\`
- `isSafeMigrationRelativePath()` (line 279) rejects absolute paths and `..`-prefixed paths
- `copyFile()` (line 232) rejects symlinks and non-regular files via `os.Lstat` check
- Failed file copies are tracked per-file in `MigrationStats.FailedFiles` with slash-normalized paths

### F4: Auth User Data Validation — Hardened [PASS after Stage 3]

**Severity:** N/A (previously medium, now remediated)
**Files:** `auth.go`

**Post-Stage 3 state:**
- Duplicate emails detected case-insensitively via `emailSeen` map with `strings.ToLower` (auth.go:~130)
- Empty and whitespace-only password hashes rejected before insertion via `strings.TrimSpace` check (auth.go:~140)
- Nil custom auth fields explicitly preserved as SQL NULL — the schema-driven custom field loop always includes all defined fields, inserting `nil` values rather than silently omitting them (auth.go:~160)

**Test coverage:** `TestParseAuthUsers_DuplicateEmails`, `_DuplicateEmailsCaseInsensitive`, `_EmptyPasswordHash`, `_WhitespaceOnlyPasswordHash`, `_NilCustomFieldsPreserved`

### F5: Source SQL Fragment Injection in Views and RLS — Remediated [PASS]

**Severity:** Critical (fixed)
**Files:** `typemap.go`, `rls.go`, `migrator.go`

**Pre-fix state:** The migrator embedded two source-controlled SQL fragments directly into destination DDL:
- `coll.ViewQuery` was interpolated into `CREATE VIEW ... AS <query>`
- converted RLS expressions were interpolated into `CREATE POLICY ... USING/WITH CHECK (<expr>)`

That allowed a tampered PocketBase export to inject stacked statements or comments, for example by smuggling `; DROP TABLE ...` into a view query or rule expression.

**Current state:**
- `normalizeEmbeddedSQLFragment()` rejects unquoted `;`, `--`, `/*`, and `*/`
- view queries are normalized to a single statement and must begin with `SELECT` or `WITH`
- converted RLS expressions are validated through the same boundary checks before policy SQL is emitted
- `migrateSchema()` now fails closed if a view definition is unsafe

**Regression coverage:** `TestBuildCreateViewSQL` rejects stacked statements, comments, and non-SELECT input; `TestConvertRuleExpression` rejects stacked statements and comments inside RLS input.

### F6: Regex False Positive on @ in String Literals — Remediated [PASS]

**Severity:** N/A (previously informational)
**File:** `rls.go`

**Pre-fix state:** The unsupported-token scanner looked at the entire rule string and could match `@...` inside SQL literals. Rules such as `email = 'user@example.com'` were rejected even though they were valid.

**Current state:**
- Token rewrites now run only on unquoted segments via `rewriteUnquotedRuleSegments()`
- Unsupported-token detection now scans only unquoted segments via `findUnsupportedPocketBaseToken()`
- Literal content (including `@request.auth.id` and email/regex strings containing `@`) is preserved verbatim
- Unsupported live PocketBase tokens outside literals are still rejected fail-closed

**Regression coverage:** `TestConvertRuleExpression` now includes literal-preservation and mixed literal/live-token cases.

## Summary Table

| ID | Finding | Severity | Status |
|----|---------|----------|--------|
| F1 | SQL injection via identifiers/values | N/A | Protected |
| F2 | RLS silent passthrough of unsupported tokens | Critical (fixed) | Remediated in Stage 5 |
| F3 | Path traversal in file migration | N/A | Protected |
| F4 | Auth user data validation gaps | Medium (fixed) | Remediated in Stage 3 |
| F5 | Source SQL fragment injection in views/RLS | Critical (fixed) | Remediated |
| F6 | Regex false positive on @ in strings | Info (fixed) | Remediated |

## Recommendations

1. **No immediate action required.** All critical and medium findings have been remediated with regression tests.
2. Keep expanding quoted-literal regression tests when new PocketBase rule patterns are introduced.
