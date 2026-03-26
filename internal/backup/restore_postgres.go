// Package backup provides utilities for managing temporary Postgres instances during point-in-time recovery operations.
package backup

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os/exec"
	"time"

	"github.com/jackc/pgx/v5"
)

// RecoveryInstance manages a temporary Postgres process used for PITR replay.
type RecoveryInstance struct {
	dataDir string
	port    int
	logger  *slog.Logger

	runCommand      func(ctx context.Context, name string, args ...string) error
	queryRecoveryFn func(ctx context.Context, connURL string) (bool, error)
	waitTimeout     time.Duration
	pollInterval    time.Duration
}

// NewRecoveryInstance creates a new RecoveryInstance configured for the given data directory and port, using the provided logger or the default if nil.
func NewRecoveryInstance(dataDir string, port int, logger *slog.Logger) *RecoveryInstance {
	if logger == nil {
		logger = slog.Default()
	}
	return &RecoveryInstance{
		dataDir: dataDir,
		port:    port,
		logger:  logger,
		runCommand: func(ctx context.Context, name string, args ...string) error {
			cmd := exec.CommandContext(ctx, name, args...)
			out, err := cmd.CombinedOutput()
			if err != nil {
				return fmt.Errorf("command %s %v failed: %w (output: %s)", name, args, err, string(out))
			}
			return nil
		},
		queryRecoveryFn: queryPGIsInRecovery,
		waitTimeout:     5 * time.Minute,
		pollInterval:    time.Second,
	}
}

func (r *RecoveryInstance) Start(ctx context.Context) error {
	args := []string{"start", "-D", r.dataDir, "-o", fmt.Sprintf("-p %d", r.port), "-w", "-t", "300"}
	if err := r.runCommand(ctx, "pg_ctl", args...); err != nil {
		return fmt.Errorf("starting recovery postgres instance: %w", err)
	}
	return nil
}

func (r *RecoveryInstance) Stop(ctx context.Context) error {
	args := []string{"stop", "-D", r.dataDir, "-m", "fast"}
	if err := r.runCommand(ctx, "pg_ctl", args...); err != nil {
		return fmt.Errorf("stopping recovery postgres instance: %w", err)
	}
	return nil
}

// WaitForRecovery polls until the instance completes recovery or the timeout is exceeded, returning nil on success or an error on timeout.
func (r *RecoveryInstance) WaitForRecovery(ctx context.Context) error {
	if r.waitTimeout <= 0 {
		r.waitTimeout = 5 * time.Minute
	}
	if r.pollInterval <= 0 {
		r.pollInterval = time.Second
	}

	waitCtx, cancel := context.WithTimeout(ctx, r.waitTimeout)
	defer cancel()
	ticker := time.NewTicker(r.pollInterval)
	defer ticker.Stop()

	for {
		inRecovery, err := r.queryRecoveryFn(waitCtx, r.ConnURL())
		if err == nil && !inRecovery {
			return nil
		}
		if err != nil {
			r.logger.Debug("wait_for_recovery probe failed", "error", err)
		}

		select {
		case <-waitCtx.Done():
			return fmt.Errorf("timed out waiting for recovery completion: %w", waitCtx.Err())
		case <-ticker.C:
		}
	}
}

func (r *RecoveryInstance) ConnURL() string {
	return fmt.Sprintf("postgresql://localhost:%d/postgres", r.port)
}

func queryPGIsInRecovery(ctx context.Context, connURL string) (bool, error) {
	conn, err := pgx.Connect(ctx, connURL)
	if err != nil {
		return false, fmt.Errorf("connecting to recovery instance: %w", err)
	}
	defer conn.Close(ctx)

	var inRecovery bool
	if err := conn.QueryRow(ctx, "SELECT pg_is_in_recovery()").Scan(&inRecovery); err != nil {
		return false, fmt.Errorf("querying pg_is_in_recovery: %w", err)
	}
	return inRecovery, nil
}

func FindFreePort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("allocating free port: %w", err)
	}
	defer ln.Close()

	addr, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		return 0, fmt.Errorf("unexpected listener address type %T", ln.Addr())
	}
	return addr.Port, nil
}
