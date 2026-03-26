package backup

import (
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
)

// CompressResult holds the outcome of a compression operation.
type CompressResult struct {
	Path     string // path to the compressed temp file
	Size     int64
	Checksum string // SHA-256 hex digest
}

// CompressToTempFile streams from r through gzip into a temporary file.
// The caller must call cleanup() when done (removes the temp file).
// On error, the temp file is cleaned up automatically.
func CompressToTempFile(r io.Reader) (CompressResult, func(), error) {
	return compressToTempFile(r, "ayb-backup-*.sql.gz", func(w io.Writer) (io.WriteCloser, error) {
		return gzip.NewWriter(w), nil
	})
}

// compressToTempFile is the shared implementation for gzip and zstd compression.
// suffix is the temp file naming pattern, newCompressor creates the compression writer.
func compressToTempFile(r io.Reader, suffix string, newCompressor func(io.Writer) (io.WriteCloser, error)) (CompressResult, func(), error) {
	tmp, err := os.CreateTemp("", suffix)
	if err != nil {
		return CompressResult{}, nil, fmt.Errorf("creating temp file: %w", err)
	}

	cleanupFn := func() { _ = os.Remove(tmp.Name()) }

	hash := sha256.New()
	cw, err := newCompressor(io.MultiWriter(tmp, hash))
	if err != nil {
		tmp.Close()
		cleanupFn()
		return CompressResult{}, nil, fmt.Errorf("creating compressor: %w", err)
	}

	if _, err := io.Copy(cw, r); err != nil {
		cw.Close()
		tmp.Close()
		cleanupFn()
		return CompressResult{}, nil, fmt.Errorf("compressing backup: %w", err)
	}
	if err := cw.Close(); err != nil {
		tmp.Close()
		cleanupFn()
		return CompressResult{}, nil, fmt.Errorf("closing compressor: %w", err)
	}

	info, err := tmp.Stat()
	if err != nil {
		tmp.Close()
		cleanupFn()
		return CompressResult{}, nil, fmt.Errorf("stat temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		cleanupFn()
		return CompressResult{}, nil, fmt.Errorf("closing temp file: %w", err)
	}

	return CompressResult{
		Path:     tmp.Name(),
		Size:     info.Size(),
		Checksum: hex.EncodeToString(hash.Sum(nil)),
	}, cleanupFn, nil
}

// DecompressReader wraps r with a gzip reader for decompression.
func DecompressReader(r io.Reader) (io.ReadCloser, error) {
	gr, err := gzip.NewReader(r)
	if err != nil {
		return nil, fmt.Errorf("creating gzip reader: %w", err)
	}
	return gr, nil
}
