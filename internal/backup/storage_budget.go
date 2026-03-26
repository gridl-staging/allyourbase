// Package backup StorageBudgetChecker monitors backup and WAL segment storage usage against configured budgets and sends alert notifications when thresholds are exceeded.
package backup

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

type StorageBudgetChecker struct {
	repo    Repo
	walRepo WALSegmentRepo
	notify  Notifier
	logger  *slog.Logger
}

func NewStorageBudgetChecker(repo Repo, walRepo WALSegmentRepo, notify Notifier, logger *slog.Logger) *StorageBudgetChecker {
	if notify == nil {
		notify = NoopNotifier{}
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &StorageBudgetChecker{
		repo:    repo,
		walRepo: walRepo,
		notify:  notify,
		logger:  logger,
	}
}

// Check validates that backup and WAL segment storage usage stays within the specified budget. It sums the sizes of all completed physical backups and WAL segments, then sends an alert notification and logs a warning if usage exceeds 70%, 90%, or 100% of the budget. A budget of zero or less disables the check.
func (c *StorageBudgetChecker) Check(ctx context.Context, projectID, databaseID string, budgetBytes int64) error {
	if budgetBytes <= 0 {
		return nil
	}

	now := time.Now().UTC()
	walBytes, err := c.walRepo.SumSizeBytes(ctx, projectID, databaseID)
	if err != nil {
		return fmt.Errorf("querying wal total for %s/%s: %w", projectID, databaseID, err)
	}

	backups, err := c.repo.ListPhysicalCompleted(ctx, projectID, databaseID)
	if err != nil {
		return fmt.Errorf("listing physical backups for %s/%s: %w", projectID, databaseID, err)
	}

	var baseBytes int64
	for _, rec := range backups {
		baseBytes += rec.SizeBytes
	}
	total := baseBytes + walBytes
	used := float64(total)
	budget := float64(budgetBytes)
	usedPct := 0.0
	if budget > 0 {
		usedPct = (used / budget) * 100
	}

	alertType := ""
	switch {
	case usedPct >= 100:
		alertType = "storage_budget_exceeded"
	case usedPct >= 90:
		alertType = "storage_budget_critical"
	case usedPct >= 70:
		alertType = "storage_budget_warning"
	}
	if alertType == "" {
		return nil
	}

	c.notify.OnAlert(ctx, AlertEvent{
		ProjectID:  projectID,
		DatabaseID: databaseID,
		AlertType:  alertType,
		Message:    "storage budget threshold exceeded",
		Timestamp:  now,
		Metadata: map[string]string{
			"budget_bytes": fmt.Sprintf("%d", budgetBytes),
			"used_bytes":   fmt.Sprintf("%d", total),
			"used_percent": fmt.Sprintf("%.2f", usedPct),
			"wal_bytes":    fmt.Sprintf("%d", walBytes),
		},
	})
	c.logger.Warn("storage budget alert", "project", projectID, "database", databaseID, "used_percent", usedPct)
	return nil
}
