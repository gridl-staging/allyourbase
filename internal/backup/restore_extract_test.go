package backup

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/klauspost/compress/zstd"
)

type tarFixtureEntry struct {
	header tar.Header
	body   []byte
}

func buildTarZstdFixture(t *testing.T, entries []tarFixtureEntry) []byte {
	t.Helper()
	var tarBuf bytes.Buffer
	tw := tar.NewWriter(&tarBuf)
	for _, entry := range entries {
		h := entry.header
		if err := tw.WriteHeader(&h); err != nil {
			t.Fatalf("WriteHeader(%s): %v", h.Name, err)
		}
		if len(entry.body) > 0 {
			if _, err := tw.Write(entry.body); err != nil {
				t.Fatalf("Write(%s): %v", h.Name, err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar writer: %v", err)
	}

	var zstdBuf bytes.Buffer
	zw, err := zstd.NewWriter(&zstdBuf)
	if err != nil {
		t.Fatalf("zstd writer: %v", err)
	}
	if _, err := zw.Write(tarBuf.Bytes()); err != nil {
		t.Fatalf("zstd write: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zstd writer: %v", err)
	}
	return zstdBuf.Bytes()
}

func TestExtractBaseBackupSuccess(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	store.objects["base.tar.zst"] = buildTarZstdFixture(t, []tarFixtureEntry{
		{header: tar.Header{Name: "PG_VERSION", Mode: 0o600, Size: int64(len("16\n"))}, body: []byte("16\n")},
	})
	dir := t.TempDir()

	if err := ExtractBaseBackup(context.Background(), store, "base.tar.zst", dir); err != nil {
		t.Fatalf("ExtractBaseBackup: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "PG_VERSION"))
	if err != nil {
		t.Fatalf("reading extracted file: %v", err)
	}
	if string(data) != "16\n" {
		t.Fatalf("content = %q; want 16\\n", string(data))
	}
	stat, err := os.Stat(filepath.Join(dir, "PG_VERSION"))
	if err != nil {
		t.Fatalf("stat extracted file: %v", err)
	}
	if stat.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %o; want 600", stat.Mode().Perm())
	}
}

func TestExtractBaseBackupCorruptZstd(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	store.objects["bad.tar.zst"] = []byte("not-zstd")

	err := ExtractBaseBackup(context.Background(), store, "bad.tar.zst", t.TempDir())
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestExtractBaseBackupEmptyTar(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	store.objects["empty.tar.zst"] = buildTarZstdFixture(t, nil)
	dir := t.TempDir()

	if err := ExtractBaseBackup(context.Background(), store, "empty.tar.zst", dir); err != nil {
		t.Fatalf("ExtractBaseBackup(empty): %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected empty extraction dir, got %d entries", len(entries))
	}
}

func TestExtractBaseBackupTargetDirNotEmpty(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	store.objects["base.tar.zst"] = buildTarZstdFixture(t, []tarFixtureEntry{{
		header: tar.Header{Name: "PG_VERSION", Mode: 0o600, Size: int64(len("16\n"))},
		body:   []byte("16\n"),
	}})
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "already-there"), []byte("x"), 0o644); err != nil {
		t.Fatalf("seed dir: %v", err)
	}

	err := ExtractBaseBackup(context.Background(), store, "base.tar.zst", dir)
	if err == nil || !strings.Contains(err.Error(), "must be empty") {
		t.Fatalf("expected non-empty dir error, got %v", err)
	}
}

func TestExtractBaseBackupRejectsPathTraversal(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	store.objects["base.tar.zst"] = buildTarZstdFixture(t, []tarFixtureEntry{{
		header: tar.Header{Name: "../evil", Mode: 0o600, Size: int64(len("oops"))},
		body:   []byte("oops"),
	}})

	err := ExtractBaseBackup(context.Background(), store, "base.tar.zst", t.TempDir())
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "path traversal") {
		t.Fatalf("expected path traversal error, got %v", err)
	}
}

type downloadStore struct {
	objects  map[string][]byte
	getErr   map[string]error
	getCalls map[string]int
}

func newDownloadStore() *downloadStore {
	return &downloadStore{
		objects:  map[string][]byte{},
		getErr:   map[string]error{},
		getCalls: map[string]int{},
	}
}

func (s *downloadStore) PutObject(context.Context, string, io.Reader, int64, string) error {
	return errors.New("not implemented")
}

func (s *downloadStore) GetObject(_ context.Context, key string) (io.ReadCloser, int64, error) {
	s.getCalls[key]++
	if err := s.getErr[key]; err != nil {
		return nil, 0, err
	}
	data, ok := s.objects[key]
	if !ok {
		return nil, 0, errors.New("not found")
	}
	return io.NopCloser(bytes.NewReader(data)), int64(len(data)), nil
}

func (s *downloadStore) HeadObject(context.Context, string) (int64, error) {
	return 0, errors.New("not implemented")
}

func (s *downloadStore) ListObjects(context.Context, string) ([]StoreObject, error) {
	return nil, errors.New("not implemented")
}

func (s *downloadStore) DeleteObject(context.Context, string) error {
	return errors.New("not implemented")
}

func TestDownloadWALSegmentsSuccess(t *testing.T) {
	t.Parallel()

	store := newDownloadStore()
	segments := []WALSegment{
		{Timeline: 1, SegmentName: "000000010000000000000002", SizeBytes: 4},
		{Timeline: 1, SegmentName: "000000010000000000000003", SizeBytes: 5},
	}
	k1 := WALSegmentKey("archive", "proj1", "db1", 1, segments[0].SegmentName)
	k2 := WALSegmentKey("archive", "proj1", "db1", 1, segments[1].SegmentName)
	store.objects[k1] = []byte("wal1")
	store.objects[k2] = []byte("wal22")
	dir := t.TempDir()

	if err := DownloadWALSegments(context.Background(), store, segments, "archive", "proj1", "db1", dir); err != nil {
		t.Fatalf("DownloadWALSegments: %v", err)
	}
	if got := string(mustReadFile(t, filepath.Join(dir, segments[0].SegmentName))); got != "wal1" {
		t.Fatalf("segment1 data = %q; want wal1", got)
	}
	if got := string(mustReadFile(t, filepath.Join(dir, segments[1].SegmentName))); got != "wal22" {
		t.Fatalf("segment2 data = %q; want wal22", got)
	}
}

func TestDownloadWALSegmentsS3Error(t *testing.T) {
	t.Parallel()

	store := newDownloadStore()
	segments := []WALSegment{{Timeline: 1, SegmentName: "000000010000000000000002", SizeBytes: 4}}
	k1 := WALSegmentKey("archive", "proj1", "db1", 1, segments[0].SegmentName)
	store.getErr[k1] = errors.New("s3 unavailable")

	err := DownloadWALSegments(context.Background(), store, segments, "archive", "proj1", "db1", t.TempDir())
	if err == nil || !strings.Contains(err.Error(), segments[0].SegmentName) {
		t.Fatalf("expected segment-specific error, got %v", err)
	}
}

func TestDownloadWALSegmentsSkipsExistingBySize(t *testing.T) {
	t.Parallel()

	store := newDownloadStore()
	segments := []WALSegment{{Timeline: 1, SegmentName: "000000010000000000000002", SizeBytes: 2}}
	k1 := WALSegmentKey("archive", "proj1", "db1", 1, segments[0].SegmentName)
	store.objects[k1] = []byte("aa")
	dir := t.TempDir()
	existing := filepath.Join(dir, segments[0].SegmentName)
	if err := os.WriteFile(existing, []byte("zz"), 0o600); err != nil {
		t.Fatalf("seed existing segment: %v", err)
	}

	if err := DownloadWALSegments(context.Background(), store, segments, "archive", "proj1", "db1", dir); err != nil {
		t.Fatalf("DownloadWALSegments: %v", err)
	}
	if got := string(mustReadFile(t, existing)); got != "zz" {
		t.Fatalf("existing segment was overwritten: %q", got)
	}
	if calls := store.getCalls[k1]; calls != 0 {
		t.Fatalf("GetObject calls = %d; want 0 for skip-existing", calls)
	}
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	return b
}
