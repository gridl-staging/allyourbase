// Package backup WALLagChecker monitors Write-Ahead Log archival latency and sends alerts when lag exceeds the configured Recovery Point Objective threshold.
package backup

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

type WALLagChecker struct {
	repo       WALSegmentRepo
	notify     Notifier
	RPOMinutes int
	logger     *slog.Logger
	nowFn      func() time.Time
}

func NewWALLagChecker(repo WALSegmentRepo, notify Notifier, RPOMinutes int, logger *slog.Logger) *WALLagChecker {
	if notify == nil {
		notify = NoopNotifier{}
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &WALLagChecker{
		repo:       repo,
		notify:     notify,
		RPOMinutes: RPOMinutes,
		logger:     logger,
		nowFn:      func() time.Time { return time.Now().UTC() },
	}
}

// Check examines WAL archive lag for a project and database by querying the latest archived segment and sending alerts if no segments exist or if the lag exceeds the configured RPO threshold. It returns an error only if the repository query fails.
func (c *WALLagChecker) Check(ctx context.Context, projectID, databaseID string) error {
	latest, err := c.repo.LatestByProject(ctx, projectID, databaseID)
	if err != nil {
		if !isNoRowsError(err) {
			return fmt.Errorf("querying latest WAL segment for %s/%s: %w", projectID, databaseID, err)
		}
	}
	if latest == nil {
		c.notify.OnAlert(ctx, AlertEvent{
			ProjectID:  projectID,
			DatabaseID: databaseID,
			AlertType:  "wal_archive_lag",
			Message:    "no WAL segments found",
			Timestamp:  c.nowFn(),
			Metadata: map[string]string{
				"project":  projectID,
				"database": databaseID,
				"reason":   "no_segments",
			},
		})
		return nil
	}

	lag := c.nowFn().Sub(latest.ArchivedAt)
	threshold := time.Duration(c.RPOMinutes) * time.Minute
	if c.RPOMinutes <= 0 {
		threshold = 0
	}
	if lag > threshold {
		c.notify.OnAlert(ctx, AlertEvent{
			ProjectID:  projectID,
			DatabaseID: databaseID,
			AlertType:  "wal_archive_lag",
			Message:    "WAL archive lag exceeded configured RPO",
			Timestamp:  c.nowFn(),
			Metadata: map[string]string{
				"lag_minutes":   fmt.Sprintf("%0.2f", lag.Minutes()),
				"threshold_rpo": fmt.Sprintf("%d", c.RPOMinutes),
			},
		})
		c.logger.Warn("wal archive lag exceeded", "project", projectID, "database", databaseID, "lag_minutes", lag.Minutes())
	}

	return nil
}
