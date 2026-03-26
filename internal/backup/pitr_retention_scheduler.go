// Package backup implements point-in-time recovery retention scheduling, which periodically cleans up expired backups and WAL files while enforcing system health constraints.
package backup

import (
	"context"
	"log/slog"
)

// PITRRetentionScheduler runs PITR retention at a cron interval.
type PITRRetentionScheduler struct {
	runner          *cronRunner
	job             *PITRRetentionJob
	walLagChecker   *WALLagChecker
	budgetChecker   *StorageBudgetChecker
	scheduleChecker *ScheduleChecker
	projectID       string
	databaseID      string
}

// NewPITRRetentionScheduler creates a scheduler that runs point-in-time recovery retention at the configured cron interval, cleaning up expired backups and WAL files while monitoring WAL lag, schedule compliance, and storage budget usage.
func NewPITRRetentionScheduler(
	job *PITRRetentionJob,
	cfg PITRConfig,
	logger *slog.Logger,
	projectID, databaseID string,
	walLagChecker *WALLagChecker,
	budgetChecker *StorageBudgetChecker,
	scheduleChecker *ScheduleChecker,
) (*PITRRetentionScheduler, error) {
	if logger == nil {
		logger = slog.Default()
	}

	onTick := func(ctx context.Context) {
		logger.Info("running scheduled PITR retention", "project", projectID, "database", databaseID)
		if job == nil {
			logger.Error("cannot run PITR retention: job is nil")
			return
		}
		result, err := job.Run(ctx, projectID, databaseID, false)
		if err != nil {
			logger.Error("PITR retention run failed", "error", err)
		} else if len(result.Errors) > 0 {
			logger.Info("PITR retention completed with errors", "deleted_backups", len(result.DeletedBackups), "deleted_wal", len(result.DeletedWAL), "errors", len(result.Errors))
		}

		if walLagChecker != nil {
			if err := walLagChecker.Check(ctx, projectID, databaseID); err != nil {
				logger.Error("WAL lag check failed", "error", err)
			}
		}
		if scheduleChecker != nil {
			if err := scheduleChecker.Check(ctx, projectID, databaseID); err != nil {
				logger.Error("schedule check failed", "error", err)
			}
		}
		if budgetChecker != nil {
			if err := budgetChecker.Check(ctx, projectID, databaseID, cfg.StorageBudgetBytes); err != nil {
				logger.Error("storage budget check failed", "error", err)
			}
		}

		if len(result.DeletedBackups) > 0 || len(result.DeletedWAL) > 0 {
			logger.Info("PITR retention completed", "deleted_backups", len(result.DeletedBackups), "deleted_wal", len(result.DeletedWAL))
		}
	}

	runner, err := newCronRunner(cfg.RetentionSchedule, "pitr retention", logger, onTick)
	if err != nil {
		return nil, err
	}
	return &PITRRetentionScheduler{
		runner:          runner,
		job:             job,
		walLagChecker:   walLagChecker,
		budgetChecker:   budgetChecker,
		scheduleChecker: scheduleChecker,
		projectID:       projectID,
		databaseID:      databaseID,
	}, nil
}

func (s *PITRRetentionScheduler) Start(ctx context.Context) { s.runner.Start(ctx) }
func (s *PITRRetentionScheduler) Stop()                     { s.runner.Stop() }
