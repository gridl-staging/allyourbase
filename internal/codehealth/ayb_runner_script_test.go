package codehealth

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const runWithAYBScript = "scripts/run-with-ayb.sh"
const skippedPostHealthCommand = "echo should-not-run"

func runAYBScript(t *testing.T, postHealthCommand string, env ...string) (string, error) {
	t.Helper()

	if !hasEnvKey(env, "HOME") {
		env = append([]string{"HOME=" + t.TempDir()}, env...)
	}

	cmd := exec.Command("bash", runWithAYBScript, postHealthCommand)
	cmd.Dir = findRepoRoot(t)
	cmd.Env = append(os.Environ(), env...)

	output, err := cmd.CombinedOutput()
	return string(output), err
}

func hasEnvKey(env []string, key string) bool {
	prefix := key + "="
	for _, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			return true
		}
	}
	return false
}

func requireOutputContains(t *testing.T, output, want string) {
	t.Helper()

	if !strings.Contains(output, want) {
		t.Fatalf("expected output to contain %q, got: %s", want, output)
	}
}

func requireWrappedCommandSkipped(t *testing.T, output string) {
	t.Helper()

	if strings.Contains(output, "should-not-run") {
		t.Fatalf("wrapped command should not run, got: %s", output)
	}
}

func requireFileContainsTrimmed(t *testing.T, path, want string) {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if got := strings.TrimSpace(string(data)); got != want {
		t.Fatalf("expected %s to contain %q, got %q", path, want, got)
	}
}

func freshTokenWriterStartCommand(port int, writeDelay time.Duration) string {
	writeFreshToken := `fs.mkdirSync(path.join(home,'.ayb'), { recursive: true }); fs.writeFileSync(path.join(home,'.ayb','admin-token'), 'fresh-token\n');`
	if writeDelay > 0 {
		return fmt.Sprintf(`AYB_START_COMMAND=node -e "const fs=require('fs'); const path=require('path'); const http=require('http'); const home=process.env.HOME; http.createServer((_,res)=>res.end('ok')).listen(%d); setTimeout(() => { %s }, %d); setInterval(() => {}, 1000);"`, port, writeFreshToken, writeDelay/time.Millisecond)
	}
	return fmt.Sprintf(`AYB_START_COMMAND=node -e "const fs=require('fs'); const path=require('path'); const http=require('http'); const home=process.env.HOME; %s http.createServer((_,res)=>res.end('ok')).listen(%d); setInterval(() => {}, 1000);"`, writeFreshToken, port)
}

func TestRunWithAYBScriptRequiresPostHealthCommandArgument(t *testing.T) {
	t.Parallel()

	cmd := exec.Command("bash", runWithAYBScript)
	cmd.Dir = findRepoRoot(t)

	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected usage failure when no command argument is provided, got success: %s", output)
	}
	requireOutputContains(t, string(output), "Usage:")
}

func TestRunWithAYBScriptFailsFastOnHealthTimeout(t *testing.T) {
	t.Parallel()

	logPath := filepath.Join(t.TempDir(), "ayb-timeout.log")

	output, err := runAYBScript(t, skippedPostHealthCommand,
		"AYB_START_COMMAND=sleep 30",
		"AYB_HEALTH_URL=http://127.0.0.1:9/health",
		"AYB_HEALTH_TIMEOUT_SECONDS=1",
		"AYB_HEALTH_POLL_INTERVAL_SECONDS=0.1",
		"AYB_START_LOG="+logPath,
	)
	if err == nil {
		t.Fatalf("expected timeout failure, got success: %s", output)
	}
	requireOutputContains(t, output, "Timed out waiting for AYB health check")
	requireWrappedCommandSkipped(t, output)
}

func TestRunWithAYBScriptRejectsInvalidHealthTimeoutConfig(t *testing.T) {
	t.Parallel()

	output, err := runAYBScript(t, skippedPostHealthCommand,
		"AYB_HEALTH_TIMEOUT_SECONDS=0",
	)
	if err == nil {
		t.Fatalf("expected invalid-timeout failure, got success: %s", output)
	}
	requireOutputContains(t, output, "AYB_HEALTH_TIMEOUT_SECONDS must be a positive integer")
	requireWrappedCommandSkipped(t, output)
}

func TestRunWithAYBScriptRejectsInvalidHealthPollIntervalConfig(t *testing.T) {
	t.Parallel()

	output, err := runAYBScript(t, skippedPostHealthCommand,
		"AYB_HEALTH_POLL_INTERVAL_SECONDS=not-a-number",
	)
	if err == nil {
		t.Fatalf("expected invalid-poll-interval failure, got success: %s", output)
	}
	requireOutputContains(t, output, "AYB_HEALTH_POLL_INTERVAL_SECONDS must be a positive number")
	requireWrappedCommandSkipped(t, output)
}

func TestRunWithAYBScriptFailsIfProcessExitsBeforeHealthy(t *testing.T) {
	t.Parallel()

	logPath := filepath.Join(t.TempDir(), "ayb-exit.log")

	output, err := runAYBScript(t, skippedPostHealthCommand,
		"AYB_START_COMMAND=sh -c 'exit 0'",
		"AYB_HEALTH_URL=http://127.0.0.1:9/health",
		"AYB_HEALTH_TIMEOUT_SECONDS=3",
		"AYB_HEALTH_POLL_INTERVAL_SECONDS=0.1",
		"AYB_START_LOG="+logPath,
	)
	if err == nil {
		t.Fatalf("expected process-exit failure, got success: %s", output)
	}
	requireOutputContains(t, output, "AYB process exited before health check passed")
	requireWrappedCommandSkipped(t, output)
}

func TestRunWithAYBScriptRunsCommandAfterHealthReady(t *testing.T) {
	t.Parallel()

	logPath := filepath.Join(t.TempDir(), "ayb-success.log")
	healthPort := reserveLocalhostPort(t)
	healthURL := fmt.Sprintf("http://127.0.0.1:%d/health", healthPort)

	output, err := runAYBScript(t, "echo command-finished",
		fmt.Sprintf(`AYB_START_COMMAND=node -e "const http=require('http'); const server=http.createServer((_,res)=>res.end('ok')); server.listen(%d);"`, healthPort),
		"AYB_HEALTH_URL="+healthURL,
		"AYB_HEALTH_TIMEOUT_SECONDS=10",
		"AYB_HEALTH_POLL_INTERVAL_SECONDS=0.1",
		"AYB_ADMIN_PASSWORD=test-admin-password",
		"AYB_START_LOG="+logPath,
	)
	if err != nil {
		t.Fatalf("expected success, got error: %v output=%s", err, output)
	}
	requireOutputContains(t, output, "command-finished")
}

func TestRunWithAYBScriptUsesFreshAdminTokenAndRestoresOriginalToken(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "ayb-token.log")
	tokenPath := filepath.Join(homeDir, ".ayb", "admin-token")
	healthPort := reserveLocalhostPort(t)
	healthURL := fmt.Sprintf("http://127.0.0.1:%d/health", healthPort)
	writeTextFile(t, tokenPath, "original-token\n")

	output, err := runAYBScript(t, `test "$(cat "$HOME/.ayb/admin-token")" = fresh-token && echo command-finished`,
		"HOME="+homeDir,
		freshTokenWriterStartCommand(healthPort, 500*time.Millisecond),
		"AYB_HEALTH_URL="+healthURL,
		"AYB_HEALTH_TIMEOUT_SECONDS=10",
		"AYB_HEALTH_POLL_INTERVAL_SECONDS=0.1",
		"AYB_START_LOG="+logPath,
	)
	if err != nil {
		t.Fatalf("expected success, got error: %v output=%s", err, output)
	}
	requireOutputContains(t, output, "command-finished")
	requireFileContainsTrimmed(t, tokenPath, "original-token")
}

func TestRunWithAYBScriptRestoresOriginalTokenWhenAdminPasswordProvided(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "ayb-token-with-password.log")
	tokenPath := filepath.Join(homeDir, ".ayb", "admin-token")
	healthPort := reserveLocalhostPort(t)
	healthURL := fmt.Sprintf("http://127.0.0.1:%d/health", healthPort)
	writeTextFile(t, tokenPath, "original-token\n")

	output, err := runAYBScript(t, `test "$(cat "$HOME/.ayb/admin-token")" = fresh-token && echo command-finished`,
		"HOME="+homeDir,
		freshTokenWriterStartCommand(healthPort, 0),
		"AYB_HEALTH_URL="+healthURL,
		"AYB_HEALTH_TIMEOUT_SECONDS=10",
		"AYB_HEALTH_POLL_INTERVAL_SECONDS=0.1",
		"AYB_ADMIN_PASSWORD=test-admin-password",
		"AYB_START_LOG="+logPath,
	)
	if err != nil {
		t.Fatalf("expected success, got error: %v output=%s", err, output)
	}
	requireOutputContains(t, output, "command-finished")
	requireFileContainsTrimmed(t, tokenPath, "original-token")
}

func TestRunWithAYBScriptStopsForegroundServerOnExit(t *testing.T) {
	t.Parallel()

	logPath := filepath.Join(t.TempDir(), "ayb-cleanup.log")
	healthPort := reserveLocalhostPort(t)
	healthURL := fmt.Sprintf("http://127.0.0.1:%d/health", healthPort)

	output, err := runAYBScript(t, "echo command-finished",
		fmt.Sprintf(`AYB_START_COMMAND=node -e "const http=require('http'); const server=http.createServer((_,res)=>res.end('ok')); server.listen(%d);"`, healthPort),
		"AYB_HEALTH_URL="+healthURL,
		"AYB_HEALTH_TIMEOUT_SECONDS=10",
		"AYB_HEALTH_POLL_INTERVAL_SECONDS=0.1",
		"AYB_ADMIN_PASSWORD=test-admin-password",
		"AYB_START_LOG="+logPath,
	)
	if err != nil {
		t.Fatalf("expected success, got error: %v output=%s", err, output)
	}
	requireOutputContains(t, output, "command-finished")

	client := &http.Client{Timeout: 200 * time.Millisecond}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		resp, getErr := client.Get(healthURL)
		if getErr != nil {
			return
		}
		resp.Body.Close()
		time.Sleep(50 * time.Millisecond)
	}

	t.Fatal("expected foreground AYB process to be stopped after wrapper exit")
}

func reserveLocalhostPort(t *testing.T) int {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserveLocalhostPort listen: %v", err)
	}
	defer listener.Close()

	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok || addr.Port <= 0 {
		t.Fatalf("reserveLocalhostPort invalid address: %#v", listener.Addr())
	}
	return addr.Port
}
