// Package ai OpenAI implements Provider for OpenAI-compatible APIs, handling text generation and embedding requests.
package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const openaiDefaultBaseURL = "https://api.openai.com/v1"

// OpenAIProvider implements Provider for OpenAI-compatible APIs.
type OpenAIProvider struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

// NewOpenAIProvider creates a provider for OpenAI (or compatible endpoints like Azure).
func NewOpenAIProvider(apiKey, baseURL string) *OpenAIProvider {
	if baseURL == "" {
		baseURL = openaiDefaultBaseURL
	}
	return &OpenAIProvider{
		apiKey:  apiKey,
		baseURL: baseURL,
		client:  &http.Client{},
	}
}

// GenerateText sends a chat completion request to the OpenAI API and returns the generated text with usage metrics. It respects the provided context for cancellation and returns a ProviderError on HTTP failures.
func (p *OpenAIProvider) GenerateText(ctx context.Context, req GenerateTextRequest) (GenerateTextResponse, error) {
	messages := buildOpenAIMessages(req)

	body := openaiRequest{
		Model:    req.Model,
		Messages: messages,
	}
	if req.MaxTokens > 0 {
		body.MaxTokens = &req.MaxTokens
	}
	if req.Temperature != nil {
		body.Temperature = req.Temperature
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return GenerateTextResponse{}, fmt.Errorf("openai: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return GenerateTextResponse{}, fmt.Errorf("openai: create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return GenerateTextResponse{}, fmt.Errorf("openai: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return GenerateTextResponse{}, fmt.Errorf("openai: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return GenerateTextResponse{}, &ProviderError{
			StatusCode: resp.StatusCode,
			Message:    fmt.Sprintf("HTTP %d: %s", resp.StatusCode, truncate(string(respBody), 200)),
			Provider:   "openai",
		}
	}

	var oaiResp openaiResponse
	if err := json.Unmarshal(respBody, &oaiResp); err != nil {
		return GenerateTextResponse{}, fmt.Errorf("openai: unmarshal response: %w", err)
	}

	text := ""
	finishReason := ""
	if len(oaiResp.Choices) > 0 {
		text = oaiResp.Choices[0].Message.Content
		finishReason = oaiResp.Choices[0].FinishReason
	}

	return GenerateTextResponse{
		Text:         text,
		Model:        oaiResp.Model,
		FinishReason: finishReason,
		Usage: Usage{
			InputTokens:  oaiResp.Usage.PromptTokens,
			OutputTokens: oaiResp.Usage.CompletionTokens,
		},
	}, nil
}

// buildOpenAIMessages converts our Message format to OpenAI's format.
func buildOpenAIMessages(req GenerateTextRequest) []openaiMessage {
	var msgs []openaiMessage

	if req.SystemPrompt != "" {
		msgs = append(msgs, openaiMessage{
			Role:    "system",
			Content: req.SystemPrompt,
		})
	}

	for _, m := range req.Messages {
		if len(m.Content) == 1 && m.Content[0].Type == "text" {
			msgs = append(msgs, openaiMessage{
				Role:    m.Role,
				Content: m.Content[0].Text,
			})
		} else {
			// Multimodal: use content array format.
			var parts []any
			for _, c := range m.Content {
				switch c.Type {
				case "text":
					parts = append(parts, map[string]string{"type": "text", "text": c.Text})
				case "image_url":
					parts = append(parts, map[string]any{
						"type":      "image_url",
						"image_url": map[string]string{"url": c.ImageURL},
					})
				}
			}
			msgs = append(msgs, openaiMessage{
				Role:         m.Role,
				ContentParts: parts,
			})
		}
	}
	return msgs
}

// --- OpenAI wire types ---

type openaiRequest struct {
	Model       string          `json:"model"`
	Messages    []openaiMessage `json:"messages"`
	MaxTokens   *int            `json:"max_tokens,omitempty"`
	Temperature *float64        `json:"temperature,omitempty"`
}

// openaiMessage supports both simple string content and multimodal arrays.
type openaiMessage struct {
	Role         string `json:"role"`
	Content      string `json:"-"`
	ContentParts []any  `json:"-"`
}

func (m openaiMessage) MarshalJSON() ([]byte, error) {
	if m.ContentParts != nil {
		return json.Marshal(struct {
			Role    string `json:"role"`
			Content []any  `json:"content"`
		}{Role: m.Role, Content: m.ContentParts})
	}
	return json.Marshal(struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}{Role: m.Role, Content: m.Content})
}

type openaiResponse struct {
	Model   string `json:"model"`
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
}

// GenerateEmbedding implements EmbeddingProvider for OpenAI.
func (p *OpenAIProvider) GenerateEmbedding(ctx context.Context, req EmbeddingRequest) (EmbeddingResponse, error) {
	body := openaiEmbeddingRequest(req)

	payload, err := json.Marshal(body)
	if err != nil {
		return EmbeddingResponse{}, fmt.Errorf("openai: marshal embedding request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/embeddings", bytes.NewReader(payload))
	if err != nil {
		return EmbeddingResponse{}, fmt.Errorf("openai: create embedding request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return EmbeddingResponse{}, fmt.Errorf("openai: embedding request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return EmbeddingResponse{}, fmt.Errorf("openai: read embedding response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return EmbeddingResponse{}, &ProviderError{
			StatusCode: resp.StatusCode,
			Message:    fmt.Sprintf("HTTP %d: %s", resp.StatusCode, truncate(string(respBody), 200)),
			Provider:   "openai",
		}
	}

	var oaiResp openaiEmbeddingResponse
	if err := json.Unmarshal(respBody, &oaiResp); err != nil {
		return EmbeddingResponse{}, fmt.Errorf("openai: unmarshal embedding response: %w", err)
	}

	embeddings := make([][]float64, len(oaiResp.Data))
	for i, d := range oaiResp.Data {
		embeddings[i] = d.Embedding
	}

	return EmbeddingResponse{
		Embeddings: embeddings,
		Model:      oaiResp.Model,
		Usage: Usage{
			InputTokens: oaiResp.Usage.PromptTokens,
		},
	}, nil
}

// --- OpenAI embedding wire types ---

type openaiEmbeddingRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type openaiEmbeddingResponse struct {
	Model string `json:"model"`
	Data  []struct {
		Embedding []float64 `json:"embedding"`
	} `json:"data"`
	Usage struct {
		PromptTokens int `json:"prompt_tokens"`
		TotalTokens  int `json:"total_tokens"`
	} `json:"usage"`
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
