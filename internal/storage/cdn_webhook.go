// Package storage Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/internal/storage/cdn_webhook.go.
package storage

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/allyourbase/ayb/internal/backoff"
)

const (
	webhookProviderName    = "webhook"
	webhookSignatureHeader = "X-CDN-Webhook-Signature"
	webhookPurgeURLsOp     = "purge_urls"
	webhookPurgeAllOp      = "purge_all"
)

// WebhookCDNOptions captures construction dependencies for a webhook provider.
type WebhookCDNOptions struct {
	Endpoint      string
	SigningSecret string
	HTTPClient    HTTPDoer
	MaxRetries    int
	BackoffConfig backoff.Config
}

// WebhookCDNProvider sends generic purge requests to a configured endpoint.
type WebhookCDNProvider struct {
	endpoint      string
	signingSecret string
	httpClient    HTTPDoer
	maxRetries    int
	backoffConfig backoff.Config
}

// NewWebhookCDNProvider constructs a webhook-backed CDN provider.
func NewWebhookCDNProvider(opts WebhookCDNOptions) *WebhookCDNProvider {
	httpClient := opts.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	maxRetries, backoffConfig := resolveCDNRetrySettings(opts.MaxRetries, opts.BackoffConfig)

	return &WebhookCDNProvider{
		endpoint:      strings.TrimSpace(opts.Endpoint),
		signingSecret: opts.SigningSecret,
		httpClient:    httpClient,
		maxRetries:    maxRetries,
		backoffConfig: backoffConfig,
	}
}

// Name returns the provider name used for logging and diagnostics.
func (p *WebhookCDNProvider) Name() string {
	return webhookProviderName
}

// PurgeURLs sends a webhook request for specific public URLs.
func (p *WebhookCDNProvider) PurgeURLs(ctx context.Context, publicURLs []string) error {
	urls := sanitizePublicURLs(publicURLs)
	if len(urls) == 0 {
		return nil
	}

	return p.send(ctx, webhookPayload{
		Operation: webhookPurgeURLsOp,
		URLs:      urls,
	})
}

// PurgeAll sends a webhook request requesting full cache purge semantics.
func (p *WebhookCDNProvider) PurgeAll(ctx context.Context) error {
	return p.send(ctx, webhookPayload{Operation: webhookPurgeAllOp})
}

// TODO: Document WebhookCDNProvider.send.
func (p *WebhookCDNProvider) send(ctx context.Context, payload webhookPayload) error {
	body, err := stableWebhookPayloadBytes(payload)
	if err != nil {
		return err
	}

	return doWithRetry(
		ctx,
		p.maxRetries,
		p.backoffConfig,
		p.isRetryableWebhookError,
		func(ctx context.Context) error {
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(body))
			if err != nil {
				return err
			}

			req.Header.Set("Content-Type", "application/json")
			req.Header.Set(webhookSignatureHeader, webhookPayloadSignature(p.signingSecret, body))

			resp, err := p.httpClient.Do(req)
			if err != nil {
				return err
			}

			bodyText, err := readResponseBody(resp)
			if err != nil {
				return err
			}

			if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
				return &responseError{statusCode: resp.StatusCode, body: bodyText}
			}

			return nil
		},
	)
}

func (p *WebhookCDNProvider) isRetryableWebhookError(err error) bool {
	return isRetryableHTTPAdapterError(err)
}

// webhookPayload defines the stable payload contract sent to webhook consumers.
//
// Schema:
// {
// "operation": "purge_urls" | "purge_all",
// "public_urls": ["https://cdn.example.com/path"], // present only for purge_urls
// }
type webhookPayload struct {
	Operation string   `json:"operation"`
	URLs      []string `json:"public_urls,omitempty"`
}

func stableWebhookPayloadBytes(payload webhookPayload) ([]byte, error) {
	return json.Marshal(payload)
}

func webhookPayloadSignature(secret string, payload []byte) string {
	hash := hmac.New(sha256.New, []byte(secret))
	_, _ = hash.Write(payload)
	return hex.EncodeToString(hash.Sum(nil))
}

var _ CDNProvider = (*WebhookCDNProvider)(nil)
