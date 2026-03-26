# MCP Server
<!-- audited 2026-03-21 -->

AYB exposes a Model Context Protocol (MCP) server so AI assistants can safely interact with your database through structured tools, resources, and prompts.

With MCP, an assistant can:

- Inspect schema and functions
- Query and mutate records
- Call RPC functions
- Run SQL (admin token required)
- Use spatial tools for PostGIS-aware queries

## Start the server

```bash
ayb mcp
```

Optional flags:

- `--url`: AYB server URL
- `--admin-token`: privileged token (or `AYB_ADMIN_TOKEN`)
- `--token`: user JWT for RLS-filtered access (or `AYB_TOKEN`)

## Claude Code setup

Register AYB as a local stdio MCP server from Claude Code:

```bash
claude mcp add ayb -- ayb mcp --url http://127.0.0.1:8090 --token "$AYB_TOKEN"
```

Use `--admin-token` instead of `--token` only when you need privileged tools like `run_sql`.

## Claude Desktop setup

`ayb mcp` also works with Claude Desktop over stdio.

Example `claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "ayb": {
      "command": "ayb",
      "args": ["mcp", "--url", "http://127.0.0.1:8090", "--token", "YOUR_USER_JWT"]
    }
  }
}
```

Keep your default Desktop config on `--token` for least-privilege, RLS-scoped access.
Create a separate, temporary `--admin-token` profile only when you explicitly need
privileged tools like `run_sql`.

## Cursor setup

In Cursor MCP settings, add a stdio MCP server:

- Command: `ayb`
- Args: `mcp --url http://127.0.0.1:8090 --token YOUR_USER_JWT`

Switch to `--admin-token` only when SQL execution is required.

## Generic MCP-compatible clients

Any MCP client that supports stdio transport can launch:

```bash
ayb mcp --url http://127.0.0.1:8090 --token "$AYB_TOKEN"
```

## Tools (13 total)

### Data & schema tools (10)

| Tool | Purpose |
|---|---|
| `list_tables` | List database tables and schema metadata |
| `describe_table` | Describe columns, PKs, FKs, indexes for one table |
| `list_functions` | List callable RPC/Postgres functions |
| `query_records` | Filter/sort/paginate table records |
| `get_record` | Fetch one record by ID |
| `create_record` | Insert one record |
| `update_record` | Partially update one record |
| `delete_record` | Delete one record |
| `run_sql` | Execute SQL (requires admin token) |
| `call_function` | Call RPC/Postgres function with named args |

### Admin tool (1)

| Tool | Purpose |
|---|---|
| `get_status` | Fetch AYB health/status and admin metadata |

### Spatial tools (2)

| Tool | Purpose |
|---|---|
| `spatial_info` | Show PostGIS and spatial column/index info |
| `spatial_query` | Query with `near`, `within`, `intersects`, `bbox` filters |

## Resources

- `ayb://schema`: complete schema cache
- `ayb://health`: server health payload

## Prompts

- `explore-table`: guides schema + sample-row exploration for one table
- `write-migration`: guides safe SQL migration drafting
- `generate-types`: guides TypeScript type generation from schema

## Security model

- User token (`--token` / `AYB_TOKEN`): RLS-filtered data access; preferred default.
- Admin token (`--admin-token` / `AYB_ADMIN_TOKEN`): privileged access for tools that require admin APIs, especially `run_sql`.

Recommended practice:

1. Default to user token for normal assistant usage.
2. Use admin token only for tasks that require privileged APIs (especially `run_sql`).

## Related docs

- [CLI Reference](/guide/cli)
- [Security](/guide/security)
- [Database RPC](/guide/database-rpc)
- [PostGIS](/guide/postgis)
