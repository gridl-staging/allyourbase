#!/usr/bin/env bash
set -euo pipefail

fail() {
  echo "FAIL: $1"
  exit 1
}

assert_file() {
  local file_path="$1"
  [[ -f "$file_path" ]] || fail "missing required file: ${file_path}"
}

assert_contains() {
  local file_path="$1"
  local needle="$2"
  local message="$3"
  grep -Fq -- "$needle" "$file_path" || fail "$message"
}

extract_section() {
  local file_path="$1"
  local section_heading="$2"
  awk -v heading="$section_heading" '
    function heading_level(line, prefix) {
      if (match(line, /^#+ /) == 0) {
        return 0
      }
      prefix = substr(line, RSTART, RLENGTH)
      sub(/ $/, "", prefix)
      return length(prefix)
    }
    BEGIN {
      target_level = heading_level(heading)
      in_section = 0
    }
    $0 == heading { in_section = 1; next }
    in_section {
      current_level = heading_level($0)
      if (current_level > 0 && current_level <= target_level) {
        exit
      }
      print
    }
  ' "$file_path"
}

assert_section_contains() {
  local file_path="$1"
  local section_heading="$2"
  local needle="$3"
  local message="$4"
  local section_text
  section_text="$(extract_section "$file_path" "$section_heading")"
  [[ -n "$section_text" ]] || fail "missing section: ${section_heading}"
  grep -Fq -- "$needle" <<<"$section_text" || fail "$message"
}

assert_not_contains() {
  local file_path="$1"
  local needle="$2"
  local message="$3"
  if grep -Fq -- "$needle" "$file_path"; then
    fail "$message"
  fi
}

assert_file tests/load/lib/realtime.js
assert_file tests/load/scenarios/realtime_ws_subscribe.js
assert_file tests/load/lib/auth.js
assert_file tests/load/lib/data.js
assert_file tests/load/lib/env.js
assert_file tests/load/README.md
assert_file tests/load/DESIGN.md
assert_file tests/test_load_harness.sh
assert_file Makefile
assert_file internal/ws/message.go
assert_file internal/realtime/ws_bridge.go
assert_file internal/ws/handler.go

assert_contains tests/load/lib/auth.js "export function allocateLoadUserIdentity" "auth helper should expose shared websocket identity allocation"
assert_contains tests/load/lib/auth.js "AYB_WS_USER_POOL_SIZE" "identity allocation should support optional pool-size env"
assert_contains tests/load/lib/auth.js "export function bootstrapNonMFASession" "auth helper should expose reusable non-MFA session bootstrap"
assert_contains tests/load/lib/auth.js "export function bootstrapTenantScopedSession" "auth helper should expose tenant-scoped session bootstrap for tenant-gated collection/realtime flows"
assert_contains tests/load/lib/auth.js "const TENANT_ADMIN_PATH = '/api/admin/tenants';" "auth helper should centralize tenant bootstrap endpoint contract"
assert_contains tests/load/lib/auth.js "tenant slug is already taken" "tenant bootstrap should recover from pooled slug races instead of aborting on 409"
assert_contains tests/load/lib/auth.js "page=1&perPage=100" "tenant bootstrap should reuse the admin tenant list to recover existing tenant ids after slug conflicts"
assert_contains tests/load/lib/auth.js "authRegisterURL" "non-MFA bootstrap should reuse shared auth URL helper"
assert_contains tests/load/lib/auth.js "authLoginURL" "non-MFA bootstrap should reuse shared auth URL helper"
assert_contains tests/load/lib/auth.js "buildRegisterBody" "non-MFA bootstrap should reuse shared auth request-body builders"
assert_contains tests/load/lib/auth.js "buildLoginBody" "non-MFA bootstrap should reuse shared auth request-body builders"
assert_contains tests/load/lib/auth.js "parseAuthSuccessResponse" "non-MFA bootstrap should reuse shared auth response parser"
assert_contains tests/load/lib/auth.js "export function buildAuthHeaders" "auth helper should expose a shared bearer-header builder"
assert_contains tests/load/lib/auth.js "export function authSessionHeaders" "auth helper should expose a shared session-header builder"
assert_contains tests/load/lib/auth.js 'Authorization: `Bearer ${token}`' "shared auth header builder should construct bearer authorization headers"
assert_contains tests/load/lib/auth.js "'X-Tenant-ID': tenantID" "shared auth header builder should include tenant scoping when available"
assert_contains tests/load/lib/auth.js "import { loadScenarioOptions, parsePositiveInt, readEnv, trimTrailingSlashes } from './env.js';" "auth helper should import env parsing and base-URL helpers from the shared env module"
assert_not_contains tests/load/lib/auth.js "function readEnv(" "auth helper should not re-implement env parsing"

assert_contains tests/load/lib/data.js "bootstrapTenantScopedSession(" "data helper should compose the shared tenant-scoped session bootstrap helper"
assert_contains tests/load/lib/data.js "allocateLoadUserIdentity(" "data helper should compose the shared identity-allocation helper"
assert_contains tests/load/lib/data.js "ENABLE ROW LEVEL SECURITY" "shared fixture helper should enable RLS so authenticated websocket users can observe collection events"
assert_contains tests/load/lib/data.js "CREATE ROLE ayb_authenticated NOLOGIN" "shared fixture helper should provision the authenticated database role expected by RLS-backed collection requests"
assert_contains tests/load/lib/data.js "GRANT USAGE ON SCHEMA public TO ayb_authenticated" "shared fixture helper should grant schema usage to the authenticated database role before applying table policies"
assert_contains tests/load/lib/data.js "GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE" "shared fixture helper should grant the authenticated database role access to the Stage 5 load table"
assert_contains tests/load/lib/data.js "CREATE POLICY load_fixture_select" "shared fixture helper should install a readable select policy for the Stage 5 load table"
assert_contains tests/load/lib/data.js "CREATE POLICY load_fixture_insert" "shared fixture helper should install an insert policy for authenticated load users"
assert_contains tests/load/lib/data.js "CREATE POLICY load_fixture_update" "shared fixture helper should install an update policy for authenticated load users"
assert_contains tests/load/lib/data.js "CREATE POLICY load_fixture_delete" "shared fixture helper should install a delete policy for authenticated load users"
assert_contains tests/load/lib/data.js "TO ayb_authenticated" "shared fixture helper policies should target the authenticated database role used by collection and realtime queries"

assert_contains tests/load/lib/realtime.js "const REALTIME_WS_PATH = '/api/realtime/ws';" "realtime helper should encode websocket endpoint contract"
assert_contains tests/load/lib/realtime.js "export function realtimeWSURL" "realtime helper should expose websocket URL builder"
assert_contains tests/load/lib/realtime.js "export function realtimeConnectParams" "realtime helper should expose header-auth websocket params builder"
assert_contains tests/load/lib/realtime.js "buildAuthHeaders(token, tenantID)" "realtime helper should delegate websocket auth headers to the shared auth header builder"
assert_contains tests/load/lib/realtime.js "export function buildRealtimeSubscribeMessage" "realtime helper should expose subscribe message builder"
assert_contains tests/load/lib/realtime.js "export function buildRealtimeUnsubscribeMessage" "realtime helper should expose unsubscribe message builder"
assert_contains tests/load/lib/realtime.js "type: 'subscribe'" "subscribe message builder should match websocket client message contract"
assert_contains tests/load/lib/realtime.js "type: 'unsubscribe'" "unsubscribe message builder should match websocket client message contract"
assert_contains tests/load/lib/realtime.js "export function assertRealtimeConnectedMessage" "realtime helper should assert the initial connected message contract"
assert_contains tests/load/lib/realtime.js "message.type !== 'connected'" "connected message assertion should enforce connected type"
assert_contains tests/load/lib/realtime.js "export function assertRealtimeReplyOK" "realtime helper should assert reply-ok contract"
assert_contains tests/load/lib/realtime.js "message.type !== 'reply'" "reply assertion should enforce reply type"
assert_contains tests/load/lib/realtime.js "message.status !== 'ok'" "reply assertion should enforce reply status"
assert_contains tests/load/lib/realtime.js "export function assertRealtimeEventMessage" "realtime helper should assert event payload contract"
assert_contains tests/load/lib/realtime.js "message.type !== 'event'" "event assertion should enforce event type"
assert_contains tests/load/lib/realtime.js "message.action" "event assertion should enforce action field"
assert_contains tests/load/lib/realtime.js "message.table" "event assertion should enforce table field"
assert_contains tests/load/lib/realtime.js "message.record" "event assertion should enforce record field"
assert_contains tests/load/lib/realtime.js "import { readEnv, trimTrailingSlashes } from './env.js';" "realtime helper should reuse shared env and base-URL helpers"
assert_not_contains tests/load/lib/realtime.js "function readEnv(" "realtime helper should not re-implement env parsing"
assert_contains tests/load/lib/realtime.js "export function runRealtimeSubscribeCreateEventUnsubscribeFlow" "realtime helper should expose reusable subscribe/create-event/unsubscribe flow runner for Stage 6 composition"
assert_contains tests/load/lib/realtime.js "requireRealtimeReadableSubscription(" "reusable realtime flow helper should enforce readable-subscription precondition before websocket wait loop"
assert_contains tests/load/lib/realtime.js "realtime auth probe requires the subscribed user to read" "reusable realtime flow helper should fail fast when the websocket user cannot read event rows"
assert_contains tests/load/lib/realtime.js "createResponse.status === 401 || createResponse.status === 403" "reusable realtime flow helper should retry collection writes only when the user create request is denied"
assert_contains tests/load/lib/realtime.js "dataCollectionListURL(" "reusable realtime flow helper should compose collection URL helper for auth probe and event create writes"
assert_contains tests/load/lib/realtime.js "buildRealtimeSubscribeMessage(" "reusable realtime flow helper should compose shared subscribe payload builder"
assert_contains tests/load/lib/realtime.js "buildRealtimeUnsubscribeMessage(" "reusable realtime flow helper should compose shared unsubscribe payload builder"
assert_contains tests/load/lib/realtime.js "assertRealtimeConnectedMessage(" "reusable realtime flow helper should enforce initial connected message contract"
assert_contains tests/load/lib/realtime.js "assertRealtimeReplyOK(" "reusable realtime flow helper should enforce subscribe/unsubscribe reply contracts"
assert_contains tests/load/lib/realtime.js "assertRealtimeEventMessage(" "reusable realtime flow helper should enforce event payload contract"
assert_contains tests/load/lib/realtime.js "realtimeMessageTimeoutMillis()" "reusable realtime flow helper should use shared websocket message timeout helper"

assert_contains tests/load/scenarios/realtime_ws_subscribe.js "import ws from 'k6/ws';" "realtime scenario should use k6 websocket client"
assert_contains tests/load/scenarios/realtime_ws_subscribe.js "import exec from 'k6/execution';" "realtime scenario should import k6 execution controls for fatal failures"
assert_contains tests/load/scenarios/realtime_ws_subscribe.js "loadDataRunTableName" "realtime scenario should reuse Stage 4 table-name helper"
assert_contains tests/load/scenarios/realtime_ws_subscribe.js "createDataFixture" "realtime scenario should reuse Stage 4 fixture setup helper"
assert_contains tests/load/scenarios/realtime_ws_subscribe.js "dropDataFixture" "realtime scenario should reuse Stage 4 fixture teardown helper"
assert_contains tests/load/scenarios/realtime_ws_subscribe.js "allocateLoadUserIdentity(__VU)" "realtime scenario should allocate websocket users via shared identity helper"
assert_contains tests/load/scenarios/realtime_ws_subscribe.js "bootstrapTenantScopedSession(" "realtime scenario should bootstrap tenant-scoped user sessions through shared auth helper"
assert_contains tests/load/scenarios/realtime_ws_subscribe.js "authSessionHeaders(authSession)" "realtime scenario should reuse shared auth-session headers for collection read/create probes"
assert_contains tests/load/scenarios/realtime_ws_subscribe.js "exec.test.abort" "realtime scenario should abort the test on fatal websocket/setup failures so smoke runs cannot false-pass"
assert_contains tests/load/scenarios/realtime_ws_subscribe.js "runRealtimeSubscribeCreateEventUnsubscribeFlow(" "realtime scenario should compose shared websocket flow helper"
assert_contains tests/load/scenarios/realtime_ws_subscribe.js "dataAdminRequestHeaders()" "realtime scenario should reuse shared admin headers for create fallback retries"
assert_contains tests/load/scenarios/realtime_ws_subscribe.js "realtime_data_create" "realtime scenario should tag collection writes separately from Stage 4 data traffic"
assert_contains tests/load/scenarios/realtime_ws_subscribe.js "realtime_ws_connect" "realtime scenario should tag websocket metrics separately from Stage 4 HTTP traffic"

assert_contains internal/ws/message.go "MsgTypeConnected = \"connected\"" "server websocket contract should define connected message type"
assert_contains internal/ws/message.go "MsgTypeReply     = \"reply\"" "server websocket contract should define reply message type"
assert_contains internal/ws/message.go "MsgTypeEvent     = \"event\"" "server websocket contract should define event message type"
assert_contains internal/ws/message.go "func connectedMsg(clientID string) ServerMessage {" "server websocket contract should define connected message helper"
assert_contains internal/ws/message.go "func replyOK(ref string) ServerMessage {" "server websocket contract should define reply-ok helper"
assert_contains internal/realtime/ws_bridge.go "func (b *WSBridge) forwardEvents(c *ws.Conn, client *Client) {" "realtime bridge should forward hub events through websocket"
assert_contains internal/ws/handler.go "token, ok := httputil.ExtractBearerToken(r)" "websocket handler should attempt header auth at upgrade"
assert_contains internal/ws/handler.go "token = r.URL.Query().Get(\"token\")" "websocket handler should retain query-token compatibility path"

assert_contains Makefile "LOAD_REALTIME_WS_SCENARIO := tests/load/scenarios/realtime_ws_subscribe.js" "makefile should define Stage 5 realtime websocket scenario script"
assert_contains Makefile "load-realtime-ws:" "makefile should expose direct realtime websocket target"
assert_contains Makefile "load-realtime-ws-local:" "makefile should expose local realtime websocket target"
assert_not_contains Makefile "\"type\":\"subscribe\"" "makefile should not duplicate websocket subscribe payload bodies"
assert_not_contains Makefile "/api/realtime/ws" "makefile should not duplicate websocket endpoint specifics"

assert_contains tests/test_load_harness.sh "make load-realtime-ws" "harness regression should validate direct Stage 5 realtime target"
assert_contains tests/test_load_harness.sh "make load-realtime-ws-local" "harness regression should validate local Stage 5 realtime target"
assert_contains tests/test_load_harness.sh "tests/load/scenarios/realtime_ws_subscribe.js" "harness regression should assert realtime target executes Stage 5 scenario"
assert_contains tests/test_load_harness.sh "realtime local target should enable auth for the started server" "harness regression should lock realtime-local auth enablement"
assert_contains tests/test_load_harness.sh "realtime local target should inject a non-empty, non-static jwt secret for the started server" "harness regression should lock realtime-local jwt env export"

assert_contains tests/load/README.md "make load-realtime-ws" "README should document direct Stage 5 realtime target"
assert_contains tests/load/README.md "make load-realtime-ws-local" "README should document local Stage 5 realtime target"
assert_contains tests/load/README.md "K6_VUS=1 K6_ITERATIONS=1 make load-realtime-ws-local" "README should document smallest realtime smoke command"
assert_section_contains tests/load/README.md "## Realtime WebSocket Scenario" "WebSocket-only" "README should document Stage 5 websocket-only scope"
assert_section_contains tests/load/README.md "## Realtime WebSocket Scenario" "SSE automation remains a follow-up gap" "README should document SSE automation deferral"
assert_section_contains tests/load/README.md "## Realtime WebSocket Scenario" 'Stage 7 measured smoke command: `K6_VUS=1 K6_ITERATIONS=1 make load-realtime-ws-local`' "README realtime section should pin the measured Stage 7 realtime smoke command"
assert_section_contains tests/load/README.md "## Realtime WebSocket Scenario" 'Stage 7 contract assertion: `bash tests/test_load_realtime_contract.sh`' "README realtime section should identify the guarding contract script"
assert_section_contains tests/load/README.md "## Realtime WebSocket Scenario" "Stage 7 caveat: websocket users must be able to read the subscribed table or the realtime flow fails fast before waiting for events." "README realtime section should preserve the Stage 7 table-read precondition caveat"

assert_contains tests/load/DESIGN.md "Stage 5 WebSocket scalability boundary" "design doc should document Stage 5 websocket scope"
assert_contains tests/load/DESIGN.md "GET /api/realtime/ws" "design doc should include Stage 5 websocket endpoint contract"
assert_contains tests/load/DESIGN.md "Authorization header at upgrade" "design doc should document websocket header-auth decision"
assert_contains tests/load/DESIGN.md "MaxConnectionsPerUser = 100" "design doc should include per-user websocket connection-cap rationale"
assert_contains tests/load/DESIGN.md "SSE automation gap remains explicit follow-up scope" "design doc should document Stage 1 SSE automation gap carry-forward"

echo "PASS: Stage 5 realtime websocket contract, helper boundaries, and documentation guardrails are wired"
