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

assert_file tests/load/README.md
assert_file tests/load/lib/env.js
assert_file tests/load/lib/admin.js
assert_file tests/load/lib/checks.js
assert_file tests/load/scenarios/admin_status.js

assert_contains tests/load/lib/admin.js "/api/admin/status" "admin helper must encode /api/admin/status contract"
assert_not_contains tests/load/scenarios/admin_status.js "/health" "baseline scenario must not measure /health"
assert_contains tests/load/scenarios/admin_status.js "adminStatusURL" "baseline scenario should resolve endpoint through shared admin helper"
assert_contains tests/load/scenarios/admin_status.js "assertAdminStatusResponse" "baseline scenario must use shared response-check helper"
assert_contains tests/load/lib/checks.js "auth" "shared checks should validate admin status auth field"
assert_contains tests/load/lib/env.js "__ENV" "env helper should read k6 environment"
assert_contains tests/load/README.md "make load-admin-status" "README should document direct baseline make target"
assert_contains tests/load/README.md "make load-admin-status-local" "README should document local baseline make target"
assert_section_contains tests/load/README.md "## Baseline Scenario" 'Stage 7 measured smoke command: `K6_VUS=1 K6_ITERATIONS=1 make load-admin-status-local`' "README baseline section should pin the measured Stage 7 baseline smoke command"
assert_section_contains tests/load/README.md "## Baseline Scenario" 'Stage 7 contract assertion: `bash tests/test_load_baseline_contract.sh`' "README baseline section should identify the guarding contract script"
assert_section_contains tests/load/README.md "## Baseline Scenario" "Stage 7 caveat: readiness stays runner-only and is not included in measured request latency." "README baseline section should preserve the readiness-only caveat from Stage 7 findings"

echo "PASS: baseline scenario contract and shared load scaffold are present"
