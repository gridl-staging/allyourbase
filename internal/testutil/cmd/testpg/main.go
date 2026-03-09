// testpg starts AYB's managed Postgres on a free port, sets TEST_DATABASE_URL,
// runs the given command (typically `go test ...`), then stops Postgres.
// This lets integration tests run without Docker or a local Postgres install.
//
// Usage: go run ./internal/testutil/cmd/testpg -- go test -tags=integration -count=1 ./...
package main

import (
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
)

func main() {
	os.Exit(run())
}

func run() int {
	args := os.Args[1:]
	if len(args) > 0 && args[0] == "--" {
		args = args[1:]
	}
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: testpg [--] <command> [args...]")
		return 1
	}

	port, err := freePort()
	if err != nil {
		fmt.Fprintf(os.Stderr, "testpg: finding free port: %v\n", err)
		return 1
	}

	// Reuse AYB's binary cache so we don't re-download on every run.
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "testpg: home dir: %v\n", err)
		return 1
	}
	cacheDir := filepath.Join(home, ".ayb", "pg")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "testpg: mkdir cache: %v\n", err)
		return 1
	}

	// Use a temp data dir — test data is throwaway.
	dataDir, err := os.MkdirTemp("", "ayb-test-pg-data-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "testpg: mkdir data: %v\n", err)
		return 1
	}
	defer os.RemoveAll(dataDir)

	runtimeDir, err := os.MkdirTemp("", "ayb-test-pg-run-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "testpg: mkdir runtime: %v\n", err)
		return 1
	}
	defer os.RemoveAll(runtimeDir)

	// Redirect PG logs to a temp file so they don't pollute test output.
	pgLogFile, err := os.CreateTemp("", "ayb-test-pg-log-*.log")
	if err != nil {
		fmt.Fprintf(os.Stderr, "testpg: create log file: %v\n", err)
		return 1
	}
	defer os.Remove(pgLogFile.Name())
	defer pgLogFile.Close()

	var pgLogger io.Writer = pgLogFile
	if os.Getenv("TESTPG_VERBOSE") != "" {
		// In verbose mode, tee PG logs to both file and stderr.
		pgLogger = io.MultiWriter(pgLogFile, os.Stderr)
	}

	db := embeddedpostgres.NewDatabase(embeddedpostgres.DefaultConfig().
		Port(uint32(port)).
		DataPath(dataDir).
		RuntimePath(runtimeDir).
		CachePath(cacheDir).
		Logger(pgLogger).
		Version(embeddedpostgres.V16).
		Username("test").
		Password("test").
		Database("postgres"))

	fmt.Fprintf(os.Stderr, "testpg: starting managed postgres on port %d (logs: %s)\n", port, pgLogFile.Name())
	if err := db.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "testpg: start postgres: %v\n", err)
		return 1
	}

	cleanup := func() {
		fmt.Fprintln(os.Stderr, "testpg: stopping managed postgres")
		_ = db.Stop()
	}
	defer cleanup()

	// Trap signals so postgres is stopped on Ctrl+C / SIGTERM instead of
	// being orphaned. A second signal force-exits immediately.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	url := fmt.Sprintf("postgresql://test:test@127.0.0.1:%d/postgres?sslmode=disable", port)
	fmt.Fprintf(os.Stderr, "testpg: TEST_DATABASE_URL=%s\n", url)

	cmd := exec.Command(args[0], args[1:]...) //nolint:gosec
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(), "TEST_DATABASE_URL="+url)

	// Run the child in its own process group so we can forward signals cleanly.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "testpg: %v\n", err)
		return 1
	}

	// Wait for either the child to finish or a signal to arrive.
	waitCh := make(chan error, 1)
	go func() { waitCh <- cmd.Wait() }()

	select {
	case err := <-waitCh:
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				return exitErr.ExitCode()
			}
			fmt.Fprintf(os.Stderr, "testpg: %v\n", err)
			return 1
		}
		return 0

	case sig := <-sigCh:
		fmt.Fprintf(os.Stderr, "\ntestpg: received %s, shutting down\n", sig)
		// Forward the signal to the child process group.
		if cmd.Process != nil {
			_ = syscall.Kill(-cmd.Process.Pid, sig.(syscall.Signal))
		}
		// Allow a second signal to force-exit immediately.
		go func() {
			<-sigCh
			fmt.Fprintln(os.Stderr, "testpg: forced exit")
			cleanup()
			os.Exit(1)
		}()
		// Wait for the child to exit after receiving the forwarded signal.
		<-waitCh
		// cleanup() runs via defer; return signal exit code.
		return 128 + int(sig.(syscall.Signal))
	}
}

func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}
