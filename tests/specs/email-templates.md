# Email Templates Test Specification (Stage 5)

## Scope

Stage 5 validates configurable email templates for auth and arbitrary app email flows:

- key validation and size constraints
- parse-time and render-time safety (`missingkey=error`, escaping, timeout)
- fallback behavior (custom -> builtin -> deterministic error)
- admin API validation and error mapping
- CLI command behavior and JSON variable handling
- admin dashboard component/browser flows

## Test matrix

| Area | Required behavior | Automated coverage |
|---|---|---|
| Key validation | dot-notation key format accepts/rejects expected cases | `internal/emailtemplates/emailtemplates_test.go` (`TestValidateKey`) |
| Rendering | subject + HTML render with `map[string]string` variables | `internal/emailtemplates/emailtemplates_test.go` (`TestRenderTemplates_Basic`) |
| Missing variables | undefined vars fail hard (`missingkey=error`) | `internal/emailtemplates/emailtemplates_test.go` (`TestRenderTemplates_MissingVariable`, `TestRenderTemplates_MissingHTMLVariable`) |
| Parse errors | invalid Go template syntax fails at parse time | `internal/emailtemplates/emailtemplates_test.go` (`TestRenderTemplates_InvalidTemplateSyntax`) |
| HTML safety | values are HTML-escaped, plaintext fallback is generated from rendered HTML | `internal/emailtemplates/emailtemplates_test.go` (`TestRenderTemplates_HTMLAutoEscaping`, `TestRenderTemplates_HTMLAutoEscapingDecodedInPlaintext`, `TestStripHTML`) |
| SSTI surface | execution data is non-method map shape | `internal/emailtemplates/emailtemplates_test.go` (`TestRenderTemplates_SSTIPrevention`) |
| Timeout/cancel | cancelled/deadline contexts fail deterministically | `internal/emailtemplates/emailtemplates_test.go` (`TestRenderTemplates_CancelledContext`, `TestServiceRender_Timeout`) |
| Fallback chain | custom success, disabled custom fallback, custom render failure fallback, missing template error | `internal/emailtemplates/emailtemplates_test.go` (`TestServiceRender_CustomTemplateSuccess`, `TestServiceRender_DisabledCustomFallsBackToBuiltin`, `TestServiceRender_CustomRenderFailsFallsBackToBuiltin`, `TestServiceRender_NoTemplate`) |
| Effective template | builtin/custom source, disabled override behavior | `internal/emailtemplates/emailtemplates_test.go` (`TestServiceGetEffective_*`) |
| Store CRUD (integration) | upsert/get/list/delete/enable-toggle with real DB | `internal/emailtemplates/store_integration_test.go` (`-tags integration`) |
| Migration constraints | Stage 5 migration file exists and constraints enforce schema rules | `internal/migrations/email_templates_sql_test.go`, `internal/migrations/email_templates_migrations_integration_test.go` |
| Auth integration path | auth keys route through template service with graceful fallback and legacy path support | `internal/auth/email_templates_test.go` |
| Admin API | list/get/upsert/delete/patch/preview/send validate inputs and map errors (400/404) | `internal/server/email_templates_handler_test.go` |
| CLI | list/get/set/delete/preview/enable/disable/send command behavior and vars JSON parsing | `internal/cli/email_templates_cli_test.go` |
| UI component | table/edit/preview/toggle/send-test interactions | `ui/src/components/__tests__/EmailTemplates.test.tsx` |
| UI browser mocked | seeded preview load + backend validation error + client JSON parse error | `ui/browser-tests-mocked/email-templates-preview.spec.ts` |
| UI browser unmocked | seeded load-and-verify + customize + preview + reset-to-default lifecycle | `ui/browser-tests-unmocked/full/email-templates-lifecycle.spec.ts` |

## Focused command set

Use focused commands for Stage 5 verification:

```bash
go test ./internal/emailtemplates -count=1
go test ./internal/auth -run 'TestRenderAuthEmail' -count=1
go test ./internal/server -run 'TestEmailTemplates|TestIsValidEmailAddress' -count=1
go test ./internal/cli -run 'TestEmailTemplates' -count=1
go test ./internal/migrations -run 'TestEmailTemplatesMigration' -count=1
```

UI/component and browser-mocked checks:

```bash
cd ui && npm test -- src/components/__tests__/EmailTemplates.test.tsx src/components/__tests__/Layout.test.tsx src/components/__tests__/CommandPalette.test.tsx
cd ui && npm run lint:browser-tests:mocked
cd ui && npm run test:browser:mocked -- --list browser-tests-mocked/email-templates-preview.spec.ts
cd ui && npx eslint browser-tests-unmocked/full/email-templates-lifecycle.spec.ts --config browser-tests-unmocked/eslint.config.mjs
cd ui && npx playwright test --list --project=full browser-tests-unmocked/full/email-templates-lifecycle.spec.ts
```

Integration-tag tests remain environment-dependent on `TEST_DATABASE_URL`.
Browser-unmocked runtime execution is environment-dependent in sandboxed CI/dev shells where Chromium launch is blocked.

## Browser 3-tier status

- Tier 1 (component): complete
- Tier 2 (browser-mocked): complete
- Tier 3 (browser-unmocked): lifecycle spec implemented (`ui/browser-tests-unmocked/full/email-templates-lifecycle.spec.ts`); runtime still environment-dependent
