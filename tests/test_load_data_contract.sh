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

assert_file tests/load/lib/data.js
assert_file tests/load/lib/auth.js
assert_file tests/load/scenarios/data_path_crud_batch.js
assert_file tests/load/scenarios/data_pool_pressure.js
assert_file tests/load/README.md
assert_file tests/load/DESIGN.md
assert_file Makefile

assert_file internal/api/handler.go
assert_file internal/api/handler_list.go
assert_file internal/api/handler_crud.go
assert_file internal/api/batch.go
assert_file internal/api/integration_test.go
assert_file internal/api/benchmark_test.go
assert_file internal/server/routes_admin.go
assert_file internal/server/sql_handler.go

assert_contains internal/api/handler.go 'r.Route("/collections/{table}", func(r chi.Router) {' "API handler should keep collection CRUD under /api/collections/{table}"
assert_contains internal/api/handler.go 'r.Post("/batch", h.handleBatch)' "API handler should keep collection batch under POST /api/collections/{table}/batch"
assert_contains internal/server/routes_admin.go 'r.Route("/admin/sql", func(r chi.Router) {' "server routes should expose admin SQL under /api/admin/sql/"
assert_contains internal/server/routes_admin.go 'r.Post("/", handleAdminSQL(s.pool, s.schema))' "admin SQL route should continue using handleAdminSQL"
assert_contains internal/server/sql_handler.go "type sqlRequest struct" "admin SQL handler should lock request contract struct"
assert_contains internal/server/sql_handler.go 'Query string `json:"query"`' "admin SQL handler should require JSON {query}"

assert_contains internal/api/handler_list.go "writeJSON(w, http.StatusOK, ListResponse{" "collection list should return HTTP 200"
assert_contains internal/api/handler_crud.go "writeJSON(w, http.StatusCreated, record)" "collection create should return HTTP 201"
assert_contains internal/api/handler_crud.go "writeJSON(w, http.StatusOK, record)" "collection read/update should return HTTP 200"
assert_contains internal/api/handler_crud.go "w.WriteHeader(http.StatusNoContent)" "collection delete should return HTTP 204"
assert_contains internal/api/batch.go "writeJSON(w, http.StatusOK, results)" "collection batch should return HTTP 200"
assert_contains internal/api/integration_test.go "func TestBatchCreatePartialFailureRollback(t *testing.T) {" "integration tests should lock batch create rollback behavior"
assert_contains internal/api/integration_test.go "func TestBatchUpdatePartialFailureRollback(t *testing.T) {" "integration tests should lock batch update rollback behavior"
assert_contains internal/api/integration_test.go "func TestBatchNotFoundRollsBack(t *testing.T) {" "integration tests should lock batch rollback on not-found mutation"
assert_contains internal/api/benchmark_test.go '"/api/collections/bench_items/batch"' "benchmarks should keep collection batch endpoint contract coverage"

assert_contains tests/load/lib/data.js "import {" "Stage 4 data helper should import shared auth helper exports"
assert_contains tests/load/lib/data.js "bootstrapTenantScopedSession(" "Stage 4 data helper should reuse shared tenant-scoped auth bootstrap helper"
assert_contains tests/load/lib/data.js "allocateLoadUserIdentity(" "Stage 4 data helper should reuse shared identity allocation helper"
assert_contains tests/load/lib/data.js "export function loadDataRunTableName" "Stage 4 data helper should expose per-run table-name generation"
assert_contains tests/load/lib/data.js "const TABLE_IDENTIFIER_MAX_LENGTH = 63;" "Stage 4 data helper should cap generated table identifiers to PostgreSQL length limits"
assert_contains tests/load/lib/data.js "function buildLoadDataTableName(tablePrefix)" "Stage 4 data helper should keep table-name truncation logic in one shared helper"
assert_contains tests/load/lib/data.js "const maxPrefixLength = Math.max(1, TABLE_IDENTIFIER_MAX_LENGTH - suffix.length);" "Stage 4 data helper should preserve run nonce even when prefixes are long"
assert_contains tests/load/lib/data.js "export function dataAdminSQLURL" "Stage 4 data helper should expose admin SQL URL builder"
assert_contains tests/load/lib/data.js "export function createDataFixture" "Stage 4 data helper should expose admin-SQL fixture setup"
assert_contains tests/load/lib/data.js "export function dropDataFixture" "Stage 4 data helper should expose admin-SQL fixture teardown"
assert_contains tests/load/lib/data.js "export function bootstrapDataUserHeaders" "Stage 4 data helper should expose authenticated request header bootstrap"
assert_contains tests/load/lib/data.js "export function runDataPathCRUDAndBatchFlow" "Stage 6 data helper should expose reusable collection CRUD/batch flow runner"
assert_contains tests/load/lib/data.js "'data_list'" "shared data flow helper should tag list endpoint separately"
assert_contains tests/load/lib/data.js "'data_create'" "shared data flow helper should tag create endpoint separately"
assert_contains tests/load/lib/data.js "'data_read'" "shared data flow helper should tag read endpoint separately"
assert_contains tests/load/lib/data.js "'data_update'" "shared data flow helper should tag update endpoint separately"
assert_contains tests/load/lib/data.js "'data_batch'" "shared data flow helper should tag batch endpoint separately"
assert_contains tests/load/lib/data.js "'data_delete'" "shared data flow helper should tag delete endpoint separately"
assert_contains tests/load/lib/data.js "res.status === 200" "shared data flow helper should assert HTTP 200 for list/read/update/batch"
assert_contains tests/load/lib/data.js "res.status === 201" "shared data flow helper should assert HTTP 201 for create"
assert_contains tests/load/lib/data.js "res.status === 204" "shared data flow helper should assert HTTP 204 for delete"
assert_contains tests/load/lib/data.js "rollback probe rejects partial commit" "shared data flow helper should assert failed batch mutations roll back"
assert_contains tests/load/lib/data.js "authSessionHeaders(loginAuth)" "Stage 4 data helper should reuse the shared auth-session header builder"
assert_contains tests/load/lib/data.js "import { readEnv, trimTrailingSlashes } from './env.js';" "Stage 4 data helper should reuse shared env and base-URL helpers"
assert_not_contains tests/load/lib/data.js "function readEnv(" "Stage 4 data helper should not duplicate env parsing helpers"
assert_contains tests/load/lib/data.js "/api/admin/sql/" "Stage 4 data helper should encode /api/admin/sql/ endpoint contract"
assert_contains tests/load/lib/data.js "/api/collections/" "Stage 4 data helper should encode /api/collections/{table}/ endpoint contract"
assert_contains tests/load/lib/data.js "/batch" "Stage 4 data helper should encode /api/collections/{table}/batch endpoint contract"

assert_contains tests/load/scenarios/data_path_crud_batch.js "loadDataRunTableName" "data-path scenario should use shared per-run table-name helper"
assert_contains tests/load/scenarios/data_path_crud_batch.js "createDataFixture" "data-path scenario should use shared admin-SQL fixture setup helper"
assert_contains tests/load/scenarios/data_path_crud_batch.js "dropDataFixture" "data-path scenario should use shared admin-SQL fixture teardown helper"
assert_contains tests/load/scenarios/data_path_crud_batch.js "bootstrapDataUserHeaders" "data-path scenario should use shared auth bootstrap helper"
assert_contains tests/load/scenarios/data_path_crud_batch.js "dataCollectionListURL" "data-path scenario should build collection URLs from shared helper"
assert_contains tests/load/scenarios/data_path_crud_batch.js "runDataPathCRUDAndBatchFlow(" "data-path scenario should compose the shared data flow helper"

assert_contains tests/load/scenarios/data_pool_pressure.js "runAdminSQL" "pool-pressure scenario should target admin SQL endpoint through shared helper"
assert_contains tests/load/scenarios/data_pool_pressure.js "SELECT pg_sleep(2)" "pool-pressure scenario should issue SELECT pg_sleep(2)"
assert_contains tests/load/scenarios/data_pool_pressure.js "'admin_sql_pool_pressure'" "pool-pressure scenario should tag admin SQL pressure traffic separately"
assert_contains tests/load/scenarios/data_pool_pressure.js "createDataFixture" "pool-pressure scenario should reuse shared fixture setup helper"
assert_contains tests/load/scenarios/data_pool_pressure.js "dropDataFixture" "pool-pressure scenario should reuse shared fixture teardown helper"
assert_contains tests/load/scenarios/data_pool_pressure.js "import { loadBaseURL, loadScenarioOptions, parsePositiveInt, readEnv } from '../lib/env.js';" "pool-pressure scenario should reuse shared env helper functions"
assert_not_contains tests/load/scenarios/data_pool_pressure.js "function readEnv(" "pool-pressure scenario should not duplicate env parsing helper"
assert_not_contains tests/load/scenarios/data_pool_pressure.js "function parsePositiveInt(" "pool-pressure scenario should not duplicate integer parsing helper"
assert_contains tests/load/scenarios/data_pool_pressure.js "requireStatus: null" "pool-pressure scenario should allow non-200 responses so saturation error-rate metrics are measurable"

assert_contains Makefile "LOAD_DATA_PATH_SCENARIO := tests/load/scenarios/data_path_crud_batch.js" "makefile should define data-path scenario script"
assert_contains Makefile "LOAD_DATA_POOL_PRESSURE_SCENARIO := tests/load/scenarios/data_pool_pressure.js" "makefile should define pool-pressure scenario script"
assert_contains Makefile "load-data-path:" "makefile should expose direct data-path target"
assert_contains Makefile "load-data-path-local:" "makefile should expose local data-path target"
assert_contains Makefile "load-data-pool-pressure:" "makefile should expose direct pool-pressure target"
assert_contains Makefile "load-data-pool-pressure-local:" "makefile should expose local pool-pressure target"
assert_not_contains Makefile "SELECT pg_sleep(2)" "makefile should not duplicate admin SQL pressure query bodies"
assert_not_contains Makefile "CREATE TABLE" "makefile should not embed Stage 4 fixture DDL bodies"
assert_not_contains Makefile "DROP TABLE" "makefile should not embed Stage 4 fixture teardown DDL bodies"

assert_contains tests/load/README.md "make load-data-path" "README should document direct Stage 4 data-path target"
assert_contains tests/load/README.md "make load-data-path-local" "README should document local Stage 4 data-path target"
assert_contains tests/load/README.md "make load-data-pool-pressure" "README should document direct Stage 4 pool-pressure target"
assert_contains tests/load/README.md "make load-data-pool-pressure-local" "README should document local Stage 4 pool-pressure target"
assert_contains tests/load/README.md "K6_VUS=1 K6_ITERATIONS=1 make load-data-path-local" "README should document smallest Stage 4 data-path smoke command"
assert_contains tests/load/README.md "K6_VUS=2 K6_ITERATIONS=2 make load-data-pool-pressure-local" "README should document smallest measured Stage 4 pool-pressure smoke command"
assert_section_contains tests/load/README.md "## Data-Path CRUD/Batch Scenario" 'Stage 7 measured smoke command: `K6_VUS=1 K6_ITERATIONS=1 make load-data-path-local`' "README data-path section should pin the measured Stage 7 data-path smoke command"
assert_section_contains tests/load/README.md "## Data-Path CRUD/Batch Scenario" 'Stage 7 contract assertion: `bash tests/test_load_data_contract.sh`' "README data-path section should identify the guarding Stage 4 contract script"
assert_section_contains tests/load/README.md "## Data-Path CRUD/Batch Scenario" "Stage 7 caveat: the rollback probe intentionally submits one invalid batch mutation and expects zero partial writes." "README data-path section should preserve the Stage 7 rollback caveat"
assert_section_contains tests/load/README.md "## Data Pool-Pressure Scenario" 'Stage 7 measured smoke command: `K6_VUS=2 K6_ITERATIONS=2 make load-data-pool-pressure-local`' "README pool-pressure section should pin the measured Stage 7 pool-pressure smoke command"
assert_section_contains tests/load/README.md "## Data Pool-Pressure Scenario" 'Stage 7 contract assertion: `bash tests/test_load_data_contract.sh`' "README pool-pressure section should identify the guarding Stage 4 contract script"
assert_section_contains tests/load/README.md "## Data Pool-Pressure Scenario" 'Stage 7 caveat: `http_req_duration` p95 is expected near `2s` because the measured pressure query is `SELECT pg_sleep(2)`.' "README pool-pressure section should preserve the expected Stage 7 latency caveat"

assert_contains tests/load/DESIGN.md "Stage 4" "design doc should record Stage 4 harness scope"
assert_contains tests/load/DESIGN.md "/api/collections/{table}" "design doc should include collection endpoint contract"
assert_contains tests/load/DESIGN.md "/api/admin/sql/" "design doc should include admin SQL pressure endpoint contract"
assert_contains tests/load/DESIGN.md "SELECT pg_sleep(2)" "design doc should include Stage 4 pool-pressure query contract"

assert_contains tests/test_load_harness.sh "make load-data-path" "harness regression should validate direct data-path make target"
assert_contains tests/test_load_harness.sh "make load-data-path-local" "harness regression should validate local data-path make target"
assert_contains tests/test_load_harness.sh "make load-data-pool-pressure" "harness regression should validate direct pool-pressure make target"
assert_contains tests/test_load_harness.sh "make load-data-pool-pressure-local" "harness regression should validate local pool-pressure make target"
assert_contains tests/test_load_harness.sh "data-path local target should enable auth for the started server" "harness regression should lock data-path local auth enablement env export"
assert_contains tests/test_load_harness.sh "data-path local target should inject a non-empty, non-static jwt secret for the started server" "harness regression should lock data-path local jwt env export"

echo "PASS: Stage 4 data-path and pool-pressure contract/harness boundaries are wired"
