package pgmanager

import (
	"archive/tar"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/ulikunitz/xz"
)

// extractTarXZ decompresses a .tar.xz archive into destDir,
// stripping the top-level directory prefix so bin/postgres lands directly under destDir.
func extractTarXZ(archivePath, destDir string) error {
	cleanDest := filepath.Clean(destDir)

	// First pass: detect common top-level prefix.
	prefix, err := detectTarPrefix(archivePath)
	if err != nil {
		return err
	}

	// Second pass: extract with prefix stripped.
	f, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("opening archive: %w", err)
	}
	defer f.Close()

	xzr, err := xz.NewReader(f)
	if err != nil {
		return fmt.Errorf("creating xz reader: %w", err)
	}

	tr := tar.NewReader(xzr)

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("reading tar entry: %w", err)
		}

		name := hdr.Name
		if prefix != "" {
			name = strings.TrimPrefix(name, prefix)
			if name == "" {
				continue // skip the prefix directory entry itself
			}
		}

		target := filepath.Join(destDir, filepath.FromSlash(name))

		// Validate path doesn't escape destDir (zip slip protection).
		cleanTarget := filepath.Clean(target)
		if !strings.HasPrefix(cleanTarget, cleanDest+string(os.PathSeparator)) &&
			cleanTarget != cleanDest {
			return fmt.Errorf("tar entry %q escapes destination directory", hdr.Name)
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, os.FileMode(hdr.Mode)); err != nil {
				return fmt.Errorf("creating directory %s: %w", target, err)
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return fmt.Errorf("creating parent directory for %s: %w", target, err)
			}
			if st, statErr := os.Lstat(target); statErr == nil && (st.Mode()&os.ModeSymlink) != 0 {
				return fmt.Errorf("refusing to write through symlink %q", hdr.Name)
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode))
			if err != nil {
				return fmt.Errorf("creating file %s: %w", target, err)
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return fmt.Errorf("writing file %s: %w", target, err)
			}
			out.Close()
		case tar.TypeSymlink:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return fmt.Errorf("creating parent directory for symlink %s: %w", target, err)
			}
			link := filepath.Clean(filepath.FromSlash(hdr.Linkname))
			if filepath.IsAbs(link) {
				return fmt.Errorf("symlink %q has absolute target %q", hdr.Name, hdr.Linkname)
			}
			resolved := filepath.Clean(filepath.Join(filepath.Dir(target), link))
			if !strings.HasPrefix(resolved, cleanDest+string(os.PathSeparator)) && resolved != cleanDest {
				return fmt.Errorf("symlink %q target escapes destination", hdr.Name)
			}
			if err := os.Symlink(link, target); err != nil {
				return fmt.Errorf("creating symlink %s: %w", target, err)
			}
		}
	}

	return nil
}

// detectTarPrefix reads a .tar.xz archive and returns the common top-level
// directory prefix shared by all entries, or "" if no common prefix exists.
func detectTarPrefix(archivePath string) (string, error) {
	f, err := os.Open(archivePath)
	if err != nil {
		return "", fmt.Errorf("opening archive for prefix detection: %w", err)
	}
	defer f.Close()

	xzr, err := xz.NewReader(f)
	if err != nil {
		return "", fmt.Errorf("creating xz reader for prefix detection: %w", err)
	}

	tr := tar.NewReader(xzr)
	prefix := ""
	first := true

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("reading tar entry for prefix detection: %w", err)
		}

		parts := strings.SplitN(hdr.Name, "/", 2)
		topLevel := parts[0]

		if first {
			prefix = topLevel
			first = true
		}
		first = false

		if topLevel != prefix {
			return "", nil // entries don't share a common prefix
		}
	}

	if prefix != "" {
		return prefix + "/", nil
	}
	return "", nil
}

// ensureBinaryOpts configures the ensureBinary function.
type ensureBinaryOpts struct {
	version       string
	platform      string
	cacheDir      string
	binDir        string
	baseURL       string // custom download URL template (empty = GitHub default)
	sha256URL     string // URL to SHA256SUMS file
	legacyBaseURL string // test hook for legacy fallback source (empty = Maven Central)
}

// ensureBinary ensures the PG binary is downloaded, verified, and extracted.
// It checks the cache and bin directory to skip unnecessary work.
func ensureBinary(ctx context.Context, opts ensureBinaryOpts) (bool, error) {
	// Check if binaries are already extracted with correct version.
	if binariesReady(opts.binDir, opts.version) {
		return false, nil
	}

	if opts.baseURL != "" {
		return false, ensureBinaryFromManagedRelease(ctx, opts)
	}

	primaryErr := ensureBinaryFromManagedRelease(ctx, opts)
	if primaryErr == nil {
		return false, nil
	}

	fallbackErr := ensureBinaryFromLegacyArchive(ctx, opts)
	if fallbackErr == nil {
		return true, nil
	}

	return false, fmt.Errorf("%w; legacy fallback also failed: %v", primaryErr, fallbackErr)
}

func ensureBinaryFromManagedRelease(ctx context.Context, opts ensureBinaryOpts) error {
	filename := fmt.Sprintf("ayb-postgres-%s-%s.tar.xz", opts.version, opts.platform)
	cachePath := filepath.Join(opts.cacheDir, filename)

	sums, err := fetchSHA256Sums(ctx, opts.sha256URL)
	if err != nil {
		return fmt.Errorf("fetching SHA256SUMS: %w", err)
	}

	expectedHash, ok := sums[filename]
	if !ok {
		return fmt.Errorf("no SHA256 hash found for %s", filename)
	}

	needDownload := true
	if _, err := os.Stat(cachePath); err == nil {
		if err := verifySHA256(cachePath, expectedHash); err == nil {
			needDownload = false
		}
	}

	if needDownload {
		url := downloadURL(opts.baseURL, opts.version, opts.platform)
		if err := downloadBinary(ctx, url, cachePath); err != nil {
			return fmt.Errorf("downloading binary: %w", err)
		}
		if err := verifySHA256(cachePath, expectedHash); err != nil {
			_ = os.Remove(cachePath)
			return fmt.Errorf("verifying downloaded binary: %w", err)
		}
	}

	if err := extractTarXZ(cachePath, opts.binDir); err != nil {
		return fmt.Errorf("extracting binary: %w", err)
	}

	return nil
}

// binariesReady returns true if the bin directory has postgres and the correct PG_VERSION.
func binariesReady(binDir, version string) bool {
	pgBin := filepath.Join(binDir, "bin", "postgres")
	if _, err := os.Stat(pgBin); err != nil {
		return false
	}

	versionFile := filepath.Join(binDir, "PG_VERSION")
	data, err := os.ReadFile(versionFile)
	if err == nil {
		return strings.TrimSpace(string(data)) == version
	}

	// Backward compatibility: older local installations may have extracted
	// binaries without a PG_VERSION sentinel file.
	if os.IsNotExist(err) {
		return postgresBinaryMatchesVersion(pgBin, version)
	}

	return false
}

func postgresBinaryMatchesVersion(pgBin, version string) bool {
	// pgBin points to a local executable path under the configured binDir.
	out, err := exec.Command(pgBin, "--version").Output() //nolint:gosec
	if err != nil {
		return false
	}
	fields := strings.Fields(string(out))
	if len(fields) == 0 {
		return false
	}

	ver := fields[len(fields)-1]
	return ver == version || strings.HasPrefix(ver, version+".")
}
