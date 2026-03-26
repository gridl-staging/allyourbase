//go:build integration

package cli

import (
	"bytes"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
)

var (
	cliE2EHarnessBaseURL      = "http://127.0.0.1:18092"
	cliE2EHarnessBinaryPath   string
	cliE2EHarnessBearerToken  string
	cliE2EHarnessDatabaseURL  string
	cliE2EHarnessDatabaseName string
)

func TestCLI_E2E_HarnessStateInitialized(t *testing.T) {
	if cliE2EHarnessBaseURL == "" {
		t.Fatal("expected cliE2EHarnessBaseURL to be initialized by TestMain")
	}
	if cliE2EHarnessBinaryPath == "" {
		t.Fatal("expected cliE2EHarnessBinaryPath to be initialized by TestMain")
	}
	if _, err := os.Stat(cliE2EHarnessBinaryPath); err != nil {
		t.Fatalf("expected compiled harness binary to exist: %v", err)
	}
	if cliE2EHarnessBearerToken == "" {
		t.Fatal("expected cliE2EHarnessBearerToken to be initialized by TestMain")
	}
}

func TestCLI_E2E_HarnessBootstrapValidation(t *testing.T) {
	t.Run("missing_TEST_DATABASE_URL_fails_fast", func(t *testing.T) {
		_, _, err := deriveHarnessDatabaseConfig("", "postgres://user:pass@127.0.0.1:5432/shared_db", "cli_e2e_db")
		if err == nil {
			t.Fatal("expected missing TEST_DATABASE_URL to fail")
		}
		if !strings.Contains(err.Error(), "TEST_DATABASE_URL") {
			t.Fatalf("expected TEST_DATABASE_URL error, got: %v", err)
		}
	})

	t.Run("sharing_demo_db_is_rejected", func(t *testing.T) {
		testDBURL := "postgres://user:pass@127.0.0.1:5432/cluster_admin"
		sharedDBURL := "postgres://user:pass@127.0.0.1:5432/shared_db"
		_, _, err := deriveHarnessDatabaseConfig(testDBURL, sharedDBURL, "shared_db")
		if err == nil {
			t.Fatal("expected shared demo database to be rejected")
		}
		if !strings.Contains(err.Error(), "sharedPG") {
			t.Fatalf("expected sharedPG rejection error, got: %v", err)
		}
	})
}

func TestCLI_E2E_HarnessBearerTokenCanReadSchema(t *testing.T) {
	if cliE2EHarnessBearerToken == "" {
		t.Fatal("expected cliE2EHarnessBearerToken to be initialized by TestMain")
	}
	req, err := http.NewRequest(http.MethodGet, cliE2EHarnessBaseURL+"/api/schema", nil)
	if err != nil {
		t.Fatalf("building schema request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+cliE2EHarnessBearerToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("calling /api/schema with harness token: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected /api/schema status 200, got %d", resp.StatusCode)
	}
}

// runCLIE2E invokes the compiled ayb binary with the given args, automatically
// appending --url and --admin-token flags (which are subcommand-local flags
// and must follow the subcommand name). Returns captured stdout, stderr, and
// the process exit code. Stages 2-5 reuse this helper.
func runCLIE2E(t *testing.T, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()

	// Append auth/url flags after the caller's subcommand args.
	fullArgs := append(args, "--url", cliE2EHarnessBaseURL, "--admin-token", cliE2EHarnessBearerToken)

	var outBuf, errBuf bytes.Buffer
	cmd := exec.Command(cliE2EHarnessBinaryPath, fullArgs...)
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	err := cmd.Run()
	stdout = outBuf.String()
	stderr = errBuf.String()

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			// Non-exit error (e.g. binary not found) — treat as fatal.
			t.Fatalf("runCLIE2E exec error: %v", err)
		}
	}
	return stdout, stderr, exitCode
}
