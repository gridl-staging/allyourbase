package ai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestOpenAIEmbeddingSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/embeddings" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		if auth := r.Header.Get("Authorization"); auth != "Bearer test-key" {
			t.Errorf("auth = %q", auth)
		}

		body, _ := io.ReadAll(r.Body)
		var req struct {
			Model string   `json:"model"`
			Input []string `json:"input"`
		}
		json.Unmarshal(body, &req)
		if req.Model != "text-embedding-3-small" {
			t.Errorf("model = %q", req.Model)
		}
		if len(req.Input) != 1 || req.Input[0] != "hello" {
			t.Errorf("input = %v", req.Input)
		}

		json.NewEncoder(w).Encode(map[string]any{
			"model": "text-embedding-3-small",
			"data": []map[string]any{
				{"embedding": []float64{0.1, 0.2, 0.3}},
			},
			"usage": map[string]int{"prompt_tokens": 2, "total_tokens": 2},
		})
	}))
	defer srv.Close()

	p := NewOpenAIProvider("test-key", srv.URL)
	resp, err := p.GenerateEmbedding(context.Background(), EmbeddingRequest{
		Model: "text-embedding-3-small",
		Input: []string{"hello"},
	})
	if err != nil {
		t.Fatalf("GenerateEmbedding: %v", err)
	}
	if len(resp.Embeddings) != 1 {
		t.Fatalf("embeddings count = %d; want 1", len(resp.Embeddings))
	}
	if len(resp.Embeddings[0]) != 3 {
		t.Errorf("embedding dim = %d; want 3", len(resp.Embeddings[0]))
	}
	if resp.Usage.InputTokens != 2 {
		t.Errorf("InputTokens = %d; want 2", resp.Usage.InputTokens)
	}
}

func TestOpenAIEmbeddingBatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"model": "text-embedding-3-small",
			"data": []map[string]any{
				{"embedding": []float64{0.1, 0.2}},
				{"embedding": []float64{0.3, 0.4}},
			},
			"usage": map[string]int{"prompt_tokens": 4, "total_tokens": 4},
		})
	}))
	defer srv.Close()

	p := NewOpenAIProvider("key", srv.URL)
	resp, err := p.GenerateEmbedding(context.Background(), EmbeddingRequest{
		Model: "text-embedding-3-small",
		Input: []string{"hello", "world"},
	})
	if err != nil {
		t.Fatalf("GenerateEmbedding: %v", err)
	}
	if len(resp.Embeddings) != 2 {
		t.Fatalf("embeddings count = %d; want 2", len(resp.Embeddings))
	}
}

func TestOpenAIEmbedding401(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		w.Write([]byte(`{"error":{"message":"invalid api key"}}`))
	}))
	defer srv.Close()

	p := NewOpenAIProvider("bad-key", srv.URL)
	_, err := p.GenerateEmbedding(context.Background(), EmbeddingRequest{
		Model: "text-embedding-3-small",
		Input: []string{"hello"},
	})
	pe, ok := err.(*ProviderError)
	if !ok {
		t.Fatalf("expected *ProviderError, got %T: %v", err, err)
	}
	if pe.StatusCode != 401 {
		t.Errorf("StatusCode = %d; want 401", pe.StatusCode)
	}
	if pe.IsRetryable() {
		t.Error("401 should not be retryable")
	}
}

func TestOpenAIEmbedding429(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(429)
		w.Write([]byte(`{"error":{"message":"rate limited"}}`))
	}))
	defer srv.Close()

	p := NewOpenAIProvider("key", srv.URL)
	_, err := p.GenerateEmbedding(context.Background(), EmbeddingRequest{
		Model: "text-embedding-3-small",
		Input: []string{"hello"},
	})
	pe, ok := err.(*ProviderError)
	if !ok {
		t.Fatalf("expected *ProviderError, got %T: %v", err, err)
	}
	if !pe.IsRetryable() {
		t.Error("429 should be retryable")
	}
}

func TestOpenAIEmbeddingMalformedResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`not json`))
	}))
	defer srv.Close()

	p := NewOpenAIProvider("key", srv.URL)
	_, err := p.GenerateEmbedding(context.Background(), EmbeddingRequest{
		Model: "text-embedding-3-small",
		Input: []string{"hello"},
	})
	if err == nil {
		t.Fatal("expected error for malformed response")
	}
}

func TestOllamaEmbeddingSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/api/embed" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}

		body, _ := io.ReadAll(r.Body)
		var req struct {
			Model string   `json:"model"`
			Input []string `json:"input"`
		}
		json.Unmarshal(body, &req)
		if req.Model != "nomic-embed-text" {
			t.Errorf("model = %q", req.Model)
		}

		json.NewEncoder(w).Encode(map[string]any{
			"model":      "nomic-embed-text",
			"embeddings": [][]float64{{0.5, 0.6, 0.7}},
		})
	}))
	defer srv.Close()

	p := NewOllamaProvider(srv.URL)
	resp, err := p.GenerateEmbedding(context.Background(), EmbeddingRequest{
		Model: "nomic-embed-text",
		Input: []string{"hello"},
	})
	if err != nil {
		t.Fatalf("GenerateEmbedding: %v", err)
	}
	if len(resp.Embeddings) != 1 {
		t.Fatalf("embeddings count = %d; want 1", len(resp.Embeddings))
	}
	if len(resp.Embeddings[0]) != 3 {
		t.Errorf("embedding dim = %d; want 3", len(resp.Embeddings[0]))
	}
}

func TestOllamaEmbeddingTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	p := NewOllamaProvider(srv.URL)
	_, err := p.GenerateEmbedding(ctx, EmbeddingRequest{
		Model: "nomic-embed-text",
		Input: []string{"hello"},
	})
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestOllamaEmbeddingHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		w.Write([]byte(`model not found`))
	}))
	defer srv.Close()

	p := NewOllamaProvider(srv.URL)
	_, err := p.GenerateEmbedding(context.Background(), EmbeddingRequest{
		Model: "nonexistent",
		Input: []string{"hello"},
	})
	pe, ok := err.(*ProviderError)
	if !ok {
		t.Fatalf("expected *ProviderError, got %T: %v", err, err)
	}
	if pe.StatusCode != 404 {
		t.Errorf("StatusCode = %d; want 404", pe.StatusCode)
	}
}

func TestAnthropicNotEmbeddingProvider(t *testing.T) {
	p := NewAnthropicProvider("key", "")
	_, ok := any(p).(EmbeddingProvider)
	if ok {
		t.Fatal("AnthropicProvider should NOT implement EmbeddingProvider")
	}
}
