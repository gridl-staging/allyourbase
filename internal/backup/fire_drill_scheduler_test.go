package backup

import (
	"log/slog"
	"testing"
)

func TestNewFireDrillSchedulerValidCron(t *testing.T) {
	scheduler, err := NewFireDrillScheduler(
		NewFireDrillRunner(NewRestorePlanner(newFakeRepo(), newFakeWALRepo(), newFakeManifestRepo()), nil, slog.Default()),
		PITRConfig{VerifySchedule: "0 */6 * * *"},
		slog.Default(),
		"proj1",
		"db1",
	)
	if err != nil {
		t.Fatalf("NewFireDrillScheduler: %v", err)
	}
	if scheduler == nil {
		t.Fatal("expected scheduler")
	}
}

func TestNewFireDrillSchedulerInvalidCron(t *testing.T) {
	_, err := NewFireDrillScheduler(
		NewFireDrillRunner(NewRestorePlanner(newFakeRepo(), newFakeWALRepo(), newFakeManifestRepo()), nil, slog.Default()),
		PITRConfig{VerifySchedule: "not-cron"},
		slog.Default(),
		"proj1",
		"db1",
	)
	if err == nil {
		t.Fatal("expected error for invalid cron")
	}
}
