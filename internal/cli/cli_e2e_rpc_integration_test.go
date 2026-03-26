//go:build integration

package cli

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Stage 4 RPC helpers — single source of truth for function fixture lifecycle
// ---------------------------------------------------------------------------

// uniqueFuncName returns a function name safe for use in a shared database,
// incorporating the s4 stage prefix, a sanitized test name, and a nanosecond
// timestamp to avoid collisions between parallel runs.
func uniqueFuncName(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf("s4_%s_%d", sanitizeTestName(t, "_"), time.Now().UnixNano())
}

// createFunctionSQL creates a PostgreSQL function via ayb sql and fatals on error.
func createFunctionSQL(t *testing.T, ddl string) {
	t.Helper()
	_, stderr, exitCode := runCLIE2E(t, "sql", ddl)
	if exitCode != 0 {
		t.Fatalf("CREATE FUNCTION failed (exit %d): %s", exitCode, stderr)
	}
}

// dropFunctionCleanup registers a t.Cleanup that drops the given function via
// ayb sql. The signature string must match the parameter types exactly (e.g.
// "integer, boolean, text, text[]") so PostgreSQL can resolve overloads.
func dropFunctionCleanup(t *testing.T, funcName, paramSignature string) {
	t.Helper()
	t.Cleanup(func() {
		stmt := fmt.Sprintf("DROP FUNCTION IF EXISTS %s(%s)", funcName, paramSignature)
		stdout, stderr, exitCode := runCLIE2E(t, "sql", stmt)
		if exitCode != 0 {
			t.Fatalf("DROP FUNCTION cleanup failed for %s (exit %d): stdout=%q stderr=%q",
				funcName, exitCode, stdout, stderr)
		}
	})
}

// parseRPCJSON unmarshals raw JSON output from ayb rpc --json.
func parseRPCJSON(t *testing.T, stdout string) any {
	t.Helper()
	var result any
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &result); err != nil {
		t.Fatalf("failed to parse RPC JSON: %v\nstdout: %s", err, stdout)
	}
	return result
}

// ---------------------------------------------------------------------------
// Happy-path tests
// ---------------------------------------------------------------------------

// TestCLI_E2E_RPC_JSONArgsCoercion verifies that the CLI→server pipeline
// preserves integer, boolean, string-fallback, and text[] semantics when
// arguments pass through parseRPCArgs → coerceRPCArg → buildRPCCall.
func TestCLI_E2E_RPC_JSONArgsCoercion(t *testing.T) {
	fn := uniqueFuncName(t)
	paramSig := "integer, boolean, text, text[]"
	dropFunctionCleanup(t, fn, paramSig)

	ddl := fmt.Sprintf(`CREATE FUNCTION %s(p_count integer, p_active boolean, p_note text, p_tags text[])
RETURNS TABLE(count integer, active boolean, note text, tags text[])
LANGUAGE sql AS $$ SELECT p_count, p_active, p_note, p_tags; $$`, fn)
	createFunctionSQL(t, ddl)

	stdout, stderr, exitCode := runCLIE2E(t, "rpc", fn, "--json",
		"--arg", "p_count=5",
		"--arg", "p_active=true",
		"--arg", "p_note=hello",
		"--arg", `p_tags=["a","b"]`,
	)
	if exitCode != 0 {
		t.Fatalf("rpc call failed (exit %d): %s", exitCode, stderr)
	}

	// RETURNS TABLE + ReturnsSet → server returns JSON array.
	result := parseRPCJSON(t, stdout)
	arr, ok := result.([]any)
	if !ok {
		t.Fatalf("expected JSON array, got %T: %v", result, result)
	}
	if len(arr) != 1 {
		t.Fatalf("expected 1 row, got %d", len(arr))
	}
	row, ok := arr[0].(map[string]any)
	if !ok {
		t.Fatalf("expected row to be JSON object, got %T", arr[0])
	}
	// Integer: JSON number that equals 5.
	if count, ok := row["count"].(float64); !ok || count != 5 {
		t.Errorf("count: expected 5, got %v (%T)", row["count"], row["count"])
	}
	// Boolean: JSON true.
	if active, ok := row["active"].(bool); !ok || !active {
		t.Errorf("active: expected true, got %v (%T)", row["active"], row["active"])
	}
	// String-fallback: note is text, should come back as string.
	if note, ok := row["note"].(string); !ok || note != "hello" {
		t.Errorf("note: expected 'hello', got %v (%T)", row["note"], row["note"])
	}
	// text[]: should come back as JSON array of strings.
	tags, ok := row["tags"].([]any)
	if !ok {
		t.Fatalf("tags: expected JSON array, got %v (%T)", row["tags"], row["tags"])
	}
	if len(tags) != 2 {
		t.Fatalf("tags: expected 2 elements, got %d", len(tags))
	}
	if tags[0] != "a" || tags[1] != "b" {
		t.Errorf("tags: expected [a, b], got %v", tags)
	}
}

// TestCLI_E2E_RPC_ScalarOutput verifies that a scalar-returning function
// displays the unwrapped value on stdout with exit code 0.
func TestCLI_E2E_RPC_ScalarOutput(t *testing.T) {
	fn := uniqueFuncName(t)
	paramSig := "integer, integer"
	dropFunctionCleanup(t, fn, paramSig)

	ddl := fmt.Sprintf(`CREATE FUNCTION %s(a integer, b integer)
RETURNS integer
LANGUAGE sql AS $$ SELECT a * b; $$`, fn)
	createFunctionSQL(t, ddl)

	stdout, stderr, exitCode := runCLIE2E(t, "rpc", fn,
		"--arg", "a=3",
		"--arg", "b=5",
	)
	if exitCode != 0 {
		t.Fatalf("rpc call failed (exit %d): %s", exitCode, stderr)
	}
	// formatScalar(15.0) → "15"; formatRPCResult prints it + newline.
	if got := strings.TrimSpace(stdout); got != "15" {
		t.Fatalf("expected stdout '15', got %q", got)
	}
}

// TestCLI_E2E_RPC_VoidOutput verifies that a void function prints "(void) OK"
// with exit code 0.
func TestCLI_E2E_RPC_VoidOutput(t *testing.T) {
	fn := uniqueFuncName(t)
	paramSig := ""
	dropFunctionCleanup(t, fn, paramSig)

	ddl := fmt.Sprintf(`CREATE FUNCTION %s()
RETURNS void
LANGUAGE plpgsql AS $$ BEGIN END; $$`, fn)
	createFunctionSQL(t, ddl)

	stdout, stderr, exitCode := runCLIE2E(t, "rpc", fn)
	if exitCode != 0 {
		t.Fatalf("rpc call failed (exit %d): %s", exitCode, stderr)
	}
	if got := strings.TrimSpace(stdout); got != "(void) OK" {
		t.Fatalf("expected stdout '(void) OK', got %q", got)
	}
}

// ---------------------------------------------------------------------------
// Structured-result and failure-path tests
// ---------------------------------------------------------------------------

// TestCLI_E2E_RPC_RecordResultJSON verifies that a RETURNS TABLE function
// returns stable structured JSON via --json. We use --json because Go map
// iteration makes text key/column order unstable in the default table output
// from formatRPCResult; JSON is the stable E2E contract for object- and
// set-shaped RPC results.
func TestCLI_E2E_RPC_RecordResultJSON(t *testing.T) {
	fn := uniqueFuncName(t)
	paramSig := "text, integer"
	dropFunctionCleanup(t, fn, paramSig)

	ddl := fmt.Sprintf(`CREATE FUNCTION %s(p_name text, p_age integer)
RETURNS TABLE(name text, age integer, greeting text)
LANGUAGE sql AS $$ SELECT p_name, p_age, 'Hello ' || p_name; $$`, fn)
	createFunctionSQL(t, ddl)

	stdout, stderr, exitCode := runCLIE2E(t, "rpc", fn, "--json",
		"--arg", "p_name=Alice",
		"--arg", "p_age=30",
	)
	if exitCode != 0 {
		t.Fatalf("rpc call failed (exit %d): %s", exitCode, stderr)
	}

	result := parseRPCJSON(t, stdout)
	arr, ok := result.([]any)
	if !ok {
		t.Fatalf("expected JSON array, got %T: %v", result, result)
	}
	if len(arr) != 1 {
		t.Fatalf("expected 1 row, got %d", len(arr))
	}
	row, ok := arr[0].(map[string]any)
	if !ok {
		t.Fatalf("expected row to be JSON object, got %T", arr[0])
	}

	if row["name"] != "Alice" {
		t.Errorf("name: expected 'Alice', got %v", row["name"])
	}
	if age, ok := row["age"].(float64); !ok || age != 30 {
		t.Errorf("age: expected 30, got %v (%T)", row["age"], row["age"])
	}
	if greeting, ok := row["greeting"].(string); !ok || greeting != "Hello Alice" {
		t.Errorf("greeting: expected 'Hello Alice', got %v", row["greeting"])
	}
}

// TestCLI_E2E_RPC_FunctionError verifies that a function raising a PG exception
// produces non-zero exit and a user-visible error message on stderr. The
// pipeline is: RAISE EXCEPTION → P0001 → mapPGError → HTTP 400 → serverError.
func TestCLI_E2E_RPC_FunctionError(t *testing.T) {
	fn := uniqueFuncName(t)
	paramSig := ""
	dropFunctionCleanup(t, fn, paramSig)

	ddl := fmt.Sprintf(`CREATE FUNCTION %s()
RETURNS void
LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'intentional s4 test error'; END; $$`, fn)
	createFunctionSQL(t, ddl)

	stdout, stderr, exitCode := runCLIE2E(t, "rpc", fn)
	if exitCode == 0 {
		t.Fatalf("expected non-zero exit for RAISE EXCEPTION; stdout: %s", stdout)
	}
	// serverError formats: "server error (400): intentional s4 test error"
	if !strings.Contains(stderr, "intentional s4 test error") {
		t.Fatalf("expected stderr to contain exception message, got: %s", stderr)
	}
	if !strings.Contains(stderr, "server error") {
		t.Fatalf("expected stderr to contain 'server error', got: %s", stderr)
	}
}
