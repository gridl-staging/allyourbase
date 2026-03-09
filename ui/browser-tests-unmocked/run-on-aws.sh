#!/bin/bash
set -euo pipefail

###############################################################################
# Run Browser (unmocked) Tests on AWS EC2 + Upload Results to S3
#
# Usage: ./run-on-aws.sh [suite]
#   suite: "smoke", "full", or "all" (default: all)
#
# Prerequisites:
#   - AWS CLI configured with credentials
#   - SSH key pair exists in ~/.ssh/
#   - S3 bucket for test results
#
# Environment Variables:
#   AWS_REGION: AWS region (default: us-east-1)
#   AWS_KEY_NAME: EC2 key pair name (default: ayb-e2e-test)
#   S3_BUCKET: S3 bucket for results (default: ayb-e2e-test-results)
#   INSTANCE_TYPE: EC2 instance type (default: t3.medium)
#   AYB_ADMIN_PASSWORD: Admin password for auth tests (default: admin)
#   AYB_STORAGE_ENABLED: Enable storage for storage tests (default: true)
###############################################################################

# Configuration
AWS_REGION="${AWS_REGION:-us-east-1}"
AWS_KEY_NAME="${AWS_KEY_NAME:-ayb-e2e-test}"
S3_BUCKET="${S3_BUCKET:-ayb-e2e-test-results}"
INSTANCE_TYPE="${INSTANCE_TYPE:-t3.medium}"
SUITE="${1:-all}"
TIMESTAMP=$(date +%Y%m%d-%H%M%S)
RUN_ID="e2e-${SUITE}-${TIMESTAMP}"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log() {
  echo -e "${GREEN}[$(date +'%Y-%m-%d %H:%M:%S')]${NC} $*"
}

error() {
  echo -e "${RED}[ERROR]${NC} $*" >&2
}

warn() {
  echo -e "${YELLOW}[WARN]${NC} $*"
}

# Check prerequisites
check_prerequisites() {
  log "Checking prerequisites..."

  if ! command -v aws &>/dev/null; then
    error "AWS CLI not found. Install it first: https://aws.amazon.com/cli/"
    exit 1
  fi

  if ! aws sts get-caller-identity &>/dev/null; then
    error "AWS credentials not configured. Run: aws configure"
    exit 1
  fi

  if [ ! -f ~/.ssh/"${AWS_KEY_NAME}.pem" ]; then
    warn "SSH key not found at ~/.ssh/${AWS_KEY_NAME}.pem"
    warn "Will attempt to create it..."
  fi

  # Validate suite argument
  if [[ "$SUITE" != "smoke" && "$SUITE" != "full" && "$SUITE" != "all" ]]; then
    error "Invalid suite: $SUITE. Must be 'smoke', 'full', or 'all'"
    exit 1
  fi

  log "Suite: $SUITE"
  log "Prerequisites OK"
}

# Create SSH key pair if needed
create_key_pair() {
  if [ -f ~/.ssh/"${AWS_KEY_NAME}.pem" ]; then
    log "Using existing key pair: ${AWS_KEY_NAME}"
    return
  fi

  log "Creating EC2 key pair: ${AWS_KEY_NAME}..."

  aws ec2 create-key-pair \
    --key-name "${AWS_KEY_NAME}" \
    --region "${AWS_REGION}" \
    --query 'KeyMaterial' \
    --output text >~/.ssh/"${AWS_KEY_NAME}.pem"

  chmod 400 ~/.ssh/"${AWS_KEY_NAME}.pem"

  log "Key pair created"
}

# Create S3 bucket if needed
create_s3_bucket() {
  if aws s3 ls "s3://${S3_BUCKET}" &>/dev/null; then
    log "Using existing S3 bucket: ${S3_BUCKET}"
    return
  fi

  log "Creating S3 bucket: ${S3_BUCKET}..."

  if [ "${AWS_REGION}" = "us-east-1" ]; then
    aws s3 mb "s3://${S3_BUCKET}" --region "${AWS_REGION}"
  else
    aws s3 mb "s3://${S3_BUCKET}" --region "${AWS_REGION}" \
      --create-bucket-configuration LocationConstraint="${AWS_REGION}"
  fi

  log "S3 bucket created"
}

# Launch EC2 instance
launch_instance() {
  log "Launching EC2 instance (${INSTANCE_TYPE})..."

  # Get latest Ubuntu 22.04 AMI
  AMI_ID=$(aws ec2 describe-images \
    --region "${AWS_REGION}" \
    --owners 099720109477 \
    --filters "Name=name,Values=ubuntu/images/hvm-ssd/ubuntu-jammy-22.04-amd64-server-*" \
    --query 'Images | sort_by(@, &CreationDate) | [-1].ImageId' \
    --output text)

  log "Using AMI: ${AMI_ID}"

  # Create security group
  SG_ID=$(aws ec2 create-security-group \
    --group-name "ayb-e2e-${RUN_ID}" \
    --description "Temporary SG for AYB E2E tests" \
    --region "${AWS_REGION}" \
    --query 'GroupId' \
    --output text)

  log "Created security group: ${SG_ID}"

  # Allow SSH
  aws ec2 authorize-security-group-ingress \
    --group-id "${SG_ID}" \
    --protocol tcp \
    --port 22 \
    --cidr 0.0.0.0/0 \
    --region "${AWS_REGION}"

  # Allow AYB port (8090)
  aws ec2 authorize-security-group-ingress \
    --group-id "${SG_ID}" \
    --protocol tcp \
    --port 8090 \
    --cidr 0.0.0.0/0 \
    --region "${AWS_REGION}"

  # Launch instance
  INSTANCE_ID=$(aws ec2 run-instances \
    --image-id "${AMI_ID}" \
    --instance-type "${INSTANCE_TYPE}" \
    --key-name "${AWS_KEY_NAME}" \
    --security-group-ids "${SG_ID}" \
    --region "${AWS_REGION}" \
    --tag-specifications "ResourceType=instance,Tags=[{Key=Name,Value=ayb-e2e-${RUN_ID}}]" \
    --query 'Instances[0].InstanceId' \
    --output text)

  log "Launched instance: ${INSTANCE_ID}"
  echo "${INSTANCE_ID}" >/tmp/ayb-e2e-instance-id.txt
  echo "${SG_ID}" >/tmp/ayb-e2e-sg-id.txt

  # Wait for instance to be running
  log "Waiting for instance to be running..."
  aws ec2 wait instance-running \
    --instance-ids "${INSTANCE_ID}" \
    --region "${AWS_REGION}"

  # Get public IP
  PUBLIC_IP=$(aws ec2 describe-instances \
    --instance-ids "${INSTANCE_ID}" \
    --region "${AWS_REGION}" \
    --query 'Reservations[0].Instances[0].PublicIpAddress' \
    --output text)

  log "Instance running at: ${PUBLIC_IP}"
  echo "${PUBLIC_IP}" >/tmp/ayb-e2e-public-ip.txt

  # Wait for SSH to be ready
  log "Waiting for SSH to be ready..."
  for i in {1..30}; do
    if ssh -o StrictHostKeyChecking=no -o ConnectTimeout=5 \
      -i ~/.ssh/"${AWS_KEY_NAME}.pem" \
      ubuntu@"${PUBLIC_IP}" "echo 'SSH ready'" &>/dev/null; then
      break
    fi
    sleep 5
  done

  log "SSH ready"
}

# Create tarball of local source code (includes untracked files)
create_source_tarball() {
  log "Creating tarball of AYB source code..."

  cd "$(dirname "$0")/../.." || exit 1

  # Remove compiled binary if present (macOS binary, useless on Linux, and
  # tar --exclude='./ayb' would also strip cmd/ayb/ source on BSD tar)
  rm -f ./ayb

  # Use tar directly (not git archive) to include untracked files like new E2E tests
  # Exclude node_modules, .git, build artifacts, and other large/irrelevant dirs
  tar --exclude='./node_modules' \
      --exclude='./.git' \
      --exclude='*.tar.gz' \
      --exclude='./playwright-report' \
      --exclude='./test-results' \
      --exclude='./.secret' \
      --exclude='./_dev/session' \
      --exclude='./trash' \
      --exclude='./ui/node_modules' \
      --exclude='./sdk/node_modules' \
      -czf /tmp/ayb-source.tar.gz .

  log "Source tarball created ($(du -h /tmp/ayb-source.tar.gz | cut -f1))"
}

# Setup instance with AYB + dependencies
setup_instance() {
  PUBLIC_IP=$(cat /tmp/ayb-e2e-public-ip.txt)
  log "Setting up instance at ${PUBLIC_IP}..."

  # Upload source tarball
  log "Uploading source code..."
  scp -o StrictHostKeyChecking=no \
    -i ~/.ssh/"${AWS_KEY_NAME}.pem" \
    /tmp/ayb-source.tar.gz \
    ubuntu@"${PUBLIC_IP}":/tmp/ayb-source.tar.gz

  log "Source uploaded"

  ssh -o StrictHostKeyChecking=no \
    -i ~/.ssh/"${AWS_KEY_NAME}.pem" \
    ubuntu@"${PUBLIC_IP}" <<'EOF'
set -euo pipefail

echo "Installing dependencies..."
sudo apt-get clean
sudo rm -rf /var/lib/apt/lists/*
sudo mkdir -p /var/lib/apt/lists/partial
sudo apt-get update -o Acquire::Check-Valid-Until=false -qq || sudo apt-get update -qq
sudo apt-get install -y curl git wget

echo "Installing Node.js 20..."
curl -fsSL https://deb.nodesource.com/setup_20.x | sudo -E bash -
sudo apt-get install -y nodejs

echo "Installing Go 1.25.7..."
wget -q https://go.dev/dl/go1.25.7.linux-amd64.tar.gz
sudo rm -rf /usr/local/go
sudo tar -C /usr/local -xzf go1.25.7.linux-amd64.tar.gz
export PATH=$PATH:/usr/local/go/bin
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc

echo "Extracting AYB source code..."
mkdir -p ~/allyourbase
cd ~/allyourbase
tar -xzf /tmp/ayb-source.tar.gz

echo "Building AYB from source..."
/usr/local/go/bin/go build -ldflags "-s -w" -o ayb ./cmd/ayb
sudo mv ayb /usr/local/bin/ayb
sudo chmod +x /usr/local/bin/ayb

echo "Setup complete"
EOF

  log "Instance setup complete"
}

# Run E2E tests on instance
run_tests() {
  PUBLIC_IP=$(cat /tmp/ayb-e2e-public-ip.txt)
  log "Running Browser (unmocked) tests (suite: ${SUITE})..."

  # Determine which projects to run
  local PROJECTS=""
  case "$SUITE" in
    smoke) PROJECTS="--project=smoke" ;;
    full)  PROJECTS="--project=full" ;;
    all)   PROJECTS="--project=smoke --project=full" ;;
  esac

  local ADMIN_PASSWORD="${AYB_ADMIN_PASSWORD:-admin}"
  local STORAGE_ENABLED="${AYB_STORAGE_ENABLED:-true}"

  ssh -o StrictHostKeyChecking=no \
    -i ~/.ssh/"${AWS_KEY_NAME}.pem" \
    ubuntu@"${PUBLIC_IP}" <<EOF
set -euo pipefail

cd ~/allyourbase/ui

echo "Installing npm dependencies..."
npm install

echo "Installing Playwright system dependencies + browsers..."
sudo npx playwright install-deps chromium
npx playwright install chromium

echo "Creating auth directory for Playwright storage state..."
mkdir -p browser-tests-unmocked/.auth

echo "Starting AYB server..."
export AYB_ADMIN_PASSWORD="${ADMIN_PASSWORD}"
export AYB_STORAGE_ENABLED="${STORAGE_ENABLED}"
export AYB_AUTH_ENABLED=true
export AYB_AUTH_JWT_SECRET="e2e-test-jwt-secret-key-long-$(date +%s)"
nohup /usr/local/bin/ayb start > ~/ayb.log 2>&1 &
AYB_PID=\$!
echo "Started AYB with PID: \${AYB_PID}"

# Wait for server to be ready
echo "Waiting for AYB to be ready..."
for i in {1..60}; do
  if curl -f http://localhost:8090/health &>/dev/null; then
    echo "AYB is ready"
    break
  fi
  if [ \$i -eq 60 ]; then
    echo "AYB failed to start. Log output:"
    tail -n 100 ~/ayb.log
    exit 1
  fi
  sleep 1
done

echo "AYB server log:"
tail -n 10 ~/ayb.log

echo ""
echo "=========================================="
echo "  Running Browser (unmocked) tests: ${SUITE}"
echo "=========================================="
echo ""

export AYB_ADMIN_PASSWORD="${ADMIN_PASSWORD}"
export PLAYWRIGHT_ENV=local

# Run with full reporting — don't fail on test failures (collect results)
npx playwright test ${PROJECTS} \
  --reporter=html --reporter=json --reporter=list \
  2>&1 | tee ~/e2e-test-output.log || true

echo ""
echo "Tests complete"

# Stop AYB server
echo "Stopping AYB server..."
kill \${AYB_PID} || true
EOF

  log "Test execution complete"
}

# Download results from instance
download_results() {
  PUBLIC_IP=$(cat /tmp/ayb-e2e-public-ip.txt)
  log "Downloading test results..."

  mkdir -p /tmp/ayb-e2e-results/${RUN_ID}

  # Download Playwright HTML report
  scp -o StrictHostKeyChecking=no -r \
    -i ~/.ssh/"${AWS_KEY_NAME}.pem" \
    ubuntu@"${PUBLIC_IP}":~/allyourbase/ui/playwright-report \
    /tmp/ayb-e2e-results/${RUN_ID}/ || warn "Failed to download HTML report"

  # Download test-results (screenshots, traces, videos)
  scp -o StrictHostKeyChecking=no -r \
    -i ~/.ssh/"${AWS_KEY_NAME}.pem" \
    ubuntu@"${PUBLIC_IP}":~/allyourbase/ui/test-results \
    /tmp/ayb-e2e-results/${RUN_ID}/ || warn "Failed to download test artifacts"

  # Download test output log
  scp -o StrictHostKeyChecking=no \
    -i ~/.ssh/"${AWS_KEY_NAME}.pem" \
    ubuntu@"${PUBLIC_IP}":~/e2e-test-output.log \
    /tmp/ayb-e2e-results/${RUN_ID}/ || warn "Failed to download test output"

  # Get AYB server logs
  scp -o StrictHostKeyChecking=no \
    -i ~/.ssh/"${AWS_KEY_NAME}.pem" \
    ubuntu@"${PUBLIC_IP}":~/ayb.log \
    /tmp/ayb-e2e-results/${RUN_ID}/ayb-server.log || warn "Failed to get server logs"

  log "Results downloaded to: /tmp/ayb-e2e-results/${RUN_ID}"
}

# Upload results to S3
upload_to_s3() {
  log "Uploading results to S3..."

  aws s3 sync /tmp/ayb-e2e-results/${RUN_ID} \
    "s3://${S3_BUCKET}/${RUN_ID}/" \
    --region "${AWS_REGION}"

  # Generate presigned URL for HTML report (7 days)
  REPORT_URL=$(aws s3 presign \
    "s3://${S3_BUCKET}/${RUN_ID}/playwright-report/index.html" \
    --region "${AWS_REGION}" \
    --expires-in 604800)

  log "Results uploaded to S3"
  echo ""
  echo "=========================================="
  echo "  Browser (unmocked) Test Report"
  echo "=========================================="
  echo "  S3 Path: s3://${S3_BUCKET}/${RUN_ID}/"
  echo "  Report:  ${REPORT_URL}"
  echo "=========================================="
  echo ""
}

# Cleanup resources
cleanup() {
  log "Cleaning up AWS resources..."

  if [ ! -f /tmp/ayb-e2e-instance-id.txt ]; then
    log "No instance to clean up"
    return
  fi

  INSTANCE_ID=$(cat /tmp/ayb-e2e-instance-id.txt)
  SG_ID=$(cat /tmp/ayb-e2e-sg-id.txt)

  # Terminate instance
  aws ec2 terminate-instances \
    --instance-ids "${INSTANCE_ID}" \
    --region "${AWS_REGION}" \
    &>/dev/null || warn "Failed to terminate instance"

  # Wait for termination
  log "Waiting for instance to terminate..."
  aws ec2 wait instance-terminated \
    --instance-ids "${INSTANCE_ID}" \
    --region "${AWS_REGION}" || warn "Failed to wait for termination"

  # Delete security group
  sleep 5
  aws ec2 delete-security-group \
    --group-id "${SG_ID}" \
    --region "${AWS_REGION}" \
    &>/dev/null || warn "Failed to delete security group"

  # Cleanup temp files
  rm -f /tmp/ayb-e2e-instance-id.txt
  rm -f /tmp/ayb-e2e-sg-id.txt
  rm -f /tmp/ayb-e2e-public-ip.txt

  log "Cleanup complete"
}

# Trap to ensure cleanup on exit
trap cleanup EXIT

# Main execution
main() {
  log "Starting AYB Browser (unmocked) Tests on AWS EC2"
  log "Suite: ${SUITE}"
  log "Run ID: ${RUN_ID}"
  echo ""

  check_prerequisites
  create_key_pair
  create_s3_bucket
  create_source_tarball
  launch_instance
  setup_instance
  run_tests
  download_results
  upload_to_s3

  log "Test run complete!"
  log "Run ID: ${RUN_ID}"
}

main "$@"
