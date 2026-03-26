//go:build integration

package cli

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// sanitizeTestName lowercases the test name and replaces slashes, spaces,
// hyphens, and underscores with the given separator. Used by all unique-name
// generators (table, function, site slug) to avoid collisions in a shared DB.
func sanitizeTestName(t *testing.T, sep string) string {
	t.Helper()
	safe := strings.NewReplacer("/", sep, " ", sep, "-", sep, "_", sep).Replace(t.Name())
	return strings.ToLower(safe)
}

// uniqueTableName returns a table name safe for use in a shared database,
// incorporating a stage prefix, the test name (sanitized), and a nanosecond
// timestamp to avoid collisions between parallel runs.
func uniqueTableName(t *testing.T, stagePrefix string) string {
	t.Helper()
	return fmt.Sprintf("e2e_%s_%s_%d", stagePrefix, sanitizeTestName(t, "_"), time.Now().UnixNano())
}

// dropTableCleanup registers a t.Cleanup that drops the given table via the CLI.
func dropTableCleanup(t *testing.T, tableName string) {
	t.Helper()
	t.Cleanup(func() {
		stdout, stderr, exitCode := runCLIE2E(t, "sql", fmt.Sprintf("DROP TABLE IF EXISTS %s", tableName))
		if exitCode != 0 {
			t.Fatalf("DROP TABLE cleanup failed for %q (exit %d): stdout=%q stderr=%q", tableName, exitCode, stdout, stderr)
		}
	})
}
