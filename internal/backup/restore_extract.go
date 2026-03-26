// Package backup contains utilities for extracting base backups and WAL segments from tar archives with streaming decompression and path traversal protections.
package backup

import (
	"archive/tar"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/klauspost/compress/zstd"
)

// ExtractBaseBackup streams a .tar.zst base-backup object from Store and
// extracts it into targetDir.
func ExtractBaseBackup(ctx context.Context, store Store, objectKey string, targetDir string) error {
	if err := os.MkdirAll(targetDir, 0o700); err != nil {
		return fmt.Errorf("creating target directory: %w", err)
	}
	entries, err := os.ReadDir(targetDir)
	if err != nil {
		return fmt.Errorf("reading target directory: %w", err)
	}
	if len(entries) != 0 {
		return fmt.Errorf("target directory %s must be empty", targetDir)
	}

	body, _, err := store.GetObject(ctx, objectKey)
	if err != nil {
		return fmt.Errorf("downloading base backup object %q: %w", objectKey, err)
	}
	defer body.Close()

	zr, err := zstd.NewReader(body)
	if err != nil {
		return fmt.Errorf("opening zstd stream for %q: %w", objectKey, err)
	}
	defer zr.Close()

	tr := tar.NewReader(zr)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("reading tar entry: %w", err)
		}

		destPath, err := safeTarDestination(targetDir, hdr.Name)
		if err != nil {
			return err
		}

		if err := extractTarEntry(tr, hdr, targetDir, destPath); err != nil {
			return err
		}
	}

	return nil
}

// extractTarEntry extracts a tar entry to destPath, creating directories and files as needed and validating symlink targets to prevent path traversal.
func extractTarEntry(tr *tar.Reader, hdr *tar.Header, targetDir, destPath string) error {
	mode := os.FileMode(hdr.Mode)
	switch hdr.Typeflag {
	case tar.TypeDir:
		if err := os.MkdirAll(destPath, mode.Perm()); err != nil {
			return fmt.Errorf("creating directory %s: %w", destPath, err)
		}
		return nil
	case tar.TypeReg:
		if err := os.MkdirAll(filepath.Dir(destPath), 0o700); err != nil {
			return fmt.Errorf("creating parent directory for %s: %w", destPath, err)
		}
		f, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode.Perm())
		if err != nil {
			return fmt.Errorf("creating file %s: %w", destPath, err)
		}
		if _, err := io.Copy(f, tr); err != nil {
			f.Close()
			return fmt.Errorf("writing file %s: %w", destPath, err)
		}
		if err := f.Close(); err != nil {
			return fmt.Errorf("closing file %s: %w", destPath, err)
		}
		return nil
	case tar.TypeSymlink:
		if err := validateSymlinkTarget(targetDir, destPath, hdr.Linkname); err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(destPath), 0o700); err != nil {
			return fmt.Errorf("creating parent directory for symlink %s: %w", destPath, err)
		}
		if err := os.Symlink(hdr.Linkname, destPath); err != nil {
			return fmt.Errorf("creating symlink %s -> %s: %w", destPath, hdr.Linkname, err)
		}
		return nil
	default:
		return fmt.Errorf("unsupported tar entry type %d for %s", hdr.Typeflag, hdr.Name)
	}
}

// safeTarDestination validates entryName and returns the full path within targetDir, rejecting absolute paths, parent references, and paths that would escape the target directory.
func safeTarDestination(targetDir, entryName string) (string, error) {
	if filepath.IsAbs(entryName) {
		return "", fmt.Errorf("path traversal rejected: absolute path %q", entryName)
	}
	clean := filepath.Clean(entryName)
	if clean == "." {
		return "", fmt.Errorf("invalid tar entry path %q", entryName)
	}
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path traversal rejected: %q", entryName)
	}
	fullPath := filepath.Join(targetDir, clean)
	if !isPathWithinBase(targetDir, fullPath) {
		return "", fmt.Errorf("path traversal rejected: %q", entryName)
	}
	return fullPath, nil
}

func validateSymlinkTarget(targetDir, symlinkPath, linkName string) error {
	if filepath.IsAbs(linkName) {
		return fmt.Errorf("path traversal rejected: absolute symlink target %q", linkName)
	}
	resolved := filepath.Clean(filepath.Join(filepath.Dir(symlinkPath), linkName))
	if !isPathWithinBase(targetDir, resolved) {
		return fmt.Errorf("path traversal rejected: symlink target %q escapes %s", linkName, targetDir)
	}
	return nil
}

func isPathWithinBase(base, candidate string) bool {
	rel, err := filepath.Rel(base, candidate)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// DownloadWALSegments fetches required WAL files into walArchiveDir for
// restore_command-based replay.
func DownloadWALSegments(
	ctx context.Context,
	store Store,
	segments []WALSegment,
	archivePrefix, projectID, databaseID string,
	walArchiveDir string,
) error {
	if err := os.MkdirAll(walArchiveDir, 0o700); err != nil {
		return fmt.Errorf("creating WAL archive directory: %w", err)
	}

	for _, seg := range segments {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := validateSegmentFileName(seg.SegmentName); err != nil {
			return err
		}
		dst := filepath.Join(walArchiveDir, seg.SegmentName)
		if stat, err := os.Stat(dst); err == nil && stat.Size() == seg.SizeBytes {
			continue
		}

		key := WALSegmentKey(archivePrefix, projectID, databaseID, seg.Timeline, seg.SegmentName)
		reader, size, err := store.GetObject(ctx, key)
		if err != nil {
			return fmt.Errorf("downloading WAL segment %s: %w", seg.SegmentName, err)
		}

		tmp := dst + ".tmp"
		writeErr := writeReaderToFile(reader, tmp)
		closeErr := reader.Close()
		if writeErr != nil {
			_ = os.Remove(tmp)
			return fmt.Errorf("writing WAL segment %s: %w", seg.SegmentName, writeErr)
		}
		if closeErr != nil {
			_ = os.Remove(tmp)
			return fmt.Errorf("closing WAL segment stream %s: %w", seg.SegmentName, closeErr)
		}
		if seg.SizeBytes > 0 && size > 0 && seg.SizeBytes != size {
			_ = os.Remove(tmp)
			return fmt.Errorf("size mismatch for WAL segment %s: expected %d bytes, got %d bytes", seg.SegmentName, seg.SizeBytes, size)
		}
		if err := os.Rename(tmp, dst); err != nil {
			_ = os.Remove(tmp)
			return fmt.Errorf("finalizing WAL segment %s: %w", seg.SegmentName, err)
		}
	}

	return nil
}

func validateSegmentFileName(name string) error {
	if name == "" {
		return fmt.Errorf("invalid WAL segment file name: empty")
	}
	if filepath.Base(name) != name {
		return fmt.Errorf("invalid WAL segment file name %q", name)
	}
	if strings.Contains(name, string(filepath.Separator)) || strings.Contains(name, "..") {
		return fmt.Errorf("invalid WAL segment file name %q", name)
	}
	return nil
}

func writeReaderToFile(reader io.Reader, path string) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, reader); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
