# Stage 5 Review Handoff (Session 101)

## What I did

Full code + test audit of the Stage 5 Custom Email Templates implementation across all layers (core package, API handlers, CLI, auth integration, UI components, browser tests). Found and fixed 4 issues:

### Bug 1: False-positive test — `TestServiceGetEffective_DisabledCustomShowsBuiltin`
- **File:** `internal/emailtemplates/emailtemplates_test.go`
- **Problem:** Test used nil store (never exercised the disabled-custom-override-falls-back-to-builtin branch). Passed trivially via the nil-store → builtin path, which is already covered by `TestServiceGetEffective_BuiltinSource`.
- **Fix:** Replaced with `TestServiceGetEffective_DisabledCustomFallsBackToBuiltin` using a stub store returning a disabled custom template for `auth.password_reset`, verifying it falls back to the builtin (source="builtin", enabled=true, correct subject). Also added `TestServiceGetEffective_DisabledCustomNoBuiltinReturnsCustom` for the disabled-custom-only (no builtin fallback) path.

### Bug 2: Redundant/misleading test — `TestServiceRenderWithFallback_CustomFailsFallsBackToBuiltin`
- **File:** `internal/emailtemplates/emailtemplates_test.go`
- **Problem:** Named "CustomFails" but used nil store (no custom lookup at all). Redundant with `TestServiceRender_BuiltinFallback`. The real custom-failure→fallback test already exists at `TestServiceRenderWithFallback_CustomRenderFailsFallsBackToBuiltin`.
- **Fix:** Replaced with `TestServiceRenderWithFallback_StoreErrorFallsBackToBuiltin` — store returns a DB connection error, verifies graceful fallback to builtin (not a silent swallow).

### Bug 3: Silent store error swallowing in `RenderWithFallback`
- **File:** `internal/emailtemplates/emailtemplates.go`
- **Problem:** When `store.Get()` returned a non-ErrNotFound error (e.g. DB connection refused), `RenderWithFallback` silently fell through to builtin with no logging. Admins had no observability into custom templates being silently bypassed due to infrastructure issues.
- **Fix:** Changed `RenderWithFallback` to use the same switch pattern as `Render`: explicit branches for `err == nil && enabled`, `err == nil` (disabled), `ErrNotFound`, and `default` (store errors). The default branch logs the error at ERROR level before falling through to builtin.

### Bug 4: (Consequence of 1-3) Missing test coverage
- The `GetEffective` disabled-custom + builtin-exists branch was untested.
- The `RenderWithFallback` store-error → fallback branch was untested.
- Both now have focused tests.

### Verification
- `go test ./internal/emailtemplates/ -run 'TestValidateKey|TestRender|TestStripHTML|TestService|TestDefaultBuiltins|TestSizeLimits|TestSystemKeys'` — 24 tests PASS
- `go test ./internal/server/ -run 'TestEmailTemplates|TestIsValidEmailAddress'` — 24 tests PASS
- `go test ./internal/cli/ -run 'TestEmailTemplates'` — 14 tests PASS
- `go test ./internal/migrations/ -run 'TestEmailTemplatesMigration'` — 1 test PASS
- `npx vitest run src/components/__tests__/EmailTemplates.test.tsx` — 5 tests PASS
- Browser-mocked and browser-unmocked spec lint — clean
- Stage 3/4 regression: `go test ./internal/jobs/` PASS, `go test ./internal/matview/` PASS

### Items confirmed as correct (no issues found)
- Migration SQL (026) — schema, constraints, size limits all correct
- API handlers — proper validation, error mapping (400/404/500), email validation with header-injection prevention
- Auth integration — `renderAuthEmail` with `RenderWithFallback` for graceful degradation, legacy path preserved
- CLI — all commands wire correctly, --html-file reads from disk, --vars validates JSON
- UI component — proper debounced preview, enable/disable toggle, send-test flow
- SSTI prevention — `map[string]string` only, empty FuncMap, `missingkey=error`
- Render timeout — 5s context deadline on all render paths
- Template compilation on save — parse errors caught at upsert time

### Completion gates — all checked
All 9 completion gates for Stage 5 are now verified and checked off.

## What's next

Stage 5 is complete. Ready to begin Stage 6: Push Notifications (FCM/APNS).

## Files modified

- `internal/emailtemplates/emailtemplates.go` — added store-error logging to `RenderWithFallback`
- `internal/emailtemplates/emailtemplates_test.go` — replaced 2 false-positive/redundant tests with 3 proper tests
- `.mike/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST-96c8e1/checklists/stage_05_checklist.md` — added session 101 review notes, checked all completion gates
- `.mike/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST-96c8e1/stages.md` — marked Stage 5 complete
- `_dev/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST.md` — marked Stage 5 complete in scope and stages
- `.mike/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST-96c8e1/session_handoffs/handoff_101_review.md` — this file
