package pgmanager

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/ulikunitz/xz"
)

func TestResolveLegacyArchiveSourceSelectsLatestPatch(t *testing.T) {
	metadata := `<?xml version="1.0" encoding="UTF-8"?>
<metadata>
  <versioning>
    <versions>
      <version>15.12.0</version>
      <version>16.9.0</version>
      <version>16.13.0</version>
      <version>17.1.0</version>
    </versions>
  </versioning>
</metadata>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.Equal(t, "/embedded-postgres-binaries-darwin-arm64v8/maven-metadata.xml", r.URL.Path)
		_, _ = w.Write([]byte(metadata))
	}))
	defer srv.Close()

	source, err := resolveLegacyArchiveSource(context.Background(), "16", "darwin-arm64", srv.URL)
	testutil.NoError(t, err)
	testutil.Equal(t, "embedded-postgres-binaries-darwin-arm64v8-16.13.0.jar", source.jarFilename)
	testutil.Equal(t, srv.URL+"/embedded-postgres-binaries-darwin-arm64v8/16.13.0/embedded-postgres-binaries-darwin-arm64v8-16.13.0.jar", source.jarURL)
}

func TestEnsureBinaryFromLegacyArchiveDownloadsAndExtracts(t *testing.T) {
	txzPayload := makeLegacyTarXZ(t, map[string]string{
		"bin/postgres":               "pg",
		"bin/initdb":                 "initdb",
		"share/postgresql/dummy.txt": "ok",
	})
	jarBytes := makeLegacyJar(t, "postgres-darwin-arm_64.txz", txzPayload)
	jarHash := sha256.Sum256(jarBytes)
	jarHashHex := hex.EncodeToString(jarHash[:])

	metadata := `<?xml version="1.0" encoding="UTF-8"?>
<metadata>
  <versioning>
    <versions>
      <version>16.13.0</version>
    </versions>
  </versioning>
</metadata>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/embedded-postgres-binaries-darwin-arm64v8/maven-metadata.xml":
			_, _ = w.Write([]byte(metadata))
		case "/embedded-postgres-binaries-darwin-arm64v8/16.13.0/embedded-postgres-binaries-darwin-arm64v8-16.13.0.jar.sha256":
			_, _ = w.Write([]byte(jarHashHex))
		case "/embedded-postgres-binaries-darwin-arm64v8/16.13.0/embedded-postgres-binaries-darwin-arm64v8-16.13.0.jar":
			_, _ = w.Write(jarBytes)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	binDir := filepath.Join(t.TempDir(), "bin")
	cacheDir := filepath.Join(t.TempDir(), "cache")
	testutil.NoError(t, os.MkdirAll(binDir, 0o755))
	testutil.NoError(t, os.MkdirAll(cacheDir, 0o755))

	err := ensureBinaryFromLegacyArchive(context.Background(), ensureBinaryOpts{
		version:       "16",
		platform:      "darwin-arm64",
		cacheDir:      cacheDir,
		binDir:        binDir,
		legacyBaseURL: srv.URL,
	})
	testutil.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(binDir, "bin", "postgres"))
	testutil.NoError(t, err)
	testutil.Equal(t, "pg", string(content))

	_, err = os.Stat(filepath.Join(cacheDir, "embedded-postgres-binaries-darwin-arm64v8-16.13.0.jar"))
	testutil.NoError(t, err)
}

func TestExtractLegacyJarArchiveRejectsMissingPayload(t *testing.T) {
	jarBytes := makeLegacyJar(t, "README.txt", []byte("not-a-txz"))
	jarPath := filepath.Join(t.TempDir(), "missing.jar")
	testutil.NoError(t, os.WriteFile(jarPath, jarBytes, 0o644))

	err := extractLegacyJarArchive(jarPath, t.TempDir())
	testutil.ErrorContains(t, err, ".txz payload")
}

func TestEnsureBinaryReportsLegacyFallbackUsage(t *testing.T) {
	archive := makeLegacyTarXZ(t, map[string]string{
		"bin/postgres": "pg",
	})
	jarBytes := makeLegacyJar(t, "postgres-darwin-arm_64.txz", archive)
	jarHash := sha256.Sum256(jarBytes)
	jarHashHex := hex.EncodeToString(jarHash[:])

	metadata := `<?xml version="1.0" encoding="UTF-8"?>
<metadata>
  <versioning>
    <versions>
      <version>16.13.0</version>
    </versions>
  </versioning>
</metadata>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/SHA256SUMS":
			http.NotFound(w, r)
		case "/embedded-postgres-binaries-darwin-arm64v8/maven-metadata.xml":
			_, _ = w.Write([]byte(metadata))
		case "/embedded-postgres-binaries-darwin-arm64v8/16.13.0/embedded-postgres-binaries-darwin-arm64v8-16.13.0.jar.sha256":
			_, _ = w.Write([]byte(jarHashHex))
		case "/embedded-postgres-binaries-darwin-arm64v8/16.13.0/embedded-postgres-binaries-darwin-arm64v8-16.13.0.jar":
			_, _ = w.Write(jarBytes)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	cacheDir := t.TempDir()
	binDir := filepath.Join(t.TempDir(), "bin")
	testutil.NoError(t, os.MkdirAll(binDir, 0o755))

	usedLegacyFallback, err := ensureBinary(context.Background(), ensureBinaryOpts{
		version:       "16",
		platform:      "darwin-arm64",
		cacheDir:      cacheDir,
		binDir:        binDir,
		sha256URL:     srv.URL + "/SHA256SUMS",
		legacyBaseURL: srv.URL,
	})
	testutil.NoError(t, err)
	testutil.True(t, usedLegacyFallback, "missing managed release assets should report legacy fallback usage")
}

func makeLegacyJar(t *testing.T, entryName string, content []byte) []byte {
	t.Helper()

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create(entryName)
	testutil.NoError(t, err)
	_, err = w.Write(content)
	testutil.NoError(t, err)
	testutil.NoError(t, zw.Close())

	return buf.Bytes()
}

func makeLegacyTarXZ(t *testing.T, files map[string]string) []byte {
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
