//go:build integration

package pgmanager

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/ulikunitz/xz"
)

// testPGHarness creates a minimal PG binary tarball from the locally installed
// postgres, serves it via httptest.Server with a SHA256SUMS endpoint, and
// returns a pgmanager.Config pointing at the test server.
type testPGHarness struct {
	server  *httptest.Server
	config  Config
	tempDir string
}

const managedPGStartTimeoutEnvVar = "AYB_PGMANAGER_TEST_START_TIMEOUT"

func managedPGStartTimeout() time.Duration {
	timeoutRaw := strings.TrimSpace(os.Getenv(managedPGStartTimeoutEnvVar))
	if timeoutRaw == "" {
		return 5 * time.Minute
	}

	timeout, err := time.ParseDuration(timeoutRaw)
	if err != nil || timeout <= 0 {
		return 5 * time.Minute
	}

	return timeout
}

func newTestHarnessServer(archive []byte, sums string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/SHA256SUMS"):
			w.Write([]byte(sums))
		default:
			w.Write(archive)
		}
	}))
}

// newTestPGHarness packages the system's PG binaries into a test fixture tarball,
// serves it via HTTP, and returns a configured harness.
func newTestPGHarness(t *testing.T) *testPGHarness {
	t.Helper()

	// Find pg_config to locate system PG installation.
	pgConfig, err := exec.LookPath("pg_config")
	if err != nil {
		t.Skip("pg_config not found — install PostgreSQL to run integration tests")
	}

	binDirOut, err := exec.Command(pgConfig, "--bindir").Output()
	if err != nil {
		t.Fatalf("pg_config --bindir failed: %v", err)
	}
	libDirOut, err := exec.Command(pgConfig, "--pkglibdir").Output()
	if err != nil {
		t.Fatalf("pg_config --pkglibdir failed: %v", err)
	}
	shareDirOut, err := exec.Command(pgConfig, "--sharedir").Output()
	if err != nil {
		t.Fatalf("pg_config --sharedir failed: %v", err)
	}

	binDir := trimNL(string(binDirOut))
	libDir := trimNL(string(libDirOut))
	shareDir := trimNL(string(shareDirOut))

	// Build a minimal tar.xz archive.
	archive := buildTestArchive(t, binDir, libDir, shareDir)

	// Compute SHA256.
	h := sha256.Sum256(archive)
	hash := fmt.Sprintf("%x", h)

	platform, err := platformKey()
	if err != nil {
		t.Skipf("unsupported test platform: %v", err)
	}
	version := "16"
	archiveName := fmt.Sprintf("ayb-postgres-%s-%s.tar.xz", version, platform)

	sums := fmt.Sprintf("%s  %s\n", hash, archiveName)

	srv := newTestHarnessServer(archive, sums)

	tempDir := t.TempDir()

	cfg := Config{
		Port:                   findFreePort(t),
		DataDir:                filepath.Join(tempDir, "data"),
		RuntimeDir:             filepath.Join(tempDir, "run"),
		BinCacheDir:            filepath.Join(tempDir, "cache"),
		BinDir:                 filepath.Join(tempDir, "bindir"),
		BinaryURL:              srv.URL + "/{version}/{platform}.tar.xz",
		PGVersion:              version,
		Extensions:             nil, // extensions tested separately
		SharedPreloadLibraries: nil,
		Logger:                 testutil.DiscardLogger(),
	}

	t.Cleanup(func() {
		srv.Close()
	})

	return &testPGHarness{
		server:  srv,
		config:  cfg,
		tempDir: tempDir,
	}
}

// buildTestArchive creates a .tar.xz archive from system PG binaries.
func buildTestArchive(t *testing.T, binDir, libDir, shareDir string) []byte {
	t.Helper()

	var buf bytes.Buffer
	xzw, err := xz.NewWriter(&buf)
	testutil.NoError(t, err)
	tw := tar.NewWriter(xzw)

	prefix := "ayb-postgres-16/"

	// Add essential binaries.
	for _, name := range []string{"postgres", "initdb", "pg_ctl", "pg_isready"} {
		addFileToTar(t, tw, filepath.Join(binDir, name), prefix+"bin/"+name)
	}

	// Add lib directory (shared libraries).
	addDirToTar(t, tw, libDir, prefix+"lib/")

	// Add share directory (timezone data, SQL files, etc.).
	addDirToTar(t, tw, shareDir, prefix+"share/")

	// Add PG_VERSION.
	testutil.NoError(t, tw.WriteHeader(&tar.Header{
		Name: prefix + "PG_VERSION",
		Mode: 0o644,
		Size: 3,
	}))
	tw.Write([]byte("16\n"))

	testutil.NoError(t, tw.Close())
	testutil.NoError(t, xzw.Close())
	return buf.Bytes()
}

// addFileToTar adds a single file to the tar archive.
func addFileToTar(t *testing.T, tw *tar.Writer, src, name string) {
	t.Helper()
	info, err := os.Stat(src)
	if err != nil {
		t.Logf("skipping %s: %v", src, err)
		return
	}
	hdr, err := tar.FileInfoHeader(info, "")
	testutil.NoError(t, err)
	hdr.Name = name

	testutil.NoError(t, tw.WriteHeader(hdr))
	f, err := os.Open(src)
	testutil.NoError(t, err)
	defer f.Close()
	io.Copy(tw, f)
}

// addDirToTar recursively adds a directory to the tar archive.
func addDirToTar(t *testing.T, tw *tar.Writer, dir, prefix string) {
	t.Helper()
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip errors
		}
		rel, _ := filepath.Rel(dir, path)
		name := prefix + rel

		if info.IsDir() {
			tw.WriteHeader(&tar.Header{
				Name:     name + "/",
				Typeflag: tar.TypeDir,
				Mode:     0o755,
			})
			return nil
		}

		if !info.Mode().IsRegular() {
			return nil // skip symlinks etc for simplicity
		}

		hdr, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return nil
		}
		hdr.Name = name
		tw.WriteHeader(hdr)

		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer f.Close()
		io.Copy(tw, f)
		return nil
	})
}

// findFreePort finds an available TCP port by binding to :0 and reading the
// assigned port. The port is released before returning, so there is a small
// TOCTOU window, but this is acceptable for test use.
func findFreePort(t *testing.T) uint32 {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("findFreePort: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	return uint32(port)
}

func trimNL(s string) string {
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	return s
}

func TestManagedPGStartTimeoutDefault(t *testing.T) {
	t.Setenv(managedPGStartTimeoutEnvVar, "")
	testutil.Equal(t, 5*time.Minute, managedPGStartTimeout())
}

func TestManagedPGStartTimeoutFromEnv(t *testing.T) {
	t.Setenv(managedPGStartTimeoutEnvVar, "7m30s")
	testutil.Equal(t, 7*time.Minute+30*time.Second, managedPGStartTimeout())
}

func TestManagedPGStartTimeoutInvalidEnvFallsBack(t *testing.T) {
	t.Setenv(managedPGStartTimeoutEnvVar, "invalid")
	testutil.Equal(t, 5*time.Minute, managedPGStartTimeout())
}

func TestManagedPGStartTimeoutNonPositiveFallsBack(t *testing.T) {
	t.Setenv(managedPGStartTimeoutEnvVar, "0s")
	testutil.Equal(t, 5*time.Minute, managedPGStartTimeout())
}

func TestHarnessServesVersionedSHA256SumsPath(t *testing.T) {
	t.Parallel()

	archive := []byte("fake-archive-bytes")
	sums := "abc123  ayb-postgres-16-darwin-arm64.tar.xz\n"
	srv := newTestHarnessServer(archive, sums)
	t.Cleanup(srv.Close)

	url := sha256SumsURL(srv.URL+"/{version}/{platform}.tar.xz", "16")
	resp, err := http.Get(url) //nolint:noctx
	testutil.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	testutil.NoError(t, err)
	testutil.Equal(t, sums, string(body))
}

// --- Integration Tests ---

func TestFullLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	harness := newTestPGHarness(t)
	mgr := New(harness.config)

	ctx, cancel := context.WithTimeout(context.Background(), managedPGStartTimeout())
	defer cancel()

	// Start.
	connURL, err := mgr.Start(ctx)
	testutil.NoError(t, err)
	testutil.True(t, connURL != "", "expected non-empty connection URL")
	testutil.True(t, mgr.IsRunning(), "manager should be running")

	// SELECT 1.
	db, err := sql.Open("pgx", connURL)
	testutil.NoError(t, err)
	defer db.Close()

	var result int
	err = db.QueryRowContext(ctx, "SELECT 1").Scan(&result)
	testutil.NoError(t, err)
	testutil.Equal(t, 1, result)

	// Create a test table and insert data.
	_, err = db.ExecContext(ctx, "CREATE TABLE test_persist (id serial PRIMARY KEY, val text)")
	testutil.NoError(t, err)
	_, err = db.ExecContext(ctx, "INSERT INTO test_persist (val) VALUES ('hello')")
	testutil.NoError(t, err)
	db.Close()

	// Stop.
	err = mgr.Stop()
	testutil.NoError(t, err)
	testutil.False(t, mgr.IsRunning(), "manager should not be running after stop")

	// Restart with existing data.
	mgr2 := New(harness.config)
	connURL2, err := mgr2.Start(ctx)
	testutil.NoError(t, err)
	testutil.Equal(t, connURL, connURL2)

	db2, err := sql.Open("pgx", connURL2)
	testutil.NoError(t, err)
	defer db2.Close()

	// Verify data persisted.
	var val string
	err = db2.QueryRowContext(ctx, "SELECT val FROM test_persist WHERE id = 1").Scan(&val)
	testutil.NoError(t, err)
	testutil.Equal(t, "hello", val)

	// Final stop.
	err = mgr2.Stop()
	testutil.NoError(t, err)
}

func TestExtensionInit(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	harness := newTestPGHarness(t)
	harness.config.Extensions = []string{"pgvector"}
	harness.config.Port = findFreePort(t)
	harness.config.DataDir = filepath.Join(harness.tempDir, "data-ext")

	mgr := New(harness.config)
	ctx, cancel := context.WithTimeout(context.Background(), managedPGStartTimeout())
	defer cancel()

	connURL, err := mgr.Start(ctx)
	testutil.NoError(t, err)
	defer mgr.Stop()

	db, err := sql.Open("pgx", connURL)
	testutil.NoError(t, err)
	defer db.Close()

	// Verify pgvector type exists.
	var typname string
	err = db.QueryRowContext(ctx, "SELECT typname FROM pg_type WHERE typname = 'vector'").Scan(&typname)
	if err != nil {
		t.Logf("pgvector not available on this system — this is expected if pgvector is not installed: %v", err)
		t.Skip("pgvector extension not available")
	}
	testutil.Equal(t, "vector", typname)
}

func TestPortAlreadyInUse(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	harness := newTestPGHarness(t)

	// Start the first manager — this claims the port.
	mgr1 := New(harness.config)
	ctx, cancel := context.WithTimeout(context.Background(), managedPGStartTimeout())
	defer cancel()

	_, err := mgr1.Start(ctx)
	testutil.NoError(t, err)
	defer mgr1.Stop()

	// Start a second manager on the same port — should fail.
	cfg2 := harness.config
	cfg2.DataDir = filepath.Join(harness.tempDir, "data-conflict")

	mgr2 := New(cfg2)
	_, err = mgr2.Start(ctx)
	if err == nil {
		mgr2.Stop()
		t.Fatal("expected error when starting on an already-used port, got nil")
	}
	testutil.Contains(t, err.Error(), "pg_ctl start failed")
}
