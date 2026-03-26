#!/usr/bin/env bash
# QA Test 03: API CRUD
# Tests full REST API lifecycle: create table, CRUD operations, filtering, sorting, pagination.
set -euo pipefail

AYB="${AYB_BIN:-$(cd "$(dirname "$0")/../.." && pwd)/ayb}"
RESULTS="${RESULTS_DIR:-$(cd "$(dirname "$0")/../results" && pwd)}"
BASE_URL="http://localhost:8090"
ADMIN_PASSWORD="${ADMIN_PASSWORD:-${AYB_ADMIN_PASSWORD:-}}"

if [ -z "$ADMIN_PASSWORD" ]; then
    echo "FATAL: set ADMIN_PASSWORD (or AYB_ADMIN_PASSWORD) before running this script"
    exit 2
fi

# Get admin token
ADMIN_TOKEN=$(curl -s -X POST "$BASE_URL/api/admin/auth" \
    -H "Content-Type: application/json" \
    -d "{\"password\": \"$ADMIN_PASSWORD\"}" | grep -o '"token":"[^"]*"' | cut -d'"' -f4)

# Register a test user and get a user JWT for collection API calls
TEST_USER_EMAIL="qa_crud_$(date +%s)@example.com"
TEST_USER_PASS="TestPassword123!"
register_result=$(curl -s -X POST "$BASE_URL/api/auth/register" \
    -H "Content-Type: application/json" \
    -d "{\"email\": \"$TEST_USER_EMAIL\", \"password\": \"$TEST_USER_PASS\"}")
USER_TOKEN=$(echo "$register_result" | grep -o '"token":"[^"]*"' | head -1 | cut -d'"' -f4)
AUTH_HEADER="Authorization: Bearer $USER_TOKEN"

defects=()
record_defect() {
    defects+=("$1")
    echo "  DEFECT: $1"
}

echo "=== QA Test 03: API CRUD ==="
echo ""

# ── 1. Create test table via CLI ──
echo "--- Creating test table via ayb sql ---"
TABLE_NAME="qa_test_posts_$(date +%s)"
create_result=$("$AYB" sql "CREATE TABLE $TABLE_NAME (id serial PRIMARY KEY, title text NOT NULL, body text, published boolean DEFAULT false, score integer DEFAULT 0, created_at timestamptz DEFAULT now())" 2>&1) || true
echo "  Result: $create_result"
if echo "$create_result" | grep -qiE "error|failed"; then
    record_defect "Failed to create test table: $create_result"
else
    echo "  ✓ Table $TABLE_NAME created"
fi
echo ""

# Give schema cache time to refresh
sleep 4

# ── 2. Create records via REST API ──
echo "--- Creating records via POST /api/collections/$TABLE_NAME ---"
created_ids=()
for i in 1 2 3 4 5; do
    create_response=$(curl -s -w "\n%{http_code}" -X POST "$BASE_URL/api/collections/$TABLE_NAME" \
        -H "Content-Type: application/json" \
        -H "$AUTH_HEADER" \
        -d "{\"title\": \"Test Post $i\", \"body\": \"Body for post $i\", \"published\": $([ $i -le 3 ] && echo true || echo false), \"score\": $((i * 10))}")
    create_body=$(echo "$create_response" | head -1)
    create_code=$(echo "$create_response" | tail -1)

    if [ "$create_code" = "201" ] || [ "$create_code" = "200" ]; then
        record_id=$(echo "$create_body" | grep -o '"id":[0-9]*' | head -1 | cut -d: -f2)
        created_ids+=("$record_id")
        echo "  ✓ Created record $i (id=$record_id)"
    else
        record_defect "POST create record $i returned $create_code: $create_body"
    fi
done
echo ""

# ── 3. List all records ──
echo "--- Listing records via GET /api/collections/$TABLE_NAME ---"
list_response=$(curl -s -w "\n%{http_code}" -H "$AUTH_HEADER" "$BASE_URL/api/collections/$TABLE_NAME")
list_body=$(echo "$list_response" | head -1)
list_code=$(echo "$list_response" | tail -1)
echo "  Status: $list_code"
if [ "$list_code" = "200" ]; then
    echo "  ✓ List returns 200"
    # Check structure
    if echo "$list_body" | grep -q '"items"'; then
        echo "  ✓ Response has 'items' array"
    else
        record_defect "List response missing 'items' field"
    fi
    if echo "$list_body" | grep -q '"totalItems"'; then
        echo "  ✓ Response has 'totalItems'"
    elif echo "$list_body" | grep -q '"total"'; then
        echo "  ✓ Response has 'total'"
    else
        record_defect "List response missing totalItems/total count"
    fi
else
    record_defect "List records returned $list_code: $list_body"
fi
echo ""

# ── 4. Get single record ──
if [ ${#created_ids[@]} -gt 0 ]; then
    first_id="${created_ids[0]}"
    echo "--- Getting single record via GET /api/collections/$TABLE_NAME/$first_id ---"
    get_response=$(curl -s -w "\n%{http_code}" -H "$AUTH_HEADER" "$BASE_URL/api/collections/$TABLE_NAME/$first_id")
    get_body=$(echo "$get_response" | head -1)
    get_code=$(echo "$get_response" | tail -1)
    echo "  Status: $get_code"
    if [ "$get_code" = "200" ]; then
        echo "  ✓ Get single record returns 200"
        if echo "$get_body" | grep -q "Test Post 1"; then
            echo "  ✓ Record has correct title"
        else
            record_defect "Single record has wrong title: $get_body"
        fi
    else
        record_defect "Get single record returned $get_code"
    fi
    echo ""
fi

# ── 5. Update record ──
if [ ${#created_ids[@]} -gt 0 ]; then
    first_id="${created_ids[0]}"
    echo "--- Updating record via PATCH /api/collections/$TABLE_NAME/$first_id ---"
    update_response=$(curl -s -w "\n%{http_code}" -X PATCH "$BASE_URL/api/collections/$TABLE_NAME/$first_id" \
        -H "Content-Type: application/json" \
        -H "$AUTH_HEADER" \
        -d '{"title": "Updated Title", "score": 999}')
    update_body=$(echo "$update_response" | head -1)
    update_code=$(echo "$update_response" | tail -1)
    echo "  Status: $update_code"
    if [ "$update_code" = "200" ]; then
        echo "  ✓ Update returns 200"
    else
        record_defect "Update record returned $update_code: $update_body"
    fi

    # Verify update
    verify_body=$(curl -s -H "$AUTH_HEADER" "$BASE_URL/api/collections/$TABLE_NAME/$first_id")
    if echo "$verify_body" | grep -q "Updated Title"; then
        echo "  ✓ Update persisted correctly"
    else
        record_defect "Update did not persist: $verify_body"
    fi
    echo ""
fi

# ── 6. Filter records ──
echo "--- Testing filter: published=true ---"
filter_response=$(curl -s -H "$AUTH_HEADER" "$BASE_URL/api/collections/$TABLE_NAME?filter=published%3Dtrue")
echo "  Response (truncated): $(echo "$filter_response" | head -c 200)"
if echo "$filter_response" | grep -q '"items"'; then
    echo "  ✓ Filter returns items"
else
    record_defect "Filter query did not return items array"
fi
echo ""

# ── 7. Sort records ──
echo "--- Testing sort: -score ---"
sort_response=$(curl -s -H "$AUTH_HEADER" "$BASE_URL/api/collections/$TABLE_NAME?sort=-score")
echo "  Response (truncated): $(echo "$sort_response" | head -c 200)"
if echo "$sort_response" | grep -q '"items"'; then
    echo "  ✓ Sort returns items"
else
    record_defect "Sort query did not return items array"
fi
echo ""

# ── 8. Pagination ──
echo "--- Testing pagination: perPage=2&page=1 ---"
page_response=$(curl -s -H "$AUTH_HEADER" "$BASE_URL/api/collections/$TABLE_NAME?perPage=2&page=1")
echo "  Response (truncated): $(echo "$page_response" | head -c 300)"
if echo "$page_response" | grep -q '"items"'; then
    echo "  ✓ Pagination returns items"
else
    record_defect "Pagination did not return items array"
fi
echo ""

# ── 9. Delete record ──
if [ ${#created_ids[@]} -gt 1 ]; then
    del_id="${created_ids[1]}"
    echo "--- Deleting record via DELETE /api/collections/$TABLE_NAME/$del_id ---"
    del_response=$(curl -s -w "\n%{http_code}" -X DELETE "$BASE_URL/api/collections/$TABLE_NAME/$del_id" \
        -H "$AUTH_HEADER")
    del_code=$(echo "$del_response" | tail -1)
    echo "  Status: $del_code"
    if [ "$del_code" = "200" ] || [ "$del_code" = "204" ]; then
        echo "  ✓ Delete returns $del_code"
    else
        del_body=$(echo "$del_response" | head -1)
        record_defect "Delete record returned $del_code: $del_body"
    fi

    # Verify deletion
    verify_del=$(curl -s -o /dev/null -w "%{http_code}" -H "$AUTH_HEADER" "$BASE_URL/api/collections/$TABLE_NAME/$del_id")
    if [ "$verify_del" = "404" ]; then
        echo "  ✓ Deleted record returns 404"
    else
        record_defect "Deleted record still accessible (returns $verify_del)"
    fi
    echo ""
fi

# ── 10. Non-existent record ──
echo "--- Testing: GET non-existent record ---"
notfound_code=$(curl -s -o /dev/null -w "%{http_code}" -H "$AUTH_HEADER" "$BASE_URL/api/collections/$TABLE_NAME/99999")
echo "  Status: $notfound_code"
if [ "$notfound_code" = "404" ]; then
    echo "  ✓ Non-existent record returns 404"
else
    record_defect "Non-existent record returns $notfound_code instead of 404"
fi
echo ""

# ── 11. Non-existent table ──
echo "--- Testing: GET non-existent table ---"
notable_code=$(curl -s -o /dev/null -w "%{http_code}" -H "$AUTH_HEADER" "$BASE_URL/api/collections/nonexistent_table_xyz")
echo "  Status: $notable_code"
if [ "$notable_code" = "404" ]; then
    echo "  ✓ Non-existent table returns 404"
else
    record_defect "Non-existent table returns $notable_code instead of 404"
fi
echo ""

# ── Cleanup ──
echo "--- Cleanup: dropping test table ---"
"$AYB" sql "DROP TABLE IF EXISTS $TABLE_NAME" > /dev/null 2>&1 || true
echo "  ✓ Cleaned up"

# ── Summary ──
echo ""
echo "========================================="
echo "  API CRUD: ${#defects[@]} defect(s)"
if [ ${#defects[@]} -gt 0 ]; then
    for d in "${defects[@]}"; do
        echo "  - $d"
    done
fi
echo "========================================="

[ ${#defects[@]} -eq 0 ]
