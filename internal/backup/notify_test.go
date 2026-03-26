package backup

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestLogNotifierOnFailure(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelError}))
	n := NewLogNotifier(logger)

	n.OnFailure(context.Background(), FailureEvent{
		BackupID:  "id-123",
		DBName:    "mydb",
		Stage:     "backup",
		Err:       fmt.Errorf("something went wrong"),
		Timestamp: time.Now(),
	})

	out := buf.String()
	if !strings.Contains(out, "id-123") {
		t.Errorf("expected backup_id in log output: %q", out)
	}
	if !strings.Contains(out, "mydb") {
		t.Errorf("expected db_name in log output: %q", out)
	}
	if !strings.Contains(out, "backup") {
		t.Errorf("expected stage in log output: %q", out)
	}
}

func TestLogNotifierOnAlert(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))
	n := NewLogNotifier(logger)

	n.OnAlert(context.Background(), AlertEvent{
		ProjectID:  "proj1",
		DatabaseID: "db1",
		AlertType:  "wal_archive_lag",
		Message:    "WAL archive lag exceeded",
		Timestamp:  time.Now(),
		Metadata:   map[string]string{"seconds": "300"},
	})

	out := buf.String()
	if !strings.Contains(out, "proj1") {
		t.Errorf("expected project_id in log output: %q", out)
	}
	if !strings.Contains(out, "db1") {
		t.Errorf("expected database_id in log output: %q", out)
	}
	if !strings.Contains(out, "wal_archive_lag") {
		t.Errorf("expected alert_type in log output: %q", out)
	}
}

func TestNoopNotifierNoPanic(t *testing.T) {
	n := NoopNotifier{}
	// Must not panic or return error.
	n.OnFailure(context.Background(), FailureEvent{
		BackupID: "x",
		DBName:   "y",
		Stage:    "backup",
		Err:      fmt.Errorf("test"),
	})
}

func TestNoopNotifierOnAlertNoPanic(t *testing.T) {
	n := NoopNotifier{}
	n.OnAlert(context.Background(), AlertEvent{
		ProjectID:  "p",
		DatabaseID: "d",
		AlertType:  "storage_budget_warning",
		Message:    "over budget",
		Timestamp:  time.Now(),
	})
}
