package pgmanager

import (
	"os/exec"
	"testing"
)

// TestBuildScriptSyntax validates that scripts/build-postgres.sh is syntactically
// correct bash. This catches typos, unmatched brackets, and other shell errors
// without actually compiling Postgres.
func TestBuildScriptSyntax(t *testing.T) {
	t.Parallel()

	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available")
	}

	cmd := exec.Command("bash", "-n", "../../scripts/build-postgres.sh")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build-postgres.sh has syntax errors:\n%s", out)
	}
}
