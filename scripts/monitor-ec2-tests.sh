#!/bin/bash
# Monitor EC2 E2E test progress via S3 streaming output
# Usage: ./scripts/monitor-ec2-tests.sh [instance-id]

set -euo pipefail

INSTANCE_ID="${1:-}"
BUCKET="ayb-ci-artifacts"
LAST_SIZE=0

echo "=== AYB E2E Test Monitor ==="

# Find the latest run
find_latest_run() {
    aws s3 ls "s3://${BUCKET}/e2e-runs/" 2>/dev/null | sort | tail -1 | awk '{print $2}' | tr -d '/'
}

RUN_ID=$(find_latest_run)
if [ -z "$RUN_ID" ]; then
    echo "No test runs found in s3://${BUCKET}/e2e-runs/"
    echo "Waiting for first upload..."
    while [ -z "$RUN_ID" ]; do
        sleep 10
        RUN_ID=$(find_latest_run)
    done
fi

S3_LOG="s3://${BUCKET}/e2e-runs/${RUN_ID}/output.log"
echo "Run ID: ${RUN_ID}"
echo "Log:    ${S3_LOG}"
if [ -n "$INSTANCE_ID" ]; then
    echo "Instance: ${INSTANCE_ID}"
fi
echo "---"
echo ""

while true; do
    # Download current log
    if aws s3 cp "$S3_LOG" /tmp/e2e-monitor.log --quiet 2>/dev/null; then
        CUR_SIZE=$(wc -c < /tmp/e2e-monitor.log)
        if [ "$CUR_SIZE" -gt "$LAST_SIZE" ]; then
            # Print only new content
            tail -c +$((LAST_SIZE + 1)) /tmp/e2e-monitor.log
            LAST_SIZE=$CUR_SIZE
        fi

        # Check if tests completed
        if grep -q "=== Test run complete" /tmp/e2e-monitor.log 2>/dev/null; then
            echo ""
            echo "=========================================="
            echo "  Tests Complete!"
            echo "=========================================="
            echo ""
            echo "Full log: ${S3_LOG}"
            echo "Summary:  s3://${BUCKET}/e2e-runs/${RUN_ID}/summary.json"
            echo ""

            # Print summary if available
            if aws s3 cp "s3://${BUCKET}/e2e-runs/${RUN_ID}/summary.json" /tmp/e2e-summary.json --quiet 2>/dev/null; then
                echo "Summary:"
                cat /tmp/e2e-summary.json
            fi

            # Show pass/fail counts
            echo ""
            echo "--- Results ---"
            grep -E "(PASSED|FAILED|ok |FAIL)" /tmp/e2e-monitor.log | tail -30
            break
        fi
    fi

    # Check instance state if ID provided
    if [ -n "$INSTANCE_ID" ]; then
        STATE=$(aws ec2 describe-instances \
            --instance-ids "$INSTANCE_ID" \
            --query 'Reservations[0].Instances[0].State.Name' \
            --output text 2>/dev/null || echo "unknown")
        if [ "$STATE" = "terminated" ] || [ "$STATE" = "shutting-down" ]; then
            echo ""
            echo "Instance ${INSTANCE_ID} is ${STATE}"
            if [ "$LAST_SIZE" -eq 0 ]; then
                echo "No output was captured. Check console output:"
                echo "  aws ec2 get-console-output --instance-id ${INSTANCE_ID}"
            fi
            break
        fi
    fi

    sleep 15
done
