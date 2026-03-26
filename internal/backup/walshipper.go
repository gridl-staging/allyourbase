package backup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"time"
)

const (
	minLSNForScan = "0/0"
	maxLSNForScan = "FFFFFFFF/FFFFFFFF"
)

// WALGap represents a discontinuity in archived WAL coverage.
type WALGap struct {
	ExpectedLSN string
	ActualLSN   string
	After       string
}

// WALShipper uploads WAL segments to object storage and records metadata.
type WALShipper struct {
	store      Store
	walRepo    WALSegmentRepo
	cfg        PITRConfig
	projectID  string
	databaseID string
	notify     Notifier
}

// NewWALShipper constructs a WAL shipper.
func NewWALShipper(store Store, walRepo WALSegmentRepo, cfg PITRConfig, projectID, databaseID string, notify Notifier) *WALShipper {
	if notify == nil {
		notify = NoopNotifier{}
	}
	return &WALShipper{
		store:      store,
		walRepo:    walRepo,
		cfg:        cfg,
		projectID:  projectID,
		databaseID: databaseID,
		notify:     notify,
	}
}

// Ship archives one WAL segment file and records metadata.
func (w *WALShipper) Ship(ctx context.Context, walFilePath string, walFileName string) error {
	parsed, err := ParseWALFileName(walFileName)
	if err != nil {
		return fmt.Errorf("parsing WAL filename: %w", err)
	}

	file, err := os.Open(walFilePath)
	if err != nil {
		return fmt.Errorf("opening WAL file %q: %w", walFilePath, err)
	}
	defer file.Close()

	hash := sha256.New()
	sizeBytes, err := io.Copy(hash, file)
	if err != nil {
		return fmt.Errorf("hashing WAL file %q: %w", walFilePath, err)
	}
	checksum := hex.EncodeToString(hash.Sum(nil))

	objectKey := WALSegmentKey(w.cfg.ArchivePrefix, w.projectID, w.databaseID, parsed.Timeline, parsed.OriginalName)

	if _, err := w.store.HeadObject(ctx, objectKey); err == nil {
		existing, lookupErr := w.walRepo.GetByName(ctx, w.projectID, w.databaseID, parsed.Timeline, parsed.OriginalName)
		if lookupErr != nil {
			return fmt.Errorf("WAL object already exists but metadata lookup failed: %w", lookupErr)
		}
		if existing.Checksum != checksum {
			return fmt.Errorf("checksum mismatch for existing WAL segment %q: existing=%s new=%s", parsed.OriginalName, existing.Checksum, checksum)
		}
		return nil
	}

	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("rewinding WAL file %q: %w", walFilePath, err)
	}
	if err := w.store.PutObject(ctx, objectKey, file, sizeBytes, "application/octet-stream"); err != nil {
		return fmt.Errorf("uploading WAL segment %q: %w", parsed.OriginalName, err)
	}

	if err := w.walRepo.Record(ctx, WALSegment{
		ProjectID:   w.projectID,
		DatabaseID:  w.databaseID,
		Timeline:    parsed.Timeline,
		SegmentName: parsed.OriginalName,
		StartLSN:    parsed.StartLSN(),
		EndLSN:      parsed.EndLSN(),
		Checksum:    checksum,
		SizeBytes:   sizeBytes,
		ArchivedAt:  time.Now().UTC(),
	}); err != nil {
		return fmt.Errorf("recording WAL segment metadata: %w", err)
	}

	return nil
}

// DetectGaps scans archived WAL metadata for discontinuities.
func (w *WALShipper) DetectGaps(ctx context.Context) ([]WALGap, error) {
	segments, err := w.walRepo.ListRange(ctx, w.projectID, w.databaseID, minLSNForScan, maxLSNForScan)
	if err != nil {
		return nil, fmt.Errorf("listing WAL segments for gap detection: %w", err)
	}

	gaps := make([]WALGap, 0)
	for i := 0; i+1 < len(segments); i++ {
		expected := segments[i].EndLSN
		actual := segments[i+1].StartLSN
		if expected == actual {
			continue
		}
		gaps = append(gaps, WALGap{
			ExpectedLSN: expected,
			ActualLSN:   actual,
			After:       segments[i].SegmentName,
		})
	}

	if len(gaps) > 0 {
		w.notify.OnFailure(ctx, FailureEvent{
			DBName:    w.databaseID,
			Stage:     "wal_gap_detection",
			Err:       fmt.Errorf("detected %d WAL gap(s) for project=%s database=%s", len(gaps), w.projectID, w.databaseID),
			Timestamp: time.Now().UTC(),
		})
	}

	return gaps, nil
}
