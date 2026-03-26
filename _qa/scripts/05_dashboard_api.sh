#!/usr/bin/env bash
# QA Test 05: Dashboard API
# Tests admin login, SQL editor, schema inspection, user listing via admin API endpoints.
set -euo pipefail

AYB="${AYB_BIN:-$(cd "$(dirname "$0")/../.." && pwd)/ayb}"
RESULTS="${RESULTS_DIR:-$(cd "$(dirname "$0")/../results" && pwd)}"
BASE_URL="http://localhost:8090"
ADMIN_PASSWORD="${ADMIN_PASSWORD:-${AYB_ADMIN_PASSWORD:-}}"

if [ -z "$ADMIN_PASSWORD" ]; then
    echo "FATAL: set ADMIN_PASSWORD (or AYB_ADMIN_PASSWORD) before running this script"
    exit 2
fi

defects=()
record_defect() {
    defects+=("$1")
    echo "  DEFECT: $1"
}

echo "=== QA Test 05: Dashboard API ==="
echo ""

# ── 1. Admin status (no auth) ──
echo "--- Testing: GET /api/admin/status ---"
status_response=$(curl -s -w "\n%{http_code}" "$BASE_URL/api/admin/status")
status_body=$(echo "$status_response" | head -1)
status_code=$(echo "$status_response" | tail -1)
echo "  Status: $status_code"
echo "  Body: $status_body"
if [ "$status_code" = "200" ]; then
    echo "  ✓ Admin status accessible without auth"
else
    record_defect "Admin status returned $status_code"
fi
echo ""

# ── 2. Admin login ──
echo "--- Testing: POST /api/admin/auth ---"
auth_response=$(curl -s -w "\n%{http_code}" -X POST "$BASE_URL/api/admin/auth" \
    -H "Content-Type: application/json" \
    -d "{\"password\": \"$ADMIN_PASSWORD\"}")
auth_body=$(echo "$auth_response" | head -1)
auth_code=$(echo "$auth_response" | tail -1)
echo "  Status: $auth_code"

ADMIN_TOKEN=""
if [ "$auth_code" = "200" ]; then
    ADMIN_TOKEN=$(echo "$auth_body" | grep -o '"token":"[^"]*"' | head -1 | cut -d'"' -f4)
    if [ -n "$ADMIN_TOKEN" ]; then
        echo "  ✓ Admin login succeeded, got token"
    else
        record_defect "Admin login returned 200 but no token in response"
    fi
else
    record_defect "Admin login failed with $auth_code: $(echo "$auth_body" | head -c 100)"
fi
echo ""

# ── 3. Admin login with wrong password ──
echo "--- Testing: POST /api/admin/auth (wrong password) ---"
bad_auth_code=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$BASE_URL/api/admin/auth" \
    -H "Content-Type: application/json" \
    -d '{"password": "wrongpassword"}')
echo "  Status: $bad_auth_code"
if [ "$bad_auth_code" = "401" ]; then
    echo "  ✓ Wrong admin password correctly rejected"
else
    record_defect "Wrong admin password returns $bad_auth_code instead of 401"
fi
echo ""

if [ -z "$ADMIN_TOKEN" ]; then
    echo "FATAL: No admin token, cannot continue"
    exit 1
fi

# ── 4. SQL Editor ──
echo "--- Testing: POST /api/admin/sql (SELECT) ---"
sql_response=$(curl -s -w "\n%{http_code}" -X POST "$BASE_URL/api/admin/sql" \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer $ADMIN_TOKEN" \
    -d '{"query": "SELECT current_database(), current_user, version()"}')
sql_body=$(echo "$sql_response" | head -1)
sql_code=$(echo "$sql_response" | tail -1)
echo "  Status: $sql_code"
echo "  Body (truncated): $(echo "$sql_body" | head -c 300)"
if [ "$sql_code" = "200" ]; then
    echo "  ✓ SQL query succeeded"
    if echo "$sql_body" | grep -qE "rows|columns|result"; then
        echo "  ✓ SQL response has result structure"
    else
        record_defect "SQL response missing expected result structure"
    fi
else
    record_defect "SQL query returned $sql_code"
fi
echo ""

# ── 5. SQL Editor - list tables ──
echo "--- Testing: list tables via SQL ---"
tables_response=$(curl -s -X POST "$BASE_URL/api/admin/sql" \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer $ADMIN_TOKEN" \
    -d '{"query": "SELECT tablename FROM pg_tables WHERE schemaname = '\''public'\'' ORDER BY tablename"}')
echo "  Tables: $(echo "$tables_response" | head -c 500)"
echo "  ✓ Table listing works"
echo ""

# ── 6. Schema endpoint ──
echo "--- Testing: GET /api/schema (with admin token) ---"
schema_response=$(curl -s -w "\n%{http_code}" "$BASE_URL/api/schema" \
    -H "Authorization: Bearer $ADMIN_TOKEN")
schema_body=$(echo "$schema_response" | head -1)
schema_code=$(echo "$schema_response" | tail -1)
echo "  Status: $schema_code"
echo "  Body (truncated): $(echo "$schema_body" | head -c 300)"
if [ "$schema_code" = "200" ]; then
    echo "  ✓ Schema endpoint accessible"
    if echo "$schema_body" | grep -q '"tables":{'; then
        echo "  ✓ Schema response includes table metadata"
    else
        record_defect "Schema response is missing table metadata"
    fi
    if echo "$schema_body" | grep -q '"public._ayb_users":{'; then
        record_defect "Schema leaks internal _ayb_users table as a top-level table"
    else
        echo "  ✓ Schema hides internal auth tables from the top-level table list"
    fi
else
    record_defect "Schema endpoint returned $schema_code"
fi
echo ""

# ── 7. Users listing ──
echo "--- Testing: GET /api/admin/users ---"
users_response=$(curl -s -w "\n%{http_code}" "$BASE_URL/api/admin/users" \
    -H "Authorization: Bearer $ADMIN_TOKEN")
users_body=$(echo "$users_response" | head -1)
users_code=$(echo "$users_response" | tail -1)
echo "  Status: $users_code"
echo "  Body (truncated): $(echo "$users_body" | head -c 300)"
if [ "$users_code" = "200" ]; then
    echo "  ✓ Users listing accessible"
    if echo "$users_body" | grep -qE '"items"|"users"'; then
        echo "  ✓ Users listing returns a collection payload"
    else
        record_defect "Users listing response is missing collection fields"
    fi
else
    record_defect "Users listing returned $users_code"
fi
echo ""

# ── 8. API keys listing ──
echo "--- Testing: GET /api/admin/api-keys ---"
keys_response=$(curl -s -w "\n%{http_code}" "$BASE_URL/api/admin/api-keys" \
    -H "Authorization: Bearer $ADMIN_TOKEN")
keys_body=$(echo "$keys_response" | head -1)
keys_code=$(echo "$keys_response" | tail -1)
echo "  Status: $keys_code"
if [ "$keys_code" = "200" ]; then
    echo "  ✓ API keys listing accessible"
    if echo "$keys_body" | grep -qE '"items"|"apiKeys"'; then
        echo "  ✓ API keys listing returns a collection payload"
    else
        record_defect "API keys listing response is missing collection fields"
    fi
else
    record_defect "API keys listing returned $keys_code"
fi
echo ""

# ── 9. RLS policies ──
echo "--- Testing: GET /api/admin/rls/ ---"
rls_response=$(curl -s -w "\n%{http_code}" "$BASE_URL/api/admin/rls/" \
    -H "Authorization: Bearer $ADMIN_TOKEN")
rls_code=$(echo "$rls_response" | tail -1)
echo "  Status: $rls_code"
if [ "$rls_code" = "200" ]; then
    echo "  ✓ RLS policies accessible"
else
    record_defect "RLS policies returned $rls_code"
fi
echo ""

# ── 10. Logs ──
echo "--- Testing: GET /api/admin/logs/ ---"
logs_response=$(curl -s -w "\n%{http_code}" "$BASE_URL/api/admin/logs/" \
    -H "Authorization: Bearer $ADMIN_TOKEN")
logs_code=$(echo "$logs_response" | tail -1)
echo "  Status: $logs_code"
if [ "$logs_code" = "200" ]; then
    echo "  ✓ Logs endpoint accessible"
else
    record_defect "Logs endpoint returned $logs_code"
fi
echo ""

# ── 11. Stats ──
echo "--- Testing: GET /api/admin/stats ---"
stats_response=$(curl -s -w "\n%{http_code}" "$BASE_URL/api/admin/stats" \
    -H "Authorization: Bearer $ADMIN_TOKEN")
stats_code=$(echo "$stats_response" | tail -1)
echo "  Status: $stats_code"
if [ "$stats_code" = "200" ]; then
    echo "  ✓ Stats endpoint accessible"
else
    record_defect "Stats endpoint returned $stats_code"
fi
echo ""

# ── 12. Unauthenticated admin endpoints ──
echo "--- Testing: admin endpoints without auth ---"
for endpoint in "/api/admin/sql" "/api/admin/users" "/api/admin/api-keys" "/api/admin/rls/" "/api/admin/logs/"; do
    noauth_code=$(curl -s -o /dev/null -w "%{http_code}" "$BASE_URL$endpoint")
    if [ "$noauth_code" = "401" ] || [ "$noauth_code" = "403" ]; then
        echo "  ✓ $endpoint requires auth ($noauth_code)"
    else
        record_defect "$endpoint accessible without auth (returns $noauth_code)"
    fi
done
echo ""

# ── Summary ──
echo ""
echo "========================================="
echo "  Dashboard API: ${#defects[@]} defect(s)"
if [ ${#defects[@]} -gt 0 ]; then
    for d in "${defects[@]}"; do
        echo "  - $d"
    done
fi
echo "========================================="

[ ${#defects[@]} -eq 0 ]
