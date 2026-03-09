# Materialized Views

PostgreSQL materialized views store the results of a query physically on disk, making them ideal for expensive aggregations like leaderboards, dashboards, and statistics. AYB serves materialized views read-only through the standard collections API and provides a built-in mechanism to register, refresh, and schedule refreshes.

## Creating a materialized view

Create the view in PostgreSQL directly (via migration, SQL editor, or `psql`):

```sql
CREATE MATERIALIZED VIEW leaderboard AS
  SELECT u.id, u.name, COUNT(w.id) AS workout_count, SUM(w.duration_minutes) AS total_minutes
  FROM users u
  JOIN workouts w ON w.user_id = u.id
  GROUP BY u.id, u.name
  ORDER BY total_minutes DESC;
```

AYB detects materialized views during schema introspection and serves them at `/api/collections/leaderboard` (read-only — inserts, updates, and deletes are rejected).

## Registering a materialized view

Register a materialized view with AYB to enable managed refresh:

**CLI:**

```bash
ayb matviews register --view leaderboard --schema public --mode standard
```

**Admin API:**

```bash
curl -X POST http://localhost:8090/api/admin/matviews \
  -H "Authorization: Bearer $AYB_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"viewName": "leaderboard", "schema": "public", "refreshMode": "standard"}'
```

**Admin dashboard:**

Navigate to the Materialized Views section, click "Register", select the view from the dropdown of discovered views, choose a refresh mode, and confirm.

Registration validates that the target is actually a materialized view in the database. Duplicate registrations are rejected.

## Refresh modes

### Standard

```sql
REFRESH MATERIALIZED VIEW "public"."leaderboard"
```

Acquires an `ACCESS EXCLUSIVE` lock on the view for the duration of the refresh. Reads are blocked while refreshing. Use this for views that can tolerate brief unavailability.

### Concurrent

```sql
REFRESH MATERIALIZED VIEW CONCURRENTLY "public"."leaderboard"
```

Refreshes without blocking reads. Requires:

1. **A UNIQUE index** covering all rows (no partial index, no expression index):

```sql
CREATE UNIQUE INDEX leaderboard_id_idx ON leaderboard (id);
```

2. **The view must be populated** (has been refreshed at least once with standard mode first).

If either prerequisite is missing, AYB returns a clear error rather than passing through a cryptic Postgres error.

## Manual refresh

Trigger an immediate, synchronous refresh:

**CLI:**

```bash
# By registration ID
ayb matviews refresh 550e8400-e29b-41d4-a716-446655440000

# By qualified name
ayb matviews refresh public.leaderboard
```

**Admin API:**

```bash
curl -X POST http://localhost:8090/api/admin/matviews/550e8400-e29b-41d4-a716-446655440000/refresh \
  -H "Authorization: Bearer $AYB_ADMIN_TOKEN"
```

The response includes the refresh duration:

```json
{
  "registration": {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "schemaName": "public",
    "viewName": "leaderboard",
    "refreshMode": "standard",
    "lastRefreshAt": "2026-02-22T10:00:00Z",
    "lastRefreshDurationMs": 342,
    "lastRefreshStatus": "success",
    "createdAt": "2026-02-22T08:00:00Z",
    "updatedAt": "2026-02-22T10:00:00Z"
  },
  "durationMs": 342
}
```

### Mutual exclusion

AYB uses PostgreSQL advisory locks (`pg_try_advisory_lock`) to prevent duplicate concurrent refreshes of the same view. If a refresh is already in progress, the request returns `409 Conflict` with message `"refresh already in progress"`. Advisory locks are session-level and do not hold a transaction open during the refresh.

## Scheduled refresh

Use the [job queue](./job-queue.md) to schedule periodic refreshes. Create a schedule with job type `materialized_view_refresh`:

```bash
ayb schedules create \
  --name leaderboard_hourly \
  --job-type materialized_view_refresh \
  --cron "0 * * * *" \
  --payload '{"schema": "public", "view_name": "leaderboard"}'
```

Or via the admin API:

```bash
curl -X POST http://localhost:8090/api/admin/schedules \
  -H "Authorization: Bearer $AYB_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "leaderboard_hourly",
    "jobType": "materialized_view_refresh",
    "cronExpr": "0 * * * *",
    "timezone": "UTC",
    "payload": {"schema": "public", "view_name": "leaderboard"},
    "enabled": true
  }'
```

The scheduled handler auto-registers the view if no registration exists yet (with `standard` mode). It then calls the same refresh path as manual refresh, including advisory lock protection.

::: tip
Scheduled refresh requires `jobs.enabled = true` in your configuration. Manual refresh via the API or CLI works regardless of whether the jobs subsystem is enabled.
:::

## Managing registrations

### List

```bash
ayb matviews list
ayb matviews list --json
```

### Update refresh mode

```bash
ayb matviews update <id> --mode concurrent
```

### Unregister

```bash
ayb matviews unregister <id>
```

Unregistering removes the registration metadata only. It does not drop the materialized view from the database.

## Monitoring refresh status

Each registration tracks:

| Field | Description |
|---|---|
| `lastRefreshAt` | Timestamp of the most recent refresh attempt |
| `lastRefreshDurationMs` | Duration of the most recent refresh in milliseconds |
| `lastRefreshStatus` | `"success"` or `"error"` |
| `lastRefreshError` | Error message if the last refresh failed |

View this data via the admin dashboard, CLI (`ayb matviews list`), or API (`GET /api/admin/matviews`).

## Error handling

| Error | HTTP status | Meaning |
|---|---|---|
| View not found in database | `404` | The target is not a materialized view or was dropped |
| Already registered | `409` | A registration for this schema/view pair already exists |
| Refresh already in progress | `409` | Another refresh is running (advisory lock held) |
| Concurrent requires unique index | `409` | `CONCURRENTLY` mode needs a UNIQUE index on the view |
| Concurrent requires populated view | `409` | `CONCURRENTLY` mode needs the view to be populated first |
| Invalid identifier | `400` | Schema or view name contains unsafe characters |

## Limitations

- Materialized views do not fire PostgreSQL triggers, so inserts/updates to underlying tables do not automatically refresh the view. Use scheduled or manual refresh.
- Realtime SSE subscriptions do not work on materialized views (no trigger-based change notification). Query the collections API to read refreshed data.
- There is no incremental or partial refresh — PostgreSQL recomputes the entire query on each refresh.
- AYB does not execute arbitrary SQL. Only validated `REFRESH MATERIALIZED VIEW [CONCURRENTLY]` statements are run.
