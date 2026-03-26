package codehealth

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

const checkCoverageMatrixScript = "scripts/check-coverage-matrix.sh"
const checkPlaywrightExecutedScript = "scripts/check-playwright-executed.sh"
const coverageMatrixLayoutTypesPath = "ui/src/components/layout-types.ts"

var viewLiteralPattern = regexp.MustCompile(`"([^"]+)"`)

func TestCheckCoverageMatrixScriptPassesForZeroGateCounts(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	matrixPath := filepath.Join(t.TempDir(), "COVERAGE_MATRIX.md")
	writeTextFile(t, matrixPath, gapSummaryMarkdown(t, repoRoot, 0, 0, 0, 25))

	cmd := exec.Command("bash", checkCoverageMatrixScript)
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(), "COVERAGE_MATRIX_PATH="+matrixPath)

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("expected script success, got error: %v output=%s", err, output)
	}
	text := string(output)
	if !strings.Contains(text, "Smoke = none: 0") {
		t.Fatalf("expected smoke-none metric in output, got: %s", text)
	}
	if !strings.Contains(text, "Views missing mocked coverage: 25") {
		t.Fatalf("expected mocked-gap metric in output, got: %s", text)
	}
}

func TestCheckCoverageMatrixScriptFailsForNonZeroGateCount(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	matrixPath := filepath.Join(t.TempDir(), "COVERAGE_MATRIX.md")
	writeTextFile(t, matrixPath, gapSummaryMarkdown(t, repoRoot, 0, 1, 0, 25))

	cmd := exec.Command("bash", checkCoverageMatrixScript)
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(), "COVERAGE_MATRIX_PATH="+matrixPath)

	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected script failure, got success: %s", output)
	}
	if !strings.Contains(string(output), "Smoke = heading-only") {
		t.Fatalf("expected failure output to mention non-zero heading-only gate, got: %s", output)
	}
}

func TestCheckPlaywrightExecutedScriptPassesWhenProjectExecutedTestsExist(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	reportPath := filepath.Join(t.TempDir(), "results.json")
	writeTextFile(t, reportPath, playwrightResultsJSON(`
{
  "suites": [
    {
      "specs": [
        {
          "tests": [
            {
              "projectName": "setup",
              "results": [{ "status": "passed" }]
            },
            {
              "projectName": "smoke",
              "results": [{ "status": "passed" }]
            }
          ]
        }
      ]
    }
  ]
}`))

	cmd := exec.Command("bash", checkPlaywrightExecutedScript, reportPath, "smoke")
	cmd.Dir = repoRoot

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("expected script success, got error: %v output=%s", err, output)
	}
	if !strings.Contains(string(output), "executed tests for project smoke: 1") {
		t.Fatalf("expected executed count output, got: %s", output)
	}
}

func TestCheckPlaywrightExecutedScriptFailsWhenProjectHasOnlySkippedTests(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	reportPath := filepath.Join(t.TempDir(), "results.json")
	writeTextFile(t, reportPath, playwrightResultsJSON(`
{
  "suites": [
    {
      "specs": [
        {
          "tests": [
            {
              "projectName": "setup",
              "results": [{ "status": "passed" }]
            },
            {
              "projectName": "smoke",
              "results": [{ "status": "skipped" }]
            }
          ]
        }
      ]
    }
  ]
}`))

	cmd := exec.Command("bash", checkPlaywrightExecutedScript, reportPath, "smoke")
	cmd.Dir = repoRoot

	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected script failure, got success: %s", output)
	}
	if !strings.Contains(string(output), "no executed tests for project smoke") {
		t.Fatalf("expected failure reason for skipped-only project, got: %s", output)
	}
}

func gapSummaryMarkdown(t *testing.T, repoRoot string, smokeNone, smokeHeadingOnly, crudMissingFull, mockedMissing int) string {
	t.Helper()

	views := coverageMatrixViews(t, repoRoot)
	totalViews := len(views)
	smokeContentVerified := totalViews - smokeNone - smokeHeadingOnly
	if smokeContentVerified < 0 {
		t.Fatalf("invalid smoke counts: none=%d heading-only=%d total=%d", smokeNone, smokeHeadingOnly, totalViews)
	}

	var matrix strings.Builder
	matrix.WriteString("# Browser Test Coverage Matrix\n\n")
	matrix.WriteString("## Coverage Matrix\n\n")
	matrix.WriteString("| View | Smoke | Full Lifecycle | CRUD-capable | Mocked | Evidence Specs |\n")
	matrix.WriteString("|---|---|---|---|---|---|\n")
	for _, view := range views {
		matrix.WriteString("| `" + view + "` | content-verified | exists | yes | not | fixture |\n")
	}
	matrix.WriteString("\n## Gap Summary\n\n")
	matrix.WriteString("| Metric | Count |\n")
	matrix.WriteString("|---|---|\n")
	matrix.WriteString("| Total views | " + intToString(totalViews) + " |\n")
	matrix.WriteString("| Smoke = none | " + intToString(smokeNone) + " |\n")
	matrix.WriteString("| Smoke = heading-only | " + intToString(smokeHeadingOnly) + " |\n")
	matrix.WriteString("| Smoke = content-verified | " + intToString(smokeContentVerified) + " |\n")
	matrix.WriteString("| All views with smoke coverage | " + intToString(totalViews) + "/" + intToString(totalViews) + " (100%) |\n")
	matrix.WriteString("| Views with full lifecycle specs | " + intToString(totalViews-crudMissingFull) + " |\n")
	matrix.WriteString("| CRUD-capable views missing full lifecycle | " + intToString(crudMissingFull) + " |\n")
	matrix.WriteString("| Views missing mocked coverage | " + intToString(mockedMissing) + " |\n")
	return matrix.String()
}

func coverageMatrixViews(t *testing.T, repoRoot string) []string {
	t.Helper()

	source, err := os.ReadFile(filepath.Join(repoRoot, coverageMatrixLayoutTypesPath))
	if err != nil {
		t.Fatalf("read layout types: %v", err)
	}

	layoutSource := string(source)
	return append(
		parseStringLiteralArrayOrUnion(t, layoutSource, "DATA_VIEWS", "DataView"),
		parseStringLiteralArray(t, layoutSource, "ADMIN_VIEWS")...,
	)
}

func parseStringLiteralArrayOrUnion(t *testing.T, layoutSource, constName, typeName string) []string {
	t.Helper()

	if values := parseStringLiteralArrayMatch(layoutSource, constName); len(values) > 0 {
		return values
	}
	if values := parseStringLiteralUnionMatch(layoutSource, typeName); len(values) > 0 {
		return values
	}

	t.Fatalf("parse %s or %s from layout types", constName, typeName)
	return nil
}

func parseStringLiteralArray(t *testing.T, layoutSource, constName string) []string {
	t.Helper()

	values := parseStringLiteralArrayMatch(layoutSource, constName)
	if len(values) == 0 {
		t.Fatalf("parse %s from layout types", constName)
	}
	return values
}

func parseStringLiteralArrayMatch(layoutSource, constName string) []string {
	blockPattern := regexp.MustCompile(`(?s)const\s+` + regexp.QuoteMeta(constName) + `\s*=\s*\[(.*?)\]\s+as\s+const`)
	match := blockPattern.FindStringSubmatch(layoutSource)
	if match == nil {
		return nil
	}
	return parseStringLiteralMatches(match[1])
}

func parseStringLiteralUnionMatch(layoutSource, typeName string) []string {
	blockPattern := regexp.MustCompile(`(?s)type\s+` + regexp.QuoteMeta(typeName) + `\s*=\s*(.*?);`)
	match := blockPattern.FindStringSubmatch(layoutSource)
	if match == nil {
		return nil
	}
	return parseStringLiteralMatches(match[1])
}

func parseStringLiteralMatches(source string) []string {
	matches := viewLiteralPattern.FindAllStringSubmatch(source, -1)
	if len(matches) == 0 {
		return nil
	}

	values := make([]string, 0, len(matches))
	for _, entry := range matches {
		values = append(values, entry[1])
	}
	return values
}

func playwrightResultsJSON(content string) string {
	return strings.TrimSpace(content) + "\n"
}

func intToString(value int) string {
	return strconv.Itoa(value)
}
