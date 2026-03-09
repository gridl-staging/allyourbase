package sms

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// WebhookProvider sends SMS by POSTing to a custom webhook URL with HMAC signing.
type WebhookProvider struct {
	url    string
	secret string
	client http.Client
}

// NewWebhookProvider creates a WebhookProvider.
func NewWebhookProvider(url, secret string) *WebhookProvider {
	return &WebhookProvider{
		url:    url,
		secret: secret,
	}
}

func (p *WebhookProvider) Send(ctx context.Context, to, body string) (*SendResult, error) {
	reqBody, err := json.Marshal(map[string]string{
		"to":        to,
		"body":      body,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
	if err != nil {
		return nil, fmt.Errorf("webhook: marshal request: %w", err)
	}

	// Compute HMAC-SHA256 signature
	mac := hmac.New(sha256.New, []byte(p.secret))
	mac.Write(reqBody)
	sig := hex.EncodeToString(mac.Sum(nil))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.url, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("webhook: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Webhook-Signature", sig)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("webhook: send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("webhook: read response: %w", err)
	}

	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("webhook: error %d: %s", resp.StatusCode, string(respBody))
	}

	var parsed struct {
		MessageID string `json:"message_id"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, fmt.Errorf("webhook: parse response: %w", err)
	}

	return &SendResult{
		MessageID: parsed.MessageID,
		Status:    "sent",
	}, nil
}
