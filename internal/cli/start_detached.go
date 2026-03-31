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

type detachedStartPreparation struct {
	cfg               *config.Config
	generatedPassword string
	firstRun          bool
	timeout           time.Duration
}

var detachedAdminTokenPathFunc = aybAdminTokenPath

func runStartDetached(cmd *cobra.Command, _ []string) error {
	if handled, err := preflightDetachedStart(); handled || err != nil {
		return err
	}
	prep, err := prepareDetachedStart(cmd)
	if err != nil {
		return err
	}

	child, logPath, logFile, err := buildDetachedChildCommand()
	if err != nil {
		return err
	}
	if prep.generatedPassword != "" {
		child.Env = append(child.Env, "AYB_ADMIN_PASSWORD="+prep.generatedPassword)
	}
	if logFile != nil {
		defer logFile.Close()
	}

	isTTY := colorEnabled()
	sp := newStartupProgress(os.Stderr, isTTY, isTTY)
	sp.header(bannerVersion(buildVersion))

	if prep.firstRun {
		sp.step("Downloading PostgreSQL and starting server (first run)...")
	} else {
		sp.step("Starting server...")
	}

	if err := child.Start(); err != nil {
		sp.fail()
		return fmt.Errorf("starting server process: %w", err)
	}

	childDone := watchDetachedChildExit(child)
	if err := waitForDetachedStartReadiness(prep, child, logPath, childDone); err != nil {
		sp.fail()
		return err
	}
	sp.done()

	printDetachedStartBanner(prep.cfg, prep.generatedPassword, logPath, isTTY)
	return nil
}

func prepareDetachedStart(cmd *cobra.Command) (detachedStartPreparation, error) {
	cfg, generatedPassword, err := loadDetachedStartConfig(cmd)
	if err != nil {
		return detachedStartPreparation{}, err
	}
	if err := ensureDetachedStartPortAvailable(cfg); err != nil {
		return detachedStartPreparation{}, err
	}

	firstRun := isFirstRun()
	return detachedStartPreparation{
		cfg:               cfg,
		generatedPassword: generatedPassword,
		firstRun:          firstRun,
		timeout:           detachedStartTimeout(firstRun),
	}, nil
}

func loadDetachedStartConfig(cmd *cobra.Command) (*config.Config, string, error) {
	configPath, _ := cmd.Flags().GetString("config")
	cfg, err := config.Load(configPath, detachedStartConfigFlags(cmd))
	if err != nil {
		return nil, "", fmt.Errorf("loading config: %w", err)
	}

	generatedPassword, err := ensureConfiguredAdminPassword(cfg)
	if err != nil {
		return nil, "", err
	}
	return cfg, generatedPassword, nil
}

func detachedStartConfigFlags(cmd *cobra.Command) map[string]string {
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
	return flags
}

func ensureDetachedStartPortAvailable(cfg *config.Config) error {
	ln, err := net.Listen("tcp", cfg.Address())
	if err != nil {
		return portError(cfg.Server.Port, err)
	}
	ln.Close()
	return nil
}

func detachedStartTimeout(firstRun bool) time.Duration {
	if firstRun {
		return 300 * time.Second
	}
	return 60 * time.Second
}

func watchDetachedChildExit(child *exec.Cmd) <-chan struct{} {
	childDone := make(chan struct{})
	go func() {
		child.Wait()
		close(childDone)
	}()
	return childDone
}

func waitForDetachedStartReadiness(prep detachedStartPreparation, child *exec.Cmd, logPath string, childDone <-chan struct{}) error {
	needAdminToken := prep.cfg.Admin.Enabled && prep.generatedPassword != ""
	tokenPath := ""
	if needAdminToken {
		var err error
		tokenPath, err = detachedAdminTokenPathFunc()
		if err != nil {
			return fmt.Errorf("resolving admin token path: %w", err)
		}
	}
	return waitForDetachedReadiness(detachedReadinessPollOptions{
		healthURL:      fmt.Sprintf("http://127.0.0.1:%d/health", prep.cfg.Server.Port),
		timeout:        prep.timeout,
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
}

func printDetachedStartBanner(cfg *config.Config, generatedPassword, logPath string, isTTY bool) {
	embeddedPG := cfg.Database.URL == ""
	if isTTY {
		printBannerBodyTo(os.Stderr, cfg, embeddedPG, true, generatedPassword, logPath)
	} else {
		printBanner(cfg, embeddedPG, generatedPassword, logPath)
	}
	fmt.Fprintf(os.Stderr, "  %s\n\n", dim("Stop with: ayb stop", isTTY))
}

func preflightDetachedStart() (bool, error) {
	pid, port, err := readAYBPID()
	if err != nil {
		return false, nil
	}
	if port <= 0 {
		// Older pid files only stored the process id. Clean them up so the normal
		// port-availability checks can decide whether a new detached start is safe.
		cleanupServerFiles()
		return false, nil
	}

	proc, findErr := os.FindProcess(pid)
	if findErr != nil || proc.Signal(syscall.Signal(0)) != nil {
		// Stale PID file.
		cleanupServerFiles()
		return false, nil
	}

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
		logFile, err = os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil {
			return nil, "", nil, fmt.Errorf("opening log file: %w", err)
		}
		// Detached server logs can include operational details, so keep the file
		// owner-readable only even when reusing an existing path.
		if err := logFile.Chmod(0o600); err != nil {
			logFile.Close()
			return nil, "", nil, fmt.Errorf("hardening log file permissions: %w", err)
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

func waitForDetachedReadiness(opts detachedReadinessPollOptions) error {
	if opts.needAdminToken && opts.tokenPath == "" {
		return fmt.Errorf("admin token path is required")
	}

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
					if !os.IsNotExist(err) {
						return fmt.Errorf("checking admin token file: %w", err)
					}
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
