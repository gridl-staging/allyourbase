#!/bin/bash
# Launch EC2 instance to run AYB integration + E2E tests
# Usage: ./scripts/launch-ec2-tests.sh
#
# Results stream to: s3://ayb-ci-artifacts/e2e-runs/<timestamp>/output.log
# Instance auto-terminates after tests complete or 2 hours

set -euo pipefail

INSTANCE_TYPE="t3.medium"  # 2 vCPU, 4GB RAM — enough for Docker + Go tests
AMI_ID="ami-0c7217cdde317cfec"  # Ubuntu 22.04 LTS (us-east-1)
REGION=$(aws configure get region 2>/dev/null || echo "us-east-1")
BUCKET="ayb-ci-artifacts"
KEY_NAME="jan2026"

echo "=== AYB E2E Test Launcher ==="
echo ""

# Create source tarball (exclude large/unnecessary dirs)
echo "[1/4] Creating source tarball..."
cd "$(git rev-parse --show-toplevel)"
tar -czf /tmp/ayb-source.tar.gz \
  --exclude='.git' \
  --exclude='node_modules' \
  --exclude='ui/dist' \
  --exclude='ui/node_modules' \
  --exclude='ayb_data' \
  --exclude='ayb_storage' \
  --exclude='examples/shareborough/node_modules' \
  --exclude='examples/shareborough/dist' \
  --exclude='docs-site' \
  --exclude='*.tar.gz' \
  .
SIZE=$(du -h /tmp/ayb-source.tar.gz | cut -f1)
echo "   Tarball: ${SIZE}"

# Upload to S3
echo "[2/4] Uploading source to S3..."
aws s3 cp /tmp/ayb-source.tar.gz "s3://${BUCKET}/source.tar.gz" --quiet
echo "   Uploaded to s3://${BUCKET}/source.tar.gz"

# Launch instance
echo "[3/4] Launching EC2 instance..."
INSTANCE_ID=$(aws ec2 run-instances \
  --region "${REGION}" \
  --image-id "${AMI_ID}" \
  --instance-type "${INSTANCE_TYPE}" \
  --key-name "${KEY_NAME}" \
  --security-group-ids sg-089784677dc281760 \
  --user-data file://scripts/run-integration-tests-ec2.sh \
  --iam-instance-profile Name=EC2-S3-Access \
  --associate-public-ip-address \
  --tag-specifications "ResourceType=instance,Tags=[{Key=Name,Value=ayb-e2e-${INSTANCE_TYPE}},{Key=Purpose,Value=e2e-tests},{Key=AutoShutdown,Value=true}]" \
  --instance-initiated-shutdown-behavior terminate \
  --query 'Instances[0].InstanceId' \
  --output text)

echo "   Instance: ${INSTANCE_ID}"

# Wait for public IP
echo "[4/4] Waiting for public IP..."
sleep 5
PUBLIC_IP=$(aws ec2 describe-instances \
  --instance-ids "${INSTANCE_ID}" \
  --query 'Reservations[0].Instances[0].PublicIpAddress' \
  --output text 2>/dev/null || echo "pending")

echo ""
echo "=========================================="
echo "  EC2 E2E Test Run Launched"
echo "=========================================="
echo ""
echo "  Instance:    ${INSTANCE_ID}"
echo "  Type:        ${INSTANCE_TYPE}"
echo "  Public IP:   ${PUBLIC_IP}"
echo "  Region:      ${REGION}"
echo ""
echo "  S3 Results:  s3://${BUCKET}/e2e-runs/"
echo "  Auto-shutdown: 2 hours or on completion"
echo "  Shutdown behavior: terminate (will be deleted)"
echo ""
echo "--- Monitor ---"
echo ""
echo "  # Watch live output (updates every 30s):"
echo "  aws s3 cp s3://${BUCKET}/e2e-runs/\$(aws s3 ls s3://${BUCKET}/e2e-runs/ | tail -1 | awk '{print \$2}')output.log -"
echo ""
echo "  # Or use the monitor script:"
echo "  ./scripts/monitor-ec2-tests.sh ${INSTANCE_ID}"
echo ""
echo "  # SSH in (if needed):"
echo "  ssh -i ~/.ssh/${KEY_NAME}.pem ubuntu@${PUBLIC_IP}"
echo ""
echo "  # Manual terminate:"
echo "  aws ec2 terminate-instances --instance-ids ${INSTANCE_ID}"
echo ""
