package ai

import (
	"context"
	"io"
)

// Provider is the interface every AI backend must implement.
type Provider interface {
	GenerateText(ctx context.Context, req GenerateTextRequest) (GenerateTextResponse, error)
}

// StreamingProvider is an optional interface for providers that support streaming text generation.
type StreamingProvider interface {
	GenerateTextStream(ctx context.Context, req GenerateTextRequest) (io.ReadCloser, error)
}

// GenerateTextRequest describes a text generation call.
type GenerateTextRequest struct {
	Model        string    `json:"model"`
	Messages     []Message `json:"messages"`
	MaxTokens    int       `json:"max_tokens,omitempty"`
	Temperature  *float64  `json:"temperature,omitempty"`
	SystemPrompt string    `json:"system_prompt,omitempty"`
}

// Message is a single turn in a conversation.
type Message struct {
	Role    string        `json:"role"` // "system", "user", "assistant"
	Content []ContentPart `json:"content"`
}

// ContentPart is a piece of message content (text or image).
type ContentPart struct {
	Type     string `json:"type"` // "text", "image_url"
	Text     string `json:"text,omitempty"`
	ImageURL string `json:"image_url,omitempty"`
}

// TextContent is a convenience for creating a text-only content part.
func TextContent(text string) []ContentPart {
	return []ContentPart{{Type: "text", Text: text}}
}

// GenerateTextResponse holds the result of a text generation call.
type GenerateTextResponse struct {
	Text         string `json:"text"`
	Model        string `json:"model"`
	Usage        Usage  `json:"usage"`
	FinishReason string `json:"finish_reason"`
}

// Usage tracks token consumption.
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// ProviderError represents an HTTP error from an AI provider.
type ProviderError struct {
	StatusCode int
	Message    string
	Provider   string
}

func (e *ProviderError) Error() string {
	return e.Provider + ": " + e.Message
}

// IsRetryable returns true for errors that should trigger a retry (429, 5xx).
func (e *ProviderError) IsRetryable() bool {
	return e.StatusCode == 429 || e.StatusCode >= 500
}

// EmbeddingProvider is an optional interface for providers that support embedding.
// Not all providers implement this (e.g. Anthropic has no public embedding API).
type EmbeddingProvider interface {
	GenerateEmbedding(ctx context.Context, req EmbeddingRequest) (EmbeddingResponse, error)
}

// EmbeddingRequest describes an embedding call.
type EmbeddingRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

// EmbeddingResponse holds the result of an embedding call.
type EmbeddingResponse struct {
	Embeddings [][]float64 `json:"embeddings"`
	Model      string      `json:"model"`
	Usage      Usage       `json:"usage"`
}
