package sms

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const vonageDefaultBaseURL = "https://rest.nexmo.com"

// VonageProvider sends SMS via the Vonage (Nexmo) REST API.
type VonageProvider struct {
	apiKey     string
	apiSecret  string
	fromNumber string
	baseURL    string
	client     http.Client
}

// NewVonageProvider creates a VonageProvider. If baseURL is empty, the Vonage
// production API is used.
func NewVonageProvider(apiKey, apiSecret, fromNumber, baseURL string) *VonageProvider {
	if baseURL == "" {
		baseURL = vonageDefaultBaseURL
	}
	return &VonageProvider{
		apiKey:     apiKey,
		apiSecret:  apiSecret,
		fromNumber: fromNumber,
		baseURL:    baseURL,
	}
}

func (p *VonageProvider) Send(ctx context.Context, to, body string) (*SendResult, error) {
	endpoint := p.baseURL + "/sms/json"

	form := url.Values{}
	form.Set("api_key", p.apiKey)
	form.Set("api_secret", p.apiSecret)
	form.Set("from", p.fromNumber)
	form.Set("to", to)
	form.Set("text", body)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("vonage: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("vonage: send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("vonage: read response: %w", err)
	}

	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("vonage: error %d: %s", resp.StatusCode, string(respBody))
	}

	var parsed struct {
		Messages []struct {
			MessageID string `json:"message-id"`
			Status    string `json:"status"`
			ErrorText string `json:"error-text"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, fmt.Errorf("vonage: parse response: %w", err)
	}

	if len(parsed.Messages) == 0 {
		return nil, fmt.Errorf("vonage: empty response")
	}

	msg := parsed.Messages[0]
	if msg.Status != "0" {
		return nil, fmt.Errorf("vonage: error (status %s): %s", msg.Status, msg.ErrorText)
	}

	return &SendResult{
		MessageID: msg.MessageID,
		Status:    "sent",
	}, nil
}
