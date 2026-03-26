#!/usr/bin/env bash
set -euo pipefail

readonly SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

readonly FILE_LINE_LIMIT=500
readonly SCAN_ROOT="${CHECK_FILE_SIZES_ROOT:-$REPO_ROOT}"
readonly ALLOWLIST_PATH="${CHECK_FILE_SIZES_ALLOWLIST:-$REPO_ROOT/scripts/allowlist-oversized.txt}"

normalize_path() {
  local path="$1"
  path="${path#./}"
  echo "$path"
}

if [[ -f "$ALLOWLIST_PATH" ]]; then
  while IFS= read -r entry; do
    entry="$(echo "$entry" | sed 's/[[:space:]]*$//')"
    [[ -z "$entry" || "$entry" =~ ^# ]] && continue

    local_path="${entry%%:*}"
    line_count="${entry##*:}"

    if [[ -z "$local_path" || -z "$line_count" || "$local_path" == "$entry" ]]; then
      echo "Invalid allowlist entry in $ALLOWLIST_PATH: $entry" >&2
      exit 1
    fi
    if ! [[ "$line_count" =~ ^[0-9]+$ ]]; then
      echo "Invalid line count in allowlist entry $entry" >&2
      exit 1
    fi

    normalized_path="$(normalize_path "$local_path")"
    if [[ "$normalized_path" != "$local_path" ]]; then
      echo "Invalid allowlist path (must not start with ./): $entry" >&2
      exit 1
    fi
  done < "$ALLOWLIST_PATH"
fi

violations=()

while IFS= read -r -d '' file; do
  relative_path="$(normalize_path "${file#$SCAN_ROOT/}")"
  line_count="$(wc -l < "$file" | tr -d '[:space:]')"

  if (( line_count <= FILE_LINE_LIMIT )); then
    continue
  fi

  allowlisted_count=""
  if [[ -f "$ALLOWLIST_PATH" ]]; then
    allowlisted_count="$(
      awk -F: -v path="$relative_path" '
        $0 !~ /^[[:space:]]*#/ && NF == 2 && $1 == path { print $2; exit }
      ' "$ALLOWLIST_PATH"
    )"
  fi
  if [[ -z "$allowlisted_count" ]]; then
    violations+=("$relative_path:$line_count (missing from allowlist)")
    continue
  fi

  if [[ "$allowlisted_count" != "$line_count" ]]; then
    violations+=("$relative_path:$line_count (allowlist has $allowlisted_count)")
  fi
done < <(find "$SCAN_ROOT" -name '*.go' -type f ! -name '*_test.go' ! -path "$SCAN_ROOT/vendor/*" ! -path "$SCAN_ROOT/_dev/*" -print0)

if (( ${#violations[@]} > 0 )); then
  echo "Go file-size guardrail failed (limit ${FILE_LINE_LIMIT} lines)."
  echo "Oversized files not covered by allowlist or with stale counts:"
  printf '%s\n' "${violations[@]}"
  exit 1
fi

echo "Go file-size guardrail passed."
