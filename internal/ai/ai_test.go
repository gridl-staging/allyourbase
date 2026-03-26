package ai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/config"
)

// --- Registry tests ---

func TestRegistryGetUnknown(t *testing.T) {
	reg := NewRegistry()
	_, err := reg.Get("nonexistent")
	if err == nil {
		t.Fatal("expected error for unknown provider")
	}
}

func TestRegistryRegisterAndGet(t *testing.T) {
	reg := NewRegistry()
	mp := &mockProvider{resp: GenerateTextResponse{Text: "ok"}}
	reg.Register("test", mp)

	p, err := reg.Get("test")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	resp, err := p.GenerateText(context.Background(), GenerateTextRequest{})
	if err != nil || resp.Text != "ok" {
		t.Errorf("unexpected response: %+v, %v", resp, err)
	}
}

func TestResolveProviderFallsBackToDefaults(t *testing.T) {
	reg := NewRegistry()
	reg.Register("openai", &mockProvider{resp: GenerateTextResponse{Text: "ok"}})

	cfg := config.AIConfig{
		DefaultProvider: "openai",
		DefaultModel:    "gpt-4o",
		Providers: map[string]config.ProviderConfig{
			"openai": {DefaultModel: "gpt-4o-mini"},
		},
	}

	// Empty args → uses config defaults.
	p, model, err := ResolveProvider(reg, "", "", cfg)
	if err != nil {
		t.Fatalf("ResolveProvider: %v", err)
	}
	if p == nil {
		t.Fatal("provider is nil")
	}
	if model != "gpt-4o-mini" {
		t.Errorf("model = %q; want %q", model, "gpt-4o-mini")
	}
}

func TestResolveProviderExplicitOverride(t *testing.T) {
	reg := NewRegistry()
	reg.Register("anthropic", &mockProvider{resp: GenerateTextResponse{Text: "ok"}})

	cfg := config.AIConfig{DefaultProvider: "openai", DefaultModel: "gpt-4o"}

	_, model, err := ResolveProvider(reg, "anthropic", "claude-sonnet-4-20250514", cfg)
	if err != nil {
		t.Fatalf("ResolveProvider: %v", err)
	}
	if model != "claude-sonnet-4-20250514" {
		t.Errorf("model = %q", model)
	}
}

// --- Vault key resolution tests ---

func TestNewProviderFromConfigVaultPrecedence(t *testing.T) {
	vault := map[string]string{"AI_OPENAI_API_KEY": "vault-key"}
	p, err := NewProviderFromConfig("openai", config.ProviderConfig{}, vault)
	if err != nil {
		t.Fatalf("NewProviderFromConfig: %v", err)
	}
	oai, ok := p.(*OpenAIProvider)
	if !ok {
		t.Fatal("expected *OpenAIProvider")
	}
	if oai.apiKey != "vault-key" {
		t.Errorf("apiKey = %q; want %q", oai.apiKey, "vault-key")
	}
}

func TestNewProviderFromConfigMissingKeyErrors(t *testing.T) {
	_, err := NewProviderFromConfig("openai", config.ProviderConfig{}, nil)
	if err == nil {
		t.Fatal("expected error for missing OpenAI key")
	}
	_, err = NewProviderFromConfig("anthropic", config.ProviderConfig{}, nil)
	if err == nil {
		t.Fatal("expected error for missing Anthropic key")
	}
}

func TestNewProviderFromConfigOllamaNoKey(t *testing.T) {
	p, err := NewProviderFromConfig("ollama", config.ProviderConfig{}, nil)
	if err != nil {
		t.Fatalf("Ollama should not require a key: %v", err)
	}
	if _, ok := p.(*OllamaProvider); !ok {
		t.Fatal("expected *OllamaProvider")
	}
}

func TestNewProviderFromConfigUnknown(t *testing.T) {
	_, err := NewProviderFromConfig("gemini", config.ProviderConfig{APIKey: "k"}, nil)
	if err == nil {
		t.Fatal("expected error for unknown provider")
	}
}

func TestBuildRegistry(t *testing.T) {
	cfg := config.AIConfig{
		Providers: map[string]config.ProviderConfig{
			"openai": {APIKey: "sk-test"},
			"ollama": {BaseURL: "http://localhost:11434"},
		},
	}
	reg, err := BuildRegistry(cfg, nil)
	if err != nil {
		t.Fatalf("BuildRegistry: %v", err)
	}
	if _, err := reg.Get("openai"); err != nil {
		t.Errorf("missing openai: %v", err)
	}
	if _, err := reg.Get("ollama"); err != nil {
		t.Errorf("missing ollama: %v", err)
	}
}

// --- OpenAI provider tests ---

func TestOpenAIProviderSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/chat/completions" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		if auth := r.Header.Get("Authorization"); auth != "Bearer test-key" {
			t.Errorf("auth = %q", auth)
		}

		body, _ := io.ReadAll(r.Body)
		var req openaiRequest
		json.Unmarshal(body, &req)
		if req.Model != "gpt-4o" {
			t.Errorf("model = %q", req.Model)
		}

		json.NewEncoder(w).Encode(map[string]any{
			"model": "gpt-4o",
			"choices": []map[string]any{
				{"message": map[string]string{"content": "Hello!"}, "finish_reason": "stop"},
			},
			"usage": map[string]int{"prompt_tokens": 10, "completion_tokens": 5},
		})
	}))
	defer srv.Close()

	p := NewOpenAIProvider("test-key", srv.URL)
	resp, err := p.GenerateText(context.Background(), GenerateTextRequest{
		Model:    "gpt-4o",
		Messages: []Message{TextMessage("user", "Hi")},
	})
	if err != nil {
		t.Fatalf("GenerateText: %v", err)
	}
	if resp.Text != "Hello!" {
		t.Errorf("Text = %q", resp.Text)
	}
	if resp.Usage.InputTokens != 10 || resp.Usage.OutputTokens != 5 {
		t.Errorf("Usage = %+v", resp.Usage)
	}
	if resp.FinishReason != "stop" {
		t.Errorf("FinishReason = %q", resp.FinishReason)
	}
}

func TestOpenAIProviderHTTPError(t *testing.T) {
	codes := []int{401, 429, 500}
	for _, code := range codes {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(code)
			w.Write([]byte(`{"error":{"message":"test error"}}`))
		}))

		p := NewOpenAIProvider("key", srv.URL)
		_, err := p.GenerateText(context.Background(), GenerateTextRequest{
			Model:    "gpt-4o",
			Messages: []Message{TextMessage("user", "Hi")},
		})
		if err == nil {
			t.Errorf("code %d: expected error", code)
		}
		pe, ok := err.(*ProviderError)
		if !ok {
			t.Errorf("code %d: expected *ProviderError, got %T", code, err)
		} else if pe.StatusCode != code {
			t.Errorf("StatusCode = %d; want %d", pe.StatusCode, code)
		}
		srv.Close()
	}
}

func TestOpenAIProviderContextCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	p := NewOpenAIProvider("key", srv.URL)
	_, err := p.GenerateText(ctx, GenerateTextRequest{
		Model:    "gpt-4o",
		Messages: []Message{TextMessage("user", "Hi")},
	})
	if err == nil {
		t.Fatal("expected context cancellation error")
	}
}

func TestOpenAIProviderSystemPrompt(t *testing.T) {
	var gotBody openaiRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		// Manually parse to check message structure
		var raw struct {
			Messages []json.RawMessage `json:"messages"`
		}
		json.Unmarshal(body, &raw)
		json.Unmarshal(body, &gotBody)

		json.NewEncoder(w).Encode(map[string]any{
			"model":   "gpt-4o",
			"choices": []map[string]any{{"message": map[string]string{"content": "ok"}, "finish_reason": "stop"}},
			"usage":   map[string]int{"prompt_tokens": 5, "completion_tokens": 2},
		})
	}))
	defer srv.Close()

	p := NewOpenAIProvider("key", srv.URL)
	p.GenerateText(context.Background(), GenerateTextRequest{
		Model:        "gpt-4o",
		SystemPrompt: "You are helpful.",
		Messages:     []Message{TextMessage("user", "Hi")},
	})

	// The system prompt should be the first message.
	if len(gotBody.Messages) < 2 {
		t.Fatalf("expected >= 2 messages, got %d", len(gotBody.Messages))
	}
}

func TextMessage(role, text string) Message {
	return Message{Role: role, Content: []ContentPart{{Type: "text", Text: text}}}
}

// --- Anthropic provider tests ---

func TestAnthropicProviderSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/messages" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if r.Header.Get("x-api-key") != "ant-key" {
			t.Errorf("x-api-key = %q", r.Header.Get("x-api-key"))
		}
		if r.Header.Get("anthropic-version") != anthropicVersion {
			t.Errorf("anthropic-version = %q", r.Header.Get("anthropic-version"))
		}

		json.NewEncoder(w).Encode(map[string]any{
			"model":       "claude-sonnet-4-20250514",
			"stop_reason": "end_turn",
			"content":     []map[string]string{{"type": "text", "text": "Response from Claude"}},
			"usage":       map[string]int{"input_tokens": 15, "output_tokens": 8},
		})
	}))
	defer srv.Close()

	p := NewAnthropicProvider("ant-key", srv.URL)
	resp, err := p.GenerateText(context.Background(), GenerateTextRequest{
		Model:    "claude-sonnet-4-20250514",
		Messages: []Message{TextMessage("user", "Hi")},
	})
	if err != nil {
		t.Fatalf("GenerateText: %v", err)
	}
	if resp.Text != "Response from Claude" {
		t.Errorf("Text = %q", resp.Text)
	}
	if resp.FinishReason != "end_turn" {
		t.Errorf("FinishReason = %q", resp.FinishReason)
	}
	if resp.Usage.InputTokens != 15 || resp.Usage.OutputTokens != 8 {
		t.Errorf("Usage = %+v", resp.Usage)
	}
}

func TestAnthropicProviderHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(429)
		w.Write([]byte(`{"error":{"message":"rate limited"}}`))
	}))
	defer srv.Close()

	p := NewAnthropicProvider("key", srv.URL)
	_, err := p.GenerateText(context.Background(), GenerateTextRequest{
		Model:    "claude-sonnet-4-20250514",
		Messages: []Message{TextMessage("user", "Hi")},
	})
	pe, ok := err.(*ProviderError)
	if !ok {
		t.Fatalf("expected *ProviderError, got %T: %v", err, err)
	}
	if pe.StatusCode != 429 {
		t.Errorf("StatusCode = %d", pe.StatusCode)
	}
}

// --- Ollama provider tests ---

func TestOllamaProviderSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			t.Errorf("path = %q", r.URL.Path)
		}
		// No auth header expected.
		if r.Header.Get("Authorization") != "" {
			t.Error("Ollama should not send Authorization")
		}

		json.NewEncoder(w).Encode(map[string]any{
			"model":             "llama3",
			"message":           map[string]string{"role": "assistant", "content": "Local response"},
			"done_reason":       "stop",
			"prompt_eval_count": 20,
			"eval_count":        12,
		})
	}))
	defer srv.Close()

	p := NewOllamaProvider(srv.URL)
	resp, err := p.GenerateText(context.Background(), GenerateTextRequest{
		Model:    "llama3",
		Messages: []Message{TextMessage("user", "Hello")},
	})
	if err != nil {
		t.Fatalf("GenerateText: %v", err)
	}
	if resp.Text != "Local response" {
		t.Errorf("Text = %q", resp.Text)
	}
	if resp.Usage.InputTokens != 20 || resp.Usage.OutputTokens != 12 {
		t.Errorf("Usage = %+v", resp.Usage)
	}
}

// --- Render tests ---

func TestRenderPromptSubstitution(t *testing.T) {
	tmpl := "Hello {{name}}, you are {{age}} years old."
	result, err := RenderPrompt(tmpl, map[string]any{"name": "Alice", "age": 30})
	if err != nil {
		t.Fatalf("RenderPrompt: %v", err)
	}
	if result != "Hello Alice, you are 30 years old." {
		t.Errorf("result = %q", result)
	}
}

func TestRenderPromptMissingVariable(t *testing.T) {
	tmpl := "Hello {{name}}, you are {{age}} years old."
	_, err := RenderPrompt(tmpl, map[string]any{"name": "Alice"})
	if err == nil {
		t.Fatal("expected error for missing variable")
	}
	if !strings.Contains(err.Error(), "age") {
		t.Errorf("error = %q; want mention of 'age'", err)
	}
}

func TestRenderPromptNoPlaceholders(t *testing.T) {
	result, err := RenderPrompt("no variables here", map[string]any{})
	if err != nil {
		t.Fatalf("RenderPrompt: %v", err)
	}
	if result != "no variables here" {
		t.Errorf("result = %q", result)
	}
}

func TestValidatePromptVariables(t *testing.T) {
	spec := []PromptVariable{
		{Name: "name", Required: true},
		{Name: "age", Required: false},
	}
	if err := ValidatePromptVariables(spec, map[string]any{"name": "Alice"}); err != nil {
		t.Errorf("expected no error: %v", err)
	}
	if err := ValidatePromptVariables(spec, map[string]any{}); err == nil {
		t.Error("expected error for missing required 'name'")
	}
}

// --- Cache tests ---

func TestPromptCacheHitAndMiss(t *testing.T) {
	cache := NewPromptCache()
	p := Prompt{Name: "greeting", Version: 1, Template: "Hello {{name}}"}

	// Miss.
	_, ok := cache.Get("greeting", 1)
	if ok {
		t.Error("expected cache miss")
	}

	// Put and hit.
	cache.Put(p)
	cached, ok := cache.Get("greeting", 1)
	if !ok {
		t.Fatal("expected cache hit")
	}
	if cached.Template != p.Template {
		t.Errorf("template = %q", cached.Template)
	}

	// Invalidate.
	cache.Invalidate("greeting", 1)
	_, ok = cache.Get("greeting", 1)
	if ok {
		t.Error("expected cache miss after invalidation")
	}
}

// --- Document parsing tests ---

func TestParseDocumentTextContent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		// Verify the request contains the document text.
		if !strings.Contains(string(body), "Document content") {
			t.Error("expected document content in request")
		}
		json.NewEncoder(w).Encode(map[string]any{
			"model":   "gpt-4o",
			"choices": []map[string]any{{"message": map[string]string{"content": `{"title":"Test","count":42}`}, "finish_reason": "stop"}},
			"usage":   map[string]int{"prompt_tokens": 5, "completion_tokens": 5},
		})
	}))
	defer srv.Close()

	reg := NewRegistry()
	reg.Register("openai", NewOpenAIProvider("key", srv.URL))

	fetcher := &mockFetcher{data: []byte("Some document text"), ct: "text/plain"}
	result, err := ParseDocument(context.Background(), ParseDocumentRequest{
		StoragePath: "test.txt",
		Provider:    "openai",
		Model:       "gpt-4o",
	}, fetcher, reg)
	if err != nil {
		t.Fatalf("ParseDocument: %v", err)
	}
	if result["title"] != "Test" {
		t.Errorf("title = %v", result["title"])
	}
}

func TestParseDocumentImageContent(t *testing.T) {
	var gotContentParts bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if strings.Contains(string(body), "image_url") {
			gotContentParts = true
		}
		json.NewEncoder(w).Encode(map[string]any{
			"model":   "gpt-4o",
			"choices": []map[string]any{{"message": map[string]string{"content": `{"label":"cat"}`}, "finish_reason": "stop"}},
			"usage":   map[string]int{"prompt_tokens": 5, "completion_tokens": 5},
		})
	}))
	defer srv.Close()

	reg := NewRegistry()
	reg.Register("openai", NewOpenAIProvider("key", srv.URL))

	fetcher := &mockFetcher{data: []byte{0xFF, 0xD8, 0xFF}, ct: "image/jpeg"}
	result, err := ParseDocument(context.Background(), ParseDocumentRequest{
		StoragePath: "photo.jpg",
		Provider:    "openai",
		Model:       "gpt-4o",
	}, fetcher, reg)
	if err != nil {
		t.Fatalf("ParseDocument: %v", err)
	}
	if !gotContentParts {
		t.Error("expected image_url content part in request")
	}
	if result["label"] != "cat" {
		t.Errorf("label = %v", result["label"])
	}
}

func TestParseDocumentSchemaValidation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return response missing required field.
		json.NewEncoder(w).Encode(map[string]any{
			"model":   "gpt-4o",
			"choices": []map[string]any{{"message": map[string]string{"content": `{"wrong_field":"value"}`}, "finish_reason": "stop"}},
			"usage":   map[string]int{"prompt_tokens": 5, "completion_tokens": 5},
		})
	}))
	defer srv.Close()

	reg := NewRegistry()
	reg.Register("openai", NewOpenAIProvider("key", srv.URL))

	fetcher := &mockFetcher{data: []byte("text"), ct: "text/plain"}
	_, err := ParseDocument(context.Background(), ParseDocumentRequest{
		StoragePath: "test.txt",
		Provider:    "openai",
		Model:       "gpt-4o",
		Schema: map[string]any{
			"required":   []any{"title"},
			"properties": map[string]any{"title": map[string]any{"type": "string"}},
		},
	}, fetcher, reg)
	if err == nil {
		t.Fatal("expected schema validation error")
	}
	if !strings.Contains(err.Error(), "missing required field") {
		t.Errorf("error = %q", err)
	}
}

func TestParseDocumentMissingSource(t *testing.T) {
	reg := NewRegistry()
	_, err := ParseDocument(context.Background(), ParseDocumentRequest{}, nil, reg)
	if err == nil {
		t.Fatal("expected error for missing source")
	}
}

func TestExtractJSONWithCodeFence(t *testing.T) {
	input := "```json\n{\"key\": \"value\"}\n```"
	result, err := extractJSON(input)
	if err != nil {
		t.Fatalf("extractJSON: %v", err)
	}
	if result["key"] != "value" {
		t.Errorf("key = %v", result["key"])
	}
}

func TestValidateJSONSchemaTypeChecks(t *testing.T) {
	schema := map[string]any{
		"properties": map[string]any{
			"name":   map[string]any{"type": "string"},
			"count":  map[string]any{"type": "number"},
			"active": map[string]any{"type": "boolean"},
		},
	}
	// Valid.
	err := validateJSONSchema(map[string]any{"name": "test", "count": 42.0, "active": true}, schema)
	if err != nil {
		t.Errorf("expected no error: %v", err)
	}
	// Wrong type.
	err = validateJSONSchema(map[string]any{"name": 123.0}, schema)
	if err == nil {
		t.Error("expected type error for name")
	}
}

// --- Additional render tests ---

func TestRenderPromptMultipleSameVariable(t *testing.T) {
	result, err := RenderPrompt("{{x}} and {{x}}", map[string]any{"x": "Y"})
	if err != nil {
		t.Fatal(err)
	}
	if result != "Y and Y" {
		t.Errorf("got %q", result)
	}
}

func TestValidatePromptVariablesDefaultFills(t *testing.T) {
	spec := []PromptVariable{
		{Name: "name", Type: "string", Required: true, Default: "World"},
	}
	if err := ValidatePromptVariables(spec, map[string]any{}); err != nil {
		t.Fatalf("required var with default should pass: %v", err)
	}
}

func TestApplyDefaults(t *testing.T) {
	spec := []PromptVariable{
		{Name: "lang", Type: "string", Default: "en"},
		{Name: "name", Type: "string"},
	}
	result := ApplyDefaults(spec, map[string]any{"name": "Alice"})
	if result["lang"] != "en" {
		t.Errorf("expected default lang=en, got %v", result["lang"])
	}
	if result["name"] != "Alice" {
		t.Errorf("expected name=Alice, got %v", result["name"])
	}
}

// --- Additional cache tests ---

func TestGetOrRenderCacheMiss(t *testing.T) {
	cache := NewPromptCache()
	store := &mockPromptStore{prompts: map[string]Prompt{
		"greeting": {Name: "greeting", Version: 1, Template: "Hello {{name}}"},
	}}

	rendered, prompt, err := cache.GetOrRender(context.Background(), "greeting", map[string]any{"name": "Alice"}, store)
	if err != nil {
		t.Fatal(err)
	}
	if rendered != "Hello Alice" {
		t.Errorf("rendered = %q", rendered)
	}
	if prompt.Name != "greeting" {
		t.Errorf("prompt.Name = %q", prompt.Name)
	}

	// Verify it's now cached.
	_, ok := cache.Get("greeting", 1)
	if !ok {
		t.Error("expected prompt to be cached after GetOrRender")
	}
}

func TestGetOrRenderCacheHit(t *testing.T) {
	cache := NewPromptCache()
	p := Prompt{Name: "greeting", Version: 1, Template: "Hi {{name}}"}
	cache.Put(p)

	store := &mockPromptStore{prompts: map[string]Prompt{
		"greeting": p,
	}}

	rendered, _, err := cache.GetOrRender(context.Background(), "greeting", map[string]any{"name": "Bob"}, store)
	if err != nil {
		t.Fatal(err)
	}
	if rendered != "Hi Bob" {
		t.Errorf("rendered = %q", rendered)
	}
}

func TestGetOrRenderMissingPrompt(t *testing.T) {
	cache := NewPromptCache()
	store := &mockPromptStore{prompts: map[string]Prompt{}}

	_, _, err := cache.GetOrRender(context.Background(), "nonexistent", nil, store)
	if err == nil {
		t.Fatal("expected error for missing prompt")
	}
}

// --- Additional docparse tests ---

func TestParseDocumentFromURL(t *testing.T) {
	docSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte("Invoice #123"))
	}))
	defer docSrv.Close()

	aiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"model":   "gpt-4o",
			"choices": []map[string]any{{"message": map[string]string{"content": `{"invoice_number":"123"}`}, "finish_reason": "stop"}},
			"usage":   map[string]int{"prompt_tokens": 10, "completion_tokens": 5},
		})
	}))
	defer aiSrv.Close()

	reg := NewRegistry()
	reg.Register("openai", NewOpenAIProvider("k", aiSrv.URL))

	result, err := ParseDocument(context.Background(), ParseDocumentRequest{
		URL:      docSrv.URL + "/invoice.txt",
		Provider: "openai",
		Model:    "gpt-4o",
	}, nil, reg)
	if err != nil {
		t.Fatal(err)
	}
	if result["invoice_number"] != "123" {
		t.Errorf("invoice_number: got %v", result["invoice_number"])
	}
}

func TestExtractJSONPlain(t *testing.T) {
	result, err := extractJSON(`{"a": 1}`)
	if err != nil {
		t.Fatal(err)
	}
	if result["a"] != float64(1) {
		t.Errorf("a: got %v", result["a"])
	}
}
