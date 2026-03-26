package backup

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

type fakePlannerRepo struct {
	backups []BackupRecord
	err     error
}

func (r *fakePlannerRepo) Create(_ context.Context, _ string, _ string) (*BackupRecord, error) {
	return nil, fmt.Errorf("not implemented")
}

func (r *fakePlannerRepo) CreatePhysical(_ context.Context, _, _, _ string) (*BackupRecord, error) {
	return nil, fmt.Errorf("not implemented")
}

func (r *fakePlannerRepo) UpdateStatus(_ context.Context, _, _, _ string) error {
	return fmt.Errorf("not implemented")
}

func (r *fakePlannerRepo) MarkCompleted(_ context.Context, _, _ string, _ int64, _ string, _ time.Time) error {
	return fmt.Errorf("not implemented")
}

func (r *fakePlannerRepo) MarkPhysicalCompleted(_ context.Context, _, _ string, _ int64, _, _, _ string, _ time.Time) error {
	return fmt.Errorf("not implemented")
}

func (r *fakePlannerRepo) Get(_ context.Context, _ string) (*BackupRecord, error) {
	return nil, fmt.Errorf("not implemented")
}

func (r *fakePlannerRepo) List(_ context.Context, _ ListFilter) ([]BackupRecord, int, error) {
	return nil, 0, fmt.Errorf("not implemented")
}

func (r *fakePlannerRepo) CompletedByDB(_ context.Context, _ string) ([]BackupRecord, error) {
	return nil, fmt.Errorf("not implemented")
}

func (r *fakePlannerRepo) RecordRestore(_ context.Context, _, _, _ string) (string, error) {
	return "", fmt.Errorf("not implemented")
}

func (r *fakePlannerRepo) ListPhysicalCompleted(_ context.Context, _, _ string) ([]BackupRecord, error) {
	if r.err != nil {
		return nil, r.err
	}
	out := make([]BackupRecord, 0, len(r.backups))
	out = append(out, r.backups...)
	return out, nil
}

func ptrLSN(v string) *string { return &v }

func completedPhysical(id string, completed time.Time, endLSN string) BackupRecord {
	completedCopy := completed
	return BackupRecord{
		ID:          id,
		BackupType:  "physical",
		Status:      StatusCompleted,
		CompletedAt: &completedCopy,
		EndLSN:      ptrLSN(endLSN),
	}
}

func walSeg(name, startLSN, endLSN string, archivedAt time.Time, size int64) WALSegment {
	return WALSegment{
		SegmentName: name,
		StartLSN:    startLSN,
		EndLSN:      endLSN,
		ArchivedAt:  archivedAt,
		SizeBytes:   size,
	}
}

func TestRestorePlannerValidateWindowSingleBackup(t *testing.T) {
	t.Parallel()

	t0 := time.Date(2026, 1, 2, 10, 0, 0, 0, time.UTC)
	target := t0.Add(20 * time.Minute)
	latestAt := t0.Add(30 * time.Minute)

	repo := &fakePlannerRepo{backups: []BackupRecord{completedPhysical("b1", t0, "0/2000000")}}
	walRepo := newFakeWALRepo()
	walRepo.listRangeResult = []WALSegment{
		walSeg("000000010000000000000002", "0/2000000", "0/3000000", t0.Add(10*time.Minute), 16),
		walSeg("000000010000000000000003", "0/3000000", "0/4000000", latestAt, 20),
	}
	manifestRepo := newFakeManifestRepo()
	manifestRepo.manifests["b1"] = &BackupManifest{BackupID: "b1"}

	planner := NewRestorePlanner(repo, walRepo, manifestRepo)
	plan, err := planner.ValidateWindow(context.Background(), "proj1", "db1", target)
	if err != nil {
		t.Fatalf("ValidateWindow: %v", err)
	}
	if plan.BaseBackup == nil || plan.BaseBackup.ID != "b1" {
		t.Fatalf("BaseBackup = %+v; want b1", plan.BaseBackup)
	}
	if got, want := plan.EarliestRecoverable, t0; !got.Equal(want) {
		t.Fatalf("EarliestRecoverable = %v; want %v", got, want)
	}
	if got, want := plan.LatestRecoverable, latestAt; !got.Equal(want) {
		t.Fatalf("LatestRecoverable = %v; want %v", got, want)
	}
	if got, want := plan.EstimatedWALBytes, int64(36); got != want {
		t.Fatalf("EstimatedWALBytes = %d; want %d", got, want)
	}
}

func TestRestorePlannerValidateWindowSelectsNearestBackup(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, 1, 2, 10, 0, 0, 0, time.UTC)
	repo := &fakePlannerRepo{backups: []BackupRecord{
		completedPhysical("newest", base.Add(2*time.Hour), "0/4000000"),
		completedPhysical("chosen", base.Add(1*time.Hour), "0/3000000"),
		completedPhysical("oldest", base, "0/2000000"),
	}}
	walRepo := newFakeWALRepo()
	walRepo.listRangeResult = []WALSegment{
		walSeg("000000010000000000000003", "0/3000000", "0/4000000", base.Add(130*time.Minute), 16),
		walSeg("000000010000000000000004", "0/4000000", "0/5000000", base.Add(140*time.Minute), 16),
	}
	manifestRepo := newFakeManifestRepo()
	manifestRepo.manifests["chosen"] = &BackupManifest{BackupID: "chosen"}

	planner := NewRestorePlanner(repo, walRepo, manifestRepo)
	plan, err := planner.ValidateWindow(context.Background(), "proj1", "db1", base.Add(90*time.Minute))
	if err != nil {
		t.Fatalf("ValidateWindow: %v", err)
	}
	if plan.BaseBackup == nil || plan.BaseBackup.ID != "chosen" {
		t.Fatalf("BaseBackup.ID = %v; want chosen", plan.BaseBackup)
	}
}

func TestRestorePlannerValidateWindowTargetBeforeEarliestBackup(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, 1, 2, 10, 0, 0, 0, time.UTC)
	repo := &fakePlannerRepo{backups: []BackupRecord{completedPhysical("b1", base, "0/2000000")}}
	walRepo := newFakeWALRepo()
	walRepo.listRangeResult = []WALSegment{walSeg("000000010000000000000002", "0/2000000", "0/3000000", base.Add(10*time.Minute), 16)}
	manifestRepo := newFakeManifestRepo()
	manifestRepo.manifests["b1"] = &BackupManifest{BackupID: "b1"}

	planner := NewRestorePlanner(repo, walRepo, manifestRepo)
	_, err := planner.ValidateWindow(context.Background(), "proj1", "db1", base.Add(-time.Minute))
	if err == nil || !strings.Contains(err.Error(), "before earliest recoverable") {
		t.Fatalf("expected earliest recoverable error, got %v", err)
	}
}

func TestRestorePlannerValidateWindowTargetAfterLatestWAL(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, 1, 2, 10, 0, 0, 0, time.UTC)
	repo := &fakePlannerRepo{backups: []BackupRecord{completedPhysical("b1", base, "0/2000000")}}
	walRepo := newFakeWALRepo()
	walRepo.listRangeResult = []WALSegment{walSeg("000000010000000000000002", "0/2000000", "0/3000000", base.Add(5*time.Minute), 16)}
	manifestRepo := newFakeManifestRepo()
	manifestRepo.manifests["b1"] = &BackupManifest{BackupID: "b1"}

	planner := NewRestorePlanner(repo, walRepo, manifestRepo)
	_, err := planner.ValidateWindow(context.Background(), "proj1", "db1", base.Add(10*time.Minute))
	if err == nil || !strings.Contains(err.Error(), "after latest recoverable") {
		t.Fatalf("expected latest recoverable error, got %v", err)
	}
}

func TestRestorePlannerValidateWindowDetectsWALGap(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, 1, 2, 10, 0, 0, 0, time.UTC)
	target := base.Add(5 * time.Minute)
	repo := &fakePlannerRepo{backups: []BackupRecord{completedPhysical("b1", base, "0/2000000")}}
	walRepo := newFakeWALRepo()
	walRepo.listRangeResult = []WALSegment{walSeg("000000010000000000000002", "0/2100000", "0/3000000", base.Add(6*time.Minute), 16)}
	manifestRepo := newFakeManifestRepo()
	manifestRepo.manifests["b1"] = &BackupManifest{BackupID: "b1"}

	planner := NewRestorePlanner(repo, walRepo, manifestRepo)
	_, err := planner.ValidateWindow(context.Background(), "proj1", "db1", target)
	if err == nil || !strings.Contains(err.Error(), "WAL gap") {
		t.Fatalf("expected WAL gap error, got %v", err)
	}
}
