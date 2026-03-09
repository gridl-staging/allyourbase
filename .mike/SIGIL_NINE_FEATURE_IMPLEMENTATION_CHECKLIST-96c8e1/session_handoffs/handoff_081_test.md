# Handoff 081 (Test Audit) - Stage 4 Data Layer Hardening

## What I did

1. Re-read Stage 4 checklist, input tracker, and browser testing standards.
2. Ran focused Stage 4 and Stage 3-regression suites (no full-project run) with sandbox-safe Go cache configuration.
3. Audited browser-tier test hygiene against linted standards.
4. Updated Stage 4 tracker files with concrete evidence, blockers, and coverage gaps.

## Tests run and results

### Go / backend focused suites
- `go test ./internal/migrations -run 'TestMatviewMigrationSQLConstraints|TestMatviewMigrationConstraintsAndUniqueness' -count=1` -> PASS
- `GOCACHE=/tmp/go-cache go test ./internal/matview -count=1` -> PASS
- `GOCACHE=/tmp/go-cache go test ./internal/jobs -run 'TestMatviewRefreshHandlerIntegration|TestAdvanceScheduleAndEnqueueRequiresEnabledSchedule|TestAdvanceScheduleAndEnqueueSkipsDisabledSchedule|TestStartWithSchedulerDisabledDoesNotRunSchedulerLoop' -count=1` -> PASS
- `GOCACHE=/tmp/go-cache go test ./internal/server -run '^TestHandleAdmin(.*Matview.*)$' -count=1` -> PASS
- `GOCACHE=/tmp/go-cache go test ./internal/server -run 'TestNewStartsLegacyWebhookPrunerWhenJobsDisabled|TestNewSkipsLegacyWebhookPrunerWhenJobsEnabled' -count=1` -> PASS
- `GOCACHE=/tmp/go-cache go test ./internal/cli -run '^TestMatviews' -count=1` -> PASS

### Integration-tag checks
- `GOCACHE=/tmp/go-cache go test -tags=integration ./internal/realtime -run 'TestCanSeeRecordJoinPolicyMembershipAccess|TestCanSeeRecordJoinPolicyMembershipTransitions|TestCanSeeRecordDeletePassThroughWithJoinPolicy' -count=1 -v` -> PASS with SKIP (requires `TEST_DATABASE_URL`)
- `GOCACHE=/tmp/go-cache go test -tags=integration ./internal/matview ...` -> FAIL (package `TestMain` panics when `TEST_DATABASE_URL` is unset)
- `GOCACHE=/tmp/go-cache go test -tags=integration ./internal/jobs -run 'TestMatviewRefreshHandlerIntegration' -count=1 -v` -> FAIL (package `TestMain` panics when `TEST_DATABASE_URL` is unset)

### UI focused suite
- `npm test -- src/components/__tests__/MatviewsAdmin.test.tsx` -> PASS (11/11)

### Browser test hygiene
- `npm run lint:browser-tests` -> FAIL (`31 errors`, `40 warnings`)
- Main issues: raw locators, `page.waitForTimeout`, conditional assertions/skips in existing `ui/browser-tests-unmocked` specs.

## Coverage gaps identified

1. Stage 4 Matviews has component coverage but no dedicated `ui/browser-tests-unmocked` Matviews spec yet.
2. Browser-unmocked suite currently violates documented standards lint in multiple existing specs, weakening the no-false-positive confidence bar for browser tier.
3. Integration-tag evidence for Stage 4 matview/job DB paths remains environment-blocked in this sandbox because `TEST_DATABASE_URL` is not set.

## Tracker updates made

- Updated Stage 4 checklist notes with this audit and blockers:
  - `.mike/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST-96c8e1/checklists/stage_04_checklist.md`
- Updated master input tracker (required "Input file") with a Stage 4 test-audit note:
  - `_dev/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST.md`

## Files modified/created this session

- Modified: `.mike/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST-96c8e1/checklists/stage_04_checklist.md`
- Modified: `_dev/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST.md`
- Created: `.mike/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST-96c8e1/session_handoffs/handoff_081_test.md`

## What’s next

1. Run Stage 4 integration-tag suites in an environment with `TEST_DATABASE_URL` set and collect pass evidence for completion gates.
2. Add a dedicated Matviews browser-unmocked spec (seeded load-and-verify + refresh/register flow) following the browser standards.
3. Clean existing browser-unmocked lint violations (`raw locator` + `waitForTimeout` + conditional test logic) so browser tests meet the “no false positives / no manual QA” quality bar.
