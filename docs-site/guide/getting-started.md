# Getting Started
<!-- audited 2026-03-20 -->

Get AYB running and make your first API call in a few minutes.

## What you get

AYB is a full PostgreSQL backend platform in one runtime:

Alongside the core [REST API](/guide/api-reference), [Authentication](/guide/authentication), [Realtime](/guide/realtime), [File Storage](/guide/file-storage), [GraphQL](/guide/graphql), [Edge Functions](/guide/edge-functions), and [Organizations](/guide/organizations) guides, notable built-in capabilities include:

- Row-level security (RLS) controls for API data access; see [Authentication](/guide/authentication)
- Database branching workflows for isolated schema/data changes; see [Branching](/guide/branching)
- Backups and point-in-time recovery (PITR) operations; see [Backups](/guide/backups)
- Vector index tooling plus semantic/hybrid search APIs; see [AI and Vector Search](/guide/ai-vector)
- SAML/SSO administration for enterprise auth; see [SAML SSO](/guide/saml)
- CLI workflow for operations and development; see [CLI Reference](/guide/cli)
- Migration tooling; see [Migrations](/guide/migrations)
- Audit logging for admin and operational actions; see [Security](/guide/security)

## Install

### curl (macOS / Linux)

```bash
curl -fsSLo /tmp/ayb-install.sh https://staging.allyourbase.io/install.sh
sh /tmp/ayb-install.sh
```

### Homebrew

```bash
brew install gridlhq/tap/ayb
```

### Binary download

Download the latest release from [GitHub Releases](https://github.com/gridlhq-staging/allyourbase/releases) for your OS and architecture.

### Docker

```bash
docker run --rm -p 8090:8090 ghcr.io/gridlhq-staging/allyourbase
```

## Start the server

### Managed PostgreSQL (zero config)

```bash
ayb start
```

`ayb start` runs the server in detached mode by default, then prints the startup banner and returns you to the shell.

The first run may take longer because AYB downloads and prepares a managed PostgreSQL binary.

Managed PostgreSQL is the zero-config path. If you need extension-backed features such as `pgvector`, `pg_cron`, or PostGIS, use an external PostgreSQL instance unless your managed PostgreSQL build explicitly includes those extensions.

If `admin.password` is not set, startup generates a random admin password and prints:

```text
Admin password: a1b2c3d4e5f6...
To reset: ayb admin reset-password
```

Default URLs from the startup banner:

- API: `http://127.0.0.1:8090/api`
- Admin: `http://127.0.0.1:8090/admin`

### External PostgreSQL

```bash
ayb start --database-url postgresql://user:pass@localhost:5432/mydb
```

### Verify readiness

```bash
curl http://127.0.0.1:8090/health
```

Typical healthy response:

```json
{"status":"ok","database":"ok"}
```

## Create a table

Create a `posts` table in your PostgreSQL database:

```sql
CREATE TABLE posts (
  id SERIAL PRIMARY KEY,
  title TEXT NOT NULL,
  body TEXT,
  published BOOLEAN DEFAULT false,
  created_at TIMESTAMPTZ DEFAULT now()
);
```

You can run this via the built-in SQL command:

```bash
ayb sql "CREATE TABLE posts (
  id SERIAL PRIMARY KEY,
  title TEXT NOT NULL,
  body TEXT,
  published BOOLEAN DEFAULT false,
  created_at TIMESTAMPTZ DEFAULT now()
)"
```

Or use the admin dashboard SQL editor at `http://127.0.0.1:8090/admin`.

## Make your first API call

### List records

```bash
curl http://127.0.0.1:8090/api/collections/posts
```

**Response:**

```json
{
  "items": [],
  "page": 1,
  "perPage": 20,
  "totalItems": 0,
  "totalPages": 0
}
```

The table is empty, so `items` is an empty array (never `null`).

### Create a record

```bash
curl -X POST http://127.0.0.1:8090/api/collections/posts \
  -H "Content-Type: application/json" \
  -d '{"title": "Hello World", "body": "My first post", "published": true}'
```

**Response** (201 Created):

```json
{
  "id": 1,
  "title": "Hello World",
  "body": "My first post",
  "published": true,
  "created_at": "2026-02-17T12:00:00Z"
}
```

The full row is returned, including server-generated fields like `id` and `created_at`.

### Filter and sort

```bash
curl "http://127.0.0.1:8090/api/collections/posts?filter=title='Hello World'&sort=-created_at"
```

**Response:**

```json
{
  "items": [
    { "id": 1, "title": "Hello World", "body": "My first post", "published": true, "created_at": "2026-02-17T12:00:00Z" }
  ],
  "page": 1,
  "perPage": 20,
  "totalItems": 1,
  "totalPages": 1
}
```

::: tip Boolean filters
Boolean filters like `filter=published=true` work but the double `=` can look confusing in a URL. For string columns, wrap values in single quotes: `filter=title='Hello World'`.
:::

### Get a single record

```bash
curl http://127.0.0.1:8090/api/collections/posts/1
```

**Response:**

```json
{
  "id": 1,
  "title": "Hello World",
  "body": "My first post",
  "published": true,
  "created_at": "2026-02-17T12:00:00Z"
}
```

## Use the JavaScript SDK

```bash
npm install @allyourbase/js
```

```ts
import { AYBClient } from "@allyourbase/js";

const ayb = new AYBClient("http://127.0.0.1:8090");

// Create
await ayb.records.create("posts", { title: "Hello", published: true });

// List
const { items } = await ayb.records.list("posts", {
  filter: "published=true",
  sort: "-created_at",
});
console.log(items);
```

::: tip Windows
AYB runs on Windows via WSL2. Install WSL (`wsl --install`) then follow the Linux instructions above.
:::

## Next steps

- [Authentication](/guide/authentication) — Add user auth, then protect data with RLS
- [JavaScript SDK](/guide/javascript-sdk) — Build your frontend with the TypeScript SDK
- [Flutter SDK](/guide/flutter-sdk) — Build mobile/web apps with the Dart SDK
- [PostGIS](/guide/postgis) — Add geospatial support with GeoJSON columns
- [Deployment](/guide/deployment) — Deploy to production with Docker or bare metal
- [REST API Reference](/guide/api-reference) — Full endpoint documentation
- [Quickstart: Todo App](/guide/quickstart) — Build a full CRUD app in 5 minutes
- [Comparison](/guide/comparison) — How AYB compares to PocketBase and Supabase
- [Configuration](/guide/configuration) — Customize AYB with `ayb.toml`
