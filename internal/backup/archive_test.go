package backup

import (
	"compress/gzip"
	"encoding/hex"
	"io"
	"os"
	"strings"
	"testing"
)

func TestCompressToTempFile(t *testing.T) {
	const input = "hello world backup data"
	result, cleanup, err := CompressToTempFile(strings.NewReader(input))
	if err != nil {
		t.Fatalf("CompressToTempFile: %v", err)
	}
	defer cleanup()

	if result.Size <= 0 {
		t.Errorf("expected size > 0, got %d", result.Size)
	}
	if result.Checksum == "" {
		t.Error("expected non-empty checksum")
	}
	if result.Path == "" {
		t.Error("expected non-empty temp path")
	}
}

func TestCompressToTempFileChecksumFormat(t *testing.T) {
	result, cleanup, err := CompressToTempFile(strings.NewReader("data"))
	if err != nil {
		t.Fatalf("CompressToTempFile: %v", err)
	}
	defer cleanup()

	if len(result.Checksum) != 64 {
		t.Errorf("SHA-256 hex should be 64 chars, got %d", len(result.Checksum))
	}
	if _, decErr := hex.DecodeString(result.Checksum); decErr != nil {
		t.Errorf("checksum not valid hex: %v", decErr)
	}
}

func TestCompressToTempFileDecompresses(t *testing.T) {
	const input = "compress and decompress me"
	result, cleanup, err := CompressToTempFile(strings.NewReader(input))
	if err != nil {
		t.Fatalf("CompressToTempFile: %v", err)
	}
	defer cleanup()

	f, err := os.Open(result.Path)
	if err != nil {
		t.Fatalf("open temp file: %v", err)
	}
	defer f.Close()

	gr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("gzip reader: %v", err)
	}
	defer gr.Close()

	got, err := io.ReadAll(gr)
	if err != nil {
		t.Fatalf("read decompressed: %v", err)
	}
	if string(got) != input {
		t.Errorf("decompressed = %q; want %q", got, input)
	}
}

func TestCompressToTempFileLargeData(t *testing.T) {
	large := strings.Repeat("x", 1<<20) // 1 MB of repeated bytes
	result, cleanup, err := CompressToTempFile(strings.NewReader(large))
	if err != nil {
		t.Fatalf("CompressToTempFile large: %v", err)
	}
	defer cleanup()
	if result.Size >= int64(len(large)) {
		t.Errorf("compressed size %d >= input size %d", result.Size, len(large))
	}
}
