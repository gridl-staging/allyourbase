package sms

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const msg91DefaultBaseURL = "https://control.msg91.com"

// MSG91Provider sends SMS via the MSG91 flow API.
type MSG91Provider struct {
	authKey    string
	templateID string
	baseURL    string
	client     http.Client
}

// NewMSG91Provider creates a MSG91Provider. If baseURL is empty, the MSG91
// production API is used.
func NewMSG91Provider(authKey, templateID, baseURL string) *MSG91Provider {
	if baseURL == "" {
		baseURL = msg91DefaultBaseURL
	}
	return &MSG91Provider{
		authKey:    authKey,
		templateID: templateID,
		baseURL:    baseURL,
	}
}

func (p *MSG91Provider) Send(ctx context.Context, to, body string) (*SendResult, error) {
	endpoint := p.baseURL + "/api/v5/flow/"

	type recipient struct {
		Mobiles string `json:"mobiles"`
		OTP     string `json:"otp"`
	}
	reqPayload := struct {
		TemplateID string      `json:"template_id"`
		Recipients []recipient `json:"recipients"`
	}{
		TemplateID: p.templateID,
		Recipients: []recipient{{Mobiles: to, OTP: body}},
	}

	reqBody, err := json.Marshal(reqPayload)
	if err != nil {
		return nil, fmt.Errorf("msg91: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("msg91: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("authkey", p.authKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("msg91: send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("msg91: read response: %w", err)
	}

	var parsed struct {
		Type      string `json:"type"`
		Message   string `json:"message"`
		RequestID string `json:"request_id"`
	}

	if resp.StatusCode >= 300 {
		if json.Unmarshal(respBody, &parsed) == nil && parsed.Message != "" {
			return nil, fmt.Errorf("msg91: error %d: %s", resp.StatusCode, parsed.Message)
		}
		return nil, fmt.Errorf("msg91: error %d: %s", resp.StatusCode, string(respBody))
	}

	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, fmt.Errorf("msg91: parse response: %w", err)
	}

	if parsed.Type == "error" {
		return nil, fmt.Errorf("msg91: error: %s", parsed.Message)
	}

	return &SendResult{
		MessageID: parsed.RequestID,
		Status:    parsed.Type,
	}, nil
}
