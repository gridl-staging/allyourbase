#!/usr/bin/env bash
# QA Test 02: Server Lifecycle
# Tests start/stop/status cycle, measures startup time, checks health endpoint.
set -euo pipefail

AYB="${AYB_BIN:-$(cd "$(dirname "$0")/../.." && pwd)/ayb}"
RESULTS="${RESULTS_DIR:-$(cd "$(dirname "$0")/../results" && pwd)}"

defects=()
record_defect() {
    defects+=("$1")
    echo "  DEFECT: $1"
}

echo "=== QA Test 02: Server Lifecycle ==="
echo ""

# ── 1. Stop any running server first ──
echo "--- Stopping any running server ---"
"$AYB" stop 2>&1 || true
sleep 2

# ── 2. Check status when stopped ──
echo "--- Testing: ayb status (when stopped) ---"
status_stopped=$("$AYB" status 2>&1) || true
echo "$status_stopped"
if echo "$status_stopped" | grep -qiE "not running|no server|stopped"; then
    echo "  ✓ status correctly reports server not running"
else
    record_defect "status does not clearly indicate server is stopped"
fi
echo ""

# ── 3. Start server and measure time ──
echo "--- Starting server (measuring time) ---"
start_time=$(date +%s)
start_output=$("$AYB" start 2>&1) || true
end_time=$(date +%s)
startup_seconds=$((end_time - start_time))
echo "$start_output"
echo "  Startup took: ${startup_seconds}s"

if [ "$startup_seconds" -gt 30 ]; then
    record_defect "Server startup took ${startup_seconds}s (>30s is too slow for UX)"
fi

# Wait for health
echo "--- Waiting for health endpoint ---"
health_wait_start=$(date +%s)
for i in $(seq 1 30); do
    if curl -s http://localhost:8090/health | grep -q "ok"; then
        break
    fi
    sleep 1
done
health_wait_end=$(date +%s)
health_wait=$((health_wait_end - health_wait_start))
echo "  Health endpoint ready after ${health_wait}s additional wait"
echo ""

# ── 4. Test health endpoint ──
echo "--- Testing: GET /health ---"
health_response=$(curl -s -w "\n%{http_code}" http://localhost:8090/health)
health_body=$(echo "$health_response" | head -1)
health_code=$(echo "$health_response" | tail -1)
echo "  Status: $health_code"
echo "  Body: $health_body"
if [ "$health_code" = "200" ]; then
    echo "  ✓ health returns 200"
else
    record_defect "health endpoint returns $health_code instead of 200"
fi
if echo "$health_body" | grep -q '"status"'; then
    echo "  ✓ health response has status field"
else
    record_defect "health response missing status field"
fi
echo ""

# ── 5. Test status when running ──
echo "--- Testing: ayb status (when running) ---"
status_running=$("$AYB" status 2>&1) || true
echo "$status_running"
if echo "$status_running" | grep -qiE "running|healthy|PID"; then
    echo "  ✓ status correctly reports server running"
else
    record_defect "status does not indicate server is running"
fi
echo ""

# ── 6. Test double start ──
echo "--- Testing: ayb start (when already running) ---"
double_start=$("$AYB" start 2>&1) || true
echo "$double_start"
if echo "$double_start" | grep -qiE "already running|already started"; then
    echo "  ✓ double start handled gracefully"
else
    record_defect "starting server when already running does not show clear message"
fi
echo ""

# ── 7. Test admin dashboard accessibility ──
echo "--- Testing: GET /admin ---"
admin_code=$(curl -s -o /dev/null -w "%{http_code}" http://localhost:8090/admin)
echo "  Status: $admin_code"
if [ "$admin_code" = "200" ]; then
    echo "  ✓ admin dashboard accessible"
else
    record_defect "admin dashboard returns $admin_code instead of 200"
fi

# Check admin page content
admin_body=$(curl -s http://localhost:8090/admin)
if echo "$admin_body" | grep -qiE "<html|<div|react|app"; then
    echo "  ✓ admin dashboard returns HTML content"
else
    record_defect "admin dashboard does not return HTML/React content"
fi
echo ""

# ── 8. Test OpenAPI spec endpoint ──
echo "--- Testing: GET /api/openapi.yaml ---"
openapi_code=$(curl -s -o /dev/null -w "%{http_code}" http://localhost:8090/api/openapi.yaml)
echo "  Status: $openapi_code"
if [ "$openapi_code" = "200" ]; then
    echo "  ✓ OpenAPI spec accessible"
else
    record_defect "OpenAPI spec returns $openapi_code instead of 200"
fi
echo ""

# ── 9. Test schema endpoint ──
echo "--- Testing: GET /api/schema ---"
schema_response=$(curl -s -w "\n%{http_code}" http://localhost:8090/api/schema)
schema_code=$(echo "$schema_response" | tail -1)
echo "  Status: $schema_code"
if [ "$schema_code" = "200" ]; then
    echo "  ✓ schema endpoint accessible"
elif [ "$schema_code" = "401" ]; then
    echo "  ✓ schema endpoint correctly requires auth"
else
    record_defect "schema endpoint returns unexpected $schema_code"
fi
echo ""

# ── 10. Stop server ──
echo "--- Testing: ayb stop ---"
stop_start=$(date +%s)
stop_output=$("$AYB" stop 2>&1) || true
stop_end=$(date +%s)
stop_seconds=$((stop_end - stop_start))
echo "$stop_output"
echo "  Stop took: ${stop_seconds}s"

if [ "$stop_seconds" -gt 15 ]; then
    record_defect "Server stop took ${stop_seconds}s (>15s)"
fi

# Verify stopped
sleep 2
if curl -s http://localhost:8090/health > /dev/null 2>&1; then
    record_defect "Server still responding after stop"
else
    echo "  ✓ Server stopped successfully"
fi
echo ""

# ── 11. Restart server for remaining tests ──
echo "--- Restarting server for remaining QA tests ---"
"$AYB" start 2>&1 || true
for i in $(seq 1 30); do
    if curl -s http://localhost:8090/health | grep -q "ok"; then
        break
    fi
    sleep 1
done
echo "  ✓ Server restarted"

# ── Summary ──
echo ""
echo "========================================="
echo "  Server Lifecycle: ${#defects[@]} defect(s)"
echo "  Startup time: ${startup_seconds}s"
echo "  Stop time: ${stop_seconds}s"
if [ ${#defects[@]} -gt 0 ]; then
    for d in "${defects[@]}"; do
        echo "  - $d"
    done
fi
echo "========================================="

[ ${#defects[@]} -eq 0 ]
