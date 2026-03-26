// Package ai OllamaProvider implements Provider and EmbeddingProvider interfaces for local Ollama servers, communicating via HTTP to generate text and embeddings.
package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const ollamaDefaultBaseURL = "http://localhost:11434"

// OllamaProvider implements Provider for a local Ollama server.
type OllamaProvider struct {
	baseURL string
	client  *http.Client
}

// NewOllamaProvider creates a provider for a local Ollama instance.
func NewOllamaProvider(baseURL string) *OllamaProvider {
	if baseURL == "" {
		baseURL = ollamaDefaultBaseURL
	}
	return &OllamaProvider{
		baseURL: baseURL,
		client:  &http.Client{},
	}
}

// GenerateText calls the Ollama chat API endpoint and returns the generated text response with token usage and finish reason information.
func (p *OllamaProvider) GenerateText(ctx context.Context, req GenerateTextRequest) (GenerateTextResponse, error) {
	messages := buildOllamaMessages(req)

	body := ollamaRequest{
		Model:    req.Model,
		Messages: messages,
		Stream:   false,
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return GenerateTextResponse{}, fmt.Errorf("ollama: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/api/chat", bytes.NewReader(payload))
	if err != nil {
		return GenerateTextResponse{}, fmt.Errorf("ollama: create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return GenerateTextResponse{}, fmt.Errorf("ollama: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return GenerateTextResponse{}, fmt.Errorf("ollama: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return GenerateTextResponse{}, &ProviderError{
			StatusCode: resp.StatusCode,
			Message:    fmt.Sprintf("HTTP %d: %s", resp.StatusCode, truncate(string(respBody), 200)),
			Provider:   "ollama",
		}
	}

	var olResp ollamaResponse
	if err := json.Unmarshal(respBody, &olResp); err != nil {
		return GenerateTextResponse{}, fmt.Errorf("ollama: unmarshal response: %w", err)
	}

	finishReason := "stop"
	if olResp.DoneReason != "" {
		finishReason = olResp.DoneReason
	}

	return GenerateTextResponse{
		Text:         olResp.Message.Content,
		Model:        olResp.Model,
		FinishReason: finishReason,
		Usage: Usage{
			InputTokens:  olResp.PromptEvalCount,
			OutputTokens: olResp.EvalCount,
		},
	}, nil
}

// buildOllamaMessages converts the request messages to Ollama message format, including the system prompt if provided and extracting text content from message blocks.
func buildOllamaMessages(req GenerateTextRequest) []ollamaMessage {
	var msgs []ollamaMessage
	if req.SystemPrompt != "" {
		msgs = append(msgs, ollamaMessage{Role: "system", Content: req.SystemPrompt})
	}
	for _, m := range req.Messages {
		text := ""
		for _, c := range m.Content {
			if c.Type == "text" {
				text += c.Text
			}
		}
		msgs = append(msgs, ollamaMessage{Role: m.Role, Content: text})
	}
	return msgs
}

// --- Ollama wire types ---

type ollamaRequest struct {
	Model    string          `json:"model"`
	Messages []ollamaMessage `json:"messages"`
	Stream   bool            `json:"stream"`
}

type ollamaMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ollamaResponse struct {
	Model           string        `json:"model"`
	Message         ollamaMessage `json:"message"`
	DoneReason      string        `json:"done_reason"`
	PromptEvalCount int           `json:"prompt_eval_count"`
	EvalCount       int           `json:"eval_count"`
}

// GenerateEmbedding implements EmbeddingProvider for Ollama.
func (p *OllamaProvider) GenerateEmbedding(ctx context.Context, req EmbeddingRequest) (EmbeddingResponse, error) {
	body := ollamaEmbedRequest(req)

	payload, err := json.Marshal(body)
	if err != nil {
		return EmbeddingResponse{}, fmt.Errorf("ollama: marshal embedding request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/api/embed", bytes.NewReader(payload))
	if err != nil {
		return EmbeddingResponse{}, fmt.Errorf("ollama: create embedding request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return EmbeddingResponse{}, fmt.Errorf("ollama: embedding request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return EmbeddingResponse{}, fmt.Errorf("ollama: read embedding response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return EmbeddingResponse{}, &ProviderError{
			StatusCode: resp.StatusCode,
			Message:    fmt.Sprintf("HTTP %d: %s", resp.StatusCode, truncate(string(respBody), 200)),
			Provider:   "ollama",
		}
	}

	var olResp ollamaEmbedResponse
	if err := json.Unmarshal(respBody, &olResp); err != nil {
		return EmbeddingResponse{}, fmt.Errorf("ollama: unmarshal embedding response: %w", err)
	}

	return EmbeddingResponse{
		Embeddings: olResp.Embeddings,
		Model:      olResp.Model,
	}, nil
}

// --- Ollama embedding wire types ---

type ollamaEmbedRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type ollamaEmbedResponse struct {
	Model      string      `json:"model"`
	Embeddings [][]float64 `json:"embeddings"`
}
