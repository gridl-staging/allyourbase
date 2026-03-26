package pgmanager

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/ulikunitz/xz"
)

// makeTarXZ creates a small .tar.xz in memory with the given file entries.
// Each entry is path -> content.
func makeTarXZ(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	xzw, err := xz.NewWriter(&buf)
	testutil.NoError(t, err)

	tw := tar.NewWriter(xzw)
	for name, content := range files {
		testutil.NoError(t, tw.WriteHeader(&tar.Header{
			Name: name,
			Mode: 0o755,
			Size: int64(len(content)),
		}))
		_, err := tw.Write([]byte(content))
		testutil.NoError(t, err)
	}
	testutil.NoError(t, tw.Close())
	testutil.NoError(t, xzw.Close())
	return buf.Bytes()
}

func makeTarXZWithHeaders(t *testing.T, headers []*tar.Header, data map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	xzw, err := xz.NewWriter(&buf)
	testutil.NoError(t, err)

	tw := tar.NewWriter(xzw)
	for _, hdr := range headers {
		testutil.NoError(t, tw.WriteHeader(hdr))
		if hdr.Typeflag == tar.TypeReg && hdr.Size > 0 {
			_, err := tw.Write([]byte(data[hdr.Name]))
			testutil.NoError(t, err)
		}
	}
	testutil.NoError(t, tw.Close())
	testutil.NoError(t, xzw.Close())
	return buf.Bytes()
}

func TestExtractTarXZ(t *testing.T) {
	t.Parallel()
	// Simulate a tarball with top-level prefix: ayb-postgres-16/bin/postgres
	archive := makeTarXZ(t, map[string]string{
		"ayb-postgres-16/bin/postgres": "#!/bin/sh\necho pg",
		"ayb-postgres-16/bin/initdb":   "#!/bin/sh\necho initdb",
		"ayb-postgres-16/PG_VERSION":   "16",
	})

	archivePath := filepath.Join(t.TempDir(), "test.tar.xz")
	testutil.NoError(t, os.WriteFile(archivePath, archive, 0o644))

	destDir := filepath.Join(t.TempDir(), "extracted")
	testutil.NoError(t, os.MkdirAll(destDir, 0o755))

	err := extractTarXZ(archivePath, destDir)
	testutil.NoError(t, err)

	// Should strip top-level prefix — bin/postgres should be directly under destDir.
	pgBin := filepath.Join(destDir, "bin", "postgres")
	content, err := os.ReadFile(pgBin)
	testutil.NoError(t, err)
	testutil.Equal(t, "#!/bin/sh\necho pg", string(content))

	// PG_VERSION at root.
	ver, err := os.ReadFile(filepath.Join(destDir, "PG_VERSION"))
	testutil.NoError(t, err)
	testutil.Equal(t, "16", string(ver))
}

func TestExtractTarXZNoPrefix(t *testing.T) {
	t.Parallel()
	// Tarball without a top-level prefix dir.
	archive := makeTarXZ(t, map[string]string{
		"bin/postgres": "pg-binary",
		"PG_VERSION":   "16",
	})

	archivePath := filepath.Join(t.TempDir(), "nopfx.tar.xz")
	testutil.NoError(t, os.WriteFile(archivePath, archive, 0o644))

	destDir := filepath.Join(t.TempDir(), "extracted")
	testutil.NoError(t, os.MkdirAll(destDir, 0o755))

	err := extractTarXZ(archivePath, destDir)
	testutil.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(destDir, "bin", "postgres"))
	testutil.NoError(t, err)
	testutil.Equal(t, "pg-binary", string(content))
}

func TestExtractTarXZRejectsEscapingSymlinkTarget(t *testing.T) {
	t.Parallel()
	archive := makeTarXZWithHeaders(t, []*tar.Header{
		{Name: "ayb-postgres-16/bin/", Typeflag: tar.TypeDir, Mode: 0o755},
		{Name: "ayb-postgres-16/bin/postgres", Typeflag: tar.TypeReg, Mode: 0o755, Size: int64(len("pg"))},
		{Name: "ayb-postgres-16/bin/bad-link", Typeflag: tar.TypeSymlink, Mode: 0o755, Linkname: "../../etc/passwd"},
	}, map[string]string{
		"ayb-postgres-16/bin/postgres": "pg",
	})

	archivePath := filepath.Join(t.TempDir(), "bad-symlink.tar.xz")
	testutil.NoError(t, os.WriteFile(archivePath, archive, 0o644))

	destDir := filepath.Join(t.TempDir(), "extracted")
	testutil.NoError(t, os.MkdirAll(destDir, 0o755))

	err := extractTarXZ(archivePath, destDir)
	testutil.ErrorContains(t, err, "escapes destination")
}

func TestCacheHitSkipsDownload(t *testing.T) {
	t.Parallel()
	archive := makeTarXZ(t, map[string]string{
		"ayb-postgres-16/bin/postgres": "pg",
		"ayb-postgres-16/PG_VERSION":   "16",
	})

	h := sha256.Sum256(archive)
	hash := fmt.Sprintf("%x", h)

	requestCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if r.URL.Path == "/SHA256SUMS" {
			fmt.Fprintf(w, "%s  ayb-postgres-16-darwin-arm64.tar.xz\n", hash)
			return
		}
		w.Write(archive)
	}))
	defer srv.Close()

	cacheDir := t.TempDir()
	binDir := filepath.Join(t.TempDir(), "bin")
	testutil.NoError(t, os.MkdirAll(binDir, 0o755))

	// Pre-populate cache.
	cachePath := filepath.Join(cacheDir, "ayb-postgres-16-darwin-arm64.tar.xz")
	testutil.NoError(t, os.WriteFile(cachePath, archive, 0o644))

	// Pre-populate binDir with extracted content.
	testutil.NoError(t, os.MkdirAll(filepath.Join(binDir, "bin"), 0o755))
	testutil.NoError(t, os.WriteFile(filepath.Join(binDir, "bin", "postgres"), []byte("pg"), 0o755))
	testutil.NoError(t, os.WriteFile(filepath.Join(binDir, "PG_VERSION"), []byte("16"), 0o644))

	usedLegacyFallback, err := ensureBinary(context.Background(), ensureBinaryOpts{
		version:   "16",
		platform:  "darwin-arm64",
		cacheDir:  cacheDir,
		binDir:    binDir,
		baseURL:   srv.URL + "/{version}/{platform}.tar.xz",
		sha256URL: srv.URL + "/SHA256SUMS",
	})
	testutil.False(t, usedLegacyFallback, "managed release path should not report legacy fallback")
	testutil.NoError(t, err)

	// No HTTP requests should be made (cache hit + binaries already extracted).
	testutil.Equal(t, 0, requestCount)
}

func TestCacheMissTriggersDownload(t *testing.T) {
	t.Parallel()
	archive := makeTarXZ(t, map[string]string{
		"ayb-postgres-16/bin/postgres": "pg",
		"ayb-postgres-16/PG_VERSION":   "16",
	})

	h := sha256.Sum256(archive)
	hash := fmt.Sprintf("%x", h)

	downloadHit := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/SHA256SUMS" {
			fmt.Fprintf(w, "%s  ayb-postgres-16-darwin-arm64.tar.xz\n", hash)
			return
		}
		downloadHit = true
		w.Write(archive)
	}))
	defer srv.Close()

	cacheDir := t.TempDir()
	binDir := filepath.Join(t.TempDir(), "bindir")
	testutil.NoError(t, os.MkdirAll(binDir, 0o755))

	usedLegacyFallback, err := ensureBinary(context.Background(), ensureBinaryOpts{
		version:   "16",
		platform:  "darwin-arm64",
		cacheDir:  cacheDir,
		binDir:    binDir,
		baseURL:   srv.URL + "/{version}/{platform}.tar.xz",
		sha256URL: srv.URL + "/SHA256SUMS",
	})
	testutil.False(t, usedLegacyFallback, "custom binary URL should not trigger legacy fallback")
	testutil.NoError(t, err)
	testutil.True(t, downloadHit, "expected download for cache miss")

	// Binary should exist after download+extract.
	_, err = os.Stat(filepath.Join(binDir, "bin", "postgres"))
	testutil.NoError(t, err)
}

func TestVersionMismatchTriggersReExtraction(t *testing.T) {
	t.Parallel()
	archive := makeTarXZ(t, map[string]string{
		"ayb-postgres-16/bin/postgres": "pg16",
		"ayb-postgres-16/PG_VERSION":   "16",
	})

	h := sha256.Sum256(archive)
	hash := fmt.Sprintf("%x", h)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/SHA256SUMS" {
			fmt.Fprintf(w, "%s  ayb-postgres-16-linux-amd64.tar.xz\n", hash)
			return
		}
		w.Write(archive)
	}))
	defer srv.Close()

	cacheDir := t.TempDir()
	binDir := filepath.Join(t.TempDir(), "bindir")
	testutil.NoError(t, os.MkdirAll(binDir, 0o755))

	// Pre-populate cache with correct archive.
	cachePath := filepath.Join(cacheDir, "ayb-postgres-16-linux-amd64.tar.xz")
	testutil.NoError(t, os.WriteFile(cachePath, archive, 0o644))

	// Pre-populate binDir but with wrong PG_VERSION.
	testutil.NoError(t, os.MkdirAll(filepath.Join(binDir, "bin"), 0o755))
	testutil.NoError(t, os.WriteFile(filepath.Join(binDir, "bin", "postgres"), []byte("old"), 0o755))
	testutil.NoError(t, os.WriteFile(filepath.Join(binDir, "PG_VERSION"), []byte("15"), 0o644))

	usedLegacyFallback, err := ensureBinary(context.Background(), ensureBinaryOpts{
		version:   "16",
		platform:  "linux-amd64",
		cacheDir:  cacheDir,
		binDir:    binDir,
		baseURL:   srv.URL + "/{version}/{platform}.tar.xz",
		sha256URL: srv.URL + "/SHA256SUMS",
	})
	testutil.False(t, usedLegacyFallback, "custom binary URL should not trigger legacy fallback")
	testutil.NoError(t, err)

	// Should have re-extracted: PG_VERSION should now be "16".
	ver, err := os.ReadFile(filepath.Join(binDir, "PG_VERSION"))
	testutil.NoError(t, err)
	testutil.Equal(t, "16", string(ver))
}

func TestBinariesReady_MissingVersionFileFallsBackToBinaryVersion(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("shell script fixture is unix-specific")
	}

	binDir := t.TempDir()
	testutil.NoError(t, os.MkdirAll(filepath.Join(binDir, "bin"), 0o755))
	testutil.NoError(t, os.WriteFile(
		filepath.Join(binDir, "bin", "postgres"),
		[]byte("#!/bin/sh\necho 'postgres (PostgreSQL) 16.9'\n"),
		0o755,
	))

	if !binariesReady(binDir, "16") {
		t.Fatal("binariesReady() = false, want true when postgres --version matches major version")
	}
}

func TestBinariesReady_MissingVersionFileMismatchedBinaryVersion(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("shell script fixture is unix-specific")
	}

	binDir := t.TempDir()
	testutil.NoError(t, os.MkdirAll(filepath.Join(binDir, "bin"), 0o755))
	testutil.NoError(t, os.WriteFile(
		filepath.Join(binDir, "bin", "postgres"),
		[]byte("#!/bin/sh\necho 'postgres (PostgreSQL) 15.12'\n"),
		0o755,
	))

	if binariesReady(binDir, "16") {
		t.Fatal("binariesReady() = true, want false when postgres --version major mismatches")
	}
}
