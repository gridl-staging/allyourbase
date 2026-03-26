package backup

import (
	"log/slog"
	"testing"
)

func TestNewPhysicalSchedulerValidCron(t *testing.T) {
	s, err := NewPhysicalScheduler(&PhysicalEngine{}, PITRConfig{BaseBackupSchedule: "0 3 * * *"}, slog.Default())
	if err != nil {
		t.Fatalf("NewPhysicalScheduler: %v", err)
	}
	if s == nil {
		t.Fatal("expected scheduler")
	}
}

func TestNewPhysicalSchedulerInvalidCron(t *testing.T) {
	_, err := NewPhysicalScheduler(&PhysicalEngine{}, PITRConfig{BaseBackupSchedule: "not-cron"}, slog.Default())
	if err == nil {
		t.Fatal("expected invalid cron error")
	}
}
