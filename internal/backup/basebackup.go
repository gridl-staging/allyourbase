package backup

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
)

// BaseBackupRunner executes pg_basebackup and streams a tar archive.
type BaseBackupRunner struct {
	DBURL string
	// PgBaseBackupPath overrides binary path. Empty uses PATH lookup.
	PgBaseBackupPath string
}

// NewBaseBackupRunner constructs a BaseBackupRunner for a database URL.
func NewBaseBackupRunner(dbURL string) *BaseBackupRunner {
	return &BaseBackupRunner{DBURL: dbURL}
}

// Run starts pg_basebackup and returns a reader for stdout tar bytes.
func (b *BaseBackupRunner) Run(ctx context.Context) (io.ReadCloser, error) {
	pgBaseBackup := b.PgBaseBackupPath
	if pgBaseBackup == "" {
		var err error
		pgBaseBackup, err = exec.LookPath("pg_basebackup")
		if err != nil {
			return nil, fmt.Errorf("pg_basebackup not found in PATH: %w", err)
		}
	}

	pr, pw := io.Pipe()
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, pgBaseBackup, buildBaseBackupArgs(b.DBURL)...)
	cmd.Stdout = pw
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		_ = pw.Close()
		_ = pr.Close()
		return nil, fmt.Errorf("starting pg_basebackup: %w", err)
	}

	go func() {
		err := cmd.Wait()
		if err != nil {
			if ctx.Err() != nil {
				_ = pw.CloseWithError(fmt.Errorf("pg_basebackup cancelled: %w", ctx.Err()))
				return
			}
			_ = pw.CloseWithError(fmt.Errorf("pg_basebackup failed: %w: %s", err, stderr.String()))
			return
		}
		_ = pw.Close()
	}()

	return pr, nil
}

func buildBaseBackupArgs(dbURL string) []string {
	return []string{
		"--dbname=" + dbURL,
		"--format=tar",
		"--checkpoint=fast",
		"--wal-method=none",
		"-D",
		"-",
	}
}
