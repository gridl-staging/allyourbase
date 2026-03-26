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

assert_file tests/load/lib/auth.js
assert_file tests/load/lib/data.js
assert_file tests/load/lib/realtime.js
assert_file tests/load/lib/env.js
assert_file tests/load/scenarios/sustained_soak.js
assert_file tests/load/README.md
assert_file _dev/performance_baseline.md
assert_file _dev/FEATURES.md
assert_file _dev/PHASES.md

assert_contains tests/load/lib/auth.js "export function runAuthRegisterLoginRefreshFlow" "auth helper should expose reusable register/login/refresh flow helper for soak composition"
assert_contains tests/load/lib/data.js "export function runDataPathCRUDAndBatchFlow" "data helper should expose reusable CRUD/batch flow helper for soak composition"
assert_contains tests/load/lib/realtime.js "export function runRealtimeSubscribeCreateEventUnsubscribeFlow" "realtime helper should expose reusable subscribe/create-event/unsubscribe flow helper for soak composition"
assert_contains tests/load/lib/env.js "AYB_SOAK_DURATION" "env helper should read sustained-soak duration override from one shared options path"
assert_contains tests/load/lib/data.js "function resolveDataFlowEndpointTags(endpointTags = {})" "data flow helper should centralize endpoint-tag resolution in a shared step helper"
assert_contains tests/load/lib/data.js "function runDataCRUDBatchFlowSteps(" "data flow helper should compose extracted CRUD/batch step helpers"
assert_contains tests/load/lib/data.js "function runDataBatchRollbackProbeStep(" "data flow helper should isolate rollback probe logic in a dedicated helper"
assert_contains tests/load/lib/data.js "const resolvedTags = resolveDataFlowEndpointTags(endpointTags);" "data flow runner should call the shared endpoint-tag resolver"
assert_contains tests/load/lib/data.js "runDataCRUDBatchFlowSteps({" "data flow runner should delegate HTTP step execution to extracted helper composition"
assert_contains tests/load/lib/realtime.js "function createRealtimeFlowState(" "realtime flow helper should isolate websocket message state initialization"
assert_contains tests/load/lib/realtime.js "function processRealtimeSocketMessage(" "realtime flow helper should isolate websocket message handling state transitions"
assert_contains tests/load/lib/realtime.js "function runRealtimeSocketFlow(" "realtime flow helper should isolate websocket connect/timeout/message wiring"
assert_contains tests/load/lib/realtime.js "const flowState = createRealtimeFlowState();" "realtime flow runner should initialize websocket flow state through shared helper"
assert_contains tests/load/lib/realtime.js "runRealtimeSocketFlow({" "realtime flow runner should delegate websocket lifecycle orchestration to an extracted helper"

assert_contains tests/load/scenarios/sustained_soak.js "runAuthRegisterLoginRefreshFlow(" "sustained soak scenario should compose shared auth flow helper"
assert_contains tests/load/scenarios/sustained_soak.js "runDataPathCRUDAndBatchFlow(" "sustained soak scenario should compose shared data flow helper"
assert_contains tests/load/scenarios/sustained_soak.js "runRealtimeSubscribeCreateEventUnsubscribeFlow(" "sustained soak scenario should compose shared realtime flow helper"
assert_contains tests/load/scenarios/sustained_soak.js "loadDataRunTableName" "sustained soak scenario should reuse Stage 4 load table naming helper"
assert_contains tests/load/scenarios/sustained_soak.js "createDataFixture" "sustained soak scenario should reuse Stage 4 fixture setup helper"
assert_contains tests/load/scenarios/sustained_soak.js "dropDataFixture" "sustained soak scenario should reuse Stage 4 fixture teardown helper"
assert_contains tests/load/scenarios/sustained_soak.js "allocateLoadUserIdentity(__VU)" "sustained soak scenario should allocate per-vu identities via shared helper"
assert_contains tests/load/scenarios/sustained_soak.js "bootstrapTenantScopedSession(" "sustained soak scenario should bootstrap tenant-scoped sessions via shared helper"
assert_contains tests/load/scenarios/sustained_soak.js "DEFAULT_POOLED_SESSION_MAX_AGE_MS = 10 * 60 * 1000" "sustained soak scenario should periodically refresh pooled sessions before default JWT expiry"
assert_contains tests/load/scenarios/sustained_soak.js "isReusablePooledSession(cachedSessionEntry, nowMillis)" "sustained soak scenario should gate pooled-session reuse by bounded age"

assert_not_contains tests/load/scenarios/sustained_soak.js "/api/auth/" "sustained soak scenario should not inline auth endpoint contracts"
assert_not_contains tests/load/scenarios/sustained_soak.js "/api/collections/" "sustained soak scenario should not inline collection endpoint contracts"
assert_not_contains tests/load/scenarios/sustained_soak.js "/api/realtime/ws" "sustained soak scenario should not inline websocket endpoint contracts"
assert_not_contains tests/load/scenarios/sustained_soak.js "CREATE TABLE" "sustained soak scenario should not inline fixture DDL"
assert_not_contains tests/load/scenarios/sustained_soak.js "DROP TABLE" "sustained soak scenario should not inline fixture teardown DDL"
assert_not_contains tests/load/scenarios/sustained_soak.js "type: 'subscribe'" "sustained soak scenario should not inline websocket subscribe payloads"
assert_not_contains tests/load/scenarios/sustained_soak.js "type: 'unsubscribe'" "sustained soak scenario should not inline websocket unsubscribe payloads"
assert_not_contains tests/load/scenarios/sustained_soak.js "jsonRequestOptions(" "sustained soak scenario should not re-declare per-endpoint HTTP option builders"
assert_not_contains tests/load/scenarios/sustained_soak.js "__ITER % 10" "sustained soak scenario should not skip auth flow on most iterations"
assert_section_contains tests/load/README.md "## Sustained Mixed-Workload Soak Scenario" 'Stage 7 measured smoke command: `AYB_SOAK_DURATION=30s K6_VUS=1 make load-sustained-soak-local`' "README sustained-soak section should pin the measured Stage 7 soak smoke command"
assert_section_contains tests/load/README.md "## Sustained Mixed-Workload Soak Scenario" 'Stage 7 contract assertion: `bash tests/test_load_soak_contract.sh`' "README sustained-soak section should identify the guarding contract script"
assert_section_contains tests/load/README.md "## Sustained Mixed-Workload Soak Scenario" "Stage 7 caveat: the 30s smoke run confirms mixed-flow wiring but does not cross the 10-minute pooled-session age rollover boundary." "README sustained-soak section should preserve the Stage 7 pooled-session age caveat"
assert_section_contains _dev/performance_baseline.md "## Stage 7 Load Harness (k6)" '| `sustained_soak` | `AYB_SOAK_DURATION=30s K6_VUS=1 make load-sustained-soak-local` |' "performance baseline Stage 7 section should pin the measured sustained-soak smoke command"
assert_section_contains _dev/performance_baseline.md "## Stage 7 Load Harness (k6)" '| `admin_status` | `K6_VUS=1 K6_ITERATIONS=1 make load-admin-status-local` |' "performance baseline Stage 7 section should pin the measured baseline smoke command"
assert_section_contains _dev/performance_baseline.md "## Stage 7 Load Harness (k6)" '| `auth_register_login_refresh` | `K6_VUS=1 K6_ITERATIONS=1 make load-auth-request-path-local` |' "performance baseline Stage 7 section should pin the measured auth smoke command"
assert_section_contains _dev/performance_baseline.md "## Stage 7 Load Harness (k6)" '| `data_path_crud_batch` | `K6_VUS=1 K6_ITERATIONS=1 make load-data-path-local` |' "performance baseline Stage 7 section should pin the measured data-path smoke command"
assert_section_contains _dev/performance_baseline.md "## Stage 7 Load Harness (k6)" '| `data_pool_pressure` | `K6_VUS=2 K6_ITERATIONS=2 make load-data-pool-pressure-local` |' "performance baseline Stage 7 section should pin the measured pool-pressure smoke command"
assert_section_contains _dev/performance_baseline.md "## Stage 7 Load Harness (k6)" '| `realtime_ws_subscribe` | `K6_VUS=1 K6_ITERATIONS=1 make load-realtime-ws-local` |' "performance baseline Stage 7 section should pin the measured realtime smoke command"
assert_section_contains _dev/performance_baseline.md "## Stage 7 Load Harness (k6)" "Stage 1 decision remains in force: realtime load automation is WebSocket-only; SSE automation is still follow-up scope." "performance baseline Stage 7 section should preserve the WebSocket-only realtime scope gap"
assert_section_contains _dev/performance_baseline.md "## Stage 7 Load Harness (k6)" "The measured 30s soak run validates mixed-flow wiring but does not cross the 10-minute pooled-session age rollover boundary." "performance baseline Stage 7 section should preserve the pooled-session rollover caveat"
assert_section_contains _dev/performance_baseline.md "## Stage 7 Load Harness (k6)" "### Scale-Tier Results" "performance baseline Stage 7 section should include a dedicated Scale-Tier Results subsection"
assert_section_contains _dev/performance_baseline.md "## Stage 7 Load Harness (k6)" '`make load-http-100`' "performance baseline Stage 7 section should log the load-http-100 tier command attempt"
assert_section_contains _dev/performance_baseline.md "## Stage 7 Load Harness (k6)" '`make load-http-500`' "performance baseline Stage 7 section should log the load-http-500 tier command attempt"
assert_section_contains _dev/performance_baseline.md "## Stage 7 Load Harness (k6)" '`make load-http-1000`' "performance baseline Stage 7 section should log the load-http-1000 tier command attempt"
assert_section_contains _dev/performance_baseline.md "## Stage 7 Load Harness (k6)" '`make load-realtime-ws-1000`' "performance baseline Stage 7 section should log the realtime ws 1000 tier command attempt"
assert_section_contains _dev/performance_baseline.md "## Stage 7 Load Harness (k6)" '`make load-realtime-ws-5000`' "performance baseline Stage 7 section should log the realtime ws 5000 tier command attempt"
assert_section_contains _dev/performance_baseline.md "## Stage 7 Load Harness (k6)" '`make load-realtime-ws-10000`' "performance baseline Stage 7 section should log the realtime ws 10000 tier command attempt"
assert_section_contains _dev/FEATURES.md '### Testing Infrastructure Gaps (Phase 5 — see `_dev/PHASES.md`)' 'Load/stress harness has smoke coverage plus scale-tier aliases (`load-http-{100,500,1000}`, `load-realtime-ws-{1000,5000,10000}`); command-by-command evidence (including deferred blockers) is centralized in `_dev/performance_baseline.md` (`## Stage 7 Load Harness (k6)` → `### Scale-Tier Results`).' "features tracking should summarize the shipped tier alias coverage while pointing to the canonical baseline evidence log"
assert_section_contains _dev/FEATURES.md '### Testing Infrastructure Gaps (Phase 5 — see `_dev/PHASES.md`)' "Realtime load automation remains WebSocket-only; SSE automation is still deferred scope." "features tracking should preserve websocket-only realtime coverage and SSE deferral in summary form"
assert_section_contains _dev/FEATURES.md '### Testing Infrastructure Gaps (Phase 5 — see `_dev/PHASES.md`)' "Remaining load gaps are successful multi-hour soak windows and proving pooled-session rollover behavior beyond the 10-minute age boundary." "features tracking should preserve long-soak and pooled-session rollover gaps in summary form"
assert_not_contains _dev/FEATURES.md '`make load-http-100`' "features tracking should not duplicate per-tier command entries outside the canonical baseline log"
assert_not_contains _dev/FEATURES.md '`make load-http-500`' "features tracking should not duplicate per-tier command entries outside the canonical baseline log"
assert_not_contains _dev/FEATURES.md '`make load-http-1000`' "features tracking should not duplicate per-tier command entries outside the canonical baseline log"
assert_not_contains _dev/FEATURES.md '`make load-realtime-ws-1000`' "features tracking should not duplicate per-tier command entries outside the canonical baseline log"
assert_not_contains _dev/FEATURES.md '`make load-realtime-ws-5000`' "features tracking should not duplicate per-tier command entries outside the canonical baseline log"
assert_not_contains _dev/FEATURES.md '`make load-realtime-ws-10000`' "features tracking should not duplicate per-tier command entries outside the canonical baseline log"
assert_section_contains _dev/PHASES.md "### Production Confidence — Testing Infrastructure (Priority Order)" '**Load & stress testing suite (baseline + tier aliases)** — smoke k6 coverage is shipped; explicit tier entry points are wired (`load-http-{100,500,1000}` and `load-realtime-ws-{1000,5000,10000}`); canonical command outcomes and blockers are tracked only in `_dev/performance_baseline.md` (`## Stage 7 Load Harness (k6)` → `### Scale-Tier Results`).' "phase tracking should summarize baseline-plus-tier status and point to the canonical baseline evidence log"
assert_section_contains _dev/PHASES.md "### Production Confidence — Testing Infrastructure (Priority Order)" "Realtime coverage in this lane remains WebSocket-only and SSE automation stays deferred." "phase tracking should preserve websocket-only realtime scope and SSE deferral in summary form"
assert_section_contains _dev/PHASES.md "### Production Confidence — Testing Infrastructure (Priority Order)" "Remaining load confidence work is successful multi-hour soak execution and pooled-session rollover validation beyond 10 minutes." "phase tracking should preserve long-soak and pooled-session rollover gaps in summary form"
assert_not_contains _dev/PHASES.md '`make load-http-100`' "phase tracking should not duplicate per-tier command entries outside the canonical baseline log"
assert_not_contains _dev/PHASES.md '`make load-http-500`' "phase tracking should not duplicate per-tier command entries outside the canonical baseline log"
assert_not_contains _dev/PHASES.md '`make load-http-1000`' "phase tracking should not duplicate per-tier command entries outside the canonical baseline log"
assert_not_contains _dev/PHASES.md '`make load-realtime-ws-1000`' "phase tracking should not duplicate per-tier command entries outside the canonical baseline log"
assert_not_contains _dev/PHASES.md '`make load-realtime-ws-5000`' "phase tracking should not duplicate per-tier command entries outside the canonical baseline log"
assert_not_contains _dev/PHASES.md '`make load-realtime-ws-10000`' "phase tracking should not duplicate per-tier command entries outside the canonical baseline log"

echo "PASS: Stage 6 soak composition and Stage 7 tracking-doc guardrails are wired"
