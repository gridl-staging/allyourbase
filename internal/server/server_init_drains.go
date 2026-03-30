package server

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/logging"
)

// normalizeLogDrainConfig fills in default values for a LogDrainConfig.
func normalizeLogDrainConfig(cfg config.LogDrainConfig, index int) config.LogDrainConfig {
	if cfg.ID == "" {
		cfg.ID = fmt.Sprintf("drain-%d", index)
	}
	if cfg.Enabled == nil {
		enabled := true
		cfg.Enabled = &enabled
	}
	if cfg.BatchSize == 0 {
		cfg.BatchSize = 100
	}
	if cfg.FlushIntervalSecs == 0 {
		cfg.FlushIntervalSecs = 5
	}
	if cfg.Headers == nil {
		cfg.Headers = map[string]string{}
	}
	return cfg
}

// newLogDrainFromConfig creates a LogDrain from config, validating that type
// and URL are set and that batch and flush parameters are non-negative.
func newLogDrainFromConfig(cfg config.LogDrainConfig, transport http.RoundTripper) (logging.LogDrain, error) {
	if cfg.Type == "" {
		return nil, fmt.Errorf("type is required")
	}
	switch cfg.Type {
	case "http", "datadog", "loki":
	default:
		return nil, fmt.Errorf("unsupported log drain type: %q", cfg.Type)
	}
	if strings.TrimSpace(cfg.URL) == "" {
		return nil, fmt.Errorf("url is required")
	}
	if cfg.BatchSize < 0 {
		return nil, fmt.Errorf("batch_size must be non-negative, got %d", cfg.BatchSize)
	}
	if cfg.FlushIntervalSecs < 0 {
		return nil, fmt.Errorf("flush_interval_seconds must be non-negative, got %d", cfg.FlushIntervalSecs)
	}
	if cfg.Headers == nil {
		cfg.Headers = map[string]string{}
	}

	drainCfg := logging.DrainConfig{
		ID:                cfg.ID,
		Type:              cfg.Type,
		URL:               cfg.URL,
		Headers:           cfg.Headers,
		BatchSize:         cfg.BatchSize,
		FlushIntervalSecs: cfg.FlushIntervalSecs,
		Enabled:           cfg.Enabled != nil && *cfg.Enabled,
	}

	switch cfg.Type {
	case "http":
		drain := logging.NewHTTPDrain(drainCfg)
		if transport != nil {
			drain.SetHTTPTransport(transport)
		}
		return drain, nil
	case "datadog":
		drain := logging.NewDatadogDrain(drainCfg)
		if transport != nil {
			drain.SetHTTPTransport(transport)
		}
		return drain, nil
	case "loki":
		drain := logging.NewLokiDrain(drainCfg)
		if transport != nil {
			drain.SetHTTPTransport(transport)
		}
		return drain, nil
	}

	return nil, fmt.Errorf("unsupported log drain type: %q", cfg.Type)
}
