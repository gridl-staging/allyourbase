# Handoff 054 — Stage 3 DB Migrations + Red→Green SQL Tests

## What I did

Completed one focused Stage 3 task: **Database schema migrations + migration test coverage**.

1. **Red phase (tests first)**
- Tightened `internal/migrations/jobs_sql_test.go` to require composite partial index keys for jobs claim/recovery indexes:
  - `ON _ayb_jobs (state, run_at)`
  - `ON _ayb_jobs (state, lease_until)`
- Ran focused test and confirmed failure before SQL changes.

2. **Green phase (migration fix)**
- Updated `internal/migrations/sql/023_ayb_jobs.sql`:
  - `idx_ayb_jobs_claimable` now indexes `(state, run_at)` with `WHERE state = 'queued'`
  - `idx_ayb_jobs_lease` now indexes `(state, lease_until)` with `WHERE state = 'running'`

3. **Added Stage 3 migration integration coverage**
- Added `internal/migrations/jobs_migrations_integration_test.go` with tests for:
  - `_ayb_jobs` and `_ayb_job_schedules` table existence
  - CHECK constraints (job state; `max_attempts >= 1`)
  - Uniqueness (`idempotency_key`, schedule `name`)
  - FK behavior (`schedule_id` FK rejects unknown IDs, `ON DELETE SET NULL` behavior)
  - Index metadata assertions for key columns and predicates

4. **Checklist/input updates (as requested)**
- Updated Stage 3 checklist items to checked in `.mike/.../checklists/stage_03_checklist.md`:
  - migration `023`
  - migration `024`
  - migration tests
- Updated master input file `_dev/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST.md` with a new Stage 3 migration build note.

## Test evidence

### Red
- Command:
  - `GOCACHE=/tmp/ayb-gocache GOTMPDIR=/tmp/ayb-gotmp go test ./internal/migrations -run '^TestJobsMigrationSQLConstraints$' -count=1`
- Result: failed on missing composite keys in `023_ayb_jobs.sql`.

### Green
- Command:
  - `GOCACHE=/tmp/ayb-gocache GOTMPDIR=/tmp/ayb-gotmp go test ./internal/migrations -run '^TestJobsMigrationSQLConstraints$' -count=1`
- Result: pass.

### Focused package verification
- Command:
  - `GOCACHE=/tmp/ayb-gocache GOTMPDIR=/tmp/ayb-gotmp go test ./internal/migrations -count=1`
- Result: pass.

### Integration test status
- Could not execute `//go:build integration` migration tests in this sandbox:
  - `TEST_DATABASE_URL` not set
  - `go run ./internal/testutil/cmd/testpg -- ...` failed because sandboxed networking could not fetch uncached `embedded-postgres` dependency from `proxy.golang.org`.

## Files created
- `internal/migrations/jobs_migrations_integration_test.go`
- `.mike/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST-96c8e1/session_handoffs/handoff_054_build.md`

## Files modified
- `internal/migrations/sql/023_ayb_jobs.sql`
- `internal/migrations/jobs_sql_test.go`
- `.mike/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST-96c8e1/checklists/stage_03_checklist.md`
- `_dev/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST.md`

## Next recommended step
- Run focused integration migration tests in an environment with `TEST_DATABASE_URL` (or with cached `embedded-postgres`):
  - `go test -tags=integration ./internal/migrations -run 'TestJobsMigrations(ConstraintsAndUniqueness|Indexes)$' -count=1`
- Then proceed to next single Stage 3 task: implement `internal/jobs` queue state machine (`Enqueue`, `Claim`, `Complete`, `Fail`, `Cancel`, `RetryNow`) with red→green tests first.
