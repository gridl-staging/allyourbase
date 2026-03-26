package backup

import (
	"log/slog"
	"testing"
)

func TestNewPITRRetentionSchedulerValidCron(t *testing.T) {
	job := NewPITRRetentionJob(
		PITRConfig{RetentionSchedule: "0 4 * * *"},
		newFakeStore(),
		newFakeRepo(),
		newFakeWALRepo(),
		&fakeManifestRepo{},
		nil,
		slog.Default(),
		"pitr",
	)
	scheduler, err := NewPITRRetentionScheduler(
		job, PITRConfig{RetentionSchedule: "0 4 * * *"}, slog.Default(), "proj1", "db1",
		nil, nil, nil,
	)
	if err != nil {
		t.Fatalf("NewPITRRetentionScheduler: %v", err)
	}
	if scheduler == nil {
		t.Fatal("expected scheduler")
	}
}

func TestNewPITRRetentionSchedulerInvalidCron(t *testing.T) {
	job := NewPITRRetentionJob(
		PITRConfig{RetentionSchedule: "0 4 * * *"},
		newFakeStore(),
		newFakeRepo(),
		newFakeWALRepo(),
		&fakeManifestRepo{},
		nil,
		slog.Default(),
		"pitr",
	)
	if _, err := NewPITRRetentionScheduler(
		job, PITRConfig{RetentionSchedule: "not-cron"}, slog.Default(), "proj1", "db1",
		nil, nil, nil,
	); err == nil {
		t.Fatal("expected invalid cron error")
	}
}
