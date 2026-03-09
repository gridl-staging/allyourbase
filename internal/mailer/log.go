package mailer

import (
	"context"
	"log/slog"
)

// LogMailer logs emails to slog instead of sending them.
// This is the default when no email provider is configured —
// developers can see verification/reset links in the console.
type LogMailer struct {
	logger *slog.Logger
}

// NewLogMailer creates a LogMailer that writes to the given logger.
func NewLogMailer(logger *slog.Logger) *LogMailer {
	return &LogMailer{logger: logger}
}

func (m *LogMailer) Send(_ context.Context, msg *Message) error {
	m.logger.Info("email (dev mode — not sent)",
		"to", msg.To,
		"subject", msg.Subject,
		"text", msg.Text,
		"html_length", len(msg.HTML),
	)
	return nil
}
