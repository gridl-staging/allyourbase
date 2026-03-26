# Log Drains
<!-- audited 2026-03-21 -->

This guide documents AYB's shipped external log-drain system: HTTP/Datadog/Loki drain types, worker batching/retry behavior, admin runtime APIs, and config defaults.

Source of truth:

- `internal/server/admin_drains_handler.go`
- `internal/server/server_init.go::normalizeLogDrainConfig`
- `internal/logging/drain.go`
- `internal/logging/http_drain.go`
- `internal/logging/datadog_drain.go`
- `internal/logging/loki_drain.go`
- `internal/logging/worker.go`
- `internal/logging/manager.go`
- Tests: `internal/server/admin_drains_handler_test.go`, `internal/logging/drain_test.go`

## Supported drain types

Only these drain `type` values are implemented:

- `http`
- `datadog`
- `loki`

Any other type is rejected.

## Config format

Each `[[logging.drains]]` entry maps to one `LogDrainConfig`:

- `id`
- `type`
- `url`
- `headers`
- `batch_size`
- `flush_interval_seconds`
- `enabled`

Defaults (from validation/normalization):

- `id`: `drain-<index>` (startup config) or `drain-<timestamp>` (runtime create path)
- `enabled`: `true`
- `batch_size`: `100`
- `flush_interval_seconds`: `5`
- `headers`: `{}`

Example:

```toml
[[logging.drains]]
id = "ops-http"
type = "http"
url = "https://logs.example.com/ingest"
enabled = true
batch_size = 200
flush_interval_seconds = 3

[logging.drains.headers]
Authorization = "Bearer token"
```

## Admin runtime API

All endpoints require admin auth (`Authorization: Bearer <admin-token>`).

- `GET /api/admin/logging/drains`
- `POST /api/admin/logging/drains`
- `DELETE /api/admin/logging/drains/{id}`

### Create drain

```bash
curl -X POST http://localhost:8090/api/admin/logging/drains \
  -H "Authorization: Bearer $AYB_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "id": "runtime-loki",
    "type": "loki",
    "url": "https://loki.example.com/loki/api/v1/push",
    "headers": {"Authorization": "Bearer loki-token"},
    "batch_size": 100,
    "flush_interval_seconds": 5,
    "enabled": true
  }'
```

If `enabled=false`, AYB returns `201` but does not register a worker.

### List drains response

`GET /api/admin/logging/drains` returns an array of:

- `id`
- `name`
- `stats`

`stats` fields:

- `sent`
- `failed`
- `dropped`

## Delivery semantics

Each active drain runs in a `DrainWorker`:

- queue size: `10000` (set by server wiring)
- non-blocking enqueue fanout from `DrainManager`
- flush triggers:
  - batch size reached
  - flush interval elapsed

Retry behavior (`sendWithRetry`):

- default max retries: 3
- exponential backoff with +/-25% jitter
- default backoff: base 1s, cap 30s
- after retries are exhausted, entries are marked dropped via `DropReporter`

Important nuance:

- queue overflow drops happen before send-retry and are non-blocking; they are not surfaced as request failures.

## Payload formats

### HTTP drain

- `POST` to `url`
- `Content-Type: application/json`
- body: JSON array of `LogEntry`

`LogEntry` fields:

- `timestamp`
- `level`
- `message`
- `source`
- `fields`

### Datadog drain

- JSON array of Datadog log objects
- includes:
  - `ddsource: "ayb"`
  - `service: "ayb"`
  - `status`
  - `message`
  - `timestamp` (Unix ms)
  - `attributes` (from AYB `fields`)

### Loki drain

- `POST` to Loki push endpoint (typically `/loki/api/v1/push`)
- payload shape: `{ "streams": [...] }`
- entries grouped into streams by `level` + `source`, with stream labels:
  - `source: "ayb"`
  - `level`
  - optional `log_source`
- line payload is JSON including `message`, `level`, `log_source`, and structured fields
- if a custom field collides with reserved keys, Loki line writer prefixes it with `field_`

## Startup config vs runtime drains

- Startup: AYB loads `logging.drains` config, normalizes, and registers only enabled + valid drains.
- Runtime admin create/delete: applies in memory via `DrainManager` only.

Disabled config entries are valid config, but they are not active runtime drains and therefore do not appear in list output.

Runtime-added drains are not persisted back into `ayb.toml`.

## Related guides

- [Admin Dashboard](/guide/admin-dashboard#admin)
- [Deployment](/guide/deployment)
