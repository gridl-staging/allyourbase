// Package ai AnthropicProvider implements the Provider interface for Anthropic's Claude API.
package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const (
	anthropicDefaultBaseURL = "https://api.anthropic.com/v1"
	anthropicVersion        = "2023-06-01"
)

// AnthropicProvider implements Provider for the Anthropic Messages API.
type AnthropicProvider struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

// NewAnthropicProvider creates a provider for Anthropic's Claude API.
func NewAnthropicProvider(apiKey, baseURL string) *AnthropicProvider {
	if baseURL == "" {
		baseURL = anthropicDefaultBaseURL
	}
	return &AnthropicProvider{
		apiKey:  apiKey,
		baseURL: baseURL,
		client:  &http.Client{},
	}
}

// GenerateText makes a request to the Anthropic Messages API and returns the generated text response. It defaults MaxTokens to 1024 if unspecified and concatenates multiple text blocks from the response.
func (p *AnthropicProvider) GenerateText(ctx context.Context, req GenerateTextRequest) (GenerateTextResponse, error) {
	anthReq := anthropicRequest{
		Model:    req.Model,
		Messages: buildAnthropicMessages(req.Messages),
	}
	if req.SystemPrompt != "" {
		anthReq.System = req.SystemPrompt
	}
	if req.MaxTokens > 0 {
		anthReq.MaxTokens = req.MaxTokens
	} else {
		anthReq.MaxTokens = 1024 // Anthropic requires max_tokens
	}
	if req.Temperature != nil {
		anthReq.Temperature = req.Temperature
	}

	payload, err := json.Marshal(anthReq)
	if err != nil {
		return GenerateTextResponse{}, fmt.Errorf("anthropic: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/messages", bytes.NewReader(payload))
	if err != nil {
		return GenerateTextResponse{}, fmt.Errorf("anthropic: create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", p.apiKey)
	httpReq.Header.Set("anthropic-version", anthropicVersion)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return GenerateTextResponse{}, fmt.Errorf("anthropic: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return GenerateTextResponse{}, fmt.Errorf("anthropic: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return GenerateTextResponse{}, &ProviderError{
			StatusCode: resp.StatusCode,
			Message:    fmt.Sprintf("HTTP %d: %s", resp.StatusCode, truncate(string(respBody), 200)),
			Provider:   "anthropic",
		}
	}

	var anthResp anthropicResponse
	if err := json.Unmarshal(respBody, &anthResp); err != nil {
		return GenerateTextResponse{}, fmt.Errorf("anthropic: unmarshal response: %w", err)
	}

	text := ""
	for _, block := range anthResp.Content {
		if block.Type == "text" {
			text += block.Text
		}
	}

	return GenerateTextResponse{
		Text:         text,
		Model:        anthResp.Model,
		FinishReason: anthResp.StopReason,
		Usage: Usage{
			InputTokens:  anthResp.Usage.InputTokens,
			OutputTokens: anthResp.Usage.OutputTokens,
		},
	}, nil
}

// buildAnthropicMessages converts our Messages to the Anthropic content format.
func buildAnthropicMessages(messages []Message) []anthropicMessage {
	var result []anthropicMessage
	for _, m := range messages {
		if m.Role == "system" {
			continue // system prompt is top-level in Anthropic
		}
		var content []anthropicContent
		for _, c := range m.Content {
			switch c.Type {
			case "text":
				content = append(content, anthropicContent{Type: "text", Text: c.Text})
			case "image_url":
				content = append(content, anthropicContent{
					Type: "image",
					Source: &anthropicImageSource{
						Type: "url",
						URL:  c.ImageURL,
					},
				})
			}
		}
		result = append(result, anthropicMessage{Role: m.Role, Content: content})
	}
	return result
}

// --- Anthropic wire types ---

type anthropicRequest struct {
	Model       string             `json:"model"`
	System      string             `json:"system,omitempty"`
	Messages    []anthropicMessage `json:"messages"`
	MaxTokens   int                `json:"max_tokens"`
	Temperature *float64           `json:"temperature,omitempty"`
}

type anthropicMessage struct {
	Role    string             `json:"role"`
	Content []anthropicContent `json:"content"`
}

type anthropicContent struct {
	Type   string                `json:"type"`
	Text   string                `json:"text,omitempty"`
	Source *anthropicImageSource `json:"source,omitempty"`
}

type anthropicImageSource struct {
	Type string `json:"type"`
	URL  string `json:"url,omitempty"`
}

type anthropicResponse struct {
	Model      string `json:"model"`
	StopReason string `json:"stop_reason"`
	Content    []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Usage struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}
