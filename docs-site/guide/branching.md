<!-- audited 2026-03-20 -->
# Database Branching

This guide covers database branching in AYB — creating isolated copies of your database for development, schema testing, and data experimentation. Based on code in `internal/branching/`, `internal/cli/branch.go`, and `internal/server/branch_admin_handler.go`.

## Use cases

- **Feature development**: Create an isolated branch to develop and test schema changes without affecting the production database.
- **Schema testing**: Validate migrations against a real copy of your data before applying them in production.
- **Data experimentation**: Run exploratory queries, test data transformations, or prototype new features on a throwaway copy.

## How branching works

AYB branches use `pg_dump | psql` to clone the source database schema and data into a new PostgreSQL database. Each branch is a fully independent database that you can connect to, modify, and discard.

## Branch lifecycle

```
create → creating → ready (or failed)
delete → (connections terminated, database dropped, metadata removed)
```

| Status | Description |
|--------|-------------|
| `creating` | Clone in progress (`pg_dump \| psql`) |
| `ready` | Branch is available for connections |
| `failed` | Clone encountered an error (check `error_message`) |

Deletion is synchronous: AYB terminates all active connections to the branch database, drops the database, and removes the metadata record in a single operation. There is no intermediate `deleting` status.

## Naming rules

From `internal/branching/naming.go`:

- **Length**: 1–63 characters (PostgreSQL identifier limit).
- **Pattern**: `^[a-z0-9][a-z0-9_-]*[a-z0-9]$` — lowercase alphanumeric, hyphens and underscores allowed in the middle. Single-character names must be alphanumeric.
- **Reserved names** (cannot be used): `main`, `master`, `default`, `postgres`, `template0`, `template1`.
- **Database naming**: Branch name is prefixed with `ayb_branch_` and hyphens are replaced with underscores. For example, branch `feature-auth` creates database `ayb_branch_feature_auth`.

## CLI commands

### `ayb branch create`

```bash
ayb branch create <name> [--from <source-url>] [--database-url <url>] [--config <path>]
```

Creates a new branch by cloning the source database.

| Flag | Description |
|------|-------------|
| `--from` | Source database URL (overrides config) |
| `--database-url` | Database URL override |
| `--config` | Path to `ayb.toml` |

### `ayb branch list`

```bash
ayb branch list [--database-url <url>] [--config <path>] [--output json]
```

Lists all branches with their status.

| Flag | Default | Description |
|------|---------|-------------|
| `--database-url` | — | Database URL override |
| `--config` | — | Path to `ayb.toml` |
| `--output` | `table` | Output format: `table` or `json` |

### `ayb branch delete`

```bash
ayb branch delete <name> [--database-url <url>] [--config <path>] [--yes] [--force]
```

Deletes a branch. Terminates all connections and drops the database.

| Flag | Description |
|------|-------------|
| `--yes`, `-y` | Skip confirmation prompt |
| `--force` | Force delete (for failed or orphaned branches) |
| `--database-url` | Database URL override |
| `--config` | Path to `ayb.toml` |

### `ayb branch diff`

```bash
ayb branch diff <branchA> <branchB> [--database-url <url>] [--config <path>] [--output <format>]
```

Shows schema differences between two branches (or between a branch and the source database).

| Flag | Default | Description |
|------|---------|-------------|
| `--output` | `table` | Output format: `table`, `json`, or `sql` |
| `--database-url` | — | Database URL override |
| `--config` | — | Path to `ayb.toml` |

### `ayb start --branch`

Start AYB using a database branch for local development:

```bash
ayb start --branch feature-auth
```

This connects AYB to the branch database (`ayb_branch_feature_auth`) instead of the default database, letting you develop against an isolated copy.

## API endpoints

All branch endpoints require admin authentication and are mounted under `/api`.

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/admin/branches` | List all branches |
| `POST` | `/api/admin/branches` | Create a new branch |
| `DELETE` | `/api/admin/branches/{name}` | Delete a branch |

### Request bodies

**Create branch** (`POST /api/admin/branches`):
```json
{
  "name": "feature-auth",
  "from": "postgres://user:pass@localhost:5432/mydb"
}
```

### Response examples

**Create** (201 Created):
```json
{
  "id": "br_abc123",
  "name": "feature-auth",
  "source_database": "mydb",
  "branch_database": "ayb_branch_feature_auth",
  "status": "ready",
  "created_at": "2026-03-15T10:30:00Z",
  "updated_at": "2026-03-15T10:30:00Z"
}
```

**List** (200 OK):
```json
{
  "branches": [
    {
      "id": "br_abc123",
      "name": "feature-auth",
      "source_database": "mydb",
      "branch_database": "ayb_branch_feature_auth",
      "status": "ready",
      "created_at": "2026-03-15T10:30:00Z",
      "updated_at": "2026-03-15T10:30:01Z"
    }
  ]
}
```

**Delete** (200 OK):
```json
{
  "status": "deleted"
}
```

### Response codes

| Code | Meaning |
|------|---------|
| `200` | Success |
| `201` | Branch created |
| `400` | Invalid branch name |
| `404` | Branch not found |
| `409` | Branch name already exists |
| `503` | Branch service not configured |

## Admin dashboard

The **Branches** view in the admin dashboard (`branches` view ID) provides a UI for managing branch lifecycle — creating, listing, and deleting branches. See [Admin Dashboard](/guide/admin-dashboard#database).

## Related guides

- [Admin Dashboard — Database section](/guide/admin-dashboard#database) for the branches admin view
- [Security](/guide/security) for API key authentication on admin endpoints
- [Deployment](/guide/deployment) for database branching in production environments
