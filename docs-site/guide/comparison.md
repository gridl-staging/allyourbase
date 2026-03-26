<!-- audited 2026-03-20 -->

# Comparison

How Allyourbase compares to PocketBase and Supabase (self-hosted).

This matrix is intentionally conservative: AYB is marked `✅` only for features that are shipped in code today.

For measured binary size, startup time, memory usage, and API benchmark methodology, see [Performance](/guide/performance).

## Feature matrix

| Feature | PocketBase | Supabase (self-hosted) | Allyourbase |
| --- | --- | --- | --- |
| **Database** | SQLite | PostgreSQL | PostgreSQL |
| **Deployment model** | Single binary | Multi-container stack | Single binary |
| **Docker required** | No | Yes (typical self-host setup) | No |
| **Configuration surface** | Small config | Many services/env vars | One file (`ayb.toml`) |
| **Admin dashboard surface** | Core admin UI | Full admin UI | **Comprehensive built-in admin views + dynamic table browser** |
| **OAuth providers (built-in)** | Limited set | Configurable via GoTrue/providers | Google, GitHub, Microsoft, Apple, Discord, Twitter, Facebook, LinkedIn, Spotify, Twitch, GitLab, Bitbucket, Slack, Zoom, Figma, Notion |
| **Row-level security (RLS)** | No | Yes | Yes |
| **SAML / SSO** | No | Available with self-host setup constraints | ✅ ([Guide](/guide/saml)) |
| **Read replicas** | No | Manual PostgreSQL operations | ✅ |
| **Database branching** | No | Not built-in for self-host | ✅ |
| **Backups + PITR** | External tooling | PostgreSQL/infra dependent | ✅ |
| **AI assistant workflows** | No | No built-in assistant surface | ✅ |
| **Vector indexes / vector search** | No | Yes (`pgvector`) | ✅ ([Guide](/guide/ai-vector)) |
| **Custom domains** | Reverse proxy/manual | Reverse proxy/manual | ✅ ([Guide](/guide/custom-domains)) |
| **Log drains** | External tooling | External tooling | ✅ ([Guide](/guide/log-drains)) |
| **Audit logging** | Basic logs | Partial | ✅ |
| **MCP server** | No | No | ✅ |
| **Migration tools (source importers)** | No built-in import suite | SQL migration workflow | ✅ (Built-in importers for PocketBase, Supabase, Firebase, Directus, Appwrite, and Nhost) |
| **PostGIS spatial** | No | Yes | ✅ |
| **Push notifications** | No | External integration | ✅ |
| **SMS operations** | No | Auth OTP focused | ✅ |
| **Email templates** | No | Auth templates | ✅ |
| **Edge functions** | No | Yes | ✅ |
| **Materialized views tooling** | No | PostgreSQL-native/manual | ✅ |

## Migration tools

AYB ships built-in migration/import flows for these source platforms:

- PocketBase
- Supabase
- Firebase
- Directus
- Appwrite
- Nhost

## When to use Allyourbase

Choose AYB when you want a PostgreSQL backend platform that runs as a single binary while still shipping advanced admin capabilities (RLS, branching, replicas, backups/PITR, AI/vector tooling, SAML, audit logs, and operational controls).

## When to use PocketBase

Choose PocketBase when SQLite is sufficient and you want the smallest operational footprint with minimal moving parts.

## When to use Supabase (self-hosted)

Choose Supabase self-hosted when you specifically want the Supabase ecosystem and are comfortable operating a multi-service container stack.
