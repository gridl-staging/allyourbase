#!/bin/bash
set -e

# Run smoke tests on local environment
# Usage: ./run-smoke-local.sh

echo "🧪 Running smoke tests on local environment..."
echo "================================================"

# Load local environment
export $(cat browser-tests-unmocked/config/.env.local | grep -v "^#" | grep -v "^$" | xargs)

# Run smoke tests only
npx playwright test --project=smoke

echo ""
echo "✅ Local smoke tests complete"
