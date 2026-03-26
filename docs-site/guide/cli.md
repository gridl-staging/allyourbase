# CLI Reference
<!-- audited 2026-03-20 -->

The `ayb` CLI manages server lifecycle, schema/data operations, deployment, and admin tooling.

## Command structure

`ayb <command> [subcommand] [flags]`

## Global flags

Only these are global (root persistent flags):

| Flag | Default | Description |
|---|---|---|
| `--json` | `false` | Shorthand for `--output json` |
| `--output table|json|csv` | `table` | Output format |

`--config` is **not** a global flag. It is defined only on commands that explicitly add it (for example `ayb start`, `ayb config`, `ayb admin create`).

## Core lifecycle

### `ayb start`

```bash
ayb start
ayb start --database-url postgresql://user:pass@localhost:5432/mydb
ayb start --from ./pb_data
ayb start --from postgres://db.xxx.supabase.co:5432/postgres
ayb start --branch feature-auth
```

Key local flags: `--database-url`, `--port`, `--host`, `--config`, `--from`, `--domain`, `--branch`.

### `ayb stop`

```bash
ayb stop
ayb stop --json
```

### `ayb status`

```bash
ayb status
ayb status --port 8090
ayb status --json
```

`ayb status` reports running state, PID, port, and health probe result.

## Configuration

### `ayb config`

```bash
ayb config
ayb config --config ./ayb.toml
ayb config get server.port --config ./ayb.toml
ayb config set server.port 9090 --config ./ayb.toml
```

Subcommands:

- `get <key>`
- `set <key> <value>`

## Data and schema

### `ayb sql [query]`

```bash
ayb sql "SELECT now()"
ayb sql "SELECT * FROM users LIMIT 10" --json
echo "SELECT 1" | ayb sql
```

Authentication resolution for SQL/admin API calls:

1. `--admin-token`
2. `AYB_ADMIN_TOKEN`
3. `~/.ayb/admin-token` (saved by `ayb start`)

### `ayb query <table>`

```bash
ayb query posts
ayb query posts --filter "published=true" --sort -created_at --limit 10
```

### `ayb rpc <function>`

```bash
ayb rpc increment_counter --arg count=5
ayb rpc search_products --arg query=laptop --arg limit=10 --json
```

### `ayb schema [table]`

```bash
ayb schema
ayb schema users --json
```

### `ayb types`

```bash
ayb types typescript --database-url "$DATABASE_URL" -o src/types/ayb.d.ts
ayb types openapi --database-url "$DATABASE_URL" -o openapi.json
```

## Admin and security

```bash
ayb admin create --email admin@example.com --password strongpass --database-url "$DATABASE_URL"
ayb admin reset-password

ayb apikeys create --user-id USER_ID --name backend --scope readwrite
ayb users list --search "@example.com"
ayb webhooks list
ayb secrets list
ayb secrets rotate --config ./ayb.toml
```

## Additional top-level command groups

Current top-level groups include:

- `migrate`, `db`, `branch`, `storage`, `sites`, `functions`
- `deploy` (`fly`, `digitalocean`, `railway`)
- `apps`, `org`, `tenants`, `prompts`, `jobs`, `schedules`, `push`
- `audit`, `email-templates`, `extensions`, `oauth`
- `mcp`, `init`, `uninstall`, `version`, `demo`

Use command help for exact flags and examples:

```bash
ayb --help
ayb <command> --help
ayb <command> <subcommand> --help
```
