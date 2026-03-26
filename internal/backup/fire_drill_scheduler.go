package backup

import (
	"context"
	"log/slog"
)

// FireDrillScheduler periodically runs PITR fire-drill checks.
type FireDrillScheduler struct {
	runner *cronRunner
}

// NewFireDrillScheduler creates a scheduler that runs fire-drill validation
// on the PITR verify cadence for the configured project/database.
func NewFireDrillScheduler(
	drillRunner *FireDrillRunner,
	cfg PITRConfig,
	logger *slog.Logger,
	projectID, databaseID string,
) (*FireDrillScheduler, error) {
	if logger == nil {
		logger = slog.Default()
	}

	onTick := func(ctx context.Context) {
		if drillRunner == nil {
			logger.Error("cannot run fire drill: runner is nil")
			return
		}

		logger.Info("running scheduled fire drill", "project", projectID, "database", databaseID)
		result, err := drillRunner.Run(ctx, projectID, databaseID)
		if err != nil {
			logger.Error("scheduled fire drill failed", "project", projectID, "database", databaseID, "error", err)
			return
		}

		logger.Info("scheduled fire drill passed",
			"project", projectID,
			"database", databaseID,
			"target_time", result.TargetTime,
		)
	}

	runner, err := newCronRunner(cfg.VerifySchedule, "fire drill", logger, onTick)
	if err != nil {
		return nil, err
	}
	return &FireDrillScheduler{runner: runner}, nil
}

func (s *FireDrillScheduler) Start(ctx context.Context) { s.runner.Start(ctx) }

func (s *FireDrillScheduler) Stop() { s.runner.Stop() }
