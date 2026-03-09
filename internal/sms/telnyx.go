package sms

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const telnyxDefaultBaseURL = "https://api.telnyx.com"

// TelnyxProvider sends SMS via the Telnyx REST API.
type TelnyxProvider struct {
	apiKey     string
	fromNumber string
	baseURL    string
	client     http.Client
}

// NewTelnyxProvider creates a TelnyxProvider. If baseURL is empty, the Telnyx
// production API is used.
func NewTelnyxProvider(apiKey, fromNumber, baseURL string) *TelnyxProvider {
	if baseURL == "" {
		baseURL = telnyxDefaultBaseURL
	}
	return &TelnyxProvider{
		apiKey:     apiKey,
		fromNumber: fromNumber,
		baseURL:    baseURL,
	}
}

func (p *TelnyxProvider) Send(ctx context.Context, to, body string) (*SendResult, error) {
	endpoint := p.baseURL + "/v2/messages"

	reqBody, err := json.Marshal(map[string]string{
		"from": p.fromNumber,
		"to":   to,
		"text": body,
	})
	if err != nil {
		return nil, fmt.Errorf("telnyx: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("telnyx: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("telnyx: send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("telnyx: read response: %w", err)
	}

	if resp.StatusCode >= 300 {
		var errResp struct {
			Errors []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			} `json:"errors"`
		}
		if json.Unmarshal(respBody, &errResp) == nil && len(errResp.Errors) > 0 {
			return nil, fmt.Errorf("telnyx: error %d: %s", resp.StatusCode, errResp.Errors[0].Title)
		}
		return nil, fmt.Errorf("telnyx: error %d: %s", resp.StatusCode, string(respBody))
	}

	var parsed struct {
		Data struct {
			ID   string `json:"id"`
			Type string `json:"type"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, fmt.Errorf("telnyx: parse response: %w", err)
	}

	return &SendResult{
		MessageID: parsed.Data.ID,
		Status:    parsed.Data.Type,
	}, nil
}
