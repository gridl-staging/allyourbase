// Package backup IntegrityScheduler implements scheduled integrity verification for database backups, running periodic verification and WAL lag checks using a cron-based scheduler.
package backup

import (
	"context"
	"log/slog"
)

type IntegrityScheduler struct {
	runner *cronRunner
}

// creates an IntegrityScheduler that periodically runs integrity verification checks and WAL lag checks according to the configured schedule, logging results for each run.
func NewIntegrityScheduler(
	verifier *IntegrityVerifier,
	cfg PITRConfig,
	logger *slog.Logger,
	projectID, databaseID string,
	lagChecker *WALLagChecker,
) (*IntegrityScheduler, error) {
	if logger == nil {
		logger = slog.Default()
	}
	onTick := func(ctx context.Context) {
		logger.Info("running scheduled integrity verification", "project", projectID, "database", databaseID)
		report, err := verifier.Verify(ctx, projectID, databaseID)
		if err != nil {
			logger.Error("scheduled integrity verification failed", "error", err)
		} else if report.Status == "fail" {
			logger.Error("integrity verification failed", "project", projectID, "database", databaseID, "status", report.Status, "checks", len(report.Checks))
		} else {
			logger.Info("integrity verification passed", "project", projectID, "database", databaseID, "checks", len(report.Checks))
		}

		if lagChecker != nil {
			if err := lagChecker.Check(ctx, projectID, databaseID); err != nil {
				logger.Warn("WAL lag check failed", "error", err)
			}
		}
	}

	runner, err := newCronRunner(cfg.VerifySchedule, "integrity verification", logger, onTick)
	if err != nil {
		return nil, err
	}
	return &IntegrityScheduler{runner: runner}, nil
}

func (s *IntegrityScheduler) Start(ctx context.Context) { s.runner.Start(ctx) }

func (s *IntegrityScheduler) Stop() { s.runner.Stop() }
