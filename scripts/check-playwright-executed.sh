#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 2 ]]; then
  echo "Usage: $0 <playwright-results-json-path> <project-name>" >&2
  exit 1
fi

readonly REPORT_PATH="$1"
readonly PROJECT_NAME="$2"

if [[ ! -f "$REPORT_PATH" ]]; then
  echo "Playwright results file not found: $REPORT_PATH" >&2
  exit 1
fi

node - "$REPORT_PATH" "$PROJECT_NAME" <<'NODE'
const fs = require("fs");

const reportPath = process.argv[2];
const projectName = process.argv[3];

if (!reportPath || !projectName) {
  console.error("Usage: check-playwright-executed.sh <playwright-results-json-path> <project-name>");
  process.exit(1);
}

let report;
try {
  report = JSON.parse(fs.readFileSync(reportPath, "utf8"));
} catch (error) {
  console.error(`Failed to parse Playwright JSON report at ${reportPath}: ${error instanceof Error ? error.message : String(error)}`);
  process.exit(1);
}

let matchedTests = 0;
let executedTests = 0;

function hasExecutedResult(results) {
  if (!Array.isArray(results)) {
    return false;
  }
  return results.some((result) => typeof result?.status === "string" && result.status.toLowerCase() !== "skipped");
}

function visitSuite(suite) {
  if (!suite || typeof suite !== "object") {
    return;
  }

  if (Array.isArray(suite.specs)) {
    for (const spec of suite.specs) {
      if (!spec || typeof spec !== "object" || !Array.isArray(spec.tests)) {
        continue;
      }
      for (const test of spec.tests) {
        if (!test || typeof test !== "object" || test.projectName !== projectName) {
          continue;
        }
        matchedTests += 1;
        if (hasExecutedResult(test.results)) {
          executedTests += 1;
        }
      }
    }
  }

  if (Array.isArray(suite.suites)) {
    for (const nestedSuite of suite.suites) {
      visitSuite(nestedSuite);
    }
  }
}

if (Array.isArray(report.suites)) {
  for (const suite of report.suites) {
    visitSuite(suite);
  }
}

if (executedTests === 0) {
  console.error(`Playwright execution guard failed: no executed tests for project ${projectName} (matched tests: ${matchedTests}).`);
  process.exit(1);
}

console.log(`Playwright execution guard passed: executed tests for project ${projectName}: ${executedTests} (matched tests: ${matchedTests}).`);
NODE
