//go:build integration

package cli

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// queryListResponse is the parsed JSON response from ayb query --json.
type queryListResponse struct {
	Items      []map[string]any `json:"items"`
	Page       int              `json:"page"`
	PerPage    int              `json:"perPage"`
	TotalItems int              `json:"totalItems"`
	TotalPages int              `json:"totalPages"`
}

// parseQueryJSON unmarshals the JSON output from ayb query --json.
func parseQueryJSON(t *testing.T, stdout string) queryListResponse {
	t.Helper()
	var resp queryListResponse
	if err := json.Unmarshal([]byte(stdout), &resp); err != nil {
		t.Fatalf("failed to parse query JSON: %v\nstdout: %s", err, stdout)
	}
	return resp
}

// seedQueryRows creates a table with (id serial, title text, score int),
// inserts n rows with predictable values (title="item_1", score=10, etc.),
// and registers DROP TABLE cleanup. Returns the table name.
func seedQueryRows(t *testing.T, n int) string {
	t.Helper()
	table := uniqueTableName(t, "s3")
	dropTableCleanup(t, table)

	ddl := fmt.Sprintf("CREATE TABLE %s (id serial PRIMARY KEY, title text NOT NULL, score integer NOT NULL)", table)
	_, stderr, exitCode := runCLIE2E(t, "sql", ddl)
	if exitCode != 0 {
		t.Fatalf("CREATE TABLE failed (exit %d): %s", exitCode, stderr)
	}

	for i := 1; i <= n; i++ {
		insert := fmt.Sprintf("INSERT INTO %s (title, score) VALUES ('item_%d', %d)", table, i, i*10)
		_, stderr, exitCode := runCLIE2E(t, "sql", insert)
		if exitCode != 0 {
			t.Fatalf("INSERT row %d failed (exit %d): %s", i, exitCode, stderr)
		}
	}
	return table
}

// ---------------------------------------------------------------------------
// ayb query tests — basic round-trip
// ---------------------------------------------------------------------------

func TestCLI_E2E_Query_BasicList(t *testing.T) {
	table := seedQueryRows(t, 3)

	stdout, stderr, exitCode := runCLIE2E(t, "query", table, "--json", "--sort", "id")
	if exitCode != 0 {
		t.Fatalf("query failed (exit %d): %s", exitCode, stderr)
	}

	resp := parseQueryJSON(t, stdout)
	if len(resp.Items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(resp.Items))
	}

	for i, item := range resp.Items {
		expectedTitle := fmt.Sprintf("item_%d", i+1)
		expectedScore := float64((i + 1) * 10)
		if title, ok := item["title"].(string); !ok || title != expectedTitle {
			t.Errorf("item %d: expected title %q, got %v", i, expectedTitle, item["title"])
		}
		if score, ok := item["score"].(float64); !ok || score != expectedScore {
			t.Errorf("item %d: expected score %v, got %v", i, expectedScore, item["score"])
		}
	}
}

func TestCLI_E2E_Query_TableOutput(t *testing.T) {
	table := seedQueryRows(t, 2)

	stdout, stderr, exitCode := runCLIE2E(t, "query", table, "--fields", "id,title,score", "--sort", "id")
	if exitCode != 0 {
		t.Fatalf("query failed (exit %d): %s", exitCode, stderr)
	}

	rawLines := strings.Split(strings.TrimSpace(stdout), "\n")
	lines := make([]string, 0, len(rawLines))
	for _, line := range rawLines {
		if strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
	}
	if len(lines) != 5 {
		t.Fatalf("expected 5 non-empty table output lines, got %d:\n%s", len(lines), stdout)
	}
	if got := strings.Fields(lines[0]); strings.Join(got, ",") != "id,title,score" {
		t.Fatalf("expected header row id/title/score, got fields %q from line %q", got, lines[0])
	}
	if got := strings.Fields(lines[1]); strings.Join(got, ",") != "---,---,---" {
		t.Fatalf("expected separator row with 3 columns, got fields %q from line %q", got, lines[1])
	}
	if got := strings.Fields(lines[2]); strings.Join(got, ",") != "1,item_1,10" {
		t.Fatalf("expected first data row 1/item_1/10, got fields %q from line %q", got, lines[2])
	}
	if got := strings.Fields(lines[3]); strings.Join(got, ",") != "2,item_2,20" {
		t.Fatalf("expected second data row 2/item_2/20, got fields %q from line %q", got, lines[3])
	}
	if lines[4] != "Page 1/1 (2 total records)" {
		t.Fatalf("expected footer 'Page 1/1 (2 total records)', got: %q", lines[4])
	}
}

func TestCLI_E2E_Query_CSVOutput(t *testing.T) {
	table := seedQueryRows(t, 2)

	stdout, stderr, exitCode := runCLIE2E(t, "query", table, "--output", "csv", "--fields", "id,title,score", "--sort", "id")
	if exitCode != 0 {
		t.Fatalf("query failed (exit %d): %s", exitCode, stderr)
	}

	reader := csv.NewReader(strings.NewReader(stdout))
	records, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("failed to parse CSV output: %v\nstdout: %s", err, stdout)
	}
	if len(records) != 3 {
		t.Fatalf("expected 3 CSV records (header + 2 rows), got %d:\n%s", len(records), stdout)
	}
	if got := strings.Join(records[0], ","); got != "id,title,score" {
		t.Fatalf("expected CSV header 'id,title,score', got: %s", got)
	}
	if got := strings.Join(records[1], ","); got != "1,item_1,10" {
		t.Fatalf("expected first CSV row '1,item_1,10', got: %s", got)
	}
	if got := strings.Join(records[2], ","); got != "2,item_2,20" {
		t.Fatalf("expected second CSV row '2,item_2,20', got: %s", got)
	}
}

func TestCLI_E2E_Query_LimitMapsToPerPage(t *testing.T) {
	table := seedQueryRows(t, 5)

	stdout, stderr, exitCode := runCLIE2E(t, "query", table, "--json", "--sort", "id", "--limit", "2")
	if exitCode != 0 {
		t.Fatalf("query failed (exit %d): %s", exitCode, stderr)
	}

	resp := parseQueryJSON(t, stdout)
	if len(resp.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(resp.Items))
	}
	if resp.PerPage != 2 {
		t.Fatalf("expected perPage 2, got %d", resp.PerPage)
	}
	if resp.TotalItems != 5 {
		t.Fatalf("expected totalItems 5, got %d", resp.TotalItems)
	}
	// First page should contain rows 1 and 2.
	if title, ok := resp.Items[0]["title"].(string); !ok || title != "item_1" {
		t.Errorf("expected first item title 'item_1', got %v", resp.Items[0]["title"])
	}
	if title, ok := resp.Items[1]["title"].(string); !ok || title != "item_2" {
		t.Errorf("expected second item title 'item_2', got %v", resp.Items[1]["title"])
	}
}

// ---------------------------------------------------------------------------
// ayb query tests — fields selection, empty results, error paths
// ---------------------------------------------------------------------------

func TestCLI_E2E_Query_FieldsSelection(t *testing.T) {
	table := uniqueTableName(t, "s3")
	dropTableCleanup(t, table)

	ddl := fmt.Sprintf("CREATE TABLE %s (id serial PRIMARY KEY, title text NOT NULL, score integer NOT NULL, description text)", table)
	_, stderr, exitCode := runCLIE2E(t, "sql", ddl)
	if exitCode != 0 {
		t.Fatalf("CREATE TABLE failed (exit %d): %s", exitCode, stderr)
	}

	for i := 1; i <= 2; i++ {
		insert := fmt.Sprintf("INSERT INTO %s (title, score, description) VALUES ('item_%d', %d, 'desc_%d')", table, i, i*10, i)
		_, stderr, exitCode := runCLIE2E(t, "sql", insert)
		if exitCode != 0 {
			t.Fatalf("INSERT row %d failed (exit %d): %s", i, exitCode, stderr)
		}
	}

	stdout, stderr, exitCode := runCLIE2E(t, "query", table, "--json", "--fields", "title,score")
	if exitCode != 0 {
		t.Fatalf("query failed (exit %d): %s", exitCode, stderr)
	}

	resp := parseQueryJSON(t, stdout)
	if len(resp.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(resp.Items))
	}
	for i, item := range resp.Items {
		if _, ok := item["title"]; !ok {
			t.Errorf("item %d: expected 'title' key", i)
		}
		if _, ok := item["score"]; !ok {
			t.Errorf("item %d: expected 'score' key", i)
		}
		if _, ok := item["description"]; ok {
			t.Errorf("item %d: unexpected 'description' key", i)
		}
		if _, ok := item["id"]; ok {
			t.Errorf("item %d: unexpected 'id' key", i)
		}
	}
}

func TestCLI_E2E_Query_EmptyTable(t *testing.T) {
	table := uniqueTableName(t, "s3")
	dropTableCleanup(t, table)

	ddl := fmt.Sprintf("CREATE TABLE %s (id serial PRIMARY KEY, name text)", table)
	_, stderr, exitCode := runCLIE2E(t, "sql", ddl)
	if exitCode != 0 {
		t.Fatalf("CREATE TABLE failed (exit %d): %s", exitCode, stderr)
	}

	stdout, stderr, exitCode := runCLIE2E(t, "query", table)
	if exitCode != 0 {
		t.Fatalf("expected exit 0 for empty table, got %d; stderr: %s", exitCode, stderr)
	}
	if !strings.Contains(stdout, "No records found.") {
		t.Fatalf("expected 'No records found.' in stdout, got: %s", stdout)
	}
}

func TestCLI_E2E_Query_NonexistentTable(t *testing.T) {
	stdout, stderr, exitCode := runCLIE2E(t, "query", "e2e_s3_no_such_table_xyz")
	if exitCode == 0 {
		t.Fatalf("expected non-zero exit code for nonexistent table; stdout: %s", stdout)
	}
	if !strings.Contains(stderr, "server error") {
		t.Fatalf("expected stderr to contain 'server error', got: %s", stderr)
	}
}

func TestCLI_E2E_Query_Pagination(t *testing.T) {
	table := seedQueryRows(t, 5)

	stdout, stderr, exitCode := runCLIE2E(t, "query", table, "--json", "--sort", "id", "--limit", "3", "--page", "2")
	if exitCode != 0 {
		t.Fatalf("query failed (exit %d): %s", exitCode, stderr)
	}

	resp := parseQueryJSON(t, stdout)
	if len(resp.Items) != 2 {
		t.Fatalf("expected 2 items on page 2, got %d", len(resp.Items))
	}
	if resp.Page != 2 {
		t.Fatalf("expected page 2, got %d", resp.Page)
	}
	if resp.TotalPages != 2 {
		t.Fatalf("expected totalPages 2, got %d", resp.TotalPages)
	}
	// Page 2 with limit 3 should contain items 4 and 5.
	if title, ok := resp.Items[0]["title"].(string); !ok || title != "item_4" {
		t.Errorf("expected first item on page 2 to be 'item_4', got %v", resp.Items[0]["title"])
	}
	if title, ok := resp.Items[1]["title"].(string); !ok || title != "item_5" {
		t.Errorf("expected second item on page 2 to be 'item_5', got %v", resp.Items[1]["title"])
	}
}
