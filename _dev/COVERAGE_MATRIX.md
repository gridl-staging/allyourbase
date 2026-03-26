# Browser Test Coverage Matrix

> Single source of truth for **view-level** browser-test coverage across all admin UI views.
> Consumed by Workstream 3 Stages 2-7. Do not maintain parallel coverage lists.
>
> Generated from Stage 1 audit. Source: `ui/src/components/layout-types.ts` (51 View literals),
> cross-checked against `ContentRouter.tsx` routes and `Sidebar.tsx` nav buttons.
>
> Last updated: 2026-03-24 (after MAR24 PM-1 reconciliation; deploy subflow row status lives in `_dev/AUDIT_LEDGER.md` R25-R31)

## How to read this matrix

| Column | Values | Meaning |
|---|---|---|
| **Smoke** | `none` / `heading-only` / `content-verified` | Quality of smoke spec in `browser-tests-unmocked/smoke/` |
| **Full Lifecycle** | `exists` / `not` | Whether a lifecycle/CRUD spec exists in `browser-tests-unmocked/full/` |
| **CRUD-capable** | `yes` / `N/A` | Whether the view has create/update/delete affordances |
| **Mocked** | `exists` / `not` | Whether a mocked spec exists in `browser-tests-mocked/` |

**Load-and-verify compliance** is derivable: a view is compliant if Smoke=`content-verified` OR Full Lifecycle=`exists`.

**Boundary note:** this matrix tracks view-level browser coverage only. Row-level audit status, evidence refs, pardons, and burndown ordering for `R01`-`R31` are owned by `_dev/AUDIT_LEDGER.md`.

**Cross-cutting specs** (`admin-login.spec.ts`, `dark-mode-persistence.spec.ts`, `theme-persistence.spec.ts`) are formally exempt from the one-row-per-view mapping — they test auth/theme flows that span multiple views.

**Multi-view specs** use a primary-view convention for counting. Secondary coverage is noted in the Evidence Specs column but doesn't add a separate matrix row.

## Coverage Matrix

| View | Smoke | Full Lifecycle | CRUD-capable | Mocked | Evidence Specs |
|---|---|---|---|---|---|
| `data` | content-verified | exists | yes | not | collections-create, table-browser-crud, collections-crud, table-browser-advanced, blog-platform-journey |
| `schema` | content-verified | not | N/A | not | schema-view |
| `sql` | content-verified | exists | yes | not | sql-view, sql-lifecycle |
| `webhooks` | content-verified | exists | yes | exists | webhooks-crud, webhooks-lifecycle, dashboard-webhook-delivery-journey (secondary), webhooks-error-flows (mocked) |
| `storage` | content-verified | exists | yes | exists | storage-upload, storage-lifecycle, storage-error-flows (mocked) |
| `sites` | content-verified | exists | yes | not | sites-hosting, sites-lifecycle (view-level Sites coverage; deploy subflow row status lives in `_dev/AUDIT_LEDGER.md` `R25`-`R31`) |
| `users` | content-verified | exists | yes | not | users-list, users-lifecycle |
| `functions` | content-verified | exists | N/A | not | functions-list, functions-browser |
| `edge-functions` | content-verified | exists | yes | exists | edge-functions-crud, edge-function-triggers, edge-functions (mocked) |
| `apps` | content-verified | exists | yes | exists | apps-list, apps-lifecycle, apps-toast-outcomes (mocked) |
| `api-keys` | content-verified | exists | yes | exists | api-keys-list, api-keys-lifecycle, api-keys-error-flows (mocked) |
| `oauth-clients` | content-verified | exists | yes | exists | oauth-clients-list, oauth-clients-lifecycle, oauth-clients-error-flows (mocked); row-level OAuth subflow status for `R02`/`R03` remains in `_dev/AUDIT_LEDGER.md` |
| `api-explorer` | content-verified | exists | N/A | not | api-explorer-view, api-explorer |
| `rls` | content-verified | exists | yes | not | rls-policies-view, rls-policies |
| `sql-editor` | content-verified | exists | yes | exists | admin-sql-query, create-table-nav, admin-dashboard-setup, sql-editor-lifecycle, sql-editor-error-flows (mocked) |
| `schema-designer` | content-verified | exists | N/A | exists | schema-designer-table, schema-designer-lifecycle, schema-designer (mocked) |
| `sms-health` | content-verified | exists | N/A | not | sms-health, sms-dashboard (secondary) |
| `sms-messages` | content-verified | exists | yes | not | sms-messages, sms-dashboard |
| `email-templates` | content-verified | exists | yes | exists | email-templates-list, email-templates-lifecycle, email-templates-preview (mocked) |
| `push` | content-verified | exists | yes | exists | push-devices, push-notifications-lifecycle, push-notifications (mocked) |
| `jobs` | content-verified | exists | yes | not | jobs-list, jobs-management |
| `schedules` | content-verified | exists | yes | not | schedules-list, schedules-lifecycle |
| `matviews` | content-verified | exists | yes | not | matviews-list, matviews-lifecycle |
| `auth-settings` | content-verified | exists | yes | exists | auth-settings-view, auth-provider-management, auth-mfa-lifecycle; auth-provider-management (mocked) |
| `mfa-management` | content-verified | exists | yes | exists | mfa-management-view, auth-mfa-lifecycle; auth-mfa-error-flows (mocked) |
| `account-linking` | content-verified | exists | yes | exists | account-linking-view, auth-mfa-lifecycle; auth-mfa-error-flows (mocked, secondary) |
| `branches` | content-verified | exists | yes | not | branches-crud, branches-lifecycle |
| `realtime-inspector` | content-verified | exists | N/A | exists | realtime-inspector-view, realtime-inspector-lifecycle, realtime-inspector (mocked) |
| `security-advisor` | content-verified | exists | N/A | exists | security-advisor-view, advisors-lifecycle, security-advisor (mocked) |
| `performance-advisor` | content-verified | exists | N/A | exists | performance-advisor-view, advisors-lifecycle, performance-advisor (mocked) |
| `backups` | content-verified | exists | yes | not | backups, backups-lifecycle |
| `analytics` | content-verified | exists | N/A | not | analytics, analytics-lifecycle |
| `usage` | content-verified | not | N/A | exists | usage-metering, usage-metering (mocked) |
| `replicas` | content-verified | exists | yes | not | replicas, replicas-lifecycle |
| `ai-assistant` | content-verified | exists | yes | not | ai-assistant, ai-assistant-lifecycle |
| `audit-logs` | content-verified | exists | N/A | not | audit-logs, audit-logs-lifecycle |
| `admin-logs` | content-verified | not | N/A | exists | admin-logs, admin-logs (mocked) |
| `secrets` | content-verified | exists | yes | exists | secrets, secrets-lifecycle, secrets-error-flows (mocked) |
| `saml` | content-verified | exists | yes | not | saml, saml-lifecycle |
| `custom-domains` | content-verified | exists | yes | not | custom-domains, custom-domains-lifecycle |
| `extensions` | content-verified | exists | yes | not | extensions, extensions-lifecycle |
| `vector-indexes` | content-verified | exists | yes | not | vector-indexes, vector-indexes-lifecycle |
| `log-drains` | content-verified | exists | yes | not | log-drains, log-drains-lifecycle |
| `stats` | content-verified | not | N/A | not | stats |
| `auth-hooks` | content-verified | exists | yes | not | auth-hooks, auth-hooks-lifecycle |
| `notifications` | content-verified | exists | yes | exists | notifications, notifications-lifecycle, notifications-error-flows (mocked) |
| `fdw` | content-verified | exists | yes | exists | fdw, fdw-lifecycle, fdw-error-flows (mocked) |
| `incidents` | content-verified | exists | yes | exists | incidents, incidents-lifecycle, incidents-error-flows (mocked) |
| `support-tickets` | content-verified | exists | yes | exists | support-tickets, support-tickets-lifecycle, support-tickets-error-flows (mocked) |
| `tenants` | content-verified | exists | yes | exists | tenants, tenants-lifecycle, tenants (mocked) |
| `organizations` | content-verified | exists | yes | exists | organizations, organizations-lifecycle, organizations (mocked) |

## Gap Summary

| Metric | Count |
|---|---|
| Total views | 51 |
| Smoke = none | 0 |
| Smoke = heading-only | 0 |
| Smoke = content-verified | 51 |
| All views with smoke coverage | 51/51 (100%) |
| Views with full lifecycle specs | 47 |
| CRUD-capable views missing full lifecycle | 0 |
| Views missing mocked coverage | 26 |

## Stage Gap Lists

### Stage 3 — Smoke specs needing rewrite (heading-only)

**COMPLETED.** All 18 previously heading-only admin smoke specs have been upgraded to content-verified (commits `4a491e6`, `d99c621`, `330fd97`). The orphan `admin-login.spec.ts` remains heading-only by design.

### Stage 4 — Admin views with no smoke spec

**COMPLETED.** All 51 views (48 admin + 3 data) now have dedicated smoke specs. Admin views closed in commits `330fd97`–`be00895`; data views `schema` and `sql` closed in `2a2c6aa`.

#### Stage 4 upgrade pass — 14 weak smoke specs upgraded to content-verified quality

**COMPLETED.** 14 specs that were nominally `content-verified` but still at chrome-only quality (asserting headings/buttons/labels without seeding data or verifying API state) have been upgraded to fixture-backed, deterministic content verification. Static validation passes (lint 0 errors, typecheck clean, `--list` 121 tests, hygiene 8/8). Runtime Playwright execution deferred to CI (local environment lacks pg_cron for managed Postgres startup).

**Data-seeded specs (6)** — seed deterministic data via API/SQL, assert seeded content renders, clean up in `afterEach`:

| Spec | Seed method | Assertion target | Cleanup |
|---|---|---|---|
| `functions-list` | `execSQL` (CREATE FUNCTION) | Function name in list | `execSQL` (DROP FUNCTION) |
| `email-templates-list` | `seedEmailTemplate` fixture | Template key button + subject input value | `cleanupEmailTemplate` fixture |
| `push-devices` | `seedPushDeviceToken` fixture | Token preview in Devices tab row | `cleanupPushTestData` fixture |
| `ai-assistant` | `seedAIPrompt` fixture | Prompt name in Prompts tab table | `cleanupAIPromptByName` fixture |
| `extensions` | `enableExtension` fixture | Extension row shows installed + Disable action | `disableExtension` fixture (conditional) |
| `vector-indexes` | `execSQL` (CREATE TABLE + INDEX) | Index name/schema/table/method in table row | `execSQL` (DROP TABLE) |

**Config-state specs (8)** — fetch live API state via fixture helper, assert rendered UI matches:

| Spec | Fixture read | Assertion target |
|---|---|---|
| `auth-hooks` | `fetchAuthHooksConfig` | 6 hook cards with per-key value text matching API state |
| `api-explorer-view` | `fetchAdminStatsSnapshot` | Response status `200 OK` + body keys (`uptime_seconds`, `go_version`) |
| `auth-settings-view` | `fetchAuthSettings` | 5 toggle checkboxes match API boolean state |
| `mfa-management-view` | `fetchAuthSettings` + `createLinkedEmailAuthSessionToken` | Enrollment buttons visible, TOTP flow transition/cancel cycle |
| `account-linking-view` | `fetchAuthSettings` | Anonymous session + email/password link form + link flow |
| `realtime-inspector-view` | `fetchRealtimeStats` | Metric card values match API snapshot totals |
| `security-advisor-view` | `fetchSecurityAdvisorReport` | Findings or empty-state consistent with API report |
| `performance-advisor-view` | `fetchPerformanceAdvisorReport` | Time-range selector + table-or-empty consistent with API report |

**New fixture helpers added** (all in `ui/browser-tests-unmocked/fixtures/`):

- `admin.ts`: `seedEmailTemplate`, `cleanupEmailTemplate`, `fetchAuthSettings`, `fetchRealtimeStats`, `fetchSecurityAdvisorReport`, `fetchPerformanceAdvisorReport`
- `auth.ts`: `createLinkedEmailAuthSessionToken` (linked email auth session for MFA spec)
- `core.ts`: `waitForDashboard(page)` (sidebar-based dashboard-ready gate replacing brittle brand-text checks)

**Component selector aids added**:

- `AuthHooks.tsx`: `data-testid="auth-hook-card-${key}"` and `data-testid="auth-hook-value-${key}"` per hook card
- `RealtimeInspector.tsx`: `data-testid` prop on `MetricCard` (`realtime-metric-total`, `-authenticated`, `-anonymous`, `-churn`) with `-value` suffix for value element

**Scope corrections documented for audit accuracy**:

- **`mfa-management`**: Enrollment flow verification only — `MFAEnrollment.tsx` renders TOTP/Email enrollment buttons unconditionally (does not read admin auth-settings), so spec asserts button visibility and enrollment step transitions, not config-coupled conditional rendering. Uses linked email auth token via `createLinkedEmailAuthSessionToken` because backend blocks MFA enrollment for anonymous users.
- **`account-linking`**: Email/password linking only — `AccountLinking.tsx` renders only anonymous session + email/password link form; `linkOAuth()` exists in `api_auth.ts` but is never imported by the component; no provider dropdown/selector exists. Spec skips when `anonymous_auth_enabled` is false.
- **Realtime inspector**: Frontend adapter `adaptRealtimeStatsToSnapshot` bridges backend `/api/admin/realtime/stats` payload shape (`connections/subscriptions/counters/version/timestamp`) to frontend `RealtimeInspectorSnapshot` shape (`summary/channels/throughput/degraded`). Endpoint URL aligned from stale `/realtime/inspector` to `/realtime/stats`.
- **Security/Performance advisors**: Backend route group `/api/admin/advisors/{security,performance}` added with admin-auth gating and deterministic report payload stubs (covered by `advisor_admin_handler_test.go`). These endpoints were previously missing from `internal/server` route wiring.
- **Vector indexes**: Frontend `listVectorIndexes()` in `api_vector.ts` now unwraps `{ indexes: [...] }` wrapped backend payload (covered by `api_vector_response_shape.test.ts`).

**Infrastructure improvements**:

- `waitForDashboard(page)` in `fixtures/core.ts` replaces inline `getByText("Allyourbase")` brand-text checks across all 14 specs — waits for sidebar `<aside>` element with 15s timeout, tolerating slow schema fetches under parallel test load.
- `auth.setup.ts` hardened to use token-based post-login readiness check (`localStorage.getItem("ayb_admin_token")`) instead of URL/sidebar navigation gating.
- 8 hygiene regression tests in `browserUnmockedHygiene.test.ts` enforce test patterns: no credential logging, MFA lifecycle cleanup, AAL2 step-up, token-based auth setup, `waitForDashboard` usage, fixtures barrel export, auth config seeding, and email template key format.

### Stage 5 — CRUD-capable views missing full-lifecycle coverage

**COMPLETED.** The remaining CRUD-capable gaps are now closed with dedicated unmocked full-lifecycle specs:
`sites` (`sites-lifecycle.spec.ts`), `tenants` (`tenants-lifecycle.spec.ts`), and
`organizations` (`organizations-lifecycle.spec.ts`).

This Stage 5 closeout is strictly view-level (`Full Lifecycle = exists`). It does not override narrower row-level subflow gaps tracked in `_dev/AUDIT_LEDGER.md` (for example `R02`, `R03`, and `R25`-`R31`).

The previously tracked Stage 5 closeout remains true for the original 22-row audit set: the final 5 closed there were `sql` (sql-lifecycle), `sql-editor` (sql-editor-lifecycle), `backups` (backups-lifecycle), `replicas` (replicas-lifecycle), and `auth-hooks` (auth-hooks-lifecycle).

### Stage 6 — Views missing mocked coverage

26 views remaining (down from 35). 10 highest-priority mutation surfaces now covered:
`api-keys`, `oauth-clients`, `webhooks`, `storage`, `secrets`, `sql-editor`, `notifications`, `support-tickets`, `incidents`, `fdw`.

## Prioritization (product-surface-first)

1. ~~Close untested CRUD admin surfaces first~~ — **DONE**: all 51 views now have smoke coverage.
2. ~~Rewrite heading-only smoke on operationally critical pages~~ — **DONE**: all heading-only specs upgraded.
3. **Add mocked error-state tests** for high-risk mutation surfaces: `api-keys`, `oauth-clients`, `webhooks`, `storage`, `sql-editor`, `notifications`, `support-tickets`.
4. ~~Resolve mapping-policy for cross-cutting specs~~ — **DONE**: formal exemption documented.
5. ~~Add full-lifecycle coverage for remaining CRUD-capable views~~ — **DONE**: `sites`, `tenants`, and `organizations` now have dedicated full-lifecycle specs.
