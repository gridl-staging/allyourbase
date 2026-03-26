// Package backup PhysicalEngine orchestrates physical backups via pg_basebackup, compression, and object storage.
package backup

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
)

// PhysicalEngine orchestrates pg_basebackup -> zstd -> object store.
// PhysicalEngine orchestrates the physical backup pipeline, executing pg_basebackup, compressing with zstd, storing to object storage, and updating backup records.
type PhysicalEngine struct {
	cfg            PITRConfig
	store          Store
	repo           Repo
	runner         *BaseBackupRunner
	notify         Notifier
	projectID      string
	databaseID     string
	manifestWriter *ManifestWriter

	mu      sync.Mutex
	running bool

	runBackup func(ctx context.Context) (io.ReadCloser, error)
	lsnFn     func(ctx context.Context) (string, error)
}

// NewPhysicalEngine wires a new physical backup engine.
func NewPhysicalEngine(
	cfg PITRConfig,
	store Store,
	repo Repo,
	runner *BaseBackupRunner,
	notify Notifier,
	projectID, databaseID string,
	manifestWriter *ManifestWriter,
) *PhysicalEngine {
	if runner == nil {
		runner = &BaseBackupRunner{}
	}
	if notify == nil {
		notify = NoopNotifier{}
	}
	engine := &PhysicalEngine{
		cfg:            cfg,
		store:          store,
		repo:           repo,
		runner:         runner,
		notify:         notify,
		projectID:      projectID,
		databaseID:     databaseID,
		manifestWriter: manifestWriter,
	}
	engine.runBackup = runner.Run
	engine.lsnFn = func(ctx context.Context) (string, error) {
		return queryCurrentWALLSN(ctx, runner.DBURL)
	}
	return engine
}

// Run creates a physical backup record and executes the backup pipeline.
func (e *PhysicalEngine) Run(ctx context.Context, triggeredBy string) error {
	if err := e.acquireRunGuard(); err != nil {
		return err
	}
	defer e.releaseRunGuard()

	rec, err := e.repo.CreatePhysical(ctx, e.projectID, e.databaseID, triggeredBy)
	if err != nil {
		return fmt.Errorf("creating physical backup record: %w", err)
	}
	return e.runWithRecord(ctx, rec)
}

// RunWithRecord executes the backup pipeline for a pre-created backup record.
func (e *PhysicalEngine) RunWithRecord(ctx context.Context, rec *BackupRecord) error {
	if err := e.acquireRunGuard(); err != nil {
		return err
	}
	defer e.releaseRunGuard()
	return e.runWithRecord(ctx, rec)
}

func (e *PhysicalEngine) acquireRunGuard() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.running {
		return fmt.Errorf("physical backup already in progress")
	}
	e.running = true
	return nil
}

func (e *PhysicalEngine) releaseRunGuard() {
	e.mu.Lock()
	e.running = false
	e.mu.Unlock()
}

// runWithRecord executes the complete backup pipeline for a pre-created record: captures start and end WAL LSNs, runs pg_basebackup, compresses output with zstd, uploads to object storage, and marks the record completed. On errors, it fails the record and notifies registered handlers.
func (e *PhysicalEngine) runWithRecord(ctx context.Context, rec *BackupRecord) error {
	if rec == nil {
		return fmt.Errorf("backup record is nil")
	}

	startLSN, err := e.lsnFn(ctx)
	if err != nil {
		return e.failRecord(ctx, rec, "physical_backup", fmt.Errorf("capturing start LSN: %w", err))
	}

	stream, err := e.runBackup(ctx)
	if err != nil {
		return e.failRecord(ctx, rec, "physical_backup", fmt.Errorf("running pg_basebackup: %w", err))
	}

	compressResult, cleanup, err := CompressZstdToTempFile(stream)
	closeErr := stream.Close()
	if err != nil {
		if cleanup != nil {
			cleanup()
		}
		return e.failRecord(ctx, rec, "physical_backup", fmt.Errorf("compressing base backup: %w", err))
	}
	if closeErr != nil {
		cleanup()
		return e.failRecord(ctx, rec, "physical_backup", fmt.Errorf("closing base backup stream: %w", closeErr))
	}
	defer cleanup()

	endLSN, err := e.lsnFn(ctx)
	if err != nil {
		return e.failRecord(ctx, rec, "physical_backup", fmt.Errorf("capturing end LSN: %w", err))
	}

	objectKey := BaseBackupKey(
		e.cfg.ArchivePrefix,
		e.projectID,
		e.databaseID,
		lsnForObjectKey(startLSN),
		time.Now().UTC(),
	)

	file, err := os.Open(compressResult.Path)
	if err != nil {
		return e.failRecord(ctx, rec, "physical_backup", fmt.Errorf("opening compressed base backup artifact: %w", err))
	}
	defer file.Close()

	if err := e.store.PutObject(ctx, objectKey, file, compressResult.Size, "application/zstd"); err != nil {
		return e.failRecord(ctx, rec, "physical_backup", fmt.Errorf("uploading physical backup: %w", err))
	}

	if e.manifestWriter != nil {
		rec.ObjectKey = objectKey
		rec.StartLSN = &startLSN
		rec.EndLSN = &endLSN
		rec.Checksum = compressResult.Checksum
		if err := e.manifestWriter.WriteForBackup(ctx, rec); err != nil {
			return e.failRecord(ctx, rec, "manifest_write", fmt.Errorf("writing manifest: %w", err))
		}
	}

	if err := e.repo.MarkPhysicalCompleted(
		ctx,
		rec.ID,
		objectKey,
		compressResult.Size,
		compressResult.Checksum,
		startLSN,
		endLSN,
		time.Now().UTC(),
	); err != nil {
		return e.failRecord(ctx, rec, "physical_backup", fmt.Errorf("marking physical backup completed: %w", err))
	}

	return nil
}

func (e *PhysicalEngine) failRecord(ctx context.Context, rec *BackupRecord, stage string, runErr error) error {
	_ = e.repo.UpdateStatus(ctx, rec.ID, StatusFailed, runErr.Error())
	e.notify.OnFailure(ctx, FailureEvent{
		BackupID:  rec.ID,
		DBName:    e.databaseID,
		Stage:     stage,
		Err:       runErr,
		Timestamp: time.Now().UTC(),
	})
	return runErr
}

func lsnForObjectKey(lsn string) string {
	return strings.ReplaceAll(lsn, "/", "_")
}

// queryCurrentWALLSN queries PostgreSQL for the current WAL LSN position using the provided database URL.
func queryCurrentWALLSN(ctx context.Context, dbURL string) (string, error) {
	if strings.TrimSpace(dbURL) == "" {
		return "", fmt.Errorf("database URL is required to query WAL LSN")
	}
	conn, err := pgx.Connect(ctx, dbURL)
	if err != nil {
		return "", fmt.Errorf("connecting for LSN query: %w", err)
	}
	defer conn.Close(ctx)

	var lsn string
	if err := conn.QueryRow(ctx, "SELECT pg_current_wal_lsn()::text").Scan(&lsn); err != nil {
		return "", fmt.Errorf("querying current WAL LSN: %w", err)
	}
	return lsn, nil
}
