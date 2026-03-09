package sms

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const plivoDefaultBaseURL = "https://api.plivo.com"

// PlivoProvider sends SMS via the Plivo REST API.
type PlivoProvider struct {
	authID     string
	authToken  string
	fromNumber string
	baseURL    string
	client     http.Client
}

// NewPlivoProvider creates a PlivoProvider. If baseURL is empty, the Plivo
// production API is used.
func NewPlivoProvider(authID, authToken, fromNumber, baseURL string) *PlivoProvider {
	if baseURL == "" {
		baseURL = plivoDefaultBaseURL
	}
	return &PlivoProvider{
		authID:     authID,
		authToken:  authToken,
		fromNumber: fromNumber,
		baseURL:    baseURL,
	}
}

func (p *PlivoProvider) Send(ctx context.Context, to, body string) (*SendResult, error) {
	endpoint := fmt.Sprintf("%s/v1/Account/%s/Message/", p.baseURL, p.authID)

	reqBody, err := json.Marshal(map[string]string{
		"src":  p.fromNumber,
		"dst":  to,
		"text": body,
	})
	if err != nil {
		return nil, fmt.Errorf("plivo: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("plivo: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(p.authID, p.authToken)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("plivo: send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("plivo: read response: %w", err)
	}

	if resp.StatusCode >= 300 {
		var errResp struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(respBody, &errResp) == nil && errResp.Error != "" {
			return nil, fmt.Errorf("plivo: error %d: %s", resp.StatusCode, errResp.Error)
		}
		return nil, fmt.Errorf("plivo: error %d: %s", resp.StatusCode, string(respBody))
	}

	var parsed struct {
		MessageUUID []string `json:"message_uuid"`
		Message     string   `json:"message"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, fmt.Errorf("plivo: parse response: %w", err)
	}

	var messageID string
	if len(parsed.MessageUUID) > 0 {
		messageID = parsed.MessageUUID[0]
	}

	return &SendResult{
		MessageID: messageID,
		Status:    "queued",
	}, nil
}
