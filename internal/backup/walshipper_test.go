package backup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"sort"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

type fakeWALRepo struct {
	records         map[string]WALSegment
	recordErr       error
	getErr          error
	listRangeErr    error
	listRangeResult []WALSegment
}

func newFakeWALRepo() *fakeWALRepo {
	return &fakeWALRepo{records: map[string]WALSegment{}}
}

func walRecordKey(projectID, databaseID string, timeline int, segmentName string) string {
	return fmt.Sprintf("%s|%s|%d|%s", projectID, databaseID, timeline, segmentName)
}

func (f *fakeWALRepo) Record(_ context.Context, seg WALSegment) error {
	if f.recordErr != nil {
		return f.recordErr
	}
	f.records[walRecordKey(seg.ProjectID, seg.DatabaseID, seg.Timeline, seg.SegmentName)] = seg
	return nil
}

func (f *fakeWALRepo) GetByName(_ context.Context, projectID, databaseID string, timeline int, segmentName string) (*WALSegment, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	seg, ok := f.records[walRecordKey(projectID, databaseID, timeline, segmentName)]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	return &seg, nil
}

func (f *fakeWALRepo) ListRange(_ context.Context, projectID, databaseID string, _, _ string) ([]WALSegment, error) {
	if f.listRangeErr != nil {
		return nil, f.listRangeErr
	}
	if f.listRangeResult != nil {
		out := append([]WALSegment(nil), f.listRangeResult...)
		return out, nil
	}

	var out []WALSegment
	for _, seg := range f.records {
		if seg.ProjectID == projectID && seg.DatabaseID == databaseID {
			out = append(out, seg)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].StartLSN == out[j].StartLSN {
			return out[i].SegmentName < out[j].SegmentName
		}
		return out[i].StartLSN < out[j].StartLSN
	})
	return out, nil
}

func (f *fakeWALRepo) ListOlderThan(_ context.Context, projectID, databaseID string, before time.Time) ([]WALSegment, error) {
	var out []WALSegment
	for _, seg := range f.records {
		if seg.ProjectID == projectID && seg.DatabaseID == databaseID && seg.ArchivedAt.Before(before) {
			out = append(out, seg)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ArchivedAt.Equal(out[j].ArchivedAt) {
			return out[i].SegmentName < out[j].SegmentName
		}
		return out[i].ArchivedAt.Before(out[j].ArchivedAt)
	})
	return out, nil
}

func (f *fakeWALRepo) SumSizeBytes(_ context.Context, projectID, databaseID string) (int64, error) {
	var total int64
	for _, seg := range f.records {
		if seg.ProjectID == projectID && seg.DatabaseID == databaseID {
			total += seg.SizeBytes
		}
	}
	return total, nil
}

func (f *fakeWALRepo) Delete(_ context.Context, id string) error {
	if _, ok := f.records[id]; !ok {
		return fmt.Errorf("WAL segment %q not found", id)
	}
	delete(f.records, id)
	return nil
}

func (f *fakeWALRepo) LatestByProject(_ context.Context, projectID, databaseID string) (*WALSegment, error) {
	segs, err := f.ListRange(context.Background(), projectID, databaseID, "0/0", "FFFFFFFF/FFFFFFFF")
	if err != nil {
		return nil, err
	}
	if len(segs) == 0 {
		return nil, fmt.Errorf("scanning WAL segment: %w", pgx.ErrNoRows)
	}
	latest := segs[len(segs)-1]
	return &latest, nil
}

func (f *fakeWALRepo) CoveringSegment(_ context.Context, projectID, databaseID, lsn string) (*WALSegment, error) {
	var result *WALSegment
	for _, seg := range f.records {
		if seg.ProjectID == projectID && seg.DatabaseID == databaseID {
			if seg.StartLSN <= lsn && lsn < seg.EndLSN {
				if result == nil || seg.StartLSN > result.StartLSN {
					result = &seg
				}
			}
		}
	}
	if result == nil {
		return nil, fmt.Errorf("no segment covering LSN %s", lsn)
	}
	return result, nil
}

func TestFakeWALRepoListOlderThan(t *testing.T) {
	repo := newFakeWALRepo()
	now := time.Now().UTC()
	repo.records["a"] = WALSegment{
		ID: "a", ProjectID: "proj1", DatabaseID: "db1", SegmentName: "0001", ArchivedAt: now.Add(-10 * time.Minute),
	}
	repo.records["b"] = WALSegment{
		ID: "b", ProjectID: "proj1", DatabaseID: "db1", SegmentName: "0002", ArchivedAt: now.Add(-30 * time.Minute),
	}
	repo.records["c"] = WALSegment{
		ID: "c", ProjectID: "proj1", DatabaseID: "db1", SegmentName: "0003", ArchivedAt: now.Add(-5 * time.Minute),
	}

	got, err := repo.ListOlderThan(context.Background(), "proj1", "db1", now.Add(-15*time.Minute))
	if err != nil {
		t.Fatalf("ListOlderThan: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len = %d; want 1", len(got))
	}
	if got[0].ID != "b" {
		t.Fatalf("unexpected order: %#v", got)
	}
}

func TestFakeWALRepoSumSizeBytes(t *testing.T) {
	repo := newFakeWALRepo()
	repo.records["a"] = WALSegment{
		ProjectID: "proj1", DatabaseID: "db1", SizeBytes: 100,
	}
	repo.records["b"] = WALSegment{
		ProjectID: "proj1", DatabaseID: "db1", SizeBytes: 200,
	}
	repo.records["c"] = WALSegment{
		ProjectID: "other", DatabaseID: "db1", SizeBytes: 1000,
	}
	total, err := repo.SumSizeBytes(context.Background(), "proj1", "db1")
	if err != nil {
		t.Fatalf("SumSizeBytes: %v", err)
	}
	if total != 300 {
		t.Fatalf("total = %d; want 300", total)
	}
}

func TestFakeWALRepoDelete(t *testing.T) {
	repo := newFakeWALRepo()
	repo.records["a"] = WALSegment{ProjectID: "proj1", DatabaseID: "db1", SegmentName: "s1"}
	if err := repo.Delete(context.Background(), "a"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, ok := repo.records["a"]; ok {
		t.Fatal("expected segment removed")
	}
}

func TestWALShipperShipSuccess(t *testing.T) {
	const name = "000000010000000000000002"
	data := []byte("wal segment bytes")
	path := writeTempWALFile(t, data)

	store := newFakeStore()
	repo := newFakeWALRepo()
	shipper := NewWALShipper(store, repo, PITRConfig{ArchivePrefix: "pitr"}, "proj1", "db1", NoopNotifier{})

	if err := shipper.Ship(context.Background(), path, name); err != nil {
		t.Fatalf("Ship: %v", err)
	}

	key := WALSegmentKey("pitr", "proj1", "db1", 1, name)
	if store.putCalled != 1 {
		t.Fatalf("putCalled = %d; want 1", store.putCalled)
	}
	if _, ok := store.objects[key]; !ok {
		t.Fatalf("uploaded object %q not found in fake store", key)
	}

	rec, ok := repo.records[walRecordKey("proj1", "db1", 1, name)]
	if !ok {
		t.Fatalf("expected WAL segment to be recorded")
	}
	if rec.StartLSN != "0/2000000" || rec.EndLSN != "0/3000000" {
		t.Fatalf("LSN range = %s -> %s; want 0/2000000 -> 0/3000000", rec.StartLSN, rec.EndLSN)
	}
	if rec.Checksum != sha256Hex(data) {
		t.Fatalf("checksum = %q; want %q", rec.Checksum, sha256Hex(data))
	}
}

func TestWALShipperShipIdempotentReshipSameChecksum(t *testing.T) {
	const name = "000000010000000000000003"
	data := []byte("same bytes")
	path := writeTempWALFile(t, data)

	store := newFakeStore()
	repo := newFakeWALRepo()
	key := WALSegmentKey("pitr", "proj1", "db1", 1, name)
	store.objects[key] = append([]byte(nil), data...)
	repo.records[walRecordKey("proj1", "db1", 1, name)] = WALSegment{
		ProjectID:   "proj1",
		DatabaseID:  "db1",
		Timeline:    1,
		SegmentName: name,
		StartLSN:    "0/3000000",
		EndLSN:      "0/4000000",
		Checksum:    sha256Hex(data),
		SizeBytes:   int64(len(data)),
		ArchivedAt:  time.Now().UTC(),
	}

	shipper := NewWALShipper(store, repo, PITRConfig{ArchivePrefix: "pitr"}, "proj1", "db1", NoopNotifier{})
	if err := shipper.Ship(context.Background(), path, name); err != nil {
		t.Fatalf("Ship (idempotent): %v", err)
	}
	if store.putCalled != 0 {
		t.Fatalf("putCalled = %d; want 0 for idempotent re-ship", store.putCalled)
	}
}

func TestWALShipperShipChecksumMismatch(t *testing.T) {
	const name = "000000010000000000000004"
	data := []byte("new bytes")
	path := writeTempWALFile(t, data)

	store := newFakeStore()
	repo := newFakeWALRepo()
	key := WALSegmentKey("pitr", "proj1", "db1", 1, name)
	store.objects[key] = []byte("existing bytes")
	repo.records[walRecordKey("proj1", "db1", 1, name)] = WALSegment{
		ProjectID:   "proj1",
		DatabaseID:  "db1",
		Timeline:    1,
		SegmentName: name,
		Checksum:    "different-checksum",
	}

	shipper := NewWALShipper(store, repo, PITRConfig{ArchivePrefix: "pitr"}, "proj1", "db1", NoopNotifier{})
	err := shipper.Ship(context.Background(), path, name)
	if err == nil {
		t.Fatal("expected checksum mismatch error")
	}
	if !containsFold(err.Error(), "checksum") {
		t.Fatalf("expected checksum error, got: %v", err)
	}
	if store.putCalled != 0 {
		t.Fatalf("putCalled = %d; want 0 when checksum mismatches", store.putCalled)
	}
}

func TestWALShipperShipInvalidWALFilename(t *testing.T) {
	path := writeTempWALFile(t, []byte("bytes"))
	store := newFakeStore()
	repo := newFakeWALRepo()
	shipper := NewWALShipper(store, repo, PITRConfig{ArchivePrefix: "pitr"}, "proj1", "db1", NoopNotifier{})

	err := shipper.Ship(context.Background(), path, "000000010000000000000001.partial")
	if err == nil {
		t.Fatal("expected invalid filename error")
	}
	if store.putCalled != 0 {
		t.Fatalf("putCalled = %d; want 0 for invalid filename", store.putCalled)
	}
}

func TestWALShipperDetectGapsContiguousChain(t *testing.T) {
	repo := newFakeWALRepo()
	repo.listRangeResult = []WALSegment{
		{SegmentName: "000000010000000000000001", StartLSN: "0/1000000", EndLSN: "0/2000000"},
		{SegmentName: "000000010000000000000002", StartLSN: "0/2000000", EndLSN: "0/3000000"},
		{SegmentName: "000000010000000000000003", StartLSN: "0/3000000", EndLSN: "0/4000000"},
	}
	notify := &captureNotifier{}
	shipper := NewWALShipper(newFakeStore(), repo, PITRConfig{}, "proj1", "db1", notify)

	gaps, err := shipper.DetectGaps(context.Background())
	if err != nil {
		t.Fatalf("DetectGaps: %v", err)
	}
	if len(gaps) != 0 {
		t.Fatalf("expected no gaps, got %+v", gaps)
	}
	if len(notify.events) != 0 {
		t.Fatalf("expected no notifications, got %d", len(notify.events))
	}
}

func TestWALShipperDetectGapsSingleGap(t *testing.T) {
	repo := newFakeWALRepo()
	repo.listRangeResult = []WALSegment{
		{SegmentName: "000000010000000000000001", StartLSN: "0/1000000", EndLSN: "0/2000000"},
		{SegmentName: "000000010000000000000002", StartLSN: "0/2100000", EndLSN: "0/3000000"},
	}
	notify := &captureNotifier{}
	shipper := NewWALShipper(newFakeStore(), repo, PITRConfig{}, "proj1", "db1", notify)

	gaps, err := shipper.DetectGaps(context.Background())
	if err != nil {
		t.Fatalf("DetectGaps: %v", err)
	}
	if len(gaps) != 1 {
		t.Fatalf("expected 1 gap, got %d", len(gaps))
	}
	if gaps[0].ExpectedLSN != "0/2000000" || gaps[0].ActualLSN != "0/2100000" {
		t.Fatalf("unexpected gap: %+v", gaps[0])
	}
	if len(notify.events) != 1 || notify.events[0].Stage != "wal_gap_detection" {
		t.Fatalf("expected wal_gap_detection notification, got %+v", notify.events)
	}
}

func TestWALShipperDetectGapsMultipleGaps(t *testing.T) {
	repo := newFakeWALRepo()
	repo.listRangeResult = []WALSegment{
		{SegmentName: "000000010000000000000001", StartLSN: "0/1000000", EndLSN: "0/2000000"},
		{SegmentName: "000000010000000000000002", StartLSN: "0/2200000", EndLSN: "0/3000000"},
		{SegmentName: "000000010000000000000003", StartLSN: "0/3100000", EndLSN: "0/4000000"},
	}
	shipper := NewWALShipper(newFakeStore(), repo, PITRConfig{}, "proj1", "db1", NoopNotifier{})

	gaps, err := shipper.DetectGaps(context.Background())
	if err != nil {
		t.Fatalf("DetectGaps: %v", err)
	}
	if len(gaps) != 2 {
		t.Fatalf("expected 2 gaps, got %d (%+v)", len(gaps), gaps)
	}
}

func TestWALShipperDetectGapsEmptySegmentList(t *testing.T) {
	repo := newFakeWALRepo()
	repo.listRangeResult = []WALSegment{}
	shipper := NewWALShipper(newFakeStore(), repo, PITRConfig{}, "proj1", "db1", NoopNotifier{})

	gaps, err := shipper.DetectGaps(context.Background())
	if err != nil {
		t.Fatalf("DetectGaps: %v", err)
	}
	if len(gaps) != 0 {
		t.Fatalf("expected no gaps for empty list, got %+v", gaps)
	}
}

func writeTempWALFile(t *testing.T, data []byte) string {
	t.Helper()
	f, err := os.CreateTemp("", "wal-segment-*")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		t.Fatalf("write temp WAL file: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close temp WAL file: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(f.Name()) })
	return f.Name()
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
