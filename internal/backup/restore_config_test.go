package backup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRecoveryConfigWritesSignalAndAutoConf(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	target := time.Date(2026, 2, 3, 4, 5, 6, 789123000, time.UTC)
	walDir := filepath.Join(dir, "wal_archive")

	if err := WriteRecoveryConfig(dir, target, walDir); err != nil {
		t.Fatalf("WriteRecoveryConfig: %v", err)
	}

	signalPath := filepath.Join(dir, "recovery.signal")
	stat, err := os.Stat(signalPath)
	if err != nil {
		t.Fatalf("stat recovery.signal: %v", err)
	}
	if stat.Size() != 0 {
		t.Fatalf("recovery.signal size = %d; want 0", stat.Size())
	}

	autoConfPath := filepath.Join(dir, "postgresql.auto.conf")
	data, err := os.ReadFile(autoConfPath)
	if err != nil {
		t.Fatalf("read postgresql.auto.conf: %v", err)
	}
	conf := string(data)
	if !strings.Contains(conf, "recovery_target_time = '2026-02-03 04:05:06.789123+00'") {
		t.Fatalf("missing recovery_target_time, got:\n%s", conf)
	}
	if !strings.Contains(conf, "recovery_target_action = 'promote'") {
		t.Fatalf("missing recovery_target_action, got:\n%s", conf)
	}
	if !strings.Contains(conf, "restore_command = 'cp "+walDir+"/%f %p'") {
		t.Fatalf("missing restore_command, got:\n%s", conf)
	}
	if !strings.HasSuffix(conf, "\n") {
		t.Fatalf("postgresql.auto.conf must end with newline")
	}
}

func TestRecoveryConfigTimestampFormattingUTC(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	loc := time.FixedZone("EST", -5*60*60)
	target := time.Date(2026, 2, 3, 1, 2, 3, 45000, loc)

	if err := WriteRecoveryConfig(dir, target, "/tmp/wal"); err != nil {
		t.Fatalf("WriteRecoveryConfig: %v", err)
	}

	conf, err := os.ReadFile(filepath.Join(dir, "postgresql.auto.conf"))
	if err != nil {
		t.Fatalf("read conf: %v", err)
	}
	if !strings.Contains(string(conf), "recovery_target_time = '2026-02-03 06:02:03.000045+00'") {
		t.Fatalf("timestamp format mismatch:\n%s", string(conf))
	}
}
