<!-- audited 2026-03-20 -->
# Read Replicas

This guide covers read replica configuration, lifecycle management, query routing, and health monitoring in AYB, based on code in `internal/replica/`, `internal/cli/db_replicas.go`, and `internal/server/admin_replicas.go`.

## Configuration

Replicas are configured in the `[database.replicas]` TOML array in `ayb.toml`. Each entry defines a read replica connection.

```toml
[[database.replicas]]
url = "postgres://user:pass@replica1:5432/mydb"
weight = 2
max_lag_bytes = 10485760

[[database.replicas]]
url = "postgres://user:pass@replica2:5432/mydb"
weight = 1
max_lag_bytes = 5242880
```

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `url` | string | — | PostgreSQL connection URL (required) |
| `weight` | int | `1` | Load-balancing weight for weighted round-robin |
| `max_lag_bytes` | int64 | `10485760` (10 MB) | Maximum acceptable replication lag in bytes |

## Replica lifecycle

### Topology states

Each replica node tracks a topology state:

| State | Description |
|-------|-------------|
| `active` | Node is actively receiving traffic |
| `draining` | Transitional state during removal — connections drain before shutdown |
| `removed` | Node fully removed from the topology |

### Adding a replica

```bash
ayb db replicas add --name replica-2 --host replica2.example.com --port 5432 \
  --database mydb --weight 2 --max-lag-bytes 10485760
```

AYB validates connectivity to the replica before adding it to the topology. If the connection check fails, the add operation returns a `502 Bad Gateway` error.

### Removing a replica

```bash
ayb db replicas remove replica-2
```

Safety constraint: removing the last active replica requires `--force`. Without it, AYB refuses the operation to prevent accidentally losing all read capacity.

```bash
ayb db replicas remove replica-2 --force
```

### Promoting a replica

```bash
ayb db replicas promote replica-2
```

Promotes a replica to primary. The promotion workflow:
1. Connects to the target replica.
2. Waits for WAL replay to complete (timeout: 30 seconds).
3. Promotes via `pg_promote()`.
4. Polls until the node reports as primary.
5. Swaps the connection pool to route writes to the new primary.

### Failover

```bash
ayb db replicas failover [--target replica-2] [--force]
```

Initiates failover to a healthy replica. If `--target` is omitted, AYB auto-selects the healthiest available replica.

Safety constraints:
- Without `--force`, failover requires the current primary to be unhealthy. If the primary is still reachable, pass `--force` to override.
- The target replica must be in `healthy` state.
- At least one healthy replica candidate must be available.

## Query routing

AYB automatically classifies SQL queries and routes them to the appropriate node.

### Read/write classification

From `internal/replica/classifier.go`:

| Routed to replicas | Routed to primary |
|---------------------|-------------------|
| `SELECT`, `WITH`, `EXPLAIN`, `SHOW` | `INSERT`, `UPDATE`, `DELETE`, `CREATE`, `ALTER`, `DROP`, `TRUNCATE`, `COPY`, `BEGIN`, `COMMIT`, `ROLLBACK`, `SAVEPOINT`, `RELEASE`, `SET` |

HTTP methods also influence routing: `POST`, `PUT`, `PATCH`, `DELETE` requests always route to the primary regardless of query content.

### Strong consistency

To force a read to the primary (read-after-write consistency), send the `X-Read-Consistency: strong` header:

```bash
curl http://localhost:8090/api/data/users \
  -H "X-Read-Consistency: strong"
```

The middleware checks this header case-insensitively and sets `RoutingState.ForcePrimary = true` for the request scope.

### Weighted round-robin

When multiple replicas are healthy, AYB distributes read queries using weighted round-robin based on each replica's `weight` value. A replica with `weight: 2` receives twice as many read queries as one with `weight: 1`.

### Graceful degradation

If all replicas become unhealthy, AYB automatically falls back to primary-only mode — all queries route to the primary until replicas recover. When no replicas are configured at all, the pool router operates in pass-through mode.

## Health monitoring

AYB runs continuous health checks against each replica on a 10-second interval.

### Health states

| State | Description |
|-------|-------------|
| `healthy` | Replica is reachable and within lag threshold |
| `suspect` | Transitional state between healthy and unhealthy |
| `unhealthy` | Replica is unreachable or exceeds lag threshold |

### State transitions

Each check result triggers a single state transition:

```
healthy ──(failure)──→ suspect ──(failure)──→ unhealthy
unhealthy ──(success)──→ suspect ──(success)──→ healthy
```

- A failure moves `healthy` → `suspect`
- A further failure moves `suspect` → `unhealthy`
- A success moves `unhealthy` → `suspect`
- A further success moves `suspect` → `healthy`

A success while `suspect` (from failures) resets to `healthy`. A failure while `suspect` (from successes) resets to `unhealthy`. This means the `suspect` state acts as a single-check buffer in both directions.

### Lag detection

Replication lag is measured via `pg_wal_lsn_diff(sent_lsn, replay_lsn)` from `pg_stat_replication` on the primary. If a replica's lag exceeds its `max_lag_bytes` threshold, the health check marks it as a failure.

Health check timeouts: 2 seconds for ping, 2 seconds for lag query.

### Triggering a manual health check

```bash
ayb db replicas check
```

Or via the API:

```bash
curl -X POST http://localhost:8090/api/admin/replicas/check \
  -H "Authorization: Bearer $ADMIN_TOKEN"
```

## CLI commands

All replica CLI commands require `--admin-token` (or `AYB_ADMIN_TOKEN` env var) and connect to `--url` (default: `http://127.0.0.1:8090`).

| Command | Description |
|---------|-------------|
| `ayb db replicas list` | List all replicas with health status |
| `ayb db replicas check` | Trigger immediate health check |
| `ayb db replicas add` | Add a new replica |
| `ayb db replicas remove <name>` | Remove a replica (use `--force` for last active) |
| `ayb db replicas promote <name>` | Promote a replica to primary |
| `ayb db replicas failover` | Initiate failover (`--target`, `--force`) |

### `ayb db replicas add` flags

| Flag | Default | Description |
|------|---------|-------------|
| `--name` | — | Replica name |
| `--host` | — | Hostname (required) |
| `--port` | `5432` | Port |
| `--database` | — | Database name |
| `--ssl-mode` | — | SSL mode |
| `--weight` | `1` | Routing weight |
| `--max-lag-bytes` | `10485760` | Max lag threshold (10 MB) |

## API endpoints

All replica endpoints require admin authentication and are mounted under `/api`.

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/admin/replicas` | List all replicas with health status |
| `POST` | `/api/admin/replicas` | Add a new replica |
| `POST` | `/api/admin/replicas/check` | Trigger health check |
| `DELETE` | `/api/admin/replicas/{name}` | Remove replica (query: `force=true`) |
| `POST` | `/api/admin/replicas/{name}/promote` | Promote to primary |
| `POST` | `/api/admin/replicas/failover` | Initiate failover |

### Request bodies

**Add replica** (`POST /api/admin/replicas`):
```json
{
  "name": "replica-2",
  "host": "replica2.example.com",
  "port": 5432,
  "database": "mydb",
  "ssl_mode": "require",
  "weight": 2,
  "max_lag_bytes": 10485760
}
```

**Failover** (`POST /api/admin/replicas/failover`):
```json
{
  "target": "replica-2",
  "force": false
}
```

### Response codes

| Code | Meaning |
|------|---------|
| `200` | Success |
| `400` | Invalid parameters (missing host, name conflict, invalid target) |
| `404` | Replica not found |
| `409` | Safety constraint violation (last replica, primary healthy, unhealthy target) |
| `500` | Internal error |
| `502` | Replica connectivity check failed |

### Replica status response

Each replica in the list response includes:

```json
{
  "name": "replica-1",
  "url": "postgres://...@replica1:5432/mydb",
  "state": "healthy",
  "lag_bytes": 1024,
  "weight": 1,
  "connections": { "total": 10, "idle": 8, "in_use": 2 },
  "last_checked_at": "2026-03-15T10:30:00Z",
  "last_error": null
}
```

## Related guides

- [Admin Dashboard — Database section](/guide/admin-dashboard#database) for the replicas admin view
- [Security](/guide/security) for API key authentication on admin endpoints
- [Deployment](/guide/deployment) for production replica topology planning
