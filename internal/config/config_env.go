package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// envInt reads an integer from the named environment variable.
// Returns an error if the value is set but not a valid integer.
func envInt(name string, dest *int) error {
	v := os.Getenv(name)
	if v == "" {
		return nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fmt.Errorf("invalid value for %s: %q is not an integer", name, v)
	}
	*dest = n
	return nil
}

// envInt64 reads an int64 from the named environment variable.
// Returns an error if the value is set but not a valid int64.
func envInt64(name string, dest *int64) error {
	v := os.Getenv(name)
	if v == "" {
		return nil
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid value for %s: %q is not an integer", name, v)
	}
	*dest = n
	return nil
}

// envFloat64 reads a float64 from the named environment variable.
// Returns an error if the value is set but not a valid float.
func envFloat64(name string, dest *float64) error {
	v := os.Getenv(name)
	if v == "" {
		return nil
	}
	n, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return fmt.Errorf("invalid value for %s: %q is not a float", name, v)
	}
	*dest = n
	return nil
}

func parseCSV(v string) []string {
	raw := strings.Split(v, ",")
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		out = append(out, item)
	}
	return out
}

// ParseRateLimitSpec parses values in the format "N/min" or "N/hour".
func ParseRateLimitSpec(spec string) (int, time.Duration, error) {
	parts := strings.Split(strings.TrimSpace(strings.ToLower(spec)), "/")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("expected format N/min or N/hour")
	}

	count, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil || count <= 0 {
		return 0, 0, fmt.Errorf("count must be a positive integer")
	}

	switch strings.TrimSpace(parts[1]) {
	case "min":
		return count, time.Minute, nil
	case "hour":
		return count, time.Hour, nil
	default:
		return 0, 0, fmt.Errorf("expected format N/min or N/hour")
	}
}

// applyEnv applies AYB_* environment variable overrides to the config. It is called by Load after parsing the TOML file and before applying CLI flags, allowing environment variables to override TOML settings.
func applyEnv(cfg *Config) error {
	if err := applyServerEnv(cfg); err != nil {
		return err
	}
	if err := applyDatabaseEnv(cfg); err != nil {
		return err
	}
	if err := applyAdminEnv(cfg); err != nil {
		return err
	}
	applyLoggingEnv(cfg)
	applyMetricsEnv(cfg)
	if err := applyRealtimeEnv(cfg); err != nil {
		return err
	}
	if err := applyTelemetryEnv(cfg); err != nil {
		return err
	}
	applyCORSOriginsEnv(cfg)
	if err := applyAuthEnv(cfg); err != nil {
		return err
	}
	if err := applyAuditEnv(cfg); err != nil {
		return err
	}
	applyDashboardAIEnv(cfg)
	applyVaultEnv(cfg)
	applyRateLimitEnv(cfg)
	if err := applyEmailEnv(cfg); err != nil {
		return err
	}
	if err := applyStorageEnv(cfg); err != nil {
		return err
	}
	if err := applyEdgeFunctionsEnv(cfg); err != nil {
		return err
	}
	if err := applyBillingEnv(cfg); err != nil {
		return err
	}
	applySupportEnv(cfg)
	if err := applyJobsEnv(cfg); err != nil {
		return err
	}
	if err := applyStatusEnv(cfg); err != nil {
		return err
	}
	applyPushEnv(cfg)
	return nil
}

// ApplyEnvironment applies AYB_* environment overrides to the config.
func (c *Config) ApplyEnvironment() error {
	return applyEnv(c)
}

// applyFlags applies CLI flag overrides to the config. Recognized flags include database-url, port, host, and tls-domain.
func applyFlags(cfg *Config, flags map[string]string) {
	if flags == nil {
		return
	}
	if v, ok := flags["database-url"]; ok && v != "" {
		cfg.Database.URL = v
	}
	if v, ok := flags["port"]; ok && v != "" {
		if port, err := strconv.Atoi(v); err == nil {
			cfg.Server.Port = port
		}
	}
	if v, ok := flags["host"]; ok && v != "" {
		cfg.Server.Host = v
	}
	if v, ok := flags["tls-domain"]; ok && v != "" {
		cfg.Server.TLSDomain = v
	}
}
