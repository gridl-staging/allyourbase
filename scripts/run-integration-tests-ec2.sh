#!/bin/bash
# EC2 user-data script to run AYB integration + E2E tests
# Auto-shuts down after completion or 2 hours
# Streams progress to S3 so results survive crashes

set -euo pipefail

BUCKET="ayb-ci-artifacts"
RUN_ID=$(date +%Y%m%d-%H%M%S)
S3_PREFIX="s3://${BUCKET}/e2e-runs/${RUN_ID}"
LOG="/tmp/test-output.log"

log() { echo "[$(date +%H:%M:%S)] $*" | tee -a "$LOG"; }
upload_log() { aws s3 cp "$LOG" "${S3_PREFIX}/output.log" --quiet 2>/dev/null || true; }

# Upload log to S3 every 30 seconds in background
( while true; do sleep 30; upload_log; done ) &
LOG_UPLOADER_PID=$!

# Schedule shutdown in 2 hours as failsafe
( sleep 7200 && log "FAILSAFE: 2-hour timeout reached, shutting down" && upload_log && shutdown -h now ) &

# On exit: upload final log and shut down
cleanup() {
    log "=== CLEANUP: uploading final results ==="
    upload_log
    kill $LOG_UPLOADER_PID 2>/dev/null || true
    shutdown -h now
}
trap cleanup EXIT

log "=== AYB E2E Test Run: ${RUN_ID} ==="
log "=== S3 results: ${S3_PREFIX}/ ==="

# -------------------------------------------------------
# 1. Install dependencies
# -------------------------------------------------------
log "--- Installing system dependencies ---"
export DEBIAN_FRONTEND=noninteractive
apt-get update -qq 2>&1 | tail -1 | tee -a "$LOG"
apt-get install -y -qq docker.io git make unzip curl wget gcc libc6-dev 2>&1 | tail -3 | tee -a "$LOG"

# Install Go 1.24 (from official tarball, not distro package)
log "--- Installing Go 1.24 ---"
wget -q https://go.dev/dl/go1.24.0.linux-amd64.tar.gz -O /tmp/go.tar.gz
tar -C /usr/local -xzf /tmp/go.tar.gz
export PATH=/usr/local/go/bin:$PATH
export GOPATH=/root/go
export GOMODCACHE=/root/go/pkg/mod
export GOCACHE=/root/go/cache
export HOME=/root
export CGO_ENABLED=1
mkdir -p $GOPATH $GOMODCACHE $GOCACHE
log "Go version: $(go version)"

# Install AWS CLI
log "--- Installing AWS CLI ---"
curl -sS "https://awscli.amazonaws.com/awscli-exe-linux-x86_64.zip" -o "/tmp/awscliv2.zip"
cd /tmp && unzip -qq awscliv2.zip && ./aws/install 2>&1 | tail -1 | tee -a "$LOG"

# Start Docker
log "--- Starting Docker ---"
systemctl start docker
systemctl enable docker
docker info 2>&1 | grep "Server Version" | tee -a "$LOG"

upload_log

# -------------------------------------------------------
# 2. Download and extract source
# -------------------------------------------------------
log "--- Downloading source from S3 ---"
cd /tmp
aws s3 cp s3://${BUCKET}/source.tar.gz . 2>&1 | tee -a "$LOG"
mkdir -p ayb && cd ayb
tar -xzf ../source.tar.gz
log "Source extracted: $(ls | head -20)"

# Create ui/dist with proper index.html (needed for SPA fallback test)
mkdir -p ui/dist/assets
cat > ui/dist/index.html << 'UIEOF'
<!DOCTYPE html>
<html lang="en">
<head><meta charset="UTF-8"><title>AYB Admin</title></head>
<body><div id="root"></div><script type="module" src="/assets/index.js"></script></body>
</html>
UIEOF
echo "// placeholder" > ui/dist/assets/index.js

# Create .ayb directory (needed for CLI PID tests)
mkdir -p /root/.ayb

upload_log

# -------------------------------------------------------
# 3. Run unit tests first (fast sanity check)
# -------------------------------------------------------
log ""
log "=========================================="
log "  PHASE 1: Unit Tests (sanity check)"
log "=========================================="
if go test -count=1 ./... 2>&1 | tee -a "$LOG"; then
    log "UNIT TESTS: PASSED"
else
    log "UNIT TESTS: FAILED"
fi
upload_log

# -------------------------------------------------------
# 4. Run integration tests (existing, with Docker Postgres)
# -------------------------------------------------------
log ""
log "=========================================="
log "  PHASE 2: Integration Tests (Docker PG)"
log "=========================================="

# Start Postgres container
PG_CID=$(docker run -d --rm \
    -e POSTGRES_USER=test -e POSTGRES_PASSWORD=test -e POSTGRES_DB=testdb \
    -p 0:5432 postgres:16-alpine)
PG_PORT=$(docker port $PG_CID 5432/tcp | cut -d: -f2)
log "Postgres container: ${PG_CID:0:12} on port ${PG_PORT}"

log "Waiting for Postgres..."
until docker exec $PG_CID pg_isready -U test -q 2>/dev/null; do sleep 0.2; done
log "Postgres ready"

export TEST_DATABASE_URL="postgresql://test:test@localhost:${PG_PORT}/testdb?sslmode=disable"

if go test -tags=integration -count=1 -v ./... 2>&1 | tee -a "$LOG"; then
    log "INTEGRATION TESTS: PASSED"
else
    log "INTEGRATION TESTS: FAILED (exit code: $?)"
fi

# Stop Postgres
docker stop $PG_CID >/dev/null 2>&1 || true

upload_log

# -------------------------------------------------------
# 5. Upload detailed results
# -------------------------------------------------------
log ""
log "=========================================="
log "  COMPLETE"
log "=========================================="
log "Results uploaded to: ${S3_PREFIX}/output.log"

# Create a summary file
cat > /tmp/summary.json <<SUMEOF
{
  "runId": "${RUN_ID}",
  "goVersion": "$(go version)",
  "timestamp": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "s3Prefix": "${S3_PREFIX}",
  "logFile": "${S3_PREFIX}/output.log"
}
SUMEOF
aws s3 cp /tmp/summary.json "${S3_PREFIX}/summary.json" --quiet

log "=== Test run complete. Instance will shut down. ==="
