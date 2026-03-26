# Changelog

All notable changes to Allyourbase will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Added
- Web hosting MVP: `ayb sites deploy` uploads static sites with deploy/promote/rollback lifecycle, host-based routing, and SPA fallback serving via the admin API
- Admin dashboard: new panels for organizations, tenants, usage metering, request logs, realtime event inspector, storage browser with CDN purge, and site/deploy management
- CLI UX overhaul: shared design system with branded output, animated step spinners, and styled error suggestions with fix hints
- `ayb demo` serves pre-built example apps with no Node.js dependency — embedded assets, API reverse proxy, and SPA fallback in a single command
- MCP server for AI coding tools (`ayb mcp`) — 13 tools, 2 resources, 3 prompts
- `ayb init` project scaffolding with 4 templates (React, Next.js, Express, plain TS)
- `ayb db backup` and `ayb db restore` commands
- `ayb stats` for server statistics
- `ayb rpc` for calling PostgreSQL functions from CLI
- `ayb query` for querying records from CLI
- Security audit: auth bypass, RLS enforcement, API key scoping, secrets handling
- Performance baseline: 1.9K–21K req/s, 310ms startup, 20.5MB RSS
- OpenAPI spec served at `/api/openapi.yaml`

### Changed
- HTTP timeout hardening: read-header, idle, and TLS redirect timeouts on the server; 30-second default on CLI HTTP client; config file written with 0600 permissions
- Go 1.25 (upgraded from 1.24)
- License clarified as MIT across all artifacts
- Documentation expanded across guides, API references, and feature inventory

## [0.1.0] - 2026-02-08

Initial release.

### Added
- Single Go binary with embedded admin dashboard
- Auto-generated REST API from PostgreSQL schema (CRUD, filter, sort, search, pagination, FK expand, batch)
- Auth: email/password, JWT, OAuth (Google, GitHub), password reset, email verification
- Row-Level Security via Postgres RLS with JWT claims injected into session vars
- Realtime via Server-Sent Events with RLS-filtered change subscriptions
- File storage on local disk or S3-compatible object stores with signed URLs
- Webhooks with HMAC-SHA256 signing, retry with exponential backoff
- TypeScript SDK with auth state management, realtime subscriptions, OAuth flows
- CLI coverage for core operational workflows (start, stop, config, migrate, types, webhooks, storage, users, apikeys)
- Managed PostgreSQL for zero-config development (`ayb start` downloads Postgres automatically)
- Migration tools: PocketBase, Supabase, Firebase — one-command import with auth user preservation
- Non-expiring API keys with scope enforcement (readonly/readwrite/full, per-table restrictions)
- Full-text search via Postgres tsquery with relevance ranking
- Type generation from live schema (`ayb types typescript`)
- Email backends: log, SMTP, webhook
- Password hashing: argon2id, bcrypt, firebase-scrypt with progressive re-hashing
- Two example apps: Live Polls, Kanban Board
