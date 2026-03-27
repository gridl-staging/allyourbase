# Priorities

**Last updated:** 2026-03-27

This file tracks the current priority stack. For detailed feature inventory see `_dev/FEATURES.md`, for execution phases see `_dev/PHASES.md`, and for public-facing roadmap see `ROADMAP.md`.

## P0 — Urgent

1. **Keep CI trustworthy** — `make test-all` was restored on 2026-03-27 via integration-only runner selection for `internal/api` / `internal/server`, duplicate-name guardrails in the helper, and a no-cache integration test pool that avoids pgx cached-plan flakes across destructive schema resets. Focus now shifts to keeping CI green post-merge, extending proof coverage to any new routes, and ensuring staging CI stays green after debbie sync.

## P1 — Important, Next Up

2. **Browser-test lint debt cleanup** — The dedicated browser-test ESLint sweep is now wired and passing with `0` errors, but it still reports a large warning backlog (conditional assertions, intentional skips, legacy patterns). Clean that debt down so browser-test lint becomes a more useful signal.

3. **Production proof expansion** — Keep extending real end-to-end proof around newly landed routes and operator workflows so the green audit ledger stays honest rather than drifting behind feature work.

## P2 — Important, Not Urgent

4. **Production confidence testing** — Load testing (k6 suite exists in `tests/load/`), fuzz-hardening (RPC deserialization, MIME validation still pending), chaos testing (Postgres restart, storage failover, clock skew), accessibility scanning (zero a11y tests).

5. **Owner-side publishing alignment** — The canonical public module/release identity is still unresolved in `_dev/MODULE_PATH_DECISION.md`. Docs and installer surfaces currently point at `gridlhq/allyourbase`, while `go.mod` still says `github.com/allyourbase/ayb`.

6. **Public Docker publication posture** — Cloudflare secret presence and docs-deploy evidence are now verified, but GHCR publication is still not trustworthy: the `ghcr.io/gridlhq/allyourbase` package is private and the latest public Docker workflow runs failed with a GHCR `403`. See `_dev/RELEASE_SECRETS_AUDIT.md`.

## Recently Completed

- **Supabase live-validation refresh and doc reconciliation** — Re-ran the local Supabase live profile on 2026-03-27, confirmed the recent self-hosted refresh from 2026-03-26, and tightened the live-fixture seed script to tolerate app-owned `auth.users` triggers that write to `public.profiles` as well as `public.tenants`. Priority / roadmap / feature docs now distinguish active-scope Supabase proof from deferred Firebase work instead of lumping them together.
- **Installer latest-release selection hardened** — `install.sh` now selects the latest `v*` AYB app release rather than the repo-wide latest release, so auxiliary `pg-*` releases no longer break auto-detection or `tests/test_install.sh`.
- **Fast-suite baseline restored** — `make test-all` is now repeatably green again. The repair combined integration-only selection for `internal/api` / `internal/server`, a helper guard that rejects integration/unit name collisions, two renamed `internal/api` integration tests to avoid accidental unit-test re-entry, and a no-cache pgx integration test pool to eliminate `cached plan must not change result type` flakes across repeated schema resets. Validation on 2026-03-27: `bash scripts/run-integration-tests.sh` x3 green, `make test-all` x3 green, and `internal/api` integration-only stress x10 green.
- **DX audit closure** — `_dev/dx-audit-services.md` is now 100% closed. The last open recommendation is proven by `ui/browser-tests-unmocked/full/edge-function-log-filters.spec.ts`.
- **Storage upload hardening** — `internal/storage/handler.go` now applies a configurable server-side upload timeout (default 5 minutes) with focused unit and integration coverage.
- **Priority-doc consolidation** — `PRIORITIES.md` is now the single source of truth for priority ordering; `ROADMAP.md` and `_dev/PHASES.md` now point to it instead of duplicating the same stack.
