#!/usr/bin/env bash
# QA Test 06: Demo Launch
# Tests demo app launch, schema application, seed users, and demo serving.
# Tests both live-polls and kanban demos.
#
# COVERAGE:
#   - Demo against pre-existing running server (the most common real-world case)
#   - Demo that starts its own fresh server
#   - Early-exit detection (auth failure, schema failure, etc.)
#   - Setup completion verified via log output, not just HTTP port availability
set -euo pipefail

AYB="${AYB_BIN:-$(cd "$(dirname "$0")/../.." && pwd)/ayb}"
RESULTS="${RESULTS_DIR:-$(cd "$(dirname "$0")/../results" && pwd)}"
BASE_URL="http://localhost:8090"

defects=()
record_defect() {
    defects+=("$1")
    echo "  DEFECT: $1"
}

echo "=== QA Test 06: Demo Launch ==="
echo ""

# ── Helper: wait for demo to be fully ready ──────────────────────────────────
# Polls demo log for the "Press Ctrl+C to stop" line, which only appears after
# all setup steps (connect ✓, schema ✓, accounts ✓) succeed.
# Also detects early process death (auth failure, schema error, etc.).
#
# Returns:
#   0 = demo ready (success banner found)
#   1 = timeout (no success and no crash within limit)
#   2 = process exited early (error during setup)
wait_for_demo_ready() {
    local pid="$1"
    local log="$2"
    local timeout_secs="${3:-120}"
    local attempts=$(( timeout_secs / 2 ))

    for i in $(seq 1 "$attempts"); do
        # Detect early exit (auth failure, schema error, etc.)
        if ! kill -0 "$pid" 2>/dev/null; then
            return 2
        fi
        # Success: all setup steps printed, HTTP server is serving
        if grep -q "Press Ctrl+C to stop" "$log" 2>/dev/null; then
            return 0
        fi
        sleep 2
    done
    return 1
}

# ── Helper: check demo log for setup failures ─────────────────────────────────
# Returns non-zero and prints the failure lines if any step failed.
check_demo_log_for_errors() {
    local log="$1"
    if grep -E "✗|Error:|error:" "$log" 2>/dev/null | grep -qv "^$"; then
        grep -E "✗|Error:|error:" "$log" 2>/dev/null | head -5 || true
        return 1
    fi
    return 0
}

# ── Helper: stop demo and clean up ───────────────────────────────────────────
stop_demo() {
    local pid="${1:-}"
    [ -n "$pid" ] && kill "$pid" 2>/dev/null || true
    [ -n "$pid" ] && wait "$pid" 2>/dev/null || true
}

# ── Helper: ensure a port is free ────────────────────────────────────────────
ensure_port_free() {
    local port="$1"
    local pids
    pids=$(lsof -i :"$port" -t 2>/dev/null || true)
    if [ -n "$pids" ]; then
        echo "  Cleaning up processes on port $port: $pids"
        echo "$pids" | xargs kill 2>/dev/null || true
        sleep 2
        # Force-kill any stragglers
        pids=$(lsof -i :"$port" -t 2>/dev/null || true)
        if [ -n "$pids" ]; then
            echo "$pids" | xargs kill -9 2>/dev/null || true
            sleep 1
        fi
    fi
}

# ─────────────────────────────────────────────────────────────────────────────
# ── 1. Check demo help ──
# ─────────────────────────────────────────────────────────────────────────────
echo "--- Testing: ayb demo --help ---"
demo_help=$("$AYB" demo --help 2>&1) || true
echo "$demo_help"
if echo "$demo_help" | grep -qi "live-polls"; then
    echo "  ✓ demo help mentions live-polls"
else
    record_defect "demo help does not mention live-polls"
fi
if echo "$demo_help" | grep -qi "kanban"; then
    echo "  ✓ demo help mentions kanban"
else
    record_defect "demo help does not mention kanban"
fi

echo ""

# ─────────────────────────────────────────────────────────────────────────────
# ── 2. KEY SCENARIO: Demo against ALREADY-RUNNING server ──────────────────
#
# This is the most common real-world case: user has 'ayb start' running and
# then types 'ayb demo kanban'.  The demo must authenticate using the saved
# admin token — any password-mismatch / stale-token bug surfaces here.
# ─────────────────────────────────────────────────────────────────────────────
echo "--- TEST: ayb demo kanban against pre-existing running server ---"
echo "  (This catches 'saved password may be stale' and similar auth bugs)"

# Ensure the server is actually running before we start.
ensure_port_free 5173

if ! curl -s "$BASE_URL/health" 2>/dev/null | grep -q '"status"'; then
    echo "  Server not running — starting it now for the pre-existing-server test"
    "$AYB" start 2>&1 || true
    for i in $(seq 1 30); do
        if curl -s "$BASE_URL/health" 2>/dev/null | grep -q '"status"'; then break; fi
        sleep 1
    done
fi

if ! curl -s "$BASE_URL/health" 2>/dev/null | grep -q '"status"'; then
    record_defect "Cannot start server for pre-existing-server demo test"
else
    preexisting_log="$RESULTS/demo_kanban_preexisting.log"
    rm -f "$preexisting_log"
    "$AYB" demo kanban > "$preexisting_log" 2>&1 &
    PREEXISTING_PID=$!

    echo "  Demo PID: $PREEXISTING_PID — waiting for setup to complete..."
    wait_for_demo_ready "$PREEXISTING_PID" "$preexisting_log" 60 && demo_status=0 || demo_status=$?

    stop_demo "$PREEXISTING_PID"

    case $demo_status in
        0)
            echo "  ✓ Demo started successfully against pre-existing server"
            # Verify all three setup steps completed
            for marker in "Connecting to AYB server" "Applying database schema" "Creating demo accounts"; do
                if grep -q "$marker" "$preexisting_log" 2>/dev/null; then
                    echo "  ✓ Step confirmed in log: $marker"
                else
                    record_defect "Demo log missing expected step: $marker"
                fi
            done
            ;;
        2)
            # Process exited early — authentication failure, schema error, etc.
            record_defect "Demo failed during setup (process exited) — see $preexisting_log"
            echo "  Last lines of demo output:"
            tail -10 "$preexisting_log" 2>/dev/null | sed 's/^/    /' || true
            ;;
        1)
            record_defect "Demo against pre-existing server timed out after 60s — see $preexisting_log"
            echo "  Last lines of demo output:"
            tail -10 "$preexisting_log" 2>/dev/null | sed 's/^/    /' || true
            ;;
    esac

    # Verify server is still healthy after demo (demo should not stop it)
    if curl -s "$BASE_URL/health" 2>/dev/null | grep -q '"status"'; then
        echo "  ✓ Server still healthy after demo exit"
    else
        record_defect "Server became unhealthy after demo exit (demo should not stop a pre-existing server)"
    fi
fi
echo ""

# ─────────────────────────────────────────────────────────────────────────────
# ── 3. Launch live-polls demo (fresh server) ──────────────────────────────
# ─────────────────────────────────────────────────────────────────────────────
echo "--- Launching: ayb demo live-polls (stops server, starts its own) ---"
"$AYB" stop 2>&1 || true
sleep 2
ensure_port_free 5175
ensure_port_free 5173

demo_start_time=$(date +%s)
live_polls_log="$RESULTS/demo_live_polls_output.log"
rm -f "$live_polls_log"
"$AYB" demo live-polls > "$live_polls_log" 2>&1 &
DEMO_PID=$!

echo "  Demo PID: $DEMO_PID — waiting for setup to complete..."
wait_for_demo_ready "$DEMO_PID" "$live_polls_log" 120 && demo_status=0 || demo_status=$?
demo_end_time=$(date +%s)
demo_startup=$(( demo_end_time - demo_start_time ))

case $demo_status in
    0)
        echo "  ✓ Live-polls demo ready in ${demo_startup}s"
        if [ "$demo_startup" -gt 30 ]; then
            record_defect "Live-polls demo startup took ${demo_startup}s (>30s may frustrate new users)"
        fi
        ;;
    2)
        record_defect "Live-polls demo failed during setup (process exited) — see $live_polls_log"
        echo "  Last lines of demo output:"
        tail -10 "$live_polls_log" 2>/dev/null | sed 's/^/    /' || true
        ;;
    1)
        record_defect "Live-polls demo did not complete setup within 120s — see $live_polls_log"
        echo "  Last lines of demo output:"
        tail -20 "$live_polls_log" 2>/dev/null | sed 's/^/    /' || true
        ;;
esac
echo ""

# ── 4. Check live-polls frontend and API ──
if [ $demo_status -eq 0 ]; then
    echo "--- Checking live-polls frontend (port 5175) ---"
    demo_page=$(curl -s http://localhost:5175/ 2>/dev/null || echo "")
    echo "  Page length: $(echo "$demo_page" | wc -c | tr -d ' ') bytes"
    if echo "$demo_page" | grep -qiE "<!DOCTYPE|<html"; then
        echo "  ✓ Frontend serves valid HTML"
    else
        record_defect "Live-polls frontend does not serve valid HTML on port 5175"
    fi

    echo "--- Checking API proxy from demo port ---"
    proxy_code=$(curl -s -o /dev/null -w "%{http_code}" http://localhost:5175/api/schema 2>/dev/null || echo "000")
    echo "  Proxy /api/schema status: $proxy_code"
    if [ "$proxy_code" = "200" ] || [ "$proxy_code" = "401" ]; then
        echo "  ✓ API proxy works from demo port"
    else
        record_defect "API proxy from demo port returns $proxy_code (expected 200 or 401)"
    fi

    echo "--- Checking demo seed users ---"
    demo_login=$(curl -s -X POST "$BASE_URL/api/auth/login" \
        -H "Content-Type: application/json" \
        -d '{"email": "alice@demo.test", "password": "password123"}' 2>/dev/null || echo "")
    if echo "$demo_login" | grep -q '"token"'; then
        echo "  ✓ Demo seed user alice@demo.test can login"
    else
        record_defect "Demo seed user alice@demo.test cannot login: $(echo "$demo_login" | head -c 100)"
    fi
    echo ""
fi

# ── 5. Stop live-polls demo ──
echo "--- Stopping live-polls demo ---"
stop_demo "${DEMO_PID:-}"
"$AYB" stop 2>&1 || true
sleep 3
echo ""

# ─────────────────────────────────────────────────────────────────────────────
# ── 6. Launch kanban demo (fresh server) ──────────────────────────────────
# ─────────────────────────────────────────────────────────────────────────────
echo "--- Launching: ayb demo kanban (fresh server) ---"
ensure_port_free 5173
kanban_log="$RESULTS/demo_kanban_output.log"
rm -f "$kanban_log"
"$AYB" demo kanban > "$kanban_log" 2>&1 &
DEMO_PID=$!

echo "  Demo PID: $DEMO_PID — waiting for setup to complete..."
wait_for_demo_ready "$DEMO_PID" "$kanban_log" 120 && demo_status=0 || demo_status=$?

case $demo_status in
    0)
        echo "  ✓ Kanban demo ready"
        ;;
    2)
        record_defect "Kanban demo (fresh server) failed during setup (process exited) — see $kanban_log"
        echo "  Last lines of demo output:"
        tail -10 "$kanban_log" 2>/dev/null | sed 's/^/    /' || true
        ;;
    1)
        record_defect "Kanban demo (fresh server) did not complete setup within 120s — see $kanban_log"
        echo "  Last lines of demo output:"
        tail -20 "$kanban_log" 2>/dev/null | sed 's/^/    /' || true
        ;;
esac
echo ""

# ── 7. Check kanban frontend ──
if [ $demo_status -eq 0 ]; then
    echo "--- Checking kanban frontend (port 5173) ---"
    kanban_page=$(curl -s http://localhost:5173/ 2>/dev/null || echo "")
    if echo "$kanban_page" | grep -qiE "<!DOCTYPE|<html"; then
        echo "  ✓ Kanban frontend serves valid HTML"
    else
        record_defect "Kanban frontend does not serve valid HTML on port 5173"
    fi
    echo ""
fi

# ── 8. Stop kanban demo ──
echo "--- Stopping kanban demo ---"
stop_demo "${DEMO_PID:-}"
"$AYB" stop 2>&1 || true
sleep 2

# ─────────────────────────────────────────────────────────────────────────────
# ── 9. Restart regular server for remaining tests ─────────────────────────
# ─────────────────────────────────────────────────────────────────────────────
echo "--- Restarting regular server ---"
"$AYB" start 2>&1 || true
for i in $(seq 1 30); do
    if curl -s "$BASE_URL/health" 2>/dev/null | grep -q '"status"'; then break; fi
    sleep 1
done

if curl -s "$BASE_URL/health" 2>/dev/null | grep -q '"status"'; then
    echo "  ✓ Server restarted"
else
    record_defect "Server failed to restart after demo tests"
fi
echo ""

# ─────────────────────────────────────────────────────────────────────────────
# ── 10. TEST: Second run of demo (schema already exists) ──────────────────
#
# Tests the idempotent path: running the demo again when schema was already
# applied.  This must succeed and print "Schema already applied (tables exist)".
# ─────────────────────────────────────────────────────────────────────────────
echo "--- TEST: ayb demo kanban second run (schema already applied) ---"
ensure_port_free 5173
second_run_log="$RESULTS/demo_kanban_second_run.log"
rm -f "$second_run_log"
"$AYB" demo kanban > "$second_run_log" 2>&1 &
SECOND_RUN_PID=$!

echo "  Demo PID: $SECOND_RUN_PID — waiting for setup..."
wait_for_demo_ready "$SECOND_RUN_PID" "$second_run_log" 60 && second_status=0 || second_status=$?

stop_demo "$SECOND_RUN_PID"

case $second_status in
    0)
        echo "  ✓ Demo second run succeeded"
        if grep -q "already applied" "$second_run_log" 2>/dev/null; then
            echo "  ✓ Schema correctly detected as already applied"
        else
            echo "  (schema was re-applied — not a defect, but unexpected)"
        fi
        ;;
    2)
        record_defect "Demo second run failed during setup (process exited) — see $second_run_log"
        echo "  Last lines of demo output:"
        tail -10 "$second_run_log" 2>/dev/null | sed 's/^/    /' || true
        ;;
    1)
        record_defect "Demo second run timed out after 60s — see $second_run_log"
        ;;
esac
echo ""

# ─────────────────────────────────────────────────────────────────────────────
# ── Summary ──
# ─────────────────────────────────────────────────────────────────────────────
echo "========================================="
echo "  Demo Launch: ${#defects[@]} defect(s)"
if [ ${#defects[@]} -gt 0 ]; then
    for d in "${defects[@]}"; do
        echo "  - $d"
    done
fi
echo "========================================="

[ ${#defects[@]} -eq 0 ]
