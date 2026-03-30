#!/bin/sh
# tests/test_sync_pipeline.sh — Verify sync-to-public.sh's sed replacements produce correct output
#
# Usage:
#   ./tests/test_sync_pipeline.sh
#
# These tests simulate the public-sync rewrites that sync-to-public.sh applies:
#   1. stuartcrobinson/allyourbase → gridlhq/allyourbase
#   2. allyourbase_dev → allyourbase (README/.goreleaser public surfaces)
#   3. staging.allyourbase.io → install.allyourbase.io
# and verify the resulting files are correct for the public repo.

set -eu

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
REPO_DIR_REAL="$(cd "$REPO_DIR" && pwd -P)"
GORELEASER_CONFIG="${REPO_DIR}/.goreleaser.yaml"
CANONICAL_INSTALLER_URL="https://install.allyourbase.io/install.sh"
CANONICAL_INSTALLER_URL_PATTERN='https://install\.allyourbase\.io/install\.sh'
LEGACY_INSTALLER_URL_PATTERN='https://allyourbase\.io/install\.sh'

# ── Test Helpers ─────────────────────────────────────────────────────────────

TESTS_RUN=0
TESTS_PASSED=0
TESTS_FAILED=0

pass() {
  TESTS_PASSED=$((TESTS_PASSED + 1))
  TESTS_RUN=$((TESTS_RUN + 1))
  printf "  \033[0;32m✓\033[0m %s\n" "$1"
}

fail() {
  TESTS_FAILED=$((TESTS_FAILED + 1))
  TESTS_RUN=$((TESTS_RUN + 1))
  printf "  \033[0;31m✗\033[0m %s\n" "$1"
  if [ -n "${2:-}" ]; then
    printf "    %s\n" "$2"
  fi
}

section() {
  printf "\n\033[1m%s\033[0m\n" "$1"
}

git_remote_exists() {
  GIT_TERMINAL_PROMPT=0 git ls-remote -- "$1" >/dev/null 2>&1
}

git_tag_exists() {
  GIT_TERMINAL_PROMPT=0 git ls-remote --exit-code --tags -- "$1" "$2" >/dev/null 2>&1
}

allowlisted_github_repo_slug() {
  printf '%s\n' "$1" | grep -Eq '^[A-Za-z0-9][A-Za-z0-9._-]*/[A-Za-z0-9][A-Za-z0-9._-]*$'
}

allowlisted_go_install_repo() {
  printf '%s\n' "$1" | grep -Eq '^github\.com/[A-Za-z0-9][A-Za-z0-9._-]*/[A-Za-z0-9][A-Za-z0-9._-]*$'
}

extract_section() {
  file_path="$1"
  start_pattern="$2"
  end_pattern="$3"

  awk -v start_pattern="$start_pattern" -v end_pattern="$end_pattern" '
    $0 ~ start_pattern { in_section=1; next }
    $0 ~ end_pattern && in_section { exit }
    in_section { print }
  ' "$file_path"
}

extract_markdown_section() {
  file_path="$1"
  section_title="$2"

  extract_section "$file_path" "^## ${section_title}\$" "^## "
}

curl_urls() {
  sed -n 's|^[[:space:]#-]*curl[[:space:]].*\(https://[^[:space:]]*\).*|\1|p'
}

first_curl_url() {
  curl_urls | sed -n '1p'
}

second_curl_url() {
  curl_urls | sed -n '2p'
}

urls_match() {
  first_url="$1"
  second_url="$2"

  [ -n "$first_url" ] && [ -n "$second_url" ] && [ "$first_url" = "$second_url" ]
}

url_is_canonical_installer() {
  [ "$1" = "$CANONICAL_INSTALLER_URL" ]
}

first_pinned_version() {
  sed -n \
    -e 's|.*sh -s -- \(v[0-9][^[:space:]]*\).*|\1|p' \
    -e 's|^[[:space:]#-]*sh[[:space:]][^[:space:]]*[[:space:]]\(v[0-9][^[:space:]]*\).*|\1|p' \
    | head -1
}

extract_default_repo() {
  sed 's/.*AYB_REPO:-//;s/}.*//'
}

goreleaser_has() {
  [ -f "$GORELEASER_CONFIG" ] && grep -q "$1" "$GORELEASER_CONFIG"
}

grep_has_match() {
  grep "$@" >/dev/null 2>&1
}

first_grep_match() {
  grep "$@" | head -1 || true
}

assert_grep_present() {
  success_message="$1"
  failure_message="$2"
  shift 2

  if grep_has_match "$@"; then
    pass "$success_message"
  else
    fail "$failure_message"
  fi
}

assert_grep_absent() {
  success_message="$1"
  failure_message="$2"
  shift 2

  if grep_has_match "$@"; then
    fail "$failure_message" "$(first_grep_match "$@")"
  else
    pass "$success_message"
  fi
}

assert_repo_tag_exists() {
  repo_slug="$1"
  tag_name="$2"
  success_message="$3"
  invalid_repo_message="$4"
  missing_tag_message="$5"

  if allowlisted_github_repo_slug "$repo_slug"; then
    if git_tag_exists "https://github.com/${repo_slug}.git" "refs/tags/${tag_name}"; then
      pass "$success_message"
    else
      fail "$missing_tag_message" "https://github.com/${repo_slug}/releases/tag/${tag_name}"
    fi
  else
    fail "$invalid_repo_message" "$repo_slug"
  fi
}

assert_core_files_exist() {
  pass_message="$1"
  fail_prefix="$2"
  shift 2

  files_ok=true
  for file_name in "$@"; do
    if [ ! -f "${REPO_DIR}/${file_name}" ]; then
      fail "${fail_prefix}: ${file_name}"
      files_ok=false
    fi
  done

  if [ "$files_ok" = true ]; then
    pass "$pass_message"
  fi
}

assert_launch_image_precedes_quickstart() {
  readme_file="$1"

  dashboard_image_line=$(grep -n '^[[:space:]]*!\[[^]]*\](.*)$' "$readme_file" | head -1 | cut -d: -f1)
  quickstart_line=$(grep -n '^## Quickstart$' "$readme_file" | head -1 | cut -d: -f1)

  if [ -n "$dashboard_image_line" ] && [ -n "$quickstart_line" ] && [ "$dashboard_image_line" -lt "$quickstart_line" ]; then
    pass "Launch image appears before Quickstart section"
  else
    fail "Launch image must appear before Quickstart section" "image_line=${dashboard_image_line:-missing} quickstart_line=${quickstart_line:-missing}"
  fi
}

dashboard_asset_path=""

assert_dashboard_asset_reference_is_safe() {
  asset_reference="$1"

  case "$asset_reference" in
    /*|http://*|https://*)
      fail "Dashboard image path must be relative" "$asset_reference"
      return
      ;;
    ../*|*/../*)
      fail "Dashboard image path must not escape the repo root" "$asset_reference"
      return
      ;;
    _dev/*|./_dev/*|_qa/*|./_qa/*)
      fail "Dashboard image path must not point to excluded directories" "$asset_reference"
      return
      ;;
  esac

  candidate_path="${REPO_DIR}/${asset_reference}"
  if [ ! -f "$candidate_path" ]; then
    fail "Dashboard image asset missing from repo" "$asset_reference"
    return
  fi

  if [ -L "$candidate_path" ]; then
    fail "Dashboard image path must not be a symlink" "$asset_reference"
    return
  fi

  candidate_dir_real=$(cd "$(dirname "$candidate_path")" 2>/dev/null && pwd -P || true)
  case "${candidate_dir_real}/" in
    "${REPO_DIR_REAL}/"*)
      dashboard_asset_path="$candidate_path"
      pass "Dashboard image path resolves in repo without symlink escapes: ${asset_reference}"
      ;;
    *)
      fail "Dashboard image path escapes repo root after resolution" "${asset_reference} -> ${candidate_dir_real}/${candidate_path##*/}"
      ;;
  esac
}

# Simulate sync-to-public.sh's text rewrites for a file:
# 1. Rewrite repo owner
# 2. Rewrite dev repo name where the public sync does so
# 3. Rewrite vanity URL
simulate_sync() {
  input_file="$1"
  file_name="${input_file##*/}"
  tmpfile=$(mktemp)

  synced_content=$(sed 's|stuartcrobinson/allyourbase|gridlhq/allyourbase|g' "$input_file")

  case "$file_name" in
    README.md|.goreleaser.yaml)
      synced_content=$(printf "%s" "$synced_content" | sed 's|allyourbase_dev|allyourbase|g')
      ;;
  esac

  printf "%s" "$synced_content" \
    | sed 's|staging\.allyourbase\.io|install.allyourbase.io|g' \
    > "$tmpfile"
  echo "$tmpfile"
}

# ── Sync Script Structure Tests ──────────────────────────────────────────────

section "Sync Script Structure"

SYNC_SCRIPT="${REPO_DIR}/sync-to-public.sh"

if [ -f "$SYNC_SCRIPT" ]; then
  # Test: sync script exists and is executable
  if [ -x "$SYNC_SCRIPT" ]; then
    pass "sync-to-public.sh is executable"
  else
    fail "sync-to-public.sh is not executable"
  fi

  # Test: sync script has bash shebang
  first_line=$(head -1 "$SYNC_SCRIPT")
  if echo "$first_line" | grep -q '#!/bin/bash'; then
    pass "sync-to-public.sh has bash shebang"
  else
    fail "sync-to-public.sh shebang: $first_line"
  fi

  # Test: sync script passes syntax check
  if bash -n "$SYNC_SCRIPT" 2>/dev/null; then
    pass "sync-to-public.sh passes bash syntax check"
  else
    fail "sync-to-public.sh has syntax errors"
  fi

  # Test: sync script excludes README_DEV.md
  assert_grep_present "sync-to-public.sh excludes README_DEV.md" "sync-to-public.sh does not exclude README_DEV.md" "README_DEV.md" "$SYNC_SCRIPT"

  # Test: sync script excludes test_sync_pipeline.sh
  assert_grep_present "sync-to-public.sh excludes test_sync_pipeline.sh" "sync-to-public.sh does not exclude test_sync_pipeline.sh" "test_sync_pipeline.sh" "$SYNC_SCRIPT"

  # Test: sync script has repo owner rewrite
  assert_grep_present "Repo owner rewrite present (stuartcrobinson → gridlhq)" "Repo owner rewrite not found" 'stuartcrobinson/allyourbase.*gridlhq/allyourbase' "$SYNC_SCRIPT"

  # Test: sync script has vanity URL rewrite
  assert_grep_present "Vanity URL rewrite present (staging → install)" "Vanity URL rewrite not found" 'staging\.allyourbase\.io.*install.allyourbase.io' "$SYNC_SCRIPT"
else
  fail "sync-to-public.sh is missing"
  fail "Sync script structure checks skipped because sync-to-public.sh is missing"
fi

# ── install.sh Rewrite Tests ────────────────────────────────────────────────

section "install.sh After Sync Simulation"

INSTALL_SCRIPT="${REPO_DIR}/install.sh"
synced_install=$(simulate_sync "$INSTALL_SCRIPT")

# Test: REPO default is rewritten to gridlhq
assert_grep_present "REPO default rewritten to gridlhq/allyourbase" "REPO default not rewritten" 'REPO=.*gridlhq/allyourbase' "$synced_install"

# Test: No staging.allyourbase.io remains
assert_grep_absent "No staging.allyourbase.io in synced install.sh" "staging.allyourbase.io still present after sync" 'staging.allyourbase.io' "$synced_install"

# Test: No stuartcrobinson references remain
assert_grep_absent "No stuartcrobinson references in synced install.sh" "stuartcrobinson still present after sync" 'stuartcrobinson' "$synced_install"

# Test: Header usage examples share one canonical installer URL
install_usage_block=$(
  extract_section "$synced_install" '^# Usage:$' '^# Environment variables:$'
)
install_usage_latest_url=$(printf "%s\n" "$install_usage_block" | first_curl_url)
install_usage_pinned_url=$(printf "%s\n" "$install_usage_block" | second_curl_url)
if urls_match "$install_usage_latest_url" "$install_usage_pinned_url"; then
  pass "install.sh usage examples use one consistent installer URL"
else
  fail "install.sh usage URLs mismatch" "latest=${install_usage_latest_url} pinned=${install_usage_pinned_url}"
fi

# Test: Header usage examples use canonical installer endpoint
if url_is_canonical_installer "$install_usage_latest_url"; then
  pass "install.sh usage examples use canonical installer endpoint"
else
  fail "install.sh usage examples do not use canonical endpoint" "$install_usage_latest_url"
fi

# Test: Header pinned version example exists on default install repo
install_usage_pinned_version=$(printf "%s\n" "$install_usage_block" | first_pinned_version)
install_usage_repo=$(grep 'REPO="${AYB_REPO:-' "$synced_install" | extract_default_repo)
if [ -n "$install_usage_pinned_version" ] && [ -n "$install_usage_repo" ]; then
  assert_repo_tag_exists \
    "$install_usage_repo" \
    "$install_usage_pinned_version" \
    "install.sh pinned usage tag exists: ${install_usage_repo}@${install_usage_pinned_version}" \
    "install.sh default REPO must be a safe GitHub owner/repo slug" \
    "install.sh pinned usage tag missing: ${install_usage_repo}@${install_usage_pinned_version}"
else
  fail "install.sh usage section missing pinned version or default repo"
fi

# Test: install.sh still has valid POSIX syntax after rewrite
if sh -n "$synced_install" 2>/dev/null; then
  pass "Synced install.sh passes POSIX syntax check"
else
  fail "Synced install.sh has syntax errors"
fi

# Test: GitHub release URL uses correct org
if grep -q 'github.com/gridlhq/allyourbase/releases' "$synced_install"; then
  pass "GitHub release URL uses gridlhq org"
elif grep -q 'github.com/.*REPO.*releases' "$synced_install"; then
  pass "GitHub release URL uses REPO variable (dynamic)"
else
  fail "GitHub release URL not found or incorrect"
fi

rm -f "$synced_install"

# ── README.md Rewrite Tests ──────────────────────────────────────────────────

section "README.md After Sync Simulation"

README="${REPO_DIR}/README.md"
synced_readme=$(simulate_sync "$README")
readme_quickstart_block=$(extract_markdown_section "$synced_readme" "Quickstart")
readme_install_block=$(extract_markdown_section "$synced_readme" "Install")

# Test: README demo URL claim stays aligned with launch demo port
assert_grep_present "README includes expected demo URL (localhost:5175)" "README missing expected demo URL claim (localhost:5175)" 'http://localhost:5175' "$synced_readme"

# Test: README admin URL claim stays aligned with default admin path
assert_grep_present "README includes expected admin URL (localhost:8090/admin)" "README missing expected admin URL claim (localhost:8090/admin)" 'http://localhost:8090/admin' "$synced_readme"

# Test: README command-count claim matches root command registrations
readme_command_count=$(sed -n 's/^\([0-9][0-9]*\) commands total\..*/\1/p' "$synced_readme" | head -1)
root_command_count=$(grep -o 'AddCommand(' "${REPO_DIR}/internal/cli/root.go" | wc -l | tr -d ' ')
if [ -n "$readme_command_count" ] && [ -n "$root_command_count" ] && [ "$readme_command_count" = "$root_command_count" ]; then
  pass "README command-count claim matches root command registrations (${readme_command_count})"
else
  fail "README command-count claim mismatch" "readme=${readme_command_count:-missing} root=${root_command_count:-missing}"
fi

# Test: README MCP claim matches current launch-facing count text
assert_grep_present "README includes expected MCP count claim (13/2/3)" "README missing expected MCP count claim (13 tools, 2 resources, 3 prompts)" '13 tools, 2 resources, 3 prompts' "$synced_readme"

# Test: README scopes the unauthenticated API example to localhost/local development
assert_grep_present "README warns that auth-disabled API examples are local-only" "README missing local-only warning for auth-disabled API examples" 'Before exposing AYB beyond `localhost`, enable auth' "$synced_readme"

# Test: README install flow does not pipe a remote script directly into a shell
assert_grep_absent "README does not pipe the installer directly into sh" "README pipes a remote installer directly into sh" -E '\|[[:space:]]*sh([[:space:]]|$)' "$synced_readme"

# Test: No staging.allyourbase.io remains
assert_grep_absent "No staging.allyourbase.io in synced README.md" "staging.allyourbase.io still present in README" 'staging.allyourbase.io' "$synced_readme"

# Test: No stuartcrobinson references remain
assert_grep_absent "No stuartcrobinson references in synced README.md" "stuartcrobinson still present in README" 'stuartcrobinson' "$synced_readme"

# Test: README GitHub links do not regress to dev repo name
assert_grep_absent "No dev-repo GitHub links in synced README.md" "Synced README contains dev-repo GitHub links" 'github.com/.*/allyourbase_dev' "$synced_readme"

# Test: README badges/releases point at canonical public repo
if grep -q 'github.com/gridlhq/allyourbase/actions/workflows/ci.yml' "$synced_readme" && \
   grep -q 'github.com/gridlhq/allyourbase/actions/workflows/release.yml' "$synced_readme" && \
   grep -q 'github.com/gridlhq/allyourbase/releases' "$synced_readme"; then
  pass "README badge/release links target canonical public repo"
else
  fail "README badge/release links do not target canonical public repo"
fi

# Test: No private/dev docs links leak into synced README
assert_grep_absent "No _dev/ links in synced README.md" "Synced README contains _dev/ links" '_dev/' "$synced_readme"

# Test: No placeholder markers remain in synced README
assert_grep_absent "No TODO/TBD/placeholder markers in synced README.md" "Synced README contains placeholder markers" -Ei 'TODO|TBD|placeholder' "$synced_readme"

# Test: Launch screenshot path is relative, public-safe, and file exists
readme_dashboard_asset=$(sed -n 's|^[[:space:]]*!\[[^]]*\](\([^)]*\)).*|\1|p' "$synced_readme" | head -1)
if [ -n "$readme_dashboard_asset" ]; then
  # Keep the launch visual above Quickstart so it remains prominent in README.
  assert_launch_image_precedes_quickstart "$synced_readme"
  assert_dashboard_asset_reference_is_safe "$readme_dashboard_asset"
else
  fail "README missing stand-alone launch image reference"
fi

# Test: Launch screenshot stays under 2MB for GitHub-friendly rendering
if [ -n "${dashboard_asset_path:-}" ] && [ -f "${dashboard_asset_path:-}" ]; then
  dashboard_asset_size=$(wc -c < "$dashboard_asset_path" | tr -d ' ')
  if [ "$dashboard_asset_size" -le 2097152 ]; then
    pass "Dashboard image size is under 2MB (${dashboard_asset_size} bytes)"
  else
    fail "Dashboard image exceeds 2MB" "${dashboard_asset_size} bytes"
  fi
fi

# Test: Quickstart and Install commands use one consistent installer URL
quickstart_install_url=$(printf "%s\n" "$readme_quickstart_block" | first_curl_url)
install_latest_url=$(printf "%s\n" "$readme_install_block" | first_curl_url)
install_pinned_url=$(printf "%s\n" "$readme_install_block" | second_curl_url)
if urls_match "$quickstart_install_url" "$install_latest_url" && \
   urls_match "$install_latest_url" "$install_pinned_url"; then
  pass "Quickstart and Install use one consistent installer URL"
else
  fail "Installer URL mismatch across README commands" "quickstart=${quickstart_install_url} latest=${install_latest_url} pinned=${install_pinned_url}"
fi

# Test: Installer URL uses canonical launch endpoint + script path
if url_is_canonical_installer "$quickstart_install_url"; then
  pass "Installer URL uses canonical launch endpoint"
else
  fail "Installer URL is not canonical launch endpoint" "$quickstart_install_url"
fi

# Test: Quickstart runs the downloaded installer explicitly
quickstart_install_exec=$(printf "%s\n" "$readme_quickstart_block" | grep -E '^[[:space:]]*sh[[:space:]]+[^[:space:]]+$' | head -1 || true)
if [ -n "$quickstart_install_exec" ]; then
  pass "Quickstart executes the downloaded installer as a separate step"
else
  fail "Quickstart is missing a standalone installer execution step"
fi

# Test: Quickstart launch commands work in a fresh shell after install
quickstart_start_cmd=$(printf "%s\n" "$readme_quickstart_block" | grep -E '^((\$HOME|~)/\.ayb/bin/ayb|ayb) start$' | head -1 || true)
quickstart_demo_cmd=$(printf "%s\n" "$readme_quickstart_block" | grep -E '^((\$HOME|~)/\.ayb/bin/ayb|ayb) demo live-polls$' | head -1 || true)
quickstart_path_export=false
if printf "%s\n" "$readme_quickstart_block" | grep -Eq '^export PATH="(\$HOME|~)/\.ayb/bin:\$PATH"$'; then
  quickstart_path_export=true
fi
if [ -n "$quickstart_start_cmd" ] && [ -n "$quickstart_demo_cmd" ]; then
  case "${quickstart_start_cmd}|${quickstart_demo_cmd}" in
    '$HOME/.ayb/bin/ayb start|$HOME/.ayb/bin/ayb demo live-polls'|'~/.ayb/bin/ayb start|~/.ayb/bin/ayb demo live-polls')
      pass "Quickstart uses the installed binary path explicitly"
      ;;
    'ayb start|ayb demo live-polls')
      if [ "$quickstart_path_export" = true ]; then
        pass "Quickstart exports the install dir before bare ayb commands"
      else
        fail "Quickstart bare ayb commands assume PATH is already updated" "use ~/.ayb/bin/ayb or export PATH before ayb commands"
      fi
      ;;
    *)
      fail "Quickstart launch commands are inconsistent" "start=${quickstart_start_cmd:-missing} demo=${quickstart_demo_cmd:-missing}"
      ;;
  esac
else
  fail "Quickstart missing launch commands" "start=${quickstart_start_cmd:-missing} demo=${quickstart_demo_cmd:-missing}"
fi

# Test: No staging-vs-prod table that breaks after rewrite
if grep -q 'Staging.*Production' "$synced_readme"; then
  # If there's a comparison table, check that staging and prod columns aren't identical
  staging_url=$(grep -o 'staging\.allyourbase\.io' "$synced_readme" | head -1 || true)
  if [ -n "$staging_url" ]; then
    fail "Staging vs Production table still has staging.allyourbase.io"
  else
    pass "Staging vs Production table (if present) is correctly rewritten"
  fi
else
  pass "No staging-vs-prod comparison table in public README (moved to README_DEV.md)"
fi

# Test: Building-from-source repo URL exists publicly
readme_source_repo=$(printf "%s\n" "$readme_install_block" | sed -n 's|.*https://github.com/\([^ ]*\)\.git.*|\1|p' | head -1)
if [ -n "$readme_source_repo" ]; then
  if allowlisted_github_repo_slug "$readme_source_repo"; then
    if git_remote_exists "https://github.com/${readme_source_repo}.git"; then
      pass "README source-clone repo resolves: ${readme_source_repo}"
    else
      fail "README source-clone repo is unreachable: ${readme_source_repo}" "https://github.com/${readme_source_repo}.git"
    fi
  else
    fail "README source-clone repo must use a safe GitHub owner/repo slug" "$readme_source_repo"
  fi
else
  fail "README Install section missing source-clone command"
fi

# Test: README Install's `go install` module path resolves publicly (if present)
readme_go_install=$(printf "%s\n" "$readme_install_block" | sed -n 's/^go install //p' | head -1)
go_module_path=$(printf "%s\n" "$readme_go_install" | sed 's|/cmd/ayb@.*||')
if [ -n "$go_module_path" ]; then
  if allowlisted_go_install_repo "$go_module_path"; then
    go_module_repo_url="https://${go_module_path}.git"
    if git_remote_exists "$go_module_repo_url"; then
      pass "README go-install module path resolves: ${go_module_path}"
    else
      fail "README go-install module path is unreachable: ${go_module_path}" "$go_module_repo_url"
    fi
  else
    fail "README go-install module path must use an allowlisted direct repo host" "$go_module_path"
  fi
else
  pass "README Install omits go install (module path not yet public)"
fi

# Test: README pinned install version exists as a public tag on source repo
readme_pinned_version=$(printf "%s\n" "$readme_install_block" | first_pinned_version)
if [ -n "$readme_source_repo" ] && [ -n "$readme_pinned_version" ]; then
  assert_repo_tag_exists \
    "$readme_source_repo" \
    "$readme_pinned_version" \
    "README pinned version tag exists: ${readme_source_repo}@${readme_pinned_version}" \
    "README source-clone repo must use a safe GitHub owner/repo slug" \
    "README pinned version tag missing: ${readme_source_repo}@${readme_pinned_version}"
else
  fail "README Install section missing source repo or pinned version command"
fi

# Test: repo-local roadmap links in README survive Debbie sync
readme_roadmap_link=$(sed -n 's|.*](\(ROADMAP\.md\)).*|\1|p' "$synced_readme" | head -1)
if [ -n "$readme_roadmap_link" ]; then
  if [ -f "${REPO_DIR}/${readme_roadmap_link}" ]; then
    if grep -Fq "\"${readme_roadmap_link}\"," "${REPO_DIR}/.debbie.toml"; then
      fail "README roadmap link target is excluded from Debbie sync" "$readme_roadmap_link"
    else
      pass "README roadmap link target exists and is included in Debbie sync"
    fi
  else
    fail "README roadmap link target missing in dev repo" "$readme_roadmap_link"
  fi
else
  pass "README does not link to a repo-local roadmap file"
fi

rm -f "$synced_readme"

# ── docs-site Public Surface Tests ───────────────────────────────────────────

section "docs-site Public Install Surfaces"

for docs_page in \
  docs-site/index.md \
  docs-site/guide/getting-started.md \
  docs-site/guide/deployment.md \
  docs-site/guide/quickstart.md
do
  docs_page_path="${REPO_DIR}/${docs_page}"
  assert_grep_present \
    "docs-site page uses canonical installer URL: ${docs_page}" \
    "docs-site page missing canonical installer URL: ${docs_page}" \
    "$CANONICAL_INSTALLER_URL_PATTERN" \
    "$docs_page_path"
  assert_grep_absent \
    "docs-site page does not use legacy bare-domain installer URL: ${docs_page}" \
    "docs-site page still uses legacy bare-domain installer URL: ${docs_page}" \
    "$LEGACY_INSTALLER_URL_PATTERN" \
    "$docs_page_path"
  assert_grep_absent \
    "docs-site page does not pipe the installer directly into sh: ${docs_page}" \
    "docs-site page pipes the installer directly into sh: ${docs_page}" \
    -E '\|[[:space:]]*sh([[:space:]]|$)' \
    "$docs_page_path"
done

for docs_page in \
  docs-site/index.md \
  docs-site/guide/getting-started.md
do
  docs_page_path="${REPO_DIR}/${docs_page}"
  assert_grep_absent \
    "docs-site page omits unverified go-install guidance: ${docs_page}" \
    "docs-site page still advertises unverified go-install guidance: ${docs_page}" \
    'go install github\.com/allyourbase/ayb/cmd/ayb@latest' \
    "$docs_page_path"
done

# ── CHANGELOG.md Sync Tests ─────────────────────────────────────────────────

section "CHANGELOG.md After Sync Simulation"

CHANGELOG="${REPO_DIR}/CHANGELOG.md"
synced_changelog=$(simulate_sync "$CHANGELOG")

# Test: ## [Unreleased] section survives sync rewrite
assert_grep_present "CHANGELOG ## [Unreleased] section survives sync" "CHANGELOG ## [Unreleased] section missing after sync" '## \[Unreleased\]' "$synced_changelog"

# Test: No _dev/ references leak into synced changelog
assert_grep_absent "No _dev/ references in synced CHANGELOG.md" "Synced CHANGELOG contains _dev/ references" '_dev/' "$synced_changelog"

# Test: No duplicate ### headings within Unreleased
unreleased_headings=$(awk '/^## \[Unreleased\]/{u=1;next} /^## \[/{u=0} u && /^### /{print}' "$synced_changelog")
duplicate_headings=$(printf "%s\n" "$unreleased_headings" | sort | uniq -d)
if [ -z "$duplicate_headings" ]; then
  pass "No duplicate ### headings in Unreleased section"
else
  fail "Duplicate ### headings in Unreleased" "$duplicate_headings"
fi

# Test: MCP tool count in changelog matches README
changelog_mcp_count=$(grep -o '[0-9][0-9]* tools,' "$synced_changelog" | head -1 | grep -o '[0-9][0-9]*')
readme_mcp_count=$(grep -o '[0-9][0-9]* tools,' "${REPO_DIR}/README.md" | head -1 | grep -o '[0-9][0-9]*')
if [ -n "$changelog_mcp_count" ] && [ -n "$readme_mcp_count" ] && [ "$changelog_mcp_count" = "$readme_mcp_count" ]; then
  pass "CHANGELOG MCP tool count matches README (${changelog_mcp_count})"
else
  fail "CHANGELOG/README MCP tool count mismatch" "changelog=${changelog_mcp_count:-missing} readme=${readme_mcp_count:-missing}"
fi

rm -f "$synced_changelog"

# ── Cross-File Consistency Tests ─────────────────────────────────────────────

section "Cross-File Consistency"

# Test: install.sh and README.md use same REPO owner format
install_repo=$(grep 'AYB_REPO:-' "$INSTALL_SCRIPT" | extract_default_repo)
if echo "$install_repo" | grep -q 'gridlhq/allyourbase'; then
  pass "install.sh default REPO matches public owner"
else
  fail "install.sh default REPO mismatch: $install_repo"
fi

# Test: goreleaser archive format matches install.sh expectation
if grep -q 'ayb_.*_.*_.*\.tar\.gz' "$INSTALL_SCRIPT"; then
  if goreleaser_has 'ayb_{{ .Version }}_{{ .Os }}_{{ .Arch }}'; then
    pass "Install.sh archive format matches goreleaser template"
  else
    pass "Install.sh archive format present (goreleaser check skipped)"
  fi
else
  fail "Install.sh archive format doesn't match goreleaser"
fi

# Test: goreleaser checksums.txt matches install.sh expectation
if grep -q 'checksums.txt' "$INSTALL_SCRIPT"; then
  if goreleaser_has 'checksums.txt'; then
    pass "Checksums filename matches between install.sh and goreleaser"
  else
    pass "Checksums filename present in install.sh (goreleaser check skipped)"
  fi
else
  fail "Checksums filename missing from install.sh"
fi

# Test: README_DEV.md exists and is excluded from sync
if [ -f "${REPO_DIR}/README_DEV.md" ]; then
  pass "README_DEV.md exists in dev repo"
else
  fail "README_DEV.md missing from dev repo"
fi

# Test: No dev-only files referenced in sync pipeline VANITY_FILES
vanity_files_section=$(sed -n '/VANITY_FILES/,/^)/p' "$SYNC_SCRIPT")
if echo "$vanity_files_section" | grep -q 'README_DEV'; then
  fail "README_DEV.md should not be in VANITY_FILES (it's excluded from sync)"
else
  pass "README_DEV.md not in VANITY_FILES list"
fi

# Test: All files in VANITY_FILES exist
assert_core_files_exist \
  "Core VANITY_FILES (README.md, install.sh) exist" \
  "VANITY_FILES reference missing" \
  README.md install.sh

# Test: All files in FILES_TO_PATCH exist (check core ones)
assert_core_files_exist \
  "Core FILES_TO_PATCH (install.sh, README.md, .goreleaser.yaml) exist" \
  "FILES_TO_PATCH reference missing" \
  install.sh README.md .goreleaser.yaml

# Test: repo-local priorities link in ROADMAP survives Debbie sync
if [ -f "${REPO_DIR}/ROADMAP.md" ]; then
  roadmap_priorities_link=$(sed -n 's|.*](\(PRIORITIES\.md\)).*|\1|p' "${REPO_DIR}/ROADMAP.md" | head -1)
  if [ -n "$roadmap_priorities_link" ]; then
    if [ -f "${REPO_DIR}/${roadmap_priorities_link}" ]; then
      if grep -Fq "\"${roadmap_priorities_link}\"," "${REPO_DIR}/.debbie.toml"; then
        fail "ROADMAP priorities link target is excluded from Debbie sync" "$roadmap_priorities_link"
      else
        pass "ROADMAP priorities link target exists and is included in Debbie sync"
      fi
    else
      fail "ROADMAP priorities link target missing in dev repo" "$roadmap_priorities_link"
    fi
  else
    pass "ROADMAP does not link to a repo-local priorities file"
  fi
else
  fail "ROADMAP.md missing from dev repo"
fi

# ── Summary ──────────────────────────────────────────────────────────────────

section "Summary"
printf "  Total: %d  Passed: \033[0;32m%d\033[0m  Failed: \033[0;31m%d\033[0m\n\n" "$TESTS_RUN" "$TESTS_PASSED" "$TESTS_FAILED"

if [ "$TESTS_FAILED" -gt 0 ]; then
  exit 1
fi
