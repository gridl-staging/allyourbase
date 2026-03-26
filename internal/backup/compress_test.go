package backup

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/klauspost/compress/zstd"
)

func TestCompressZstdToTempFileCompressesAndDecompresses(t *testing.T) {
	const input = "physical backup tar stream bytes"

	result, cleanup, err := CompressZstdToTempFile(strings.NewReader(input))
	if err != nil {
		t.Fatalf("CompressZstdToTempFile: %v", err)
	}
	defer cleanup()

	if result.Path == "" {
		t.Fatal("expected temp file path")
	}
	if result.Size <= 0 {
		t.Fatalf("expected size > 0, got %d", result.Size)
	}
	if result.Checksum == "" {
		t.Fatal("expected non-empty checksum")
	}

	f, err := os.Open(result.Path)
	if err != nil {
		t.Fatalf("opening compressed file: %v", err)
	}
	defer f.Close()

	zr, err := zstd.NewReader(f)
	if err != nil {
		t.Fatalf("creating zstd reader: %v", err)
	}
	defer zr.Close()

	got, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("decompressing zstd data: %v", err)
	}
	if string(got) != input {
		t.Fatalf("decompressed content mismatch: got %q want %q", string(got), input)
	}
}

func TestCompressZstdToTempFileChecksumDeterministic(t *testing.T) {
	const input = "checksum should be deterministic for same input"

	first, cleanupFirst, err := CompressZstdToTempFile(strings.NewReader(input))
	if err != nil {
		t.Fatalf("first CompressZstdToTempFile: %v", err)
	}
	defer cleanupFirst()

	second, cleanupSecond, err := CompressZstdToTempFile(strings.NewReader(input))
	if err != nil {
		t.Fatalf("second CompressZstdToTempFile: %v", err)
	}
	defer cleanupSecond()

	if first.Checksum != second.Checksum {
		t.Fatalf("checksum mismatch for identical input: %q vs %q", first.Checksum, second.Checksum)
	}
}

func TestCompressZstdToTempFileCleanupRemovesFile(t *testing.T) {
	result, cleanup, err := CompressZstdToTempFile(strings.NewReader("cleanup me"))
	if err != nil {
		t.Fatalf("CompressZstdToTempFile: %v", err)
	}

	if _, err := os.Stat(result.Path); err != nil {
		t.Fatalf("expected temp file to exist before cleanup: %v", err)
	}

	cleanup()
	if _, err := os.Stat(result.Path); err == nil {
		t.Fatal("expected temp file to be removed after cleanup")
	}
}
