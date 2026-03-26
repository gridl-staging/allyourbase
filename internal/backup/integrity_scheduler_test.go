package backup

import (
	"context"
	"log/slog"
	"testing"
)

func TestNewIntegritySchedulerValidCron(t *testing.T) {
	verifier := NewIntegrityVerifier(
		newFakeRepo(), newFakeManifestRepo(), newFakeWALRepo(),
		newFakeStore(), &fakeIntegrityReportRepo{}, NoopNotifier{}, "pitr",
	)
	cfg := PITRConfig{VerifySchedule: "0 */6 * * *"}
	s, err := NewIntegrityScheduler(verifier, cfg, slog.Default(), "proj1", "db1", nil)
	if err != nil {
		t.Fatalf("NewIntegrityScheduler: %v", err)
	}
	if s == nil {
		t.Fatal("expected scheduler")
	}
}

func TestNewIntegritySchedulerInvalidCron(t *testing.T) {
	verifier := NewIntegrityVerifier(
		newFakeRepo(), newFakeManifestRepo(), newFakeWALRepo(),
		newFakeStore(), &fakeIntegrityReportRepo{}, NoopNotifier{}, "pitr",
	)
	cfg := PITRConfig{VerifySchedule: "not-cron"}
	_, err := NewIntegrityScheduler(verifier, cfg, slog.Default(), "proj1", "db1", nil)
	if err == nil {
		t.Fatal("expected invalid cron error")
	}
}

func TestIntegritySchedulerStartStop(t *testing.T) {
	verifier := NewIntegrityVerifier(
		newFakeRepo(), newFakeManifestRepo(), newFakeWALRepo(),
		newFakeStore(), &fakeIntegrityReportRepo{}, NoopNotifier{}, "pitr",
	)
	cfg := PITRConfig{VerifySchedule: "0 */6 * * *"}
	s, err := NewIntegrityScheduler(verifier, cfg, slog.Default(), "proj1", "db1", nil)
	if err != nil {
		t.Fatalf("NewIntegrityScheduler: %v", err)
	}

	ctx := context.Background()
	s.Start(ctx)
	s.Stop() // must not hang
}
