#!/usr/bin/env bash
# Shared assertion helpers for bash contract tests.
# Source this file: source "$(dirname "$0")/bash_assert_helpers.sh"

fail() {
  echo "FAIL: $1"
  exit 1
}

assert_contains() {
  local file_path="$1"
  local needle="$2"
  local message="$3"
  grep -Fq -- "$needle" "$file_path" || fail "$message"
}

assert_not_contains() {
  local file_path="$1"
  local needle="$2"
  local message="$3"
  if grep -Fq -- "$needle" "$file_path"; then
    fail "$message"
  fi
}
