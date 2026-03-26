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

assert_file tests/load/lib/env.js
assert_file tests/load/lib/checks.js
assert_file tests/load/lib/auth.js
assert_file tests/load/scenarios/admin_status.js
assert_file tests/load/scenarios/auth_register_login_refresh.js
assert_file tests/load/README.md
assert_file Makefile
assert_file internal/auth/handler.go
assert_file internal/auth/auth_login.go
assert_file internal/auth/auth_integration_test.go

assert_contains tests/load/lib/env.js "export function loadScenarioOptions" "env helper should expose a reusable scenario option builder"
assert_contains tests/load/lib/env.js "endpointThresholds" "scenario options should accept endpoint threshold configuration"
assert_contains tests/load/lib/env.js "function toDurationThresholds" "env helper should keep endpoint threshold key formatting centralized"
assert_contains tests/load/lib/env.js "return loadScenarioOptions({" "loadCommonOptions should compose via shared scenario option builder"

assert_contains tests/load/lib/checks.js "export function parseJSONResponse" "checks helper should expose shared JSON parsing"
assert_contains tests/load/lib/checks.js "export function assertResponseChecks" "checks helper should expose shared response-check wrapper"
assert_contains tests/load/lib/checks.js "return assertResponseChecks(response" "admin status check should compose shared response-check wrapper"

assert_contains tests/load/scenarios/admin_status.js "loadCommonOptions('admin_status_baseline')" "admin status scenario should preserve baseline option identity"
assert_contains tests/load/scenarios/admin_status.js "endpoint: 'admin_status'" "admin status scenario should continue tagging admin_status endpoint"
assert_contains tests/load/scenarios/admin_status.js "assertAdminStatusResponse(response)" "admin status scenario should continue delegating response checks"

assert_contains tests/load/lib/auth.js "export function buildAuthScenarioOptions" "auth helper should expose auth scenario option builder"
assert_contains tests/load/lib/auth.js "export function buildRegisterBody" "auth helper should expose register request-body builder"
assert_contains tests/load/lib/auth.js "export function buildLoginBody" "auth helper should expose login request-body builder"
assert_contains tests/load/lib/auth.js "export function buildRefreshBody" "auth helper should expose refresh request-body builder"
assert_contains tests/load/lib/auth.js "export function parseAuthSuccessResponse" "auth helper should expose non-MFA auth response parser"
assert_contains tests/load/lib/auth.js "export function uniqueAuthIdentity" "auth helper should expose unique per-vu/per-iteration identity generation"
assert_contains tests/load/lib/auth.js "export function runAuthRegisterLoginRefreshFlow" "auth helper should expose reusable register/login/refresh flow runner for Stage 6 composition"
assert_contains tests/load/lib/auth.js "'auth_register'" "auth flow helper should tag register endpoint separately"
assert_contains tests/load/lib/auth.js "'auth_login'" "auth flow helper should tag login endpoint separately"
assert_contains tests/load/lib/auth.js "'auth_refresh'" "auth flow helper should tag refresh endpoint separately"
assert_contains tests/load/lib/auth.js "'auth_refresh_reuse'" "auth flow helper should tag refresh-token-reuse check separately"
assert_contains tests/load/lib/auth.js "res.status === 201" "auth flow helper should assert HTTP 201 for register"
assert_contains tests/load/lib/auth.js "res.status === 200" "auth flow helper should assert HTTP 200 for login and refresh"
assert_contains tests/load/lib/auth.js "res.status === 401" "auth flow helper should assert HTTP 401 for invalid credentials and token reuse"
assert_contains tests/load/lib/auth.js "extraExpectedStatuses.includes(res.status)" "auth flow helper should allow tolerant-mode checks to accept configured extra expected statuses"
assert_contains tests/load/lib/auth.js "rotated refresh token differs from consumed refresh token" "auth flow helper should assert refresh rotation"

assert_contains tests/load/scenarios/auth_register_login_refresh.js "runAuthRegisterLoginRefreshFlow(" "auth scenario should compose the shared register/login/refresh flow helper"

assert_contains internal/auth/handler.go "type authRequest struct" "auth handler should define email/password request contract"
assert_contains internal/auth/handler.go 'Email    string `json:"email"`' "auth request contract should include email field"
assert_contains internal/auth/handler.go 'Password string `json:"password"`' "auth request contract should include password field"
assert_contains internal/auth/handler.go "type refreshRequest struct" "auth handler should define refresh request contract"
assert_contains internal/auth/handler.go 'RefreshToken string `json:"refreshToken"`' "refresh request contract should include refreshToken field"
assert_contains internal/auth/handler.go "httputil.WriteJSON(w, http.StatusCreated, authResponse{Token: token, RefreshToken: refreshToken, User: user})" "register should return HTTP 201 auth response"
assert_contains internal/auth/handler.go "httputil.WriteJSON(w, http.StatusOK, authResponse{Token: token, RefreshToken: refreshToken, User: user})" "login should return HTTP 200 auth response"
assert_contains internal/auth/handler.go "httputil.WriteJSON(w, http.StatusOK, authResponse{Token: accessToken, RefreshToken: refreshToken, User: user})" "refresh should return HTTP 200 auth response"
assert_contains internal/auth/handler.go "\"invalid email or password\"" "login failure contract should return invalid credentials error"
assert_contains internal/auth/handler.go "\"invalid or expired refresh token\"" "refresh failure contract should return invalid/expired token error"

assert_contains internal/auth/auth_login.go "Rotate: generate new refresh token and update the session row." "auth service should rotate refresh token on successful refresh"
assert_contains internal/auth/auth_login.go "return user, accessToken, newPlaintext, nil" "auth service should return newly rotated refresh token"

assert_contains internal/auth/auth_integration_test.go "func TestLoginWrongPassword(t *testing.T) {" "integration suite should lock invalid-credential behavior"
assert_contains internal/auth/auth_integration_test.go "func TestRefreshTokenExpired(t *testing.T) {" "integration suite should lock expired refresh-token behavior"
assert_contains internal/auth/auth_integration_test.go "func TestRefreshTokenRotation(t *testing.T) {" "integration suite should lock refresh-token rotation behavior"
assert_contains internal/auth/auth_integration_test.go "testutil.NotEqual(t, resp1.RefreshToken, resp2.RefreshToken)" "integration suite should assert refresh-token rotation changes token value"

assert_contains Makefile "LOAD_AUTH_REQUEST_PATH_SCENARIO := tests/load/scenarios/auth_register_login_refresh.js" "makefile should define auth request-path scenario script"
assert_contains Makefile "load-auth-request-path:" "makefile should expose direct auth request-path target"
assert_contains Makefile "load-auth-request-path-local:" "makefile should expose local auth request-path target"
assert_contains tests/load/README.md "make load-auth-request-path" "README should document direct auth request-path target"
assert_contains tests/load/README.md "make load-auth-request-path-local" "README should document local auth request-path target"
assert_contains tests/load/README.md "K6_VUS=1 K6_ITERATIONS=1 make load-auth-request-path-local" "README should document smallest local auth request-path smoke command"
assert_section_contains tests/load/README.md "## Auth Request-Path Scenario" 'Stage 7 measured smoke command: `K6_VUS=1 K6_ITERATIONS=1 make load-auth-request-path-local`' "README auth section should pin the measured Stage 7 auth smoke command"
assert_section_contains tests/load/README.md "## Auth Request-Path Scenario" 'Stage 7 contract assertion: `bash tests/test_load_auth_contract.sh`' "README auth section should identify the guarding contract script"
assert_section_contains tests/load/README.md "## Auth Request-Path Scenario" 'Stage 7 caveat: refresh-token reuse must keep returning `401` after token rotation.' "README auth section should preserve the Stage 7 refresh-rotation caveat"

assert_contains tests/test_load_harness.sh "make load-auth-request-path" "harness regression should validate direct auth request-path make target"
assert_contains tests/test_load_harness.sh "make load-auth-request-path-local" "harness regression should validate local auth request-path make target"

echo "PASS: Stage 3 auth contract and harness boundary assertions are wired"
