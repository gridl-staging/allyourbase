<!-- audited 2026-03-20 -->
# Backups & Point-in-Time Recovery

This guide covers backup, restore, and PITR workflows in AYB, based on code in `internal/backup/`, `internal/cli/db_backup.go`, and `internal/server/backup_admin_handler.go`.

## Overview

AYB supports two backup strategies:

- **Logical backups** (`pg_dump`): Portable SQL-format dumps for cross-version restores and selective table recovery.
- **Physical backups** (`pg_basebackup`): Full binary copies of the data directory for fast, exact-state restores. Required for point-in-time recovery (PITR).

Both strategies store artifacts in S3-compatible object storage with optional server-side encryption.

### WAL archival

When PITR is enabled, AYB continuously archives PostgreSQL write-ahead log (WAL) segments to a dedicated S3 bucket. Combined with periodic physical base backups, WAL archival enables recovery to any point within the configured retention window.

## Configuration

Backup and PITR settings live in your `ayb.toml` config file.

### `[backup]` section

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `enabled` | bool | `false` | Enable the backup subsystem |
| `bucket` | string | — | S3 bucket name (required when enabled) |
| `region` | string | `"us-east-1"` | S3 region |
| `prefix` | string | `"backups"` | Object key prefix for backup artifacts |
| `schedule` | string | `"0 2 * * *"` | Cron expression for automated backups (daily 2 AM UTC) |
| `retention_count` | int | `7` | Maximum number of backups to retain (0 = unlimited) |
| `retention_days` | int | `30` | Delete backups older than N days (0 = unlimited) |
| `encryption` | string | `"AES256"` | Server-side encryption: `""` (none), `"AES256"`, or `"aws:kms"` |
| `endpoint` | string | `""` | Custom S3 endpoint for MinIO or LocalStack |
| `access_key` | string | `""` | S3 access key |
| `secret_key` | string | `""` | S3 secret key |

### `[backup.pitr]` section

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `enabled` | bool | `false` | Enable point-in-time recovery |
| `archive_bucket` | string | — | S3 bucket for WAL archives (required when enabled) |
| `archive_prefix` | string | `""` | Optional namespace prefix for WAL archive paths |
| `wal_retention_days` | int | `14` | Days to retain archived WAL segments |
| `base_backup_retention_days` | int | `35` | Days to retain physical base backups |
| `compliance_snapshot_months` | int | `12` | Months to retain compliance snapshots |
| `environment_class` | string | `"non-prod"` | Environment label (e.g. `"prod"`, `"staging"`) |
| `kms_key_id` | string | `""` | KMS key ID for WAL encryption |
| `retention_schedule` | string | `"0 4 * * *"` | Cron expression for retention cleanup |
| `rpo_minutes` | int | `5` | Target recovery point objective in minutes (must be > 0) |
| `storage_budget_bytes` | int64 | `0` | Maximum storage for WAL archives (0 = unlimited) |
| `shadow_mode` | bool | `true` | See [Shadow mode](#shadow-mode) below |
| `base_backup_schedule` | string | `"0 3 * * *"` | Cron expression for physical base backups |
| `verify_schedule` | string | `"0 */6 * * *"` | Cron expression for backup verification |

### Shadow mode

::: warning
`shadow_mode` defaults to **`true`**. In shadow mode, AYB archives WAL segments and takes base backups normally, but **refuses actual restore cutover requests** with a `409 Conflict` error. This lets you validate that archival works before enabling real restores.

Set `shadow_mode = false` in your `[backup.pitr]` config to enable actual point-in-time restores.
:::

## CLI commands

### `ayb db backup`

Trigger an immediate logical backup.

```bash
ayb db backup [--database-url <url>] [--config <path>] [--output json]
```

| Flag | Description |
|------|-------------|
| `--database-url` | Database URL (overrides config) |
| `--config` | Path to `ayb.toml` config file |
| `--output` | Output format: `table` (default) or `json` |

Output includes: `BackupID`, `Status`, `ObjectKey`, `SizeBytes`, `Checksum`.

### `ayb db backup list`

List existing backups with optional filtering.

```bash
ayb db backup list [--status <status>] [--limit <n>] [--database-url <url>] [--config <path>] [--output json]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--status` | — | Filter by status: `running`, `completed`, `failed` |
| `--limit` | `20` | Maximum number of records |
| `--database-url` | — | Database URL (overrides config) |
| `--config` | — | Path to `ayb.toml` |
| `--output` | `table` | Output format: `table` or `json` |

### `ayb db restore`

Restore from a backup.

```bash
ayb db restore --from <backup-id-or-key> [--database-url <url>] [--config <path>] [--yes]
```

| Flag | Description |
|------|-------------|
| `--from` | Backup ID or S3 object key to restore from |
| `--database-url` | Target database URL |
| `--config` | Path to `ayb.toml` |
| `--yes`, `-y` | Skip confirmation prompt |

## PITR workflow

Point-in-time recovery follows a validate → dry-run → execute → monitor sequence.

### 1. Validate the restore window

Before attempting a restore, validate that the target time falls within the available recovery window:

```bash
curl -X POST http://localhost:8090/api/admin/backups/projects/{projectId}/pitr/validate \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"target_time": "2026-03-15T10:30:00Z", "database_id": "db-1"}'
```

The response includes `earliest_recoverable` and `latest_recoverable` timestamps, the base backup that would be used, and estimated WAL bytes to replay.

**Validation requirements** (from `restore_planner.go`):
- At least one completed physical backup must exist.
- Target time must be between the earliest completed backup and the latest archived WAL segment.
- WAL segments must form a contiguous chain from the base backup's end LSN to the target.

### 2. Dry-run the restore

Pass `"dry_run": true` to the restore endpoint to see the plan without executing:

```bash
curl -X POST http://localhost:8090/api/admin/backups/projects/{projectId}/pitr/restore \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"target_time": "2026-03-15T10:30:00Z", "database_id": "db-1", "dry_run": true}'
```

### 3. Execute the restore

Set `"dry_run": false` (or omit it) to start the restore:

```bash
curl -X POST http://localhost:8090/api/admin/backups/projects/{projectId}/pitr/restore \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"target_time": "2026-03-15T10:30:00Z", "database_id": "db-1"}'
```

Returns a `job_id` for tracking. The restore proceeds through these phases:

| Phase | Description |
|-------|-------------|
| `pending` | Restore job created |
| `validating` | Validating restore window and backup availability |
| `restoring` | Extracting base backup, downloading WAL segments, applying recovery |
| `verifying` | WAL replay complete, verifying data consistency |
| `ready_for_cutover` | Recovery instance ready for application switchover |
| `completed` | Restore finished |
| `failed` | Restore encountered an error (check job status for details) |

### 4. Monitor the restore job

```bash
# List jobs for a project
curl http://localhost:8090/api/admin/backups/projects/{projectId}/pitr/jobs?database_id=db-1 \
  -H "Authorization: Bearer $ADMIN_TOKEN"

# Get a specific job
curl http://localhost:8090/api/admin/backups/restore-jobs/{jobId} \
  -H "Authorization: Bearer $ADMIN_TOKEN"

# Abandon a running job
curl -X DELETE http://localhost:8090/api/admin/backups/restore-jobs/{jobId} \
  -H "Authorization: Bearer $ADMIN_TOKEN"
```

## API endpoints

All backup endpoints require admin authentication and are mounted under `/api`.

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/admin/backups` | List backups (query: `status`, `limit`, `offset`) |
| `POST` | `/api/admin/backups` | Trigger a new backup |
| `POST` | `/api/admin/backups/projects/{projectId}/pitr/validate` | Validate PITR restore window |
| `POST` | `/api/admin/backups/projects/{projectId}/pitr/restore` | Start or dry-run a PITR restore |
| `GET` | `/api/admin/backups/projects/{projectId}/pitr/jobs` | List restore jobs (query: `database_id`) |
| `GET` | `/api/admin/backups/restore-jobs/{jobId}` | Get restore job status |
| `DELETE` | `/api/admin/backups/restore-jobs/{jobId}` | Abandon a restore job |

### Response codes

| Code | Meaning |
|------|---------|
| `200` | Success |
| `202` | Backup triggered / restore started |
| `400` | Invalid parameters or target time |
| `404` | Job not found |
| `409` | Shadow mode active (restore refused) |
| `500` | Internal error |
| `503` | Backup or PITR service not configured |

## Fire drill testing

Automated fire drills validate that your backup and recovery pipeline works end-to-end. A fire drill attempts a PITR restore to a target 5 minutes in the past and reports whether the restore plan is viable.

The `FireDrillResult` includes:
- Whether the drill `Passed`
- The `RestorePlan` (base backup + WAL segments) that would be used
- Any error encountered

Fire drills run on the schedule configured by `verify_schedule` (default: every 6 hours).

## Retention and storage budget

AYB provides multiple retention controls:

- **`retention_count`**: Maximum number of logical backups to keep.
- **`retention_days`**: Delete logical backups older than N days.
- **`wal_retention_days`**: Delete archived WAL segments older than N days (default: 14).
- **`base_backup_retention_days`**: Delete physical base backups older than N days (default: 35).
- **`compliance_snapshot_months`**: Retain compliance snapshots for N months (default: 12).
- **`storage_budget_bytes`**: Cap total WAL archive storage (0 = unlimited).

Retention cleanup runs on the `retention_schedule` cron (default: daily at 4 AM UTC).

## Backup statuses

| Status | Description |
|--------|-------------|
| `pending` | Backup record created, not yet started |
| `running` | Backup in progress |
| `completed` | Backup finished successfully |
| `failed` | Backup encountered an error |
| `deleted` | Backup artifact removed by retention policy |

## Related guides

- [Admin Dashboard — Database section](/guide/admin-dashboard#database) for the backup admin view
- [Security](/guide/security) for encryption-at-rest of backup artifacts
- [Deployment](/guide/deployment) for S3 and storage configuration
