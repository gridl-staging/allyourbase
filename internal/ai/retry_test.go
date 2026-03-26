package ai

import (
	"context"
	"io"
	"testing"
	"time"
)

func TestRetryOn429(t *testing.T) {
	calls := 0
	mp := &mockProvider{fn: func(ctx context.Context, req GenerateTextRequest) (GenerateTextResponse, error) {
		calls++
		if calls <= 2 {
			return GenerateTextResponse{}, &ProviderError{StatusCode: 429, Message: "rate limited", Provider: "test"}
		}
		return GenerateTextResponse{Text: "ok"}, nil
	}}

	rp := NewRetryProvider(mp, 3, 0)
	resp, err := rp.GenerateText(context.Background(), GenerateTextRequest{})
	if err != nil {
		t.Fatalf("expected success after retries: %v", err)
	}
	if resp.Text != "ok" {
		t.Errorf("Text = %q", resp.Text)
	}
	if calls != 3 {
		t.Errorf("calls = %d; want 3", calls)
	}
}

func TestRetryOn500(t *testing.T) {
	calls := 0
	mp := &mockProvider{fn: func(ctx context.Context, req GenerateTextRequest) (GenerateTextResponse, error) {
		calls++
		return GenerateTextResponse{}, &ProviderError{StatusCode: 500, Message: "server error", Provider: "test"}
	}}

	rp := NewRetryProvider(mp, 2, 0)
	_, err := rp.GenerateText(context.Background(), GenerateTextRequest{})
	if err == nil {
		t.Fatal("expected error after max retries")
	}
	if calls != 3 {
		t.Errorf("calls = %d; want 3", calls)
	}
}

func TestNoRetryOn400(t *testing.T) {
	calls := 0
	mp := &mockProvider{fn: func(ctx context.Context, req GenerateTextRequest) (GenerateTextResponse, error) {
		calls++
		return GenerateTextResponse{}, &ProviderError{StatusCode: 400, Message: "bad request", Provider: "test"}
	}}

	rp := NewRetryProvider(mp, 3, 0)
	_, err := rp.GenerateText(context.Background(), GenerateTextRequest{})
	if err == nil {
		t.Fatal("expected error")
	}
	if calls != 1 {
		t.Errorf("calls = %d; want 1 (no retries for 400)", calls)
	}
}

func TestRetryRespectsContextTimeout(t *testing.T) {
	mp := &mockProvider{fn: func(ctx context.Context, req GenerateTextRequest) (GenerateTextResponse, error) {
		return GenerateTextResponse{}, &ProviderError{StatusCode: 429, Message: "rate limited", Provider: "test"}
	}}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	rp := NewRetryProvider(mp, 10, 0)
	_, err := rp.GenerateText(ctx, GenerateTextRequest{})
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestRetryEmbeddingOn429(t *testing.T) {
	calls := 0
	inner := &mockEmbeddingProvider{fn: func(ctx context.Context, req EmbeddingRequest) (EmbeddingResponse, error) {
		calls++
		if calls <= 2 {
			return EmbeddingResponse{}, &ProviderError{StatusCode: 429, Message: "rate limited", Provider: "test"}
		}
		return EmbeddingResponse{Embeddings: [][]float64{{0.1, 0.2}}}, nil
	}}

	rp := NewRetryProvider(inner, 3, 0)
	ep, ok := rp.(EmbeddingProvider)
	if !ok {
		t.Fatal("RetryProvider wrapping EmbeddingProvider should implement EmbeddingProvider")
	}
	resp, err := ep.GenerateEmbedding(context.Background(), EmbeddingRequest{})
	if err != nil {
		t.Fatalf("expected success after retries: %v", err)
	}
	if len(resp.Embeddings) != 1 {
		t.Errorf("embeddings count = %d", len(resp.Embeddings))
	}
	if calls != 3 {
		t.Errorf("calls = %d; want 3", calls)
	}
}

func TestRetryEmbeddingNotSupportedByInner(t *testing.T) {
	inner := &mockProvider{resp: GenerateTextResponse{Text: "ok"}}
	rp := NewRetryProvider(inner, 3, 0)
	_, ok := rp.(EmbeddingProvider)
	if ok {
		t.Fatal("RetryProvider wrapping non-EmbeddingProvider should NOT implement EmbeddingProvider")
	}
}

func TestRetryStreamingKeepsContextAliveUntilRead(t *testing.T) {
	inner := &mockStreamingProvider{
		streamFn: func(ctx context.Context, _ GenerateTextRequest) (io.ReadCloser, error) {
			return &contextAwareStreamReader{ctx: ctx, remaining: []byte("hello")}, nil
		},
	}

	rp := NewRetryProvider(inner, 0, 5*time.Second)
	sp, ok := rp.(StreamingProvider)
	if !ok {
		t.Fatal("RetryProvider wrapping StreamingProvider should implement StreamingProvider")
	}

	reader, err := sp.GenerateTextStream(context.Background(), GenerateTextRequest{Model: "gpt-4o-mini"})
	if err != nil {
		t.Fatalf("GenerateTextStream: %v", err)
	}
	defer reader.Close()

	got, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(got) != "hello" {
		t.Fatalf("stream output = %q; want hello", string(got))
	}
}
