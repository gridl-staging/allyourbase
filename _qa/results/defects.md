# Allyourbase QA Defect Report

**Date:** 2026-02-21
**Tester:** Automated exploratory QA (bash + Playwright)
**Build:** `ayb dev (commit: none, built: unknown)` — dev build from source
**Perspective:** Brand-new user following the README quickstart

---

## Critical Issues (must fix before publish)

### C1. `ayb --help` output is duplicated

Every section (CORE, DATA & SCHEMA, AUTH & SECURITY, MIGRATIONS, CONFIGURATION) appears **twice** in the help output. A new user running `ayb --help` sees a wall of duplicated text.

**Root cause:** `initHelp()` is called twice — once in `init()` (root.go:75) and again in `Execute()` (root.go:80). Each call adds the same command groups, doubling the output.

**File:** `internal/cli/root.go:75,80`

---

### C2. Health endpoint reports "ok" even when database is unreachable

When the AYB server is running but the embedded Postgres process has died or is not connected, `GET /health` still returns `{"status":"ok"}` with HTTP 200. Meanwhile, ALL database-dependent operations (SQL, auth, CRUD, users, RLS) fail with 400/500 errors.

A new user sees "healthy" but nothing actually works. The health check must verify database connectivity.

**Observed:** Server reported healthy while:
- `ayb sql` failed with "connection refused to port 15432"
- `/api/auth/register` returned 500
- `/api/admin/users/` returned 500
- `/api/admin/sql/` returned 400

**File:** `internal/server/server.go` (handleHealth function)

---

### C3. `ayb config` exposes all secrets in plaintext

Running `ayb config` outputs the **full resolved configuration** including:
- `admin.password` (admin dashboard password)
- `auth.jwt_secret` (JWT signing key)
- SMTP passwords, OAuth secrets, SMS API keys, S3 credentials

There is zero masking or redaction. If a user pastes `ayb config` output into a GitHub issue or support request, their secrets are exposed.

**File:** `internal/cli/config.go:52-71`

---

### C4. README "Working with the API" examples fail with 401

The README (lines 67-78) shows curl commands without authentication:
```bash
curl -X POST http://localhost:8090/api/collections/posts \
  -H "Content-Type: application/json" \
  -d '{"title": "Hello world", "body": "First post"}'
```

But auth is **enabled by default** in `ayb.toml` (line 49: `enabled = true`), so these return:
```json
{"code":401,"message":"missing or invalid authorization header"}
```

A new user following the README will immediately hit a wall. The README either needs auth examples or needs to explain the auth setup.

**File:** `README.md:67-78`, `ayb.toml:49`

---

### C5. Startup banner "Try:" suggestion also fails with 401

The `ayb start` output suggests:
```
  Try:
    curl http://localhost:8090/api/schema
```

But this returns 401 when auth is enabled. A brand-new user's very first suggested command fails.

**File:** `internal/cli/start.go` (startup banner)

---

## High Issues (should fix before publish)

---

### H2. Multiple CLI commands fail silently with 401 when auth is enabled

These CLI commands require admin authentication but don't explain how to provide it:

| Command | Error | Expected UX |
|---------|-------|-------------|
| `ayb schema` | 401: missing or invalid authorization header | Should work against running server |
| `ayb logs` | 401: admin authentication required | Should auto-detect admin token |
| `ayb stats` | 401: admin authentication required | Should auto-detect admin token |
| `ayb apikeys list` | 401: admin authentication required | Should auto-detect admin token |

These commands only work if `AYB_ADMIN_TOKEN` is set, but nothing in the docs or error messages tells the user this. The CLI should either:
1. Read the admin token from the PID file or a token cache, or
2. Display a helpful error like "Run `export AYB_ADMIN_TOKEN=...` or use `--admin-token`"

**Files:** `internal/cli/schema.go:88-90`, `internal/cli/logs.go:65-68`, `internal/cli/stats.go:128-131`, `internal/cli/apikeys.go:54-72`

---

### H3. `ayb types typescript` requires `--database-url` even with managed Postgres

When the server is running with embedded Postgres, `ayb types typescript` fails with:
```
Error: --database-url is required (or set DATABASE_URL)
```

This is confusing because the user already has a running server with a database. The command connects directly to Postgres (not via the API), so it can't use the running server. Either:
1. The command should discover the embedded Postgres connection string, or
2. The error should explain: "This command connects directly to PostgreSQL. Use: `ayb types typescript --database-url postgresql://ayb:ayb@localhost:15432/ayb`"

**File:** `internal/cli/types.go:48-55`

---

### H4. Admin login rate limiter is hardcoded at 5/minute — too aggressive for localhost

The admin dashboard rate limiter allows only 5 login attempts per minute per IP. This is **hardcoded** and not configurable. During testing, repeated logins (even with correct passwords) trigger "too many requests" errors, locking the developer out of their own dashboard.

Playwright tests confirmed: after 2-3 successful logins, subsequent attempts are blocked, showing "too many requests" on the login page.

This rate limit is appropriate for production but too aggressive for local development. It should be configurable or relaxed for localhost connections.

**File:** `internal/server/server.go:93`

---

## Medium Issues (nice to fix)

### M1. `ayb users` without subcommand shows help instead of user list

Running `ayb users` shows the subcommand help text instead of listing users. A new user expects `ayb users` to list users (similar to `docker ps`). The intuitive behavior would be to default to `ayb users list`.

---

### M2. `ayb logs` and `ayb stats` output is just an error message

Even when the error is a 401, the CLI prints `Error: server returned 401: {...}` on stdout and exits with code 0 in some cases. The error JSON contains a `doc_url` field pointing to documentation, but the CLI doesn't format this helpfully.

---

### M3. `ayb config` shows `password = '6ec99beaa59ff6a5ad311ed7929fedec'` — but user never set this

When using the default `ayb.toml`, the admin password shown by `ayb config` is the one from the config file. But a new user doing `ayb start` for the first time with no config file doesn't know what the admin password is. The startup banner doesn't show it. They have to run `ayb admin reset-password` to get one, but nothing tells them to do this.

---

### M4. The admin status endpoint reveals auth configuration

`GET /api/admin/status` returns `{"auth":true}` without authentication. While this is by design (the login page needs to know if auth is enabled), it also tells attackers that auth is enabled, which is minor information leakage.

---

## Low Issues (cosmetic / polish)

### L1. `ayb version` shows `dev (commit: none, built: unknown)` in dev builds

This is expected for a local build, but if a user builds from source, the version string is unhelpful. The Makefile injects version info, but `go build` alone does not.

---

### L2. Internal `_ayb_*` tables visible in dashboard sidebar

The dashboard sidebar shows all tables including internal ones like `_ayb_api_keys`, `_ayb_migrations`, `_ayb_sessions`, etc. These are implementation details that clutter the UI. They should be hidden or grouped separately (like Supabase hides its `auth.*` schema).

**Observed in:** Dashboard mobile screenshot showing tables: boards, cards, columns, poll_options, polls, smoke_test_records, votes — but also internal tables visible in schema API response.

---

### L3. Dashboard `smoke_test_records` table is leftover test data

The dashboard shows a `smoke_test_records` table from a previous test run. Demo/test artifacts should be cleaned up. A new user seeing "smoke_test_records" in their dashboard would be confused.

---

### L4. No `--version` flag (only `ayb version` subcommand)

Standard CLI convention is to support both `ayb --version` and `ayb version`. Currently `ayb --version` returns `Error: unknown flag: --version`.

---

## Test Results Summary

| Test Suite | Result | Defects |
|-----------|--------|---------|
| 01 CLI Help & Version | FAIL | 1 (version string detection — minor, help duplication is real bug) |
| 02 Server Lifecycle | (skipped — tested manually) | Health check false positive is critical |
| 03 API CRUD | FAIL | 11 (all 401s — README/auth documentation issue) |
| 04 Auth Flow | PASS | 0 |
| 05 Dashboard API | PASS | 0 (with working database) |
| 06 Demo Launch | PASS | 0 |
| 07 CLI Data Commands | FAIL | 2 (schema 401, types needs --database-url) |
| Playwright Dashboard | PASS | 6 reported (but 5 are rate-limiter false positives — real issue is H4) |
| Playwright Live-Polls | (not run — demo requires separate server lifecycle) |
| Playwright Kanban | (not run — demo requires separate server lifecycle) |

---

## What Works Well

- **Login page** — clean, professional, branded. Looks great.
- **Dashboard layout** — well-organized sidebar with logical grouping (Tables, Database, Services, Messaging, Admin)
- **Auth flow** — registration, login, JWT, refresh tokens, /me endpoint all work perfectly
- **Demo apps** — both live-polls and kanban start quickly (~2s), serve correctly, seed users work
- **Admin API** — SQL editor, schema, users, API keys, RLS, logs, stats all work when authenticated
- **CLI help** — every command has comprehensive help text with good formatting
- **Error messages** — include `doc_url` pointing to real documentation pages (verified: 200 OK)
- **No console errors** — dashboard loads without JavaScript errors
- **Responsive design** — dashboard works at mobile widths
- **No horizontal overflow** — layout is clean at all viewport sizes
