#!/bin/sh
set -eu

AYB_USER="${AYB_USER:-ayb}"
AYB_HOME="${HOME:-/home/ayb}"
AYB_RUNTIME_USER="$AYB_USER"
AYB_RUNTIME_UID="$(id -u "$AYB_USER")"
AYB_RUNTIME_GID="$(id -g "$AYB_USER")"

warn() {
  printf 'warning: %s\n' "$*" >&2
}

lookup_group_name() {
  gid="$1"
  awk -F: -v gid="$gid" '$3 == gid { print $1; exit }' /etc/group
}

lookup_user_name() {
  uid="$1"
  awk -F: -v uid="$uid" '$3 == uid { print $1; exit }' /etc/passwd
}

ensure_runtime_group() {
  gid="$1"
  name="$(lookup_group_name "$gid")"
  if [ -n "$name" ]; then
    printf '%s\n' "$name"
    return 0
  fi
  name="aybhost"
  if grep -q "^${name}:" /etc/group; then
    name="${name}${gid}"
  fi
  printf '%s:x:%s:\n' "$name" "$gid" >> /etc/group
  printf '%s\n' "$name"
}

ensure_runtime_user() {
  uid="$1"
  gid="$2"
  name="$(lookup_user_name "$uid")"
  if [ -n "$name" ]; then
    printf '%s\n' "$name"
    return 0
  fi
  ensure_runtime_group "$gid" >/dev/null
  name="aybhost"
  if grep -q "^${name}:" /etc/passwd; then
    name="${name}${uid}"
  fi
  printf '%s:x:%s:%s:AYB runtime user:%s:/sbin/nologin\n' "$name" "$uid" "$gid" "$AYB_HOME" >> /etc/passwd
  printf '%s\n' "$name"
}

configure_runtime_user_from_owner() {
  owner_uid="$1"
  owner_gid="$2"
  if [ "$owner_uid" = "$AYB_RUNTIME_UID" ] && [ "$owner_gid" = "$AYB_RUNTIME_GID" ]; then
    return 0
  fi
  AYB_RUNTIME_USER="$(ensure_runtime_user "$owner_uid" "$owner_gid")"
  AYB_RUNTIME_UID="$owner_uid"
  AYB_RUNTIME_GID="$owner_gid"
}

configure_runtime_user_from_pgdata_dir() {
  dir="$1"
  if [ -z "$dir" ] || [ ! -e "$dir" ]; then
    return 0
  fi
  owner_uid="$(stat -c '%u' "$dir")"
  owner_gid="$(stat -c '%g' "$dir")"
  if [ "$owner_uid" = "0" ]; then
    return 0
  fi
  configure_runtime_user_from_owner "$owner_uid" "$owner_gid"
}

run_as_runtime() {
  su-exec "$AYB_RUNTIME_USER" "$@"
}

ensure_writable_dir() {
  dir="$1"
  if [ -z "$dir" ]; then
    return 0
  fi
  mkdir -p "$dir"
  if run_as_runtime test -w "$dir"; then
    return 0
  fi
  if ! chown -R "${AYB_RUNTIME_UID}:${AYB_RUNTIME_GID}" "$dir" 2>/dev/null; then
    warn "could not change ownership for $dir; continuing if write access already works"
  fi
  if run_as_runtime test -w "$dir"; then
    return 0
  fi
  warn "$dir is not writable for ${AYB_RUNTIME_USER}"
  return 1
}

if [ "$(id -u)" -eq 0 ]; then
  configure_runtime_user_from_pgdata_dir "${AYB_DATABASE_EMBEDDED_DATA_DIR:-}"
  ensure_writable_dir "$AYB_HOME"
  ensure_writable_dir "$AYB_HOME/.ayb"
  ensure_writable_dir "$AYB_HOME/.ayb/data"
  ensure_writable_dir "$AYB_HOME/.ayb/logs"
  ensure_writable_dir "$AYB_HOME/.ayb/run"

  if [ -n "${AYB_DATABASE_EMBEDDED_DATA_DIR:-}" ]; then
    ensure_writable_dir "${AYB_DATABASE_EMBEDDED_DATA_DIR}"
  fi

  if [ -n "${AYB_STORAGE_LOCAL_PATH:-}" ]; then
    ensure_writable_dir "${AYB_STORAGE_LOCAL_PATH}"
  fi

  export USER="$AYB_RUNTIME_USER"
  exec su-exec "$AYB_RUNTIME_USER" "$@"
fi

exec "$@"
