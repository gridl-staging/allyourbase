---
layout: home

hero:
  name: "👾 Allyourbase"
  text: PostgreSQL backend in one binary
  tagline: "Auto-generated REST API. Auth. Realtime. File storage. Admin dashboard. Open source."
  actions:
    - theme: brand
      text: Get Started
      link: /guide/getting-started
    - theme: alt
      text: View on GitHub
      link: https://github.com/gridlhq-staging/allyourbase

features:
  - icon: "\u26A1"
    title: Auto-generated REST API
    details: CRUD endpoints for every PostgreSQL table. Full-text search, filtering, sorting, pagination, and FK expansion. No code generation, no ORM.
    link: /guide/api-reference
    linkText: API docs
  - icon: "\uD83D\uDD12"
    title: Built-in Auth & RLS
    details: Email/password, JWT sessions, OAuth (Google, GitHub). Row-Level Security lets you write access rules in SQL.
    link: /guide/authentication
    linkText: Auth docs
  - icon: "\uD83D\uDCE1"
    title: Realtime
    details: Server-Sent Events filtered by table subscriptions and RLS policies. Subscribe from the browser with EventSource.
    link: /guide/realtime
    linkText: Realtime docs
  - icon: "\uD83D\uDCC1"
    title: File Storage
    details: Upload and serve files from local disk or any S3-compatible object store — Cloudflare R2, MinIO, DigitalOcean Spaces, AWS S3, and more. Signed URLs for secure access.
    link: /guide/file-storage
    linkText: Storage docs
  - icon: "\uD83D\uDDA5\uFE0F"
    title: Admin Dashboard
    details: Browse tables, create and edit records, run SQL, and inspect your schema from a built-in web UI.
    link: /guide/admin-dashboard
    linkText: Dashboard docs
  - icon: "\uD83D\uDC18"
    title: Embedded PostgreSQL
    details: "Run <code>ayb start</code> and AYB downloads and manages its own PostgreSQL. Zero config for development."
    link: /guide/getting-started
    linkText: Get started
---

<div class="custom-sections">

## Install

<div class="install-grid">
<div class="install-option">

**curl (macOS / Linux)**

```bash
curl -fsSL https://allyourbase.io/install.sh | sh
```

</div>
<div class="install-option">

**Homebrew**

```bash
brew install gridlhq/tap/ayb
```

</div>
<div class="install-option">

**Docker**

```bash
docker run -p 8090:8090 ghcr.io/gridlhq-staging/allyourbase
```

</div>
<div class="install-option">

**Go**

```bash
go install github.com/allyourbase/ayb/cmd/ayb@latest
```

</div>
</div>

## Quickstart

<div class="quickstart-steps">

**1. Start AYB**

```bash
ayb start
```

AYB starts with managed PostgreSQL. No database setup needed.

**2. Create a table**

Use the admin dashboard at `http://localhost:8090/admin`, `psql`, or any PostgreSQL client:

```sql
CREATE TABLE posts (
  id SERIAL PRIMARY KEY,
  title TEXT NOT NULL,
  body TEXT,
  published BOOLEAN DEFAULT false,
  created_at TIMESTAMPTZ DEFAULT now()
);
```

**3. Use the API**

```bash
# Create a record
curl -X POST http://localhost:8090/api/collections/posts \
  -H "Content-Type: application/json" \
  -d '{"title": "Hello World", "published": true}'

# List records
curl "http://localhost:8090/api/collections/posts?sort=-created_at&perPage=10"
```

```json
{
  "items": [
    { "id": 1, "title": "Hello World", "published": true, "created_at": "2026-02-17T12:00:00Z" }
  ],
  "page": 1, "perPage": 10, "totalItems": 1, "totalPages": 1
}
```

</div>

## JavaScript SDK

```bash
npm install @allyourbase/js
```

```ts
import { AYBClient } from "@allyourbase/js";

const ayb = new AYBClient("http://localhost:8090");

// Auth
await ayb.auth.register("user@example.com", "password");

// Create
await ayb.records.create("posts", {
  title: "Hello World",
  published: true,
});

// List with filtering and sorting
const { items } = await ayb.records.list("posts", {
  filter: "title='Hello World'",
  sort: "-created_at",
});

// Realtime
ayb.realtime.subscribe(["posts"], (event) => {
  console.log(event.action, event.record);
});
```

## Why PostgreSQL?

Most single-binary backends use SQLite. AYB uses PostgreSQL instead, giving you:

- **Row-Level Security** — access control rules written in SQL, enforced by the database itself
- **Horizontal scaling** — connect to any PostgreSQL instance, including managed services (RDS, Cloud SQL, Neon)
- **Concurrent writes** — no single-writer bottleneck
- **The full PostgreSQL ecosystem** — extensions, tooling, monitoring, backups, replication

AYB keeps the single-binary simplicity. For development, `ayb start` runs its own managed PostgreSQL with zero config. For production, point it at any PostgreSQL instance.

<div class="why-table">

| | PocketBase | Supabase (self-hosted) | Allyourbase |
|---|---|---|---|
| **Database** | SQLite | PostgreSQL | PostgreSQL |
| **Deployment** | Single binary | 10+ Docker containers | Single binary |
| **Configuration** | One file | Dozens of env vars | One file |
| **Full-text search** | No | Yes | Yes |
| **Row-Level Security** | No | Yes | Yes |
| **Docker required** | No | Yes | No |

</div>

[Full comparison &rarr;](/guide/comparison)

## License

Allyourbase is open source under the [MIT License](https://github.com/gridlhq-staging/allyourbase/blob/main/LICENSE).

</div>
