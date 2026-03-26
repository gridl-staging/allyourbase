#!/usr/bin/env bash
# QA Test 04: Auth Flow
# Tests user registration, login, JWT usage, /me, refresh tokens.
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

echo "=== QA Test 04: Auth Flow ==="
echo ""

TEST_EMAIL="qa_test_$(date +%s)@example.com"
TEST_PASSWORD="SecurePassword123!"

# ── 1. Register a new user ──
echo "--- Registering new user: $TEST_EMAIL ---"
register_response=$(curl -s -w "\n%{http_code}" -X POST "$BASE_URL/api/auth/register" \
    -H "Content-Type: application/json" \
    -d "{\"email\": \"$TEST_EMAIL\", \"password\": \"$TEST_PASSWORD\"}")
register_body=$(echo "$register_response" | head -1)
register_code=$(echo "$register_response" | tail -1)
echo "  Status: $register_code"
echo "  Body: $(echo "$register_body" | head -c 200)"

if [ "$register_code" = "200" ] || [ "$register_code" = "201" ]; then
    echo "  ✓ Registration succeeded"
    if echo "$register_body" | grep -q '"token"'; then
        echo "  ✓ Registration returns token"
    else
        record_defect "Registration response missing token"
    fi
else
    record_defect "Registration failed with $register_code: $(echo "$register_body" | head -c 100)"
fi
echo ""

# ── 2. Login ──
echo "--- Logging in as $TEST_EMAIL ---"
login_response=$(curl -s -w "\n%{http_code}" -X POST "$BASE_URL/api/auth/login" \
    -H "Content-Type: application/json" \
    -d "{\"email\": \"$TEST_EMAIL\", \"password\": \"$TEST_PASSWORD\"}")
login_body=$(echo "$login_response" | head -1)
login_code=$(echo "$login_response" | tail -1)
echo "  Status: $login_code"

if [ "$login_code" = "200" ]; then
    echo "  ✓ Login succeeded"
else
    record_defect "Login failed with $login_code: $(echo "$login_body" | head -c 100)"
fi

# Extract tokens
USER_TOKEN=$(echo "$login_body" | grep -o '"token":"[^"]*"' | head -1 | cut -d'"' -f4)
REFRESH_TOKEN=$(echo "$login_body" | grep -o '"refreshToken":"[^"]*"' | head -1 | cut -d'"' -f4)

if [ -n "$USER_TOKEN" ]; then
    echo "  ✓ Got access token"
else
    record_defect "Login response missing access token"
fi
if [ -n "$REFRESH_TOKEN" ]; then
    echo "  ✓ Got refresh token"
else
    record_defect "Login response missing refresh token"
fi
echo ""

# ── 3. Get /me ──
echo "--- Testing: GET /api/auth/me ---"
if [ -n "$USER_TOKEN" ]; then
    me_response=$(curl -s -w "\n%{http_code}" "$BASE_URL/api/auth/me" \
        -H "Authorization: Bearer $USER_TOKEN")
    me_body=$(echo "$me_response" | head -1)
    me_code=$(echo "$me_response" | tail -1)
    echo "  Status: $me_code"
    echo "  Body: $(echo "$me_body" | head -c 200)"

    if [ "$me_code" = "200" ]; then
        echo "  ✓ /me returns 200"
        if echo "$me_body" | grep -q "$TEST_EMAIL"; then
            echo "  ✓ /me returns correct email"
        else
            record_defect "/me response does not contain the user's email"
        fi
    else
        record_defect "/me returned $me_code"
    fi
else
    record_defect "Cannot test /me — no token available"
fi
echo ""

# ── 4. Unauthenticated /me ──
echo "--- Testing: GET /api/auth/me (no token) ---"
noauth_code=$(curl -s -o /dev/null -w "%{http_code}" "$BASE_URL/api/auth/me")
echo "  Status: $noauth_code"
if [ "$noauth_code" = "401" ]; then
    echo "  ✓ Unauthenticated /me returns 401"
else
    record_defect "Unauthenticated /me returns $noauth_code instead of 401"
fi
echo ""

# ── 5. Refresh token ──
echo "--- Testing: POST /api/auth/refresh ---"
if [ -n "$REFRESH_TOKEN" ]; then
    refresh_response=$(curl -s -w "\n%{http_code}" -X POST "$BASE_URL/api/auth/refresh" \
        -H "Content-Type: application/json" \
        -d "{\"refreshToken\": \"$REFRESH_TOKEN\"}")
    refresh_body=$(echo "$refresh_response" | head -1)
    refresh_code=$(echo "$refresh_response" | tail -1)
    echo "  Status: $refresh_code"

    if [ "$refresh_code" = "200" ]; then
        echo "  ✓ Refresh returns 200"
        if echo "$refresh_body" | grep -q '"token"'; then
            echo "  ✓ Refresh returns new token"
        else
            record_defect "Refresh response missing new token"
        fi
    else
        record_defect "Refresh token returned $refresh_code: $(echo "$refresh_body" | head -c 100)"
    fi
else
    record_defect "Cannot test refresh — no refresh token"
fi
echo ""

# ── 6. Invalid login ──
echo "--- Testing: login with wrong password ---"
bad_login_response=$(curl -s -w "\n%{http_code}" -X POST "$BASE_URL/api/auth/login" \
    -H "Content-Type: application/json" \
    -d "{\"email\": \"$TEST_EMAIL\", \"password\": \"wrongpassword\"}")
bad_login_code=$(echo "$bad_login_response" | tail -1)
echo "  Status: $bad_login_code"
if [ "$bad_login_code" = "401" ] || [ "$bad_login_code" = "400" ]; then
    echo "  ✓ Wrong password correctly rejected ($bad_login_code)"
else
    record_defect "Wrong password returns $bad_login_code instead of 401"
fi
echo ""

# ── 7. Duplicate registration ──
echo "--- Testing: duplicate registration ---"
dup_response=$(curl -s -w "\n%{http_code}" -X POST "$BASE_URL/api/auth/register" \
    -H "Content-Type: application/json" \
    -d "{\"email\": \"$TEST_EMAIL\", \"password\": \"$TEST_PASSWORD\"}")
dup_code=$(echo "$dup_response" | tail -1)
dup_body=$(echo "$dup_response" | head -1)
echo "  Status: $dup_code"
if [ "$dup_code" = "409" ] || [ "$dup_code" = "400" ] || [ "$dup_code" = "422" ]; then
    echo "  ✓ Duplicate registration correctly rejected ($dup_code)"
else
    record_defect "Duplicate registration returns $dup_code instead of 409/400: $dup_body"
fi
echo ""

# ── 8. Invalid email format ──
echo "--- Testing: registration with invalid email ---"
bad_email_response=$(curl -s -w "\n%{http_code}" -X POST "$BASE_URL/api/auth/register" \
    -H "Content-Type: application/json" \
    -d '{"email": "not-an-email", "password": "SecurePassword123!"}')
bad_email_code=$(echo "$bad_email_response" | tail -1)
echo "  Status: $bad_email_code"
if [ "$bad_email_code" = "400" ] || [ "$bad_email_code" = "422" ]; then
    echo "  ✓ Invalid email correctly rejected ($bad_email_code)"
else
    record_defect "Invalid email registration returns $bad_email_code instead of 400/422"
fi
echo ""

# ── Cleanup: delete test user via admin API ──
echo "--- Cleanup ---"
ADMIN_TOKEN=$(curl -s -X POST "$BASE_URL/api/admin/auth" \
    -H "Content-Type: application/json" \
    -d "{\"password\": \"$ADMIN_PASSWORD\"}" | grep -o '"token":"[^"]*"' | cut -d'"' -f4)
# Find and delete user
users_list=$(curl -s "$BASE_URL/api/admin/users/" \
    -H "Authorization: Bearer $ADMIN_TOKEN")
user_id=$(echo "$users_list" | grep -o "\"id\":\"[^\"]*\"" | head -1 | cut -d'"' -f4)
# We don't know the exact user ID format, just note cleanup
echo "  (test user cleanup: manual)"

# ── Summary ──
echo ""
echo "========================================="
echo "  Auth Flow: ${#defects[@]} defect(s)"
if [ ${#defects[@]} -gt 0 ]; then
    for d in "${defects[@]}"; do
        echo "  - $d"
    done
fi
echo "========================================="

[ ${#defects[@]} -eq 0 ]
