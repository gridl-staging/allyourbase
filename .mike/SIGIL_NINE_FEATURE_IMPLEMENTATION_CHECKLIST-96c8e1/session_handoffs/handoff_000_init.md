# Handoff 000 — Init (Stage Breakdown)

## What Was Done

Read the full Sigil nine-feature implementation checklist and explored the AYB codebase architecture to understand existing systems, then broke the 9 features into 8 dependency-ordered stages.

### Stage Breakdown Rationale

The 9 features map to 8 stages (mat view refresh and joined-table RLS combined into one stage since both are data/realtime correctness concerns at ~4 sessions each):

1. **Per-App API Key Scoping** (~5 sessions) — Extends existing `internal/auth/apikeys.go`. Creates app identity model (apps table, app-scoped keys, per-app rate limits). Must come first because OAuth provider mode (Stage 2) needs app registration.

2. **OAuth 2.0 Provider Mode** (~8 sessions) — Builds authorization server on top of Stage 1's app model. AYB currently only *consumes* OAuth (`internal/auth/oauth.go` handles Google/GitHub login). This adds the *provider* side: `/authorize`, `/token`, `/revoke` endpoints, consent flow, authorization codes.

3. **Job Queue & Scheduler** (~7 sessions) — No existing job infrastructure (only `webhookDispatcher.StartPruner` runs on a timer). Needed before Stage 4 so mat view refresh can use scheduled jobs.

4. **Data Layer Hardening** (~8 sessions) — Two related changes: (a) Mat view refresh triggers (scheduled via Stage 3's job queue + on-demand API/CLI), (b) Joined-table RLS in SSE — `internal/realtime/handler.go` currently does single-table visibility checks; needs full RLS policy evaluation for join-based membership policies.

5. **Custom Email Templates** (~4 sessions) — `internal/mailer/templates.go` has 3 hardcoded templates with only `AppName` and `ActionURL` variables. Needs user-defined templates with arbitrary variables.

6. **Push Notifications** (~7 sessions) — New channel. Existing `internal/sms/` (20 files, 6 providers) is a good architectural reference for provider abstraction pattern.

7. **Dart/Flutter SDK** (~10 sessions) — New SDK. Existing `sdk/src/client.ts` is the TypeScript reference implementation to mirror.

8. **PostGIS Extension Support** (~4 sessions) — Most independent. Research-heavy (embedded vs external Postgres strategy), moderate implementation.

### Key Architectural Observations

- **API keys already have scope + per-table restrictions** — Stage 1 extends this with app-level grouping, not replaces it.
- **OAuth consumer code is solid** (501-line `oauth.go`, comprehensive tests) — Stage 2 adds provider endpoints alongside, no need to refactor consumer code.
- **No background processing exists** — Stage 3 is greenfield. Webhook pruner goroutine is the only precedent.
- **SSE hub is small** (3 files in `internal/realtime/`) — Stage 4's RLS fix is surgical.
- **SMS provider pattern** (`internal/sms/`: interface + 6 provider implementations) — Stage 6 should follow the same pattern for push providers.
- **Server uses chi router** — all new endpoints wire into `internal/server/server.go`.

## Files Created/Modified

- **Created:** `.mike/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST-96c8e1/stages.md` — 8-stage breakdown
- **Created:** `.mike/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST-96c8e1/checklists/stage_01_checklist.md` — Detailed checklist for Per-App API Key Scoping
- **Modified:** `_dev/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST.md` — Replaced "Proposed Execution Order" with finalized stage list and rationale

## What's Next

Stage 1, Session 1: Begin Per-App API Key Scoping discovery. Read existing API key implementation in detail, research app identity models, document design decision in `_dev/ARCHITECTURE_DECISIONS.md`.
