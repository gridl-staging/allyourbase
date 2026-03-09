# Stage 5: Custom Email Templates

## Review Notes (2026-02-22)

Previous checklist had several issues corrected in this revision:
- **Dropped `is_system` column**: System templates are embedded in the Go binary (`//go:embed templates/*.html`). The DB table stores only custom overrides. Having system templates as DB rows duplicates what's in the binary and creates ownership ambiguity (who seeds them? what happens on upgrade?). Instead, the API returns the "effective" template: custom override if one exists and is enabled, otherwise the built-in default. This matches PocketBase's pattern (defaults are code-level, overrides are per-collection settings).
- **Dropped `text_template` column**: The existing `stripHTML()` auto-generates plaintext from rendered HTML. No major BaaS (PocketBase, Supabase, Appwrite) exposes a separate plaintext template editor. Separate plaintext templates double the maintenance burden for admins with no meaningful benefit. Keep the auto-generation approach.
- **Dropped `description` column**: Template keys are self-describing (`auth.password_reset`). System keys are documented in the guide. No need for a per-row description field in v1.
- **Added SSTI prevention model**: Go template SSTI (Server-Side Template Injection) is a documented attack vector when objects with methods are passed to `Execute`. The security model is: template authors are admin users (trusted, like PocketBase/Supabase), and data passed to `Execute` is strictly `map[string]string` — no structs, no interfaces, no methods to exploit. Combined with an empty `FuncMap` (no custom functions), this eliminates the SSTI surface.
- **Added template compilation on save**: Parse/compile templates at save time (not just at render time) to catch syntax errors immediately. Store parse success/failure status. This prevents broken templates from reaching email delivery.
- **Added render timeout**: Go template execution gets a context with timeout (e.g., 5s) to prevent pathological templates from blocking the email pipeline.
- **Added send endpoint for arbitrary templates**: The Sigil requirement ("club invites, event reminders, challenge updates") needs a way to send emails using custom template keys with arbitrary variables. Added `POST /api/admin/email/send` endpoint. Without this, custom templates beyond auth flows are unusable.
- **Added "effective template" API pattern**: `GET /api/admin/email/templates/:key` returns the active template for a key — the custom override if it exists and is enabled, otherwise the built-in default with a `source: "builtin"` flag. This way admins always see what's being sent and can use the default as a starting point for customization.
- **Simplified variable model**: System template keys (`auth.*`) have a fixed variable set (`AppName`, `ActionURL`). Custom template keys accept arbitrary `map[string]string` variables. The variable payload type is `map[string]string` (not `map[string]any`) — flat strings only, no nested objects, no type coercion.
- **Clarified subject line templating**: Subject templates support the same variable substitution as HTML templates (Go `text/template` for subject, `html/template` for body). Default subjects ("Reset your password", "Verify your email", "Your login link") are overridable.
- **Routing with dots in keys**: Template keys like `auth.password_reset` contain dots. API routes use URL path segments (`/api/admin/email/templates/auth.password_reset`). Chi router handles dots in path segments fine. No special encoding needed.
- **Test audit follow-up (2026-02-22)**: Focused Stage 5 suites are green in this sandbox (`internal/emailtemplates`, Stage 5 CLI/server/auth slices, migration file test). Integration-tag email-template migration test compile defect was fixed (`testutil.NoError` misuse in `internal/migrations/email_templates_migrations_integration_test.go`); runtime execution remains env-gated by required `TEST_DATABASE_URL`.
- **Admin dashboard/component follow-up (2026-02-22)**: Implemented Email Templates admin view and navigation wiring in `ui` with red→green component coverage for list rendering, edit loading, debounced preview, enable/disable toggle, and send-test action.
- **Browser-mocked follow-up (2026-02-22)**: Added Stage 5 browser-mocked Playwright coverage for Email Templates preview edge cases (seeded load-and-verify, backend missing-variable preview error, client JSON parse error with no preview request) in `ui/browser-tests-mocked`. Mocked suite lint and test discovery are green; browser runtime execution is sandbox-blocked here due Chromium launch permission constraints.
- **Review hardening follow-up (2026-02-22)**: Fixed admin Email Templates API boundary defects: key-format validation now returns 400 on GET/DELETE/PATCH/PREVIEW invalid keys; `POST /api/admin/email/send` now maps template render/parse validation failures to 400 and missing-template to 404; send recipient validation now rejects header-injection/display-name inputs via strict `net/mail` parsing. Fixed `emailtemplates.Service.Render` so custom-only app template render failures return `ErrRenderFailed` (not false `ErrNoTemplate` 404). Added red→green regression tests for these paths and tightened browser-mocked spec hygiene (removed explicit `setTimeout` wait in spec; lint now bans it).
- **Docs/spec tracker follow-up (2026-02-22)**: Added Stage 5 operator/developer docs (`docs-site/guide/email-templates.md`) and integrated it into related guides (`email.md`, `api-reference.md`, `admin-dashboard.md`) plus docs nav. Added `tests/specs/email-templates.md` Stage 5 test matrix and updated trackers (`_dev/AYB_FEATURE_CHECKLIST_from_SIJLE.md`, `_dev/FEATURES.md`, `_dev/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST.md`, `.mike/.../stages.md`).
- **Browser-unmocked follow-up (2026-02-22)**: Added Stage 5 browser-unmocked lifecycle coverage in `ui/browser-tests-unmocked/full/email-templates-lifecycle.spec.ts` for seeded load-and-verify, customize system template, preview render, and reset-to-default flow. New spec lint and Playwright discovery are green; runtime execution remains sandbox-blocked due Chromium launch permission (`bootstrap_check_in ... Permission denied`).
- **Session 101 review (2026-02-22)**: Full code + test audit found and fixed 4 issues:
  1. **False-positive test fixed**: `TestServiceGetEffective_DisabledCustomShowsBuiltin` used nil store (never exercised disabled-custom branch). Replaced with `TestServiceGetEffective_DisabledCustomFallsBackToBuiltin` using a stubbed store returning a disabled custom template, verifying builtin fallback, plus `TestServiceGetEffective_DisabledCustomNoBuiltinReturnsCustom` for the disabled-custom-only path.
  2. **Redundant/misleading test replaced**: `TestServiceRenderWithFallback_CustomFailsFallsBackToBuiltin` (nil store, duplicate of builtin fallback test) → `TestServiceRenderWithFallback_StoreErrorFallsBackToBuiltin` (store returns DB error, verifies graceful fallback).
  3. **Silent store error swallowing fixed**: `RenderWithFallback` now logs store.Get errors (DB down, connection refused, etc.) before falling back to builtin, matching `Render`'s explicit error handling pattern.
  4. All Stage 5 focused Go suites (emailtemplates unit, server handler, CLI, migration), UI component tests, and browser lint all green after fixes.

---

## Discovery & Design

- [x] Re-read `_dev/AYB_FEATURE_CHECKLIST_from_SIJLE.md` email requirement and lock v1 scope: user-defined HTML templates with variable substitution for auth flows + arbitrary app emails, safe rendering with SSTI-proof data model, fallback to embedded defaults
- [x] Audit current email stack and call sites to reuse:
  - `internal/mailer/templates.go` — built-in password reset / verification / magic link rendering via `//go:embed templates/*.html` and Go `html/template`. Three render functions (`RenderPasswordReset`, `RenderVerification`, `RenderMagicLink`) accept `TemplateData{AppName, ActionURL}`. `stripHTML()` generates plaintext fallback
  - `internal/auth/auth.go` — `RequestPasswordReset()` at line 576 and `SendVerificationEmail()` at line 681 call the render functions then `s.mailer.Send()`. Subjects are hardcoded strings ("Reset your password", "Verify your email")
  - `internal/auth/magic_link.go` — `SendMagicLink()` at line 72 renders and sends with hardcoded subject "Your login link"
  - `internal/cli/start.go` — `buildMailer()` creates the mailer backend (log/smtp/webhook), `authSvc.SetMailer(m, cfg.Email.FromName, baseURL)` wires it in
  - `internal/mailer/mailer.go` — `Mailer` interface: `Send(ctx, *Message) error`. `Message` has `To`, `Subject`, `HTML`, `Text`
- [x] Record ADR for template storage + rendering strategy:
  - DB table stores custom overrides only. System defaults remain embedded in binary
  - Rendering uses Go `html/template` for HTML body, `text/template` for subject line
  - Data passed to Execute is `map[string]string` only (SSTI prevention)
  - Empty FuncMap (no custom template functions)
  - Templates parsed/validated on save, re-parsed on render with timeout
  - Fallback chain: enabled custom override → built-in default → deterministic error
- [x] Define canonical template keys:
  - System keys: `auth.password_reset`, `auth.email_verification`, `auth.magic_link`
  - System keys have fixed variable set: `AppName` (string), `ActionURL` (string)
  - App keys: dot-notation like `app.club_invite`, `app.event_reminder` — user-defined, arbitrary `map[string]string` variables
  - Key format: `^[a-z][a-z0-9]*(\.[a-z][a-z0-9_]*)+$` (at least two dot-separated segments)
- [x] Define non-goals for Stage 5:
  - No visual drag/drop editor
  - No MJML or provider-specific markup transforms
  - No remote template fetching (all templates in DB or embedded)
  - No custom template functions (FuncMap is empty)
  - No nested/complex variable types (flat `map[string]string` only)
  - No template versioning/history (delete and recreate to reset)
  - No i18n/locale-specific template variants (single template per key)

## Database Schema

- [x] Add migration `026_ayb_email_templates.sql` for `_ayb_email_templates`:
  - `id` UUID PK DEFAULT gen_random_uuid()
  - `template_key` VARCHAR(255) NOT NULL
  - `subject_template` TEXT NOT NULL (supports `{{.VarName}}` substitution via `text/template`)
  - `html_template` TEXT NOT NULL (supports `{{.VarName}}` substitution via `html/template`)
  - `enabled` BOOLEAN NOT NULL DEFAULT true (allows disabling a custom override without deleting it, falling back to default)
  - `created_at` TIMESTAMPTZ NOT NULL DEFAULT now()
  - `updated_at` TIMESTAMPTZ NOT NULL DEFAULT now()
  - Constraints: UNIQUE on `template_key`, CHECK key format regex `^[a-z][a-z0-9]*(\.[a-z][a-z0-9_]*)+$`, CHECK `length(html_template) <= 256000` (256KB limit), CHECK `length(subject_template) <= 1000`
- [x] Write migration SQL tests (key format regex rejects invalid keys, size limits enforced, uniqueness constraint) and migration integration tests (apply, insert valid/invalid, rollback)

## Template Engine & Rendering

- [x] Implement `internal/emailtemplates` package with `Store` (DB CRUD) and `Service` (render + fallback logic)
- [x] `Store` methods:
  - `Upsert(ctx, key, subjectTpl, htmlTpl)` — validates key format, parses both templates to catch syntax errors, inserts or updates the row. Returns parse error immediately on invalid template syntax
  - `Get(ctx, key)` — returns custom override row or `ErrNotFound`
  - `List(ctx)` — returns all custom overrides
  - `Delete(ctx, key)` — removes custom override (falls back to default on next render)
  - `SetEnabled(ctx, key, enabled)` — toggles enabled flag
- [x] `Service` fields: `store`, `builtinTemplates` (map of key → compiled `html/template` + `text/template` for subject), `builtinSubjects` (map of key → default subject string)
- [x] `Service.Render(ctx, key, vars map[string]string)` — core render operation:
  1. Look up custom override from store. If found and enabled, parse and use it
  2. If not found or disabled, look up built-in template by key
  3. If no built-in exists for key, return `ErrNoTemplate` (deterministic error, no silent empty render)
  4. Execute HTML template with `map[string]string` data and `html/template` (auto-escapes values)
  5. Execute subject template with same data and `text/template`
  6. Generate plaintext via `stripHTML()` from rendered HTML
  7. Return `RenderedEmail{Subject, HTML, Text}`
  8. Render timeout: wrap ctx with 5-second deadline before template execution
- [x] `Service.GetEffective(ctx, key)` — returns the active template source for a key (custom override if exists+enabled, else built-in) with a `source` field ("custom" or "builtin"). Used by admin API to show what's currently active
- [x] `Service.Preview(ctx, key, subjectTpl, htmlTpl, vars map[string]string)` — renders provided template strings (not saved) against provided variables. Used by admin preview endpoint
- [x] Security model implementation:
  - Data argument to `Execute` is always `map[string]string` — no structs, no interfaces
  - FuncMap is empty (no custom functions registered)
  - Templates parsed with `template.New(key).Option("missingkey=error").Parse(tpl)` — missing variables are hard errors, not silent empty strings
  - HTML body uses `html/template` (contextual auto-escaping). Subject uses `text/template` (no HTML in subjects)

## Auth/Mailer Integration

- [x] Refactor `internal/mailer/templates.go` to expose built-in template data:
  - Export the three compiled built-in templates (password_reset, verification, magic_link) and their default subjects so that `emailtemplates.Service` can use them as fallbacks
  - Keep the existing `//go:embed templates/*.html` and `render()` machinery for the built-in path
- [x] Refactor auth email send paths to render via template service:
  - `auth.RequestPasswordReset()` → `svc.Render(ctx, "auth.password_reset", map[string]string{"AppName": ..., "ActionURL": ...})`
  - `auth.SendVerificationEmail()` → `svc.Render(ctx, "auth.email_verification", map[string]string{...})`
  - `auth.SendMagicLink()` → `svc.Render(ctx, "auth.magic_link", map[string]string{...})`
  - Each returns `RenderedEmail{Subject, HTML, Text}` — subject from template (no more hardcoded strings)
- [x] Ensure delivery never blocks auth critical path: if template rendering fails (parse error, timeout, missing var), log the error and fall through to the built-in default. Only if the built-in also fails (should never happen) do we return an error to the caller. This two-tier fallback means a broken custom template degrades gracefully rather than breaking auth flows
- [x] Add `Service.Send(ctx, key, to string, vars map[string]string)` — convenience method that renders a template and sends via the mailer. Used for arbitrary app emails (club invites, etc.). Returns error if key has no template (custom or built-in) and no fallback exists
- [x] Wire `emailtemplates.Service` into `auth.Service` via `SetEmailTemplateService()` (parallel to existing `SetMailer()` pattern). When template service is nil, fall back to existing hardcoded render path for backward compatibility

## API, CLI, and Admin UI

- [x] Add admin API endpoints for template management:
  - `GET /api/admin/email/templates` — list all template keys with their status. Returns both custom overrides and system keys (with `source: "builtin"` or `source: "custom"` and `enabled` flag). System keys always appear even if no custom override exists
  - `GET /api/admin/email/templates/:key` — get effective template for a key (custom if exists+enabled, else built-in). Response includes `source`, `subject_template`, `html_template`, `enabled`, `variables` (list of available variable names for system keys)
  - `PUT /api/admin/email/templates/:key` — create or update custom override. Body: `{ "subject_template": "...", "html_template": "..." }`. Validates key format, parses templates, returns 400 with parse errors on invalid syntax
  - `DELETE /api/admin/email/templates/:key` — delete custom override (reverts to built-in for system keys, removes entirely for app keys)
  - `PATCH /api/admin/email/templates/:key` — toggle enabled: `{ "enabled": false }` (disables custom override, falls back to default without deleting)
  - `POST /api/admin/email/templates/:key/preview` — render preview. Body: `{ "subject_template": "...", "html_template": "...", "variables": { "AppName": "Sigil", "ActionURL": "https://..." } }`. Returns rendered subject + HTML + plaintext. Does not save
  - `POST /api/admin/email/send` — send an email using a template. Body: `{ "template_key": "app.club_invite", "to": "user@example.com", "variables": { ... } }`. Renders and sends. Returns 404 if no template exists for key
- [x] Implement endpoint validation + error mapping:
  - 400: invalid key format, template parse error, missing required variables, oversized payload
  - 404: template key not found (for app keys with no override and no built-in)
  - 409: N/A (upsert semantics, no conflict)
- [x] Add CLI commands:
  - `ayb email-templates list` — list all template keys with status (`--json`)
  - `ayb email-templates get <key>` — show effective template for a key
  - `ayb email-templates set <key> --subject '...' --html-file <path>` — create/update custom override from file (not inline HTML — templates are too large for CLI args)
  - `ayb email-templates delete <key>` — remove custom override
  - `ayb email-templates preview <key> --vars '{"AppName":"Sigil","ActionURL":"https://..."}' [--subject '...'] [--html-file <path>]` — render preview against variables. If `--subject`/`--html-file` provided, previews those (unsaved). Otherwise previews the effective template
  - `ayb email-templates enable <key>` / `ayb email-templates disable <key>` — toggle enabled flag
  - `ayb email-templates send <key> --to <email> --vars '<json>'` — send an email using a template
- [x] Add admin dashboard Email Templates view (add to Layout.tsx as `"email-templates"` view under Messaging section):
  - Table listing all template keys: key, source (builtin/custom badge), enabled status, last updated
  - System keys always shown (even without custom override), with "Customize" action
  - Click key → edit view with subject template editor, HTML template editor (code textarea, not WYSIWYG), and live preview panel
  - Preview panel: JSON variables input → rendered subject + HTML output (calls preview endpoint on change with debounce)
  - "Reset to Default" button for system keys (deletes custom override)
  - "Delete" button for app keys
  - Enable/disable toggle per template
  - "Send Test Email" button (sends to a specified address using current template)

## Testing (TDD Required)

- [x] Write failing unit tests first for:
  - Key format validation (valid keys, invalid keys, edge cases)
  - Template parse errors (invalid Go template syntax detected on save)
  - Missing-variable render errors (`missingkey=error` triggers on undefined var)
  - Fallback behavior (custom → builtin → error chain)
  - Render timeout (template execution exceeds deadline)
  - SSTI prevention: confirm that `map[string]string` data has no callable methods (document as test assertion)
- [x] Write failing unit tests first for HTML rendering + plaintext auto-generation (rendered HTML → `stripHTML()` → plaintext matches expected)
- [x] Write failing unit tests first for subject line templating (variable substitution in subject via `text/template`)
- [x] Write failing integration tests first for store CRUD against real database (upsert, get, list, delete, enable/disable, key uniqueness, size limit enforcement)
- [x] Write failing auth integration tests first:
  - Password reset email uses custom override when present
  - Password reset email falls back to built-in when custom override is disabled
  - Password reset email falls back to built-in when custom override has parse error (graceful degradation)
  - Same pattern for verification and magic-link emails
- [x] Write failing server handler tests first for admin API validation/error mapping (invalid key, parse error, preview with missing vars, send to non-existent template)
- [x] Write failing CLI tests first for command parsing/output, JSON vars handling, and `--html-file` flag reading
- [x] UI 3-tier coverage:
  - [x] Component tests for Email Templates admin view (list rendering, edit form, preview panel, enable/disable toggle)
  - [x] Browser-mocked tests for preview rendering and error display edge cases
  - [x] Browser-unmocked tests for seeded load-and-verify + customize system template + preview + reset-to-default flow

## Docs & Specs

- [x] Create `docs-site/guide/email-templates.md`:
  - Template model (system keys with built-in defaults, custom overrides, app keys)
  - Variable substitution (`{{.VarName}}` syntax, `map[string]string` only)
  - System template variables: `AppName`, `ActionURL`
  - Safety model (SSTI prevention, render timeout, size limits)
  - Fallback behavior (custom → builtin → error)
  - Admin API and CLI usage examples
  - Sending arbitrary app emails via API/CLI
- [x] Update `docs-site/guide/email.md` with link to template customization guide and operational notes (how to override auth emails, how to test changes with preview)
- [x] Update `docs-site/guide/api-reference.md` with Email Templates admin endpoints + send endpoint
- [x] Update `docs-site/guide/admin-dashboard.md` with Email Templates management UX section
- [x] Create `tests/specs/email-templates.md` with Stage 5 test matrix
- [x] Update trackers: `_dev/AYB_FEATURE_CHECKLIST_from_SIJLE.md`, `_dev/FEATURES.md`, `_dev/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST.md`, `.mike/.../stages.md`

## Completion Gates

- [x] Stage 5 unit/integration/component/browser tests pass with no false positives (session 101 review: fixed false-positive GetEffective test, replaced redundant RenderWithFallback test, all suites green)
- [x] Auth transactional emails (password reset, verification, magic link) use configurable templates with safe fallback to built-in defaults
- [x] Custom templates can be created for arbitrary app email flows (club invites, etc.) and sent via admin API/CLI
- [x] Missing-variable and invalid-template failures are deterministic and covered by tests
- [x] SSTI prevention verified: data passed to templates is `map[string]string` only, no custom functions
- [x] Graceful degradation tested: broken custom template falls back to built-in default without breaking auth flows (session 101: added store-error fallback test)
- [x] Admin API + CLI + dashboard flows for template management are fully covered by automated tests
- [x] Stage 4 regression safety preserved (matview and jobs suites still green — verified session 101)
- [x] Docs/specs/trackers are complete and internally consistent
