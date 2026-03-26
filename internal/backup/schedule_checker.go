// Package backup ScheduleChecker monitors whether physical backups are being performed according to a configured cron schedule, sending alerts on violations.
package backup

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/adhocore/gronx"
)

type ScheduleChecker struct {
	repo               Repo
	notify             Notifier
	baseBackupSchedule string
	logger             *slog.Logger
	nowFn              func() time.Time
}

func NewScheduleChecker(repo Repo, notify Notifier, baseBackupSchedule string, logger *slog.Logger) *ScheduleChecker {
	if notify == nil {
		notify = NoopNotifier{}
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &ScheduleChecker{
		repo:               repo,
		notify:             notify,
		baseBackupSchedule: baseBackupSchedule,
		logger:             logger,
		nowFn:              func() time.Time { return time.Now().UTC() },
	}
}

// Check verifies that physical backups have been performed on schedule for the given project and database, sending alert notifications if the latest backup is older than twice the configured schedule interval. Errors are returned only if the schedule computation or backup listing fails.
func (c *ScheduleChecker) Check(ctx context.Context, projectID, databaseID string) error {
	now := c.nowFn().UTC()
	if c.baseBackupSchedule == "" {
		return nil
	}

	interval, err := scheduleInterval(c.baseBackupSchedule, now)
	if err != nil {
		return fmt.Errorf("computing base backup interval from %q: %w", c.baseBackupSchedule, err)
	}

	backups, err := c.repo.ListPhysicalCompleted(ctx, projectID, databaseID)
	if err != nil {
		return fmt.Errorf("listing physical backups for %s/%s: %w", projectID, databaseID, err)
	}
	if len(backups) == 0 {
		c.notify.OnAlert(ctx, AlertEvent{
			ProjectID:  projectID,
			DatabaseID: databaseID,
			AlertType:  "base_backup_missed",
			Message:    "no completed physical backups found",
			Timestamp:  now,
			Metadata: map[string]string{
				"schedule": c.baseBackupSchedule,
			},
		})
		return nil
	}

	latest := latestCompletedBackup(backups)
	if latest == nil || latest.CompletedAt == nil {
		c.notify.OnAlert(ctx, AlertEvent{
			ProjectID:  projectID,
			DatabaseID: databaseID,
			AlertType:  "base_backup_missed",
			Message:    "latest completed physical backup has missing timestamp",
			Timestamp:  now,
			Metadata: map[string]string{
				"schedule": c.baseBackupSchedule,
			},
		})
		return nil
	}

	age := now.Sub(latest.CompletedAt.UTC())
	limit := 2 * interval
	if age > limit {
		c.notify.OnAlert(ctx, AlertEvent{
			ProjectID:  projectID,
			DatabaseID: databaseID,
			AlertType:  "base_backup_missed",
			Message:    "base backup schedule missed",
			Timestamp:  now,
			Metadata: map[string]string{
				"latest_backup_id":        latest.ID,
				"base_backup_schedule":    c.baseBackupSchedule,
				"age_seconds":             fmt.Sprintf("%.0f", age.Seconds()),
				"max_allowed_age_seconds": fmt.Sprintf("%.0f", limit.Seconds()),
				"interval_seconds":        fmt.Sprintf("%.0f", interval.Seconds()),
			},
		})
		c.logger.Warn("base backup schedule missed", "project", projectID, "database", databaseID)
	}

	return nil
}

func scheduleInterval(expr string, now time.Time) (time.Duration, error) {
	first, err := gronx.NextTickAfter(expr, now, false)
	if err != nil {
		return 0, err
	}
	second, err := gronx.NextTickAfter(expr, first.Add(time.Minute), false)
	if err != nil {
		return 0, err
	}
	if !second.After(first) {
		return 0, fmt.Errorf("invalid cron interval for expression %q", expr)
	}
	return second.Sub(first), nil
}

func latestCompletedBackup(backups []BackupRecord) *BackupRecord {
	var latest *BackupRecord
	for _, backup := range backups {
		if backup.CompletedAt == nil {
			continue
		}
		if latest == nil || backup.CompletedAt.After(*latest.CompletedAt) {
			candidate := backup
			latest = &candidate
		}
	}
	return latest
}
