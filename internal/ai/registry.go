package ai

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/allyourbase/ayb/internal/config"
)

// Registry maps provider names to Provider instances.
type Registry struct {
	mu        sync.RWMutex
	providers map[string]Provider
}

// NewRegistry creates an empty provider registry.
func NewRegistry() *Registry {
	return &Registry{providers: make(map[string]Provider)}
}

// Register adds a named provider to the registry.
func (r *Registry) Register(name string, p Provider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[name] = p
}

// Get returns a provider by name or an error if not found.
func (r *Registry) Get(name string) (Provider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.providers[name]
	if !ok {
		return nil, fmt.Errorf("unknown AI provider %q", name)
	}
	return p, nil
}

// ResolveProvider resolves a provider and model from explicit args or config defaults.
// If providerName is empty, the config default is used.
// If model is empty, the provider-specific or config-level default is used.
func ResolveProvider(reg *Registry, providerName, model string, cfg config.AIConfig) (Provider, string, error) {
	if providerName == "" {
		providerName = cfg.DefaultProvider
	}
	if providerName == "" {
		return nil, "", fmt.Errorf("no AI provider specified and no default configured")
	}

	p, err := reg.Get(providerName)
	if err != nil {
		return nil, "", err
	}

	if model == "" {
		if pc, ok := cfg.Providers[providerName]; ok && pc.DefaultModel != "" {
			model = pc.DefaultModel
		} else {
			model = cfg.DefaultModel
		}
	}
	if model == "" {
		return nil, "", fmt.Errorf("no model specified and no default configured for provider %q", providerName)
	}

	return p, model, nil
}

// NewProviderFromConfig creates a Provider from its config entry.
// API key resolution order: config → vault secret (AI_{NAME}_API_KEY) → env ({NAME}_API_KEY).
func NewProviderFromConfig(name string, pcfg config.ProviderConfig, vaultSecrets map[string]string) (Provider, error) {
	apiKey := pcfg.APIKey
	upperName := strings.ToUpper(name)

	if apiKey == "" && vaultSecrets != nil {
		apiKey = vaultSecrets["AI_"+upperName+"_API_KEY"]
	}
	if apiKey == "" {
		apiKey = os.Getenv(upperName + "_API_KEY")
	}

	switch name {
	case "openai":
		if apiKey == "" {
			return nil, fmt.Errorf("openai provider requires an API key (set ai.providers.openai.api_key, vault secret AI_OPENAI_API_KEY, or env OPENAI_API_KEY)")
		}
		return NewOpenAIProvider(apiKey, pcfg.BaseURL), nil
	case "anthropic":
		if apiKey == "" {
			return nil, fmt.Errorf("anthropic provider requires an API key (set ai.providers.anthropic.api_key, vault secret AI_ANTHROPIC_API_KEY, or env ANTHROPIC_API_KEY)")
		}
		return NewAnthropicProvider(apiKey, pcfg.BaseURL), nil
	case "ollama":
		return NewOllamaProvider(pcfg.BaseURL), nil
	default:
		return nil, fmt.Errorf("unknown AI provider type %q", name)
	}
}

// BuildRegistry creates a fully wired Registry from config and vault secrets.
func BuildRegistry(cfg config.AIConfig, vaultSecrets map[string]string) (*Registry, error) {
	reg := NewRegistry()
	for name, pcfg := range cfg.Providers {
		p, err := NewProviderFromConfig(name, pcfg, vaultSecrets)
		if err != nil {
			return nil, fmt.Errorf("configuring AI provider %q: %w", name, err)
		}
		reg.Register(name, p)
	}
	return reg, nil
}
