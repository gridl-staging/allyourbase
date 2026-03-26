package backup

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"time"
)

// RetentionJob applies count-based and age-based retention policies for a database's backups.
type RetentionJob struct {
	cfg    Config
	store  Store
	repo   Repo
	notify Notifier
	logger *slog.Logger
}

// NewRetentionJob creates a RetentionJob.
func NewRetentionJob(cfg Config, store Store, repo Repo, notify Notifier, logger *slog.Logger) *RetentionJob {
	return &RetentionJob{cfg: cfg, store: store, repo: repo, notify: notify, logger: logger}
}

// RetentionResult summarises what was (or would be) deleted.
type RetentionResult struct {
	Deleted []string // backup IDs deleted
	Errors  []error
	DryRun  bool
}

// Run applies the configured retention policy for dbName.
// When dryRun is true, no deletions occur.
func (j *RetentionJob) Run(ctx context.Context, dbName string, dryRun bool) (RetentionResult, error) {
	completed, err := j.repo.CompletedByDB(ctx, dbName)
	if err != nil {
		return RetentionResult{}, fmt.Errorf("listing completed backups for retention: %w", err)
	}

	// Sort newest first.
	sort.Slice(completed, func(i, k int) bool {
		return completed[i].StartedAt.After(completed[k].StartedAt)
	})

	toDelete := j.computeDeletions(completed)
	result := RetentionResult{DryRun: dryRun}

	for _, rec := range toDelete {
		if dryRun {
			result.Deleted = append(result.Deleted, rec.ID)
			j.logger.Info("retention dry-run: would delete", "backup_id", rec.ID, "object_key", rec.ObjectKey)
			continue
		}

		deleteErr := j.deleteOne(ctx, rec)
		if deleteErr != nil {
			result.Errors = append(result.Errors, deleteErr)
			j.notify.OnFailure(ctx, FailureEvent{
				BackupID:  rec.ID,
				DBName:    dbName,
				Stage:     "retention",
				Err:       deleteErr,
				Timestamp: time.Now().UTC(),
			})
		} else {
			result.Deleted = append(result.Deleted, rec.ID)
		}
	}

	return result, nil
}

func (j *RetentionJob) deleteOne(ctx context.Context, rec BackupRecord) error {
	if rec.ObjectKey != "" {
		if err := j.store.DeleteObject(ctx, rec.ObjectKey); err != nil {
			return fmt.Errorf("delete S3 object %q: %w", rec.ObjectKey, err)
		}
	}
	if err := j.repo.UpdateStatus(ctx, rec.ID, StatusDeleted, ""); err != nil {
		return fmt.Errorf("mark backup %q deleted: %w", rec.ID, err)
	}
	return nil
}

// computeDeletions returns records that must be deleted per the retention policy.
// sorted is assumed to be newest-first.
func (j *RetentionJob) computeDeletions(sorted []BackupRecord) []BackupRecord {
	doomed := map[string]BackupRecord{}

	// Count-based: keep only the most recent RetentionCount.
	if j.cfg.RetentionCount > 0 && len(sorted) > j.cfg.RetentionCount {
		for _, rec := range sorted[j.cfg.RetentionCount:] {
			doomed[rec.ID] = rec
		}
	}

	// Age-based: delete backups older than RetentionDays.
	if j.cfg.RetentionDays > 0 {
		cutoff := time.Now().UTC().AddDate(0, 0, -j.cfg.RetentionDays)
		for _, rec := range sorted {
			if rec.StartedAt.Before(cutoff) {
				doomed[rec.ID] = rec
			}
		}
	}

	out := make([]BackupRecord, 0, len(doomed))
	for _, rec := range doomed {
		out = append(out, rec)
	}
	return out
}
