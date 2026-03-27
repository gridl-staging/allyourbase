#!/usr/bin/env bash
set -euo pipefail

repo_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
guide_dir="${repo_dir}/docs-site/guide"
readme_doc="${repo_dir}/README.md"
kotlin_guide="${guide_dir}/kotlin-sdk.md"
swift_guide="${guide_dir}/swift-sdk.md"
index_guide="${repo_dir}/docs-site/index.md"
getting_started_guide="${guide_dir}/getting-started.md"
deployment_guide="${guide_dir}/deployment.md"
postgis_guide="${guide_dir}/postgis.md"
audited_guides=(
  "${guide_dir}/admin-dashboard.md"
  "${guide_dir}/ai-vector.md"
  "${guide_dir}/email-templates.md"
  "${guide_dir}/email.md"
  "${guide_dir}/flutter-sdk.md"
  "${guide_dir}/go-sdk.md"
  "${guide_dir}/job-queue.md"
  "${guide_dir}/kotlin-sdk.md"
  "${guide_dir}/log-drains.md"
  "${guide_dir}/mcp.md"
  "${guide_dir}/organizations.md"
  "${guide_dir}/push-notifications.md"
  "${guide_dir}/python-sdk.md"
  "${guide_dir}/security.md"
  "${guide_dir}/swift-sdk.md"
)

fail() {
  echo "FAIL: $1"
  exit 1
}

require_file_match() {
  local file="$1"
  local pattern="$2"
  local message="$3"

  if ! rg -q -- "$pattern" "$file"; then
    fail "$message"
  fi
}

require_text_match() {
  local text="$1"
  local pattern="$2"
  local message="$3"

  if ! printf "%s\n" "$text" | rg -q -- "$pattern"; then
    fail "$message"
  fi
}

reject_files_match() {
  local pattern="$1"
  local message="$2"
  shift 2

  if rg -q -- "$pattern" "$@"; then
    fail "$message"
  fi
}

extract_kotlin_batch_code_block() {
  awk '
    /^Batch:/ { in_batch = 1; next }
    in_batch && /^```kotlin$/ { in_code = 1; next }
    in_code && /^```$/ { exit }
    in_code { print }
  ' "$kotlin_guide"
}

batch_code_block="$(extract_kotlin_batch_code_block)"

for guide in "${audited_guides[@]}"; do
  require_file_match "$guide" "<!-- audited 2026-03-[0-9]{2} -->" "$(basename "$guide") missing audited marker"
done

if rg -q "Stage [0-9]" "${audited_guides[@]}"; then
  rg -n "Stage [0-9]" "${audited_guides[@]}"
  fail "audited guides leak internal stage labels"
fi

require_text_match "$batch_code_block" "import kotlinx\.serialization\.json\.buildJsonObject" "kotlin-sdk.md batch sample missing buildJsonObject import"
require_text_match "$batch_code_block" "import kotlinx\.serialization\.json\.put" "kotlin-sdk.md batch sample missing put import"

reject_files_match "\.package\(url: \"https://github\.com/allyourbase/allyourbase\.git\"" "swift-sdk.md contains invalid repo-root SwiftPM URL example" "$swift_guide"
require_file_match "$swift_guide" "\.package\(path: \"\.\./sdk_swift\"\)" "swift-sdk.md missing local sdk_swift package path example"
require_file_match "${guide_dir}/go-sdk.md" "go mod edit -replace=github\.com/allyourbase/ayb/sdk_go=" "go-sdk.md missing verified local replace workflow"
require_file_match "${guide_dir}/go-sdk.md" 'Public `go get` is not available yet' "go-sdk.md missing public module-path caveat"
reject_files_match "brew install gridlhq/tap/ayb" "public docs still advertise a nonexistent Homebrew tap" "$readme_doc" "$index_guide" "$getting_started_guide"
reject_files_match "ghcr\.io/gridlhq/allyourbase" "public docs still advertise a private GHCR image" "$readme_doc" "$index_guide" "$getting_started_guide" "$deployment_guide" "$postgis_guide"
reject_files_match "https://allyourbase\\.io/install\\.sh" "public docs still advertise the retired bare-domain installer URL" "$readme_doc" "$index_guide" "$getting_started_guide" "$deployment_guide"
require_file_match "$deployment_guide" "docker build -t ayb-local" "deployment.md missing local Docker build instructions"
require_file_match "$deployment_guide" "image: ayb-local" "deployment.md missing local image tag in compose example"
require_file_match "$postgis_guide" "image: ayb-local" "postgis.md missing local image tag in compose example"

echo "PASS: non-JS SDK guide install/sample checks"
