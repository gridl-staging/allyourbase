package config

import "testing"

func TestAIConfigDefaults(t *testing.T) {
	cfg := Default()

	if cfg.AI.DefaultProvider != "openai" {
		t.Errorf("DefaultProvider: got %q, want %q", cfg.AI.DefaultProvider, "openai")
	}
	if cfg.AI.TimeoutSecs != 30 {
		t.Errorf("TimeoutSecs: got %d, want 30", cfg.AI.TimeoutSecs)
	}
	if cfg.AI.MaxRetries != 2 {
		t.Errorf("MaxRetries: got %d, want 2", cfg.AI.MaxRetries)
	}
	if cfg.AI.Providers == nil {
		t.Error("Providers map should be initialized")
	}
}

func TestAIConfigTOMLRoundtrip(t *testing.T) {
	tomlData := []byte(`
[ai]
default_provider = "anthropic"
default_model    = "claude-opus-4-6"
max_retries      = 3
timeout          = 60

[ai.providers.openai]
api_key       = "sk-test"
base_url      = "https://api.openai.com"
default_model = "gpt-4o"

[ai.providers.anthropic]
api_key       = "ant-test"
default_model = "claude-sonnet-4-6"
`)

	cfg, err := ParseTOML(tomlData)
	if err != nil {
		t.Fatalf("ParseTOML: %v", err)
	}

	if cfg.AI.DefaultProvider != "anthropic" {
		t.Errorf("DefaultProvider: got %q, want %q", cfg.AI.DefaultProvider, "anthropic")
	}
	if cfg.AI.DefaultModel != "claude-opus-4-6" {
		t.Errorf("DefaultModel: got %q", cfg.AI.DefaultModel)
	}
	if cfg.AI.MaxRetries != 3 {
		t.Errorf("MaxRetries: got %d, want 3", cfg.AI.MaxRetries)
	}
	if cfg.AI.TimeoutSecs != 60 {
		t.Errorf("TimeoutSecs: got %d, want 60", cfg.AI.TimeoutSecs)
	}

	openai, ok := cfg.AI.Providers["openai"]
	if !ok {
		t.Fatal("expected openai provider")
	}
	if openai.APIKey != "sk-test" {
		t.Errorf("openai api_key: got %q", openai.APIKey)
	}
	if openai.BaseURL != "https://api.openai.com" {
		t.Errorf("openai base_url: got %q", openai.BaseURL)
	}
	if openai.DefaultModel != "gpt-4o" {
		t.Errorf("openai default_model: got %q", openai.DefaultModel)
	}

	anthropic, ok := cfg.AI.Providers["anthropic"]
	if !ok {
		t.Fatal("expected anthropic provider")
	}
	if anthropic.APIKey != "ant-test" {
		t.Errorf("anthropic api_key: got %q", anthropic.APIKey)
	}
}

func TestAIConfigDefaultsAppliedWhenSectionOmitted(t *testing.T) {
	tomlData := []byte(`
[server]
port = 8090

[database]
url = "postgres://localhost/test"
max_conns = 5
`)
	cfg, err := ParseTOML(tomlData)
	if err != nil {
		t.Fatalf("ParseTOML: %v", err)
	}

	if cfg.AI.DefaultProvider != "openai" {
		t.Errorf("DefaultProvider: got %q, want %q", cfg.AI.DefaultProvider, "openai")
	}
	if cfg.AI.TimeoutSecs != 30 {
		t.Errorf("TimeoutSecs: got %d, want 30", cfg.AI.TimeoutSecs)
	}
	if cfg.AI.MaxRetries != 2 {
		t.Errorf("MaxRetries: got %d, want 2", cfg.AI.MaxRetries)
	}
}

func TestAIConfigMultipleProviders(t *testing.T) {
	tomlData := []byte(`
[ai.providers.openai]
api_key = "sk-1"

[ai.providers.anthropic]
api_key = "ant-1"

[ai.providers.ollama]
base_url = "http://localhost:11434"
`)

	cfg, err := ParseTOML(tomlData)
	if err != nil {
		t.Fatalf("ParseTOML: %v", err)
	}

	if len(cfg.AI.Providers) != 3 {
		t.Errorf("expected 3 providers, got %d", len(cfg.AI.Providers))
	}
	if cfg.AI.Providers["openai"].APIKey != "sk-1" {
		t.Error("openai api_key mismatch")
	}
	if cfg.AI.Providers["anthropic"].APIKey != "ant-1" {
		t.Error("anthropic api_key mismatch")
	}
	if cfg.AI.Providers["ollama"].BaseURL != "http://localhost:11434" {
		t.Error("ollama base_url mismatch")
	}
}

func TestAIConfigBreakerDefaults(t *testing.T) {
	cfg := Default()
	if cfg.AI.Breaker.FailureThreshold != 5 {
		t.Fatalf("FailureThreshold = %d; want 5", cfg.AI.Breaker.FailureThreshold)
	}
	if cfg.AI.Breaker.OpenSeconds != 30 {
		t.Fatalf("OpenSeconds = %d; want 30", cfg.AI.Breaker.OpenSeconds)
	}
	if cfg.AI.Breaker.HalfOpenProbeLimit != 1 {
		t.Fatalf("HalfOpenProbeLimit = %d; want 1", cfg.AI.Breaker.HalfOpenProbeLimit)
	}
}

func TestAIConfigInvalidBreakerSettings(t *testing.T) {
	cfg := Default()
	cfg.AI.Breaker.FailureThreshold = 0
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error for ai.breaker.failure_threshold")
	}

	cfg = Default()
	cfg.AI.Breaker.OpenSeconds = 0
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error for ai.breaker.open_seconds")
	}

	cfg = Default()
	cfg.AI.Breaker.HalfOpenProbeLimit = 0
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error for ai.breaker.half_open_probe_limit")
	}
}

func TestAIConfigEmbeddingDimensionsValidation(t *testing.T) {
	cfg := Default()
	cfg.AI.EmbeddingDimensions = map[string]int{
		"openai:text-embedding-3-small": 1536,
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected valid config, got: %v", err)
	}

	cfg.AI.EmbeddingDimensions["openai:text-embedding-3-large"] = 0
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected invalid dimension error")
	}
}

func TestAIConfigEmbeddingDimensionsCaseConflict(t *testing.T) {
	cfg := Default()
	cfg.AI.EmbeddingDimensions = map[string]int{
		"OpenAI:text-embedding-3-small": 1536,
		"openai:text-embedding-3-small": 1536,
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected case-insensitive duplicate key error")
	}
}
