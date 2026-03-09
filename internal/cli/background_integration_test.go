//go:build integration

// Integration tests for background mode (ayb start / ayb stop / ayb status).
// These tests build and run the real ayb binary, starting an actual server.
//
// Run with: go test -tags integration -v -count=1 ./internal/cli/ -run TestBackground -timeout 300s
//
// Prerequisites:
//   - Go toolchain available
//   - Port 18090 must be free (uses non-default port to avoid conflicts)
//   - Internet access (first run downloads PostgreSQL binaries)

package cli

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// buildTestBinary builds the ayb binary into a temp directory and returns its path.
func buildTestBinary(t *testing.T) string {
	t.Helper()
	binDir := t.TempDir()
	binPath := filepath.Join(binDir, "ayb")

	// Find the module root (where go.mod is).
	modRoot, err := findModRoot()
	if err != nil {
		t.Fatalf("finding module root: %v", err)
	}

	cmd := exec.Command("go", "build", "-o", binPath, "./cmd/ayb")
	cmd.Dir = modRoot
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("building ayb binary: %v\n%s", err, out)
	}
	return binPath
}

func findModRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod not found")
		}
		dir = parent
	}
}

func waitForHealthPort(port int, timeout time.Duration) bool {
	client := &http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/health", port))
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return true
			}
		}
		time.Sleep(300 * time.Millisecond)
	}
	return false
}

func waitForNoHealthPort(port int, timeout time.Duration) bool {
	client := &http.Client{Timeout: 1 * time.Second}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/health", port))
		if err != nil {
			return true
		}
		resp.Body.Close()
		time.Sleep(300 * time.Millisecond)
	}
	return false
}

// TestBackgroundStartStopCycle tests 7.5 (start), 7.7 (double-start), and 7.6 (stop).
func TestBackgroundStartStopCycle(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test, skipping with -short")
	}

	bin := buildTestBinary(t)
	port := 18090

	// Ensure clean state.
	stopCmd := exec.Command(bin, "stop")
	stopCmd.Run()
	time.Sleep(500 * time.Millisecond)

	// ── 7.5: ayb start → background, banner, status ──

	t.Run("7.5_start_background", func(t *testing.T) {
		cmd := exec.Command(bin, "start", "--port", fmt.Sprintf("%d", port))
		out, err := cmd.CombinedOutput()
		output := string(out)

		if err != nil {
			t.Fatalf("ayb start failed: %v\nOutput: %s", err, output)
		}

		// Banner checks.
		if !strings.Contains(output, "Allyourbase") {
			t.Error("banner missing product name")
		}
		if !strings.Contains(output, "API:") {
			t.Error("banner missing API URL")
		}
		if !strings.Contains(output, "ayb stop") {
			t.Error("banner missing stop hint")
		}
		if !strings.Contains(output, "Admin password:") {
			t.Error("banner missing generated admin password")
		}

		// Health check.
		if !waitForHealthPort(port, 5*time.Second) {
			t.Fatal("health endpoint not responding after start")
		}

		// PID file.
		pidPath, _ := aybPIDPath()
		if _, err := os.Stat(pidPath); err != nil {
			t.Errorf("PID file not found: %v", err)
		}

		// Admin token file.
		tokenPath, _ := aybAdminTokenPath()
		if _, err := os.Stat(tokenPath); err != nil {
			t.Errorf("admin token file not found: %v", err)
		}

		// ayb status.
		statusCmd := exec.Command(bin, "status", "--json")
		statusOut, err := statusCmd.CombinedOutput()
		if err != nil {
			t.Errorf("ayb status failed: %v", err)
		}
		statusStr := string(statusOut)
		if !strings.Contains(statusStr, `"status":"running"`) {
			t.Errorf("status should show running, got: %s", statusStr)
		}
	})

	// ── 7.7: double-start → already running ──

	t.Run("7.7_double_start", func(t *testing.T) {
		cmd := exec.Command(bin, "start", "--port", fmt.Sprintf("%d", port))
		out, err := cmd.CombinedOutput()
		output := string(out)

		if err != nil {
			t.Errorf("second ayb start should not error: %v", err)
		}
		if !strings.Contains(strings.ToLower(output), "already running") {
			t.Errorf("expected 'already running', got: %s", output)
		}
	})

	// ── 7.6: ayb stop → clean shutdown ──

	t.Run("7.6_stop_clean", func(t *testing.T) {
		cmd := exec.Command(bin, "stop")
		out, err := cmd.CombinedOutput()
		output := string(out)

		if err != nil {
			t.Fatalf("ayb stop failed: %v\nOutput: %s", err, output)
		}
		if !strings.Contains(strings.ToLower(output), "stopped") {
			t.Errorf("expected 'stopped' in output, got: %s", output)
		}

		// Wait for health to go away.
		if !waitForNoHealthPort(port, 10*time.Second) {
			t.Error("health endpoint still responding after stop")
		}

		// PID and token files should be cleaned up.
		pidPath, _ := aybPIDPath()
		if _, err := os.Stat(pidPath); !os.IsNotExist(err) {
			t.Error("PID file still exists after stop")
		}
		tokenPath, _ := aybAdminTokenPath()
		if _, err := os.Stat(tokenPath); !os.IsNotExist(err) {
			t.Error("admin token file still exists after stop")
		}

		// ayb status should show not running.
		statusCmd := exec.Command(bin, "status")
		statusOut, _ := statusCmd.CombinedOutput()
		if !strings.Contains(string(statusOut), "not running") {
			t.Errorf("expected 'not running', got: %s", statusOut)
		}

		// Idempotent: stop on stopped server should be safe.
		stopAgain := exec.Command(bin, "stop")
		stopOut, err := stopAgain.CombinedOutput()
		if err != nil {
			t.Errorf("stop on stopped server should not error: %v", err)
		}
		stopStr := strings.ToLower(string(stopOut))
		if !strings.Contains(stopStr, "not running") && !strings.Contains(stopStr, "no ayb") {
			t.Errorf("expected 'not running' message, got: %s", stopOut)
		}
	})
}

// TestBackgroundForegroundSignal tests 7.8: foreground mode + Ctrl-C graceful shutdown.
func TestBackgroundForegroundSignal(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test, skipping with -short")
	}

	bin := buildTestBinary(t)
	port := 18091

	// Ensure clean state.
	stopCmd := exec.Command(bin, "stop")
	stopCmd.Run()
	time.Sleep(500 * time.Millisecond)

	cmd := exec.Command(bin, "start", "--foreground", "--port", fmt.Sprintf("%d", port))
	var stderr strings.Builder
	cmd.Stderr = &stderr
	cmd.Stdout = &stderr

	if err := cmd.Start(); err != nil {
		t.Fatalf("starting foreground server: %v", err)
	}

	// Wait for health.
	if !waitForHealthPort(port, 60*time.Second) {
		cmd.Process.Kill()
		t.Fatalf("foreground server did not become healthy\nOutput: %s", stderr.String())
	}

	// Verify process is still running (should be blocking).
	if cmd.ProcessState != nil {
		t.Fatal("foreground process exited prematurely")
	}

	// Send SIGINT.
	cmd.Process.Signal(os.Interrupt)

	// Wait for exit with timeout.
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case <-done:
		// Process exited.
	case <-time.After(15 * time.Second):
		cmd.Process.Kill()
		t.Fatal("foreground process did not exit within 15s after SIGINT")
	}

	output := stderr.String()
	if !strings.Contains(output, "Shutting down") {
		t.Errorf("expected 'Shutting down' message, got: %s", output)
	}

	// Health should be gone.
	if !waitForNoHealthPort(port, 5*time.Second) {
		t.Error("health endpoint still responding after foreground shutdown")
	}
}
