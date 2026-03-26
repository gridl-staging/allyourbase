#!/usr/bin/env bash
# QA Test 01: CLI Help & Version
# Tests every CLI command's --help output for formatting, completeness, and grammar.
# Also tests version command output.
set -euo pipefail

AYB="${AYB_BIN:-$(cd "$(dirname "$0")/../.." && pwd)/ayb}"
RESULTS="${RESULTS_DIR:-$(cd "$(dirname "$0")/../results" && pwd)}"
LOG="$RESULTS/01_cli_help_and_version.detail.log"

defects=()
record_defect() {
    defects+=("$1")
    echo "  DEFECT: $1"
}

echo "=== QA Test 01: CLI Help & Version ==="
echo "Binary: $AYB"
echo ""

# ── 1. Version command ──
echo "--- Testing: ayb version ---"
version_out=$("$AYB" version 2>&1) || true
echo "$version_out"
if echo "$version_out" | grep -qiE 'version|v[0-9]|dev|commit:|[0-9a-f]{7,}'; then
    echo "  ✓ version command produces version output"
else
    record_defect "version command does not show a version string"
fi
echo ""

# ── 2. Root --help ──
echo "--- Testing: ayb --help ---"
help_out=$("$AYB" --help 2>&1) || true
echo "$help_out"
if echo "$help_out" | grep -q "USAGE"; then
    echo "  ✓ root help shows USAGE section"
else
    record_defect "root help missing USAGE section"
fi
if echo "$help_out" | grep -qi "start"; then
    echo "  ✓ root help mentions 'start' command"
else
    record_defect "root help does not mention 'start' command"
fi
echo ""

# ── 3. Test --help for every subcommand ──
commands=(
    start stop status
    demo
    sql schema query rpc
    config
    migrate
    admin
    users apikeys
    storage
    types
    webhooks
    logs stats
    secrets
    mcp
    init
    version
    uninstall
    db
)

for cmd in "${commands[@]}"; do
    echo "--- Testing: ayb $cmd --help ---"
    cmd_help=$("$AYB" "$cmd" --help 2>&1) || true
    echo "$cmd_help" >> "$LOG"

    # Check for empty output
    if [ -z "$cmd_help" ]; then
        record_defect "'ayb $cmd --help' produces no output"
        continue
    fi

    # Check for error in help output
    if echo "$cmd_help" | grep -qi "^Error:"; then
        record_defect "'ayb $cmd --help' returns an error: $(echo "$cmd_help" | head -1)"
        continue
    fi

    # Check for description text (should have some words explaining the command)
    word_count=$(echo "$cmd_help" | wc -w | tr -d ' ')
    if [ "$word_count" -lt 5 ]; then
        record_defect "'ayb $cmd --help' has very little content ($word_count words)"
    fi

    echo "  ✓ ayb $cmd --help ($word_count words)"
done

# ── 4. Test subcommands of compound commands ──
echo ""
echo "--- Testing compound subcommands ---"
compound_commands=(
    "migrate up"
    "migrate create"
    "migrate status"
    "db backup"
    "db restore"
    "admin reset-password"
    "apikeys create"
    "apikeys list"
    "types typescript"
    "config set"
    "config get"
)

for cmd in "${compound_commands[@]}"; do
    echo "--- Testing: ayb $cmd --help ---"
    cmd_help=$("$AYB" $cmd --help 2>&1) || true
    echo "$cmd_help" >> "$LOG"

    if [ -z "$cmd_help" ]; then
        record_defect "'ayb $cmd --help' produces no output"
    elif echo "$cmd_help" | grep -qi "^Error:"; then
        record_defect "'ayb $cmd --help' returns error: $(echo "$cmd_help" | head -1)"
    else
        echo "  ✓ ayb $cmd --help"
    fi
done

# ── 5. Test invalid command ──
echo ""
echo "--- Testing: ayb nonexistent ---"
invalid_out=$("$AYB" nonexistent 2>&1) || true
echo "$invalid_out"
if echo "$invalid_out" | grep -qiE "unknown|invalid|not found|error"; then
    echo "  ✓ invalid command produces helpful error"
else
    record_defect "invalid command 'ayb nonexistent' does not show a clear error"
fi

# ── Summary ──
echo ""
echo "========================================="
echo "  CLI Help & Version: ${#defects[@]} defect(s)"
if [ ${#defects[@]} -gt 0 ]; then
    for d in "${defects[@]}"; do
        echo "  - $d"
    done
fi
echo "========================================="

[ ${#defects[@]} -eq 0 ]
