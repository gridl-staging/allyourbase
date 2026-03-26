package backup

import (
	"context"
	"fmt"
	"log/slog"
	"testing"
	"time"
)

type fakeStoreWithDeleteErr struct {
	*fakeStore
	deleteErr error
}

func (s *fakeStoreWithDeleteErr) DeleteObject(_ context.Context, key string) error {
	if s.deleteErr != nil {
		return s.deleteErr
	}
	return s.fakeStore.DeleteObject(context.Background(), key)
}

func TestPITRRetentionJobDeletesExpiredWALAndBase(t *testing.T) {
	now := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)

	cfg := PITRConfig{
		Enabled:                  true,
		EnvironmentClass:         "non-prod",
		WALRetentionDays:         14,
		BaseBackupRetentionDays:  5,
		ComplianceSnapshotMonths: 0,
		ArchivePrefix:            "pitr",
		RPOMinutes:               5,
		RetentionSchedule:        "0 4 * * *",
		BaseBackupSchedule:       "0 */6 * * *",
		VerifySchedule:           "0 */6 * * *",
	}

	store := newFakeStore()
	store.objects[WALSegmentKey("pitr", "proj1", "db1", 1, "000000010000000000000001")] = []byte("segment1")
	store.objects["base-backup-1.tar.zst"] = []byte("backup1")
	store.objects["base-backup-2.tar.zst"] = []byte("backup2")

	repo := newFakeRepo()
	repo.records["backup-1"] = &BackupRecord{
		ID:          "backup-1",
		ProjectID:   "proj1",
		DatabaseID:  "db1",
		BackupType:  "physical",
		Status:      StatusCompleted,
		ObjectKey:   "base-backup-1.tar.zst",
		StartLSN:    ptr("0/1000000"),
		EndLSN:      ptr("0/2000000"),
		CompletedAt: ptrTime(now.Add(-10 * 24 * time.Hour)),
	}
	repo.records["backup-2"] = &BackupRecord{
		ID:          "backup-2",
		ProjectID:   "proj1",
		DatabaseID:  "db1",
		BackupType:  "physical",
		Status:      StatusCompleted,
		ObjectKey:   "base-backup-2.tar.zst",
		StartLSN:    ptr("0/4000000"),
		EndLSN:      ptr("0/5000000"),
		CompletedAt: ptrTime(now.Add(-1 * 24 * time.Hour)),
	}

	walRepo := newFakeWALRepo()
	walRepo.records["seg-1"] = WALSegment{
		ID:          "seg-1",
		ProjectID:   "proj1",
		DatabaseID:  "db1",
		Timeline:    1,
		SegmentName: "000000010000000000000001",
		StartLSN:    "0/1000000",
		EndLSN:      "0/3000000",
		ArchivedAt:  now.Add(-20 * 24 * time.Hour),
	}

	job := NewPITRRetentionJob(cfg, store, repo, walRepo, &fakeManifestRepo{}, nil, slog.Default(), "pitr")
	job.nowFn = func() time.Time { return now }
	result, err := job.Run(context.Background(), "proj1", "db1", false)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(result.DeletedWAL) != 1 || result.DeletedWAL[0] != "seg-1" {
		t.Fatalf("DeletedWAL = %#v; want [seg-1]", result.DeletedWAL)
	}
	if len(result.DeletedBackups) != 1 || result.DeletedBackups[0] != "backup-1" {
		t.Fatalf("DeletedBackups = %#v; want [backup-1]", result.DeletedBackups)
	}

	if _, ok := store.objects[WALSegmentKey("pitr", "proj1", "db1", 1, "000000010000000000000001")]; ok {
		t.Fatal("expected WAL segment object deleted from store")
	}
	if _, ok := store.objects["base-backup-1.tar.zst"]; ok {
		t.Fatal("expected base backup object deleted from store")
	}
	if _, ok := store.objects["base-backup-2.tar.zst"]; !ok {
		t.Fatal("expected retained base backup object to remain")
	}
	if repo.records["backup-1"].Status != StatusDeleted {
		t.Fatalf("backup-1 status = %q; want deleted", repo.records["backup-1"].Status)
	}
}

func TestPITRRetentionJobDryRun(t *testing.T) {
	now := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)
	cfg := PITRConfig{
		Enabled:                 true,
		WALRetentionDays:        14,
		BaseBackupRetentionDays: 5,
		ArchivePrefix:           "pitr",
		RPOMinutes:              5,
		RetentionSchedule:       "0 4 * * *",
		BaseBackupSchedule:      "0 */6 * * *",
		VerifySchedule:          "0 */6 * * *",
	}

	store := newFakeStore()
	store.objects["pitr/projects/proj1/db/db1/base/base-backup-1.tar.zst"] = []byte("b1")
	store.objects[WALSegmentKey("pitr", "proj1", "db1", 1, "000000010000000000000001")] = []byte("segment1")

	repo := newFakeRepo()
	repo.records["backup-1"] = &BackupRecord{
		ID:          "backup-1",
		ProjectID:   "proj1",
		DatabaseID:  "db1",
		BackupType:  "physical",
		Status:      StatusCompleted,
		ObjectKey:   "pitr/projects/proj1/db/db1/base/base-backup-1.tar.zst",
		StartLSN:    ptr("0/1000000"),
		CompletedAt: ptrTime(now.Add(-10 * 24 * time.Hour)),
	}
	repo.records["backup-2"] = &BackupRecord{
		ID:          "backup-2",
		ProjectID:   "proj1",
		DatabaseID:  "db1",
		BackupType:  "physical",
		Status:      StatusCompleted,
		ObjectKey:   "pitr/projects/proj1/db/db1/base/base-backup-2.tar.zst",
		StartLSN:    ptr("0/3000001"),
		EndLSN:      ptr("0/4000000"),
		CompletedAt: ptrTime(now.Add(-1 * 24 * time.Hour)),
	}

	walRepo := newFakeWALRepo()
	walRepo.records["seg-1"] = WALSegment{
		ID:          "seg-1",
		ProjectID:   "proj1",
		DatabaseID:  "db1",
		Timeline:    1,
		SegmentName: "000000010000000000000001",
		EndLSN:      "0/3000000",
		ArchivedAt:  now.Add(-20 * 24 * time.Hour),
	}

	job := NewPITRRetentionJob(cfg, store, repo, walRepo, &fakeManifestRepo{}, nil, slog.Default(), "pitr")
	job.nowFn = func() time.Time { return now }
	result, err := job.Run(context.Background(), "proj1", "db1", true)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !result.DryRun {
		t.Fatal("expected dry run")
	}
	if len(result.DeletedWAL) != 1 || len(result.DeletedBackups) != 1 {
		t.Fatalf("expected one WAL and one backup candidate in dry run, got %#v", result)
	}
	if len(result.Errors) != 0 {
		t.Fatalf("expected no errors, got %d", len(result.Errors))
	}
	if len(store.objects) != 2 {
		t.Fatalf("expected no object deletions in dry run, got %d", len(store.objects))
	}
	if repo.records["backup-1"].Status != StatusCompleted {
		t.Fatalf("backup status changed during dry run: %q", repo.records["backup-1"].Status)
	}
}

func TestPITRRetentionJobWALSafetyPreventsDeletion(t *testing.T) {
	now := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)
	cfg := PITRConfig{
		Enabled:                 true,
		WALRetentionDays:        14,
		BaseBackupRetentionDays: 5,
		ArchivePrefix:           "pitr",
		RPOMinutes:              5,
		RetentionSchedule:       "0 4 * * *",
		BaseBackupSchedule:      "0 */6 * * *",
		VerifySchedule:          "0 */6 * * *",
	}

	store := newFakeStore()
	store.objects[WALSegmentKey("pitr", "proj1", "db1", 1, "000000010000000000000001")] = []byte("segment1")

	repo := newFakeRepo()
	repo.records["backup-1"] = &BackupRecord{
		ID:          "backup-1",
		ProjectID:   "proj1",
		DatabaseID:  "db1",
		BackupType:  "physical",
		Status:      StatusCompleted,
		ObjectKey:   "base-backup-1.tar.zst",
		StartLSN:    ptr("0/1000000"),
		EndLSN:      ptr("0/1500000"),
		CompletedAt: ptrTime(now.Add(-1 * 24 * time.Hour)),
	}

	walRepo := newFakeWALRepo()
	walRepo.records["seg-1"] = WALSegment{
		ID:          "seg-1",
		ProjectID:   "proj1",
		DatabaseID:  "db1",
		Timeline:    1,
		SegmentName: "000000010000000000000001",
		EndLSN:      "0/3000000",
		ArchivedAt:  now.Add(-20 * 24 * time.Hour),
	}

	job := NewPITRRetentionJob(cfg, store, repo, walRepo, &fakeManifestRepo{}, nil, slog.Default(), "pitr")
	job.nowFn = func() time.Time { return now }
	result, err := job.Run(context.Background(), "proj1", "db1", false)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(result.DeletedWAL) != 0 {
		t.Fatalf("expected no WAL deletions, got %#v", result.DeletedWAL)
	}
	if _, ok := store.objects[WALSegmentKey("pitr", "proj1", "db1", 1, "000000010000000000000001")]; !ok {
		t.Fatal("expected WAL segment to remain")
	}
}

func TestPITRRetentionJobKeepsComplianceSnapshotInProd(t *testing.T) {
	now := time.Date(2026, 1, 31, 12, 0, 0, 0, time.UTC)
	cfg := PITRConfig{
		Enabled:                  true,
		EnvironmentClass:         "prod",
		WALRetentionDays:         14,
		BaseBackupRetentionDays:  1,
		ComplianceSnapshotMonths: 12,
		ArchivePrefix:            "pitr",
		RPOMinutes:               5,
		RetentionSchedule:        "0 4 * * *",
		BaseBackupSchedule:       "0 */6 * * *",
		VerifySchedule:           "0 */6 * * *",
	}

	store := newFakeStore()
	store.objects["base-backup-jan-1.tar.zst"] = []byte("jan1")
	store.objects["base-backup-jan-2.tar.zst"] = []byte("jan2")
	store.objects["base-backup-jan-3.tar.zst"] = []byte("jan3")

	repo := newFakeRepo()
	repo.records["b1"] = &BackupRecord{
		ID:          "b1",
		ProjectID:   "proj1",
		DatabaseID:  "db1",
		BackupType:  "physical",
		Status:      StatusCompleted,
		ObjectKey:   "base-backup-jan-1.tar.zst",
		StartLSN:    ptr("0/1000000"),
		CompletedAt: ptrTime(now.Add(-25 * 24 * time.Hour)),
	}
	repo.records["b2"] = &BackupRecord{
		ID:          "b2",
		ProjectID:   "proj1",
		DatabaseID:  "db1",
		BackupType:  "physical",
		Status:      StatusCompleted,
		ObjectKey:   "base-backup-jan-2.tar.zst",
		StartLSN:    ptr("0/2000000"),
		CompletedAt: ptrTime(now.Add(-20 * 24 * time.Hour)),
	}
	repo.records["b3"] = &BackupRecord{
		ID:          "b3",
		ProjectID:   "proj1",
		DatabaseID:  "db1",
		BackupType:  "physical",
		Status:      StatusCompleted,
		ObjectKey:   "base-backup-jan-3.tar.zst",
		StartLSN:    ptr("0/3000000"),
		CompletedAt: ptrTime(now.Add(-2 * 24 * time.Hour)),
	}

	walRepo := newFakeWALRepo()
	job := NewPITRRetentionJob(cfg, store, repo, walRepo, &fakeManifestRepo{}, nil, slog.Default(), "pitr")
	job.nowFn = func() time.Time { return now }
	_, err := job.Run(context.Background(), "proj1", "db1", false)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if _, ok := store.objects["base-backup-jan-1.tar.zst"]; !ok {
		t.Fatal("expected compliance snapshot b1 retained")
	}
	if _, ok := store.objects["base-backup-jan-2.tar.zst"]; ok {
		t.Fatal("expected non-compliance backup b2 deleted")
	}
	if _, ok := store.objects["base-backup-jan-3.tar.zst"]; !ok {
		t.Fatal("expected newest backup b3 retained")
	}
}

func TestPITRRetentionJobNonProdDoesNotRetainCompliance(t *testing.T) {
	now := time.Date(2026, 1, 31, 12, 0, 0, 0, time.UTC)
	cfg := PITRConfig{
		Enabled:                  true,
		EnvironmentClass:         "staging",
		WALRetentionDays:         14,
		BaseBackupRetentionDays:  1,
		ComplianceSnapshotMonths: 12,
		ArchivePrefix:            "pitr",
		RPOMinutes:               5,
		RetentionSchedule:        "0 4 * * *",
		BaseBackupSchedule:       "0 */6 * * *",
		VerifySchedule:           "0 */6 * * *",
	}

	store := newFakeStore()
	store.objects["base-backup-jan-1.tar.zst"] = []byte("jan1")
	store.objects["base-backup-jan-2.tar.zst"] = []byte("jan2")
	store.objects["base-backup-jan-3.tar.zst"] = []byte("jan3")

	repo := newFakeRepo()
	repo.records["b1"] = &BackupRecord{
		ID:          "b1",
		ProjectID:   "proj1",
		DatabaseID:  "db1",
		BackupType:  "physical",
		Status:      StatusCompleted,
		ObjectKey:   "base-backup-jan-1.tar.zst",
		StartLSN:    ptr("0/1000000"),
		CompletedAt: ptrTime(now.Add(-25 * 24 * time.Hour)),
	}
	repo.records["b2"] = &BackupRecord{
		ID:          "b2",
		ProjectID:   "proj1",
		DatabaseID:  "db1",
		BackupType:  "physical",
		Status:      StatusCompleted,
		ObjectKey:   "base-backup-jan-2.tar.zst",
		StartLSN:    ptr("0/2000000"),
		CompletedAt: ptrTime(now.Add(-20 * 24 * time.Hour)),
	}
	repo.records["b3"] = &BackupRecord{
		ID:          "b3",
		ProjectID:   "proj1",
		DatabaseID:  "db1",
		BackupType:  "physical",
		Status:      StatusCompleted,
		ObjectKey:   "base-backup-jan-3.tar.zst",
		StartLSN:    ptr("0/3000000"),
		CompletedAt: ptrTime(now.Add(-2 * 24 * time.Hour)),
	}

	walRepo := newFakeWALRepo()
	job := NewPITRRetentionJob(cfg, store, repo, walRepo, &fakeManifestRepo{}, nil, slog.Default(), "pitr")
	job.nowFn = func() time.Time { return now }
	_, err := job.Run(context.Background(), "proj1", "db1", false)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if _, ok := store.objects["base-backup-jan-1.tar.zst"]; ok {
		t.Fatal("expected non-prod retention to delete b1")
	}
	if _, ok := store.objects["base-backup-jan-2.tar.zst"]; ok {
		t.Fatal("expected non-prod retention to delete b2")
	}
	if _, ok := store.objects["base-backup-jan-3.tar.zst"]; !ok {
		t.Fatal("expected newest backup b3 retained")
	}
}

func TestPITRRetentionJobNoCandidates(t *testing.T) {
	job := NewPITRRetentionJob(
		PITRConfig{Enabled: true, WALRetentionDays: 14, BaseBackupRetentionDays: 5},
		newFakeStore(),
		newFakeRepo(),
		newFakeWALRepo(),
		&fakeManifestRepo{},
		nil,
		slog.Default(),
		"pitr",
	)

	result, err := job.Run(context.Background(), "proj1", "db1", false)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(result.DeletedWAL) != 0 {
		t.Fatalf("expected no WAL deletions, got %#v", result.DeletedWAL)
	}
	if len(result.DeletedBackups) != 0 {
		t.Fatalf("expected no base backup deletions, got %#v", result.DeletedBackups)
	}
	if len(result.Errors) != 0 {
		t.Fatalf("expected no errors, got %d", len(result.Errors))
	}
}

func TestPITRRetentionJobDeleteFailureNotifies(t *testing.T) {
	now := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)
	cfg := PITRConfig{
		Enabled:                 true,
		WALRetentionDays:        14,
		BaseBackupRetentionDays: 100,
		ArchivePrefix:           "pitr",
		RPOMinutes:              5,
		RetentionSchedule:       "0 4 * * *",
		BaseBackupSchedule:      "0 */6 * * *",
		VerifySchedule:          "0 */6 * * *",
	}

	store := &fakeStoreWithDeleteErr{
		fakeStore: newFakeStore(),
		deleteErr: fmt.Errorf("delete denied"),
	}
	store.objects[WALSegmentKey("pitr", "proj1", "db1", 1, "000000010000000000000001")] = []byte("segment1")
	store.objects["base-backup-1.tar.zst"] = []byte("backup")

	repo := newFakeRepo()
	repo.records["backup-1"] = &BackupRecord{
		ID:          "backup-1",
		ProjectID:   "proj1",
		DatabaseID:  "db1",
		BackupType:  "physical",
		Status:      StatusCompleted,
		ObjectKey:   "base-backup-1.tar.zst",
		StartLSN:    ptr("0/5000000"),
		CompletedAt: ptrTime(now.Add(-1 * 24 * time.Hour)),
	}

	walRepo := newFakeWALRepo()
	walRepo.records["seg-1"] = WALSegment{
		ID:          "seg-1",
		ProjectID:   "proj1",
		DatabaseID:  "db1",
		Timeline:    1,
		SegmentName: "000000010000000000000001",
		EndLSN:      "0/3000000",
		ArchivedAt:  now.Add(-20 * 24 * time.Hour),
	}

	notifier := &captureNotifier{}
	job := NewPITRRetentionJob(cfg, store, repo, walRepo, &fakeManifestRepo{}, notifier, slog.Default(), "pitr")
	job.nowFn = func() time.Time { return now }
	result, err := job.Run(context.Background(), "proj1", "db1", false)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(result.Errors) != 1 {
		t.Fatalf("expected 1 error, got %d", len(result.Errors))
	}
	if len(notifier.events) != 1 {
		t.Fatalf("expected 1 failure notification, got %d", len(notifier.events))
	}
	if notifier.events[0].Stage != "pitr_retention" {
		t.Fatalf("expected stage pitr_retention, got %q", notifier.events[0].Stage)
	}
}

func ptr(v string) *string {
	return &v
}
