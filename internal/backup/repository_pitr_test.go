package backup

import (
	"context"
	"testing"
	"time"
)

// Tests for PITR-related BackupRecord fields using fakeRepo.

func TestFakeRepoCreateSetsLogicalType(t *testing.T) {
	repo := newFakeRepo()
	rec, err := repo.Create(context.Background(), "mydb", "test")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if rec.BackupType != "logical" {
		t.Errorf("BackupType = %q; want %q", rec.BackupType, "logical")
	}
	if rec.StartLSN != nil {
		t.Errorf("StartLSN should be nil for logical backups, got %v", rec.StartLSN)
	}
	if rec.EndLSN != nil {
		t.Errorf("EndLSN should be nil for logical backups, got %v", rec.EndLSN)
	}
}

func TestFakeRepoCreatePhysicalSetsPhysicalType(t *testing.T) {
	repo := newFakeRepo()
	rec, err := repo.CreatePhysical(context.Background(), "proj1", "db1", "test")
	if err != nil {
		t.Fatalf("CreatePhysical: %v", err)
	}
	if rec.BackupType != "physical" {
		t.Errorf("BackupType = %q; want %q", rec.BackupType, "physical")
	}
	if rec.ProjectID != "proj1" {
		t.Errorf("ProjectID = %q; want %q", rec.ProjectID, "proj1")
	}
	if rec.DatabaseID != "db1" {
		t.Errorf("DatabaseID = %q; want %q", rec.DatabaseID, "db1")
	}
}

func TestFakeRepoMarkPhysicalCompletedSetsLSN(t *testing.T) {
	repo := newFakeRepo()
	rec, err := repo.CreatePhysical(context.Background(), "proj1", "db1", "test")
	if err != nil {
		t.Fatalf("CreatePhysical: %v", err)
	}

	startLSN := "0/1000000"
	endLSN := "0/2000000"
	completedAt := time.Now()

	if err := repo.MarkPhysicalCompleted(context.Background(),
		rec.ID, "projects/proj1/db/db1/base/2026/03/01/foo.tar.zst",
		16*1024*1024, "sha256:abc", startLSN, endLSN, completedAt,
	); err != nil {
		t.Fatalf("MarkPhysicalCompleted: %v", err)
	}

	got, err := repo.Get(context.Background(), rec.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Status != StatusCompleted {
		t.Errorf("Status = %q; want %q", got.Status, StatusCompleted)
	}
	if got.StartLSN == nil || *got.StartLSN != startLSN {
		t.Errorf("StartLSN = %v; want %q", got.StartLSN, startLSN)
	}
	if got.EndLSN == nil || *got.EndLSN != endLSN {
		t.Errorf("EndLSN = %v; want %q", got.EndLSN, endLSN)
	}
}

func TestFakeRepoMarkCompletedIsBackwardCompatible(t *testing.T) {
	repo := newFakeRepo()
	rec, err := repo.Create(context.Background(), "mydb", "test")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	completedAt := time.Now()
	if err := repo.MarkCompleted(context.Background(),
		rec.ID, "backups/mydb/2026/01/01/foo.sql.gz",
		1024, "sha256:xyz", completedAt,
	); err != nil {
		t.Fatalf("MarkCompleted: %v", err)
	}

	got, err := repo.Get(context.Background(), rec.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Status != StatusCompleted {
		t.Errorf("Status = %q; want %q", got.Status, StatusCompleted)
	}
	// Logical backups must not have LSN set.
	if got.StartLSN != nil {
		t.Errorf("StartLSN should remain nil for logical backups, got %v", got.StartLSN)
	}
}
