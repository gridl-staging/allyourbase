# 👾 Allyourbase ![Beta](https://img.shields.io/badge/status-beta-orange)

[![CI](https://github.com/gridlhq-staging/allyourbase/actions/workflows/ci.yml/badge.svg)](https://github.com/gridlhq-staging/allyourbase/actions/workflows/ci.yml)
[![Release](https://github.com/gridlhq-staging/allyourbase/actions/workflows/release.yml/badge.svg)](https://github.com/gridlhq-staging/allyourbase/releases)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

Open-source backend for PostgreSQL. Single binary. Auto-generated REST API, auth, realtime, storage, admin dashboard.

![AYB Local API Demo](assets/readme/demo.gif)

## Quickstart

Download the installer, then launch a demo app:

```bash
curl -fsSLo /tmp/ayb-install.sh https://staging.allyourbase.io/install.sh
sh /tmp/ayb-install.sh
~/.ayb/bin/ayb start
~/.ayb/bin/ayb demo live-polls
```

Open http://localhost:5175 — you've got a real-time polling app with auth, RLS, SSE, and a REST API. No Docker. No config.

The admin dashboard is at http://localhost:8090/admin — SQL editor, API explorer, schema browser, and user management for core workflows.

On first run, AYB downloads a prebuilt PostgreSQL binary for your platform and manages it as a child process — no system install required.

Managed PostgreSQL is the zero-config path. If you need extensions beyond the managed build's default set, such as PostGIS, use an external PostgreSQL instance unless your managed PostgreSQL build explicitly includes them.

Two demos ship in [`/examples`](examples/):

- **[Live Polls](examples/live-polls/)** — real-time polling with auth, RLS, SSE, and database RPC
- **[Kanban Board](examples/kanban/)** — Trello-lite with drag-and-drop, per-user boards via RLS

## Who is this for?

- **Indie devs and small teams** who want a full backend without managing infrastructure. One binary, one command, done.
- **AI-first projects** building with Claude Code, Cursor, or Windsurf. The built-in MCP server gives AI tools direct access to your backend.
- **PocketBase graduates** who hit the SQLite ceiling and need Postgres — concurrent writes, RLS, extensions, horizontal scaling — without rewriting everything.

## Features

- **REST API** — CRUD for every table. Filter, sort, paginate, full-text search, FK expand.
- **Auth** — email/password, JWT, OAuth (Google, GitHub, Microsoft, Apple, and more built-in providers), email verify, password reset
- **Realtime** — SSE subscriptions per table, filtered by RLS
- **Row-Level Security** — JWT claims mapped to Postgres session vars. Write policies in SQL.
- **Storage** — local disk or S3-compatible (R2, MinIO, DO Spaces, AWS)
- **Admin dashboard** — SQL editor, API explorer, schema browser, RLS manager, user management
- **RPC** — call Postgres functions via `POST /api/rpc/{function}`
- **Type generation** — `ayb types typescript` emits types from your schema
- **Embedded Postgres** — zero external dependencies for development
- **MCP server** — `ayb mcp` gives AI tools (Claude Code, Cursor, Windsurf) direct access to your schema, records, SQL, and RLS policies. See the [MCP Server guide](https://allyourbase.io/guide/mcp) for current tools, resources, and prompts.
13 tools, 2 resources, 3 prompts.

Your data lives in standard PostgreSQL. No lock-in — take your database and go.

## Working with the API

Create a table:

```bash
ayb sql "CREATE TABLE posts (
  id serial PRIMARY KEY,
  title text NOT NULL,
  body text,
  created_at timestamptz DEFAULT now()
)"
```

Every table gets a full REST API automatically. For local development, AYB starts with auth disabled by default, so the API is open on `localhost`:

```bash
# Create
curl -X POST http://localhost:8090/api/collections/posts \
  -H "Content-Type: application/json" \
  -d '{"title": "Hello world", "body": "First post"}'

# List (with sort, pagination)
curl 'http://localhost:8090/api/collections/posts?sort=-created_at&perPage=10'
```

Before exposing AYB beyond `localhost`, enable auth (`auth.enabled = true`) and rely on JWTs plus RLS policies for the routes you publish.

With auth enabled (`auth.enabled = true` in `ayb.toml`), include a JWT:

```bash
# Get a token
TOKEN=$(curl -s -X POST http://localhost:8090/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"you@example.com","password":"yourpassword"}' | jq -r .token)

# Use it
curl -X POST http://localhost:8090/api/collections/posts \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"title": "Hello world", "body": "First post"}'
```

```bash
# Admin dashboard
open http://localhost:8090/admin
```

Every table gets CRUD, filtering, sorting, pagination, full-text search, and FK expansion.

## SDK

```bash
npm install @allyourbase/js
```

```typescript
import { AYBClient } from "@allyourbase/js";
const ayb = new AYBClient("http://localhost:8090");

// Records
const { items } = await ayb.records.list("posts", {
  filter: "published=true",
  sort: "-created_at",
  expand: "author",
});
await ayb.records.create("posts", { title: "New post" });

// Auth
await ayb.auth.login("user@example.com", "password");

// Realtime
ayb.realtime.subscribe(["posts"], (e) => {
  console.log(e.action, e.record);
});
```

## Existing database

Point at any Postgres instance. Existing tables become API endpoints on startup.

```bash
ayb start --database-url postgresql://user:pass@localhost:5432/mydb
```

## Config

Zero config by default. Customize via `ayb.toml`, env vars (`AYB_` prefix), or CLI flags.

```toml
[server]
port = 8090

[database]
url = "postgresql://user:pass@localhost:5432/mydb"

[auth]
enabled = true

[storage]
backend = "s3"
```

Precedence: defaults → `ayb.toml` → env vars → CLI flags. Check resolved config: `ayb config`.

## CLI

```
ayb start                Start server (embedded or external Postgres)
ayb sql "..."            Execute SQL
ayb schema [table]       Inspect database schema
ayb migrate up           Apply pending migrations
ayb migrate create       Create a new migration
ayb admin reset-password Reset admin password
ayb apikeys create       Create an API key
ayb types typescript     Generate TypeScript types
ayb mcp                  Start MCP server for AI tools
```

Run `ayb --help` or `ayb <command> --help` for the full command list.
32 commands total.

## Migrate from PocketBase or Supabase

Current support (live-validated): PocketBase, Supabase Cloud, and self-hosted Supabase.

Fastest path (single CLI command into managed AYB Postgres):

```bash
# PocketBase (source is pb_data directory)
ayb start --from ./pb_data

# Supabase (source is direct Postgres URL; use port 5432, not pooler 6543)
ayb start --from "postgresql://postgres:<password>@db.<ref>.supabase.co:5432/postgres"
```

If you want explicit control over target DB and options, use standalone commands:

```bash
# PocketBase -> specific target DB
ayb migrate pocketbase \
  --source ./pb_data \
  --database-url "postgresql://user:pass@host:5432/mydb" \
  -y

# Supabase -> specific target DB
ayb migrate supabase \
  --source-url "postgresql://postgres:<password>@db.<ref>.supabase.co:5432/postgres" \
  --database-url "postgresql://user:pass@host:5432/mydb" \
  -y
```

Supabase storage files: include `--storage-export <dir>` only if you have an exported storage directory to migrate.

Local-dev caveat (does not affect customer cloud/self-hosted migrations): on macOS + Colima, `supabase start` may fail on a Docker socket mount for Logflare/Vector. Workaround: `supabase start -x logflare,vector`.

## Install (Staging)

```bash
# Install script (recommended)
curl -fsSLo /tmp/ayb-install.sh https://staging.allyourbase.io/install.sh
sh /tmp/ayb-install.sh

# From source
git clone https://github.com/gridlhq-staging/allyourbase.git && cd allyourbase && make build

# Specific version
curl -fsSLo /tmp/ayb-install.sh https://staging.allyourbase.io/install.sh
sh /tmp/ayb-install.sh v0.0.6-beta
```

## vs. PocketBase vs. Supabase

| | PocketBase | Supabase (self-hosted) | Allyourbase |
|---|---|---|---|
| Database | SQLite | PostgreSQL | PostgreSQL |
| Deployment | Single binary | Multi-container stack | Single binary |
| Config | One file | Dozens of env vars | One file (or none) |
| Row-level security | No | Yes | Yes |
| Docker required | No | Yes | No |
| AI/MCP server | No | No | Yes |

[Full comparison →](https://allyourbase.io/guide/comparison)

## Roadmap

**→ [Full Project Roadmap](ROADMAP.md)** — feature status and planned work; see CHANGELOG.md for release history.

## License

[MIT](LICENSE)
