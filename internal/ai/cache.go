package ai

import (
	"context"
	"fmt"
	"sync"
)

// PromptCache caches rendered prompts by name:version.
type PromptCache struct {
	mu    sync.RWMutex
	store map[string]cachedPrompt
}

type cachedPrompt struct {
	prompt Prompt
}

// NewPromptCache creates an empty prompt cache.
func NewPromptCache() *PromptCache {
	return &PromptCache{store: make(map[string]cachedPrompt)}
}

func cacheKey(name string, version int) string {
	return fmt.Sprintf("%s:%d", name, version)
}

// Get returns a cached prompt if present.
func (c *PromptCache) Get(name string, version int) (Prompt, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	entry, ok := c.store[cacheKey(name, version)]
	if !ok {
		return Prompt{}, false
	}
	return entry.prompt, true
}

// Put stores a prompt in the cache.
func (c *PromptCache) Put(p Prompt) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.store[cacheKey(p.Name, p.Version)] = cachedPrompt{prompt: p}
}

// Invalidate removes a cached prompt entry.
func (c *PromptCache) Invalidate(name string, version int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.store, cacheKey(name, version))
}

// GetOrRender fetches a prompt by name (cache first, then store), renders it with variables.
func (c *PromptCache) GetOrRender(ctx context.Context, name string, vars map[string]any, store PromptStore) (string, Prompt, error) {
	// Try store to get latest version.
	prompt, err := store.GetByName(ctx, name)
	if err != nil {
		return "", Prompt{}, fmt.Errorf("loading prompt %q: %w", name, err)
	}

	// Check cache for this specific version.
	if cached, ok := c.Get(name, prompt.Version); ok {
		rendered, err := RenderPrompt(cached.Template, vars)
		if err != nil {
			return "", cached, err
		}
		return rendered, cached, nil
	}

	// Cache miss — render and cache.
	c.Put(prompt)

	rendered, err := RenderPrompt(prompt.Template, vars)
	if err != nil {
		return "", prompt, err
	}
	return rendered, prompt, nil
}
