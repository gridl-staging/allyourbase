package backup

import (
	"context"
	"log/slog"
	"testing"
	"time"
)

func TestFireDrillSuccess(t *testing.T) {
	repo := newFakeRepo()
	now := time.Date(2026, 1, 2, 10, 0, 0, 0, time.UTC)
	startLSN := "0/1000000"
	endLSN := "0/2000000"
	endTime := now.Add(-10 * time.Minute)
	repo.records["base-1"] = &BackupRecord{
		ID:          "base-1",
		ProjectID:   "proj1",
		DatabaseID:  "db1",
		BackupType:  "physical",
		Status:      StatusCompleted,
		CompletedAt: &endTime,
		StartLSN:    &startLSN,
		EndLSN:      &endLSN,
	}

	manifestRepo := newFakeManifestRepo()
	manifestRepo.manifests["base-1"] = &BackupManifest{
		BackupID:  "base-1",
		ObjectKey: "base/base-1.tar.zst",
		Checksum:  "abc123",
		StartLSN:  startLSN,
		EndLSN:    endLSN,
	}

	walRepo := newFakeWALRepo()
	walRepo.listRangeResult = []WALSegment{
		{
			ProjectID:   "proj1",
			DatabaseID:  "db1",
			SegmentName: "000000010000000000000002",
			StartLSN:    endLSN,
			EndLSN:      "0/3000000",
			ArchivedAt:  now.Add(-3 * time.Minute),
		},
	}

	planner := NewRestorePlanner(repo, walRepo, manifestRepo)
	notifier := &captureNotifier{}
	runner := NewFireDrillRunner(planner, notifier, slog.Default())
	runner.nowFn = func() time.Time { return now }

	result, err := runner.Run(context.Background(), "proj1", "db1")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !result.Passed {
		t.Fatal("expected drill to pass")
	}
	if result.Plan == nil {
		t.Fatal("expected non-nil plan")
	}
	if len(notifier.alertEvents) != 0 {
		t.Fatalf("expected 0 alerts, got %d", len(notifier.alertEvents))
	}
}

func TestFireDrillFailureNoBackups(t *testing.T) {
	planner := NewRestorePlanner(newFakeRepo(), newFakeWALRepo(), newFakeManifestRepo())
	notifier := &captureNotifier{}
	runner := NewFireDrillRunner(planner, notifier, slog.Default())
	runner.nowFn = func() time.Time { return time.Date(2026, 1, 2, 10, 0, 0, 0, time.UTC) }

	result, err := runner.Run(context.Background(), "proj1", "db1")
	if err == nil {
		t.Fatal("expected failure")
	}
	if result.Passed {
		t.Fatal("expected passed=false")
	}
	if result.Plan != nil {
		t.Fatal("expected nil plan on failure")
	}
	if len(notifier.alertEvents) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(notifier.alertEvents))
	}
	if notifier.alertEvents[0].AlertType != "fire_drill_failed" {
		t.Fatalf("alert type = %s; want fire_drill_failed", notifier.alertEvents[0].AlertType)
	}
}

func TestFireDrillFailureWALGap(t *testing.T) {
	repo := newFakeRepo()
	now := time.Date(2026, 1, 2, 10, 0, 0, 0, time.UTC)
	startLSN := "0/1000000"
	endLSN := "0/2000000"
	endTime := now
	repo.records["base-1"] = &BackupRecord{
		ID:          "base-1",
		ProjectID:   "proj1",
		DatabaseID:  "db1",
		BackupType:  "physical",
		Status:      StatusCompleted,
		CompletedAt: &endTime,
		StartLSN:    &startLSN,
		EndLSN:      &endLSN,
	}

	manifestRepo := newFakeManifestRepo()
	manifestRepo.manifests["base-1"] = &BackupManifest{
		BackupID:  "base-1",
		ObjectKey: "base/base-1.tar.zst",
		Checksum:  "abc123",
		StartLSN:  startLSN,
		EndLSN:    endLSN,
	}

	walRepo := newFakeWALRepo()
	walRepo.listRangeResult = []WALSegment{
		{
			ProjectID:   "proj1",
			DatabaseID:  "db1",
			SegmentName: "000000010000000000000002",
			StartLSN:    "0/2100000",
			EndLSN:      "0/3000000",
			ArchivedAt:  now.Add(5 * time.Minute),
		},
	}

	planner := NewRestorePlanner(repo, walRepo, manifestRepo)
	notifier := &captureNotifier{}
	runner := NewFireDrillRunner(planner, notifier, slog.Default())
	runner.nowFn = func() time.Time { return now }

	result, err := runner.Run(context.Background(), "proj1", "db1")
	if err == nil {
		t.Fatal("expected failure")
	}
	if result.Passed {
		t.Fatal("expected passed=false")
	}
	if result.Error == "" {
		t.Fatal("expected error message")
	}
	if len(notifier.alertEvents) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(notifier.alertEvents))
	}
	if notifier.alertEvents[0].AlertType != "fire_drill_failed" {
		t.Fatalf("alert type = %s; want fire_drill_failed", notifier.alertEvents[0].AlertType)
	}
}
