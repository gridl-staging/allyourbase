# Job Queue

AYB includes a persistent Postgres-backed job queue with an in-process scheduler for recurring maintenance work.

It is designed for at-least-once execution with retry and crash recovery.

## When to enable

Enable jobs when you want built-in background cleanup to run through the queue and scheduler:

- stale session cleanup
- webhook delivery log pruning
- expired OAuth token/code cleanup
- expired magic-link/password-reset cleanup

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

## Default schedules

These schedules are upserted on startup when jobs are enabled:

| Name | Job type | Cron (UTC) |
|---|---|---|
| `session_cleanup_hourly` | `stale_session_cleanup` | `0 * * * *` |
| `webhook_delivery_prune_daily` | `webhook_delivery_prune` | `0 3 * * *` |
| `expired_oauth_cleanup_daily` | `expired_oauth_cleanup` | `0 4 * * *` |
| `expired_auth_cleanup_daily` | `expired_auth_cleanup` | `0 5 * * *` |

## State model

Jobs move through:

- `queued` -> `running` -> `completed`
- `queued` -> `running` -> `queued` (retry with backoff)
- `queued` -> `running` -> `failed` (after max attempts)
- `queued` -> `canceled`

Crash recovery requeues stale `running` jobs when lease expires.

## Admin and CLI operations

Admin API:

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
