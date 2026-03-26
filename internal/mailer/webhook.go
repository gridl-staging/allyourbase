// Package mailer WebhookMailer sends email messages via HTTP webhooks to a user-configured endpoint.
package mailer

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

// WebhookConfig holds webhook mailer parameters.
type WebhookConfig struct {
	URL     string
	Secret  string
	Timeout time.Duration
}

// WebhookMailer sends email payloads to a user-configured HTTP endpoint.
type WebhookMailer struct {
	cfg    WebhookConfig
	client *http.Client
}

// NewWebhookMailer creates a WebhookMailer with the given config.
func NewWebhookMailer(cfg WebhookConfig) *WebhookMailer {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 10 * time.Second
	}
	return &WebhookMailer{
		cfg:    cfg,
		client: &http.Client{Timeout: timeout},
	}
}

type webhookPayload struct {
	To      string `json:"to"`
	Subject string `json:"subject"`
	HTML    string `json:"html"`
	Text    string `json:"text"`
	From    string `json:"from,omitempty"`
}

// Send marshals the message to JSON and posts it to the configured webhook URL. If a secret is configured, it includes an HMAC-SHA256 signature in the X-AYB-Signature header. Returns an error if the request fails or the webhook returns a non-2xx status code.
func (m *WebhookMailer) Send(ctx context.Context, msg *Message) error {
	payload, err := json.Marshal(webhookPayload{
		To:      msg.To,
		Subject: msg.Subject,
		HTML:    msg.HTML,
		Text:    msg.Text,
		From:    msg.From,
	})
	if err != nil {
		return fmt.Errorf("marshaling webhook payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.cfg.URL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("creating webhook request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	if m.cfg.Secret != "" {
		mac := hmac.New(sha256.New, []byte(m.cfg.Secret))
		mac.Write(payload)
		sig := hex.EncodeToString(mac.Sum(nil))
		req.Header.Set("X-AYB-Signature", sig)
	}

	resp, err := m.client.Do(req)
	if err != nil {
		return fmt.Errorf("webhook request failed: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}
	return nil
}
