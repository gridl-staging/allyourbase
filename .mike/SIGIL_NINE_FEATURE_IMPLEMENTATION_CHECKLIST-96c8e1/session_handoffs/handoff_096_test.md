# Handoff 096 (Test) - Stage 5 Email Templates Test Audit

## What I did

Audited all Stage 5 (Custom Email Templates) test suites for coverage gaps, false positives, redundancy, and efficiency.

### Issues found and fixed

**1. False positive: `TestServiceRender_CustomTemplateRenderFailure_FallsBackToBuiltin`**
- This test claimed to verify that a broken custom template falls back to builtin, but it used `store=nil`, which means it never exercised the custom template path at all. It was identical to `TestServiceRender_BuiltinFallback`.
- Replaced with `TestServiceRender_CustomRenderFailsFallsBackToBuiltin` using a stubbed store that returns a custom template with a missing variable. Verifies the actual graceful degradation path: custom template render fails → logs error → falls back to builtin → succeeds.

**2. Missing coverage: custom template success path**
- No test verified that `Service.Render` uses a custom template from the store when it exists and is enabled. Added `TestServiceRender_CustomTemplateSuccess` with stubbed store.

**3. Missing coverage: disabled custom → builtin fallback**
- No test verified that a disabled custom template falls through to the builtin. Added `TestServiceRender_DisabledCustomFallsBackToBuiltin` with stubbed store.

**4. Missing coverage: RenderWithFallback graceful degradation**
- Added `TestServiceRenderWithFallback_CustomRenderFailsFallsBackToBuiltin` testing the same graceful degradation through the `RenderWithFallback` API (which auth.renderAuthEmail calls).

**5. Redundant tests removed from `internal/auth/email_templates_test.go`**
- `TestRenderAuthEmail_LegacyFallback`: tested all 3 keys inline. Fully superseded by `TestRenderAuthEmail_AllKeysLegacyPath` which tests the same things as parallel subtests with better isolation and failure messages.
- `TestRenderAuthEmail_TemplateServiceUsed`: tested password_reset with mock. Fully superseded by `TestRenderAuthEmail_TemplateServiceUsed_AllKeys` which tests all 3 keys.

**6. Missing handler edge cases**
- `TestEmailTemplatesPatch_NotFound`: PATCH on nonexistent template → 404. Was untested.
- `TestEmailTemplatesPreview_MissingFields`: Preview with missing subject or HTML → 400. Was untested.
- `TestEmailTemplatesSend_MissingTemplateKey`: Send with no template key → 400. Was untested.

### Test summary

| Suite | File | Tests | Status |
|-------|------|-------|--------|
| emailtemplates unit | `internal/emailtemplates/emailtemplates_test.go` | 23 | PASS |
| server handlers | `internal/server/email_templates_handler_test.go` | 27 | PASS |
| auth rendering | `internal/auth/email_templates_test.go` | 5 (was 7, 2 removed) | PASS |
| CLI commands | `internal/cli/email_templates_cli_test.go` | 14 | PASS |
| migration file | `internal/migrations/email_templates_sql_test.go` | 1 | PASS |
| UI component | `ui/src/components/__tests__/EmailTemplates.test.tsx` | 5 | PASS |
| browser mocked lint | `ui/browser-tests-mocked/email-templates-preview.spec.ts` | lint clean | PASS |

Net change: -3 redundant/false-positive tests, +7 new coverage tests = +4 total.

### Efficiency observations

- All Go test runs complete in <0.5s per package — fast, no wasteful setup.
- UI component tests complete in ~230ms — fast, properly mocked.
- No redundant tests remain. Each test verifies a distinct behavior path.

## Files modified

- `internal/emailtemplates/emailtemplates_test.go` — replaced false-positive test with 4 real stub-store tests
- `internal/auth/email_templates_test.go` — removed 2 redundant tests
- `internal/server/email_templates_handler_test.go` — added 3 edge case tests
- `_dev/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST.md` — added test audit note

## What's next

1. Finish remaining unchecked Stage 5 checklist items (implementation):
   - Template Engine & Rendering checklist items (mark as [x] — implementation exists)
   - Auth/Mailer Integration checklist items (mark as [x] — implementation exists)
   - API endpoints checklist items (mark as [x] — implementation exists)
   - Migration SQL/integration tests (env-gated, need TEST_DATABASE_URL)
   - Browser-unmocked Stage 5 lifecycle tests
   - Docs/specs/trackers
2. Many unchecked items in the checklist appear to have working implementations already — a checklist status reconciliation pass would be valuable.
3. Stage 4 regression safety check (jobs/matview suites) is a completion gate.
