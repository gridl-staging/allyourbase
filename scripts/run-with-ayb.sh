#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 1 ]]; then
  echo "Usage: $0 \"<command-to-run-after-ayb-is-healthy>\"" >&2
  exit 1
fi

readonly POST_HEALTH_COMMAND="$1"
readonly AYB_START_COMMAND="${AYB_START_COMMAND:-./ayb start --foreground}"
readonly AYB_START_LOG="${AYB_START_LOG:-/tmp/ayb-e2e.log}"
readonly AYB_HEALTH_URL="${AYB_HEALTH_URL:-http://localhost:8090/health}"
readonly AYB_HEALTH_TIMEOUT_SECONDS="${AYB_HEALTH_TIMEOUT_SECONDS:-60}"
readonly AYB_HEALTH_POLL_INTERVAL_SECONDS="${AYB_HEALTH_POLL_INTERVAL_SECONDS:-0.5}"
readonly AYB_ADMIN_TOKEN_PATH="${AYB_ADMIN_TOKEN_PATH:-${HOME}/.ayb/admin-token}"
ADMIN_TOKEN_BACKUP_PATH=""
ADMIN_TOKEN_HAD_ORIGINAL=0

# Browser suites can issue hundreds of requests across workers. Keep local
# e2e runs from tripping the default API/auth limits unless the caller has
# already provided explicit limits.
export AYB_AUTH_RATE_LIMIT="${AYB_AUTH_RATE_LIMIT:-10000}"
export AYB_AUTH_ANONYMOUS_RATE_LIMIT="${AYB_AUTH_ANONYMOUS_RATE_LIMIT:-10000}"
export AYB_AUTH_RATE_LIMIT_AUTH="${AYB_AUTH_RATE_LIMIT_AUTH:-10000/min}"
export AYB_AUTH_ENABLED="${AYB_AUTH_ENABLED:-true}"
export AYB_AUTH_JWT_SECRET="${AYB_AUTH_JWT_SECRET:-devsecret-min-32-chars-long-000000}"
export AYB_RATE_LIMIT_API="${AYB_RATE_LIMIT_API:-10000/min}"
export AYB_RATE_LIMIT_API_ANONYMOUS="${AYB_RATE_LIMIT_API_ANONYMOUS:-10000/min}"

if ! [[ "$AYB_HEALTH_TIMEOUT_SECONDS" =~ ^[0-9]+$ ]] || (( AYB_HEALTH_TIMEOUT_SECONDS < 1 )); then
  echo "AYB_HEALTH_TIMEOUT_SECONDS must be a positive integer; got: $AYB_HEALTH_TIMEOUT_SECONDS" >&2
  exit 1
fi

if ! [[ "$AYB_HEALTH_POLL_INTERVAL_SECONDS" =~ ^[0-9]+([.][0-9]+)?$ ]] || ! awk -v value="$AYB_HEALTH_POLL_INTERVAL_SECONDS" 'BEGIN { exit !(value > 0) }'; then
  echo "AYB_HEALTH_POLL_INTERVAL_SECONDS must be a positive number; got: $AYB_HEALTH_POLL_INTERVAL_SECONDS" >&2
  exit 1
fi

print_start_log_excerpt() {
  local line_count=40
  if [[ -f "$AYB_START_LOG" ]]; then
    echo "AYB startup log excerpt ($AYB_START_LOG):" >&2
    tail -n "$line_count" "$AYB_START_LOG" >&2 || true
  fi
}

remove_admin_token_file() {
  rm -f "$AYB_ADMIN_TOKEN_PATH" 2>/dev/null || true
}

report_startup_failure() {
  echo "AYB process exited before health check passed." >&2
  print_start_log_excerpt
  return 1
}

admin_token_ready() {
  [[ -n "${AYB_ADMIN_PASSWORD:-}" || -s "$AYB_ADMIN_TOKEN_PATH" ]]
}

prepare_admin_token_file() {
  if [[ -f "$AYB_ADMIN_TOKEN_PATH" ]]; then
    ADMIN_TOKEN_BACKUP_PATH="$(mktemp)"
    cp "$AYB_ADMIN_TOKEN_PATH" "$ADMIN_TOKEN_BACKUP_PATH"
    ADMIN_TOKEN_HAD_ORIGINAL=1
  fi

  remove_admin_token_file
}

# Restore the original admin-token file (or remove the test-generated one) so
# the wrapped run never leaves behind stale credentials from the temporary AYB.
restore_admin_token_if_needed() {
  if (( ADMIN_TOKEN_HAD_ORIGINAL )); then
    mkdir -p "$(dirname "$AYB_ADMIN_TOKEN_PATH")"
    cp "$ADMIN_TOKEN_BACKUP_PATH" "$AYB_ADMIN_TOKEN_PATH"
  else
    remove_admin_token_file
  fi

  if [[ -n "$ADMIN_TOKEN_BACKUP_PATH" ]]; then
    rm -f "$ADMIN_TOKEN_BACKUP_PATH" 2>/dev/null || true
  fi
}

ayb_process_running() {
  local ayb_pid="$1"

  if ! kill -0 "$ayb_pid" 2>/dev/null; then
    return 1
  fi

  local process_state
  process_state="$(ps -o stat= -p "$ayb_pid" 2>/dev/null | tr -d '[:space:]' || true)"
  if [[ -z "$process_state" || "$process_state" == *Z* ]]; then
    return 1
  fi

  return 0
}

# Poll health endpoint + admin-token readiness until both succeed or deadline
# expires. Exits with error if the AYB process dies before becoming healthy.
wait_for_ayb_readiness() {
  local ayb_pid="$1"
  local deadline=$((SECONDS + AYB_HEALTH_TIMEOUT_SECONDS))

  while true; do
    if ! ayb_process_running "$ayb_pid"; then
      report_startup_failure
    fi

    if curl -fsS "$AYB_HEALTH_URL" > /dev/null 2>&1 && admin_token_ready; then
      if ! ayb_process_running "$ayb_pid"; then
        report_startup_failure
      fi
      return 0
    fi

    if (( SECONDS >= deadline )); then
      echo "Timed out waiting for AYB health check at $AYB_HEALTH_URL after ${AYB_HEALTH_TIMEOUT_SECONDS}s." >&2
      print_start_log_excerpt
      return 1
    fi

    sleep "$AYB_HEALTH_POLL_INTERVAL_SECONDS"
  done
}

prepare_admin_token_file
bash -lc "$AYB_START_COMMAND" > "$AYB_START_LOG" 2>&1 &
AYB_PID=$!

cleanup() {
  kill "$AYB_PID" 2>/dev/null || true
  wait "$AYB_PID" 2>/dev/null || true
  restore_admin_token_if_needed
}
trap cleanup EXIT

wait_for_ayb_readiness "$AYB_PID"
bash -lc "$POST_HEALTH_COMMAND"
