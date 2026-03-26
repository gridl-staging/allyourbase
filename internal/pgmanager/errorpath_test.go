package pgmanager

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestChecksumMismatchRejectsExtract(t *testing.T) {
	t.Parallel()

	archive := makeTarXZ(t, map[string]string{
		"ayb-postgres-16/bin/postgres": "fake",
		"ayb-postgres-16/PG_VERSION":   "16",
	})

	// Serve archive with wrong hash in SHA256SUMS.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/SHA256SUMS" {
			fmt.Fprintf(w, "%s  ayb-postgres-16-darwin-arm64.tar.xz\n",
				"0000000000000000000000000000000000000000000000000000000000000000")
			return
		}
		w.Write(archive)
	}))
	defer srv.Close()

	binDir := filepath.Join(t.TempDir(), "bin")
	_, err := ensureBinary(context.Background(), ensureBinaryOpts{
		version:   "16",
		platform:  "darwin-arm64",
		cacheDir:  t.TempDir(),
		binDir:    binDir,
		baseURL:   srv.URL + "/{version}/{platform}.tar.xz",
		sha256URL: srv.URL + "/SHA256SUMS",
	})
	testutil.True(t, err != nil, "expected error for checksum mismatch")
	testutil.Contains(t, err.Error(), "verifying downloaded binary")

	// Binary should NOT have been extracted.
	entries, _ := filepath.Glob(filepath.Join(binDir, "*"))
	testutil.True(t, len(entries) == 0, "no files should be extracted after checksum mismatch")
}

func TestDownloadFromUnreachableServer(t *testing.T) {
	t.Parallel()

	_, err := ensureBinary(context.Background(), ensureBinaryOpts{
		version:   "16",
		platform:  "darwin-arm64",
		cacheDir:  t.TempDir(),
		binDir:    filepath.Join(t.TempDir(), "bin"),
		baseURL:   "http://127.0.0.1:1/{version}/{platform}.tar.xz",
		sha256URL: "http://127.0.0.1:1/SHA256SUMS",
	})
	testutil.True(t, err != nil, "expected error for unreachable server")
	testutil.Contains(t, err.Error(), "fetching SHA256SUMS")
}

func TestCorruptTarballReturnsError(t *testing.T) {
	t.Parallel()

	corrupt := []byte("this is not a valid tar.xz file")
	h := sha256.Sum256(corrupt)
	hash := fmt.Sprintf("%x", h)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/SHA256SUMS" {
			fmt.Fprintf(w, "%s  ayb-postgres-16-darwin-arm64.tar.xz\n", hash)
			return
		}
		w.Write(corrupt)
	}))
	defer srv.Close()

	_, err := ensureBinary(context.Background(), ensureBinaryOpts{
		version:   "16",
		platform:  "darwin-arm64",
		cacheDir:  t.TempDir(),
		binDir:    filepath.Join(t.TempDir(), "bin"),
		baseURL:   srv.URL + "/{version}/{platform}.tar.xz",
		sha256URL: srv.URL + "/SHA256SUMS",
	})
	testutil.True(t, err != nil, "expected error for corrupt tarball")
	testutil.Contains(t, err.Error(), "extracting binary")
}
