package config

import (
	"strings"
	"testing"
)

func TestDashboardAIConfigDefaults(t *testing.T) {
	cfg := Default()
	if cfg.DashboardAI.Enabled {
		t.Fatalf("DashboardAI.Enabled = true; want false")
	}
	if cfg.DashboardAI.RateLimit != "20/min" {
		t.Fatalf("DashboardAI.RateLimit = %q; want 20/min", cfg.DashboardAI.RateLimit)
	}
}

func TestDashboardAIConfigTOMLRoundTrip(t *testing.T) {
	tomlData := []byte(`
[dashboard_ai]
enabled = true
rate_limit = "9/hour"

[ai]
default_provider = "openai"
default_model = "gpt-4o"

[ai.providers.openai]
api_key = "sk-test"
default_model = "gpt-4o-mini"
`)

	cfg, err := ParseTOML(tomlData)
	if err != nil {
		t.Fatalf("ParseTOML: %v", err)
	}
	if !cfg.DashboardAI.Enabled {
		t.Fatalf("DashboardAI.Enabled = false; want true")
	}
	if cfg.DashboardAI.RateLimit != "9/hour" {
		t.Fatalf("DashboardAI.RateLimit = %q; want 9/hour", cfg.DashboardAI.RateLimit)
	}
	if cfg.AI.DefaultProvider != "openai" {
		t.Fatalf("AI.DefaultProvider = %q; want openai", cfg.AI.DefaultProvider)
	}
}

func TestDashboardAIConfigEnvOverride(t *testing.T) {
	t.Setenv("AYB_DASHBOARD_AI_ENABLED", "1")
	t.Setenv("AYB_DASHBOARD_AI_RATE_LIMIT", "7/hour")
	cfg := Default()
	if err := cfg.ApplyEnvironment(); err != nil {
		t.Fatalf("ApplyEnvironment: %v", err)
	}
	if !cfg.DashboardAI.Enabled {
		t.Fatalf("DashboardAI.Enabled = false; want true")
	}
	if cfg.DashboardAI.RateLimit != "7/hour" {
		t.Fatalf("DashboardAI.RateLimit = %q; want 7/hour", cfg.DashboardAI.RateLimit)
	}
}

func TestDashboardAIConfigSectionOmittedBackCompat(t *testing.T) {
	cfg, err := ParseTOML([]byte(`
[server]
port = 8090
`))
	if err != nil {
		t.Fatalf("ParseTOML: %v", err)
	}
	if cfg.DashboardAI.Enabled {
		t.Fatalf("DashboardAI.Enabled = true; want false")
	}
	if cfg.DashboardAI.RateLimit != "20/min" {
		t.Fatalf("DashboardAI.RateLimit = %q; want 20/min", cfg.DashboardAI.RateLimit)
	}
}

func TestDashboardAIConfigRejectsInvalidRateLimit(t *testing.T) {
	_, err := ParseTOML([]byte(`
[dashboard_ai]
rate_limit = "later"
`))
	if err == nil {
		t.Fatal("expected invalid dashboard_ai.rate_limit error")
	}
	if !strings.Contains(err.Error(), "dashboard_ai.rate_limit") {
		t.Fatalf("err = %v; want dashboard_ai.rate_limit context", err)
	}
}
