// Package cli Stub summary for /Users/stuart/parallel_development/allyourbase_dev/mar19_03_go_code_quality_refactoring/allyourbase_dev/internal/cli/start_detached.go.
package cli

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/allyourbase/ayb/internal/cli/ui"
	"github.com/allyourbase/ayb/internal/config"
	"github.com/spf13/cobra"
)

// TODO: Document runStartDetached.
func runStartDetached(cmd *cobra.Command, _ []string) error {
	// --- 1. Preflight existing PID state ---
	if handled, err := preflightDetachedStart(); handled || err != nil {
		return err
	}

	// --- 2. Load config (for port, banner info) ---
	configPath, _ := cmd.Flags().GetString("config")
	flags := make(map[string]string)
	if v, _ := cmd.Flags().GetString("database-url"); v != "" {
		flags["database-url"] = v
	}
	if v, _ := cmd.Flags().GetInt("port"); v != 0 {
		flags["port"] = fmt.Sprintf("%d", v)
	}
	if v, _ := cmd.Flags().GetString("host"); v != "" {
		flags["host"] = v
	}

	cfg, err := config.Load(configPath, flags)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	generatedPassword, err := ensureConfiguredAdminPassword(cfg)
	if err != nil {
		return err
	}

	// --- 3. Early port check ---
	if ln, err := net.Listen("tcp", cfg.Address()); err != nil {
		return portError(cfg.Server.Port, err)
	} else {
		ln.Close()
	}

	// --- 4. Detect first run (G6) ---
	firstRun := isFirstRun()
	timeout := 60 * time.Second
	if firstRun {
		timeout = 300 * time.Second
	}

	// --- 5. Build child command (G2, G3) ---
	child, logPath, logFile, err := buildDetachedChildCommand()
	if err != nil {
		return err
	}
	if generatedPassword != "" {
		child.Env = append(child.Env, "AYB_ADMIN_PASSWORD="+generatedPassword)
	}
	if logFile != nil {
		defer logFile.Close()
	}

	// --- 6. Start ---
	isTTY := colorEnabled()
	sp := newStartupProgress(os.Stderr, isTTY, isTTY)
	sp.header(bannerVersion(buildVersion))

	if firstRun {
		sp.step("Downloading PostgreSQL and starting server (first run)...")
	} else {
		sp.step("Starting server...")
	}

	if err := child.Start(); err != nil {
		sp.fail()
		return fmt.Errorf("starting server process: %w", err)
	}

	// Detect early child death (G10).
	childDone := make(chan struct{})
	go func() {
		child.Wait()
		close(childDone)
	}()

	// --- 7. Poll for readiness (G4: check health AND admin-token file) ---
	port := cfg.Server.Port
	healthURL := fmt.Sprintf("http://127.0.0.1:%d/health", port)
	needAdminToken := cfg.Admin.Enabled && generatedPassword != ""
	tokenPath, _ := aybAdminTokenPath()
	readinessErr := waitForDetachedReadiness(detachedReadinessPollOptions{
		healthURL:      healthURL,
		timeout:        timeout,
		pollInterval:   300 * time.Millisecond,
		needAdminToken: needAdminToken,
		tokenPath:      tokenPath,
		logPath:        logPath,
		childDone:      childDone,
		httpClient:     &http.Client{Timeout: 2 * time.Second},
		terminateChild: func() {
			_ = child.Process.Signal(syscall.SIGTERM)
		},
	})
	if readinessErr != nil {
		sp.fail()
		return readinessErr
	}
	sp.done()

	// --- 11. Print banner ---
	embeddedPG := cfg.Database.URL == ""
	if isTTY {
		printBannerBodyTo(os.Stderr, cfg, embeddedPG, true, generatedPassword, logPath)
	} else {
		printBanner(cfg, embeddedPG, generatedPassword, logPath)
	}

	fmt.Fprintf(os.Stderr, "  %s\n\n", dim("Stop with: ayb stop", isTTY))

	return nil
}

// TODO: Document preflightDetachedStart.
func preflightDetachedStart() (bool, error) {
	pid, port, err := readAYBPID()
	if err != nil {
		return false, nil
	}

	proc, findErr := os.FindProcess(pid)
	if findErr == nil && proc.Signal(syscall.Signal(0)) == nil {
		client := &http.Client{Timeout: 2 * time.Second}
		healthURL := fmt.Sprintf("http://127.0.0.1:%d/health", port)
		if resp, hErr := client.Get(healthURL); hErr == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				fmt.Fprintf(os.Stderr, "AYB server is already running (PID %d, port %d).\n", pid, port)
				fmt.Fprintf(os.Stderr, "Stop with: ayb stop\n")
				return true, nil
			}
		}
		return true, waitForExistingServer(port)
	}

	// Stale PID file.
	cleanupServerFiles()
	return false, nil
}

// TODO: Document buildDetachedChildCommand.
func buildDetachedChildCommand() (*exec.Cmd, string, *os.File, error) {
	exePath, err := os.Executable()
	if err != nil {
		return nil, "", nil, fmt.Errorf("resolving executable: %w", err)
	}
	exePath, err = filepath.EvalSymlinks(exePath)
	if err != nil {
		return nil, "", nil, fmt.Errorf("resolving executable symlinks: %w", err)
	}

	child := exec.Command(exePath, buildChildArgs()...)
	child.Dir, _ = os.Getwd()
	child.Env = os.Environ()

	logPath := logFilePath()
	var logFile *os.File
	if logPath != "" {
		logFile, err = os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return nil, "", nil, fmt.Errorf("opening log file: %w", err)
		}
		child.Stdout = logFile
		child.Stderr = logFile
	}

	// setDetachAttrs is a no-op on Windows.
	setDetachAttrs(child)
	return child, logPath, logFile, nil
}

type detachedReadinessPollOptions struct {
	healthURL      string
	timeout        time.Duration
	pollInterval   time.Duration
	needAdminToken bool
	tokenPath      string
	logPath        string
	childDone      <-chan struct{}
	httpClient     *http.Client
	terminateChild func()
}

// TODO: Document waitForDetachedReadiness.
func waitForDetachedReadiness(opts detachedReadinessPollOptions) error {
	deadline := time.Now().Add(opts.timeout)
	ticker := time.NewTicker(opts.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-opts.childDone:
			return fmt.Errorf("server exited during startup (check %s)", opts.logPath)
		case <-ticker.C:
			if time.Now().After(deadline) {
				if opts.terminateChild != nil {
					opts.terminateChild()
				}
				return fmt.Errorf("server did not become ready within %s (check %s)", opts.timeout, opts.logPath)
			}

			resp, err := opts.httpClient.Get(opts.healthURL)
			if err != nil {
				continue
			}
			resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				continue
			}
			if opts.needAdminToken {
				if _, err := os.Stat(opts.tokenPath); err != nil {
					continue
				}
			}
			return nil
		}
	}
}

// waitForExistingServer polls an already-running server until it becomes healthy (G7).
func waitForExistingServer(port int) error {
	isTTY := colorEnabled()
	sp := newStartupProgress(os.Stderr, isTTY, isTTY)
	sp.step("Waiting for server to become ready...")

	healthURL := fmt.Sprintf("http://127.0.0.1:%d/health", port)
	client := &http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(60 * time.Second)

	for time.Now().Before(deadline) {
		time.Sleep(300 * time.Millisecond)
		resp, err := client.Get(healthURL)
		if err != nil {
			continue
		}
		resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			sp.done()
			fmt.Fprintf(os.Stderr, "AYB server is running (port %d).\n", port)
			return nil
		}
	}
	sp.fail()
	return fmt.Errorf("existing server (port %d) did not become ready within 60s", port)
}

// aybPIDPath returns the path to the AYB server PID file (~/.ayb/ayb.pid).
func aybPIDPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".ayb", "ayb.pid"), nil
}

// aybAdminTokenPath returns the path to the saved admin token (~/.ayb/admin-token).
func aybAdminTokenPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".ayb", "admin-token"), nil
}

// aybResetResultPath returns the path for the password reset result file (~/.ayb/.pw_reset_result).
func aybResetResultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".ayb", ".pw_reset_result"), nil
}

// readAYBPID reads the PID and port from the AYB PID file.
// Returns pid, port, error. Port may be 0 if the file uses the old format.
func readAYBPID() (int, int, error) {
	pidPath, err := aybPIDPath()
	if err != nil {
		return 0, 0, err
	}
	data, err := os.ReadFile(pidPath)
	if err != nil {
		return 0, 0, err
	}
	lines := strings.SplitN(strings.TrimSpace(string(data)), "\n", 2)
	if len(lines) == 0 || lines[0] == "" {
		return 0, 0, fmt.Errorf("empty pid file")
	}
	pid, err := strconv.Atoi(strings.TrimSpace(lines[0]))
	if err != nil {
		return 0, 0, fmt.Errorf("parsing pid: %w", err)
	}
	var port int
	if len(lines) > 1 && strings.TrimSpace(lines[1]) != "" {
		port, err = strconv.Atoi(strings.TrimSpace(lines[1]))
		if err != nil {
			return 0, 0, fmt.Errorf("parsing port: %w", err)
		}
	}
	return pid, port, nil
}

// buildChildArgs returns the arguments to pass when re-exec'ing as a background
// child. It takes os.Args[1:], strips any existing --foreground flags, and
// appends --foreground so the child runs in the foreground.
func buildChildArgs() []string {
	args := make([]string, 0, len(os.Args))
	for _, a := range os.Args[1:] {
		if a == "--foreground" || strings.HasPrefix(a, "--foreground=") {
			continue
		}
		args = append(args, a)
	}
	return append(args, "--foreground")
}

// cleanupServerFiles removes the PID and admin token files left by a previous run.
func cleanupServerFiles() {
	if pidPath, err := aybPIDPath(); err == nil {
		os.Remove(pidPath) //nolint:errcheck
	}
	if tokenPath, err := aybAdminTokenPath(); err == nil {
		os.Remove(tokenPath) //nolint:errcheck
	}
}

// isFirstRun returns true when AYB has never downloaded its embedded PostgreSQL.
func isFirstRun() bool {
	home, err := os.UserHomeDir()
	if err != nil {
		return true
	}
	_, err = os.Stat(filepath.Join(home, ".ayb", "pg", "postgres.txz"))
	return os.IsNotExist(err)
}

// portInUse returns true if the given port is already bound on the local machine.
func portInUse(port int) bool {
	if port <= 0 {
		return false
	}
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return true
	}
	ln.Close()
	return false
}

// portError wraps common listen errors with actionable suggestions.
func portError(port int, err error) error {
	if strings.Contains(err.Error(), "address already in use") {
		return fmt.Errorf("%s", ui.FormatError(
			fmt.Sprintf("port %d is already in use", port),
			fmt.Sprintf("ayb start --port %d   # use a different port", port+1),
			"ayb stop                # stop the running server",
		))
	}
	return err
}
