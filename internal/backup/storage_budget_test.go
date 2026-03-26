package backup

import (
	"context"
	"log/slog"
	"testing"
)

func TestStorageBudgetCheckerUnderBudget(t *testing.T) {
	repo := newFakeRepo()
	repo.records["base-1"] = &BackupRecord{
		ID:         "base-1",
		ProjectID:  "proj1",
		DatabaseID: "db1",
		BackupType: "physical",
		Status:     StatusCompleted,
		SizeBytes:  200,
	}
	repo.records["base-2"] = &BackupRecord{
		ID:         "base-2",
		ProjectID:  "proj1",
		DatabaseID: "db1",
		BackupType: "physical",
		Status:     StatusCompleted,
		SizeBytes:  200,
	}
	walRepo := newFakeWALRepo()
	walRepo.records["wal-1"] = WALSegment{ProjectID: "proj1", DatabaseID: "db1", SizeBytes: 200}

	checker := NewStorageBudgetChecker(repo, walRepo, &captureNotifier{}, slog.Default())
	if err := checker.Check(context.Background(), "proj1", "db1", 1000); err != nil {
		t.Fatalf("Check: %v", err)
	}
}

func TestStorageBudgetCheckerWarning(t *testing.T) {
	repo := newFakeRepo()
	repo.records["base-1"] = &BackupRecord{
		ID:         "base-1",
		ProjectID:  "proj1",
		DatabaseID: "db1",
		BackupType: "physical",
		Status:     StatusCompleted,
		SizeBytes:  250,
	}
	walRepo := newFakeWALRepo()
	walRepo.records["wal-1"] = WALSegment{ProjectID: "proj1", DatabaseID: "db1", SizeBytes: 450}
	notifier := &captureNotifier{}

	checker := NewStorageBudgetChecker(repo, walRepo, notifier, slog.Default())
	if err := checker.Check(context.Background(), "proj1", "db1", 1000); err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(notifier.alertEvents) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(notifier.alertEvents))
	}
	if notifier.alertEvents[0].AlertType != "storage_budget_warning" {
		t.Fatalf("alert type = %s; want storage_budget_warning", notifier.alertEvents[0].AlertType)
	}
}

func TestStorageBudgetCheckerCritical(t *testing.T) {
	repo := newFakeRepo()
	repo.records["base-1"] = &BackupRecord{
		ID:         "base-1",
		ProjectID:  "proj1",
		DatabaseID: "db1",
		BackupType: "physical",
		Status:     StatusCompleted,
		SizeBytes:  450,
	}
	walRepo := newFakeWALRepo()
	walRepo.records["wal-1"] = WALSegment{ProjectID: "proj1", DatabaseID: "db1", SizeBytes: 450}
	notifier := &captureNotifier{}

	checker := NewStorageBudgetChecker(repo, walRepo, notifier, slog.Default())
	if err := checker.Check(context.Background(), "proj1", "db1", 1000); err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(notifier.alertEvents) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(notifier.alertEvents))
	}
	if notifier.alertEvents[0].AlertType != "storage_budget_critical" {
		t.Fatalf("alert type = %s; want storage_budget_critical", notifier.alertEvents[0].AlertType)
	}
}

func TestStorageBudgetCheckerExceeded(t *testing.T) {
	repo := newFakeRepo()
	repo.records["base-1"] = &BackupRecord{
		ID:         "base-1",
		ProjectID:  "proj1",
		DatabaseID: "db1",
		BackupType: "physical",
		Status:     StatusCompleted,
		SizeBytes:  1000,
	}
	walRepo := newFakeWALRepo()
	notifier := &captureNotifier{}

	checker := NewStorageBudgetChecker(repo, walRepo, notifier, slog.Default())
	if err := checker.Check(context.Background(), "proj1", "db1", 1000); err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(notifier.alertEvents) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(notifier.alertEvents))
	}
	if notifier.alertEvents[0].AlertType != "storage_budget_exceeded" {
		t.Fatalf("alert type = %s; want storage_budget_exceeded", notifier.alertEvents[0].AlertType)
	}
}

func TestStorageBudgetCheckerUnlimited(t *testing.T) {
	repo := newFakeRepo()
	repo.records["base-1"] = &BackupRecord{
		ID:         "base-1",
		ProjectID:  "proj1",
		DatabaseID: "db1",
		Status:     StatusCompleted,
		SizeBytes:  10000,
	}
	walRepo := newFakeWALRepo()
	walRepo.records["wal-1"] = WALSegment{ProjectID: "proj1", DatabaseID: "db1", SizeBytes: 10000}
	notifier := &captureNotifier{}

	checker := NewStorageBudgetChecker(repo, walRepo, notifier, slog.Default())
	if err := checker.Check(context.Background(), "proj1", "db1", 0); err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(notifier.alertEvents) != 0 {
		t.Fatalf("expected 0 alerts for unlimited budget, got %d", len(notifier.alertEvents))
	}
}
