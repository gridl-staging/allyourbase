# Handoff 060 — Stage 3 Webhook Pruner Toggle Validation (Build)

## What I did

Completed one focused Stage 3 task using red→green TDD: validated webhook pruner migration behavior in server startup wiring.

1. Added failing tests first (red):
   - `TestNewStartsLegacyWebhookPrunerWhenJobsDisabled`
   - `TestNewSkipsLegacyWebhookPrunerWhenJobsEnabled`

   These tests verify that the legacy timer-based pruner is started only when `jobs.enabled=false` and skipped when `jobs.enabled=true`.

2. Implemented production wiring to make behavior testable and deterministic:
   - Introduced a small `webhookDispatcher` interface in `internal/server/server.go`
   - Added injectable factory var `newWebhookDispatcher` (defaults to `webhooks.NewDispatcher`)
   - Switched server startup to use this factory, preserving runtime behavior

3. Updated Stage 3 trackers:
   - Marked built-in item complete: migrate `Dispatcher.StartPruner` behavior
   - Marked completion gate complete: webhook pruner migration validated
   - Added progress note in `_dev/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST.md`

## Tests run (focused)

- `GOCACHE=/tmp/ayb-gocache go test ./internal/server -run 'TestNew(Starts|Skips)LegacyWebhookPrunerWhenJobs' -count=1`

Result: passing.

Note: running all `internal/server` tests is sandbox-limited in this environment because tests that bind network listeners fail with `bind: operation not permitted`.

## Files created or modified

- `internal/server/webhook_pruner_toggle_test.go` (new)
- `internal/server/server.go`
- `.mike/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST-96c8e1/checklists/stage_03_checklist.md`
- `_dev/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST.md`

## Next recommended task

Continue Stage 3 with one focused runtime-wiring/config item from the remaining checklist, e.g. documenting/enforcing `[jobs]` defaults in `ayb.toml` template and related config docs, or validating the broader backward-compat completion gate (`jobs.enabled=false` startup path) with focused tests.
