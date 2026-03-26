package backup

import (
	"context"
	"log/slog"
	"testing"
	"time"
)

type fakeScheduleRepo struct {
	backups []BackupRecord
}

func (f *fakeScheduleRepo) Create(_ context.Context, _, _ string) (*BackupRecord, error) {
	return nil, nil
}
func (f *fakeScheduleRepo) CreatePhysical(_ context.Context, _, _, _ string) (*BackupRecord, error) {
	return nil, nil
}
func (f *fakeScheduleRepo) UpdateStatus(_ context.Context, _, _, _ string) error { return nil }
func (f *fakeScheduleRepo) MarkCompleted(_ context.Context, _, _ string, _ int64, _ string, _ time.Time) error {
	return nil
}
func (f *fakeScheduleRepo) MarkPhysicalCompleted(_ context.Context, _, _ string, _ int64, _, _, _ string, _ time.Time) error {
	return nil
}
func (f *fakeScheduleRepo) Get(_ context.Context, _ string) (*BackupRecord, error) { return nil, nil }
func (f *fakeScheduleRepo) List(_ context.Context, _ ListFilter) ([]BackupRecord, int, error) {
	return nil, 0, nil
}
func (f *fakeScheduleRepo) CompletedByDB(_ context.Context, _ string) ([]BackupRecord, error) {
	return nil, nil
}
func (f *fakeScheduleRepo) RecordRestore(_ context.Context, _, _, _ string) (string, error) {
	return "", nil
}
func (f *fakeScheduleRepo) ListPhysicalCompleted(_ context.Context, _, _ string) ([]BackupRecord, error) {
	out := make([]BackupRecord, len(f.backups))
	copy(out, f.backups)
	return out, nil
}

func TestScheduleCheckerNoMiss(t *testing.T) {
	now := time.Date(2026, 1, 2, 10, 0, 0, 0, time.UTC)
	repo := &fakeScheduleRepo{
		backups: []BackupRecord{
			{
				ID:          "backup-1",
				ProjectID:   "proj1",
				DatabaseID:  "db1",
				Status:      StatusCompleted,
				CompletedAt: ptrTime(now.Add(-1 * time.Hour)),
			},
		},
	}
	notifier := &captureNotifier{}
	checker := NewScheduleChecker(repo, notifier, "0 */6 * * *", slog.Default())
	checker.nowFn = func() time.Time { return now }

	if err := checker.Check(context.Background(), "proj1", "db1"); err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(notifier.alertEvents) != 0 {
		t.Fatalf("expected 0 alerts, got %d", len(notifier.alertEvents))
	}
}

func TestScheduleCheckerMissed(t *testing.T) {
	now := time.Date(2026, 1, 2, 10, 0, 0, 0, time.UTC)
	repo := &fakeScheduleRepo{
		backups: []BackupRecord{
			{
				ID:          "backup-1",
				ProjectID:   "proj1",
				DatabaseID:  "db1",
				Status:      StatusCompleted,
				CompletedAt: ptrTime(now.Add(-13 * time.Hour)),
			},
		},
	}
	notifier := &captureNotifier{}
	checker := NewScheduleChecker(repo, notifier, "0 */6 * * *", slog.Default())
	checker.nowFn = func() time.Time { return now }

	if err := checker.Check(context.Background(), "proj1", "db1"); err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(notifier.alertEvents) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(notifier.alertEvents))
	}
	if notifier.alertEvents[0].AlertType != "base_backup_missed" {
		t.Fatalf("alert type = %s; want base_backup_missed", notifier.alertEvents[0].AlertType)
	}
}

func TestScheduleCheckerNoBackups(t *testing.T) {
	now := time.Date(2026, 1, 2, 10, 0, 0, 0, time.UTC)
	notifier := &captureNotifier{}
	checker := NewScheduleChecker(&fakeScheduleRepo{}, notifier, "0 */6 * * *", slog.Default())
	checker.nowFn = func() time.Time { return now }

	if err := checker.Check(context.Background(), "proj1", "db1"); err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(notifier.alertEvents) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(notifier.alertEvents))
	}
	if notifier.alertEvents[0].AlertType != "base_backup_missed" {
		t.Fatalf("alert type = %s; want base_backup_missed", notifier.alertEvents[0].AlertType)
	}
}

func ptrTime(t time.Time) *time.Time {
	return &t
}
