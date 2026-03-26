#!/usr/bin/env bash
# QA Test 07: CLI Data Commands
# Tests ayb sql, ayb schema, ayb config, ayb logs, ayb stats, ayb users, ayb apikeys.
set -euo pipefail

AYB="${AYB_BIN:-$(cd "$(dirname "$0")/../.." && pwd)/ayb}"
RESULTS="${RESULTS_DIR:-$(cd "$(dirname "$0")/../results" && pwd)}"

defects=()
record_defect() {
    defects+=("$1")
    echo "  DEFECT: $1"
}

echo "=== QA Test 07: CLI Data Commands ==="
echo ""

# ── 1. ayb sql ──
echo "--- Testing: ayb sql ---"
sql_out=$("$AYB" sql "SELECT 1+1 AS result" 2>&1) || true
echo "$sql_out"
if echo "$sql_out" | grep -qE "2|result"; then
    echo "  ✓ ayb sql executes and returns result"
else
    record_defect "ayb sql 'SELECT 1+1' did not return expected result"
fi
echo ""

# ── 2. ayb sql with table creation ──
echo "--- Testing: ayb sql (CREATE TABLE + INSERT) ---"
"$AYB" sql "CREATE TABLE IF NOT EXISTS qa_cli_test (id serial PRIMARY KEY, name text, val int)" 2>&1 || true
insert_out=$("$AYB" sql "INSERT INTO qa_cli_test (name, val) VALUES ('test1', 42) RETURNING *" 2>&1) || true
echo "$insert_out"
if echo "$insert_out" | grep -qE "test1|42"; then
    echo "  ✓ SQL insert with RETURNING works"
else
    record_defect "SQL insert with RETURNING did not show inserted data"
fi
echo ""

# ── 3. ayb schema ──
echo "--- Testing: ayb schema ---"
schema_out=$("$AYB" schema 2>&1) || true
echo "$schema_out"
if echo "$schema_out" | grep -qiE "table|ayb_users|qa_cli_test"; then
    echo "  ✓ ayb schema shows tables"
else
    record_defect "ayb schema does not show tables"
fi
echo ""

# ── 4. ayb schema <table> ──
echo "--- Testing: ayb schema qa_cli_test ---"
table_schema=$("$AYB" schema qa_cli_test 2>&1) || true
echo "$table_schema"
if echo "$table_schema" | grep -qiE "column|name|val|id"; then
    echo "  ✓ ayb schema <table> shows columns"
else
    record_defect "ayb schema <table> does not show column details"
fi
echo ""

# ── 5. ayb config ──
echo "--- Testing: ayb config ---"
config_out=$("$AYB" config 2>&1) || true
echo "$config_out"
if echo "$config_out" | grep -qiE "port|server|database|auth"; then
    echo "  ✓ ayb config shows configuration"
else
    record_defect "ayb config does not show configuration"
fi
echo ""

# ── 6. ayb logs ──
echo "--- Testing: ayb logs ---"
logs_out=$("$AYB" logs 2>&1) || true
echo "  Output (first 5 lines):"
echo "$logs_out" | head -5
if [ -n "$logs_out" ]; then
    echo "  ✓ ayb logs produces output"
else
    record_defect "ayb logs produces no output"
fi
echo ""

# ── 7. ayb stats ──
echo "--- Testing: ayb stats ---"
stats_out=$("$AYB" stats 2>&1) || true
echo "$stats_out"
if [ -n "$stats_out" ]; then
    echo "  ✓ ayb stats produces output"
else
    record_defect "ayb stats produces no output"
fi
echo ""

# ── 8. ayb users ──
echo "--- Testing: ayb users ---"
users_out=$("$AYB" users 2>&1) || true
echo "$users_out"
if [ -n "$users_out" ]; then
    echo "  ✓ ayb users produces output"
else
    record_defect "ayb users produces no output (empty user list is ok if no users registered)"
fi
echo ""

# ── 9. ayb apikeys list ──
echo "--- Testing: ayb apikeys list ---"
apikeys_out=$("$AYB" apikeys list 2>&1) || true
echo "$apikeys_out"
if [ -n "$apikeys_out" ]; then
    echo "  ✓ ayb apikeys list produces output"
else
    echo "  (empty output — no API keys exist yet, which is expected)"
fi
echo ""

# ── 10. ayb version ──
echo "--- Testing: ayb version ---"
version_out=$("$AYB" version 2>&1) || true
echo "$version_out"
if echo "$version_out" | grep -qiE "version|v[0-9]|dev|commit"; then
    echo "  ✓ ayb version produces version info"
else
    record_defect "ayb version does not produce version info"
fi
echo ""

# ── 11. ayb types typescript ──
echo "--- Testing: ayb types typescript ---"
types_out=$("$AYB" types typescript 2>&1) || true
echo "  Output (first 20 lines):"
echo "$types_out" | head -20
if echo "$types_out" | grep -qiE "interface|type|export"; then
    echo "  ✓ TypeScript type generation works"
else
    record_defect "TypeScript type generation does not produce type definitions"
fi
echo ""

# ── Cleanup ──
"$AYB" sql "DROP TABLE IF EXISTS qa_cli_test" 2>&1 || true

# ── Summary ──
echo ""
echo "========================================="
echo "  CLI Data Commands: ${#defects[@]} defect(s)"
if [ ${#defects[@]} -gt 0 ]; then
    for d in "${defects[@]}"; do
        echo "  - $d"
    done
fi
echo "========================================="

[ ${#defects[@]} -eq 0 ]
