package backup

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

// --- test doubles for manifest ---

type fakeManifestRepo struct {
	manifests map[string]*BackupManifest
	upsertErr error
}

func newFakeManifestRepo() *fakeManifestRepo {
	return &fakeManifestRepo{manifests: map[string]*BackupManifest{}}
}

func (r *fakeManifestRepo) Upsert(_ context.Context, m BackupManifest) error {
	if r.upsertErr != nil {
		return r.upsertErr
	}
	r.manifests[m.BackupID] = &m
	return nil
}

func (r *fakeManifestRepo) GetByBackupID(_ context.Context, backupID string) (*BackupManifest, error) {
	m, ok := r.manifests[backupID]
	if !ok {
		return nil, nil // not found
	}
	return m, nil
}

func (r *fakeManifestRepo) ListByProjectDatabase(_ context.Context, projectID, databaseID string) ([]BackupManifest, error) {
	var out []BackupManifest
	for _, m := range r.manifests {
		if m.ProjectID == projectID && m.DatabaseID == databaseID {
			out = append(out, *m)
		}
	}
	return out, nil
}

// --- ManifestWriter tests ---

func TestManifestWriterWriteForBackupSuccess(t *testing.T) {
	store := newFakeStore()
	manifestRepo := newFakeManifestRepo()
	walRepo := newFakeWALRepo()

	// Seed a WAL segment covering the backup's start_lsn.
	walRepo.records[walRecordKey("proj1", "db1", 1, "000000010000000000000001")] = WALSegment{
		ProjectID:   "proj1",
		DatabaseID:  "db1",
		Timeline:    1,
		SegmentName: "000000010000000000000001",
		StartLSN:    "0/1000000",
		EndLSN:      "0/2000000",
	}

	writer := NewManifestWriter(store, manifestRepo, walRepo, PITRConfig{ArchivePrefix: "pitr"})

	startLSN := "0/1500000"
	endLSN := "0/2500000"
	rec := &BackupRecord{
		ID:         "backup-1",
		ProjectID:  "proj1",
		DatabaseID: "db1",
		ObjectKey:  "projects/proj1/db/db1/base/2025/01/01/20250101T030000Z_0_1500000.tar.zst",
		StartLSN:   &startLSN,
		EndLSN:     &endLSN,
		Checksum:   "abc123",
	}

	if err := writer.WriteForBackup(context.Background(), rec); err != nil {
		t.Fatalf("WriteForBackup: %v", err)
	}

	// Verify manifest was uploaded to S3.
	if store.putCalled != 1 {
		t.Fatalf("putCalled = %d; want 1", store.putCalled)
	}
	if store.lastType != "application/json" {
		t.Fatalf("content type = %q; want application/json", store.lastType)
	}

	// Verify manifest was persisted to DB.
	m, ok := manifestRepo.manifests["backup-1"]
	if !ok {
		t.Fatal("manifest not persisted to repo")
	}
	if m.Timeline != 1 {
		t.Fatalf("timeline = %d; want 1", m.Timeline)
	}
	if m.ObjectKey != rec.ObjectKey {
		t.Fatalf("object key = %q; want %q", m.ObjectKey, rec.ObjectKey)
	}
	if m.Checksum != "abc123" {
		t.Fatalf("checksum = %q; want abc123", m.Checksum)
	}
}

func TestManifestWriterIdempotentRewrite(t *testing.T) {
	store := newFakeStore()
	manifestRepo := newFakeManifestRepo()
	walRepo := newFakeWALRepo()

	walRepo.records[walRecordKey("proj1", "db1", 1, "000000010000000000000001")] = WALSegment{
		ProjectID:  "proj1",
		DatabaseID: "db1",
		Timeline:   1,
		StartLSN:   "0/1000000",
		EndLSN:     "0/2000000",
	}

	writer := NewManifestWriter(store, manifestRepo, walRepo, PITRConfig{ArchivePrefix: "pitr"})

	startLSN := "0/1500000"
	endLSN := "0/2500000"
	rec := &BackupRecord{
		ID: "backup-1", ProjectID: "proj1", DatabaseID: "db1",
		ObjectKey: "base/key.tar.zst", StartLSN: &startLSN, EndLSN: &endLSN, Checksum: "abc",
	}

	// First write.
	if err := writer.WriteForBackup(context.Background(), rec); err != nil {
		t.Fatalf("first write: %v", err)
	}

	// Second write with same content — should be a no-op.
	store.putCalled = 0
	if err := writer.WriteForBackup(context.Background(), rec); err != nil {
		t.Fatalf("idempotent rewrite: %v", err)
	}
	if store.putCalled != 0 {
		t.Fatalf("putCalled = %d; want 0 for idempotent rewrite", store.putCalled)
	}
}

func TestManifestWriterDivergentContentReturnsError(t *testing.T) {
	store := newFakeStore()
	manifestRepo := newFakeManifestRepo()
	walRepo := newFakeWALRepo()

	walRepo.records[walRecordKey("proj1", "db1", 1, "000000010000000000000001")] = WALSegment{
		ProjectID:  "proj1",
		DatabaseID: "db1",
		Timeline:   1,
		StartLSN:   "0/1000000",
		EndLSN:     "0/2000000",
	}

	writer := NewManifestWriter(store, manifestRepo, walRepo, PITRConfig{ArchivePrefix: "pitr"})

	startLSN := "0/1500000"
	endLSN := "0/2500000"
	rec := &BackupRecord{
		ID: "backup-1", ProjectID: "proj1", DatabaseID: "db1",
		ObjectKey: "base/key.tar.zst", StartLSN: &startLSN, EndLSN: &endLSN, Checksum: "abc",
	}

	// First write.
	if err := writer.WriteForBackup(context.Background(), rec); err != nil {
		t.Fatalf("first write: %v", err)
	}

	// Second write with divergent checksum — should error.
	rec.Checksum = "different-checksum"
	err := writer.WriteForBackup(context.Background(), rec)
	if err == nil {
		t.Fatal("expected error for divergent content")
	}
	if !containsFold(err.Error(), "mismatch") {
		t.Fatalf("expected mismatch error, got: %v", err)
	}
}

func TestManifestWriterTimelineLookupFailure(t *testing.T) {
	store := newFakeStore()
	manifestRepo := newFakeManifestRepo()
	walRepo := newFakeWALRepo() // empty — no covering segment

	writer := NewManifestWriter(store, manifestRepo, walRepo, PITRConfig{ArchivePrefix: "pitr"})

	startLSN := "0/1500000"
	endLSN := "0/2500000"
	rec := &BackupRecord{
		ID: "backup-1", ProjectID: "proj1", DatabaseID: "db1",
		ObjectKey: "base/key.tar.zst", StartLSN: &startLSN, EndLSN: &endLSN, Checksum: "abc",
	}

	err := writer.WriteForBackup(context.Background(), rec)
	if err == nil {
		t.Fatal("expected error when no WAL segment covers start_lsn")
	}
	if store.putCalled != 0 {
		t.Fatalf("putCalled = %d; want 0 on timeline lookup failure", store.putCalled)
	}
}

func TestManifestWriterS3UploadFailure(t *testing.T) {
	store := newFakeStore()
	store.putErr = fmt.Errorf("access denied")
	manifestRepo := newFakeManifestRepo()
	walRepo := newFakeWALRepo()

	walRepo.records[walRecordKey("proj1", "db1", 1, "000000010000000000000001")] = WALSegment{
		ProjectID:  "proj1",
		DatabaseID: "db1",
		Timeline:   1,
		StartLSN:   "0/1000000",
		EndLSN:     "0/2000000",
	}

	writer := NewManifestWriter(store, manifestRepo, walRepo, PITRConfig{ArchivePrefix: "pitr"})

	startLSN := "0/1500000"
	endLSN := "0/2500000"
	rec := &BackupRecord{
		ID: "backup-1", ProjectID: "proj1", DatabaseID: "db1",
		ObjectKey: "base/key.tar.zst", StartLSN: &startLSN, EndLSN: &endLSN, Checksum: "abc",
	}

	err := writer.WriteForBackup(context.Background(), rec)
	if err == nil {
		t.Fatal("expected error on S3 upload failure")
	}
	if !containsFold(err.Error(), "uploading manifest") {
		t.Fatalf("expected upload error, got: %v", err)
	}
	// Manifest should NOT be persisted to DB if upload failed.
	if _, ok := manifestRepo.manifests["backup-1"]; ok {
		t.Fatal("manifest should not be persisted when S3 upload fails")
	}
}

func TestManifestWriterValidatesRequiredFields(t *testing.T) {
	writer := NewManifestWriter(newFakeStore(), newFakeManifestRepo(), newFakeWALRepo(), PITRConfig{})

	rec := &BackupRecord{ID: "backup-1"} // all fields empty
	err := writer.WriteForBackup(context.Background(), rec)
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !containsFold(err.Error(), "ObjectKey") {
		t.Fatalf("expected ObjectKey in error, got: %v", err)
	}
}

func TestBackupManifestJSONDeterministic(t *testing.T) {
	ts := time.Date(2025, 1, 1, 3, 0, 0, 0, time.UTC)
	m := BackupManifest{
		BackupID:   "backup-1",
		ProjectID:  "proj1",
		DatabaseID: "db1",
		ObjectKey:  "base/key.tar.zst",
		StartLSN:   "0/1000000",
		EndLSN:     "0/2000000",
		Checksum:   "abc123",
		Timeline:   1,
		CreatedAt:  ts,
	}

	data1, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("first marshal: %v", err)
	}
	data2, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("second marshal: %v", err)
	}
	if string(data1) != string(data2) {
		t.Fatalf("non-deterministic JSON:\n  %s\n  %s", data1, data2)
	}

	// Verify expected fields present.
	var decoded map[string]interface{}
	if err := json.Unmarshal(data1, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	expectedFields := []string{"backup_id", "project_id", "database_id", "object_key", "start_lsn", "end_lsn", "checksum", "timeline", "created_at"}
	for _, f := range expectedFields {
		if _, ok := decoded[f]; !ok {
			t.Errorf("missing field %q in JSON output", f)
		}
	}
}
