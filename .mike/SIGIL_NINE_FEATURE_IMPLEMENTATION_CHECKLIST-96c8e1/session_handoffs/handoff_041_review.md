# Handoff 041 — Stage 2 Review (Consent/Login Redirect Hardening)

## What I reviewed

Focused review of the most recent Stage 2 OAuth provider-mode work (sessions 037-040), with emphasis on:
- OAuth consent/login UX flow correctness
- false-positive/insufficient UI regression tests
- open-redirect and boundary security behavior
- checklist/input-file consistency updates

## Bugs/issues found and fixed

1. **Consent/login flow mismatch (functional bug):**
   - `OAuthConsent` wrote `return_to` using absolute URL (`window.location.href`), while `App` only redirected if `return_to` started with `/`.
   - Result: unauthenticated consent flow could fail to return users to consent after login.

2. **Protocol-relative open redirect bypass (security bug):**
   - `App` used `returnTo.startsWith("/")` as guard.
   - Payload like `//evil.com/steal` passes that guard and can redirect off-origin.

3. **False-positive coverage gap in tests:**
   - Existing open-redirect test only used `https://evil.com` and missed protocol-relative bypass.
   - Existing consent/login tests did not assert path-shape interoperability tightly enough.

## TDD (red → green)

### Red tests added/strengthened first
- `ui/src/components/__tests__/App.test.tsx`
  - strengthened redirect tests to cover bypass attempt with `//evil.com`.
- `ui/src/components/__tests__/OAuthConsent.test.tsx`
  - strengthened 401 redirect test to assert `return_to` shape is a relative consent path.

Both test files failed before fixes, confirming defects.

### Green implementation
- `ui/src/components/OAuthConsent.tsx`
  - changed 401 `return_to` to relative URL (`pathname + search + hash`) instead of absolute `href`.
- `ui/src/App.tsx`
  - added `normalizeReturnTo()` that resolves and validates same-origin return targets and returns safe path/search/hash.
  - rejects off-origin and protocol-relative redirect attempts.

### Focused tests run
- `cd ui && npx vitest run src/components/__tests__/App.test.tsx`
- `cd ui && npx vitest run src/components/__tests__/OAuthConsent.test.tsx`
- `cd ui && npx vitest run src/components/__tests__/OAuthClients.test.tsx`

All pass.

## Browser/UI standards check

Checked browser spec files against `resources/BROWSER_TESTING_STANDARDS_2.md` patterns. Found legacy violations in existing unmodified browser specs (e.g. `waitForTimeout`, CSS selectors, `page.evaluate`) under `ui/browser-tests-unmocked/` including `debug-login.spec.ts` and several smoke/full specs. No browser-test files were changed in this session.

## Checklist/input updates

Updated:
- `.mike/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST-96c8e1/checklists/stage_02_checklist.md`
  - added new 2026-02-22 UI auth-flow hardening review note.
- `_dev/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST.md`
  - added corresponding Stage 2 review hardening note.

## Files modified

- `ui/src/App.tsx`
- `ui/src/components/OAuthConsent.tsx`
- `ui/src/components/__tests__/App.test.tsx`
- `ui/src/components/__tests__/OAuthConsent.test.tsx`
- `.mike/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST-96c8e1/checklists/stage_02_checklist.md`
- `_dev/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST.md`

## Files created

- `.mike/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST-96c8e1/session_handoffs/handoff_041_review.md`

## What’s next

1. Triage/fix legacy browser-spec standards violations in `ui/browser-tests-unmocked/**/*.spec.ts` to fully align with `BROWSER_TESTING_STANDARDS_2.md`.
2. Continue remaining Stage 2 docs/checklist completion gates.
