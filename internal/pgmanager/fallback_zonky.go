package pgmanager

import (
	"archive/zip"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const legacyBinaryBaseURL = "https://repo.maven.apache.org/maven2/io/zonky/test/postgres"

type legacyArchiveSource struct {
	jarFilename string
	jarURL      string
	sha256URL   string
}

type mavenMetadata struct {
	Versioning struct {
		Versions struct {
			Items []string `xml:"version"`
		} `xml:"versions"`
	} `xml:"versioning"`
}

func ensureBinaryFromLegacyArchive(ctx context.Context, opts ensureBinaryOpts) error {
	source, err := resolveLegacyArchiveSource(ctx, opts.version, opts.platform, opts.legacyBaseURL)
	if err != nil {
		return fmt.Errorf("resolving legacy embedded-postgres archive: %w", err)
	}

	cachePath := filepath.Join(opts.cacheDir, source.jarFilename)
	expectedHash, err := fetchSHA256Digest(ctx, source.sha256URL)
	if err != nil {
		return fmt.Errorf("fetching legacy SHA256 digest: %w", err)
	}

	needDownload := true
	if _, err := os.Stat(cachePath); err == nil {
		if err := verifySHA256(cachePath, expectedHash); err == nil {
			needDownload = false
		}
	}

	if needDownload {
		if err := downloadBinary(ctx, source.jarURL, cachePath); err != nil {
			return fmt.Errorf("downloading legacy embedded-postgres archive: %w", err)
		}
		if err := verifySHA256(cachePath, expectedHash); err != nil {
			_ = os.Remove(cachePath)
			return fmt.Errorf("verifying legacy embedded-postgres archive: %w", err)
		}
	}

	if err := extractLegacyJarArchive(cachePath, opts.binDir); err != nil {
		return fmt.Errorf("extracting legacy embedded-postgres archive: %w", err)
	}

	return nil
}

func resolveLegacyArchiveSource(ctx context.Context, version, platform, baseURL string) (legacyArchiveSource, error) {
	artifact, err := zonkyArtifactForPlatform(platform)
	if err != nil {
		return legacyArchiveSource{}, err
	}
	if baseURL == "" {
		baseURL = legacyBinaryBaseURL
	}

	metadataURL := fmt.Sprintf("%s/%s/maven-metadata.xml", strings.TrimRight(baseURL, "/"), artifact)
	fullVersion, err := fetchLatestLegacyVersion(ctx, metadataURL, version)
	if err != nil {
		return legacyArchiveSource{}, err
	}

	jarFilename := fmt.Sprintf("%s-%s.jar", artifact, fullVersion)
	jarURL := fmt.Sprintf("%s/%s/%s/%s", strings.TrimRight(baseURL, "/"), artifact, fullVersion, jarFilename)

	return legacyArchiveSource{
		jarFilename: jarFilename,
		jarURL:      jarURL,
		sha256URL:   jarURL + ".sha256",
	}, nil
}

func zonkyArtifactForPlatform(platform string) (string, error) {
	switch platform {
	case "darwin-arm64":
		return "embedded-postgres-binaries-darwin-arm64v8", nil
	case "darwin-amd64":
		return "embedded-postgres-binaries-darwin-amd64", nil
	case "linux-arm64":
		return "embedded-postgres-binaries-linux-arm64v8", nil
	case "linux-amd64":
		return "embedded-postgres-binaries-linux-amd64", nil
	default:
		return "", fmt.Errorf("unsupported legacy embedded-postgres platform: %s", platform)
	}
}

func fetchLatestLegacyVersion(ctx context.Context, metadataURL, major string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, metadataURL, nil)
	if err != nil {
		return "", fmt.Errorf("creating metadata request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetching metadata: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", &httpStatusError{
			op:         "fetching metadata",
			url:        metadataURL,
			statusCode: resp.StatusCode,
		}
	}

	var metadata mavenMetadata
	if err := xml.NewDecoder(resp.Body).Decode(&metadata); err != nil {
		return "", fmt.Errorf("decoding metadata XML: %w", err)
	}

	prefix := major + "."
	for i := len(metadata.Versioning.Versions.Items) - 1; i >= 0; i-- {
		version := strings.TrimSpace(metadata.Versioning.Versions.Items[i])
		if strings.HasPrefix(version, prefix) {
			return version, nil
		}
	}

	return "", fmt.Errorf("no legacy embedded-postgres version found for major %s", major)
}

func extractLegacyJarArchive(jarPath, destDir string) error {
	reader, err := zip.OpenReader(jarPath)
	if err != nil {
		return fmt.Errorf("opening jar archive: %w", err)
	}
	defer reader.Close()

	var txzFile *zip.File
	for _, file := range reader.File {
		if strings.HasSuffix(file.Name, ".txz") {
			txzFile = file
			break
		}
	}
	if txzFile == nil {
		return fmt.Errorf("jar archive did not contain a .txz payload")
	}

	rc, err := txzFile.Open()
	if err != nil {
		return fmt.Errorf("opening jar payload: %w", err)
	}
	defer rc.Close()

	tmp, err := os.CreateTemp(filepath.Dir(jarPath), ".legacy-pg-*.txz")
	if err != nil {
		return fmt.Errorf("creating temp payload file: %w", err)
	}
	tmpPath := tmp.Name()
	success := false
	defer func() {
		_ = tmp.Close()
		if !success {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := io.Copy(tmp, rc); err != nil {
		return fmt.Errorf("writing temp payload file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing temp payload file: %w", err)
	}

	if err := extractTarXZ(tmpPath, destDir); err != nil {
		return err
	}

	success = true
	return os.Remove(tmpPath)
}
