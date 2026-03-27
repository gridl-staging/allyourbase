#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

REGULAR_ARGS=(
  go test
  -p 1
  -race
  -count=1
  -tags=integration
)

SPECIAL_ARGS=(
  go test
  -p 1
  -parallel 1
  -race
  -count=1
  -tags=integration
)

SPECIAL_PACKAGES=(
  "github.com/allyourbase/ayb/internal/api"
  "github.com/allyourbase/ayb/internal/server"
)

all_packages=()
while IFS= read -r pkg; do
  all_packages+=("$pkg")
done < <(go list ./...)

regular_packages=()
for pkg in "${all_packages[@]}"; do
  skip=false
  for special in "${SPECIAL_PACKAGES[@]}"; do
    if [[ "$pkg" == "$special" ]]; then
      skip=true
      break
    fi
  done
  if [[ "$skip" == false ]]; then
    regular_packages+=("$pkg")
  fi
done

if ((${#regular_packages[@]} > 0)); then
  # Package-level serialization is enough for the broad integration sweep.
  # Leaving test-level parallelism enabled avoids timing out packages with lots
  # of t.Parallel() unit coverage, such as internal/storage.
  go run ./internal/testutil/cmd/testpg -- "${REGULAR_ARGS[@]}" "${regular_packages[@]}"
fi

for pkg in "${SPECIAL_PACKAGES[@]}"; do
  # These packages have large unit-test bodies that already run during make test.
  # During the integration phase we run only integration-tagged Test* functions
  # so we do not re-run unrelated unit coverage in the same package process.
  go run ./internal/testutil/cmd/testpg -- \
    bash scripts/run-package-integration-tests.sh "$pkg" -- "${SPECIAL_ARGS[@]:2}"
done
