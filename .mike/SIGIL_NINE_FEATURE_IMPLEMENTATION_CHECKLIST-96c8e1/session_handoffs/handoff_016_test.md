# Handoff 016 — Stage 1 Test Review + Hardening

## What I did

1. Reviewed Stage 1 test coverage against completed checklist items.
2. Ran focused Stage 1 suites (backend + UI component) and targeted browser-test validation commands.
3. Refactored the unmocked browser API-key lifecycle spec to be faster and standards-compliant.
4. Added explicit Stage 1 app-scoping test cases to `tests/specs/admin.md`.
5. Updated stage checklist and input tracker notes.

## Key changes

### 1) Browser-unmocked API key lifecycle test hardened
File: `ui/browser-tests-unmocked/full/api-keys-lifecycle.spec.ts`

- Replaced slow setup-through-UI (SQL Editor typing) with fixture-driven SQL arrange (`execSQL`) helpers.
- Removed raw locator usage that violated browser-test lint rules.
- Added app-scoped lifecycle coverage:
  - create app-scoped API key
  - verify app name + app rate in created modal
  - verify app name + app rate in list row
  - revoke and verify revoked status
- Kept load-and-verify seeded-row coverage.

### 2) Admin test specification updated for Stage 1 app scoping
File: `tests/specs/admin.md`

Added test cases:
- `TC-ADMIN-APP-001` Apps CRUD via admin API
- `TC-ADMIN-APP-002` Create app-scoped API key
- `TC-ADMIN-APP-003` Scope enforcement for app-scoped keys
- `TC-ADMIN-APP-004` App rate-limit enforcement
- `TC-ADMIN-APP-005` Browser-unmocked app-scoped lifecycle

### 3) Stage artifacts updated
- Marked checklist item complete:
  - `.mike/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST-96c8e1/checklists/stage_01_checklist.md`
  - `Update tests/specs/admin.md with app scoping test cases` -> `[x]`
- Added session progress note:
  - `_dev/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST.md`

## Test results

### Passed
- `go test ./internal/auth -run 'Test(App|APIKey|ValidScopes|Claims|CheckWriteScope|CheckTableScope|ApplyAppRateLimitClaims|MapCreateAPIKeyInsertError|HandleCreateAPIKey|HandleListAPIKeys|HandleRevokeAPIKey|HandleAPIKeyRoutesRegistered|AppRateLimiter)'`
- `go test ./internal/server -run 'Test(AdminListApps|AdminGetApp|AdminCreateApp|AdminUpdateApp|AdminDeleteApp|AdminListAPIKeys|AdminRevokeAPIKey|AdminCreateAPIKey|RequireAdminOrUserAuth)'`
- `npm test -- --run src/components/__tests__/ApiKeys.test.tsx src/components/__tests__/Apps.test.tsx`
- `npx eslint browser-tests-unmocked/full/api-keys-lifecycle.spec.ts --config browser-tests-unmocked/eslint.config.mjs`
- `npx playwright test browser-tests-unmocked/full/api-keys-lifecycle.spec.ts --project=full --list`

### Blocked by sandbox constraints
- CLI tests in `internal/cli` that use `httptest.NewServer` (socket bind denied in sandbox).
- Executing Playwright browser tests (Chromium launch denied in sandbox).

## Coverage gaps identified

1. Browser tier imbalance remains: there is no `ui/browser-tests-mocked/` suite yet, despite 3-tier standard guidance.
2. Large set of pre-existing lint violations exists across other browser-unmocked specs (outside this file/session scope).
3. CLI tests cannot be validated in this sandbox due local socket restrictions; should be executed in CI/local unrestricted environment.

## Files modified/created

Modified:
- `ui/browser-tests-unmocked/full/api-keys-lifecycle.spec.ts`
- `tests/specs/admin.md`
- `.mike/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST-96c8e1/checklists/stage_01_checklist.md`
- `_dev/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST.md`

Created:
- `.mike/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST-96c8e1/session_handoffs/handoff_016_test.md`

## What’s next

1. Run blocked Stage 1 CLI and browser-unmocked execution tests in an environment with socket + browser permissions.
2. Triage/fix broader browser-unmocked lint violations outside `api-keys-lifecycle.spec.ts`.
3. Add browser-mocked tier skeleton (`ui/browser-tests-mocked/`) to satisfy 3-tier policy end-to-end.
