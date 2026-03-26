package pgmanager

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestDownloadBinarySuccess(t *testing.T) {
	t.Parallel()
	content := []byte("fake-postgres-binary-content-12345")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(content)
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "postgres.tar.xz")
	err := downloadBinary(context.Background(), srv.URL+"/binary.tar.xz", dest)
	testutil.NoError(t, err)

	got, err := os.ReadFile(dest)
	testutil.NoError(t, err)
	testutil.Equal(t, string(content), string(got))
}

func TestDownloadBinary404(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "postgres.tar.xz")
	err := downloadBinary(context.Background(), srv.URL+"/missing", dest)
	testutil.True(t, err != nil, "expected error for 404")
	testutil.Contains(t, err.Error(), "404")
}

func TestDownloadBinary500(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "postgres.tar.xz")
	err := downloadBinary(context.Background(), srv.URL+"/error", dest)
	testutil.True(t, err != nil, "expected error for 500")
	testutil.Contains(t, err.Error(), "500")
}

func TestDownloadBinaryContextCancellation(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("data"))
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "postgres.tar.xz")
	err := downloadBinary(ctx, srv.URL+"/binary", dest)
	testutil.True(t, err != nil, "expected error on cancelled context")
}

func TestDownloadBinaryNoTempFileOnFailure(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	dir := t.TempDir()
	dest := filepath.Join(dir, "postgres.tar.xz")
	_ = downloadBinary(context.Background(), srv.URL+"/missing", dest)

	// Neither final file nor temp file should exist.
	entries, _ := os.ReadDir(dir)
	testutil.Equal(t, 0, len(entries))
}

func TestVerifySHA256Pass(t *testing.T) {
	t.Parallel()
	content := []byte("hello world")
	h := sha256.Sum256(content)
	hash := fmt.Sprintf("%x", h)

	path := filepath.Join(t.TempDir(), "test.bin")
	testutil.NoError(t, os.WriteFile(path, content, 0o644))

	err := verifySHA256(path, hash)
	testutil.NoError(t, err)
}

func TestVerifySHA256Mismatch(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "test.bin")
	testutil.NoError(t, os.WriteFile(path, []byte("hello"), 0o644))

	err := verifySHA256(path, "0000000000000000000000000000000000000000000000000000000000000000")
	testutil.True(t, err != nil, "expected error for hash mismatch")
	testutil.Contains(t, err.Error(), "mismatch")
}

func TestFetchSHA256Sums(t *testing.T) {
	t.Parallel()
	body := "abc123  ayb-postgres-16-darwin-arm64.tar.xz\ndef456  ayb-postgres-16-linux-amd64.tar.xz\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(body))
	}))
	defer srv.Close()

	sums, err := fetchSHA256Sums(context.Background(), srv.URL+"/SHA256SUMS")
	testutil.NoError(t, err)
	testutil.Equal(t, "abc123", sums["ayb-postgres-16-darwin-arm64.tar.xz"])
	testutil.Equal(t, "def456", sums["ayb-postgres-16-linux-amd64.tar.xz"])
}

func TestFetchSHA256SumsServerError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	_, err := fetchSHA256Sums(context.Background(), srv.URL+"/SHA256SUMS")
	testutil.True(t, err != nil, "expected error for 500")
}
