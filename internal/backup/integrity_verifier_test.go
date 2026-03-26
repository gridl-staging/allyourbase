package backup

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

// --- test doubles for integrity verification ---

type fakeIntegrityReportRepo struct {
	reports []VerificationReport
	saveErr error
}

func (r *fakeIntegrityReportRepo) Save(_ context.Context, report VerificationReport) error {
	if r.saveErr != nil {
		return r.saveErr
	}
	r.reports = append(r.reports, report)
	return nil
}

func (r *fakeIntegrityReportRepo) LatestByProject(_ context.Context, projectID, databaseID string) (*VerificationReport, error) {
	for i := len(r.reports) - 1; i >= 0; i-- {
		if r.reports[i].ProjectID == projectID && r.reports[i].DatabaseID == databaseID {
			return &r.reports[i], nil
		}
	}
	return nil, nil
}

// --- IntegrityVerifier tests ---

func TestIntegrityVerifierAllChecksPass(t *testing.T) {
	repo := newFakeRepo()
	manifestRepo := newFakeManifestRepo()
	walRepo := newFakeWALRepo()
	store := newFakeStore()
	reportRepo := &fakeIntegrityReportRepo{}
	notify := &captureNotifier{}

	// Seed a completed physical backup.
	startLSN := "0/1000000"
	endLSN := "0/2000000"
	completedAt := time.Now().UTC()
	repo.records["backup-1"] = &BackupRecord{
		ID:          "backup-1",
		ProjectID:   "proj1",
		DatabaseID:  "db1",
		BackupType:  "physical",
		Status:      StatusCompleted,
		ObjectKey:   "base/key.tar.zst",
		Checksum:    "abc123",
		StartLSN:    &startLSN,
		EndLSN:      &endLSN,
		CompletedAt: &completedAt,
	}

	// Seed a matching manifest.
	manifestRepo.manifests["backup-1"] = &BackupManifest{
		BackupID:  "backup-1",
		ObjectKey: "base/key.tar.zst",
		Checksum:  "abc123",
	}

	// Seed contiguous WAL segments covering from the backup's end LSN onward.
	walRepo.records[walRecordKey("proj1", "db1", 1, "000000010000000000000002")] = WALSegment{
		ProjectID:   "proj1",
		DatabaseID:  "db1",
		Timeline:    1,
		SegmentName: "000000010000000000000002",
		StartLSN:    "0/2000000",
		EndLSN:      "0/3000000",
	}
	walRepo.records[walRecordKey("proj1", "db1", 1, "000000010000000000000003")] = WALSegment{
		ProjectID:   "proj1",
		DatabaseID:  "db1",
		Timeline:    1,
		SegmentName: "000000010000000000000003",
		StartLSN:    "0/3000000",
		EndLSN:      "0/4000000",
	}

	// S3 objects for the WAL segments.
	store.objects[WALSegmentKey("pitr", "proj1", "db1", 1, "000000010000000000000002")] = []byte("wal2")
	store.objects[WALSegmentKey("pitr", "proj1", "db1", 1, "000000010000000000000003")] = []byte("wal3")

	verifier := NewIntegrityVerifier(repo, manifestRepo, walRepo, store, reportRepo, notify, "pitr")
	report, err := verifier.Verify(context.Background(), "proj1", "db1")
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if report.Status != "pass" {
		t.Fatalf("status = %q; want pass", report.Status)
	}
	for _, check := range report.Checks {
		if !check.Passed {
			t.Errorf("check %q failed: %s", check.Name, check.Message)
		}
	}
	if len(notify.events) != 0 {
		t.Fatalf("expected no failure notifications, got %d", len(notify.events))
	}
	if len(notify.alertEvents) != 0 {
		t.Fatalf("expected no alert notifications, got %d", len(notify.alertEvents))
	}
	if len(reportRepo.reports) != 1 {
		t.Fatalf("expected 1 persisted report, got %d", len(reportRepo.reports))
	}
}

func TestIntegrityVerifierWALGapDetected(t *testing.T) {
	repo := newFakeRepo()
	walRepo := newFakeWALRepo()
	store := newFakeStore()
	reportRepo := &fakeIntegrityReportRepo{}
	notify := &captureNotifier{}

	startLSN := "0/1000000"
	endLSN := "0/2000000"
	completedAt := time.Now().UTC()
	repo.records["backup-1"] = &BackupRecord{
		ID:          "backup-1",
		ProjectID:   "proj1",
		DatabaseID:  "db1",
		BackupType:  "physical",
		Status:      StatusCompleted,
		ObjectKey:   "base/key.tar.zst",
		Checksum:    "abc",
		StartLSN:    &startLSN,
		EndLSN:      &endLSN,
		CompletedAt: &completedAt,
	}

	// WAL segments with a gap: segment 2 ends at 0/3000000, segment 4 starts at 0/4000000.
	walRepo.records[walRecordKey("proj1", "db1", 1, "000000010000000000000002")] = WALSegment{
		ProjectID:   "proj1",
		DatabaseID:  "db1",
		Timeline:    1,
		SegmentName: "000000010000000000000002",
		StartLSN:    "0/2000000",
		EndLSN:      "0/3000000",
	}
	walRepo.records[walRecordKey("proj1", "db1", 1, "000000010000000000000004")] = WALSegment{
		ProjectID:   "proj1",
		DatabaseID:  "db1",
		Timeline:    1,
		SegmentName: "000000010000000000000004",
		StartLSN:    "0/4000000",
		EndLSN:      "0/5000000",
	}

	// S3 objects exist for both segments.
	store.objects[WALSegmentKey("pitr", "proj1", "db1", 1, "000000010000000000000002")] = []byte("wal")
	store.objects[WALSegmentKey("pitr", "proj1", "db1", 1, "000000010000000000000004")] = []byte("wal")

	// Matching manifest.
	manifestRepo := newFakeManifestRepo()
	manifestRepo.manifests["backup-1"] = &BackupManifest{
		BackupID:  "backup-1",
		ObjectKey: "base/key.tar.zst",
		Checksum:  "abc",
	}

	verifier := NewIntegrityVerifier(repo, manifestRepo, walRepo, store, reportRepo, notify, "pitr")
	report, err := verifier.Verify(context.Background(), "proj1", "db1")
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if report.Status != "fail" {
		t.Fatalf("status = %q; want fail", report.Status)
	}
	found := false
	for _, check := range report.Checks {
		if check.Name == "wal_contiguity" && !check.Passed {
			found = true
		}
	}
	if !found {
		t.Fatal("expected wal_contiguity check to fail")
	}
	if len(notify.events) != 1 {
		t.Fatalf("expected 1 failure notification, got %d", len(notify.events))
	}
	if len(notify.alertEvents) != 1 {
		t.Fatalf("expected 1 alert notification, got %d", len(notify.alertEvents))
	}
	if notify.alertEvents[0].AlertType != "wal_gap_detected" {
		t.Fatalf("alert type = %s; want wal_gap_detected", notify.alertEvents[0].AlertType)
	}
}

func TestIntegrityVerifierLatestWALRepoError(t *testing.T) {
	repo := newFakeRepo()
	walRepo := newFakeWALRepo()
	walRepo.listRangeErr = errors.New("db timeout")
	store := newFakeStore()
	reportRepo := &fakeIntegrityReportRepo{}
	notify := &captureNotifier{}

	startLSN := "0/1000000"
	endLSN := "0/2000000"
	completedAt := time.Now().UTC()
	repo.records["backup-1"] = &BackupRecord{
		ID:          "backup-1",
		ProjectID:   "proj1",
		DatabaseID:  "db1",
		BackupType:  "physical",
		Status:      StatusCompleted,
		ObjectKey:   "base/key.tar.zst",
		Checksum:    "abc",
		StartLSN:    &startLSN,
		EndLSN:      &endLSN,
		CompletedAt: &completedAt,
	}

	verifier := NewIntegrityVerifier(repo, newFakeManifestRepo(), walRepo, store, reportRepo, notify, "pitr")
	report, err := verifier.Verify(context.Background(), "proj1", "db1")
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if report.Status != "fail" {
		t.Fatalf("status = %q; want fail", report.Status)
	}

	found := false
	for _, check := range report.Checks {
		if check.Name == "wal_contiguity" && !check.Passed {
			found = true
			if !containsFold(check.Message, "failed to get latest WAL segment") {
				t.Fatalf("unexpected wal_contiguity message: %s", check.Message)
			}
		}
	}
	if !found {
		t.Fatal("expected wal_contiguity check to fail")
	}
}

func TestIntegrityVerifierMissingWALObject(t *testing.T) {
	repo := newFakeRepo()
	walRepo := newFakeWALRepo()
	store := newFakeStore() // empty — no S3 objects
	reportRepo := &fakeIntegrityReportRepo{}

	// Seed a WAL segment but don't add the corresponding object to the store.
	walRepo.records[walRecordKey("proj1", "db1", 1, "000000010000000000000001")] = WALSegment{
		ProjectID:   "proj1",
		DatabaseID:  "db1",
		Timeline:    1,
		SegmentName: "000000010000000000000001",
		StartLSN:    "0/1000000",
		EndLSN:      "0/2000000",
	}

	verifier := NewIntegrityVerifier(repo, newFakeManifestRepo(), walRepo, store, reportRepo, NoopNotifier{}, "pitr")
	report, err := verifier.Verify(context.Background(), "proj1", "db1")
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if report.Status != "fail" {
		t.Fatalf("status = %q; want fail", report.Status)
	}

	found := false
	for _, check := range report.Checks {
		if check.Name == "wal_object_integrity" && !check.Passed {
			found = true
		}
	}
	if !found {
		t.Fatal("expected wal_object_integrity check to fail")
	}
}

func TestIntegrityVerifierMissingManifest(t *testing.T) {
	repo := newFakeRepo()
	manifestRepo := newFakeManifestRepo() // empty — no manifests
	walRepo := newFakeWALRepo()
	store := newFakeStore()
	notify := &captureNotifier{}
	reportRepo := &fakeIntegrityReportRepo{}

	completedAt := time.Now().UTC()
	startLSN := "0/1000000"
	endLSN := "0/2000000"
	repo.records["backup-1"] = &BackupRecord{
		ID:          "backup-1",
		ProjectID:   "proj1",
		DatabaseID:  "db1",
		BackupType:  "physical",
		Status:      StatusCompleted,
		ObjectKey:   "base/key.tar.zst",
		Checksum:    "abc",
		StartLSN:    &startLSN,
		EndLSN:      &endLSN,
		CompletedAt: &completedAt,
	}

	verifier := NewIntegrityVerifier(repo, manifestRepo, walRepo, store, reportRepo, notify, "pitr")
	report, err := verifier.Verify(context.Background(), "proj1", "db1")
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if report.Status != "fail" {
		t.Fatalf("status = %q; want fail", report.Status)
	}

	found := false
	for _, check := range report.Checks {
		if check.Name == "backup_manifest_integrity" && !check.Passed {
			found = true
			if !containsFold(check.Message, "manifest not found") {
				t.Fatalf("expected 'manifest not found' in message, got: %s", check.Message)
			}
		}
	}
	if !found {
		t.Fatal("expected backup_manifest_integrity check to fail")
	}
	if len(notify.alertEvents) != 2 {
		t.Fatalf("expected 2 alert notifications, got %d", len(notify.alertEvents))
	}
	if notify.alertEvents[0].AlertType != "wal_gap_detected" {
		t.Fatalf("alert type = %s; want wal_gap_detected", notify.alertEvents[0].AlertType)
	}
	if notify.alertEvents[1].AlertType != "checksum_mismatch" {
		t.Fatalf("alert type = %s; want checksum_mismatch", notify.alertEvents[1].AlertType)
	}
}

func TestIntegrityVerifierChecksumMismatch(t *testing.T) {
	repo := newFakeRepo()
	manifestRepo := newFakeManifestRepo()
	walRepo := newFakeWALRepo()
	store := newFakeStore()
	reportRepo := &fakeIntegrityReportRepo{}
	notify := &captureNotifier{}

	completedAt := time.Now().UTC()
	startLSN := "0/1000000"
	endLSN := "0/2000000"
	repo.records["backup-1"] = &BackupRecord{
		ID:          "backup-1",
		ProjectID:   "proj1",
		DatabaseID:  "db1",
		BackupType:  "physical",
		Status:      StatusCompleted,
		ObjectKey:   "base/key.tar.zst",
		Checksum:    "abc",
		StartLSN:    &startLSN,
		EndLSN:      &endLSN,
		CompletedAt: &completedAt,
	}

	// Manifest with divergent checksum.
	manifestRepo.manifests["backup-1"] = &BackupManifest{
		BackupID:  "backup-1",
		ObjectKey: "base/key.tar.zst",
		Checksum:  "different-checksum",
	}

	verifier := NewIntegrityVerifier(repo, manifestRepo, walRepo, store, reportRepo, notify, "pitr")
	report, err := verifier.Verify(context.Background(), "proj1", "db1")
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if report.Status != "fail" {
		t.Fatalf("status = %q; want fail", report.Status)
	}

	found := false
	for _, check := range report.Checks {
		if check.Name == "backup_manifest_integrity" && !check.Passed {
			found = true
			if !containsFold(check.Message, "checksum mismatch") {
				t.Fatalf("expected 'checksum mismatch' in message, got: %s", check.Message)
			}
		}
	}
	if !found {
		t.Fatal("expected backup_manifest_integrity check to fail")
	}
	if len(notify.alertEvents) != 2 {
		t.Fatalf("expected 2 alert notifications, got %d", len(notify.alertEvents))
	}
	if notify.alertEvents[0].AlertType != "wal_gap_detected" {
		t.Fatalf("alert type = %s; want wal_gap_detected", notify.alertEvents[0].AlertType)
	}
	if notify.alertEvents[1].AlertType != "checksum_mismatch" {
		t.Fatalf("alert type = %s; want checksum_mismatch", notify.alertEvents[1].AlertType)
	}
}

func TestIntegrityVerifierReportSaveFailure(t *testing.T) {
	reportRepo := &fakeIntegrityReportRepo{saveErr: fmt.Errorf("db timeout")}
	verifier := NewIntegrityVerifier(newFakeRepo(), newFakeManifestRepo(), newFakeWALRepo(), newFakeStore(), reportRepo, NoopNotifier{}, "pitr")

	_, err := verifier.Verify(context.Background(), "proj1", "db1")
	if err == nil {
		t.Fatal("expected error when report save fails")
	}
	if !containsFold(err.Error(), "saving integrity report") {
		t.Fatalf("expected 'saving integrity report' in error, got: %v", err)
	}
}

func TestIntegrityVerifierNoBackupsNoSegments(t *testing.T) {
	reportRepo := &fakeIntegrityReportRepo{}
	verifier := NewIntegrityVerifier(newFakeRepo(), newFakeManifestRepo(), newFakeWALRepo(), newFakeStore(), reportRepo, NoopNotifier{}, "pitr")

	report, err := verifier.Verify(context.Background(), "proj1", "db1")
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if report.Status != "pass" {
		t.Fatalf("status = %q; want pass (empty state should pass)", report.Status)
	}
	for _, check := range report.Checks {
		if !check.Passed {
			t.Errorf("check %q failed on empty state: %s", check.Name, check.Message)
		}
	}
}
