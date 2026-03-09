# Getting Started

Get AYB running and make your first API call in under 5 minutes.

## Install

### curl (macOS / Linux)

```bash
curl -fsSL https://allyourbase.io/install.sh | sh
```

### Homebrew

```bash
brew install gridlhq/tap/ayb
```

### Go

```bash
go install github.com/allyourbase/ayb/cmd/ayb@latest
```

### Binary download

Download the latest release from [GitHub Releases](https://github.com/gridlhq-staging/allyourbase/releases) for your OS and architecture.

### Docker

```bash
docker run -p 8090:8090 ghcr.io/gridlhq-staging/allyourbase
```

## Start the server

### Embedded PostgreSQL (zero config)

```bash
ayb start
```

AYB will download and manage its own PostgreSQL instance automatically. No database setup needed.

::: info First run
The very first `ayb start` downloads a PostgreSQL binary (~70MB). This is a one-time download — subsequent starts take ~300ms.
:::

When you run `ayb start` for the first time, it will generate a random admin password and display it in the console:

```
Admin password: a1b2c3d4e5f6...
To reset: ayb admin reset-password
```

Save this password — you'll need it to access the admin dashboard at `http://localhost:8090/admin`. If you lose it, run `ayb admin reset-password` to generate a new one.

### External PostgreSQL

```bash
ayb start --database-url postgresql://user:pass@localhost:5432/mydb
```

The API is now live at `http://localhost:8090/api` and the admin dashboard at `http://localhost:8090/admin`.

### Verify the server is running

```bash
curl http://localhost:8090/health
```

You should see `{"status":"ok"}`.

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

Or use the admin dashboard SQL editor at `http://localhost:8090/admin`.

## Make your first API call

### List records

```bash
curl http://localhost:8090/api/collections/posts
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
curl -X POST http://localhost:8090/api/collections/posts \
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
curl "http://localhost:8090/api/collections/posts?filter=title='Hello World'&sort=-created_at"
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
curl http://localhost:8090/api/collections/posts/1
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

const ayb = new AYBClient("http://localhost:8090");

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
- [Deployment](/guide/deployment) — Deploy to production with Docker or bare metal
- [REST API Reference](/guide/api-reference) — Full endpoint documentation
- [Quickstart: Todo App](/guide/quickstart) — Build a full CRUD app in 5 minutes
- [Comparison](/guide/comparison) — How AYB compares to PocketBase and Supabase
- [Configuration](/guide/configuration) — Customize AYB with `ayb.toml`
