# Handoff 095 (Review) - Stage 5 Email Templates API/Rendering Hardening

## What I reviewed

Reviewed recent Stage 5 backend and UI/browser-mocked work with focus on:
- API boundary validation and error mapping
- False-positive/missing coverage in handler and rendering tests
- Email-template rendering edge behavior for custom app keys
- Browser-mocked spec compliance with `resources/BROWSER_TESTING_STANDARDS_2.md`

## Issues found and fixed

1. **Invalid template keys were not rejected consistently (API boundary bug)**
- `GET /api/admin/email/templates/:key`
- `DELETE /api/admin/email/templates/:key`
- `PATCH /api/admin/email/templates/:key`
- `POST /api/admin/email/templates/:key/preview`

These paths now validate `:key` with `emailtemplates.ValidateKey` and return `400` for invalid format.

2. **`POST /api/admin/email/send` misclassified validation/render failures as 500**
- Send handler now maps render/parse validation failures to `400` and keeps missing template as `404`.
- Added explicit regression tests for both mapping paths.

3. **Security hardening: weak email recipient validation in send endpoint**
- Replaced naive email check with strict `net/mail` parsing plus plain-addr/domain sanity checks.
- Rejects header-injection and display-name forms.

4. **Root-cause rendering bug in `emailtemplates.Service.Render` for app custom templates**
- Previously: if a custom-only app template existed but render failed (e.g. missing variable), service returned `ErrNoTemplate` (looked like 404 not found).
- Now: when no builtin fallback exists, service returns the original render failure (`ErrRenderFailed`) instead of false `ErrNoTemplate`.
- Also changed `GetEffective` to return non-`ErrNotFound` store errors instead of silently masking them.

5. **Browser-mocked test standards compliance hardening**
- Removed explicit `setTimeout` waiting pattern from spec.
- Added lint rule banning `setTimeout` in mocked browser spec files.
- Updated spec to use direct role-based interaction for Email Templates navigation.

6. **Flaky component assertion hardened**
- Made initial effective-template load assertion wait-based in `EmailTemplates` component test to avoid timing flake.

## TDD / tests added (red -> green)

Added failing tests first, then fixed implementation:

- `internal/server/email_templates_handler_test.go`
  - `TestEmailTemplatesGetEffective_InvalidKey`
  - `TestEmailTemplatesDelete_InvalidKey`
  - `TestEmailTemplatesPatch_InvalidKey`
  - `TestEmailTemplatesPreview_InvalidKey`
  - `TestEmailTemplatesPreview_MissingVariableErrorMappedToBadRequest`
  - `TestEmailTemplatesSend_RenderValidationErrorMappedToBadRequest`
  - `TestEmailTemplatesSend_TemplateNotFoundMappedToNotFound`
  - `TestIsValidEmailAddress` strengthened with injection/display-name invalid cases

- `internal/emailtemplates/emailtemplates_test.go`
  - `TestServiceRender_CustomOnlyMissingVariableReturnsRenderError`
  - `TestServiceGetEffective_StoreError`

- Browser mocked test/lint hardening
  - `ui/browser-tests-mocked/email-templates-preview.spec.ts`
  - `ui/browser-tests-mocked/eslint.config.mjs`

## Focused tests run

Passed:
- `GOCACHE=/tmp/go-build go test ./internal/server -run 'TestEmailTemplates|TestIsValidEmailAddress' -count=1`
- `GOCACHE=/tmp/go-build go test ./internal/emailtemplates -count=1`
- `GOCACHE=/tmp/go-build go test ./internal/auth -run 'TestRenderAuthEmail' -count=1`
- `npm --prefix ui run lint:browser-tests:mocked`
- `npm --prefix ui run test -- src/components/__tests__/EmailTemplates.test.tsx`
- `npm --prefix ui run test:browser:mocked -- --list browser-tests-mocked/email-templates-preview.spec.ts`

Environment-limited / intentionally not run here:
- Full Playwright runtime for browser-mocked spec remains blocked in this sandbox by Chromium launch permissions.
- Broader Go package test runs that bind local listeners are sandbox-limited; only focused non-listener Stage 5 slices were run.

## Checklist/tracker updates

Updated:
- `.mike/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST-96c8e1/checklists/stage_05_checklist.md`
  - Added review hardening note
  - Marked endpoint validation/error mapping item complete
  - Marked server handler validation/error-mapping tests item complete
- `_dev/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST.md`
  - Added Stage 5 review hardening progress note

## Files modified

- `internal/emailtemplates/emailtemplates.go`
- `internal/emailtemplates/emailtemplates_test.go`
- `internal/server/email_templates_handler.go`
- `internal/server/email_templates_handler_test.go`
- `ui/browser-tests-mocked/email-templates-preview.spec.ts`
- `ui/browser-tests-mocked/eslint.config.mjs`
- `ui/src/components/__tests__/EmailTemplates.test.tsx`
- `.mike/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST-96c8e1/checklists/stage_05_checklist.md`
- `_dev/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST.md`
- `.mike/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST-96c8e1/session_handoffs/handoff_095_review.md`

## What’s next

1. Add browser-unmocked Stage 5 lifecycle coverage (seeded load, customize system template, preview, reset-to-default).
2. Finish remaining Stage 5 backend/docs/spec checklist items (store integration slices, docs-site pages, tests/specs matrix).
3. Run focused Stage 4 regression safety checks required by Stage 5 completion gate (jobs/matview slices).
