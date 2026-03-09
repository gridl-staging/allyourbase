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
if grep -q '^set -eu' "$INSTALL_SCRIPT"; then
  pass "set -eu present (fail-fast mode)"
else
  fail "set -eu not found — script won't fail on errors"
fi

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
      if grep -q 'REPO=.*gridlhq-staging/allyourbase' "$INSTALL_SCRIPT"; then
        pass "Default REPO is gridlhq-staging/allyourbase (staging environment)"
      else
        fail "Default REPO should be gridlhq-staging/allyourbase in staging environment"
      fi
      ;;
    gridlhq/allyourbase)
      if grep -q 'REPO=.*gridlhq/allyourbase' "$INSTALL_SCRIPT" && ! grep -q 'gridlhq-staging' "$INSTALL_SCRIPT"; then
        pass "Default REPO is gridlhq/allyourbase (production environment)"
      else
        fail "Default REPO should be gridlhq/allyourbase in production environment"
      fi
      ;;
    gridlhq/allyourbase_dev)
      # Dev repo: install.sh defaults to gridlhq/allyourbase (sync rewrites for staging/prod)
      if grep -q 'REPO=.*gridlhq/allyourbase' "$INSTALL_SCRIPT"; then
        pass "Default REPO is gridlhq/allyourbase (dev environment)"
      else
        fail "Default REPO should be gridlhq/allyourbase in dev environment"
      fi
      ;;
    *)
      fail "Unexpected GITHUB_REPOSITORY: $GITHUB_REPOSITORY"
      ;;
  esac
else
  # Local: accept either repo (dev has prod default, staging sync rewrites it)
  if grep -q 'REPO=.*gridlhq/allyourbase' "$INSTALL_SCRIPT"; then
    pass "Default REPO is set (local environment)"
  else
    fail "Default REPO should be gridlhq/allyourbase or gridlhq-staging/allyourbase"
  fi
fi

# Test: BINARY_NAME is ayb
if grep -q 'BINARY_NAME="ayb"' "$INSTALL_SCRIPT"; then
  pass "BINARY_NAME is ayb"
else
  fail "BINARY_NAME is not ayb"
fi

# Test: Install dir defaults to ~/.ayb/bin
if grep -q 'INSTALL_DIR=.*HOME/.ayb.*/bin' "$INSTALL_SCRIPT"; then
  pass "Default install dir is ~/.ayb/bin"
else
  fail "Default install dir not ~/.ayb/bin"
fi

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
if grep -q "sysctl.proc_translated" "$INSTALL_SCRIPT"; then
  pass "Rosetta 2 detection present"
else
  fail "Rosetta 2 detection missing"
fi

# Test: Windows detection with helpful error
if grep -q "MINGW\|MSYS\|CYGWIN" "$INSTALL_SCRIPT"; then
  pass "Windows detection present (with error message)"
else
  fail "Windows detection missing"
fi

# ── Unit Tests: Download Tool Detection ─────────────────────────────────────

section "Download Tool Detection"

# Test: curl support
if grep -q 'command -v curl' "$INSTALL_SCRIPT"; then
  pass "curl detection present"
else
  fail "curl detection missing"
fi

# Test: wget fallback
if grep -q 'command -v wget' "$INSTALL_SCRIPT"; then
  pass "wget fallback present"
else
  fail "wget fallback missing"
fi

# ── Unit Tests: Version Resolution ──────────────────────────────────────────

section "Version Resolution"

# Test: AYB_VERSION env var support
if grep -q 'AYB_VERSION' "$INSTALL_SCRIPT"; then
  pass "AYB_VERSION env var support"
else
  fail "AYB_VERSION env var not supported"
fi

# Test: CLI argument version pinning
if grep -q '${1:-}' "$INSTALL_SCRIPT" || grep -q '"$1"' "$INSTALL_SCRIPT"; then
  pass "CLI argument version pinning supported"
else
  fail "CLI argument version pinning not found"
fi

# Test: GitHub API latest release detection
if grep -q 'api.github.com/repos.*releases/latest' "$INSTALL_SCRIPT"; then
  pass "GitHub API latest release detection"
else
  fail "GitHub API latest release detection missing"
fi

# Test: Version number stripped from tag (v prefix handling)
if grep -q "sed 's/^v//'" "$INSTALL_SCRIPT"; then
  pass "Version v-prefix stripping (goreleaser compat)"
else
  fail "No v-prefix stripping — goreleaser archives use version without v"
fi

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
if grep -q 'GITHUB_TOKEN' "$INSTALL_SCRIPT"; then
  pass "GITHUB_TOKEN support for private repos"
else
  fail "GITHUB_TOKEN support missing"
fi

# Test: GitHub API asset download (for private repos)
if grep -q 'application/octet-stream' "$INSTALL_SCRIPT"; then
  pass "GitHub API asset download (Accept: application/octet-stream)"
else
  fail "GitHub API asset download not implemented"
fi

# Test: Temp directory cleanup
if grep -q 'trap.*rm -rf' "$INSTALL_SCRIPT"; then
  pass "Temp directory cleanup on exit (trap)"
else
  fail "No temp directory cleanup"
fi

# ── Unit Tests: PATH Management ─────────────────────────────────────────────

section "PATH Management"

# Test: Bash profile update
if grep -q '.bashrc' "$INSTALL_SCRIPT" && grep -q '.bash_profile' "$INSTALL_SCRIPT"; then
  pass "Bash profile update (.bashrc/.bash_profile)"
else
  fail "Bash profile update incomplete"
fi

# Test: Zsh profile update
if grep -q '.zshrc' "$INSTALL_SCRIPT"; then
  pass "Zsh profile update (.zshrc)"
else
  fail "Zsh profile update missing"
fi

# Test: Fish config update
if grep -q 'config.fish' "$INSTALL_SCRIPT"; then
  pass "Fish config update"
else
  fail "Fish config update missing"
fi

# Test: NO_MODIFY_PATH support
if grep -q 'NO_MODIFY_PATH' "$INSTALL_SCRIPT"; then
  pass "NO_MODIFY_PATH env var supported"
else
  fail "NO_MODIFY_PATH not supported"
fi

# Test: Idempotent PATH update (won't add duplicate)
if grep -q 'grep -qF.*INSTALL_DIR' "$INSTALL_SCRIPT"; then
  pass "Idempotent PATH update (checks for existing entry)"
else
  fail "PATH update may not be idempotent"
fi

# Test: Permission-denied handling for shell profiles
if grep -q 'permission denied' "$INSTALL_SCRIPT"; then
  pass "Permission-denied handling for shell profiles"
else
  fail "No permission-denied handling for shell profiles"
fi

# ── Unit Tests: Environment Variable Overrides ──────────────────────────────

section "Environment Variable Overrides"

for var in AYB_INSTALL AYB_REPO AYB_VERSION GITHUB_TOKEN NO_MODIFY_PATH; do
  if grep -q "$var" "$INSTALL_SCRIPT"; then
    pass "Env var override: $var"
  else
    fail "Env var override missing: $var"
  fi
done

# ── Unit Tests: Output & UX ────────────────────────────────────────────────

section "Output & UX"

# Test: Colored output with terminal detection
if grep -q '\[ -t 1 \]' "$INSTALL_SCRIPT"; then
  pass "Color output with terminal detection"
else
  fail "No terminal detection for colors"
fi

# Test: Success message with getting-started instructions
if grep -q 'ayb start' "$INSTALL_SCRIPT"; then
  pass "Getting-started instructions in success output"
else
  fail "No getting-started instructions"
fi

# Test: PATH reminder when binary not in PATH
if grep -q 'Restart your terminal' "$INSTALL_SCRIPT"; then
  pass "PATH reminder for new installs"
else
  fail "No PATH reminder"
fi

# Test: Archive name uses goreleaser format
if grep -q 'ayb_.*_.*_.*\.tar\.gz' "$INSTALL_SCRIPT"; then
  pass "Archive name matches goreleaser format (ayb_{ver}_{os}_{arch}.tar.gz)"
else
  fail "Archive name doesn't match goreleaser format"
fi

# Test: Downloads checksums.txt (goreleaser format)
if grep -q 'checksums.txt' "$INSTALL_SCRIPT"; then
  pass "Uses checksums.txt (goreleaser format)"
else
  fail "Does not reference checksums.txt"
fi

# ── Unit Tests: Install to User Directory ───────────────────────────────────

section "Install Location"

# Test: Installs to user directory (no sudo by default)
if grep -q 'HOME/.ayb' "$INSTALL_SCRIPT"; then
  pass "Default install to ~/.ayb (no sudo required)"
else
  fail "Does not install to user directory by default"
fi

# Test: No sudo in the script
if grep -q 'sudo' "$INSTALL_SCRIPT"; then
  fail "Script contains sudo — should install to user directory"
else
  pass "No sudo in install script"
fi

# Test: mkdir -p for install directory
if grep -q 'mkdir -p.*INSTALL_DIR' "$INSTALL_SCRIPT"; then
  pass "Creates install directory with mkdir -p"
else
  fail "No mkdir -p for install directory"
fi

# ── Release API Reachability (public, no token needed) ────────────────────────

section "Release API Reachability"

# Extract the default REPO from install.sh
default_repo=$(grep 'AYB_REPO:-' "$INSTALL_SCRIPT" | sed 's/.*AYB_REPO:-//;s/}.*//;s/"//g')

# Test: GitHub API /releases/latest returns a valid tag_name
if [ -n "${GITHUB_TOKEN:-}" ]; then
  api_resp=$(curl -fsSL -H "Authorization: token ${GITHUB_TOKEN}" "https://api.github.com/repos/${default_repo}/releases/latest" 2>&1) || true
else
  api_resp=$(curl -fsSL "https://api.github.com/repos/${default_repo}/releases/latest" 2>&1) || true
fi
if echo "$api_resp" | grep -q '"tag_name"'; then
  latest_tag=$(echo "$api_resp" | grep '"tag_name"' | head -1 | sed 's/.*"tag_name": *"//;s/".*//')
  pass "GitHub API releases/latest returns tag: ${latest_tag}"
else
  # No release yet is acceptable (staging may not have any)
  if echo "$api_resp" | grep -q '"message".*"Not Found"'; then
    pass "GitHub API reachable (no releases yet for ${default_repo})"
  else
    fail "GitHub API releases/latest for ${default_repo} failed" \
      "Got: $(echo "$api_resp" | head -3)"
  fi
fi

# Test: If releases exist, check for .tar.gz assets
if echo "$api_resp" | grep -q '"tag_name"'; then
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

    if NO_MODIFY_PATH=1 AYB_INSTALL="$test_dir" GITHUB_TOKEN="$GITHUB_TOKEN" sh "$INSTALL_SCRIPT" "$PIN_VERSION" 2>&1 | grep -q "installed successfully"; then
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
    if NO_MODIFY_PATH=1 AYB_INSTALL="$test_dir2" GITHUB_TOKEN="$GITHUB_TOKEN" sh "$INSTALL_SCRIPT" 2>&1 | grep -q "installed successfully"; then
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
    if NO_MODIFY_PATH=1 AYB_INSTALL="$test_dir4/custom" GITHUB_TOKEN="$GITHUB_TOKEN" sh "$INSTALL_SCRIPT" "$PIN_VERSION" 2>&1 | grep -q "installed successfully"; then
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
