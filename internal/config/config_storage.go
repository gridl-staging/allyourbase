package config

import "strings"

// CDNConfig controls purge provider selection for storage CDN invalidation.
// Storage.CDNURL remains the only public URL rewrite setting.
type CDNConfig struct {
	Provider   string              `toml:"provider"`
	Cloudflare CDNCloudflareConfig `toml:"cloudflare"`
	CloudFront CDNCloudFrontConfig `toml:"cloudfront"`
	Webhook    CDNWebhookConfig    `toml:"webhook"`
}

// CDNCloudflareConfig stores Cloudflare purge credentials.
type CDNCloudflareConfig struct {
	ZoneID   string `toml:"zone_id"`
	APIToken string `toml:"api_token"`
}

// CDNCloudFrontConfig stores CloudFront purge target information.
type CDNCloudFrontConfig struct {
	DistributionID string `toml:"distribution_id"`
}

// CDNWebhookConfig stores webhook purge endpoint settings.
type CDNWebhookConfig struct {
	Endpoint      string `toml:"endpoint"`
	SigningSecret string `toml:"signing_secret"`
}

func (c CDNConfig) NormalizedProvider() string {
	return strings.ToLower(strings.TrimSpace(c.Provider))
}
