package backup

import (
	"context"
	"log/slog"
)

// PhysicalScheduler runs periodic physical base backups.
type PhysicalScheduler struct {
	runner *cronRunner
}

// NewPhysicalScheduler validates the PITR base backup schedule and returns a scheduler.
func NewPhysicalScheduler(engine *PhysicalEngine, cfg PITRConfig, logger *slog.Logger) (*PhysicalScheduler, error) {
	if logger == nil {
		logger = slog.Default()
	}
	onTick := func(ctx context.Context) {
		logger.Info("running scheduled physical backup")
		if err := engine.Run(ctx, "schedule"); err != nil {
			logger.Error("scheduled physical backup failed", "error", err)
		} else {
			logger.Info("scheduled physical backup completed")
		}
	}
	runner, err := newCronRunner(cfg.BaseBackupSchedule, "physical backup", logger, onTick)
	if err != nil {
		return nil, err
	}
	return &PhysicalScheduler{runner: runner}, nil
}

// Start begins the physical scheduler loop in the background.
func (s *PhysicalScheduler) Start(ctx context.Context) { s.runner.Start(ctx) }

// Stop cancels the scheduler and waits for in-flight work.
func (s *PhysicalScheduler) Stop() { s.runner.Stop() }
