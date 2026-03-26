#!/usr/bin/env bash
# Run all QA bash scripts sequentially, collecting results.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
QA_DIR="$(dirname "$SCRIPT_DIR")"
RESULTS_DIR="$QA_DIR/results"
AYB_BIN="${AYB_BIN:-$(dirname "$QA_DIR")/ayb}"

export AYB_BIN RESULTS_DIR QA_DIR

# Exploratory QA needs a stable auth-enabled server because the suite exercises
# public auth flows, demo account creation, and admin login.
export AYB_AUTH_ENABLED="${AYB_AUTH_ENABLED:-true}"
export AYB_AUTH_JWT_SECRET="${AYB_AUTH_JWT_SECRET:-qa-suite-jwt-secret-2026-03-26-at-least-32-chars}"
export AYB_ADMIN_PASSWORD="${AYB_ADMIN_PASSWORD:-QaSuiteAdminPass123!}"
export ADMIN_PASSWORD="${ADMIN_PASSWORD:-$AYB_ADMIN_PASSWORD}"

mkdir -p "$RESULTS_DIR/screenshots"

passed=0
failed=0
errors=()

for script in "$SCRIPT_DIR"/[0-9]*.sh; do
    name="$(basename "$script")"
    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "  Running: $name"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    log_file="$RESULTS_DIR/${name%.sh}.log"
    if bash "$script" > "$log_file" 2>&1; then
        echo "  ✓ PASSED"
        ((passed++))
    else
        echo "  ✗ FAILED (see $log_file)"
        ((failed++))
        errors+=("$name")
    fi
done

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  QA SUMMARY: $passed passed, $failed failed"
if [ ${#errors[@]} -gt 0 ]; then
    echo "  Failed: ${errors[*]}"
fi
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
[ $failed -eq 0 ]
