#!/bin/bash
set -euo pipefail

###############################################################################
# Run E2E Test Locally
#
# Usage: ./run-local.sh [test-name]
#   test-name: Name of the spec file (default: blog-platform-journey)
#
# This script:
# 1. Builds and starts AYB server
# 2. Runs the E2E test
# 3. Captures results
# 4. Stops the server
###############################################################################

TEST_NAME="${1:-blog-platform-journey}"
TIMESTAMP=$(date +%Y%m%d-%H%M%S)
RESULTS_DIR="/tmp/ayb-e2e-local-${TIMESTAMP}"

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m'

log() {
  echo -e "${GREEN}[$(date +'%H:%M:%S')]${NC} $*"
}

error() {
  echo -e "${RED}[ERROR]${NC} $*" >&2
}

# Find project root
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

log "🚀 Starting local E2E test: ${TEST_NAME}"
log "Project root: ${PROJECT_ROOT}"

# Check if AYB is already running
if curl -f http://localhost:8090/health &>/dev/null; then
  log "⚠️  AYB is already running at localhost:8090"
  log "Will use existing instance (or stop it with: ayb stop)"
  STARTED_SERVER=false
else
  log "📦 Building AYB..."
  cd "${PROJECT_ROOT}"
  make build

  log "🚀 Starting AYB server..."
  ./ayb start >/tmp/ayb-server-${TIMESTAMP}.log 2>&1 &
  AYB_PID=$!
  echo $AYB_PID >/tmp/ayb-server-${TIMESTAMP}.pid

  # Wait for server to be ready
  log "⏳ Waiting for AYB to be ready..."
  for i in {1..30}; do
    if curl -f http://localhost:8090/health &>/dev/null; then
      log "✓ AYB is ready (PID: $AYB_PID)"
      break
    fi
    if [ "$i" -eq 30 ]; then
      error "Timeout waiting for AYB to start"
      kill $AYB_PID 2>/dev/null || true
      exit 1
    fi
    sleep 1
  done
  STARTED_SERVER=true
fi

# Install npm dependencies if needed
cd "${PROJECT_ROOT}/ui"
if [ ! -d "node_modules/@playwright" ]; then
  log "📦 Installing Playwright..."
  npm install
fi

# Run the test
log "🧪 Running E2E test..."
mkdir -p "${RESULTS_DIR}"

if npx playwright test "browser-tests-unmocked/${TEST_NAME}.spec.ts" --reporter=html --reporter=json 2>&1 | tee "${RESULTS_DIR}/test-output.log"; then
  log "✅ Test passed!"
  TEST_PASSED=true
else
  error "❌ Test failed!"
  TEST_PASSED=false
fi

# Copy results
if [ -d "playwright-report" ]; then
  cp -r playwright-report "${RESULTS_DIR}/"
  log "📊 HTML report: ${RESULTS_DIR}/playwright-report/index.html"
fi

if [ -f "test-results.json" ]; then
  cp test-results.json "${RESULTS_DIR}/"
fi

if [ -d "test-results" ]; then
  cp -r test-results "${RESULTS_DIR}/"
fi

# Get server logs
if "${STARTED_SERVER}"; then
  log "📝 Capturing server logs..."
  if [ -f "/tmp/ayb-server-${TIMESTAMP}.log" ]; then
    cp "/tmp/ayb-server-${TIMESTAMP}.log" "${RESULTS_DIR}/ayb-server.log"
  fi

  log "🛑 Stopping AYB server..."
  if [ -f "/tmp/ayb-server-${TIMESTAMP}.pid" ]; then
    AYB_PID=$(cat "/tmp/ayb-server-${TIMESTAMP}.pid")
    kill $AYB_PID 2>/dev/null || true
    rm -f "/tmp/ayb-server-${TIMESTAMP}.pid"
  fi

  # Clean up log file
  rm -f "/tmp/ayb-server-${TIMESTAMP}.log"
fi

log "📁 Results saved to: ${RESULTS_DIR}"

if "${TEST_PASSED}"; then
  echo ""
  log "✅ Test run complete!"
  exit 0
else
  echo ""
  error "❌ Test failed. Check results at: ${RESULTS_DIR}"
  exit 1
fi
