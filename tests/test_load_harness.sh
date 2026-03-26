#!/usr/bin/env bash
set -euo pipefail

TMP_DIR="$(mktemp -d)"
DIRECT_SERVER_PID=""
SUSTAINED_SOAK_DIRECT_SERVER_PID=""
cleanup() {
  if [[ -n "$DIRECT_SERVER_PID" ]]; then
    kill "$DIRECT_SERVER_PID" 2>/dev/null || true
    wait "$DIRECT_SERVER_PID" 2>/dev/null || true
  fi
  if [[ -n "$SUSTAINED_SOAK_DIRECT_SERVER_PID" ]]; then
    kill "$SUSTAINED_SOAK_DIRECT_SERVER_PID" 2>/dev/null || true
    wait "$SUSTAINED_SOAK_DIRECT_SERVER_PID" 2>/dev/null || true
  fi
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT

PASS_COUNT=0

assert_contains() {
  local file_path="$1"
  local needle="$2"
  local fail_message="$3"
  if ! grep -Fq -- "$needle" "$file_path"; then
    echo "FAIL: ${fail_message}"
    echo "--- ${file_path} ---"
    cat "$file_path"
    exit 1
  fi
}

assert_not_contains() {
  local file_path="$1"
  local needle="$2"
  local fail_message="$3"
  if grep -Fq -- "$needle" "$file_path"; then
    echo "FAIL: ${fail_message}"
    echo "--- ${file_path} ---"
    cat "$file_path"
    exit 1
  fi
}

assert_equals() {
  local actual="$1"
  local expected="$2"
  local fail_message="$3"
  if [[ "$actual" != "$expected" ]]; then
    echo "FAIL: ${fail_message}"
    echo "expected: ${expected}"
    echo "actual:   ${actual}"
    exit 1
  fi
}

assert_nonempty_dynamic_jwt_secret() {
  local file_path="$1"
  local fail_message="$2"
  if ! grep -Eq '^AYB_AUTH_JWT_SECRET=.+$' "$file_path"; then
    echo "FAIL: ${fail_message}"
    echo "--- ${file_path} ---"
    cat "$file_path"
    exit 1
  fi
  if grep -Fq -- "AYB_AUTH_JWT_SECRET=stage3-load-jwt-secret-0123456789" "$file_path"; then
    echo "FAIL: ${fail_message}"
    echo "--- ${file_path} ---"
    cat "$file_path"
    exit 1
  fi
}

# run_direct_make invokes a Makefile target with the standard direct-mode env
# (no local server boot). Args: RECORD_PATH TARGET LABEL PORT
run_direct_make() {
  local record_path="$1" target="$2" label="$3" port="$4"
  if ! env -u AYB_AUTH_ENABLED -u AYB_AUTH_JWT_SECRET \
    PATH="${TMP_DIR}/bin:${PATH}" \
    HOME="${TMP_DIR}/home" \
    K6_RECORD_PATH="$record_path" \
    LOAD_K6_BIN="${TMP_DIR}/bin/k6" \
    AYB_BASE_URL="http://127.0.0.1:${port}" \
    make "$target" > "${TMP_DIR}/${label}.stdout" 2> "${TMP_DIR}/${label}.stderr"; then
    echo "FAIL: make ${target} failed"
    cat "${TMP_DIR}/${label}.stdout"
    cat "${TMP_DIR}/${label}.stderr"
    exit 1
  fi
}

# run_local_make invokes a Makefile -local target, booting the fixture server.
# Args: RECORD_PATH TARGET LABEL PORT AUTH_LOG TOKEN_NAME
run_local_make() {
  local record_path="$1" target="$2" label="$3" port="$4" auth_log="$5" token_name="$6"
  if ! env -u AYB_AUTH_ENABLED -u AYB_AUTH_JWT_SECRET \
    PATH="${TMP_DIR}/bin:${PATH}" \
    HOME="${TMP_DIR}/home" \
    K6_RECORD_PATH="$record_path" \
    LOAD_K6_BIN="${TMP_DIR}/bin/k6" \
    REQUIRE_HEALTH_READY=1 \
    AYB_BASE_URL="http://127.0.0.1:${port}" \
    AYB_HEALTH_URL="http://127.0.0.1:${port}/health" \
    AYB_ADMIN_PASSWORD='unused-in-test' \
    AYB_START_COMMAND="python3 \"${TMP_DIR}/auth_server.py\" ${port} \"${auth_log}\" password-from-file ${token_name}" \
    make "$target" > "${TMP_DIR}/${label}.stdout" 2> "${TMP_DIR}/${label}.stderr"; then
    echo "FAIL: make ${target} failed"
    cat "${TMP_DIR}/${label}.stdout"
    cat "${TMP_DIR}/${label}.stderr"
    exit 1
  fi
}

mkdir -p "${TMP_DIR}/home/.ayb" "${TMP_DIR}/bin"
printf "password-from-file\n" > "${TMP_DIR}/home/.ayb/admin-token"

cat > "${TMP_DIR}/auth_server.py" <<'PY'
#!/usr/bin/env python3
import json
import pathlib
import sys
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

port = int(sys.argv[1])
auth_log_path = pathlib.Path(sys.argv[2])
expected_password = sys.argv[3]
issued_token = sys.argv[4]


class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        if self.path == "/health":
            body = b"ok\n"
            self.send_response(200)
            self.send_header("Content-Type", "text/plain")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)
            return

        self.send_error(404)

    def do_POST(self):
        if self.path != "/api/admin/auth":
            self.send_error(404)
            return

        length = int(self.headers.get("Content-Length", "0"))
        raw_body = self.rfile.read(length)
        auth_log_path.write_bytes(raw_body)
        try:
            payload = json.loads(raw_body.decode("utf-8"))
        except json.JSONDecodeError:
            self.send_error(400)
            return

        if payload.get("password") != expected_password:
            self.send_error(401)
            return

        response = json.dumps({"token": issued_token}).encode("utf-8")
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(response)))
        self.end_headers()
        self.wfile.write(response)

    def log_message(self, _format, *_args):
        return


ThreadingHTTPServer(("127.0.0.1", port), Handler).serve_forever()
PY
chmod +x "${TMP_DIR}/auth_server.py"

cat > "${TMP_DIR}/bin/k6" <<'K6'
#!/usr/bin/env bash
set -euo pipefail
: "${K6_RECORD_PATH:?missing K6_RECORD_PATH}"
{
  printf 'argv=%s\n' "$*"
  printf 'AYB_ADMIN_TOKEN=%s\n' "${AYB_ADMIN_TOKEN:-}"
  if [[ -n "${AYB_AUTH_ENABLED+x}" ]]; then
    printf 'AYB_AUTH_ENABLED=%s\n' "${AYB_AUTH_ENABLED}"
  fi
  if [[ -n "${AYB_AUTH_JWT_SECRET+x}" ]]; then
    printf 'AYB_AUTH_JWT_SECRET=%s\n' "${AYB_AUTH_JWT_SECRET}"
  fi
  if [[ -n "${K6_VUS+x}" ]]; then
    printf 'K6_VUS=%s\n' "${K6_VUS}"
  fi
  if [[ -n "${K6_ITERATIONS+x}" ]]; then
    printf 'K6_ITERATIONS=%s\n' "${K6_ITERATIONS}"
  fi
  if [[ -n "${AYB_POOL_PRESSURE_VUS+x}" ]]; then
    printf 'AYB_POOL_PRESSURE_VUS=%s\n' "${AYB_POOL_PRESSURE_VUS}"
  fi
  if [[ -n "${AYB_POOL_PRESSURE_ITERATIONS+x}" ]]; then
    printf 'AYB_POOL_PRESSURE_ITERATIONS=%s\n' "${AYB_POOL_PRESSURE_ITERATIONS}"
  fi
  printf 'AYB_AUTH_RATE_LIMIT=%s\n' "${AYB_AUTH_RATE_LIMIT:-}"
  printf 'AYB_RATE_LIMIT_API=%s\n' "${AYB_RATE_LIMIT_API:-}"
  printf 'AYB_RATE_LIMIT_API_ANONYMOUS=%s\n' "${AYB_RATE_LIMIT_API_ANONYMOUS:-}"
  printf 'AYB_BASE_URL=%s\n' "${AYB_BASE_URL:-}"
  printf '%s\n' '--'
} >> "$K6_RECORD_PATH"

if [[ "${REQUIRE_HEALTH_READY:-0}" == "1" ]]; then
  curl -fsS "${AYB_BASE_URL}/health" > /dev/null
fi
K6
chmod +x "${TMP_DIR}/bin/k6"

DIRECT_AUTH_LOG="${TMP_DIR}/direct-auth.log"
DIRECT_PORT="$(python3 - <<'PY'
import socket

with socket.socket() as sock:
    sock.bind(("127.0.0.1", 0))
    print(sock.getsockname()[1])
PY
)"
python3 "${TMP_DIR}/auth_server.py" "$DIRECT_PORT" "$DIRECT_AUTH_LOG" "password-from-file" "token-from-direct-auth" > "${TMP_DIR}/direct.server.log" 2>&1 &
DIRECT_SERVER_PID=$!

wait_for_http() {
  local url="$1"
  local fail_message="$2"
  for _ in $(seq 1 50); do
    if curl -fsS "$url" > /dev/null 2>&1; then
      return 0
    fi
    sleep 0.1
  done

  echo "FAIL: ${fail_message}"
  exit 1
}

wait_for_http "http://127.0.0.1:${DIRECT_PORT}/health" "direct auth fixture did not become healthy"

DIRECT_RECORD_PATH="${TMP_DIR}/direct-k6.log"
run_direct_make "$DIRECT_RECORD_PATH" load-admin-status direct "$DIRECT_PORT"

assert_contains "$DIRECT_RECORD_PATH" "argv=run" "k6 should be invoked in run mode for direct target"
assert_contains "$DIRECT_RECORD_PATH" "tests/load/scenarios/admin_status.js" "direct target should execute admin_status scenario"
assert_contains "$DIRECT_RECORD_PATH" "AYB_ADMIN_TOKEN=token-from-direct-auth" "direct target should export bearer token returned by /api/admin/auth"
assert_contains "$DIRECT_RECORD_PATH" "AYB_AUTH_RATE_LIMIT=10000" "direct target should export load-safe auth rate limit"
assert_contains "$DIRECT_RECORD_PATH" "AYB_RATE_LIMIT_API=10000/min" "direct target should export load-safe API rate limit"
assert_contains "$DIRECT_RECORD_PATH" "AYB_RATE_LIMIT_API_ANONYMOUS=10000/min" "direct target should export load-safe anonymous API rate limit"
assert_not_contains "$DIRECT_RECORD_PATH" "AYB_AUTH_ENABLED=" "baseline direct target should not force auth enablement for the Stage 2 server config"
assert_not_contains "$DIRECT_RECORD_PATH" "AYB_AUTH_JWT_SECRET=" "baseline direct target should not inject auth jwt config"
assert_contains "$DIRECT_AUTH_LOG" "\"password\": \"password-from-file\"" "direct target should exchange the saved admin password via /api/admin/auth"
PASS_COUNT=$((PASS_COUNT + 1))

AUTH_DIRECT_RECORD_PATH="${TMP_DIR}/auth-direct-k6.log"
run_direct_make "$AUTH_DIRECT_RECORD_PATH" load-auth-request-path auth-direct "$DIRECT_PORT"

assert_contains "$AUTH_DIRECT_RECORD_PATH" "argv=run" "auth direct target should execute k6 run"
assert_contains "$AUTH_DIRECT_RECORD_PATH" "tests/load/scenarios/auth_register_login_refresh.js" "auth direct target should execute auth request-path scenario"
assert_contains "$AUTH_DIRECT_RECORD_PATH" "AYB_ADMIN_TOKEN=token-from-direct-auth" "auth direct target should resolve admin auth bootstrap token"
assert_contains "$AUTH_DIRECT_RECORD_PATH" "AYB_AUTH_ENABLED=true" "auth direct target should enable auth for auth scenario smoke runs"
assert_nonempty_dynamic_jwt_secret "$AUTH_DIRECT_RECORD_PATH" "auth direct target should inject a non-empty, non-static jwt secret for auth scenario smoke runs"
assert_not_contains "Makefile" "/api/auth/register" "makefile should not duplicate auth payload request logic"
assert_not_contains "Makefile" "/api/auth/login" "makefile should not duplicate auth payload request logic"
assert_not_contains "Makefile" "/api/auth/refresh" "makefile should not duplicate auth payload request logic"
PASS_COUNT=$((PASS_COUNT + 1))

DATA_PATH_DIRECT_RECORD_PATH="${TMP_DIR}/data-path-direct-k6.log"
run_direct_make "$DATA_PATH_DIRECT_RECORD_PATH" load-data-path data-path-direct "$DIRECT_PORT"

assert_contains "$DATA_PATH_DIRECT_RECORD_PATH" "argv=run" "data-path direct target should execute k6 run"
assert_contains "$DATA_PATH_DIRECT_RECORD_PATH" "tests/load/scenarios/data_path_crud_batch.js" "data-path direct target should execute CRUD/batch scenario"
assert_contains "$DATA_PATH_DIRECT_RECORD_PATH" "AYB_ADMIN_TOKEN=token-from-direct-auth" "data-path direct target should resolve admin auth bootstrap token"
assert_contains "$DATA_PATH_DIRECT_RECORD_PATH" "AYB_AUTH_RATE_LIMIT=10000" "data-path direct target should export load-safe auth rate limit"
assert_contains "$DATA_PATH_DIRECT_RECORD_PATH" "AYB_RATE_LIMIT_API=10000/min" "data-path direct target should export load-safe API rate limit"
assert_contains "$DATA_PATH_DIRECT_RECORD_PATH" "AYB_RATE_LIMIT_API_ANONYMOUS=10000/min" "data-path direct target should export load-safe anonymous API rate limit"
PASS_COUNT=$((PASS_COUNT + 1))

POOL_PRESSURE_DIRECT_RECORD_PATH="${TMP_DIR}/pool-pressure-direct-k6.log"
run_direct_make "$POOL_PRESSURE_DIRECT_RECORD_PATH" load-data-pool-pressure pool-pressure-direct "$DIRECT_PORT"

assert_contains "$POOL_PRESSURE_DIRECT_RECORD_PATH" "argv=run" "pool-pressure direct target should execute k6 run"
assert_contains "$POOL_PRESSURE_DIRECT_RECORD_PATH" "tests/load/scenarios/data_pool_pressure.js" "pool-pressure direct target should execute admin SQL pressure scenario"
assert_contains "$POOL_PRESSURE_DIRECT_RECORD_PATH" "AYB_ADMIN_TOKEN=token-from-direct-auth" "pool-pressure direct target should resolve admin auth bootstrap token"
assert_contains "$POOL_PRESSURE_DIRECT_RECORD_PATH" "AYB_AUTH_RATE_LIMIT=10000" "pool-pressure direct target should export load-safe auth rate limit"
assert_contains "$POOL_PRESSURE_DIRECT_RECORD_PATH" "AYB_RATE_LIMIT_API=10000/min" "pool-pressure direct target should export load-safe API rate limit"
assert_contains "$POOL_PRESSURE_DIRECT_RECORD_PATH" "AYB_RATE_LIMIT_API_ANONYMOUS=10000/min" "pool-pressure direct target should export load-safe anonymous API rate limit"
assert_not_contains "Makefile" "SELECT pg_sleep(2)" "makefile should not duplicate admin SQL pressure query bodies"
assert_not_contains "Makefile" "CREATE TABLE" "makefile should not embed Stage 4 fixture DDL bodies"
assert_not_contains "Makefile" "DROP TABLE" "makefile should not embed Stage 4 fixture teardown DDL bodies"
PASS_COUNT=$((PASS_COUNT + 1))

REALTIME_DIRECT_RECORD_PATH="${TMP_DIR}/realtime-direct-k6.log"
run_direct_make "$REALTIME_DIRECT_RECORD_PATH" load-realtime-ws realtime-direct "$DIRECT_PORT"

assert_contains "$REALTIME_DIRECT_RECORD_PATH" "argv=run" "realtime direct target should execute k6 run"
assert_contains "$REALTIME_DIRECT_RECORD_PATH" "tests/load/scenarios/realtime_ws_subscribe.js" "realtime direct target should execute Stage 5 websocket scenario"
assert_contains "$REALTIME_DIRECT_RECORD_PATH" "AYB_ADMIN_TOKEN=token-from-direct-auth" "realtime direct target should resolve admin auth bootstrap token"
assert_contains "$REALTIME_DIRECT_RECORD_PATH" "AYB_AUTH_ENABLED=true" "realtime direct target should enable auth for websocket user sessions"
assert_nonempty_dynamic_jwt_secret "$REALTIME_DIRECT_RECORD_PATH" "realtime direct target should inject a non-empty, non-static jwt secret for websocket user sessions"
assert_contains "$REALTIME_DIRECT_RECORD_PATH" "AYB_AUTH_RATE_LIMIT=10000" "realtime direct target should export load-safe auth rate limit"
assert_contains "$REALTIME_DIRECT_RECORD_PATH" "AYB_RATE_LIMIT_API=10000/min" "realtime direct target should export load-safe API rate limit"
assert_contains "$REALTIME_DIRECT_RECORD_PATH" "AYB_RATE_LIMIT_API_ANONYMOUS=10000/min" "realtime direct target should export load-safe anonymous API rate limit"
assert_not_contains "Makefile" "\"type\":\"subscribe\"" "makefile should not duplicate realtime websocket payload bodies"
PASS_COUNT=$((PASS_COUNT + 1))

for tier in 100 500 1000; do
  HTTP_TIER_RECORD_PATH="${TMP_DIR}/http-tier-${tier}-k6.log"
  : > "$HTTP_TIER_RECORD_PATH"
  run_direct_make "$HTTP_TIER_RECORD_PATH" "load-http-${tier}" "http-tier-${tier}" "$DIRECT_PORT"
  assert_contains "$HTTP_TIER_RECORD_PATH" "tests/load/scenarios/admin_status.js" "load-http-${tier} should include the admin status scenario"
  assert_contains "$HTTP_TIER_RECORD_PATH" "tests/load/scenarios/auth_register_login_refresh.js" "load-http-${tier} should include the auth request-path scenario"
  assert_contains "$HTTP_TIER_RECORD_PATH" "tests/load/scenarios/data_path_crud_batch.js" "load-http-${tier} should include the data-path scenario"
  assert_contains "$HTTP_TIER_RECORD_PATH" "tests/load/scenarios/data_pool_pressure.js" "load-http-${tier} should include the pool-pressure scenario"
  assert_contains "$HTTP_TIER_RECORD_PATH" "K6_VUS=${tier}" "load-http-${tier} should set K6_VUS for non-pool scenarios"
  assert_contains "$HTTP_TIER_RECORD_PATH" "K6_ITERATIONS=${tier}" "load-http-${tier} should set K6_ITERATIONS for non-pool scenarios"
  assert_contains "$HTTP_TIER_RECORD_PATH" "AYB_POOL_PRESSURE_VUS=${tier}" "load-http-${tier} should map tier VUs to pool-pressure specific env"
  assert_contains "$HTTP_TIER_RECORD_PATH" "AYB_POOL_PRESSURE_ITERATIONS=${tier}" "load-http-${tier} should map tier iterations to pool-pressure specific env"
  assert_contains "$HTTP_TIER_RECORD_PATH" "AYB_ADMIN_TOKEN=token-from-direct-auth" "load-http-${tier} should resolve admin token through shared bootstrap helpers"
  PASS_COUNT=$((PASS_COUNT + 1))
done

for tier in 1000 5000 10000; do
  REALTIME_TIER_RECORD_PATH="${TMP_DIR}/realtime-tier-${tier}-k6.log"
  : > "$REALTIME_TIER_RECORD_PATH"
  run_direct_make "$REALTIME_TIER_RECORD_PATH" "load-realtime-ws-${tier}" "realtime-tier-${tier}" "$DIRECT_PORT"
  assert_contains "$REALTIME_TIER_RECORD_PATH" "tests/load/scenarios/realtime_ws_subscribe.js" "load-realtime-ws-${tier} should execute the shared websocket scenario"
  assert_contains "$REALTIME_TIER_RECORD_PATH" "AYB_ADMIN_TOKEN=token-from-direct-auth" "load-realtime-ws-${tier} should preserve admin token resolution via shared bootstrap helpers"
  assert_contains "$REALTIME_TIER_RECORD_PATH" "AYB_AUTH_ENABLED=true" "load-realtime-ws-${tier} should preserve auth bootstrap for websocket subscriptions"
  assert_nonempty_dynamic_jwt_secret "$REALTIME_TIER_RECORD_PATH" "load-realtime-ws-${tier} should preserve shared auth secret bootstrap"
  assert_contains "$REALTIME_TIER_RECORD_PATH" "K6_VUS=${tier}" "load-realtime-ws-${tier} should set K6_VUS through shared env parsing path"
  assert_contains "$REALTIME_TIER_RECORD_PATH" "K6_ITERATIONS=${tier}" "load-realtime-ws-${tier} should set K6_ITERATIONS through shared env parsing path"
  PASS_COUNT=$((PASS_COUNT + 1))
done

HELP_OUTPUT_PATH="${TMP_DIR}/make-help.out"
make help > "$HELP_OUTPUT_PATH"
for target in \
  load-http-100 \
  load-http-500 \
  load-http-1000 \
  load-realtime-ws-1000 \
  load-realtime-ws-5000 \
  load-realtime-ws-10000; do
  assert_contains "$HELP_OUTPUT_PATH" "$target" "make help should list ${target} as a stable Stage 6 entry point"
done
PASS_COUNT=$((PASS_COUNT + 1))

kill "$DIRECT_SERVER_PID" 2>/dev/null || true
wait "$DIRECT_SERVER_PID" 2>/dev/null || true

LOCAL_RECORD_PATH="${TMP_DIR}/local-k6.log"
LOCAL_AUTH_LOG="${TMP_DIR}/local-auth.log"
LOCAL_PORT="18091"
run_local_make "$LOCAL_RECORD_PATH" load-admin-status-local local "$LOCAL_PORT" "$LOCAL_AUTH_LOG" token-from-local-auth

assert_contains "$LOCAL_RECORD_PATH" "argv=run" "local target should execute k6 run"
assert_contains "$LOCAL_RECORD_PATH" "tests/load/scenarios/admin_status.js" "local target should execute admin_status scenario"
assert_contains "$LOCAL_RECORD_PATH" "AYB_ADMIN_TOKEN=token-from-local-auth" "local target should resolve saved admin password after AYB is ready"
assert_contains "$LOCAL_RECORD_PATH" "AYB_BASE_URL=http://127.0.0.1:${LOCAL_PORT}" "local target should pass base URL to k6"
assert_not_contains "$LOCAL_RECORD_PATH" "AYB_AUTH_ENABLED=" "baseline local target should not force auth enablement for the started Stage 2 server"
assert_not_contains "$LOCAL_RECORD_PATH" "AYB_AUTH_JWT_SECRET=" "baseline local target should not inject auth jwt config"
assert_contains "$LOCAL_AUTH_LOG" "\"password\": \"password-from-file\"" "local target should exchange the saved admin password via /api/admin/auth"
PASS_COUNT=$((PASS_COUNT + 1))

AUTH_LOCAL_RECORD_PATH="${TMP_DIR}/auth-local-k6.log"
AUTH_LOCAL_AUTH_LOG="${TMP_DIR}/auth-local-auth.log"
AUTH_LOCAL_PORT="18092"
run_local_make "$AUTH_LOCAL_RECORD_PATH" load-auth-request-path-local auth-local "$AUTH_LOCAL_PORT" "$AUTH_LOCAL_AUTH_LOG" token-from-auth-local-auth

assert_contains "$AUTH_LOCAL_RECORD_PATH" "argv=run" "auth local target should execute k6 run"
assert_contains "$AUTH_LOCAL_RECORD_PATH" "tests/load/scenarios/auth_register_login_refresh.js" "auth local target should execute auth request-path scenario"
assert_contains "$AUTH_LOCAL_RECORD_PATH" "AYB_ADMIN_TOKEN=token-from-auth-local-auth" "auth local target should resolve saved admin password via /api/admin/auth"
assert_contains "$AUTH_LOCAL_RECORD_PATH" "AYB_BASE_URL=http://127.0.0.1:${AUTH_LOCAL_PORT}" "auth local target should pass base URL to k6"
assert_contains "$AUTH_LOCAL_RECORD_PATH" "AYB_AUTH_ENABLED=true" "auth local target should enable auth for the started server"
assert_nonempty_dynamic_jwt_secret "$AUTH_LOCAL_RECORD_PATH" "auth local target should inject a non-empty, non-static jwt secret for the started server"
assert_contains "$AUTH_LOCAL_AUTH_LOG" "\"password\": \"password-from-file\"" "auth local target should exchange the saved admin password via /api/admin/auth"
PASS_COUNT=$((PASS_COUNT + 1))

DATA_PATH_LOCAL_RECORD_PATH="${TMP_DIR}/data-path-local-k6.log"
DATA_PATH_LOCAL_AUTH_LOG="${TMP_DIR}/data-path-local-auth.log"
DATA_PATH_LOCAL_PORT="18093"
run_local_make "$DATA_PATH_LOCAL_RECORD_PATH" load-data-path-local data-path-local "$DATA_PATH_LOCAL_PORT" "$DATA_PATH_LOCAL_AUTH_LOG" token-from-data-path-local-auth

assert_contains "$DATA_PATH_LOCAL_RECORD_PATH" "argv=run" "data-path local target should execute k6 run"
assert_contains "$DATA_PATH_LOCAL_RECORD_PATH" "tests/load/scenarios/data_path_crud_batch.js" "data-path local target should execute CRUD/batch scenario"
assert_contains "$DATA_PATH_LOCAL_RECORD_PATH" "AYB_ADMIN_TOKEN=token-from-data-path-local-auth" "data-path local target should resolve saved admin password via /api/admin/auth"
assert_contains "$DATA_PATH_LOCAL_RECORD_PATH" "AYB_BASE_URL=http://127.0.0.1:${DATA_PATH_LOCAL_PORT}" "data-path local target should pass base URL to k6"
assert_contains "$DATA_PATH_LOCAL_RECORD_PATH" "AYB_AUTH_ENABLED=true" "data-path local target should enable auth for the started server"
assert_nonempty_dynamic_jwt_secret "$DATA_PATH_LOCAL_RECORD_PATH" "data-path local target should inject a non-empty, non-static jwt secret for the started server"
assert_contains "$DATA_PATH_LOCAL_AUTH_LOG" "\"password\": \"password-from-file\"" "data-path local target should exchange the saved admin password via /api/admin/auth"
PASS_COUNT=$((PASS_COUNT + 1))

POOL_PRESSURE_LOCAL_RECORD_PATH="${TMP_DIR}/pool-pressure-local-k6.log"
POOL_PRESSURE_LOCAL_AUTH_LOG="${TMP_DIR}/pool-pressure-local-auth.log"
POOL_PRESSURE_LOCAL_PORT="18094"
run_local_make "$POOL_PRESSURE_LOCAL_RECORD_PATH" load-data-pool-pressure-local pool-pressure-local "$POOL_PRESSURE_LOCAL_PORT" "$POOL_PRESSURE_LOCAL_AUTH_LOG" token-from-pool-pressure-local-auth

assert_contains "$POOL_PRESSURE_LOCAL_RECORD_PATH" "argv=run" "pool-pressure local target should execute k6 run"
assert_contains "$POOL_PRESSURE_LOCAL_RECORD_PATH" "tests/load/scenarios/data_pool_pressure.js" "pool-pressure local target should execute admin SQL pressure scenario"
assert_contains "$POOL_PRESSURE_LOCAL_RECORD_PATH" "AYB_ADMIN_TOKEN=token-from-pool-pressure-local-auth" "pool-pressure local target should resolve saved admin password via /api/admin/auth"
assert_contains "$POOL_PRESSURE_LOCAL_RECORD_PATH" "AYB_BASE_URL=http://127.0.0.1:${POOL_PRESSURE_LOCAL_PORT}" "pool-pressure local target should pass base URL to k6"
assert_contains "$POOL_PRESSURE_LOCAL_AUTH_LOG" "\"password\": \"password-from-file\"" "pool-pressure local target should exchange the saved admin password via /api/admin/auth"
PASS_COUNT=$((PASS_COUNT + 1))

REALTIME_LOCAL_RECORD_PATH="${TMP_DIR}/realtime-local-k6.log"
REALTIME_LOCAL_AUTH_LOG="${TMP_DIR}/realtime-local-auth.log"
REALTIME_LOCAL_PORT="18095"
run_local_make "$REALTIME_LOCAL_RECORD_PATH" load-realtime-ws-local realtime-local "$REALTIME_LOCAL_PORT" "$REALTIME_LOCAL_AUTH_LOG" token-from-realtime-local-auth

assert_contains "$REALTIME_LOCAL_RECORD_PATH" "argv=run" "realtime local target should execute k6 run"
assert_contains "$REALTIME_LOCAL_RECORD_PATH" "tests/load/scenarios/realtime_ws_subscribe.js" "realtime local target should execute Stage 5 websocket scenario"
assert_contains "$REALTIME_LOCAL_RECORD_PATH" "AYB_ADMIN_TOKEN=token-from-realtime-local-auth" "realtime local target should resolve saved admin password via /api/admin/auth"
assert_contains "$REALTIME_LOCAL_RECORD_PATH" "AYB_BASE_URL=http://127.0.0.1:${REALTIME_LOCAL_PORT}" "realtime local target should pass base URL to k6"
assert_contains "$REALTIME_LOCAL_RECORD_PATH" "AYB_AUTH_ENABLED=true" "realtime local target should enable auth for the started server"
assert_nonempty_dynamic_jwt_secret "$REALTIME_LOCAL_RECORD_PATH" "realtime local target should inject a non-empty, non-static jwt secret for the started server"
assert_contains "$REALTIME_LOCAL_AUTH_LOG" "\"password\": \"password-from-file\"" "realtime local target should exchange the saved admin password via /api/admin/auth"
PASS_COUNT=$((PASS_COUNT + 1))

SUSTAINED_SOAK_DIRECT_RECORD_PATH="${TMP_DIR}/sustained-soak-direct-k6.log"
SUSTAINED_SOAK_DIRECT_AUTH_LOG="${TMP_DIR}/sustained-soak-direct-auth.log"
SUSTAINED_SOAK_DIRECT_PORT="18097"
python3 "${TMP_DIR}/auth_server.py" "${SUSTAINED_SOAK_DIRECT_PORT}" "${SUSTAINED_SOAK_DIRECT_AUTH_LOG}" "password-from-file" "token-from-sustained-soak-direct-auth" > "${TMP_DIR}/sustained-soak-direct.server.log" 2>&1 &
SUSTAINED_SOAK_DIRECT_SERVER_PID=$!
wait_for_http "http://127.0.0.1:${SUSTAINED_SOAK_DIRECT_PORT}/health" "sustained-soak direct auth fixture did not become healthy"
# Note: SUSTAINED_SOAK_DIRECT_SERVER_PID cleanup on failure is handled by the
# EXIT trap, so run_direct_make's exit 1 is safe here.
run_direct_make "$SUSTAINED_SOAK_DIRECT_RECORD_PATH" load-sustained-soak sustained-soak-direct "$SUSTAINED_SOAK_DIRECT_PORT"

assert_contains "$SUSTAINED_SOAK_DIRECT_RECORD_PATH" "argv=run" "sustained-soak direct target should execute k6 run"
assert_contains "$SUSTAINED_SOAK_DIRECT_RECORD_PATH" "tests/load/scenarios/sustained_soak.js" "sustained-soak direct target should execute Stage 6 soak scenario"
assert_contains "$SUSTAINED_SOAK_DIRECT_RECORD_PATH" "AYB_ADMIN_TOKEN=token-from-sustained-soak-direct-auth" "sustained-soak direct target should resolve admin auth bootstrap token"
assert_contains "$SUSTAINED_SOAK_DIRECT_RECORD_PATH" "AYB_AUTH_ENABLED=true" "sustained-soak direct target should enable auth for mixed workload flows"
assert_nonempty_dynamic_jwt_secret "$SUSTAINED_SOAK_DIRECT_RECORD_PATH" "sustained-soak direct target should inject a non-empty, non-static jwt secret for mixed workload flows"
assert_contains "$SUSTAINED_SOAK_DIRECT_RECORD_PATH" "AYB_AUTH_RATE_LIMIT=10000" "sustained-soak direct target should export load-safe auth rate limit"
assert_contains "$SUSTAINED_SOAK_DIRECT_RECORD_PATH" "AYB_RATE_LIMIT_API=10000/min" "sustained-soak direct target should export load-safe API rate limit"
assert_contains "$SUSTAINED_SOAK_DIRECT_RECORD_PATH" "AYB_RATE_LIMIT_API_ANONYMOUS=10000/min" "sustained-soak direct target should export load-safe anonymous API rate limit"
assert_contains "$SUSTAINED_SOAK_DIRECT_AUTH_LOG" "\"password\": \"password-from-file\"" "sustained-soak direct target should exchange the saved admin password via /api/admin/auth"
kill "$SUSTAINED_SOAK_DIRECT_SERVER_PID" 2>/dev/null || true
wait "$SUSTAINED_SOAK_DIRECT_SERVER_PID" 2>/dev/null || true
PASS_COUNT=$((PASS_COUNT + 1))

SUSTAINED_SOAK_LOCAL_RECORD_PATH="${TMP_DIR}/sustained-soak-local-k6.log"
SUSTAINED_SOAK_LOCAL_AUTH_LOG="${TMP_DIR}/sustained-soak-local-auth.log"
SUSTAINED_SOAK_LOCAL_PORT="18096"
run_local_make "$SUSTAINED_SOAK_LOCAL_RECORD_PATH" load-sustained-soak-local sustained-soak-local "$SUSTAINED_SOAK_LOCAL_PORT" "$SUSTAINED_SOAK_LOCAL_AUTH_LOG" token-from-sustained-soak-local-auth

assert_contains "$SUSTAINED_SOAK_LOCAL_RECORD_PATH" "argv=run" "sustained-soak local target should execute k6 run"
assert_contains "$SUSTAINED_SOAK_LOCAL_RECORD_PATH" "tests/load/scenarios/sustained_soak.js" "sustained-soak local target should execute Stage 6 soak scenario"
assert_contains "$SUSTAINED_SOAK_LOCAL_RECORD_PATH" "AYB_ADMIN_TOKEN=token-from-sustained-soak-local-auth" "sustained-soak local target should resolve saved admin password via /api/admin/auth"
assert_contains "$SUSTAINED_SOAK_LOCAL_RECORD_PATH" "AYB_BASE_URL=http://127.0.0.1:${SUSTAINED_SOAK_LOCAL_PORT}" "sustained-soak local target should pass base URL to k6"
assert_contains "$SUSTAINED_SOAK_LOCAL_RECORD_PATH" "AYB_AUTH_ENABLED=true" "sustained-soak local target should enable auth for mixed workload flows"
assert_nonempty_dynamic_jwt_secret "$SUSTAINED_SOAK_LOCAL_RECORD_PATH" "sustained-soak local target should inject a non-empty, non-static jwt secret for mixed workload flows"
assert_contains "$SUSTAINED_SOAK_LOCAL_AUTH_LOG" "\"password\": \"password-from-file\"" "sustained-soak local target should exchange the saved admin password via /api/admin/auth"
PASS_COUNT=$((PASS_COUNT + 1))

assert_equals "$PASS_COUNT" "19" "expected baseline, tiered HTTP/realtime aliases, help output, local targets, and sustained-soak load target assertions to run"
echo "PASS: load Makefile targets bootstrap env once and run k6 after local readiness for baseline, tiered HTTP/realtime aliases, auth request-path, data-path, pool-pressure, realtime websocket, and sustained-soak scenarios"
