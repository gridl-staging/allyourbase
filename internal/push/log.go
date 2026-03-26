package push

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// LogProvider logs push sends instead of delivering them.
type LogProvider struct {
	logger *slog.Logger
}

// NewLogProvider creates a LogProvider. If logger is nil, slog.Default is used.
func NewLogProvider(logger *slog.Logger) *LogProvider {
	if logger == nil {
		logger = slog.Default()
	}
	return &LogProvider{logger: logger}
}

func (p *LogProvider) Send(_ context.Context, token string, msg *Message) (*Result, error) {
	var title, body string
	var data map[string]string
	if msg != nil {
		title = msg.Title
		body = msg.Body
		data = msg.Data
	}
	p.logger.Info("push.LogProvider", "token", token, "title", title, "body", body, "data", data)
	return &Result{MessageID: fmt.Sprintf("log-%d", time.Now().UnixNano())}, nil
}
