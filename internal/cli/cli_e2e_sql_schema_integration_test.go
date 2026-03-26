//go:build integration

package cli

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// ayb sql tests
// ---------------------------------------------------------------------------

func TestCLI_E2E_SQL_CreateTable(t *testing.T) {
	table := uniqueTableName(t, "s2")
	dropTableCleanup(t, table)

	ddl := fmt.Sprintf("CREATE TABLE %s (id serial PRIMARY KEY, name text NOT NULL)", table)
	stdout, stderr, exitCode := runCLIE2E(t, "sql", ddl)
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d; stderr: %s", exitCode, stderr)
	}
	if strings.TrimSpace(stdout) == "" {
		t.Fatal("expected non-empty stdout from CREATE TABLE")
	}
}

func TestCLI_E2E_SQL_CreateTableJSON(t *testing.T) {
	table := uniqueTableName(t, "s2")
	dropTableCleanup(t, table)

	ddl := fmt.Sprintf("CREATE TABLE %s (id serial PRIMARY KEY, name text NOT NULL)", table)
	stdout, stderr, exitCode := runCLIE2E(t, "sql", ddl, "--json")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d; stderr: %s", exitCode, stderr)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("expected valid JSON stdout, got parse error: %v\nstdout: %s", err, stdout)
	}
	if _, ok := result["rowCount"]; !ok {
		t.Fatalf("expected rowCount field in JSON response, got: %s", stdout)
	}
}

func TestCLI_E2E_SQL_InvalidSQL(t *testing.T) {
	stdout, stderr, exitCode := runCLIE2E(t, "sql", "SELECT FROM nonexistent_gobbledygook")
	if exitCode == 0 {
		t.Fatalf("expected non-zero exit code for invalid SQL; stdout: %s", stdout)
	}
	if !strings.Contains(stderr, "server error") {
		t.Fatalf("expected stderr to contain 'server error', got: %s", stderr)
	}
}

// ---------------------------------------------------------------------------
// ayb schema tests
// ---------------------------------------------------------------------------

func TestCLI_E2E_Schema_ListAfterCreate(t *testing.T) {
	table := uniqueTableName(t, "s2")
	dropTableCleanup(t, table)

	// Create the table via sql command.
	ddl := fmt.Sprintf("CREATE TABLE %s (id serial PRIMARY KEY)", table)
	_, stderr, exitCode := runCLIE2E(t, "sql", ddl)
	if exitCode != 0 {
		t.Fatalf("CREATE TABLE failed (exit %d): %s", exitCode, stderr)
	}

	// Immediately list schema — proves synchronous ReloadWait after DDL.
	stdout, stderr, exitCode := runCLIE2E(t, "schema", "--json")
	if exitCode != 0 {
		t.Fatalf("schema --json failed (exit %d): %s", exitCode, stderr)
	}

	var tables []map[string]any
	if err := json.Unmarshal([]byte(stdout), &tables); err != nil {
		t.Fatalf("expected JSON array from schema --json, got parse error: %v\nstdout: %s", err, stdout)
	}

	found := false
	for _, tbl := range tables {
		if name, ok := tbl["name"].(string); ok && name == table {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected table %q in schema list, not found in %d tables", table, len(tables))
	}
}

func TestCLI_E2E_Schema_DetailAfterCreate(t *testing.T) {
	table := uniqueTableName(t, "s2")
	dropTableCleanup(t, table)

	ddl := fmt.Sprintf("CREATE TABLE %s (id serial PRIMARY KEY, title text NOT NULL, score integer)", table)
	_, stderr, exitCode := runCLIE2E(t, "sql", ddl)
	if exitCode != 0 {
		t.Fatalf("CREATE TABLE failed (exit %d): %s", exitCode, stderr)
	}

	// Fetch detail for the specific table.
	stdout, stderr, exitCode := runCLIE2E(t, "schema", table, "--json")
	if exitCode != 0 {
		t.Fatalf("schema detail failed (exit %d): %s", exitCode, stderr)
	}

	var detail struct {
		Name    string `json:"name"`
		Columns []struct {
			Name     string `json:"name"`
			Type     string `json:"type"`
			Nullable bool   `json:"nullable"`
		} `json:"columns"`
	}
	if err := json.Unmarshal([]byte(stdout), &detail); err != nil {
		t.Fatalf("expected JSON object from schema detail, got parse error: %v\nstdout: %s", err, stdout)
	}

	if detail.Name != table {
		t.Fatalf("expected table name %q, got %q", table, detail.Name)
	}

	// Verify expected columns exist with correct properties.
	expectedCols := map[string]struct {
		typ      string
		nullable bool
	}{
		"id":    {typ: "integer", nullable: false},
		"title": {typ: "text", nullable: false},
		"score": {typ: "integer", nullable: true},
	}

	for _, col := range detail.Columns {
		if exp, ok := expectedCols[col.Name]; ok {
			if col.Type != exp.typ {
				t.Errorf("column %q: expected type %q, got %q", col.Name, exp.typ, col.Type)
			}
			if col.Nullable != exp.nullable {
				t.Errorf("column %q: expected nullable=%v, got %v", col.Name, exp.nullable, col.Nullable)
			}
			delete(expectedCols, col.Name)
		}
	}
	if len(expectedCols) > 0 {
		missing := make([]string, 0, len(expectedCols))
		for name := range expectedCols {
			missing = append(missing, name)
		}
		t.Fatalf("missing expected columns: %v", missing)
	}
}

func TestCLI_E2E_Schema_TableNotFound(t *testing.T) {
	stdout, stderr, exitCode := runCLIE2E(t, "schema", "e2e_nonexistent_table_xyz")
	if exitCode == 0 {
		t.Fatalf("expected non-zero exit code for nonexistent table; stdout: %s", stdout)
	}
	if !strings.Contains(stderr, "not found") {
		t.Fatalf("expected stderr to contain 'not found', got: %s", stderr)
	}
}
