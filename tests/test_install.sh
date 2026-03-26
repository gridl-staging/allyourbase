#!/bin/sh
# tests/test_install.sh — Unit and integration tests for install.sh
#
# Usage:
#   ./tests/test_install.sh                     # Run all tests
#   GITHUB_TOKEN=xxx ./tests/test_install.sh    # Include private-repo download tests
#
# Tests are split into:
#   1. Unit tests (no network) — validate platform detection, PATH logic, etc.
#   2. Integration tests (network) — validate actual downloads

set -eu

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
INSTALL_SCRIPT="${REPO_DIR}/install.sh"

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

allowlisted_github_repo_slug() {
  printf '%s\n' "$1" | grep -Eq '^[A-Za-z0-9][A-Za-z0-9._-]*/[A-Za-z0-9][A-Za-z0-9._-]*$'
}

install_script_matches() {
  grep "$@" "$INSTALL_SCRIPT" >/dev/null 2>&1
}

assert_install_script_match() {
  assert_pass_message="$1"
  assert_fail_message="$2"
  shift 2

  if install_script_matches "$@"; then
    pass "$assert_pass_message"
  else
    fail "$assert_fail_message"
  fi
}

install_succeeds() {
  install_target_dir="$1"
  install_version="${2:-}"

  if [ -n "$install_version" ]; then
    NO_MODIFY_PATH=1 AYB_INSTALL="$install_target_dir" GITHUB_TOKEN="$GITHUB_TOKEN" \
      sh "$INSTALL_SCRIPT" "$install_version" 2>&1 | grep -q "installed successfully"
  else
    NO_MODIFY_PATH=1 AYB_INSTALL="$install_target_dir" GITHUB_TOKEN="$GITHUB_TOKEN" \
      sh "$INSTALL_SCRIPT" 2>&1 | grep -q "installed successfully"
  fi
}

# ── Unit Tests: Syntax & Structure ──────────────────────────────────────────

section "Install Script Syntax & Structure"

# Test: Script is valid POSIX shell
if sh -n "$INSTALL_SCRIPT" 2>/dev/null; then
  pass "install.sh passes POSIX shell syntax check"
else
  fail "install.sh has shell syntax errors"
fi

# Test: Script starts with proper shebang
first_line=$(head -1 "$INSTALL_SCRIPT")
if [ "$first_line" = "#!/bin/sh" ]; then
  pass "Shebang is #!/bin/sh (POSIX compatible)"
else
  fail "Shebang should be #!/bin/sh, got: $first_line"
fi

# Test: set -eu is present (fail-fast)
assert_install_script_match "set -eu present (fail-fast mode)" "set -eu not found — script won't fail on errors" '^set -eu'

# Test: Script is executable
if [ -x "$INSTALL_SCRIPT" ]; then
  pass "install.sh is executable"
else
  fail "install.sh is not executable"
fi

# ── Unit Tests: Configuration Defaults ──────────────────────────────────────

section "Configuration Defaults"

# Test: REPO default matches the environment (staging vs prod)
# In staging CI: expect gridlhq-staging/allyourbase
# In prod CI: expect gridlhq/allyourbase
# Locally: accept either (dev repo has prod default, but staging sync rewrites it)
if [ -n "${GITHUB_REPOSITORY:-}" ]; then
  case "$GITHUB_REPOSITORY" in
    gridlhq-staging/allyourbase)
      assert_install_script_match \
        "Default REPO is gridlhq-staging/allyourbase (staging environment)" \
        "Default REPO should be gridlhq-staging/allyourbase in staging environment" \
        'REPO=.*gridlhq-staging/allyourbase'
      ;;
    gridlhq/allyourbase)
      if install_script_matches 'REPO=.*gridlhq/allyourbase' && ! install_script_matches 'gridlhq-staging'; then
        pass "Default REPO is gridlhq/allyourbase (production environment)"
      else
        fail "Default REPO should be gridlhq/allyourbase in production environment"
      fi
      ;;
    gridlhq/allyourbase_dev|gridl-dev/allyourbase_dev)
      # Dev repo: install.sh still defaults to the public repo identity after sync.
      assert_install_script_match \
        "Default REPO is gridlhq/allyourbase (dev environment)" \
        "Default REPO should be gridlhq/allyourbase in dev environment" \
        'REPO=.*gridlhq/allyourbase'
      ;;
    *)
      fail "Unexpected GITHUB_REPOSITORY: $GITHUB_REPOSITORY"
      ;;
  esac
else
  # Local: accept either repo (dev has prod default, staging sync rewrites it)
  if install_script_matches 'REPO=.*gridlhq/allyourbase'; then
    pass "Default REPO is set (local environment)"
  else
    fail "Default REPO should be gridlhq/allyourbase or gridlhq-staging/allyourbase"
  fi
fi

# Test: BINARY_NAME is ayb
assert_install_script_match "BINARY_NAME is ayb" "BINARY_NAME is not ayb" 'BINARY_NAME="ayb"'

# Test: Install dir defaults to ~/.ayb/bin
assert_install_script_match "Default install dir is ~/.ayb/bin" "Default install dir not ~/.ayb/bin" 'INSTALL_DIR=.*HOME/.ayb.*/bin'

# ── Unit Tests: Platform Detection ──────────────────────────────────────────

section "Platform Detection"

# Test: All four Go OS/arch combos handled
for combo in "linux.*amd64" "linux.*arm64" "darwin.*amd64" "darwin.*arm64"; do
  os_part=$(echo "$combo" | cut -d'.' -f1)
  if grep -q "$os_part" "$INSTALL_SCRIPT"; then
    pass "OS handled: $os_part"
  else
    fail "OS not handled: $os_part"
  fi
done

# Test: amd64 arch mapping
if grep -q 'x86_64|amd64.*amd64' "$INSTALL_SCRIPT" || grep -q 'x86_64|amd64).*arch="amd64"' "$INSTALL_SCRIPT"; then
  pass "x86_64/amd64 architecture mapping"
else
  fail "x86_64/amd64 architecture mapping missing"
fi

# Test: arm64 arch mapping
if grep -q 'aarch64|arm64.*arm64' "$INSTALL_SCRIPT" || grep -q 'aarch64|arm64).*arch="arm64"' "$INSTALL_SCRIPT"; then
  pass "aarch64/arm64 architecture mapping"
else
  fail "aarch64/arm64 architecture mapping missing"
fi

# Test: Rosetta 2 detection exists
assert_install_script_match "Rosetta 2 detection present" "Rosetta 2 detection missing" "sysctl.proc_translated"

# Test: Windows detection with helpful error
assert_install_script_match "Windows detection present (with error message)" "Windows detection missing" "MINGW\\|MSYS\\|CYGWIN"

# ── Unit Tests: Download Tool Detection ─────────────────────────────────────

section "Download Tool Detection"

# Test: curl support
assert_install_script_match "curl detection present" "curl detection missing" 'command -v curl'

# Test: wget fallback
assert_install_script_match "wget fallback present" "wget fallback missing" 'command -v wget'

# ── Unit Tests: Version Resolution ──────────────────────────────────────────

section "Version Resolution"

# Test: AYB_VERSION env var support
assert_install_script_match "AYB_VERSION env var support" "AYB_VERSION env var not supported" 'AYB_VERSION'

# Test: CLI argument version pinning
if grep -q '${1:-}' "$INSTALL_SCRIPT" || grep -q '"$1"' "$INSTALL_SCRIPT"; then
  pass "CLI argument version pinning supported"
else
  fail "CLI argument version pinning not found"
fi

# Test: GitHub API latest release detection
assert_install_script_match "GitHub API latest release detection" "GitHub API latest release detection missing" 'api.github.com/repos.*releases/latest'

# Test: Version number stripped from tag (v prefix handling)
assert_install_script_match "Version v-prefix stripping (goreleaser compat)" "No v-prefix stripping — goreleaser archives use version without v" "sed 's/^v//'"

# ── Unit Tests: Security Features ───────────────────────────────────────────

section "Security Features"

# Test: SHA256 checksum verification
if grep -q 'sha256sum' "$INSTALL_SCRIPT" && grep -q 'shasum -a 256' "$INSTALL_SCRIPT"; then
  pass "SHA256 checksum verification (sha256sum + shasum fallback)"
else
  fail "SHA256 checksum verification incomplete"
fi

# Test: Checksum failure exits with error code
if grep -A 3 'Checksum verification FAILED' "$INSTALL_SCRIPT" | grep -q 'exit'; then
  pass "Checksum failure causes exit"
else
  fail "Checksum failure message found but no exit statement"
fi

# Test: GITHUB_TOKEN support for private repos
assert_install_script_match "GITHUB_TOKEN support for private repos" "GITHUB_TOKEN support missing" 'GITHUB_TOKEN'

# Test: GitHub API asset download (for private repos)
assert_install_script_match "GitHub API asset download (Accept: application/octet-stream)" "GitHub API asset download not implemented" 'application/octet-stream'

# Test: Temp directory cleanup
assert_install_script_match "Temp directory cleanup on exit (trap)" "No temp directory cleanup" 'trap.*rm -rf'

# ── Unit Tests: PATH Management ─────────────────────────────────────────────

section "PATH Management"

# Test: Bash profile update
if grep -q '.bashrc' "$INSTALL_SCRIPT" && grep -q '.bash_profile' "$INSTALL_SCRIPT"; then
  pass "Bash profile update (.bashrc/.bash_profile)"
else
  fail "Bash profile update incomplete"
fi

# Test: Zsh profile update
assert_install_script_match "Zsh profile update (.zshrc)" "Zsh profile update missing" '.zshrc'

# Test: Fish config update
assert_install_script_match "Fish config update" "Fish config update missing" 'config.fish'

# Test: NO_MODIFY_PATH support
assert_install_script_match "NO_MODIFY_PATH env var supported" "NO_MODIFY_PATH not supported" 'NO_MODIFY_PATH'

# Test: Idempotent PATH update (won't add duplicate)
assert_install_script_match "Idempotent PATH update (checks for existing entry)" "PATH update may not be idempotent" 'grep -qF.*INSTALL_DIR'

# Test: Permission-denied handling for shell profiles
assert_install_script_match "Permission-denied handling for shell profiles" "No permission-denied handling for shell profiles" 'permission denied'

# ── Unit Tests: Environment Variable Overrides ──────────────────────────────

section "Environment Variable Overrides"

for var in AYB_INSTALL AYB_REPO AYB_VERSION GITHUB_TOKEN NO_MODIFY_PATH; do
  assert_install_script_match "Env var override: $var" "Env var override missing: $var" "$var"
done

# ── Unit Tests: Output & UX ────────────────────────────────────────────────

section "Output & UX"

# Test: Colored output with terminal detection
assert_install_script_match "Color output with terminal detection" "No terminal detection for colors" '\[ -t 1 \]'

# Test: Success message with getting-started instructions
assert_install_script_match "Getting-started instructions in success output" "No getting-started instructions" 'ayb start'

# Test: PATH reminder when binary not in PATH
assert_install_script_match "PATH reminder for new installs" "No PATH reminder" 'Restart your terminal'

# Test: Archive name uses goreleaser format
assert_install_script_match "Archive name matches goreleaser format (ayb_{ver}_{os}_{arch}.tar.gz)" "Archive name doesn't match goreleaser format" 'ayb_.*_.*_.*\.tar\.gz'

# Test: Downloads checksums.txt (goreleaser format)
assert_install_script_match "Uses checksums.txt (goreleaser format)" "Does not reference checksums.txt" 'checksums.txt'

# ── Unit Tests: Install to User Directory ───────────────────────────────────

section "Install Location"

# Test: Installs to user directory (no sudo by default)
assert_install_script_match "Default install to ~/.ayb (no sudo required)" "Does not install to user directory by default" 'HOME/.ayb'

# Test: No sudo in the script
if install_script_matches 'sudo'; then
  fail "Script contains sudo — should install to user directory"
else
  pass "No sudo in install script"
fi

# Test: mkdir -p for install directory
assert_install_script_match "Creates install directory with mkdir -p" "No mkdir -p for install directory" 'mkdir -p.*INSTALL_DIR'

# ── Release API Reachability (public, no token needed) ────────────────────────

section "Release API Reachability"

# Extract the default REPO from install.sh
default_repo=$(grep 'AYB_REPO:-' "$INSTALL_SCRIPT" | sed 's/.*AYB_REPO:-//;s/}.*//;s/"//g')

# Test: install.sh default repo stays constrained to a GitHub owner/repo slug
if allowlisted_github_repo_slug "$default_repo"; then
  pass "Default REPO uses a safe GitHub owner/repo slug"
else
  fail "Default REPO must be a safe GitHub owner/repo slug" "$default_repo"
fi

# Test: GitHub API /releases/latest returns a valid tag_name
if allowlisted_github_repo_slug "$default_repo"; then
  if [ -n "${GITHUB_TOKEN:-}" ]; then
    api_resp=$(curl -fsSL -H "Authorization: token ${GITHUB_TOKEN}" "https://api.github.com/repos/${default_repo}/releases/latest" 2>&1) || true
  else
    api_resp=$(curl -fsSL "https://api.github.com/repos/${default_repo}/releases/latest" 2>&1) || true
  fi
else
  api_resp=''
fi
if [ -n "${api_resp:-}" ] && echo "$api_resp" | grep -q '"tag_name"'; then
  latest_tag=$(echo "$api_resp" | grep '"tag_name"' | head -1 | sed 's/.*"tag_name": *"//;s/".*//')
  pass "GitHub API releases/latest returns tag: ${latest_tag}"
else
  # No release yet is acceptable (staging may not have any)
  if [ -n "${api_resp:-}" ] && echo "$api_resp" | grep -q '"message".*"Not Found"'; then
    pass "GitHub API reachable (no releases yet for ${default_repo})"
  elif allowlisted_github_repo_slug "$default_repo"; then
    fail "GitHub API releases/latest for ${default_repo} failed" \
      "Got: $(echo "$api_resp" | head -3)"
  else
    fail "Skipped GitHub API releases/latest due to unsafe default REPO" "$default_repo"
  fi
fi

# Test: If releases exist, check for .tar.gz assets
if [ -n "${api_resp:-}" ] && echo "$api_resp" | grep -q '"tag_name"'; then
  if echo "$api_resp" | grep -q 'ayb_.*\.tar\.gz'; then
    pass "Release has .tar.gz assets"
  else
    fail "No .tar.gz assets found in latest release"
  fi
fi

# ── Integration Tests (requires network + GITHUB_TOKEN) ──────────────────────

section "Integration Tests"

if [ -z "${GITHUB_TOKEN:-}" ]; then
  # Try to get token from gh CLI
  if command -v gh >/dev/null 2>&1; then
    GITHUB_TOKEN=$(gh auth token 2>/dev/null || true)
  fi
fi

if [ -z "${GITHUB_TOKEN:-}" ]; then
  printf "  \033[1;33mSkipped\033[0m (set GITHUB_TOKEN or install gh CLI for integration tests)\n"
elif ! allowlisted_github_repo_slug "$default_repo"; then
  printf "  \033[1;33mSkipped\033[0m (default REPO is not a safe GitHub owner/repo slug)\n"
else
  # Resolve a pinned version dynamically (use the latest release tag)
  PIN_VERSION=$(curl -fsSL -H "Authorization: token ${GITHUB_TOKEN}" \
    "https://api.github.com/repos/${default_repo}/releases/latest" 2>/dev/null \
    | grep '"tag_name"' | head -1 | sed 's/.*"tag_name": *"//;s/".*//')

  if [ -z "$PIN_VERSION" ]; then
    printf "  \033[1;33mSkipped\033[0m (no releases found for ${default_repo} — create a release first)\n"
  else
    # Test: Full install with version pinning
    test_dir=$(mktemp -d)
    trap_cleanup() { rm -rf "$test_dir"; }
    trap trap_cleanup EXIT

    if install_succeeds "$test_dir" "$PIN_VERSION"; then
      pass "Full install with version pinning (${PIN_VERSION})"
    else
      fail "Full install with version pinning failed (${PIN_VERSION})"
    fi

    # Test: Binary exists and is executable
    if [ -x "$test_dir/bin/ayb" ]; then
      pass "Binary is executable at expected path"
    else
      fail "Binary not found or not executable at $test_dir/bin/ayb"
    fi

    # Test: Binary runs
    if "$test_dir/bin/ayb" --help >/dev/null 2>&1 || "$test_dir/bin/ayb" version >/dev/null 2>&1; then
      pass "Binary runs successfully"
    else
      fail "Binary failed to run (may be incompatible with this platform)"
    fi

    # Test: Latest version auto-detection
    test_dir2=$(mktemp -d)
    if install_succeeds "$test_dir2"; then
      pass "Latest version auto-detection works"
    else
      fail "Latest version auto-detection failed"
    fi
    rm -rf "$test_dir2"

    # Test: Idempotent reinstall (run twice, check no errors)
    test_dir3=$(mktemp -d)
    NO_MODIFY_PATH=1 AYB_INSTALL="$test_dir3" GITHUB_TOKEN="$GITHUB_TOKEN" sh "$INSTALL_SCRIPT" "$PIN_VERSION" >/dev/null 2>&1 || true
    output2=$(NO_MODIFY_PATH=1 AYB_INSTALL="$test_dir3" GITHUB_TOKEN="$GITHUB_TOKEN" sh "$INSTALL_SCRIPT" "$PIN_VERSION" 2>&1)
    if echo "$output2" | grep -q "installed successfully"; then
      pass "Idempotent reinstall works"
    else
      fail "Idempotent reinstall failed" "$output2"
    fi
    rm -rf "$test_dir3"

    # Test: Custom install directory
    test_dir4=$(mktemp -d)
    if install_succeeds "$test_dir4/custom" "$PIN_VERSION"; then
      pass "Custom install directory (AYB_INSTALL)"
    else
      fail "Custom install directory failed"
    fi
    rm -rf "$test_dir4"

    # Test: Invalid version fails gracefully
    test_dir5=$(mktemp -d)
    if NO_MODIFY_PATH=1 AYB_INSTALL="$test_dir5" GITHUB_TOKEN="$GITHUB_TOKEN" sh "$INSTALL_SCRIPT" v999.999.999 2>&1 | grep -qi "error\|fail\|not found\|404"; then
      pass "Invalid version fails gracefully"
    else
      fail "Invalid version did not produce error"
    fi
    rm -rf "$test_dir5"

    rm -rf "$test_dir"
  fi
fi

# ── Summary ──────────────────────────────────────────────────────────────────

section "Summary"
printf "  Total: %d  Passed: \033[0;32m%d\033[0m  Failed: \033[0;31m%d\033[0m\n\n" "$TESTS_RUN" "$TESTS_PASSED" "$TESTS_FAILED"

if [ "$TESTS_FAILED" -gt 0 ]; then
  exit 1
fi
