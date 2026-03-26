package backup

import (
	"io"

	"github.com/klauspost/compress/zstd"
)

// CompressZstdToTempFile streams from r through zstd into a temporary file.
// The caller must invoke cleanup() when done. On error, temp artifacts are removed.
func CompressZstdToTempFile(r io.Reader) (CompressResult, func(), error) {
	return compressToTempFile(r, "ayb-backup-*.tar.zst", func(w io.Writer) (io.WriteCloser, error) {
		return zstd.NewWriter(w)
	})
}
