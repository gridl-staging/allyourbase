package pgmanager

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type httpStatusError struct {
	op         string
	url        string
	statusCode int
}

func (e *httpStatusError) Error() string {
	return fmt.Sprintf("%s %s: HTTP %d", e.op, e.url, e.statusCode)
}

func isHTTPStatus(err error, statusCode int) bool {
	var statusErr *httpStatusError
	return errors.As(err, &statusErr) && statusErr.statusCode == statusCode
}

// downloadBinary streams an HTTP GET to a temp file, then renames on success.
// On failure the temp file is cleaned up so no partial file remains at destPath.
func downloadBinary(ctx context.Context, url, destPath string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("creating download request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("downloading %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return &httpStatusError{
			op:         "downloading",
			url:        url,
			statusCode: resp.StatusCode,
		}
	}

	dir := filepath.Dir(destPath)
	tmp, err := os.CreateTemp(dir, ".download-*")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := tmp.Name()

	// Always clean up the temp file on failure.
	success := false
	defer func() {
		if !success {
			tmp.Close()
			os.Remove(tmpPath)
		}
	}()

	if _, err := io.Copy(tmp, resp.Body); err != nil {
		return fmt.Errorf("writing download to disk: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing temp file: %w", err)
	}

	if err := os.Rename(tmpPath, destPath); err != nil {
		return fmt.Errorf("moving download to final path: %w", err)
	}

	success = true
	return nil
}

// verifySHA256 computes the SHA256 of a file and compares to the expected hex digest.
func verifySHA256(filePath, expectedHash string) error {
	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("opening file for SHA256: %w", err)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return fmt.Errorf("computing SHA256: %w", err)
	}

	got := hex.EncodeToString(h.Sum(nil))
	if got != expectedHash {
		return fmt.Errorf("SHA256 mismatch: expected %s, got %s", expectedHash, got)
	}
	return nil
}

// fetchSHA256Sums downloads a SHA256SUMS file and parses "hash  filename" lines.
func fetchSHA256Sums(ctx context.Context, url string) (map[string]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating SHA256SUMS request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching SHA256SUMS: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, &httpStatusError{
			op:         "fetching SHA256SUMS",
			url:        url,
			statusCode: resp.StatusCode,
		}
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading SHA256SUMS body: %w", err)
	}

	sums := make(map[string]string)
	for _, line := range strings.Split(strings.TrimSpace(string(body)), "\n") {
		parts := strings.Fields(line)
		if len(parts) != 2 {
			continue
		}
		sums[parts[1]] = parts[0]
	}
	return sums, nil
}

func fetchSHA256Digest(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("creating SHA256 digest request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetching SHA256 digest: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", &httpStatusError{
			op:         "fetching SHA256 digest",
			url:        url,
			statusCode: resp.StatusCode,
		}
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading SHA256 digest body: %w", err)
	}

	digest := strings.TrimSpace(string(body))
	if digest == "" {
		return "", fmt.Errorf("empty SHA256 digest response")
	}
	return digest, nil
}
