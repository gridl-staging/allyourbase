package cli

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/spf13/cobra"
)

func isolatedDetachedTestHome(t *testing.T) string {
	t.Helper()

	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	// Some CLI integration helpers invoke the Go toolchain while package tests are
	// still running. If those subprocesses inherit a temp HOME, Go may populate
	// read-only module-cache paths under that temp directory, which makes TempDir
	// cleanup flaky on macOS. Keep Go caches out of the temp HOME for these tests.
	cacheRoot := filepath.Join(os.TempDir(), "ayb-cli-test-go-cache")
	testutil.NoError(t, os.MkdirAll(filepath.Join(cacheRoot, "gocache"), 0o755))
	testutil.NoError(t, os.MkdirAll(filepath.Join(cacheRoot, "gomodcache"), 0o755))
	testutil.NoError(t, os.MkdirAll(filepath.Join(cacheRoot, "gopath"), 0o755))
	t.Setenv("GOCACHE", filepath.Join(cacheRoot, "gocache"))
	t.Setenv("GOMODCACHE", filepath.Join(cacheRoot, "gomodcache"))
	t.Setenv("GOPATH", filepath.Join(cacheRoot, "gopath"))

	return homeDir
}

func TestRunStartDetached_ExistingHealthyPIDShortCircuits(t *testing.T) {
	aybDir := filepath.Join(isolatedDetachedTestHome(t), ".ayb")
	testutil.NoError(t, os.MkdirAll(aybDir, 0o755))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)

	_, portString, err := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
	testutil.NoError(t, err)
	port, err := strconv.Atoi(portString)
	testutil.NoError(t, err)

	pidPath := filepath.Join(aybDir, "ayb.pid")
	pidContent := fmt.Sprintf("%d\n%d", os.Getpid(), port)
	testutil.NoError(t, os.WriteFile(pidPath, []byte(pidContent), 0o644))

	cmd := &cobra.Command{}
	stderr := captureStderr(t, func() {
		err = runStartDetached(cmd, nil)
	})
	testutil.NoError(t, err)
	testutil.Contains(t, stderr, "AYB server is already running")

	_, statErr := os.Stat(pidPath)
	testutil.NoError(t, statErr)
}

func TestRunStartDetached_StalePIDCleansFilesBeforeEarlyExit(t *testing.T) {
	aybDir := filepath.Join(isolatedDetachedTestHome(t), ".ayb")
	testutil.NoError(t, os.MkdirAll(aybDir, 0o755))

	pidPath := filepath.Join(aybDir, "ayb.pid")
	tokenPath := filepath.Join(aybDir, "admin-token")
	testutil.NoError(t, os.WriteFile(pidPath, []byte("999999\n0"), 0o644))
	testutil.NoError(t, os.WriteFile(tokenPath, []byte("token"), 0o600))

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	testutil.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })
	port := ln.Addr().(*net.TCPAddr).Port

	cmd := &cobra.Command{}
	cmd.Flags().Int("port", 0, "")
	testutil.NoError(t, cmd.Flags().Set("port", strconv.Itoa(port)))

	err = runStartDetached(cmd, nil)
	testutil.NotNil(t, err)
	testutil.True(
		t,
		strings.Contains(err.Error(), "already in use") ||
			strings.Contains(err.Error(), "server exited during startup"),
	)

	_, pidErr := os.Stat(pidPath)
	_, tokenErr := os.Stat(tokenPath)
	testutil.True(t, os.IsNotExist(pidErr))
	testutil.True(t, os.IsNotExist(tokenErr))
}

func TestRunStartDetachedReadiness_WaitsForAdminTokenWhenPasswordMissing(t *testing.T) {
	tokenPath := filepath.Join(t.TempDir(), "admin-token")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	childDone := make(chan struct{})
	terminated := false

	go func() {
		time.Sleep(80 * time.Millisecond)
		_ = os.WriteFile(tokenPath, []byte("generated-secret"), 0o600)
	}()

	started := time.Now()
	err := waitForDetachedReadiness(detachedReadinessPollOptions{
		healthURL:      srv.URL + "/health",
		timeout:        400 * time.Millisecond,
		pollInterval:   10 * time.Millisecond,
		needAdminToken: true,
		tokenPath:      tokenPath,
		logPath:        "/tmp/detached.log",
		childDone:      childDone,
		httpClient:     &http.Client{Timeout: 50 * time.Millisecond},
		terminateChild: func() { terminated = true },
	})
	elapsed := time.Since(started)

	testutil.NoError(t, err)
	testutil.False(t, terminated)
	testutil.True(t, elapsed >= 60*time.Millisecond)
}
