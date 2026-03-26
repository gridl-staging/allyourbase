// testpg starts AYB's managed Postgres on a free port, sets TEST_DATABASE_URL,
// runs the given command (typically `go test ...`), then stops Postgres.
// This lets integration tests run without Docker or a local Postgres install.
//
// Usage: go run ./internal/testutil/cmd/testpg -- go test -tags=integration -count=1 ./...
// Package main testpg starts AYB's managed Postgres on a free port and runs a given command with TEST_DATABASE_URL set, allowing integration tests to run without Docker or a local Postgres install.
package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/allyourbase/ayb/internal/pgmanager"
)

func main() {
	os.Exit(run())
}

// run starts AYB's managed Postgres on a free port, sets TEST_DATABASE_URL, executes the given command with that environment, and gracefully stops Postgres on command completion or signal. It returns the command's exit code, or a non-zero code if startup fails.
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

	tempRoot, err := createTempRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "testpg: create temp postgres root: %v\n", err)
		return 1
	}
	defer os.RemoveAll(tempRoot)

	pgLogFile, logWriter, err := openPostgresLog()
	if err != nil {
		fmt.Fprintf(os.Stderr, "testpg: create log file: %v\n", err)
		return 1
	}
	defer os.Remove(pgLogFile.Name())
	defer pgLogFile.Close()

	logger := slog.New(slog.NewTextHandler(logWriter, &slog.HandlerOptions{Level: slog.LevelDebug}))

	mgr := pgmanager.New(newTestPGConfig(tempRoot, port, logger))

	ctx := context.Background()

	fmt.Fprintf(os.Stderr, "testpg: starting managed postgres on port %d (logs: %s)\n", port, pgLogFile.Name())
	connURL, err := mgr.Start(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "testpg: start postgres: %v\n", err)
		return 1
	}
	testDBURL, err := replaceDatabaseInConnURL(connURL, "postgres")
	if err != nil {
		fmt.Fprintf(os.Stderr, "testpg: build TEST_DATABASE_URL: %v\n", err)
		return 1
	}

	cleanup := func() {
		fmt.Fprintln(os.Stderr, "testpg: stopping managed postgres")
		_ = mgr.Stop()
	}
	defer cleanup()

	// Trap signals so postgres is stopped on Ctrl+C / SIGTERM instead of
	// being orphaned. A second signal force-exits immediately.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	fmt.Fprintf(os.Stderr, "testpg: TEST_DATABASE_URL=%s\n", testDBURL)

	cmd := newChildCommand(args, testDBURL)
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

func createTempRoot() (string, error) {
	return os.MkdirTemp("", "ayb-testpg-*")
}

func openPostgresLog() (*os.File, io.Writer, error) {
	pgLogFile, err := os.CreateTemp("", "ayb-test-pg-log-*.log")
	if err != nil {
		return nil, nil, err
	}

	logWriter := io.Writer(pgLogFile)
	if os.Getenv("TESTPG_VERBOSE") != "" {
		logWriter = io.MultiWriter(pgLogFile, os.Stderr)
	}
	return pgLogFile, logWriter, nil
}

func newChildCommand(args []string, connURL string) *exec.Cmd {
	cmd := exec.Command(args[0], args[1:]...) //nolint:gosec
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(), "TEST_DATABASE_URL="+connURL)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return cmd
}

func newTestPGConfig(root string, port int, logger *slog.Logger) pgmanager.Config {
	return pgmanager.Config{
		Port:       uint32(port),
		DataDir:    filepath.Join(root, "data"),
		RuntimeDir: filepath.Join(root, "run"),
		PIDFile:    filepath.Join(root, "pg.pid"),
		Logger:     logger,
	}
}

func replaceDatabaseInConnURL(connURL, databaseName string) (string, error) {
	parsedURL, err := url.Parse(connURL)
	if err != nil {
		return "", err
	}
	parsedURL.Path = "/" + databaseName
	return parsedURL.String(), nil
}
