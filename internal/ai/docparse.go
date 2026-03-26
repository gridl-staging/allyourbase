// Package ai ai parses documents with AI providers and validates JSON responses against schemas.
package ai

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// ParseDocumentRequest describes a document parsing request.
type ParseDocumentRequest struct {
	StoragePath string         `json:"storage_path,omitempty"`
	URL         string         `json:"url,omitempty"`
	ContentType string         `json:"content_type,omitempty"`
	Schema      map[string]any `json:"schema,omitempty"`
	Provider    string         `json:"provider,omitempty"`
	Model       string         `json:"model,omitempty"`
	Prompt      string         `json:"prompt,omitempty"`
}

// StorageFetcher retrieves document bytes from AYB storage.
type StorageFetcher interface {
	Fetch(ctx context.Context, path string) ([]byte, string, error) // bytes, content-type, error
}

// ParseDocument fetches a document, sends it to an AI provider, parses the JSON response,
// and optionally validates it against a schema.
func ParseDocument(ctx context.Context, req ParseDocumentRequest, fetcher StorageFetcher, registry *Registry) (map[string]any, error) {
	docBytes, contentType, err := fetchDocument(ctx, req, fetcher)
	if err != nil {
		return nil, err
	}
	if req.ContentType != "" {
		contentType = req.ContentType
	}

	prompt := req.Prompt
	if prompt == "" {
		prompt = "Extract the structured data from this document and return it as JSON."
	}
	if req.Schema != nil {
		schemaJSON, _ := json.MarshalIndent(req.Schema, "", "  ")
		prompt += "\n\nThe output must conform to this JSON schema:\n" + string(schemaJSON)
	}

	var messages []Message
	if isImageType(contentType) {
		dataURI := "data:" + contentType + ";base64," + base64.StdEncoding.EncodeToString(docBytes)
		messages = append(messages, Message{
			Role: "user",
			Content: []ContentPart{
				{Type: "image_url", ImageURL: dataURI},
				{Type: "text", Text: prompt},
			},
		})
	} else {
		messages = append(messages, Message{
			Role: "user",
			Content: []ContentPart{
				{Type: "text", Text: "Document content:\n\n" + string(docBytes) + "\n\n" + prompt},
			},
		})
	}

	providerName := req.Provider
	model := req.Model
	if providerName == "" {
		providerName = "openai"
	}

	provider, err := registry.Get(providerName)
	if err != nil {
		return nil, fmt.Errorf("docparse: %w", err)
	}

	resp, err := provider.GenerateText(ctx, GenerateTextRequest{
		Model:    model,
		Messages: messages,
	})
	if err != nil {
		return nil, fmt.Errorf("docparse: AI call failed: %w", err)
	}

	result, err := extractJSON(resp.Text)
	if err != nil {
		return nil, fmt.Errorf("docparse: failed to parse AI response as JSON: %w", err)
	}

	if req.Schema != nil {
		if err := validateJSONSchema(result, req.Schema); err != nil {
			return nil, fmt.Errorf("docparse: response does not match schema: %w", err)
		}
	}

	return result, nil
}

func fetchDocument(ctx context.Context, req ParseDocumentRequest, fetcher StorageFetcher) ([]byte, string, error) {
	if req.StoragePath != "" {
		if fetcher == nil {
			return nil, "", fmt.Errorf("docparse: storage fetcher not configured")
		}
		return fetcher.Fetch(ctx, req.StoragePath)
	}
	if req.URL != "" {
		return fetchURL(ctx, req.URL)
	}
	return nil, "", fmt.Errorf("docparse: either storage_path or url is required")
}

// fetchURL retrieves document bytes from the given URL via HTTP GET and returns the response body and Content-Type header.
func fetchURL(ctx context.Context, url string) ([]byte, string, error) {
	httpReq, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, "", fmt.Errorf("docparse: create request: %w", err)
	}
	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, "", fmt.Errorf("docparse: fetch url: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("docparse: fetch url returned HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("docparse: read response: %w", err)
	}
	ct := resp.Header.Get("Content-Type")
	return data, ct, nil
}

func isImageType(contentType string) bool {
	return strings.HasPrefix(contentType, "image/")
}

// extractJSON tries to parse AI response as JSON, stripping markdown code fences if present.
func extractJSON(text string) (map[string]any, error) {
	text = strings.TrimSpace(text)

	if strings.HasPrefix(text, "```") {
		lines := strings.SplitN(text, "\n", 2)
		if len(lines) > 1 {
			text = lines[1]
		}
		if idx := strings.LastIndex(text, "```"); idx >= 0 {
			text = text[:idx]
		}
		text = strings.TrimSpace(text)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	return result, nil
}

// validateJSONSchema performs basic structural validation against a JSON Schema.
func validateJSONSchema(result map[string]any, schema map[string]any) error {
	properties, _ := schema["properties"].(map[string]any)
	required, _ := schema["required"].([]any)

	for _, r := range required {
		key, ok := r.(string)
		if !ok {
			continue
		}
		if _, exists := result[key]; !exists {
			return fmt.Errorf("missing required field %q", key)
		}
	}

	for key, val := range result {
		prop, ok := properties[key]
		if !ok {
			continue
		}
		propMap, ok := prop.(map[string]any)
		if !ok {
			continue
		}
		expectedType, _ := propMap["type"].(string)
		if expectedType == "" {
			continue
		}
		if err := checkJSONType(key, val, expectedType); err != nil {
			return err
		}
	}
	return nil
}

// checkJSONType validates that a value matches the expected JSON Schema type and returns an error if validation fails.
func checkJSONType(key string, val any, expected string) error {
	switch expected {
	case "string":
		if _, ok := val.(string); !ok {
			return fmt.Errorf("field %q: expected string, got %T", key, val)
		}
	case "number", "integer":
		if _, ok := val.(float64); !ok {
			return fmt.Errorf("field %q: expected number, got %T", key, val)
		}
	case "boolean":
		if _, ok := val.(bool); !ok {
			return fmt.Errorf("field %q: expected boolean, got %T", key, val)
		}
	case "array":
		if _, ok := val.([]any); !ok {
			return fmt.Errorf("field %q: expected array, got %T", key, val)
		}
	case "object":
		if _, ok := val.(map[string]any); !ok {
			return fmt.Errorf("field %q: expected object, got %T", key, val)
		}
	}
	return nil
}
