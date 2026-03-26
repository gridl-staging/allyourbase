package backup

import (
	"context"
	"log/slog"
	"time"
)

// FailureEvent carries context about a backup or retention failure.
type FailureEvent struct {
	BackupID  string
	DBName    string
	Stage     string // "backup" | "retention"
	Err       error
	Timestamp time.Time
}

// AlertEvent carries non-failure operational alerts.
type AlertEvent struct {
	ProjectID  string
	DatabaseID string
	AlertType  string
	Message    string
	Timestamp  time.Time
	Metadata   map[string]string
}

// NotifyPayload is the payload sent to a NotifyHook.
type NotifyPayload struct {
	BackupID  string
	DBName    string
	Stage     string
	Error     string
	Timestamp time.Time
}

// NotifyHook is a function that receives failure payloads.
type NotifyHook func(payload NotifyPayload)

// Notifier is called on backup or retention failures.
type Notifier interface {
	OnFailure(ctx context.Context, evt FailureEvent)
	OnAlert(ctx context.Context, evt AlertEvent)
}

// LogNotifier logs failure events via slog. Used by default when no external
// notification hook is configured.
type LogNotifier struct {
	logger *slog.Logger
}

// NewLogNotifier creates a Notifier that logs via slog.
func NewLogNotifier(logger *slog.Logger) Notifier {
	return &LogNotifier{logger: logger}
}

func (n *LogNotifier) OnFailure(_ context.Context, evt FailureEvent) {
	n.logger.Error("backup failure",
		"backup_id", evt.BackupID,
		"db_name", evt.DBName,
		"stage", evt.Stage,
		"error", evt.Err,
		"timestamp", evt.Timestamp,
	)
}

func (n *LogNotifier) OnAlert(_ context.Context, evt AlertEvent) {
	n.logger.Warn("backup alert",
		"project_id", evt.ProjectID,
		"database_id", evt.DatabaseID,
		"alert_type", evt.AlertType,
		"message", evt.Message,
		"timestamp", evt.Timestamp,
	)
}

// NoopNotifier silently discards failure events. Useful in tests.
type NoopNotifier struct{}

func (NoopNotifier) OnFailure(_ context.Context, _ FailureEvent) {}

func (NoopNotifier) OnAlert(_ context.Context, _ AlertEvent) {}
