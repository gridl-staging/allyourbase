package backup

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"
)

func TestWALLagCheckerNoAlertWithinThreshold(t *testing.T) {
	fakeWAL := newFakeWALRepo()
	now := time.Now().UTC().Add(10 * time.Minute)
	fakeWAL.records["seg-1"] = WALSegment{
		ProjectID:   "proj1",
		DatabaseID:  "db1",
		SegmentName: "000000010000000000000001",
		ArchivedAt:  now.Add(-2 * time.Minute),
	}

	notifier := &captureNotifier{}
	checker := NewWALLagChecker(fakeWAL, notifier, 5, slog.Default())
	checker.nowFn = func() time.Time { return now }

	if err := checker.Check(context.Background(), "proj1", "db1"); err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(notifier.alertEvents) != 0 {
		t.Fatalf("expected 0 alerts, got %d", len(notifier.alertEvents))
	}
}

func TestWALLagCheckerAlertWhenLagExceeded(t *testing.T) {
	fakeWAL := newFakeWALRepo()
	now := time.Now().UTC().Add(10 * time.Minute)
	fakeWAL.records["seg-1"] = WALSegment{
		ProjectID:   "proj1",
		DatabaseID:  "db1",
		SegmentName: "000000010000000000000001",
		ArchivedAt:  now.Add(-20 * time.Minute),
	}

	notifier := &captureNotifier{}
	checker := NewWALLagChecker(fakeWAL, notifier, 10, slog.Default())
	checker.nowFn = func() time.Time { return now }

	if err := checker.Check(context.Background(), "proj1", "db1"); err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(notifier.alertEvents) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(notifier.alertEvents))
	}
	if notifier.alertEvents[0].AlertType != "wal_archive_lag" {
		t.Fatalf("alert type = %s; want wal_archive_lag", notifier.alertEvents[0].AlertType)
	}
}

func TestWALLagCheckerAlertWhenNoSegments(t *testing.T) {
	notifier := &captureNotifier{}
	checker := NewWALLagChecker(newFakeWALRepo(), notifier, 5, slog.Default())

	if err := checker.Check(context.Background(), "proj1", "db1"); err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(notifier.alertEvents) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(notifier.alertEvents))
	}
	if notifier.alertEvents[0].AlertType != "wal_archive_lag" {
		t.Fatalf("alert type = %s; want wal_archive_lag", notifier.alertEvents[0].AlertType)
	}
}

func TestWALLagCheckerReturnsRepoErrors(t *testing.T) {
	fakeWAL := newFakeWALRepo()
	fakeWAL.listRangeErr = errors.New("db unavailable")
	notifier := &captureNotifier{}
	checker := NewWALLagChecker(fakeWAL, notifier, 5, slog.Default())

	err := checker.Check(context.Background(), "proj1", "db1")
	if err == nil {
		t.Fatal("expected error")
	}
	if len(notifier.alertEvents) != 0 {
		t.Fatalf("expected 0 alerts, got %d", len(notifier.alertEvents))
	}
}
