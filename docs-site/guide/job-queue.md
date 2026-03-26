# Job Queue
<!-- audited 2026-03-23 -->

AYB includes a persistent Postgres-backed job queue with an in-process scheduler for recurring maintenance work.

It is designed for at-least-once execution with retry and crash recovery.

## When to enable

Enable jobs when you want built-in background cleanup to run through the queue and scheduler:

- stale session cleanup
- webhook delivery log pruning
- expired OAuth token/code cleanup
- expired magic-link/password-reset cleanup
- expired resumable upload cleanup
- audit log retention pruning
- request log retention pruning

Keep it disabled if you want legacy timer-only behavior (default).

## Enable and tune

```toml
[jobs]
enabled = true
worker_concurrency = 4
poll_interval_ms = 1000
lease_duration_s = 300
max_retries_default = 3
scheduler_enabled = true
scheduler_tick_s = 15
```

Environment variable overrides:

- `AYB_JOBS_ENABLED`
- `AYB_JOBS_WORKER_CONCURRENCY`
- `AYB_JOBS_POLL_INTERVAL_MS`
- `AYB_JOBS_LEASE_DURATION_S`
- `AYB_JOBS_MAX_RETRIES_DEFAULT`
- `AYB_JOBS_SCHEDULER_ENABLED`
- `AYB_JOBS_SCHEDULER_TICK_S`

## Built-in job types

| Job type | What it cleans up |
|---|---|
| `stale_session_cleanup` | Expired rows in `_ayb_sessions` |
| `webhook_delivery_prune` | Old rows in `_ayb_webhook_deliveries` (default retention `168` hours) |
| `expired_oauth_cleanup` | Expired/revoked rows in `_ayb_oauth_tokens`; expired/used-old rows in `_ayb_oauth_authorization_codes` |
| `expired_auth_cleanup` | Expired rows in `_ayb_magic_links` and `_ayb_password_resets` |
| `expired_resumable_upload_cleanup` | Expired resumable upload sessions/artifacts |
| `audit_log_retention` | Audit rows older than configured retention days (`audit.retention_days`, default 90) |
| `request_log_retention` | Request-log rows older than configured retention days (`logging.request_log_retention_days`, default 7) |

## Default schedules

These schedules are upserted on startup when jobs are enabled:

| Name | Job type | Cron (UTC) |
|---|---|---|
| `audit_log_retention_daily` | `audit_log_retention` | `0 2 * * *` |
| `session_cleanup_hourly` | `stale_session_cleanup` | `0 * * * *` |
| `webhook_delivery_prune_daily` | `webhook_delivery_prune` | `0 3 * * *` |
| `expired_oauth_cleanup_daily` | `expired_oauth_cleanup` | `0 4 * * *` |
| `expired_auth_cleanup_daily` | `expired_auth_cleanup` | `0 5 * * *` |
| `request_log_retention_daily` | `request_log_retention` | `0 6 * * *` |
| `expired_resumable_upload_cleanup` | `expired_resumable_upload_cleanup` | `*/10 * * * *` |

When push notifications are also enabled, startup wiring additionally upserts:

| Name | Job type | Cron (UTC) |
|---|---|---|
| `push_token_cleanup_daily` | `push_token_cleanup` | `0 2 * * *` |

## State model

Jobs move through:

- `queued` -> `running` -> `completed`
- `queued` -> `running` -> `queued` (retry with backoff)
- `queued` -> `running` -> `failed` (after max attempts)
- `queued` -> `canceled`

Crash recovery requeues stale `running` jobs when lease expires.

## Admin and CLI operations

All endpoints require admin auth (`Authorization: Bearer <admin-token>`).

- `GET /api/admin/jobs`
- `GET /api/admin/jobs/stats`
- `GET /api/admin/jobs/{id}`
- `POST /api/admin/jobs/{id}/retry`
- `POST /api/admin/jobs/{id}/cancel`
- `GET /api/admin/schedules`
- `POST /api/admin/schedules`
- `PUT /api/admin/schedules/{id}`
- `DELETE /api/admin/schedules/{id}`
- `POST /api/admin/schedules/{id}/enable`
- `POST /api/admin/schedules/{id}/disable`

Retry/cancel return HTTP `409` when the job is missing or not in the required source state (`failed` for retry, `queued` for cancel).

`GET /api/admin/jobs` supports optional query params:

- `state`: filters by one of `queued`, `running`, `completed`, `failed`, `canceled`; invalid values return HTTP `400`.
- `type`: filters by job type string.
- `limit`: page size; defaults to `50`, clamped to a max of `500`.
- `offset`: zero-based offset; defaults to `0` (negative values are normalized to `0`).

CLI:

```bash
ayb jobs list --state failed
ayb jobs retry <job-id>
ayb jobs cancel <job-id>

ayb schedules list
ayb schedules create --name cleanup --job-type stale_session_cleanup --cron "0 * * * *"
ayb schedules update <schedule-id> --cron "15 * * * *" --enabled true
ayb schedules enable <schedule-id>
ayb schedules disable <schedule-id>
ayb schedules delete <schedule-id>
```

## Operational guidance

- Monitor queue pressure with `GET /api/admin/jobs/stats`:
  - `queued` growth and `oldestQueuedAgeSec` indicate lag.
- Increase `worker_concurrency` for higher throughput.
- Increase `lease_duration_s` if handlers legitimately run longer than current lease.
- Inspect failed jobs and use retry once root cause is fixed.
- Handlers must be idempotent because delivery is at-least-once, not exactly-once.

## Compatibility note

When `jobs.enabled = false`, jobs/schedules admin endpoints return `503`, no workers start, and legacy webhook pruning remains active.
