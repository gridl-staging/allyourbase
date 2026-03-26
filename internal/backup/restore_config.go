// Package backup restore_config.go contains functions for configuring PostgreSQL recovery settings during point-in-time recovery (PITR).
package backup

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const recoveryTargetTimeLayout = "2006-01-02 15:04:05.000000+00"

// WriteRecoveryConfig writes recovery.signal and recovery settings for PITR.
func WriteRecoveryConfig(dataDir string, targetTime time.Time, walArchiveDir string) error {
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return fmt.Errorf("creating data directory: %w", err)
	}

	recoverySignalPath := filepath.Join(dataDir, "recovery.signal")
	if err := os.WriteFile(recoverySignalPath, []byte{}, 0o600); err != nil {
		return fmt.Errorf("writing recovery.signal: %w", err)
	}

	timestamp := targetTime.UTC().Format(recoveryTargetTimeLayout)
	confLines := []string{
		fmt.Sprintf("recovery_target_time = '%s'", escapePGConfLiteral(timestamp)),
		"recovery_target_action = 'promote'",
		fmt.Sprintf("restore_command = 'cp %s/%%f %%p'", escapePGConfLiteral(walArchiveDir)),
	}
	confPayload := strings.Join(confLines, "\n") + "\n"

	autoConfPath := filepath.Join(dataDir, "postgresql.auto.conf")
	if err := appendConfig(autoConfPath, confPayload); err != nil {
		return fmt.Errorf("writing postgresql.auto.conf: %w", err)
	}
	return nil
}

// appendConfig appends payload to the file at path, creating it if it doesn't exist and adding a newline prefix if existing content doesn't end with one.
func appendConfig(path, payload string) error {
	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	prefix := ""
	if len(existing) > 0 && existing[len(existing)-1] != '\n' {
		prefix = "\n"
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.WriteString(prefix + payload); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func escapePGConfLiteral(v string) string {
	return strings.ReplaceAll(v, "'", "''")
}
