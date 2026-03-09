#!/bin/bash
set -e

# Complete staging→prod validation pipeline
# Usage: ./run-staging-to-prod.sh
#
# Pipeline:
# 1. Run smoke tests on staging
# 2. Run full tests on staging
# 3. Prompt for production deployment
# 4. Run smoke tests on production
#
# Any failure aborts the pipeline

echo "🚀 STAGING → PRODUCTION TEST PIPELINE"
echo "======================================"
echo ""

# Step 1: Staging smoke tests
echo "Step 1/4: Staging smoke tests..."
./browser-tests-unmocked/run-smoke-staging.sh

if [ $? -ne 0 ]; then
  echo ""
  echo "❌ Pipeline FAILED at: Staging smoke tests"
  exit 1
fi

echo ""
echo "--------------------------------------"
echo ""

# Step 2: Staging full tests
echo "Step 2/4: Staging full test suite..."
export $(cat browser-tests-unmocked/config/.env.staging | xargs)
npx playwright test --project=full

if [ $? -ne 0 ]; then
  echo ""
  echo "❌ Pipeline FAILED at: Staging full tests"
  exit 1
fi

echo ""
echo "--------------------------------------"
echo ""
echo "✅ All staging tests PASSED"
echo ""

# Step 3: Prompt for production deployment
read -p "Deploy to PRODUCTION and run validation? (yes/no): " deploy

if [ "$deploy" != "yes" ]; then
  echo "Pipeline stopped. Staging validated, production deployment skipped."
  exit 0
fi

echo ""
echo "--------------------------------------"
echo ""

# Step 4: Production smoke tests
echo "Step 4/4: Production smoke tests..."
export $(cat browser-tests-unmocked/config/.env.prod | xargs)
npx playwright test --project=smoke

if [ $? -ne 0 ]; then
  echo ""
  echo "❌ Pipeline FAILED at: Production smoke tests"
  echo "⚠️  Production deployment may have issues"
  exit 1
fi

echo ""
echo "======================================"
echo "✅ COMPLETE PIPELINE PASSED"
echo "======================================"
echo ""
echo "✅ Staging: smoke + full tests passed"
echo "✅ Production: smoke tests passed"
echo ""
echo "🎉 Safe to proceed with production deployment"
