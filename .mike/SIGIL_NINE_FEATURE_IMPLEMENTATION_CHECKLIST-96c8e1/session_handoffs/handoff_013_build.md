# Handoff 013 — Build (Stage 1: Per-App API Key Scoping)

## What I built

Completed the remaining **Admin Dashboard** Stage 1 items with TDD in the API Keys UI:

1. Added optional app scoping in API key creation:
- New `App Scope` dropdown in create-key modal.
- Supports `User-scoped (no app)` default.
- Sends `appId` in `createApiKey` payload when selected.

2. Added app association display in API key list:
- New `App` column in API Keys table.
- Shows app name when metadata exists.
- Falls back to raw `appId` if app metadata is unavailable.
- Shows `User-scoped` label for legacy/non-app keys.

3. Added per-app rate limit stats display:
- For app-scoped keys, displays configured app limit in the App column (e.g. `Rate: 120 req/60s`, `Rate: unlimited`).
- Also shown in the “API Key Created” modal for app-scoped keys.

4. Cleaned a UI compile issue:
- Removed unused `cn` import from `ui/src/components/Apps.tsx` so `tsc --noEmit` passes.

## TDD evidence (red → green)

Red:
- Added new failing tests first in `ui/src/components/__tests__/ApiKeys.test.tsx`:
  - app selector presence/options
  - `appId` payload forwarding
  - app association rendering
  - app rate-limit stats rendering
  - unknown app fallback rendering
- Ran:
  - `npm --prefix ui test -- src/components/__tests__/ApiKeys.test.tsx`
- Result: failed as expected before implementation.

Green:
- Implemented UI changes in `ui/src/components/ApiKeys.tsx`.
- Re-ran focused tests:
  - `npm --prefix ui test -- src/components/__tests__/ApiKeys.test.tsx src/components/__tests__/Apps.test.tsx`
- Result: pass (64 tests).

Type-check:
- `cd ui && npx tsc --noEmit`
- Result: pass.

## Checklist updates

Updated and checked off in:
- `.mike/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST-96c8e1/checklists/stage_01_checklist.md`

Checked items:
- Add Apps management page: list apps, create app, delete app
- Add app selector to API key creation flow (optional: "scope to app" dropdown)
- Show app association on API key list view
- Show per-app rate limit usage/stats
- Write component tests for new UI

Updated progress note in input file:
- `_dev/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST.md`

## Files modified

- `ui/src/components/ApiKeys.tsx`
- `ui/src/components/__tests__/ApiKeys.test.tsx`
- `ui/src/components/Apps.tsx`
- `.mike/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST-96c8e1/checklists/stage_01_checklist.md`
- `_dev/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST.md`

Also present in working tree from session runtime metadata:
- `.mike/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST-96c8e1/analytics/events_v1.jsonl`
- `.mike/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST-96c8e1/state.json`

## What’s next

Stage 1 remaining unchecked items are now in **SDK & Docs** and final feature-status updates:
- Update TypeScript SDK types (if needed)
- Update docs:
  - `docs-site/guide/api-reference.md`
  - `docs-site/guide/admin-dashboard.md`
  - `docs-site/guide/configuration.md` (if config impact)
  - `tests/specs/admin.md`
- Completion updates:
  - `_dev/AYB_FEATURE_CHECKLIST_from_SIJLE.md`
  - `_dev/FEATURES.md`
