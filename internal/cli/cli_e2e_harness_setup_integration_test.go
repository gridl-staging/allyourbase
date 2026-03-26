//go:build integration

package cli

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/branching"
	"github.com/allyourbase/ayb/internal/sqlutil"
	"github.com/jackc/pgx/v5/pgxpool"
)

const cliE2EHarnessPort = 18092

type cliE2EHarnessResources struct {
	adminPool    *pgxpool.Pool
	databaseName string
	binDir       string
	binPath      string
	homeDir      string
	storageDir   string
	serverCmd    *exec.Cmd
	serverExit   <-chan error
	serverDone   <-chan struct{}
}

func bootstrapCLIHarness(ctx context.Context, sharedDatabaseURL string) (cleanup func(), err error) {
	testDatabaseURL := os.Getenv("TEST_DATABASE_URL")
	e2eDatabaseName := fmt.Sprintf("cli_e2e_%d", time.Now().UnixNano())
	e2eDatabaseURL, _, err := deriveHarnessDatabaseConfig(testDatabaseURL, sharedDatabaseURL, e2eDatabaseName)
	if err != nil {
		return nil, err
	}

	resources, err := newCLIHarnessResources(ctx, testDatabaseURL, e2eDatabaseName)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			resources.cleanup()
		}
	}()

	if err = resources.prepareBinary(); err != nil {
		return nil, err
	}
	if err = resources.prepareHomeDir(); err != nil {
		return nil, err
	}
	serverOutput, err := resources.startServer(e2eDatabaseURL)
	if err != nil {
		return nil, err
	}
	if err = waitForHarnessReadiness(resources.serverCmd, resources.serverExit, resources.serverDone, serverOutput); err != nil {
		return nil, err
	}
	bearerToken, err := readHarnessAdminToken(resources.homeDir)
	if err != nil {
		return nil, err
	}

	cliE2EHarnessBinaryPath = resources.binPath
	cliE2EHarnessBearerToken = bearerToken
	cliE2EHarnessDatabaseURL = e2eDatabaseURL
	cliE2EHarnessDatabaseName = e2eDatabaseName

	return resources.cleanup, nil
}

func newCLIHarnessResources(ctx context.Context, testDatabaseURL, databaseName string) (*cliE2EHarnessResources, error) {
	adminPool, err := pgxpool.New(ctx, testDatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("connecting to TEST_DATABASE_URL: %w", err)
	}
	if _, err := adminPool.Exec(ctx, "CREATE DATABASE "+sqlutil.QuoteIdent(databaseName)); err != nil {
		adminPool.Close()
		return nil, fmt.Errorf("creating harness database %q: %w", databaseName, err)
	}
	return &cliE2EHarnessResources{
		adminPool:    adminPool,
		databaseName: databaseName,
	}, nil
}

func (r *cliE2EHarnessResources) prepareBinary() error {
	binDir, err := os.MkdirTemp("", "ayb-cli-e2e-bin-*")
	if err != nil {
		return fmt.Errorf("creating harness binary directory: %w", err)
	}
	binPath, err := buildBinaryAtDir(binDir)
	if err != nil {
		_ = os.RemoveAll(binDir)
		return err
	}
	r.binDir = binDir
	r.binPath = binPath
	return nil
}

func (r *cliE2EHarnessResources) prepareHomeDir() error {
	homeDir, err := os.MkdirTemp("", "ayb-cli-e2e-home-*")
	if err != nil {
		return fmt.Errorf("creating harness HOME directory: %w", err)
	}
	r.homeDir = homeDir

	// Storage directory for sites deploy file uploads.
	storageDir := filepath.Join(homeDir, "storage")
	if err := os.MkdirAll(storageDir, 0o755); err != nil {
		return fmt.Errorf("creating harness storage directory: %w", err)
	}
	r.storageDir = storageDir
	return nil
}

func (r *cliE2EHarnessResources) startServer(databaseURL string) (*bytes.Buffer, error) {
	serverOutput := &bytes.Buffer{}
	serverCmd := exec.Command(
		r.binPath,
		"start",
		"--foreground",
		"--port", fmt.Sprintf("%d", cliE2EHarnessPort),
		"--database-url", databaseURL,
	)
	serverCmd.Stdout = serverOutput
	serverCmd.Stderr = serverOutput
	serverCmd.Env = append(os.Environ(),
		"HOME="+r.homeDir,
		"AYB_STORAGE_ENABLED=true",
		"AYB_STORAGE_LOCAL_PATH="+r.storageDir,
	)
	if err := serverCmd.Start(); err != nil {
		return nil, fmt.Errorf("starting harness ayb server: %w", err)
	}

	r.serverCmd = serverCmd
	r.serverExit, r.serverDone = observeProcessExit(serverCmd)
	return serverOutput, nil
}

func observeProcessExit(cmd *exec.Cmd) (<-chan error, <-chan struct{}) {
	serverExit := make(chan error, 1)
	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		serverExit <- cmd.Wait()
	}()
	return serverExit, serverDone
}

func (r *cliE2EHarnessResources) cleanup() {
	stopHarnessServer(r.serverCmd, r.serverDone)
	if r.adminPool != nil {
		if _, err := r.adminPool.Exec(context.Background(), "DROP DATABASE IF EXISTS "+sqlutil.QuoteIdent(r.databaseName)+" WITH (FORCE)"); err != nil {
			fmt.Fprintf(os.Stderr, "harness cleanup: dropping database %q: %v\n", r.databaseName, err)
		}
		r.adminPool.Close()
	}
	if r.homeDir != "" {
		_ = os.RemoveAll(r.homeDir)
	}
	if r.binDir != "" {
		_ = os.RemoveAll(r.binDir)
	}
}

func waitForHarnessReadiness(serverCmd *exec.Cmd, serverExit <-chan error, serverDone <-chan struct{}, serverOutput *bytes.Buffer) error {
	healthReady := make(chan bool, 1)
	go func() {
		healthReady <- waitForHealthPort(cliE2EHarnessPort, 60*time.Second)
	}()

	select {
	case err := <-serverExit:
		return fmt.Errorf("harness server exited before /health became ready: %v\n%s", err, strings.TrimSpace(serverOutput.String()))
	case ready := <-healthReady:
		if ready {
			return nil
		}
		stopHarnessServer(serverCmd, serverDone)
		return fmt.Errorf("harness server failed readiness on /health: %s", strings.TrimSpace(serverOutput.String()))
	}
}

func readHarnessAdminToken(homeDir string) (string, error) {
	tokenPath := filepath.Join(homeDir, ".ayb", "admin-token")
	deadline := time.Now().Add(10 * time.Second)
	for {
		tokenBytes, err := os.ReadFile(tokenPath)
		if err == nil {
			token := strings.TrimSpace(string(tokenBytes))
			if token != "" {
				return token, nil
			}
		} else if !os.IsNotExist(err) {
			return "", fmt.Errorf("reading harness admin-token: %w", err)
		}
		if time.Now().After(deadline) {
			return "", fmt.Errorf("reading harness admin-token: timeout waiting for non-empty %s", tokenPath)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func stopHarnessServer(serverCmd *exec.Cmd, serverDone <-chan struct{}) {
	if serverCmd == nil || serverCmd.Process == nil {
		return
	}

	select {
	case <-serverDone:
		return
	default:
	}

	_ = serverCmd.Process.Signal(os.Interrupt)
	select {
	case <-serverDone:
		return
	case <-time.After(10 * time.Second):
	}

	_ = serverCmd.Process.Kill()
	select {
	case <-serverDone:
	case <-time.After(2 * time.Second):
	}
}

func deriveHarnessDatabaseConfig(testDatabaseURL, sharedDatabaseURL, harnessDatabaseName string) (string, string, error) {
	if strings.TrimSpace(testDatabaseURL) == "" {
		return "", "", fmt.Errorf("TEST_DATABASE_URL is required for CLI E2E harness")
	}
	if strings.TrimSpace(harnessDatabaseName) == "" {
		return "", "", fmt.Errorf("harness database name cannot be empty")
	}

	harnessDatabaseURL, err := branching.ReplaceDatabaseInURL(testDatabaseURL, harnessDatabaseName)
	if err != nil {
		return "", "", fmt.Errorf("building harness database URL: %w", err)
	}

	sharedDatabaseName, err := branching.ExtractDBNameFromURL(sharedDatabaseURL)
	if err != nil {
		return "", "", fmt.Errorf("parsing sharedPG database URL: %w", err)
	}
	derivedDatabaseName, err := branching.ExtractDBNameFromURL(harnessDatabaseURL)
	if err != nil {
		return "", "", fmt.Errorf("parsing harness database URL: %w", err)
	}
	if sharedDatabaseName == derivedDatabaseName {
		return "", "", fmt.Errorf("harness database %q must differ from sharedPG database %q", derivedDatabaseName, sharedDatabaseName)
	}

	return harnessDatabaseURL, derivedDatabaseName, nil
}

func TestCLI_E2E_HarnessStopServer_ReturnsAfterExitAlreadyObserved(t *testing.T) {
	cmd := exec.Command("bash", "-lc", "exit 0")
	if err := cmd.Start(); err != nil {
		t.Fatalf("starting short-lived process: %v", err)
	}

	serverExit, serverDone := observeProcessExit(cmd)

	if err := <-serverExit; err != nil {
		t.Fatalf("waiting for short-lived process: %v", err)
	}

	done := make(chan struct{})
	go func() {
		stopHarnessServer(cmd, serverDone)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("stopHarnessServer blocked after the process exit was already observed")
	}
}

func TestCLI_E2E_ReadHarnessAdminToken_RetriesUntilNonEmpty(t *testing.T) {
	homeDir := t.TempDir()
	tokenDir := filepath.Join(homeDir, ".ayb")
	if err := os.MkdirAll(tokenDir, 0o755); err != nil {
		t.Fatalf("creating token directory: %v", err)
	}
	tokenPath := filepath.Join(tokenDir, "admin-token")
	if err := os.WriteFile(tokenPath, []byte(""), 0o600); err != nil {
		t.Fatalf("creating empty admin-token: %v", err)
	}

	go func() {
		time.Sleep(250 * time.Millisecond)
		_ = os.WriteFile(tokenPath, []byte("stage-admin-token\n"), 0o600)
	}()

	token, err := readHarnessAdminToken(homeDir)
	if err != nil {
		t.Fatalf("reading admin token after delayed write: %v", err)
	}
	if token != "stage-admin-token" {
		t.Fatalf("expected stage-admin-token, got %q", token)
	}
}
