#!/usr/bin/env bash
set -euo pipefail

readonly DEFAULT_MATRIX_PATH="_dev/COVERAGE_MATRIX.md"
readonly MATRIX_PATH="${COVERAGE_MATRIX_PATH:-$DEFAULT_MATRIX_PATH}"
readonly DEFAULT_LAYOUT_TYPES_PATH="ui/src/components/layout-types.ts"
readonly LAYOUT_TYPES_PATH="${COVERAGE_MATRIX_LAYOUT_TYPES_PATH:-$DEFAULT_LAYOUT_TYPES_PATH}"

if [[ ! -f "$MATRIX_PATH" ]]; then
  echo "Coverage matrix not found: $MATRIX_PATH" >&2
  exit 1
fi

if [[ ! -f "$LAYOUT_TYPES_PATH" ]]; then
  echo "Layout types source not found: $LAYOUT_TYPES_PATH" >&2
  exit 1
fi

# TODO: Document extract_metric_count.
extract_metric_count() {
  local metric_name="$1"

  awk -F'|' -v metric="$metric_name" '
    function trim(value) {
      gsub(/^[[:space:]]+|[[:space:]]+$/, "", value)
      return value
    }

    /^##[[:space:]]+Gap Summary/ {
      in_gap_summary = 1
      next
    }

    in_gap_summary && /^##[[:space:]]+/ {
      in_gap_summary = 0
    }

    in_gap_summary && /^\|/ {
      left = trim($2)
      right = trim($3)

      if (left == metric) {
        if (match(right, /[0-9]+/)) {
          print substr(right, RSTART, RLENGTH)
          exit
        }
      }
    }
  ' "$MATRIX_PATH"
}

assert_integer() {
  local label="$1"
  local value="$2"

  if [[ -z "$value" || ! "$value" =~ ^[0-9]+$ ]]; then
    echo "Unable to parse metric '$label' from $MATRIX_PATH" >&2
    exit 1
  fi
}

smoke_none_count="$(extract_metric_count "Smoke = none")"
smoke_heading_only_count="$(extract_metric_count "Smoke = heading-only")"
crud_missing_full_count="$(extract_metric_count "CRUD-capable views missing full lifecycle")"
mocked_coverage_missing_count="$(extract_metric_count "Views missing mocked coverage")"

assert_integer "Smoke = none" "$smoke_none_count"
assert_integer "Smoke = heading-only" "$smoke_heading_only_count"
assert_integer "CRUD-capable views missing full lifecycle" "$crud_missing_full_count"
assert_integer "Views missing mocked coverage" "$mocked_coverage_missing_count"

node - "$MATRIX_PATH" "$LAYOUT_TYPES_PATH" <<'NODE'
const fs = require("fs");

const matrixPath = process.argv[2];
const layoutTypesPath = process.argv[3];

const layoutSource = fs.readFileSync(layoutTypesPath, "utf8");
const matrixSource = fs.readFileSync(matrixPath, "utf8");

function parseStringLiteralArray(constName) {
  const pattern = new RegExp(
    `const\\s+${constName}\\s*=\\s*\\[([\\s\\S]*?)\\]\\s+as\\s+const`,
  );
  const match = layoutSource.match(pattern);
  if (!match) {
    return [];
  }

  return [...match[1].matchAll(/"([^"]+)"/g)].map((entry) => entry[1]);
}

function parseStringLiteralUnion(typeName) {
  const pattern = new RegExp(`type\\s+${typeName}\\s*=\\s*([\\s\\S]*?);`);
  const match = layoutSource.match(pattern);
  if (!match) {
    return [];
  }

  return [...match[1].matchAll(/"([^"]+)"/g)].map((entry) => entry[1]);
}

const uniqueViews = [
  ...new Set([
    ...(
      parseStringLiteralArray("DATA_VIEWS").length > 0
        ? parseStringLiteralArray("DATA_VIEWS")
        : parseStringLiteralUnion("DataView")
    ),
    ...parseStringLiteralArray("ADMIN_VIEWS"),
  ]),
];

if (uniqueViews.length === 0) {
  console.error(`Unable to parse view inventory from ${layoutTypesPath}`);
  process.exit(1);
}

const matrixSection = matrixSource.split("## Gap Summary")[0];
const matrixViews = [...matrixSection.matchAll(/\|\s*`([^`]+)`\s*\|/g)].map(
  (match) => match[1],
);
const uniqueMatrixViews = [...new Set(matrixViews)];

const missingViews = uniqueViews.filter(
  (view) => !uniqueMatrixViews.includes(view),
);
const extraViews = uniqueMatrixViews.filter((view) => !uniqueViews.includes(view));

if (missingViews.length > 0 || extraViews.length > 0) {
  if (missingViews.length > 0) {
    console.error(
      `Coverage matrix missing views from ${layoutTypesPath}: ${missingViews.join(", ")}`,
    );
  }
  if (extraViews.length > 0) {
    console.error(
      `Coverage matrix has unknown views not present in ${layoutTypesPath}: ${extraViews.join(", ")}`,
    );
  }
  process.exit(1);
}

console.log(
  `Coverage matrix view inventory matches ${layoutTypesPath}: ${uniqueViews.length} views.`,
);
NODE

echo "Coverage matrix summary from $MATRIX_PATH"
echo "Smoke = none: $smoke_none_count"
echo "Smoke = heading-only: $smoke_heading_only_count"
echo "CRUD-capable views missing full lifecycle: $crud_missing_full_count"
echo "Views missing mocked coverage: $mocked_coverage_missing_count"

if (( smoke_none_count > 0 || smoke_heading_only_count > 0 || crud_missing_full_count > 0 )); then
  echo "Coverage matrix gate failed: browser coverage gaps remain." >&2
  exit 1
fi

echo "Coverage matrix gate passed."
