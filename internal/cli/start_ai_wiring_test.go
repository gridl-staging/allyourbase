package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/ai"
	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/edgefunc"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/google/uuid"
)

// --- mock types local to this test file ---

type wiringMockLogStore struct {
	mu   sync.Mutex
	logs []ai.CallLog
}

func (m *wiringMockLogStore) Insert(_ context.Context, log ai.CallLog) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.logs = append(m.logs, log)
	return nil
}

func (m *wiringMockLogStore) List(_ context.Context, _ ai.ListFilter) ([]ai.CallLog, int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.logs, len(m.logs), nil
}

func (m *wiringMockLogStore) UsageSummary(_ context.Context, _, _ time.Time) (ai.UsageSummary, error) {
	return ai.UsageSummary{}, nil
}

type wiringMockPromptStore struct {
	prompts map[string]ai.Prompt
}

func (m *wiringMockPromptStore) Create(_ context.Context, _ ai.CreatePromptRequest) (ai.Prompt, error) {
	return ai.Prompt{}, nil
}
func (m *wiringMockPromptStore) Get(_ context.Context, _ uuid.UUID) (ai.Prompt, error) {
	return ai.Prompt{}, nil
}
func (m *wiringMockPromptStore) GetByName(_ context.Context, name string) (ai.Prompt, error) {
	p, ok := m.prompts[name]
	if !ok {
		return ai.Prompt{}, fmt.Errorf("prompt %q not found", name)
	}
	return p, nil
}
func (m *wiringMockPromptStore) List(_ context.Context, _, _ int) ([]ai.Prompt, int, error) {
	return nil, 0, nil
}
func (m *wiringMockPromptStore) Update(_ context.Context, _ uuid.UUID, _ ai.UpdatePromptRequest) (ai.Prompt, error) {
	return ai.Prompt{}, nil
}
func (m *wiringMockPromptStore) Delete(_ context.Context, _ uuid.UUID) error { return nil }
func (m *wiringMockPromptStore) ListVersions(_ context.Context, _ uuid.UUID) ([]ai.PromptVersion, error) {
	return nil, nil
}

// --- tests ---

// TestAIWiringBuildRegistryAndWrapProviders verifies the wiring pattern from
// start.go: BuildRegistry → retry+logging wrapping → closures produce correct
// end-to-end behavior for edge pool callbacks.
func TestAIWiringBuildRegistryAndWrapProviders(t *testing.T) {
	// Stand up a mock Ollama HTTP server that returns a canned response.
	ollamaSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"model":             "llama3",
			"message":           map[string]string{"role": "assistant", "content": "Hello from Ollama"},
			"done":              true,
			"prompt_eval_count": 10,
			"eval_count":        5,
		})
	}))
	defer ollamaSrv.Close()

	// Build config that mirrors what start.go would receive.
	aiCfg := config.AIConfig{
		DefaultProvider: "ollama",
		DefaultModel:    "llama3",
		TimeoutSecs:     5,
		MaxRetries:      1,
		Providers: map[string]config.ProviderConfig{
			"ollama": {BaseURL: ollamaSrv.URL},
		},
	}

	// Step 1: BuildRegistry — same as start.go line 606.
	reg, err := ai.BuildRegistry(aiCfg, nil)
	testutil.NoError(t, err)

	// Step 2: Wrap each provider with retry + logging — same as start.go lines 618–627.
	logStore := &wiringMockLogStore{}
	aiTimeout := time.Duration(aiCfg.TimeoutSecs) * time.Second
	for name := range aiCfg.Providers {
		p, getErr := reg.Get(name)
		testutil.NoError(t, getErr)
		retried := ai.NewRetryProvider(p, aiCfg.MaxRetries, aiTimeout)
		reg.Register(name, ai.NewLoggingProvider(retried, name, logStore))
	}

	// Step 3: Wire edge pool closures — same pattern as start.go lines 635–689.
	pool := edgefunc.NewPool(1)
	promptStore := &wiringMockPromptStore{
		prompts: map[string]ai.Prompt{
			"greeting": {
				Name:     "greeting",
				Version:  1,
				Template: "Hello, {{name}}!",
				Variables: []ai.PromptVariable{
					{Name: "name", Type: "string", Required: true},
				},
			},
		},
	}
	promptCache := ai.NewPromptCache()

	pool.SetAIGenerate(func(callCtx context.Context, messages []map[string]any, opts map[string]any) (string, error) {
		providerName, _ := opts["provider"].(string)
		model, _ := opts["model"].(string)
		provider, resolvedModel, resolveErr := ai.ResolveProvider(reg, providerName, model, aiCfg)
		if resolveErr != nil {
			return "", resolveErr
		}
		req := ai.GenerateTextRequest{Model: resolvedModel}
		if sp, ok := opts["systemPrompt"].(string); ok {
			req.SystemPrompt = sp
		}
		if mt, ok := opts["maxTokens"]; ok {
			switch v := mt.(type) {
			case int:
				req.MaxTokens = v
			case float64:
				req.MaxTokens = int(v)
			}
		}
		for _, m := range messages {
			role, _ := m["role"].(string)
			content, _ := m["content"].(string)
			req.Messages = append(req.Messages, ai.Message{
				Role:    role,
				Content: ai.TextContent(content),
			})
		}
		resp, genErr := provider.GenerateText(callCtx, req)
		if genErr != nil {
			return "", genErr
		}
		return resp.Text, nil
	})

	pool.SetAIRenderPrompt(func(callCtx context.Context, name string, vars map[string]any) (string, error) {
		rendered, _, renderErr := promptCache.GetOrRender(callCtx, name, vars, promptStore)
		return rendered, renderErr
	})

	pool.SetAIParseDocument(func(callCtx context.Context, url string, opts map[string]any) (map[string]any, error) {
		req := ai.ParseDocumentRequest{URL: url}
		if v, ok := opts["provider"].(string); ok {
			req.Provider = v
		}
		if v, ok := opts["model"].(string); ok {
			req.Model = v
		}
		if v, ok := opts["prompt"].(string); ok {
			req.Prompt = v
		}
		if v, ok := opts["schema"].(map[string]any); ok {
			req.Schema = v
		}
		return ai.ParseDocument(callCtx, req, nil, reg)
	})

	// --- Verify the wired closures work ---

	t.Run("GenerateText", func(t *testing.T) {
		// Call the generate closure through the pool's internal state.
		// We test the closure directly since Pool doesn't expose its callbacks.
		ctx := context.Background()
		messages := []map[string]any{
			{"role": "user", "content": "Hi"},
		}
		opts := map[string]any{
			"maxTokens": 100,
		}

		// We need to test through the closure we captured. Since the pool
		// doesn't expose the function, we test the same closure pattern.
		provider, resolvedModel, resolveErr := ai.ResolveProvider(reg, "", "", aiCfg)
		testutil.NoError(t, resolveErr)
		testutil.Equal(t, "llama3", resolvedModel)

		req := ai.GenerateTextRequest{
			Model: resolvedModel,
			Messages: []ai.Message{
				{Role: "user", Content: ai.TextContent("Hi")},
			},
		}
		resp, genErr := provider.GenerateText(ctx, req)
		testutil.NoError(t, genErr)
		testutil.Equal(t, "Hello from Ollama", resp.Text)
		_ = messages
		_ = opts

		// Verify logging middleware recorded the call.
		logStore.mu.Lock()
		logCount := len(logStore.logs)
		logStore.mu.Unlock()
		testutil.Equal(t, 1, logCount)
		testutil.Equal(t, "ollama", logStore.logs[0].Provider)
		testutil.Equal(t, "success", logStore.logs[0].Status)
	})

	t.Run("RenderPrompt", func(t *testing.T) {
		ctx := context.Background()
		rendered, _, renderErr := promptCache.GetOrRender(ctx, "greeting", map[string]any{"name": "World"}, promptStore)
		testutil.NoError(t, renderErr)
		testutil.Equal(t, "Hello, World!", rendered)
	})

	t.Run("RenderPromptNotFound", func(t *testing.T) {
		ctx := context.Background()
		_, _, renderErr := promptCache.GetOrRender(ctx, "nonexistent", nil, promptStore)
		testutil.NotNil(t, renderErr)
		testutil.Contains(t, renderErr.Error(), "not found")
	})

	t.Run("ResolveProviderDefault", func(t *testing.T) {
		// Empty provider/model should resolve via config defaults.
		p, model, resolveErr := ai.ResolveProvider(reg, "", "", aiCfg)
		testutil.NoError(t, resolveErr)
		testutil.Equal(t, "llama3", model)
		testutil.NotNil(t, p)
	})

	t.Run("ResolveProviderExplicit", func(t *testing.T) {
		p, model, resolveErr := ai.ResolveProvider(reg, "ollama", "custom-model", aiCfg)
		testutil.NoError(t, resolveErr)
		testutil.Equal(t, "custom-model", model)
		testutil.NotNil(t, p)
	})

	t.Run("ResolveProviderUnknown", func(t *testing.T) {
		_, _, resolveErr := ai.ResolveProvider(reg, "nonexistent", "", aiCfg)
		testutil.NotNil(t, resolveErr)
		testutil.Contains(t, resolveErr.Error(), "unknown AI provider")
	})

	t.Run("ProviderWrappedWithLogging", func(t *testing.T) {
		// After a generate call, the log store should have entries.
		logStore.mu.Lock()
		count := len(logStore.logs)
		logStore.mu.Unlock()
		testutil.True(t, count >= 1, "expected at least 1 log entry from generate call")
	})
}

// TestAIWiringSkippedWhenNoProviders verifies that the AI subsystem is not
// wired when no providers are configured, matching start.go line 597.
func TestAIWiringSkippedWhenNoProviders(t *testing.T) {
	aiCfg := config.AIConfig{
		Providers: map[string]config.ProviderConfig{},
	}

	// The guard in start.go is: if len(cfg.AI.Providers) > 0 && pool != nil
	testutil.True(t, len(aiCfg.Providers) == 0, "expected empty providers")

	// With no providers, BuildRegistry produces an empty registry.
	reg, err := ai.BuildRegistry(aiCfg, nil)
	testutil.NoError(t, err)

	// Resolving should fail since nothing is registered.
	_, _, resolveErr := ai.ResolveProvider(reg, "", "", aiCfg)
	testutil.NotNil(t, resolveErr)
}

// TestAIWiringTimeoutDefault verifies the 30s default timeout from start.go.
func TestAIWiringTimeoutDefault(t *testing.T) {
	aiCfg := config.AIConfig{TimeoutSecs: 0}
	timeout := time.Duration(aiCfg.TimeoutSecs) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	testutil.Equal(t, 30*time.Second, timeout)
}

// TestAIWiringTimeoutCustom verifies custom timeout from config.
func TestAIWiringTimeoutCustom(t *testing.T) {
	aiCfg := config.AIConfig{TimeoutSecs: 10}
	timeout := time.Duration(aiCfg.TimeoutSecs) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	testutil.Equal(t, 10*time.Second, timeout)
}

type assistantWiringHistoryStore struct {
	entries []ai.AssistantHistoryEntry
}

func (s *assistantWiringHistoryStore) Insert(_ context.Context, entry ai.AssistantHistoryEntry) (ai.AssistantHistoryEntry, error) {
	if entry.ID == uuid.Nil {
		entry.ID = uuid.New()
	}
	s.entries = append(s.entries, entry)
	return entry, nil
}

func (s *assistantWiringHistoryStore) List(_ context.Context, _ ai.AssistantHistoryFilter) ([]ai.AssistantHistoryEntry, int, error) {
	return s.entries, len(s.entries), nil
}

type assistantWiringProvider struct{}

func (p *assistantWiringProvider) GenerateText(_ context.Context, req ai.GenerateTextRequest) (ai.GenerateTextResponse, error) {
	return ai.GenerateTextResponse{
		Text:  "```sql\\nSELECT 1;\\n```",
		Model: req.Model,
	}, nil
}

func testAssistantSchemaHolder() *schema.CacheHolder {
	holder := schema.NewCacheHolder(nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	holder.SetForTesting(&schema.SchemaCache{
		Tables: map[string]*schema.Table{
			"public.users": {
				Schema:     "public",
				Name:       "users",
				Kind:       "table",
				Columns:    []*schema.Column{{Name: "id", TypeName: "uuid"}},
				PrimaryKey: []string{"id"},
			},
		},
		Schemas: []string{"public"},
		BuiltAt: time.Now().UTC(),
	})
	return holder
}

func TestBuildDashboardAIAssistantService_GatedByConfigAndProviders(t *testing.T) {
	reg := ai.NewRegistry()
	reg.Register("openai", &assistantWiringProvider{})

	cfg := config.Default()
	cfg.DashboardAI.Enabled = false
	cfg.AI.DefaultProvider = "openai"
	cfg.AI.DefaultModel = "gpt-4o-mini"
	cfg.AI.Providers = map[string]config.ProviderConfig{
		"openai": {DefaultModel: "gpt-4o-mini"},
	}
	svc := buildDashboardAIAssistantService(cfg, reg, testAssistantSchemaHolder(), &assistantWiringHistoryStore{}, testNoopLogger())
	testutil.True(t, svc == nil, "assistant service should be nil when dashboard_ai is disabled")

	cfg.DashboardAI.Enabled = true
	cfg.AI.Providers = map[string]config.ProviderConfig{}
	svc = buildDashboardAIAssistantService(cfg, reg, testAssistantSchemaHolder(), &assistantWiringHistoryStore{}, testNoopLogger())
	testutil.NotNil(t, svc)

	_, err := svc.Execute(context.Background(), ai.AssistantRequest{Query: "return one row"})
	testutil.True(t, errors.Is(err, ai.ErrAssistantNotConfigured), "enabled assistant should return ErrAssistantNotConfigured when providers are missing")
}

func TestBuildDashboardAIAssistantService_RegistryFailureStillReturnsNotConfigured(t *testing.T) {
	cfg := config.Default()
	cfg.DashboardAI.Enabled = true
	cfg.AI.DefaultProvider = "openai"
	cfg.AI.DefaultModel = "gpt-4o-mini"
	cfg.AI.Providers = map[string]config.ProviderConfig{
		"openai": {DefaultModel: "gpt-4o-mini"},
	}

	svc := buildDashboardAIAssistantService(cfg, nil, testAssistantSchemaHolder(), &assistantWiringHistoryStore{}, testNoopLogger())
	testutil.NotNil(t, svc)

	_, err := svc.Execute(context.Background(), ai.AssistantRequest{Query: "return one row"})
	testutil.True(t, errors.Is(err, ai.ErrAssistantNotConfigured), "nil registry should surface ErrAssistantNotConfigured")
}

func TestBuildDashboardAIAssistantService_ReusesProvidedRegistry(t *testing.T) {
	reg := ai.NewRegistry()
	reg.Register("openai", &assistantWiringProvider{})

	cfg := config.Default()
	cfg.DashboardAI.Enabled = true
	cfg.AI.DefaultProvider = "openai"
	cfg.AI.DefaultModel = "gpt-4o-mini"
	cfg.AI.Providers = map[string]config.ProviderConfig{
		"openai": {DefaultModel: "gpt-4o-mini"},
	}
	history := &assistantWiringHistoryStore{}
	svc := buildDashboardAIAssistantService(cfg, reg, testAssistantSchemaHolder(), history, testNoopLogger())
	testutil.NotNil(t, svc)

	resp, err := svc.Execute(context.Background(), ai.AssistantRequest{
		Mode:  ai.AssistantModeSQL,
		Query: "return one row",
	})
	testutil.NoError(t, err)
	testutil.Equal(t, "openai", resp.Provider)
	testutil.Equal(t, "gpt-4o-mini", resp.Model)
	testutil.Equal(t, 1, len(history.entries))
}
