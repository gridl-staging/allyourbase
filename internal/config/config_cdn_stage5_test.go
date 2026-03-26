package config

import (
	"path/filepath"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func configureLocalStorage(cfg *Config) {
	cfg.Storage.Enabled = true
	cfg.Storage.Backend = "local"
	cfg.Storage.LocalPath = "/tmp/storage"
}

func assertConfigValue(t *testing.T, cfg *Config, key, want string) {
	t.Helper()

	got, err := GetValue(cfg, key)
	testutil.NoError(t, err)
	value, ok := got.(string)
	testutil.True(t, ok)
	testutil.Equal(t, want, value)
}

type configEntry struct {
	key   string
	value string
}

func writeConfigValues(t *testing.T, configPath string, entries ...configEntry) {
	t.Helper()

	for _, entry := range entries {
		testutil.NoError(t, SetValue(configPath, entry.key, entry.value))
	}
}

func TestParseTOMLStorageCDNProviderConfig(t *testing.T) {
	t.Parallel()

	cfg, err := ParseTOML([]byte(`
[storage]
enabled = true
backend = "local"
local_path = "/tmp/storage"
cdn_url = "https://cdn.example.com"

[storage.cdn]
provider = "cloudflare"

[storage.cdn.cloudflare]
zone_id = "zone-123"
api_token = "token-xyz"
`))
	testutil.NoError(t, err)
	testutil.Equal(t, "https://cdn.example.com", cfg.Storage.CDNURL)
	testutil.Equal(t, "cloudflare", cfg.Storage.CDN.Provider)
	testutil.Equal(t, "zone-123", cfg.Storage.CDN.Cloudflare.ZoneID)
	testutil.Equal(t, "token-xyz", cfg.Storage.CDN.Cloudflare.APIToken)
}

func TestApplyEnvStorageCDNProviderOverride(t *testing.T) {
	t.Setenv("AYB_STORAGE_CDN_PROVIDER", "webhook")
	t.Setenv("AYB_STORAGE_CDN_WEBHOOK_ENDPOINT", "https://purge.example.com/hook")
	t.Setenv("AYB_STORAGE_CDN_WEBHOOK_SIGNING_SECRET", "signing-secret")

	cfg := Default()
	err := applyEnv(cfg)
	testutil.NoError(t, err)
	testutil.Equal(t, "webhook", cfg.Storage.CDN.Provider)
	testutil.Equal(t, "https://purge.example.com/hook", cfg.Storage.CDN.Webhook.Endpoint)
	testutil.Equal(t, "signing-secret", cfg.Storage.CDN.Webhook.SigningSecret)
}

func TestValidateStorageCDNProviderRequiredFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		modify  func(*Config)
		wantErr string
	}{
		{
			name: "rewrite only no provider uses nop",
			modify: func(c *Config) {
				c.Storage.CDN.Provider = ""
				c.Storage.CDNURL = "https://cdn.example.com"
			},
		},
		{
			name: "invalid provider",
			modify: func(c *Config) {
				c.Storage.CDN.Provider = "fastly"
			},
			wantErr: "storage.cdn.provider must be one of",
		},
		{
			name: "provider requires cdn url",
			modify: func(c *Config) {
				c.Storage.CDN.Provider = "cloudflare"
				c.Storage.CDN.Cloudflare.ZoneID = "zone-123"
				c.Storage.CDN.Cloudflare.APIToken = "token-xyz"
			},
			wantErr: "storage.cdn_url is required",
		},
		{
			name: "cloudflare requires zone and token",
			modify: func(c *Config) {
				c.Storage.CDNURL = "https://cdn.example.com"
				c.Storage.CDN.Provider = "cloudflare"
				c.Storage.CDN.Cloudflare.ZoneID = ""
				c.Storage.CDN.Cloudflare.APIToken = ""
			},
			wantErr: "storage.cdn.cloudflare.zone_id is required",
		},
		{
			name: "cloudfront requires distribution id",
			modify: func(c *Config) {
				c.Storage.CDNURL = "https://cdn.example.com"
				c.Storage.CDN.Provider = "cloudfront"
				c.Storage.CDN.CloudFront.DistributionID = ""
			},
			wantErr: "storage.cdn.cloudfront.distribution_id is required",
		},
		{
			name: "webhook requires endpoint",
			modify: func(c *Config) {
				c.Storage.CDNURL = "https://cdn.example.com"
				c.Storage.CDN.Provider = "webhook"
				c.Storage.CDN.Webhook.Endpoint = ""
				c.Storage.CDN.Webhook.SigningSecret = "secret"
			},
			wantErr: "storage.cdn.webhook.endpoint is required",
		},
		{
			name: "webhook requires signing secret",
			modify: func(c *Config) {
				c.Storage.CDNURL = "https://cdn.example.com"
				c.Storage.CDN.Provider = "webhook"
				c.Storage.CDN.Webhook.Endpoint = "https://purge.example.com"
				c.Storage.CDN.Webhook.SigningSecret = ""
			},
			wantErr: "storage.cdn.webhook.signing_secret is required",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg := Default()
			configureLocalStorage(cfg)
			tt.modify(cfg)
			err := cfg.Validate()
			if tt.wantErr == "" {
				testutil.NoError(t, err)
				return
			}
			testutil.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestStorageCDNKeyPlumbing(t *testing.T) {
	t.Parallel()

	cfg := Default()
	cfg.Storage.CDN.Provider = "cloudflare"
	cfg.Storage.CDN.Cloudflare.ZoneID = "zone-1"
	cfg.Storage.CDN.Cloudflare.APIToken = "token-1"
	cfg.Storage.CDN.CloudFront.DistributionID = "dist-1"
	cfg.Storage.CDN.Webhook.Endpoint = "https://purge.example.com"
	cfg.Storage.CDN.Webhook.SigningSecret = "secret-1"

	testutil.True(t, IsValidKey("storage.cdn.provider"))
	testutil.True(t, IsValidKey("storage.cdn.cloudflare.zone_id"))
	testutil.True(t, IsValidKey("storage.cdn.cloudflare.api_token"))
	testutil.True(t, IsValidKey("storage.cdn.cloudfront.distribution_id"))
	testutil.True(t, IsValidKey("storage.cdn.webhook.endpoint"))
	testutil.True(t, IsValidKey("storage.cdn.webhook.signing_secret"))

	assertConfigValue(t, cfg, "storage.cdn.provider", "cloudflare")
	assertConfigValue(t, cfg, "storage.cdn.cloudflare.zone_id", "zone-1")
	assertConfigValue(t, cfg, "storage.cdn.cloudflare.api_token", "token-1")
	assertConfigValue(t, cfg, "storage.cdn.cloudfront.distribution_id", "dist-1")
	assertConfigValue(t, cfg, "storage.cdn.webhook.endpoint", "https://purge.example.com")
	assertConfigValue(t, cfg, "storage.cdn.webhook.signing_secret", "secret-1")
}

func TestSetValueStorageCDNNestedKeys(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "ayb.toml")

	writeConfigValues(t, configPath,
		configEntry{key: "storage.enabled", value: "true"},
		configEntry{key: "storage.backend", value: "local"},
		configEntry{key: "storage.local_path", value: "/tmp/storage"},
		configEntry{key: "storage.cdn_url", value: "https://cdn.example.com"},
		configEntry{key: "storage.cdn.provider", value: "webhook"},
		configEntry{key: "storage.cdn.webhook.endpoint", value: "https://purge.example.com/hook"},
		configEntry{key: "storage.cdn.webhook.signing_secret", value: "secret-1"},
	)

	cfg, err := Load(configPath, nil)
	testutil.NoError(t, err)
	testutil.Equal(t, "https://cdn.example.com", cfg.Storage.CDNURL)
	testutil.Equal(t, "webhook", cfg.Storage.CDN.Provider)
	testutil.Equal(t, "https://purge.example.com/hook", cfg.Storage.CDN.Webhook.Endpoint)
	testutil.Equal(t, "secret-1", cfg.Storage.CDN.Webhook.SigningSecret)
}
