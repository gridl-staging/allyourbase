// Package cli Provides database restoration functionality from S3 or local backups using PostgreSQL command-line tools.
package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/allyourbase/ayb/internal/backup"
	"github.com/spf13/cobra"
)

// runDBRestore handles database restoration from either an S3 backup specified via the --from flag or a local file path provided as an argument.
func runDBRestore(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	fromFlag, _ := cmd.Flags().GetString("from")
	if fromFlag != "" {
		return runDBRestoreFromS3(ctx, cmd, fromFlag)
	}

	if len(args) == 0 {
		return fmt.Errorf("provide a file path as argument or use --from <backup-id> for S3 restore")
	}

	dbURL, err := resolveDBURL(cmd)
	if err != nil {
		return err
	}
	return runDBRestoreLocal(cmd, dbURL, args[0])
}

// runDBRestoreFromS3 restores a database from an S3-backed backup, supporting backup ID lookup and decompression, with user confirmation to prevent accidental overwrites.
func runDBRestoreFromS3(ctx context.Context, cmd *cobra.Command, from string) error {
	cfg, err := loadDBConfig(cmd)
	if err != nil {
		return err
	}
	if !cfg.Backup.Enabled {
		return fmt.Errorf("backups are not enabled in config — cannot restore from S3")
	}

	dbURL, err := resolveDBURL(cmd)
	if err != nil {
		return err
	}

	yes, _ := cmd.Flags().GetBool("yes")
	if !yes {
		fmt.Fprintf(cmd.OutOrStdout(), "Restore database from backup %q? This will OVERWRITE the current database. [y/N]: ", from)
		reader := bufio.NewReader(os.Stdin)
		answer, _ := reader.ReadString('\n')
		answer = strings.TrimSpace(strings.ToLower(answer))
		if answer != "y" && answer != "yes" {
			fmt.Fprintln(cmd.OutOrStdout(), "Restore cancelled.")
			return nil
		}
	}

	store, err := s3StoreFromConfig(ctx, cfg)
	if err != nil {
		return fmt.Errorf("initialising S3 client: %w", err)
	}

	objectKey := from
	if !strings.Contains(from, "/") {
		pool, poolErr := openPool(ctx, dbURL)
		if poolErr != nil {
			return poolErr
		}
		defer pool.Close()

		repo := backup.NewRepository(pool)
		record, lookupErr := repo.Get(ctx, from)
		if lookupErr != nil {
			return fmt.Errorf("looking up backup %q: %w", from, lookupErr)
		}
		if record.Status != "completed" {
			return fmt.Errorf("backup %q has status %q, cannot restore", from, record.Status)
		}
		objectKey = record.ObjectKey
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Downloading %q from S3...\n", objectKey)
	body, _, err := store.GetObject(ctx, objectKey)
	if err != nil {
		return fmt.Errorf("downloading backup: %w", err)
	}
	defer body.Close()

	decompressedBody, err := backup.DecompressReader(body)
	if err != nil {
		return fmt.Errorf("decompressing backup: %w", err)
	}
	defer decompressedBody.Close()

	fmt.Fprintln(cmd.OutOrStdout(), "Restoring to database...")
	restorer := &backup.RestoreRunner{}
	if err := restorer.Run(ctx, dbURL, decompressedBody); err != nil {
		return fmt.Errorf("restore failed: %w", err)
	}

	fmt.Fprintln(cmd.OutOrStdout(), "Restore complete.")
	return nil
}

func runDBRestoreLocal(cmd *cobra.Command, dbURL, inputPath string) error {
	fileInfo, err := os.Stat(inputPath)
	if err != nil {
		return fmt.Errorf("backup file not found: %s", inputPath)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Restoring database from %s (%d bytes)...\n", inputPath, fileInfo.Size())

	extension := filepath.Ext(inputPath)
	if extension == ".dump" || extension == ".tar" {
		return runPgRestore(cmd, dbURL, inputPath)
	}

	return runPSQLRestore(cmd, dbURL, inputPath, extension)
}

// runPgRestore restores a database from a binary dump or tar format backup using the PostgreSQL pg_restore command.
func runPgRestore(cmd *cobra.Command, dbURL, inputPath string) error {
	pgRestorePath, err := exec.LookPath("pg_restore")
	if err != nil {
		return fmt.Errorf("pg_restore not found in PATH: install PostgreSQL client tools")
	}

	command := exec.Command(pgRestorePath,
		"--dbname="+dbURL,
		"--clean",
		"--if-exists",
		inputPath,
	)
	command.Stdout = cmd.OutOrStdout()
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("pg_restore failed: %w", err)
	}

	fmt.Fprintln(cmd.OutOrStdout(), "Restore complete.")
	return nil
}

// runPSQLRestore restores a database from a SQL text file using the PostgreSQL psql command, transparently handling gzip compression.
func runPSQLRestore(cmd *cobra.Command, dbURL, inputPath, extension string) error {
	psqlPath, err := exec.LookPath("psql")
	if err != nil {
		return fmt.Errorf("psql not found in PATH: install PostgreSQL client tools")
	}

	stdinReader, closeReader, err := openRestoreInputReader(inputPath, extension)
	if err != nil {
		return err
	}
	defer closeReader()

	command := exec.Command(psqlPath, "--dbname="+dbURL)
	command.Stdin = stdinReader
	command.Stdout = cmd.OutOrStdout()
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("psql failed: %w", err)
	}

	fmt.Fprintln(cmd.OutOrStdout(), "Restore complete.")
	return nil
}

// openRestoreInputReader opens a backup file and returns a reader with decompression applied if the file is gzip-compressed, along with a cleanup function.
func openRestoreInputReader(inputPath, extension string) (io.Reader, func(), error) {
	file, err := os.Open(inputPath)
	if err != nil {
		return nil, nil, fmt.Errorf("opening backup file: %w", err)
	}

	cleanup := func() {
		_ = file.Close()
	}

	if extension != ".gz" && !strings.HasSuffix(inputPath, ".sql.gz") {
		return file, cleanup, nil
	}

	gzipReader, err := backup.DecompressReader(file)
	if err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("opening gzip: %w", err)
	}

	return gzipReader, func() {
		_ = gzipReader.Close()
		cleanup()
	}, nil
}
