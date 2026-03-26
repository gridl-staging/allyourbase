package ai

import (
	"context"
	"testing"
)

func TestLoggingProviderRecordsSuccess(t *testing.T) {
	store := &mockLogStore{}
	inner := &mockProvider{resp: GenerateTextResponse{
		Text:  "response",
		Model: "gpt-4o",
		Usage: Usage{InputTokens: 10, OutputTokens: 5},
	}}

	lp := NewLoggingProvider(inner, "openai", store)
	resp, err := lp.GenerateText(context.Background(), GenerateTextRequest{Model: "gpt-4o"})
	if err != nil {
		t.Fatalf("GenerateText: %v", err)
	}
	if resp.Text != "response" {
		t.Errorf("Text = %q", resp.Text)
	}

	if len(store.logs) != 1 {
		t.Fatalf("logs count = %d; want 1", len(store.logs))
	}
	log := store.logs[0]
	if log.Provider != "openai" || log.Status != "success" || log.Model != "gpt-4o" {
		t.Errorf("log = %+v", log)
	}
	if log.InputTokens != 10 || log.OutputTokens != 5 {
		t.Errorf("tokens: in=%d out=%d", log.InputTokens, log.OutputTokens)
	}
}

func TestLoggingProviderRecordsError(t *testing.T) {
	store := &mockLogStore{}
	inner := &mockProvider{fn: func(ctx context.Context, req GenerateTextRequest) (GenerateTextResponse, error) {
		return GenerateTextResponse{}, &ProviderError{StatusCode: 500, Message: "internal error", Provider: "openai"}
	}}

	lp := NewLoggingProvider(inner, "openai", store)
	_, err := lp.GenerateText(context.Background(), GenerateTextRequest{Model: "gpt-4o"})
	if err == nil {
		t.Fatal("expected error")
	}

	if len(store.logs) != 1 {
		t.Fatalf("logs count = %d; want 1", len(store.logs))
	}
	if store.logs[0].Status != "error" || store.logs[0].ErrorMessage == "" {
		t.Errorf("log = %+v", store.logs[0])
	}
}

func TestLoggingEmbeddingRecordsSuccess(t *testing.T) {
	store := &mockLogStore{}
	inner := &mockEmbeddingProvider{fn: func(ctx context.Context, req EmbeddingRequest) (EmbeddingResponse, error) {
		return EmbeddingResponse{
			Embeddings: [][]float64{{0.1, 0.2}},
			Model:      "text-embedding-3-small",
			Usage:      Usage{InputTokens: 5},
		}, nil
	}}

	lp := NewLoggingProvider(inner, "openai", store)
	ep, ok := lp.(EmbeddingProvider)
	if !ok {
		t.Fatal("LoggingProvider wrapping EmbeddingProvider should implement EmbeddingProvider")
	}
	resp, err := ep.GenerateEmbedding(context.Background(), EmbeddingRequest{Model: "text-embedding-3-small", Input: []string{"hi"}})
	if err != nil {
		t.Fatalf("GenerateEmbedding: %v", err)
	}
	if len(resp.Embeddings) != 1 {
		t.Errorf("embeddings = %d", len(resp.Embeddings))
	}

	if len(store.logs) != 1 {
		t.Fatalf("logs = %d; want 1", len(store.logs))
	}
	log := store.logs[0]
	if log.Provider != "openai" || log.Status != "success" || log.Model != "text-embedding-3-small" {
		t.Errorf("log = %+v", log)
	}
	if log.InputTokens != 5 {
		t.Errorf("InputTokens = %d; want 5", log.InputTokens)
	}
}

func TestLoggingEmbeddingNotSupportedByInner(t *testing.T) {
	store := &mockLogStore{}
	inner := &mockProvider{resp: GenerateTextResponse{Text: "ok"}}
	lp := NewLoggingProvider(inner, "openai", store)
	_, ok := lp.(EmbeddingProvider)
	if ok {
		t.Fatal("LoggingProvider wrapping non-EmbeddingProvider should NOT implement EmbeddingProvider")
	}
}
