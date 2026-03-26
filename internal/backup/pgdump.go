package backup

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
)

// DumpRunner executes pg_dump and streams output to a writer.
type DumpRunner struct {
	// PgDumpPath overrides the default pg_dump binary path.
	// When empty, exec.LookPath("pg_dump") is used.
	PgDumpPath string
}

// Dump executes pg_dump against the given database URL and writes the SQL
// dump to dst. It uses --format=plain for streaming gzip compatibility.
// The context controls timeout/cancellation.
func (d *DumpRunner) Dump(ctx context.Context, dbURL string, dst io.Writer) error {
	pgDump := d.PgDumpPath
	if pgDump == "" {
		var err error
		pgDump, err = exec.LookPath("pg_dump")
		if err != nil {
			return fmt.Errorf("pg_dump not found in PATH: %w", err)
		}
	}

	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, pgDump,
		"--dbname="+dbURL,
		"--format=plain",
		"--no-owner",
		"--no-privileges",
	)
	cmd.Stdout = dst
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("pg_dump cancelled: %w", ctx.Err())
		}
		return fmt.Errorf("pg_dump failed: %w: %s", err, stderr.String())
	}
	return nil
}

// RestoreRunner executes psql to restore a SQL dump.
type RestoreRunner struct {
	PsqlPath string
}

// Run restores a SQL dump from r into the database at dbURL.
func (rr *RestoreRunner) Run(ctx context.Context, dbURL string, r io.Reader) error {
	psql := rr.PsqlPath
	if psql == "" {
		var err error
		psql, err = exec.LookPath("psql")
		if err != nil {
			return fmt.Errorf("psql not found in PATH: %w", err)
		}
	}

	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, psql,
		"--dbname="+dbURL,
		"--quiet",
		"--no-psqlrc",
	)
	cmd.Stdin = r
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("psql restore cancelled: %w", ctx.Err())
		}
		return fmt.Errorf("psql restore failed: %w: %s", err, stderr.String())
	}
	return nil
}
