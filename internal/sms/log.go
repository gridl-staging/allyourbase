package sms

import (
	"context"
	"log/slog"
)

// LogProvider logs SMS sends instead of delivering them. Useful for development.
type LogProvider struct {
	logger *slog.Logger
}

// NewLogProvider creates a LogProvider. If logger is nil, slog.Default() is used.
func NewLogProvider(logger *slog.Logger) *LogProvider {
	if logger == nil {
		logger = slog.Default()
	}
	return &LogProvider{logger: logger}
}

func (p *LogProvider) Send(_ context.Context, to, body string) (*SendResult, error) {
	p.logger.Info("sms.LogProvider", "to", to, "body", body)
	return &SendResult{Status: "logged"}, nil
}
