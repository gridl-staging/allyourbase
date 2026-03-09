# Comparison

How Allyourbase compares to PocketBase and Supabase (self-hosted).

## Feature matrix

| Feature | PocketBase | Supabase (self-hosted) | Allyourbase |
|---------|-----------|----------------------|-------------|
| **Database** | SQLite | PostgreSQL | PostgreSQL |
| **Deployment** | Single binary | 10+ Docker containers | Single binary |
| **Configuration** | One file | Dozens of env vars | One file (`ayb.toml`) |
| **Docker required** | No | Yes | No |
| **Auto-generated API** | Yes | Yes | Yes |
| **Full-text search** | No | Yes (PostgREST) | Yes (`?search=`) |
| **Filtering** | Custom syntax | PostgREST | SQL-like, parameterized |
| **Sorting** | Yes | Yes | Yes |
| **Pagination** | Yes | Yes | Yes |
| **FK expansion** | Yes | Select embedding | Yes (`?expand=`) |
| **Row-Level Security** | No | Yes | Yes |
| **Auth** | Yes | Yes | Yes |
| **OAuth** | Many providers | Many providers | Google, GitHub |
| **File storage** | Yes | Yes | Yes (local + S3-compatible) |
| **Realtime** | SSE | WebSocket | SSE |
| **Admin dashboard** | Yes | Yes | Yes |
| **Database RPC** | No | Yes (PostgREST) | Yes |
| **Horizontal scaling** | No (SQLite) | Yes | Yes (PostgreSQL) |
| **Bulk operations** | No | Yes | Yes (batch endpoint) |
| **Maturity** | Stable (56K+ stars) | Stable (managed + self-hosted) | New (v0.1, early adopters) |
| **Startup time** | ~100ms | Minutes (Docker stack) | ~310ms (external PG) |
| **Memory (idle)** | ~15MB | 3-5GB (12 containers) | ~20MB |
| **Binary/install size** | ~40MB | 10+ Docker images | ~36MB |

::: info OAuth Provider Roadmap
Allyourbase currently supports Google and GitHub OAuth, which cover the majority of use cases. Additional providers (Apple, Discord, Microsoft, and others) are planned for future releases. The OAuth framework is extensible — [contributions welcome](https://github.com/gridlhq/allyourbase).
:::

## When to use Allyourbase

**PocketBase simplicity + PostgreSQL power.** Allyourbase gives you a single binary with everything included — REST API, auth, realtime, storage, admin dashboard — running on top of PostgreSQL. Deploy it on a $5 VPS with 20MB of RAM, or point it at RDS for production scale.

**Choose Allyourbase if you:**
- Want a single-command backend that runs anywhere — your laptop, a VPS, a Raspberry Pi, an air-gapped network
- Need PostgreSQL features (RLS, extensions, concurrent writes, horizontal scaling) without managing 10+ containers
- Are migrating from PocketBase and hit the SQLite ceiling
- Want your data in standard Postgres — no lock-in, take your database and go

## When to use PocketBase

**Choose PocketBase if you:**
- Want the simplest possible setup and SQLite is sufficient
- Don't need Row-Level Security
- Need many OAuth providers out of the box
- Want a mature, battle-tested product

## When to use Supabase

**Choose Supabase if you:**
- Want a managed cloud service (Supabase Cloud)
- Need the full ecosystem (edge functions, branching, SQL editor, API explorer)
- Need advanced PostgREST features (select embedding, computed columns)
- Want a large community and extensive documentation

## Key advantages of Allyourbase

### PostgreSQL without complexity

Supabase self-hosted requires 10+ containers (PostgREST, GoTrue, Realtime, Storage, Kong, etc.). AYB gives you the same PostgreSQL foundation in a single binary.

### Single binary + managed PostgreSQL

For development and prototyping, `ayb start` downloads and manages its own PostgreSQL — no database setup needed. For production, point it at any PostgreSQL instance.

### SQL-safe filtering

AYB's filter syntax is familiar and parameterized:

```
?filter=status='active' AND age>21
```

No custom DSL to learn. Values are parameterized to prevent SQL injection.
