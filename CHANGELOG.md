# Changelog

All notable changes to Allyourbase will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Added
- MCP server for AI coding tools (`ayb mcp`) — 11 tools, 2 resources, 3 prompts
- `ayb init` project scaffolding with 4 templates (React, Next.js, Express, plain TS)
- `ayb db backup` and `ayb db restore` commands
- `ayb stats` for server statistics
- `ayb rpc` for calling PostgreSQL functions from CLI
- `ayb query` for querying records from CLI
- Security audit: auth bypass, RLS enforcement, API key scoping, secrets handling
- Performance baseline: 1.9K–21K req/s, 310ms startup, 20.5MB RSS
- OpenAPI spec served at `/api/openapi.yaml`

### Changed
- Go 1.25 (upgraded from 1.24)
- License clarified as MIT across all artifacts

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
- CLI with full dashboard parity (start, stop, config, migrate, types, webhooks, storage, users, apikeys)
- Managed PostgreSQL for zero-config development (`ayb start` downloads Postgres automatically)
- Migration tools: PocketBase, Supabase, Firebase — one-command import with auth user preservation
- Non-expiring API keys with scope enforcement (readonly/readwrite/full, per-table restrictions)
- Full-text search via Postgres tsquery with relevance ranking
- Type generation from live schema (`ayb types typescript`)
- Email backends: log, SMTP, webhook
- Password hashing: argon2id, bcrypt, firebase-scrypt with progressive re-hashing
- Two example apps: Live Polls, Kanban Board
