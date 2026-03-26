#!/usr/bin/env bash
set -euo pipefail

DOCKER_BIN="${DOCKER_BIN:-docker}"
AYB_DOCKER_IMAGE="${AYB_DOCKER_IMAGE:-allyourbase/ayb:latest}"
AYB_DOCKER_CONTAINER="${AYB_DOCKER_CONTAINER:-ayb-docker-runtime-smoke}"
AYB_DOCKER_PORT="${AYB_DOCKER_PORT:-18093}"
AYB_ADMIN_PASSWORD="${AYB_ADMIN_PASSWORD:-DockerSmokeAdminPass123!}"
AYB_AUTH_JWT_SECRET="${AYB_AUTH_JWT_SECRET:-docker-smoke-jwt-secret-at-least-32-chars}"
AYB_SMOKE_EMAIL="${AYB_SMOKE_EMAIL:-docker.smoke@example.com}"
AYB_SMOKE_PASSWORD="${AYB_SMOKE_PASSWORD:-DockerSmokeUserPass123!}"

# Keep all mutable runtime state outside the repo tree by default so smoke runs
# never create Git noise under _dev/release/evidence or local data folders.
RUNTIME_ROOT="${AYB_DOCKER_RUNTIME_ROOT:-$(mktemp -d /tmp/ayb-docker-smoke.XXXXXX)}"
PGDATA_DIR="${AYB_DOCKER_PGDATA_DIR:-$RUNTIME_ROOT/pgdata}"
STORAGE_DIR="${AYB_DOCKER_STORAGE_DIR:-$RUNTIME_ROOT/storage}"

cleanup() {
  "$DOCKER_BIN" rm -f "$AYB_DOCKER_CONTAINER" >/dev/null 2>&1 || true
}
trap cleanup EXIT

mkdir -p "$PGDATA_DIR" "$STORAGE_DIR"
chmod 0777 "$PGDATA_DIR" "$STORAGE_DIR"

BASE_URL="http://127.0.0.1:${AYB_DOCKER_PORT}"

wait_for_health() {
  local attempts="${1:-90}"
  local i
  for i in $(seq 1 "$attempts"); do
    if curl -fsS "${BASE_URL}/health" >/tmp/ayb-docker-health.json 2>/dev/null; then
      return 0
    fi
    sleep 1
  done
  echo "health check timed out for ${BASE_URL}/health" >&2
  return 1
}

require_http() {
  local expected="$1"
  local actual="$2"
  local label="$3"
  if [[ "$expected" != "$actual" ]]; then
    echo "${label} failed: expected HTTP ${expected}, got ${actual}" >&2
    return 1
  fi
}

start_container() {
  "$DOCKER_BIN" rm -f "$AYB_DOCKER_CONTAINER" >/dev/null 2>&1 || true
  "$DOCKER_BIN" run -d \
    --name "$AYB_DOCKER_CONTAINER" \
    -p "${AYB_DOCKER_PORT}:8090" \
    -e "AYB_ADMIN_PASSWORD=${AYB_ADMIN_PASSWORD}" \
    -e "AYB_AUTH_ENABLED=true" \
    -e "AYB_AUTH_JWT_SECRET=${AYB_AUTH_JWT_SECRET}" \
    -e "AYB_STORAGE_ENABLED=true" \
    -e "AYB_DATABASE_EMBEDDED_DATA_DIR=/ayb_pgdata" \
    -e "AYB_STORAGE_LOCAL_PATH=/ayb_storage" \
    -v "${PGDATA_DIR}:/ayb_pgdata" \
    -v "${STORAGE_DIR}:/ayb_storage" \
    "$AYB_DOCKER_IMAGE" >/dev/null
}

start_container
wait_for_health
printf 'health: %s\n' "$(cat /tmp/ayb-docker-health.json)"

admin_code="$(
  curl -sS -o /tmp/ayb-docker-admin-auth.json -w '%{http_code}' \
    -X POST "${BASE_URL}/api/admin/auth" \
    -H 'Content-Type: application/json' \
    --data "{\"password\":\"${AYB_ADMIN_PASSWORD}\"}"
)"
require_http 200 "$admin_code" "admin auth"
admin_token="$(jq -r '.token' /tmp/ayb-docker-admin-auth.json)"
[[ -n "$admin_token" && "$admin_token" != "null" ]]
echo "admin auth: ok"

register_code="$(
  curl -sS -o /tmp/ayb-docker-register.json -w '%{http_code}' \
    -X POST "${BASE_URL}/api/auth/register" \
    -H 'Content-Type: application/json' \
    --data "{\"email\":\"${AYB_SMOKE_EMAIL}\",\"password\":\"${AYB_SMOKE_PASSWORD}\"}"
)"
require_http 201 "$register_code" "register"
register_token="$(jq -r '.token' /tmp/ayb-docker-register.json)"
[[ -n "$register_token" && "$register_token" != "null" ]]
echo "register: ok"

login_code="$(
  curl -sS -o /tmp/ayb-docker-login.json -w '%{http_code}' \
    -X POST "${BASE_URL}/api/auth/login" \
    -H 'Content-Type: application/json' \
    --data "{\"email\":\"${AYB_SMOKE_EMAIL}\",\"password\":\"${AYB_SMOKE_PASSWORD}\"}"
)"
require_http 200 "$login_code" "login"
login_token="$(jq -r '.token' /tmp/ayb-docker-login.json)"
[[ -n "$login_token" && "$login_token" != "null" ]]
echo "login: ok"

payload="docker runtime persistence payload $(date +%s)"
printf '%s' "$payload" >/tmp/ayb-docker-payload.txt

upload_code="$(
  curl -sS -o /tmp/ayb-docker-upload.json -w '%{http_code}' \
    -X POST "${BASE_URL}/api/storage/journey" \
    -H "Authorization: Bearer ${login_token}" \
    -F 'file=@/tmp/ayb-docker-payload.txt;type=text/plain'
)"
require_http 201 "$upload_code" "storage upload"
uploaded_name="$(jq -r '.name' /tmp/ayb-docker-upload.json)"
[[ -n "$uploaded_name" && "$uploaded_name" != "null" ]]
printf 'storage upload: %s\n' "$uploaded_name"

list_code="$(
  curl -sS -o /tmp/ayb-docker-storage-list.json -w '%{http_code}' \
    -H "Authorization: Bearer ${login_token}" \
    "${BASE_URL}/api/storage/journey"
)"
require_http 200 "$list_code" "storage list"
jq -e --arg name "$uploaded_name" '.items[] | select(.name == $name)' /tmp/ayb-docker-storage-list.json >/dev/null
echo "storage list: ok"

fetch_code="$(
  curl -sS -o /tmp/ayb-docker-storage-file.txt -w '%{http_code}' \
    "${BASE_URL}/api/storage/journey/${uploaded_name}"
)"
require_http 200 "$fetch_code" "storage fetch before restart"
[[ "$(cat /tmp/ayb-docker-storage-file.txt)" == "$payload" ]]
echo "storage fetch before restart: ok"

"$DOCKER_BIN" restart "$AYB_DOCKER_CONTAINER" >/dev/null
wait_for_health
echo "restart health: ok"

relogin_code="$(
  curl -sS -o /tmp/ayb-docker-relogin.json -w '%{http_code}' \
    -X POST "${BASE_URL}/api/auth/login" \
    -H 'Content-Type: application/json' \
    --data "{\"email\":\"${AYB_SMOKE_EMAIL}\",\"password\":\"${AYB_SMOKE_PASSWORD}\"}"
)"
require_http 200 "$relogin_code" "login after restart"
relogin_token="$(jq -r '.token' /tmp/ayb-docker-relogin.json)"
[[ -n "$relogin_token" && "$relogin_token" != "null" ]]
echo "login after restart: ok"

fetch_after_code="$(
  curl -sS -o /tmp/ayb-docker-storage-file-after.txt -w '%{http_code}' \
    "${BASE_URL}/api/storage/journey/${uploaded_name}"
)"
require_http 200 "$fetch_after_code" "storage fetch after restart"
[[ "$(cat /tmp/ayb-docker-storage-file-after.txt)" == "$payload" ]]
echo "storage fetch after restart: ok"

list_after_code="$(
  curl -sS -o /tmp/ayb-docker-storage-list-after.json -w '%{http_code}' \
    -H "Authorization: Bearer ${relogin_token}" \
    "${BASE_URL}/api/storage/journey"
)"
require_http 200 "$list_after_code" "storage list after restart"
jq -e --arg name "$uploaded_name" '.items[] | select(.name == $name)' /tmp/ayb-docker-storage-list-after.json >/dev/null
echo "storage list after restart: ok"

printf '\nSMOKE RESULT: PASS\n'
printf 'runtime_root=%s\n' "$RUNTIME_ROOT"
