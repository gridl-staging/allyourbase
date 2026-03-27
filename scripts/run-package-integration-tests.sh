#!/usr/bin/env bash

set -euo pipefail

# Run only integration-tagged Test* functions for a single Go package.
# This avoids re-running large unit-test bodies during the integration phase
# while still deriving selection from the source tree instead of a manual list.

if (($# < 1)); then
  echo "usage: $0 <go-package> [-- <go test args...>]" >&2
  exit 1
fi

package_path="$1"
shift

go_test_args=()
if (($# > 0)); then
  if [[ "$1" != "--" ]]; then
    echo "expected '--' before go test args" >&2
    exit 1
  fi
  shift
  go_test_args=("$@")
fi

package_dir="$(go list -f '{{.Dir}}' "$package_path")"

integration_files=()
if command -v rg >/dev/null 2>&1; then
  while IFS= read -r file; do
    integration_files+=("$file")
  done < <(rg -l '^//go:build integration$' "$package_dir"/*_test.go 2>/dev/null || true)
else
  while IFS= read -r file; do
    integration_files+=("$file")
  done < <(grep -l '^//go:build integration$' "$package_dir"/*_test.go 2>/dev/null || true)
fi

if ((${#integration_files[@]} == 0)); then
  echo "no integration-tagged test files found for $package_path" >&2
  exit 1
fi

test_names=()
while IFS= read -r name; do
  if [[ "$name" == "TestMain" ]]; then
    continue
  fi
  test_names+=("$name")
done < <(sed -nE 's/^func (Test[A-Za-z0-9_]+)\(.*/\1/p' "${integration_files[@]}" | sort -u)

if ((${#test_names[@]} == 0)); then
  echo "no Test* functions found in integration-tagged files for $package_path" >&2
  exit 1
fi

unit_files=()
for file in "$package_dir"/*_test.go; do
  skip=false
  for integration_file in "${integration_files[@]}"; do
    if [[ "$file" == "$integration_file" ]]; then
      skip=true
      break
    fi
  done
  if [[ "$skip" == false ]]; then
    unit_files+=("$file")
  fi
done

if ((${#unit_files[@]} > 0)); then
  duplicate_names=()
  while IFS= read -r name; do
    duplicate_names+=("$name")
  done < <(
    comm -12 \
      <(printf '%s\n' "${test_names[@]}" | sort -u) \
      <(sed -nE 's/^func (Test[A-Za-z0-9_]+)\(.*/\1/p' "${unit_files[@]}" | grep -v '^TestMain$' | sort -u)
  )
  if ((${#duplicate_names[@]} > 0)); then
    printf 'integration test names collide with non-integration tests in %s:\n' "$package_path" >&2
    printf '  %s\n' "${duplicate_names[@]}" >&2
    printf 'rename the integration tests or add explicit !integration tags to the unit tests\n' >&2
    exit 1
  fi
fi

run_regex='^('
for i in "${!test_names[@]}"; do
  if ((i > 0)); then
    run_regex+='|'
  fi
  run_regex+="${test_names[$i]}"
done
run_regex+=')$'

go test "${go_test_args[@]}" -run "$run_regex" "$package_path"
