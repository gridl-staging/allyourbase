package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/allyourbase/ayb/internal/backoff"
)

const (
	cloudflareProviderName        = "cloudflare"
	cloudflarePurgeURLsBatchLimit = 30
	cloudflarePurgeURLTemplate    = "https://api.cloudflare.com/client/v4/zones/%s/purge_cache"
)

// CloudflareCDNOptions captures construction dependencies for a Cloudflare provider.
type CloudflareCDNOptions struct {
	ZoneID        string
	APIToken      string
	HTTPClient    HTTPDoer
	MaxRetries    int
	BackoffConfig backoff.Config
}

// CloudflareCDNProvider invalidates objects via the Cloudflare purge-cache API.
type CloudflareCDNProvider struct {
	zoneID        string
	apiToken      string
	httpClient    HTTPDoer
	maxRetries    int
	backoffConfig backoff.Config
}

// NewCloudflareCDNProvider constructs a CloudflareCDNProvider with defaults.
func NewCloudflareCDNProvider(opts CloudflareCDNOptions) *CloudflareCDNProvider {
	httpClient := opts.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	maxRetries, backoffConfig := resolveCDNRetrySettings(opts.MaxRetries, opts.BackoffConfig)

	return &CloudflareCDNProvider{
		zoneID:        strings.TrimSpace(opts.ZoneID),
		apiToken:      strings.TrimSpace(opts.APIToken),
		httpClient:    httpClient,
		maxRetries:    maxRetries,
		backoffConfig: backoffConfig,
	}
}

// Name returns the provider name used for logging and diagnostics.
func (p *CloudflareCDNProvider) Name() string {
	return cloudflareProviderName
}

// PurgeURLs invalidates public URLs in Cloudflare at most 30 files per request.
func (p *CloudflareCDNProvider) PurgeURLs(ctx context.Context, publicURLs []string) error {
	urls := sanitizePublicURLs(publicURLs)
	if len(urls) == 0 {
		return nil
	}

	for _, chunk := range chunkStrings(urls, cloudflarePurgeURLsBatchLimit) {
		if err := p.purgeBatch(ctx, cloudflarePurgeRequest{Files: chunk}); err != nil {
			return err
		}
	}

	return nil
}

// PurgeAll purges the entire Cloudflare zone cache.
func (p *CloudflareCDNProvider) PurgeAll(ctx context.Context) error {
	return p.purgeBatch(ctx, cloudflarePurgeRequest{PurgeEverything: true})
}

// purgeBatch sends one purge request with shared retry behavior.
func (p *CloudflareCDNProvider) purgeBatch(ctx context.Context, request cloudflarePurgeRequest) error {
	if strings.TrimSpace(p.zoneID) == "" {
		return fmt.Errorf("cloudflare zone ID is required")
	}

	url := fmt.Sprintf(cloudflarePurgeURLTemplate, p.zoneID)

	return doWithRetry(
		ctx,
		p.maxRetries,
		p.backoffConfig,
		p.isRetryableCloudflareError,
		func(ctx context.Context) error {
			body, err := json.Marshal(request)
			if err != nil {
				return fmt.Errorf("marshal cloudflare purge request: %w", err)
			}

			req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
			if err != nil {
				return fmt.Errorf("create cloudflare request: %w", err)
			}

			req.Header.Set("Authorization", "Bearer "+p.apiToken)
			req.Header.Set("Content-Type", "application/json")

			resp, err := p.httpClient.Do(req)
			if err != nil {
				return err
			}

			bodyText, err := readResponseBody(resp)
			if err != nil {
				return fmt.Errorf("read cloudflare response body: %w", err)
			}

			if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
				return &responseError{statusCode: resp.StatusCode, body: bodyText}
			}

			var parsed cloudflarePurgeResponse
			if err := json.Unmarshal([]byte(bodyText), &parsed); err != nil {
				return fmt.Errorf("decode cloudflare response: %w", err)
			}

			if !parsed.Success {
				return fmt.Errorf("cloudflare purge rejected request: %s", strings.Join(cloudflarePurgeErrorMessages(parsed.Errors), "; "))
			}

			return nil
		},
	)
}

// isRetryableCloudflareError applies the shared provider retry policy.
func (p *CloudflareCDNProvider) isRetryableCloudflareError(err error) bool {
	return isRetryableHTTPAdapterError(err)
}

// cloudflarePurgeRequest is the request body for Cloudflare cache purge endpoints.
type cloudflarePurgeRequest struct {
	Files           []string `json:"files,omitempty"`
	PurgeEverything bool     `json:"purge_everything,omitempty"`
}

// cloudflarePurgeResponse is parsed when Cloudflare returns a JSON response.
type cloudflarePurgeResponse struct {
	Success bool                   `json:"success"`
	Errors  []cloudflarePurgeError `json:"errors,omitempty"`
}

type cloudflarePurgeError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func cloudflarePurgeErrorMessages(errs []cloudflarePurgeError) []string {
	messages := make([]string, 0, len(errs))
	for _, err := range errs {
		if err.Message != "" {
			messages = append(messages, err.Message)
			continue
		}
		messages = append(messages, fmt.Sprintf("code=%d", err.Code))
	}
	if len(messages) == 0 {
		return []string{"request failed"}
	}
	return messages
}

var _ CDNProvider = (*CloudflareCDNProvider)(nil)
