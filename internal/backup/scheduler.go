// Package backup scheduler.go defines cron-based scheduling primitives for periodic backup operations, including the cronRunner background loop and high-level Scheduler API.
package backup

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/adhocore/gronx"
)

// cronRunner encapsulates a cron-based scheduling loop.
// Both Scheduler and PhysicalScheduler delegate to this shared implementation.
type cronRunner struct {
	cronExpr string
	logger   *slog.Logger
	label    string
	onTick   func(ctx context.Context)
	cancel   context.CancelFunc
	wg       sync.WaitGroup
}

func newCronRunner(cronExpr, label string, logger *slog.Logger, onTick func(ctx context.Context)) (*cronRunner, error) {
	gron := gronx.New()
	if !gron.IsValid(cronExpr) {
		return nil, &InvalidCronError{Expr: cronExpr}
	}
	return &cronRunner{
		cronExpr: cronExpr,
		logger:   logger,
		label:    label,
		onTick:   onTick,
	}, nil
}

func (c *cronRunner) Start(ctx context.Context) {
	ctx, c.cancel = context.WithCancel(ctx)
	c.wg.Add(1)
	go c.loop(ctx)
	c.logger.Info(c.label+" scheduler started", "schedule", c.cronExpr)
}

func (c *cronRunner) Stop() {
	if c.cancel != nil {
		c.cancel()
	}
	c.wg.Wait()
	c.logger.Info(c.label + " scheduler stopped")
}

// loop runs the scheduling loop, repeatedly waiting for the next scheduled time according to the cron expression and executing the onTick callback. It computes the next scheduled time, waits for it using a timer, executes the callback upon arrival, and repeats until context cancellation or scheduling errors occur.
func (c *cronRunner) loop(ctx context.Context) {
	defer c.wg.Done()

	next, err := gronx.NextTickAfter(c.cronExpr, time.Now(), false)
	if err != nil {
		c.logger.Error("failed to compute next "+c.label+" time", "error", err)
		return
	}
	c.logger.Info("next scheduled "+c.label, "at", next.UTC())

	for {
		delay := time.Until(next)
		if delay < 0 {
			delay = 0
		}
		timer := time.NewTimer(delay)

		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
			c.onTick(ctx)

			next, err = gronx.NextTickAfter(c.cronExpr, time.Now(), false)
			if err != nil {
				c.logger.Error("failed to compute next "+c.label+" time", "error", err)
				return
			}
			c.logger.Info("next scheduled "+c.label, "at", next.UTC())
		}
	}
}

// Scheduler runs periodic backup jobs based on a cron expression.
type Scheduler struct {
	runner *cronRunner
}

// InvalidCronError is returned when a cron expression is invalid.
type InvalidCronError struct {
	Expr string
}

func (e *InvalidCronError) Error() string {
	return "invalid cron expression: " + e.Expr
}

// NewScheduler creates a scheduler. retention may be nil (skips cleanup).
// cronExpr must be a valid cron expression.
func NewScheduler(engine *Engine, retention *RetentionJob, cronExpr string, logger *slog.Logger) (*Scheduler, error) {
	onTick := func(ctx context.Context) {
		logger.Info("running scheduled backup")
		result := engine.Run(ctx, "schedule")
		if result.Err != nil {
			logger.Error("scheduled backup failed", "error", result.Err)
		} else {
			logger.Info("scheduled backup completed",
				"backup_id", result.BackupID,
				"size_bytes", result.SizeBytes,
			)
		}

		if retention != nil {
			res, retErr := retention.Run(ctx, engine.dbName, false)
			if retErr != nil {
				logger.Error("retention cleanup failed", "error", retErr)
			} else if len(res.Deleted) > 0 {
				logger.Info("retention cleanup", "deleted", len(res.Deleted), "errors", len(res.Errors))
			}
		}
	}

	runner, err := newCronRunner(cronExpr, "backup", logger, onTick)
	if err != nil {
		return nil, err
	}
	return &Scheduler{runner: runner}, nil
}

// Start begins the scheduler loop in the background.
func (s *Scheduler) Start(ctx context.Context) { s.runner.Start(ctx) }

// Stop cancels the scheduler and waits for in-flight work to finish.
func (s *Scheduler) Stop() { s.runner.Stop() }
