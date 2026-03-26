package backup

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync"
	"time"
)

// Status values for a backup record.
const (
	StatusPending   = "pending"
	StatusRunning   = "running"
	StatusCompleted = "completed"
	StatusFailed    = "failed"
	StatusDeleted   = "deleted"
)

// Dumper streams a plain SQL dump from a database.
type Dumper interface {
	Dump(ctx context.Context, dbURL string, dst io.Writer) error
}

// RunResult summarises the outcome of a single engine run.
type RunResult struct {
	BackupID  string
	ObjectKey string
	SizeBytes int64
	Checksum  string
	Status    string
	Err       error
}

// Engine orchestrates the backup lifecycle.
// It is safe for concurrent use; overlapping runs are skipped.
type Engine struct {
	cfg    Config
	store  Store
	repo   Repo
	dumper Dumper
	notify Notifier
	logger *slog.Logger
	dbName string
	dbURL  string

	mu      sync.Mutex
	running bool
}

// NewEngine creates an Engine with all dependencies wired.
func NewEngine(
	cfg Config,
	store Store,
	repo Repo,
	dumper Dumper,
	notify Notifier,
	logger *slog.Logger,
	dbName, dbURL string,
) *Engine {
	return &Engine{
		cfg:    cfg,
		store:  store,
		repo:   repo,
		dumper: dumper,
		notify: notify,
		logger: logger,
		dbName: dbName,
		dbURL:  dbURL,
	}
}

// Run executes a backup. triggeredBy describes the caller ("scheduler", "cli", "api").
// The result always indicates success or failure; it never panics.
func (e *Engine) Run(ctx context.Context, triggeredBy string) RunResult {
	e.mu.Lock()
	if e.running {
		e.mu.Unlock()
		return RunResult{Status: "skipped", Err: fmt.Errorf("backup already in progress")}
	}
	e.running = true
	e.mu.Unlock()
	defer func() {
		e.mu.Lock()
		e.running = false
		e.mu.Unlock()
	}()

	now := time.Now().UTC()
	rec, err := e.repo.Create(ctx, e.dbName, triggeredBy)
	if err != nil {
		return RunResult{
			Status: StatusFailed,
			Err:    fmt.Errorf("creating backup record: %w", err),
		}
	}

	objectKey := ObjectKey(e.cfg.Prefix, e.dbName, rec.ID, now)
	sizeBytes, checksum, runErr := e.runPipeline(ctx, objectKey)

	if runErr != nil {
		_ = e.repo.UpdateStatus(ctx, rec.ID, StatusFailed, runErr.Error())
		e.notify.OnFailure(ctx, FailureEvent{
			BackupID:  rec.ID,
			DBName:    e.dbName,
			Stage:     "backup",
			Err:       runErr,
			Timestamp: time.Now().UTC(),
		})
		return RunResult{
			BackupID: rec.ID,
			Status:   StatusFailed,
			Err:      runErr,
		}
	}

	completedAt := time.Now().UTC()
	if updateErr := e.repo.MarkCompleted(ctx, rec.ID, objectKey, sizeBytes, checksum, completedAt); updateErr != nil {
		e.logger.Warn("failed to mark backup completed", "backup_id", rec.ID, "err", updateErr)
	}

	return RunResult{
		BackupID:  rec.ID,
		ObjectKey: objectKey,
		SizeBytes: sizeBytes,
		Checksum:  checksum,
		Status:    StatusCompleted,
	}
}

// RunWithRecord executes the backup pipeline for a pre-created record.
// Used by AdminService for async triggering.
func (e *Engine) RunWithRecord(ctx context.Context, rec *BackupRecord) RunResult {
	e.mu.Lock()
	if e.running {
		e.mu.Unlock()
		return RunResult{BackupID: rec.ID, Status: "skipped", Err: fmt.Errorf("backup already in progress")}
	}
	e.running = true
	e.mu.Unlock()
	defer func() {
		e.mu.Lock()
		e.running = false
		e.mu.Unlock()
	}()

	now := time.Now().UTC()
	objectKey := ObjectKey(e.cfg.Prefix, e.dbName, rec.ID, now)
	sizeBytes, checksum, runErr := e.runPipeline(ctx, objectKey)

	if runErr != nil {
		_ = e.repo.UpdateStatus(ctx, rec.ID, StatusFailed, runErr.Error())
		e.notify.OnFailure(ctx, FailureEvent{
			BackupID:  rec.ID,
			DBName:    e.dbName,
			Stage:     "backup",
			Err:       runErr,
			Timestamp: time.Now().UTC(),
		})
		return RunResult{BackupID: rec.ID, Status: StatusFailed, Err: runErr}
	}

	completedAt := time.Now().UTC()
	if updateErr := e.repo.MarkCompleted(ctx, rec.ID, objectKey, sizeBytes, checksum, completedAt); updateErr != nil {
		e.logger.Warn("failed to mark backup completed", "backup_id", rec.ID, "err", updateErr)
	}

	return RunResult{
		BackupID:  rec.ID,
		ObjectKey: objectKey,
		SizeBytes: sizeBytes,
		Checksum:  checksum,
		Status:    StatusCompleted,
	}
}

// runPipeline performs pg_dump → compress → upload atomically.
// Returns size and checksum of the uploaded artifact.
func (e *Engine) runPipeline(ctx context.Context, objectKey string) (int64, string, error) {
	pr, pw := io.Pipe()

	dumpErrCh := make(chan error, 1)
	go func() {
		err := e.dumper.Dump(ctx, e.dbURL, pw)
		pw.CloseWithError(err)
		dumpErrCh <- err
	}()

	result, cleanup, archErr := CompressToTempFile(pr)
	dumpErr := <-dumpErrCh

	if archErr != nil {
		if cleanup != nil {
			cleanup()
		}
		return 0, "", fmt.Errorf("compress: %w", archErr)
	}
	if dumpErr != nil {
		cleanup()
		return 0, "", fmt.Errorf("pg_dump: %w", dumpErr)
	}
	defer cleanup()

	f, err := os.Open(result.Path)
	if err != nil {
		return 0, "", fmt.Errorf("opening compressed artifact: %w", err)
	}
	defer f.Close()

	if err := e.store.PutObject(ctx, objectKey, f, result.Size, "application/gzip"); err != nil {
		return 0, "", fmt.Errorf("s3 upload: %w", err)
	}

	return result.Size, result.Checksum, nil
}
