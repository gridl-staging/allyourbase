// Package backup fire drill functionality to test database restoration capabilities at specific points in time.
package backup

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

type FireDrillResult struct {
	ProjectID  string
	DatabaseID string
	TargetTime time.Time
	Passed     bool
	Plan       *RestorePlan
	Error      string
	RunAt      time.Time
}

type FireDrillRunner struct {
	planner *RestorePlanner
	notify  Notifier
	logger  *slog.Logger
	nowFn   func() time.Time
}

func NewFireDrillRunner(planner *RestorePlanner, notify Notifier, logger *slog.Logger) *FireDrillRunner {
	if notify == nil {
		notify = NoopNotifier{}
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &FireDrillRunner{
		planner: planner,
		notify:  notify,
		logger:  logger,
		nowFn:   func() time.Time { return time.Now().UTC() },
	}
}

// Run executes a fire drill to validate database restoration capability at a point five minutes in the past. It validates the restore window, logs the result, and sends an alert notification on failure.
func (r *FireDrillRunner) Run(ctx context.Context, projectID, databaseID string) (*FireDrillResult, error) {
	runAt := r.nowFn().UTC()
	target := runAt.Add(-5 * time.Minute)
	result := &FireDrillResult{
		ProjectID:  projectID,
		DatabaseID: databaseID,
		TargetTime: target,
		RunAt:      runAt,
	}

	plan, err := r.planner.ValidateWindow(ctx, projectID, databaseID, target)
	if err != nil {
		result.Error = err.Error()
		r.notify.OnAlert(ctx, AlertEvent{
			ProjectID:  projectID,
			DatabaseID: databaseID,
			AlertType:  "fire_drill_failed",
			Message:    fmt.Sprintf("fire drill failed: %v", err),
			Timestamp:  runAt,
			Metadata: map[string]string{
				"target_time": target.Format(time.RFC3339),
			},
		})
		r.logger.Warn("fire drill failed", "project", projectID, "database", databaseID, "error", err)
		return result, err
	}

	result.Passed = true
	result.Plan = plan
	r.logger.Info("fire drill passed", "project", projectID, "database", databaseID, "target_time", target.Format(time.RFC3339))
	return result, nil
}
