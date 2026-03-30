#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/bash_assert_helpers.sh"

makefile_path="Makefile"
[[ -f "$makefile_path" ]] || fail "missing ${makefile_path}"

assert_contains "$makefile_path" 'test-api-journey' "Makefile should define a dedicated test-api-journey target and phony entry"
assert_contains "$makefile_path" 'test-api-journey: build' "test-api-journey should depend on build"
assert_contains "$makefile_path" "@AYB_STORAGE_ENABLED=true bash scripts/run-with-ayb.sh 'cd _dev/manual_smoke_tests && python3 full_journey.test.py'" "test-api-journey should enable storage and run full_journey through run-with-ayb"

assert_contains "$makefile_path" 'test-api-smoke: build' "Makefile should keep test-api-smoke target"
assert_contains "$makefile_path" "@AYB_STORAGE_ENABLED=true bash scripts/run-with-ayb.sh 'cd _dev/manual_smoke_tests && ./run_all_tests.sh'" "test-api-smoke should enable storage and run through run-with-ayb"
assert_not_contains "$makefile_path" '@./ayb start; \\' "test-api-smoke should not open-code ayb startup"
assert_not_contains "$makefile_path" 'run_step "API smoke tests"    "./ayb start; cd _dev/manual_smoke_tests && ./run_all_tests.sh; R=\$$?; cd ../.. && ./ayb stop 2>/dev/null || true; exit \$$R"; \\' "test-everything should not inline duplicate api-smoke lifecycle"
assert_not_contains "$makefile_path" 'run_step "Playwright e2e"     "bash scripts/run-with-ayb.sh '\''cd ui && npm run test:browser'\''"; \\' "test-everything should not inline a second Playwright lifecycle"

assert_contains "$makefile_path" 'run_step "API smoke tests"    "$(MAKE) test-api-smoke"' "test-everything should delegate API smoke coverage to make test-api-smoke"
assert_contains "$makefile_path" 'run_step "Playwright e2e"     "$(MAKE) test-e2e"' "test-everything should delegate Playwright coverage to make test-e2e"
assert_contains "$makefile_path" 'load_base_url_is_loopback() {' "load bootstrap should expose a loopback check helper before reusing local admin secrets"
assert_contains "$makefile_path" 'AYB_ADMIN_TOKEN must be set for non-loopback AYB_BASE_URL; refusing ~/.ayb/admin-token fallback' "remote load targets should refuse implicit ~/.ayb/admin-token password fallback"

echo "PASS: Makefile smoke targets delegate through canonical make targets"
