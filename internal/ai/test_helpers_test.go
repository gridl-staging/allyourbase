package ai

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
)

type mockProvider struct {
	resp GenerateTextResponse
	fn   func(context.Context, GenerateTextRequest) (GenerateTextResponse, error)
}

func (m *mockProvider) GenerateText(ctx context.Context, req GenerateTextRequest) (GenerateTextResponse, error) {
	if m.fn != nil {
		return m.fn(ctx, req)
	}
	return m.resp, nil
}

type mockStreamingProvider struct {
	mockProvider
	streamFn func(context.Context, GenerateTextRequest) (io.ReadCloser, error)
}

func (m *mockStreamingProvider) GenerateTextStream(ctx context.Context, req GenerateTextRequest) (io.ReadCloser, error) {
	if m.streamFn != nil {
		return m.streamFn(ctx, req)
	}
	return io.NopCloser(strings.NewReader("")), nil
}

type contextAwareStreamReader struct {
	ctx       context.Context
	remaining []byte
}

func (r *contextAwareStreamReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	if len(r.remaining) == 0 {
		return 0, io.EOF
	}
	n := copy(p, r.remaining)
	r.remaining = r.remaining[n:]
	return n, nil
}

func (r *contextAwareStreamReader) Close() error {
	return nil
}

type mockLogStore struct {
	logs []CallLog
}

func (m *mockLogStore) Insert(_ context.Context, log CallLog) error {
	m.logs = append(m.logs, log)
	return nil
}

func (m *mockLogStore) List(_ context.Context, _ ListFilter) ([]CallLog, int, error) {
	return m.logs, len(m.logs), nil
}

func (m *mockLogStore) UsageSummary(_ context.Context, _, _ time.Time) (UsageSummary, error) {
	return UsageSummary{}, nil
}

type mockFetcher struct {
	data []byte
	ct   string
}

func (f *mockFetcher) Fetch(_ context.Context, _ string) ([]byte, string, error) {
	return f.data, f.ct, nil
}

type mockPromptStore struct {
	prompts map[string]Prompt
}

func (m *mockPromptStore) Create(_ context.Context, _ CreatePromptRequest) (Prompt, error) {
	return Prompt{}, nil
}

func (m *mockPromptStore) Get(_ context.Context, _ uuid.UUID) (Prompt, error) {
	return Prompt{}, nil
}

func (m *mockPromptStore) GetByName(_ context.Context, name string) (Prompt, error) {
	p, ok := m.prompts[name]
	if !ok {
		return Prompt{}, fmt.Errorf("prompt %q not found", name)
	}
	return p, nil
}

func (m *mockPromptStore) List(_ context.Context, _, _ int) ([]Prompt, int, error) {
	return nil, 0, nil
}

func (m *mockPromptStore) Update(_ context.Context, _ uuid.UUID, _ UpdatePromptRequest) (Prompt, error) {
	return Prompt{}, nil
}

func (m *mockPromptStore) Delete(_ context.Context, _ uuid.UUID) error { return nil }

func (m *mockPromptStore) ListVersions(_ context.Context, _ uuid.UUID) ([]PromptVersion, error) {
	return nil, nil
}

type mockEmbeddingProvider struct {
	mockProvider
	fn func(context.Context, EmbeddingRequest) (EmbeddingResponse, error)
}

func (m *mockEmbeddingProvider) GenerateEmbedding(ctx context.Context, req EmbeddingRequest) (EmbeddingResponse, error) {
	return m.fn(ctx, req)
}
