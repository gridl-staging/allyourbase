# Stage 2/3/4/5/6 Load Harness

This directory contains the shared Stage 2 k6 foundation, Stage 3 auth request-path scenario, Stage 4 data-path/pool-pressure scenarios, Stage 5 realtime websocket scenario, and Stage 6 sustained mixed-workload soak scenario.

## Bootstrap Boundary

- Host-side bootstrap is owned by `Makefile` targets:
  - resolve `AYB_ADMIN_TOKEN` from env first,
  - otherwise read the saved admin password from `~/.ayb/admin-token` and exchange it via `POST /api/admin/auth`,
  - export load-safe rate-limit overrides once.
- k6 helper modules under `tests/load/lib/` only consume environment values.

## Baseline Scenario

- Measured endpoint: `GET /api/admin/status`
- Readiness endpoint: `GET /health` (runner preflight only; not measured)
- Stage 7 measured smoke command: `K6_VUS=1 K6_ITERATIONS=1 make load-admin-status-local`
- Stage 7 contract assertion: `bash tests/test_load_baseline_contract.sh`
- Stage 7 caveat: readiness stays runner-only and is not included in measured request latency.

## Auth Request-Path Scenario

- Measured flow: `POST /api/auth/register` -> `POST /api/auth/login` -> `POST /api/auth/refresh`
- The scenario validates refresh-token rotation by asserting a second refresh attempt with the pre-rotation token returns `401`.
- Stage 7 measured smoke command: `K6_VUS=1 K6_ITERATIONS=1 make load-auth-request-path-local`
- Stage 7 contract assertion: `bash tests/test_load_auth_contract.sh`
- Stage 7 caveat: refresh-token reuse must keep returning `401` after token rotation.

## Data-Path CRUD/Batch Scenario

- Fixture lifecycle: dedicated load table created/dropped through `POST /api/admin/sql/` JSON `{query}`.
- Measured flow: `GET /api/collections/{table}/` -> `POST /api/collections/{table}/` -> `GET /api/collections/{table}/{id}` -> `PATCH /api/collections/{table}/{id}` -> `POST /api/collections/{table}/batch` -> `DELETE /api/collections/{table}/{id}`.
- The scenario includes a failed batch rollback probe to ensure partial batch mutations do not commit.
- Stage 7 measured smoke command: `K6_VUS=1 K6_ITERATIONS=1 make load-data-path-local`
- Stage 7 contract assertion: `bash tests/test_load_data_contract.sh`
- Stage 7 caveat: the rollback probe intentionally submits one invalid batch mutation and expects zero partial writes.

## Data Pool-Pressure Scenario

- Measured endpoint: `POST /api/admin/sql/` with JSON `{query}`.
- Pressure query: `SELECT pg_sleep(2)`.
- Traffic is tagged separately as `admin_sql_pool_pressure`.
- Stage 7 measured smoke command: `K6_VUS=2 K6_ITERATIONS=2 make load-data-pool-pressure-local`
- Stage 7 contract assertion: `bash tests/test_load_data_contract.sh`
- Stage 7 caveat: `http_req_duration` p95 is expected near `2s` because the measured pressure query is `SELECT pg_sleep(2)`.

## Realtime WebSocket Scenario

- Stage 5 automation scope is **WebSocket-only** against `GET /api/realtime/ws`.
- Canonical auth path uses the `Authorization` header during websocket upgrade.
- The scenario waits for the initial `connected` websocket message before sending `{"type":"subscribe"}`.
- Collection writes are issued through shared Stage 4 collection URL helpers to generate realtime `event` payloads.
- SSE automation remains a follow-up gap and is not part of Stage 5 acceptance.
- Stage 7 measured smoke command: `K6_VUS=1 K6_ITERATIONS=1 make load-realtime-ws-local`
- Stage 7 contract assertion: `bash tests/test_load_realtime_contract.sh`
- Stage 7 caveat: websocket users must be able to read the subscribed table or the realtime flow fails fast before waiting for events.

## Sustained Mixed-Workload Soak Scenario

- Stage 6 runs one duration-based workload that composes shared Stage 3/4/5 flow helpers in a single loop.
- The scenario keeps setup/teardown fixture lifecycle in one shared path and uses per-VU pooled identities via `allocateLoadUserIdentity()` + `bootstrapTenantScopedSession()`.
- Stage 6 tracks auth/data/realtime latency and failures with separate endpoint tags inside the same scenario.
- Stage 7 measured smoke command: `AYB_SOAK_DURATION=30s K6_VUS=1 make load-sustained-soak-local`
- Stage 7 contract assertion: `bash tests/test_load_soak_contract.sh`
- Stage 7 caveat: the 30s smoke run confirms mixed-flow wiring but does not cross the 10-minute pooled-session age rollover boundary.

## Commands

- Direct baseline against an already-running AYB:
  - `make load-admin-status`
- Local AYB + baseline run via `scripts/run-with-ayb.sh`:
  - `make load-admin-status-local`
- Direct auth request-path scenario against an already-running AYB:
  - `make load-auth-request-path`
- Local AYB + auth request-path scenario via `scripts/run-with-ayb.sh`:
  - `make load-auth-request-path-local`
- Direct data-path CRUD/batch scenario against an already-running AYB:
  - `make load-data-path`
- Local AYB + data-path CRUD/batch scenario via `scripts/run-with-ayb.sh`:
  - `make load-data-path-local`
- Direct admin SQL pool-pressure scenario against an already-running AYB:
  - `make load-data-pool-pressure`
- Local AYB + admin SQL pool-pressure scenario via `scripts/run-with-ayb.sh`:
  - `make load-data-pool-pressure-local`
- Direct realtime websocket scenario against an already-running AYB:
  - `make load-realtime-ws`
- Local AYB + realtime websocket scenario via `scripts/run-with-ayb.sh`:
  - `make load-realtime-ws-local`
- Direct sustained mixed-workload soak scenario against an already-running AYB:
  - `make load-sustained-soak`
- Local AYB + sustained mixed-workload soak scenario via `scripts/run-with-ayb.sh`:
  - `make load-sustained-soak-local`

## Smallest Smoke Mode

Use the smallest measured smoke commands for fast validation:

```bash
K6_VUS=1 K6_ITERATIONS=1 make load-admin-status
K6_VUS=1 K6_ITERATIONS=1 make load-admin-status-local
K6_VUS=1 K6_ITERATIONS=1 make load-auth-request-path
K6_VUS=1 K6_ITERATIONS=1 make load-auth-request-path-local
K6_VUS=1 K6_ITERATIONS=1 make load-data-path-local
K6_VUS=2 K6_ITERATIONS=2 make load-data-pool-pressure-local
K6_VUS=1 K6_ITERATIONS=1 make load-realtime-ws-local
AYB_SOAK_DURATION=30s K6_VUS=1 make load-sustained-soak-local
```

## Optional Overrides

- `AYB_BASE_URL` (default `http://127.0.0.1:8090`)
- `AYB_ADMIN_TOKEN` (if omitted, the host bootstrap will try the saved `~/.ayb/admin-token` password flow)
- `K6_VUS`, `K6_ITERATIONS`
- `AYB_SOAK_DURATION` (default `5m`, used by `load-sustained-soak*` duration executor)
