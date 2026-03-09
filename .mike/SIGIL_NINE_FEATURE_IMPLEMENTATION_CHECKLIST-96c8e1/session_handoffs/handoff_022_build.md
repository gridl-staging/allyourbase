# Handoff 022 — Stage 2 Database Schema Hardening (OAuth 019-022)

## What I did

Focused task completed: **Database schema hardening + migration test coverage for OAuth provider tables**.

1. Added a **red-first migration SQL test** to enforce required OAuth schema constraints:
   - New file: `internal/migrations/oauth_sql_test.go`
   - Test: `TestOAuthMigrationSQLConstraints`
   - Asserts required constraints are present in migration SQL for files `019`-`022`

2. Ran red test and confirmed failure before changes:
   - Command: `GOCACHE=$PWD/.gocache go test ./internal/migrations -run TestOAuthMigrationSQLConstraints -count=1`
   - Failed on missing constraints (client_id format, scope checks, PKCE S256-only, client type/secret consistency)

3. Updated OAuth migrations to enforce constraints at DB level:
   - `internal/migrations/sql/019_ayb_oauth_clients.sql`
     - `client_id` regex check: `^ayb_cid_[0-9a-f]{48}$`
     - `client_type`/`client_secret_hash` consistency check
     - `scopes` validity check (`readonly`, `readwrite`, `*`)
     - non-empty `scopes` and non-empty `redirect_uris`
   - `internal/migrations/sql/020_ayb_oauth_authorization_codes.sql`
     - `scope` validity check
     - `code_challenge_method = 'S256'` check
   - `internal/migrations/sql/021_ayb_oauth_tokens.sql`
     - `scope` validity check
   - `internal/migrations/sql/022_ayb_oauth_consents.sql`
     - `scope` validity check

4. Added an integration test (for full DB behavior) in `internal/migrations/runner_test.go`:
   - `TestOAuthMigrationsEnforceProviderConstraints`
   - Verifies invalid inserts are rejected across clients/codes/tokens/consents

5. Updated checklists:
   - Marked Stage 2 Database Schema items complete in:
     - `.mike/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST-96c8e1/checklists/stage_02_checklist.md`
   - Updated original input file progress note in:
     - `_dev/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST.md`

## Test runs

Passed:
- `GOCACHE=$PWD/.gocache go test ./internal/migrations -run TestOAuthMigrationSQLConstraints -count=1`
- `GOCACHE=$PWD/.gocache go test ./internal/migrations -count=1`

Could not run in this sandbox:
- Integration-tag migration tests requiring local port binding / TEST_DATABASE_URL bootstrap (`testpg` could not bind localhost port in sandbox)

## Files created/modified

Created:
- `internal/migrations/oauth_sql_test.go`
- `.mike/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST-96c8e1/session_handoffs/handoff_022_build.md`

Modified:
- `internal/migrations/sql/019_ayb_oauth_clients.sql`
- `internal/migrations/sql/020_ayb_oauth_authorization_codes.sql`
- `internal/migrations/sql/021_ayb_oauth_tokens.sql`
- `internal/migrations/sql/022_ayb_oauth_consents.sql`
- `internal/migrations/runner_test.go`
- `.mike/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST-96c8e1/checklists/stage_02_checklist.md`
- `_dev/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST.md`

Also pre-existing session state files changed automatically:
- `.mike/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST-96c8e1/analytics/events_v1.jsonl`
- `.mike/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST-96c8e1/state.json`

## What’s next

1. Run integration migration tests in an environment that allows local port binding:
   - `go run ./internal/testutil/cmd/testpg -- go test -tags=integration ./internal/migrations -run TestOAuthMigrationsEnforceProviderConstraints -count=1`
2. Begin next Stage 2 task (one-task session): OAuth client registration admin handlers/routes checklist block.
3. Commit + push once git index write is permitted (current sandbox blocked `.git/index.lock` creation).
