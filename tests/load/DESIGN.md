# Stage 1 Load Harness Design (Frozen)

Last updated: 2026-03-19
Scope: Stage 1 research only (no load scripts yet)

## Objective

Freeze the k6 integration approach before Stage 2 implementation. Decide whether realtime load coverage is WebSocket-only or includes SSE, and lock supporting contracts (admin bootstrap, rate-limit overrides, endpoint contracts, per-VU identity pooling, preferred WS auth path).

## Decisions (Frozen)

1. Stage 2 realtime load coverage is **WebSocket-only**.
2. SSE automation is a **documented gap** for follow-up, not part of Stage 2.
3. Canonical WS auth method is **Authorization header at upgrade**.
4. Admin bootstrap flow is fixed to `AYB_ADMIN_TOKEN` -> `~/.ayb/admin-token` password -> `POST /api/admin/auth`.
5. Load runs should override rate limits with:
   - `AYB_AUTH_RATE_LIMIT=10000`
   - `AYB_RATE_LIMIT_API=10000/min`
   - `AYB_RATE_LIMIT_API_ANONYMOUS=10000/min`
6. Realtime harness must use per-VU or pooled identities because default `MaxConnectionsPerUser=100` can cap shared users.

## Evidence and Rationale

### 1) k6 protocol support covers Stage 2 HTTP + WS needs

- AYB mounts user/admin APIs under `/api`: `internal/server/server.go:273`.
- Required Stage 2 endpoints exist:
  - `GET /api/admin/status`: `internal/server/routes_admin.go:26`
  - `POST /api/auth/*`: `internal/server/routes_auth.go:130`, `internal/auth/handler.go:97-103`
  - `GET/POST/PATCH/DELETE /api/collections/{table}/` + `POST /batch`: `internal/api/handler.go:191-200`
  - `GET /api/realtime/ws`: `internal/server/routes_api.go:89`
- k6 documents built-in protocol support for HTTP/1.1, HTTP/2, WebSockets, gRPC (SSE is not listed as built-in).

Decision impact: Stage 2 can implement baseline/admin/auth/CRUD/WS flows with k6 built-ins only.

### 2) SSE is intentionally excluded from Stage 2

- AYB SSE endpoint exists at `GET /api/realtime` with `tables`, `token`, `oauth` behavior:
  - `internal/server/routes_api.go:86`
  - `internal/realtime/handler.go:51-53`, `internal/realtime/handler.go:62`, `internal/realtime/handler.go:91-94`, `internal/realtime/handler.go:333-340`
- k6 has no built-in SSE/EventSource entry in supported protocols docs.
- k6 HTTP `Response.body` is documented as a string, not an SSE event-stream API.
- Upstream k6 SSE feature request is closed as not planned.
- Community extension path exists (`xk6-sse`) and registry entry is present; latest release page points to `v0.1.12`.

Decision impact: Stage 2 avoids extension/runtime variability and focuses on first-class k6 capabilities. SSE remains a tracked follow-up stream.

### 3) WebSocket auth + protocol feasibility

- AYB WS auth at upgrade supports bearer header or `?token=` query:
  - `internal/ws/handler.go:104-107`
- AYB also supports post-connect auth message handling (`type: "auth"`):
  - `internal/ws/handler.go:203-205`, `internal/ws/handler.go:263-286`
- AYB subscription messages use JSON message types including `auth` and `subscribe`:
  - `internal/ws/message.go:11-12`, `internal/ws/message.go:75`
- k6 WebSocket params support custom headers on connection setup.

Decision impact: use header auth as canonical path; query-token and post-connect auth stay compatibility-only.

### 4) Admin auth bootstrap contract

- CLI resolves admin credentials in this order:
  - `AYB_ADMIN_TOKEN` env var, else
  - `~/.ayb/admin-token` file (password), then exchanges via `/api/admin/auth`.
- Evidence:
  - `internal/cli/logs.go:102-112`
  - `internal/cli/sql.go:23-24`, `internal/cli/sql.go:45-57`, `internal/cli/sql.go:167-176`
  - `internal/server/routes_admin.go:27`
  - `internal/server/admin.go:70-85`

Decision impact: Stage 2 harness bootstrap logic should mirror this sequence.

### 5) Rate-limit override contract for load tests

- Env bindings:
  - `AYB_AUTH_RATE_LIMIT` -> auth integer limit: `internal/config/config_env_auth.go:19`
  - `AYB_RATE_LIMIT_API` and `AYB_RATE_LIMIT_API_ANONYMOUS` -> string specs: `internal/config/config_env.go:213-218`
- API/anonymous formats are validated by `ParseRateLimitSpec` (`N/min` or `N/hour`):
  - `internal/config/config_env.go:69-87`
  - `internal/config/config_validate_auth.go:16-20`
- Defaults:
  - auth rate limit `10`: `internal/config/config.go:71`
  - API `100/min`, anonymous `30/min`: `internal/config/config.go:97-100`

Decision impact: load-safe overrides must be `10000`, `10000/min`, `10000/min` (not bare `10000` for API/anonymous).

### 6) Per-VU identity pooling requirement

- Default cap: `Realtime.MaxConnectionsPerUser = 100`: `internal/config/config.go:217`
- Cap is propagated into runtime connection manager:
  - `internal/server/server_init.go:210`

Decision impact: for >100 concurrent WS sessions, a single shared account will throttle/fail; Stage 2 must pool identities per VU cohort.

## Endpoint Contract Reference (Stage 2 baseline)

- Baseline health:
  - `GET /api/admin/status` (no auth required, no database dependency)
  - `GET /health` is not the baseline measurement target; use it only as a readiness preflight because `handleHealth` pings the configured database and can return degraded when the pool is unreachable.
  - Evidence: `internal/server/routes_admin.go:26`, `internal/server/admin.go:49-56`, `internal/server/helpers.go:31-60`
- Auth:
  - `POST /api/auth/register` body uses `email`, `password`
  - `POST /api/auth/login` body uses `email`, `password`
  - `POST /api/auth/refresh` body uses `refreshToken`
  - Evidence: `internal/auth/handler.go:97-104`, `internal/auth/handler.go:183-184`
- CRUD:
  - `GET /api/collections/{table}/`
  - `POST /api/collections/{table}/`
  - `PATCH /api/collections/{table}/{id}`
  - `DELETE /api/collections/{table}/{id}`
  - `POST /api/collections/{table}/batch`
  - Evidence: `internal/api/handler.go:191-200`
- Admin SQL:
  - `POST /api/admin/sql/` with JSON body `{ "query": "..." }`
  - Evidence: `internal/server/routes_admin.go:142-145`, `internal/server/sql_handler.go:19`

## Stage 2 Implementation Constraints

1. Use k6 built-in HTTP and WS only.
2. Use WS bearer header auth as default helper path.
3. Add SSE as future work item, not in Stage 2 acceptance criteria.
4. Apply load-safe rate limit overrides during harness runs.
5. Use per-VU (or pooled) identities for realtime workloads.

## Stage 4 Data-Path And Pool-Pressure Boundary

1. Shared fixture lifecycle:
   - create/drop dedicated load tables only through `POST /api/admin/sql/` with JSON `{ "query": "..." }`.
   - fixture SQL lives in `tests/load/lib/data.js`, not in `Makefile`.
2. Collection request-path coverage:
   - `GET /api/collections/{table}/`
   - `POST /api/collections/{table}/`
   - `GET /api/collections/{table}/{id}`
   - `PATCH /api/collections/{table}/{id}`
   - `POST /api/collections/{table}/batch`
   - `DELETE /api/collections/{table}/{id}`
3. Pool-pressure path:
   - target `POST /api/admin/sql/` with `SELECT pg_sleep(2)`.
   - tag traffic as `admin_sql_pool_pressure` so pressure metrics remain separate from collection CRUD traffic.

## Stage 5 WebSocket scalability boundary

1. Scope is WebSocket automation only:
   - target `GET /api/realtime/ws`.
   - SSE automation gap remains explicit follow-up scope.
2. Authentication at websocket upgrade:
   - canonical path is `Authorization` header at upgrade.
   - query-token and post-connect `{"type":"auth"}` remain compatibility behavior, not the Stage 5 default path.
3. Shared helper constraints:
   - user sessions reuse Stage 3 auth helpers and Stage 4 fixture/collection helpers.
   - websocket payload assertions are aligned to `internal/ws/message.go` (`connected`, `reply`, `event`).
4. Connection-cap rationale:
   - realtime limits include `MaxConnectionsPerUser = 100`, so Stage 5 identity allocation must support stable per-VU mapping with optional pooling.

## Stage 6 Sustained mixed-workload soak boundary

1. One duration-based mixed scenario:
   - target script: `tests/load/scenarios/sustained_soak.js`.
   - local/direct commands: `make load-sustained-soak-local`, `make load-sustained-soak`.
   - duration override is `AYB_SOAK_DURATION` (default `5m`).
2. Shared helper composition only:
   - auth loop uses shared Stage 3 helper flow (`runAuthRegisterLoginRefreshFlow`).
   - data loop uses shared Stage 4 helper flow (`runDataPathCRUDAndBatchFlow`).
   - realtime loop uses shared Stage 5 helper flow (`runRealtimeSubscribeCreateEventUnsubscribeFlow`).
   - scenario file must not inline `/api/auth/*`, `/api/collections/*`, `/api/realtime/ws`, fixture DDL, or websocket subscribe/unsubscribe payload bodies.
3. Shared setup/teardown and identity anchor:
   - setup/teardown continues through `loadDataRunTableName()` + `createDataFixture()` + `dropDataFixture()`.
   - per-VU identities stay anchored to `allocateLoadUserIdentity()` + `bootstrapTenantScopedSession()`, with no soak-specific bootstrap path.
4. Metrics separation inside one soak:
   - Stage 6 uses endpoint tags prefixed for auth/data/realtime within one scenario so latency/failure trends remain separable.

## Open Questions

1. SSE follow-up scope: only OAuth one-shot + query-token compatibility, or throughput parity benchmarking vs WS?
2. Should Stage 2 include a minimal compatibility smoke for post-connect WS `auth` messages, or defer all non-canonical auth paths to a protocol-compatibility stage?

## External Sources

- k6 protocols (built-in support list): https://grafana.com/docs/k6/latest/using-k6/protocols/
- k6 WebSocket params (custom headers): https://grafana.com/docs/k6/latest/javascript-api/k6-websockets/params/
- k6 HTTP response (`Response.body`): https://grafana.com/docs/k6/latest/javascript-api/k6-http/response/
- k6 extension registry (`xk6-sse` entry): https://grafana.com/docs/k6/latest/extensions/explore/
- SSE feature request closed as not planned: https://github.com/grafana/k6/issues/746
- xk6-sse latest release page: https://github.com/phymbert/xk6-sse/releases/latest
