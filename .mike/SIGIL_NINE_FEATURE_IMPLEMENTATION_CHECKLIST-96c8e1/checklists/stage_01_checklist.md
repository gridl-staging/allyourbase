# Stage 1: Per-App API Key Scoping

## Discovery & Design

- [x] Read existing API key implementation (`internal/auth/apikeys.go`, `apikeys_handler.go`, `apikeys_test.go`) and trace enforcement path through middleware
- [x] Read existing API key admin UI (`ui/src/ApiKeys.tsx`), CLI (`internal/cli/apikeys.go`), and SDK usage (`sdk/src/client.ts`)
- [x] Read requirements source: `_dev/AYB_FEATURE_CHECKLIST_from_SIJLE.md` (Per-app API key scoping section) and `handoff_207`
- [x] Research at least 2 app identity models (e.g., flat app-key namespacing vs org/project hierarchy vs OAuth client_id pattern) and document tradeoffs
- [x] Record chosen approach and rejected alternatives in `_dev/ARCHITECTURE_DECISIONS.md`
- [x] Define explicit non-goals for v1 app scoping (e.g., no multi-org hierarchy, no billing integration)

## Database Schema

- [x] Design and write migration SQL: `apps` table (id, name, description, owner_user_id, created_at, updated_at)
- [x] Design and write migration SQL: add `app_id` nullable FK column to `api_keys` table (null = legacy user-scoped key)
- [x] Design and write migration SQL: `app_rate_limits` table or rate-limit columns on `apps` (requests/window, configurable per app)
- [x] Write tests for migration: apply, verify schema, rollback

## Backend Implementation

- [x] Implement `internal/auth/apps.go`: App struct, CreateApp, GetApp, ListApps, UpdateApp, DeleteApp with DB operations
- [x] Implement `internal/auth/apps_handler.go`: HTTP handlers for app CRUD (POST/GET/PUT/DELETE `/api/admin/apps`)
- [x] Wire app routes into server router (`internal/server/server.go`)
- [x] Extend `apikeys.go` CreateAPIKey to accept optional `app_id` parameter; store app association
- [x] Extend API key validation (`ValidateAPIKey`) to load and enforce app-level scope restrictions
- [x] Implement per-app rate limiting: track request counts per app_id, enforce configurable limits in middleware
- [x] Write failing tests first for: app CRUD operations, app-scoped key creation, scope enforcement, rate limiting
- [x] Make all tests pass

## CLI

- [x] Add `ayb apps create <name>` command with `--description` flag
- [x] Add `ayb apps list` command with JSON output support
- [x] Add `ayb apps delete <id>` command
- [x] Extend `ayb apikeys create` with `--app <id>` flag to create app-scoped keys
- [x] Write tests for all new CLI commands

## Admin Dashboard

- [x] Add Apps management page: list apps, create app, delete app
- [x] Add app selector to API key creation flow (optional: "scope to app" dropdown)
- [x] Show app association on API key list view
- [x] Show per-app rate limit usage/stats
- [x] Write component tests for new UI

## SDK & Docs

- [x] Update TypeScript SDK type definitions if API response shapes change
- [x] Update `docs-site/guide/api-reference.md` with app scoping endpoints and usage
- [x] Update `docs-site/guide/admin-dashboard.md` with app management UI
- [x] Add app scoping section to `docs-site/guide/configuration.md` if config changes needed
- [x] Update `tests/specs/admin.md` with app scoping test cases

## Completion Gates

- [x] All new tests green (unit + integration)
- [x] Existing API key tests still pass (backward compatibility: null app_id = legacy behavior)
- [x] Negative tests: app-scoped key denied access to out-of-scope tables/operations
- [x] Rate limiting test: app exceeding limit gets 429
- [x] Update `_dev/AYB_FEATURE_CHECKLIST_from_SIJLE.md` to mark Per-app API key scoping complete
- [x] Update `_dev/FEATURES.md` to reflect app scoping
