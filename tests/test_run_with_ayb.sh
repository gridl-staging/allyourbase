#!/usr/bin/env bash
set -euo pipefail

TMP_DIR="$(mktemp -d)"
HTTP_PID=""

cleanup() {
  if [[ -n "$HTTP_PID" ]]; then
    kill "$HTTP_PID" 2>/dev/null || true
    wait "$HTTP_PID" 2>/dev/null || true
  fi
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT

printf 'ok\n' > "${TMP_DIR}/health"
PORT_PATH="${TMP_DIR}/health.port"
python3 - "$TMP_DIR" "$PORT_PATH" > "${TMP_DIR}/http.log" 2>&1 <<'PY' &
import functools
import http.server
import pathlib
import socketserver
import sys

directory = pathlib.Path(sys.argv[1])
port_path = pathlib.Path(sys.argv[2])
handler = functools.partial(http.server.SimpleHTTPRequestHandler, directory=str(directory))

with socketserver.TCPServer(("127.0.0.1", 0), handler) as server:
    port_path.write_text(str(server.server_address[1]))
    server.serve_forever()
PY
HTTP_PID=$!

HEALTH_PORT=""
for _ in $(seq 1 20); do
  if [[ -z "$HEALTH_PORT" && -s "$PORT_PATH" ]]; then
    HEALTH_PORT="$(cat "$PORT_PATH")"
  fi

  if [[ -n "$HEALTH_PORT" ]] && curl -fsS "http://127.0.0.1:${HEALTH_PORT}/health" > /dev/null 2>&1; then
    break
  fi
  sleep 0.1
done

if [[ -z "$HEALTH_PORT" ]] || ! curl -fsS "http://127.0.0.1:${HEALTH_PORT}/health" > /dev/null 2>&1; then
  echo "FAIL: test fixture did not start a healthy HTTP listener"
  cat "${TMP_DIR}/http.log"
  exit 1
fi

STDOUT_PATH="${TMP_DIR}/stdout.log"
STDERR_PATH="${TMP_DIR}/stderr.log"
if AYB_START_COMMAND='bash -lc "exit 1"' \
  AYB_HEALTH_URL="http://127.0.0.1:${HEALTH_PORT}/health" \
  AYB_ADMIN_PASSWORD='unused-for-test' \
  bash scripts/run-with-ayb.sh 'printf "unexpected success\n"' > "$STDOUT_PATH" 2> "$STDERR_PATH"; then
  echo "FAIL: scripts/run-with-ayb.sh reported success even though AYB never started"
  cat "$STDOUT_PATH"
  cat "$STDERR_PATH"
  exit 1
fi

if ! grep -q 'AYB process exited before health check passed.' "$STDERR_PATH"; then
  echo "FAIL: expected readiness failure message when AYB exits before startup completes"
  cat "$STDERR_PATH"
  exit 1
fi

echo "PASS: scripts/run-with-ayb.sh rejects unrelated healthy listeners when AYB startup fails"

SUCCESS_DIR="${TMP_DIR}/success"
mkdir -p "$SUCCESS_DIR"
printf 'ok\n' > "${SUCCESS_DIR}/health"
SUCCESS_PORT="$(python3 - <<'PY'
import socket

with socket.socket() as sock:
    sock.bind(("127.0.0.1", 0))
    print(sock.getsockname()[1])
PY
)"
SUCCESS_STDOUT_PATH="${TMP_DIR}/success.stdout.log"
SUCCESS_STDERR_PATH="${TMP_DIR}/success.stderr.log"

if ! AYB_START_COMMAND="python3 -m http.server ${SUCCESS_PORT} --bind 127.0.0.1 --directory \"${SUCCESS_DIR}\"" \
  AYB_HEALTH_URL="http://127.0.0.1:${SUCCESS_PORT}/health" \
  AYB_ADMIN_PASSWORD='unused-for-test' \
  bash scripts/run-with-ayb.sh 'printf "post-health-ok\n"' > "$SUCCESS_STDOUT_PATH" 2> "$SUCCESS_STDERR_PATH"; then
  echo "FAIL: scripts/run-with-ayb.sh did not run the post-health command after a healthy startup"
  cat "$SUCCESS_STDOUT_PATH"
  cat "$SUCCESS_STDERR_PATH"
  exit 1
fi

if ! grep -q '^post-health-ok$' "$SUCCESS_STDOUT_PATH"; then
  echo "FAIL: expected post-health command output after helper startup succeeded"
  cat "$SUCCESS_STDOUT_PATH"
  exit 1
fi

if curl -fsS "http://127.0.0.1:${SUCCESS_PORT}/health" > /dev/null 2>&1; then
  echo "FAIL: scripts/run-with-ayb.sh left the started server running after completion"
  exit 1
fi

echo "PASS: scripts/run-with-ayb.sh runs post-health commands and cleans up the started server"
