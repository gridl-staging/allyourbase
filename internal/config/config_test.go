package config

import (
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestDefault(t *testing.T) {
	cfg := Default()

	testutil.Equal(t, "127.0.0.1", cfg.Server.Host)
	testutil.Equal(t, 8090, cfg.Server.Port)
	testutil.Equal(t, "1MB", cfg.Server.BodyLimit)
	testutil.Equal(t, 10, cfg.Server.ShutdownTimeout)
	testutil.SliceLen(t, cfg.Server.CORSAllowedOrigins, 1)
	testutil.Equal(t, "*", cfg.Server.CORSAllowedOrigins[0])
	testutil.SliceLen(t, cfg.Server.AllowedIPs, 0)

	testutil.Equal(t, 25, cfg.Database.MaxConns)
	testutil.Equal(t, 2, cfg.Database.MinConns)
	testutil.Equal(t, 30, cfg.Database.HealthCheckSecs)
	testutil.Equal(t, 15432, cfg.Database.EmbeddedPort)
	testutil.Equal(t, "", cfg.Database.EmbeddedDataDir)
	testutil.True(t, len(cfg.Database.Replicas) == 0)

	testutil.Equal(t, true, cfg.Admin.Enabled)
	testutil.Equal(t, "/admin", cfg.Admin.Path)
	testutil.SliceLen(t, cfg.Admin.AllowedIPs, 0)

	testutil.Equal(t, false, cfg.Auth.Enabled)
	testutil.Equal(t, "", cfg.Auth.JWTSecret)
	testutil.Equal(t, 900, cfg.Auth.TokenDuration)
	testutil.Equal(t, 604800, cfg.Auth.RefreshTokenDuration)
	testutil.Equal(t, 10, cfg.Auth.RateLimit)
	testutil.Equal(t, 30, cfg.Auth.AnonymousRateLimit)
	testutil.Equal(t, "10/min", cfg.Auth.RateLimitAuth)
	testutil.Equal(t, 8, cfg.Auth.MinPasswordLength)
	testutil.Equal(t, false, cfg.Auth.OAuthProviderMode.Enabled)
	testutil.Equal(t, 3600, cfg.Auth.OAuthProviderMode.AccessTokenDuration)
	testutil.Equal(t, 2592000, cfg.Auth.OAuthProviderMode.RefreshTokenDuration)
	testutil.Equal(t, 600, cfg.Auth.OAuthProviderMode.AuthCodeDuration)

	testutil.Equal(t, "log", cfg.Email.Backend)
	testutil.Equal(t, "Allyourbase", cfg.Email.FromName)
	testutil.Equal(t, "", cfg.Email.From)

	testutil.Equal(t, false, cfg.Storage.Enabled)
	testutil.Equal(t, "local", cfg.Storage.Backend)
	testutil.Equal(t, "./ayb_storage", cfg.Storage.LocalPath)
	testutil.Equal(t, "10MB", cfg.Storage.MaxFileSize)
	testutil.Equal(t, "us-east-1", cfg.Storage.S3Region)
	testutil.Equal(t, true, cfg.Storage.S3UseSSL)

	testutil.Equal(t, "./migrations", cfg.Database.MigrationsDir)

	testutil.Equal(t, "info", cfg.Logging.Level)
	testutil.Equal(t, "json", cfg.Logging.Format)
	testutil.Equal(t, true, cfg.Metrics.Enabled)
	testutil.Equal(t, "/metrics", cfg.Metrics.Path)
	testutil.Equal(t, "", cfg.Metrics.AuthToken)
	testutil.Equal(t, 100, cfg.Realtime.MaxConnectionsPerUser)
	testutil.Equal(t, 25, cfg.Realtime.HeartbeatIntervalSeconds)
	testutil.Equal(t, 100, cfg.Realtime.BroadcastRateLimitPerSecond)
	testutil.Equal(t, 262144, cfg.Realtime.BroadcastMaxMessageBytes)
	testutil.Equal(t, 10, cfg.Realtime.PresenceLeaveTimeoutSeconds)
	testutil.Equal(t, 50, cfg.API.ImportMaxSizeMB)
	testutil.Equal(t, 100000, cfg.API.ImportMaxRows)
	testutil.Equal(t, 1000000, cfg.API.ExportMaxRows)
	testutil.Equal(t, true, cfg.API.AggregateEnabled)

	testutil.Equal(t, "", cfg.Billing.Provider)
	testutil.Equal(t, 3600, cfg.Billing.UsageSyncIntervalSecs)
	testutil.Equal(t, "", cfg.Billing.StripeSecretKey)
	testutil.Equal(t, "", cfg.Billing.StripeWebhookSecret)
	testutil.Equal(t, "", cfg.Billing.StripeStarterPriceID)
	testutil.Equal(t, "", cfg.Billing.StripeProPriceID)
	testutil.Equal(t, "", cfg.Billing.StripeEnterprisePriceID)
	testutil.Equal(t, false, cfg.DashboardAI.Enabled)
	testutil.Equal(t, "20/min", cfg.DashboardAI.RateLimit)
	testutil.Equal(t, false, cfg.Status.Enabled)
	testutil.Equal(t, 30, cfg.Status.CheckIntervalSeconds)
	testutil.Equal(t, 1000, cfg.Status.HistorySize)
	testutil.Equal(t, true, cfg.Status.PublicEndpointEnabled)

	testutil.Equal(t, 12, cfg.EdgeFunctions.PoolSize)
	testutil.Equal(t, 5000, cfg.EdgeFunctions.DefaultTimeoutMs)
	testutil.Equal(t, int64(1<<20), cfg.EdgeFunctions.MaxRequestBodyBytes)
	testutil.SliceLen(t, cfg.EdgeFunctions.FetchDomainAllowlist, 0)
}

func TestStatusConfigParseOverrides(t *testing.T) {
	cfg, err := ParseTOML([]byte(`
[status]
enabled = true
check_interval_seconds = 45
history_size = 250
public_endpoint_enabled = false
`))
	testutil.NoError(t, err)
	testutil.Equal(t, true, cfg.Status.Enabled)
	testutil.Equal(t, 45, cfg.Status.CheckIntervalSeconds)
	testutil.Equal(t, 250, cfg.Status.HistorySize)
	testutil.Equal(t, false, cfg.Status.PublicEndpointEnabled)
}

func TestAddress(t *testing.T) {
	tests := []struct {
		name string
		host string
		port int
		want string
	}{
		{name: "default", host: "127.0.0.1", port: 8090, want: "127.0.0.1:8090"},
		{name: "localhost", host: "127.0.0.1", port: 3000, want: "127.0.0.1:3000"},
		{name: "custom host", host: "myserver.local", port: 443, want: "myserver.local:443"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{Server: ServerConfig{Host: tt.host, Port: tt.port}}
			testutil.Equal(t, tt.want, cfg.Address())
		})
	}
}

func TestPublicBaseURL(t *testing.T) {
	tests := []struct {
		name    string
		host    string
		port    int
		siteURL string
		want    string
	}{
		{name: "default replaces 0.0.0.0", host: "0.0.0.0", port: 8090, want: "http://localhost:8090"},
		{name: "empty host uses localhost", host: "", port: 8090, want: "http://localhost:8090"},
		{name: "custom host preserved", host: "myserver.local", port: 3000, want: "http://myserver.local:3000"},
		{name: "site_url overrides", host: "0.0.0.0", port: 8090, siteURL: "https://myapp.example.com", want: "https://myapp.example.com"},
		{name: "site_url trailing slash stripped", host: "0.0.0.0", port: 8090, siteURL: "https://myapp.example.com/", want: "https://myapp.example.com"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{Server: ServerConfig{Host: tt.host, Port: tt.port, SiteURL: tt.siteURL}}
			testutil.Equal(t, tt.want, cfg.PublicBaseURL())
		})
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		modify  func(*Config)
		wantErr string
	}{
		{
			name:   "valid defaults",
			modify: func(c *Config) {},
		},
		{
			name:    "port zero",
			modify:  func(c *Config) { c.Server.Port = 0 },
			wantErr: "server.port must be between 1 and 65535",
		},
		{
			name:    "port negative",
			modify:  func(c *Config) { c.Server.Port = -1 },
			wantErr: "server.port must be between 1 and 65535",
		},
		{
			name:    "port too high",
			modify:  func(c *Config) { c.Server.Port = 70000 },
			wantErr: "server.port must be between 1 and 65535",
		},
		{
			name:   "port 1 valid",
			modify: func(c *Config) { c.Server.Port = 1 },
		},
		{
			name:   "port 65535 valid",
			modify: func(c *Config) { c.Server.Port = 65535 },
		},
		{
			name:    "max_conns zero",
			modify:  func(c *Config) { c.Database.MaxConns = 0 },
			wantErr: "database.max_conns must be at least 1",
		},
		{
			name:    "min_conns negative",
			modify:  func(c *Config) { c.Database.MinConns = -1 },
			wantErr: "database.min_conns must be non-negative",
		},
		{
			name: "replica empty url",
			modify: func(c *Config) {
				c.Database.Replicas = []ReplicaConfig{{URL: "", Weight: 1, MaxLagBytes: 1}}
			},
			wantErr: "database.replicas[0].url must not be empty",
		},
		{
			name: "replica weight too low",
			modify: func(c *Config) {
				c.Database.Replicas = []ReplicaConfig{{URL: "postgresql://replica-1/db", Weight: 0, MaxLagBytes: 1}}
			},
			wantErr: "database.replicas[0].weight must be at least 1",
		},
		{
			name: "replica max lag negative",
			modify: func(c *Config) {
				c.Database.Replicas = []ReplicaConfig{{URL: "postgresql://replica-1/db", Weight: 1, MaxLagBytes: -1}}
			},
			wantErr: "database.replicas[0].max_lag_bytes must be non-negative",
		},
		{
			name: "min_conns exceeds max_conns",
			modify: func(c *Config) {
				c.Database.MaxConns = 5
				c.Database.MinConns = 10
			},
			wantErr: "database.min_conns (10) cannot exceed database.max_conns (5)",
		},
		{
			name:   "min_conns equals max_conns",
			modify: func(c *Config) { c.Database.MinConns = 25 },
		},
		{
			name:    "invalid log level",
			modify:  func(c *Config) { c.Logging.Level = "trace" },
			wantErr: `logging.level must be one of`,
		},
		{
			name: "invalid metrics path without leading slash",
			modify: func(c *Config) {
				c.Metrics.Path = "metrics"
			},
			wantErr: "metrics.path must start with /",
		},
		{
			name: "invalid request log batch size",
			modify: func(c *Config) {
				c.Logging.RequestLogBatchSize = 0
			},
			wantErr: "logging.request_log_batch_size must be at least 1",
		},
		{
			name: "invalid request log flush interval",
			modify: func(c *Config) {
				c.Logging.RequestLogFlushIntervalSecs = 0
			},
			wantErr: "logging.request_log_flush_interval_seconds must be at least 1",
		},
		{
			name: "invalid request log queue size",
			modify: func(c *Config) {
				c.Logging.RequestLogQueueSize = -1
			},
			wantErr: "logging.request_log_queue_size must be at least 1",
		},
		{
			name: "valid log drain config",
			modify: func(c *Config) {
				c.Logging.Drains = []LogDrainConfig{{Type: "http", URL: "https://logs.example.com/ingest", ID: "drain-1", BatchSize: 100, FlushIntervalSecs: 5}}
			},
		},
		{
			name: "invalid log drain type",
			modify: func(c *Config) {
				c.Logging.Drains = []LogDrainConfig{{Type: "s3", URL: "https://example.com"}}
			},
			wantErr: "logging.drains[0].type must be http, datadog, or loki",
		},
		{
			name: "missing log drain URL",
			modify: func(c *Config) {
				c.Logging.Drains = []LogDrainConfig{{Type: "http"}}
			},
			wantErr: "logging.drains[0].url is required",
		},
		{
			name: "negative log drain batch_size",
			modify: func(c *Config) {
				c.Logging.Drains = []LogDrainConfig{{Type: "http", URL: "https://example.com", BatchSize: -10}}
			},
			wantErr: "logging.drains[0].batch_size must be non-negative",
		},
		{
			name: "negative log drain flush interval",
			modify: func(c *Config) {
				c.Logging.Drains = []LogDrainConfig{{Type: "http", URL: "https://example.com", FlushIntervalSecs: -1}}
			},
			wantErr: "logging.drains[0].flush_interval_seconds must be non-negative",
		},
		{
			name: "disabled log drain is valid",
			modify: func(c *Config) {
				c.Logging.Drains = []LogDrainConfig{{Type: "http", URL: "https://example.com", Enabled: boolPtr(false)}}
			},
		},
		{
			name:   "debug log level",
			modify: func(c *Config) { c.Logging.Level = "debug" },
		},
		{
			name:   "billing disabled is valid",
			modify: func(c *Config) { c.Billing.Provider = "" },
		},
		{
			name: "billing provider must be stripe or empty",
			modify: func(c *Config) {
				c.Billing.Provider = "paypal"
			},
			wantErr: `billing.provider must be empty or "stripe", got "paypal"`,
		},
		{
			name: "stripe requires secret key",
			modify: func(c *Config) {
				c.Billing.Provider = "stripe"
				c.Billing.StripeWebhookSecret = "whsec_123"
				c.Billing.StripeStarterPriceID = "price_starter"
				c.Billing.StripeProPriceID = "price_pro"
				c.Billing.StripeEnterprisePriceID = "price_enterprise"
			},
			wantErr: "billing.stripe_secret_key is required when billing.provider = stripe",
		},
		{
			name: "stripe requires webhook secret",
			modify: func(c *Config) {
				c.Billing.Provider = "stripe"
				c.Billing.StripeSecretKey = "sk_test_123"
				c.Billing.StripeStarterPriceID = "price_starter"
				c.Billing.StripeProPriceID = "price_pro"
				c.Billing.StripeEnterprisePriceID = "price_enterprise"
			},
			wantErr: "billing.stripe_webhook_secret is required when billing.provider = stripe",
		},
		{
			name: "stripe requires starter price",
			modify: func(c *Config) {
				c.Billing.Provider = "stripe"
				c.Billing.StripeSecretKey = "sk_test_123"
				c.Billing.StripeWebhookSecret = "whsec_123"
				c.Billing.StripeProPriceID = "price_pro"
				c.Billing.StripeEnterprisePriceID = "price_enterprise"
			},
			wantErr: "billing.stripe_starter_price_id is required when billing.provider = stripe",
		},
		{
			name: "stripe requires pro price",
			modify: func(c *Config) {
				c.Billing.Provider = "stripe"
				c.Billing.StripeSecretKey = "sk_test_123"
				c.Billing.StripeWebhookSecret = "whsec_123"
				c.Billing.StripeStarterPriceID = "price_starter"
				c.Billing.StripeEnterprisePriceID = "price_enterprise"
			},
			wantErr: "billing.stripe_pro_price_id is required when billing.provider = stripe",
		},
		{
			name: "stripe requires enterprise price",
			modify: func(c *Config) {
				c.Billing.Provider = "stripe"
				c.Billing.StripeSecretKey = "sk_test_123"
				c.Billing.StripeWebhookSecret = "whsec_123"
				c.Billing.StripeStarterPriceID = "price_starter"
				c.Billing.StripeProPriceID = "price_pro"
			},
			wantErr: "billing.stripe_enterprise_price_id is required when billing.provider = stripe",
		},
		{
			name: "stripe with required credentials is valid",
			modify: func(c *Config) {
				c.Billing.Provider = "stripe"
				c.Billing.StripeSecretKey = "sk_test_123"
				c.Billing.StripeWebhookSecret = "whsec_123"
				c.Billing.StripeStarterPriceID = "price_starter"
				c.Billing.StripeProPriceID = "price_pro"
				c.Billing.StripeEnterprisePriceID = "price_enterprise"
				c.Billing.StripeMeterAPIRequests = "meter.api_requests"
				c.Billing.StripeMeterStorageBytes = "meter.storage_bytes"
				c.Billing.StripeMeterBandwidthBytes = "meter.bandwidth_bytes"
				c.Billing.StripeMeterFunctionInvs = "meter.function_invocations"
			},
		},
		{
			name: "stripe requires meter api requests event name",
			modify: func(c *Config) {
				c.Billing.Provider = "stripe"
				c.Billing.StripeSecretKey = "sk_test_123"
				c.Billing.StripeWebhookSecret = "whsec_123"
				c.Billing.StripeStarterPriceID = "price_starter"
				c.Billing.StripeProPriceID = "price_pro"
				c.Billing.StripeEnterprisePriceID = "price_enterprise"
				c.Billing.StripeMeterStorageBytes = "meter.storage_bytes"
				c.Billing.StripeMeterBandwidthBytes = "meter.bandwidth_bytes"
				c.Billing.StripeMeterFunctionInvs = "meter.function_invocations"
			},
			wantErr: "billing.stripe_meter_api_requests is required when billing.provider = stripe",
		},
		{
			name: "stripe requires meter storage bytes event name",
			modify: func(c *Config) {
				c.Billing.Provider = "stripe"
				c.Billing.StripeSecretKey = "sk_test_123"
				c.Billing.StripeWebhookSecret = "whsec_123"
				c.Billing.StripeStarterPriceID = "price_starter"
				c.Billing.StripeProPriceID = "price_pro"
				c.Billing.StripeEnterprisePriceID = "price_enterprise"
				c.Billing.StripeMeterAPIRequests = "meter.api_requests"
				c.Billing.StripeMeterBandwidthBytes = "meter.bandwidth_bytes"
				c.Billing.StripeMeterFunctionInvs = "meter.function_invocations"
			},
			wantErr: "billing.stripe_meter_storage_bytes is required when billing.provider = stripe",
		},
		{
			name: "stripe requires meter bandwidth bytes event name",
			modify: func(c *Config) {
				c.Billing.Provider = "stripe"
				c.Billing.StripeSecretKey = "sk_test_123"
				c.Billing.StripeWebhookSecret = "whsec_123"
				c.Billing.StripeStarterPriceID = "price_starter"
				c.Billing.StripeProPriceID = "price_pro"
				c.Billing.StripeEnterprisePriceID = "price_enterprise"
				c.Billing.StripeMeterAPIRequests = "meter.api_requests"
				c.Billing.StripeMeterStorageBytes = "meter.storage_bytes"
				c.Billing.StripeMeterFunctionInvs = "meter.function_invocations"
			},
			wantErr: "billing.stripe_meter_bandwidth_bytes is required when billing.provider = stripe",
		},
		{
			name: "stripe requires meter function invocations event name",
			modify: func(c *Config) {
				c.Billing.Provider = "stripe"
				c.Billing.StripeSecretKey = "sk_test_123"
				c.Billing.StripeWebhookSecret = "whsec_123"
				c.Billing.StripeStarterPriceID = "price_starter"
				c.Billing.StripeProPriceID = "price_pro"
				c.Billing.StripeEnterprisePriceID = "price_enterprise"
				c.Billing.StripeMeterAPIRequests = "meter.api_requests"
				c.Billing.StripeMeterStorageBytes = "meter.storage_bytes"
				c.Billing.StripeMeterBandwidthBytes = "meter.bandwidth_bytes"
			},
			wantErr: "billing.stripe_meter_function_invocations is required when billing.provider = stripe",
		},
		{
			name:    "invalid server allowed ip",
			modify:  func(c *Config) { c.Server.AllowedIPs = []string{"not-an-ip"} },
			wantErr: `invalid server.allowed_ips entry`,
		},
		{
			name:    "invalid admin allowed ip",
			modify:  func(c *Config) { c.Admin.AllowedIPs = []string{"300.0.0.1"} },
			wantErr: `invalid admin.allowed_ips entry`,
		},
		{
			name:   "warn log level",
			modify: func(c *Config) { c.Logging.Level = "warn" },
		},
		{
			name:   "error log level",
			modify: func(c *Config) { c.Logging.Level = "error" },
		},
		{
			name:    "min_password_length zero",
			modify:  func(c *Config) { c.Auth.MinPasswordLength = 0 },
			wantErr: "auth.min_password_length must be at least 1",
		},
		{
			name:    "min_password_length negative",
			modify:  func(c *Config) { c.Auth.MinPasswordLength = -5 },
			wantErr: "auth.min_password_length must be at least 1",
		},
		{
			name:   "min_password_length 1 valid",
			modify: func(c *Config) { c.Auth.MinPasswordLength = 1 },
		},
		{
			name:   "min_password_length 6 valid",
			modify: func(c *Config) { c.Auth.MinPasswordLength = 6 },
		},
		{
			name: "auth enabled without secret",
			modify: func(c *Config) {
				c.Auth.Enabled = true
				c.Auth.JWTSecret = ""
			},
			wantErr: "auth.jwt_secret is required when auth is enabled",
		},
		{
			name: "auth secret too short",
			modify: func(c *Config) {
				c.Auth.JWTSecret = "tooshort"
			},
			wantErr: "auth.jwt_secret must be at least 32 characters",
		},
		{
			name: "auth enabled with valid secret",
			modify: func(c *Config) {
				c.Auth.Enabled = true
				c.Auth.JWTSecret = "this-is-a-secret-that-is-at-least-32-characters-long"
			},
		},
		{
			name:   "auth disabled without secret is fine",
			modify: func(c *Config) { c.Auth.Enabled = false },
		},
		{
			name: "oauth enabled without auth enabled",
			modify: func(c *Config) {
				c.Auth.Enabled = false
				c.Auth.OAuth = map[string]OAuthProvider{
					"google": {Enabled: true, ClientID: "id", ClientSecret: "secret"},
				}
			},
			wantErr: "auth.enabled must be true to use OAuth provider",
		},
		{
			name: "oauth enabled without client_id",
			modify: func(c *Config) {
				c.Auth.Enabled = true
				c.Auth.JWTSecret = "this-is-a-secret-that-is-at-least-32-characters-long"
				c.Auth.OAuth = map[string]OAuthProvider{
					"google": {Enabled: true, ClientID: "", ClientSecret: "secret"},
				}
			},
			wantErr: "client_id is required",
		},
		{
			name: "oauth enabled without client_secret",
			modify: func(c *Config) {
				c.Auth.Enabled = true
				c.Auth.JWTSecret = "this-is-a-secret-that-is-at-least-32-characters-long"
				c.Auth.OAuth = map[string]OAuthProvider{
					"github": {Enabled: true, ClientID: "id", ClientSecret: ""},
				}
			},
			wantErr: "client_secret is required",
		},
		{
			name: "unsupported oauth provider",
			modify: func(c *Config) {
				c.Auth.Enabled = true
				c.Auth.JWTSecret = "this-is-a-secret-that-is-at-least-32-characters-long"
				c.Auth.OAuth = map[string]OAuthProvider{
					"myspace": {Enabled: true, ClientID: "id", ClientSecret: "secret"},
				}
			},
			wantErr: "unsupported OAuth provider",
		},
		{
			name: "valid oauth config",
			modify: func(c *Config) {
				c.Auth.Enabled = true
				c.Auth.JWTSecret = "this-is-a-secret-that-is-at-least-32-characters-long"
				c.Auth.OAuth = map[string]OAuthProvider{
					"google":    {Enabled: true, ClientID: "id", ClientSecret: "secret"},
					"github":    {Enabled: true, ClientID: "id2", ClientSecret: "secret2"},
					"microsoft": {Enabled: true, ClientID: "id3", ClientSecret: "secret3"},
				}
			},
		},
		{
			name: "valid apple oauth config",
			modify: func(c *Config) {
				c.Auth.Enabled = true
				c.Auth.JWTSecret = "this-is-a-secret-that-is-at-least-32-characters-long"
				c.Auth.OAuth = map[string]OAuthProvider{
					"apple": {Enabled: true, ClientID: "com.example.app", TeamID: "TEAM123456", KeyID: "KEY123", PrivateKey: "-----BEGIN EC PRIVATE KEY-----\nfake\n-----END EC PRIVATE KEY-----"},
				}
			},
		},
		{
			name: "apple oauth missing team_id",
			modify: func(c *Config) {
				c.Auth.Enabled = true
				c.Auth.JWTSecret = "this-is-a-secret-that-is-at-least-32-characters-long"
				c.Auth.OAuth = map[string]OAuthProvider{
					"apple": {Enabled: true, ClientID: "com.example.app", KeyID: "KEY123", PrivateKey: "pem"},
				}
			},
			wantErr: "team_id is required",
		},
		{
			name: "apple oauth missing key_id",
			modify: func(c *Config) {
				c.Auth.Enabled = true
				c.Auth.JWTSecret = "this-is-a-secret-that-is-at-least-32-characters-long"
				c.Auth.OAuth = map[string]OAuthProvider{
					"apple": {Enabled: true, ClientID: "com.example.app", TeamID: "TEAM123456", PrivateKey: "pem"},
				}
			},
			wantErr: "key_id is required",
		},
		{
			name: "apple oauth missing private_key",
			modify: func(c *Config) {
				c.Auth.Enabled = true
				c.Auth.JWTSecret = "this-is-a-secret-that-is-at-least-32-characters-long"
				c.Auth.OAuth = map[string]OAuthProvider{
					"apple": {Enabled: true, ClientID: "com.example.app", TeamID: "TEAM123456", KeyID: "KEY123"},
				}
			},
			wantErr: "private_key is required",
		},
		{
			name: "apple oauth does not require client_secret",
			modify: func(c *Config) {
				c.Auth.Enabled = true
				c.Auth.JWTSecret = "this-is-a-secret-that-is-at-least-32-characters-long"
				c.Auth.OAuth = map[string]OAuthProvider{
					"apple": {Enabled: true, ClientID: "com.example.app", TeamID: "T", KeyID: "K", PrivateKey: "P"},
				}
			},
		},
		{
			name: "disabled oauth provider doesn't need credentials",
			modify: func(c *Config) {
				c.Auth.OAuth = map[string]OAuthProvider{
					"google": {Enabled: false},
				}
			},
		},
		{
			name: "valid oidc config",
			modify: func(c *Config) {
				c.Auth.Enabled = true
				c.Auth.JWTSecret = "this-is-a-secret-that-is-at-least-32-characters-long"
				c.Auth.OIDC = map[string]OIDCProvider{
					"keycloak": {Enabled: true, IssuerURL: "https://kc.example.com/realms/test", ClientID: "kc-id", ClientSecret: "kc-secret"},
				}
			},
		},
		{
			name: "oidc enabled without auth enabled",
			modify: func(c *Config) {
				c.Auth.Enabled = false
				c.Auth.OIDC = map[string]OIDCProvider{
					"keycloak": {Enabled: true, IssuerURL: "https://kc.example.com", ClientID: "id", ClientSecret: "secret"},
				}
			},
			wantErr: "auth.enabled must be true to use OIDC provider",
		},
		{
			name: "oidc missing issuer_url",
			modify: func(c *Config) {
				c.Auth.Enabled = true
				c.Auth.JWTSecret = "this-is-a-secret-that-is-at-least-32-characters-long"
				c.Auth.OIDC = map[string]OIDCProvider{
					"keycloak": {Enabled: true, ClientID: "id", ClientSecret: "secret"},
				}
			},
			wantErr: "issuer_url is required",
		},
		{
			name: "oidc missing client_id",
			modify: func(c *Config) {
				c.Auth.Enabled = true
				c.Auth.JWTSecret = "this-is-a-secret-that-is-at-least-32-characters-long"
				c.Auth.OIDC = map[string]OIDCProvider{
					"keycloak": {Enabled: true, IssuerURL: "https://kc.example.com", ClientSecret: "secret"},
				}
			},
			wantErr: "client_id is required",
		},
		{
			name: "oidc missing client_secret",
			modify: func(c *Config) {
				c.Auth.Enabled = true
				c.Auth.JWTSecret = "this-is-a-secret-that-is-at-least-32-characters-long"
				c.Auth.OIDC = map[string]OIDCProvider{
					"auth0": {Enabled: true, IssuerURL: "https://auth0.example.com", ClientID: "id"},
				}
			},
			wantErr: "client_secret is required",
		},
		{
			name: "oidc name conflicts with built-in provider",
			modify: func(c *Config) {
				c.Auth.Enabled = true
				c.Auth.JWTSecret = "this-is-a-secret-that-is-at-least-32-characters-long"
				c.Auth.OIDC = map[string]OIDCProvider{
					"google": {Enabled: true, IssuerURL: "https://accounts.google.com", ClientID: "id", ClientSecret: "secret"},
				}
			},
			wantErr: "conflicts with built-in OAuth provider",
		},
		{
			name: "multiple oidc providers valid",
			modify: func(c *Config) {
				c.Auth.Enabled = true
				c.Auth.JWTSecret = "this-is-a-secret-that-is-at-least-32-characters-long"
				c.Auth.OIDC = map[string]OIDCProvider{
					"keycloak": {Enabled: true, IssuerURL: "https://kc.example.com", ClientID: "kc-id", ClientSecret: "kc-secret"},
					"auth0":    {Enabled: true, IssuerURL: "https://auth0.example.com", ClientID: "a0-id", ClientSecret: "a0-secret"},
				}
			},
		},
		{
			name: "disabled oidc provider doesn't need credentials",
			modify: func(c *Config) {
				c.Auth.OIDC = map[string]OIDCProvider{
					"keycloak": {Enabled: false},
				}
			},
		},
		{
			name: "saml provider requires auth enabled",
			modify: func(c *Config) {
				c.Auth.Enabled = false
				c.Auth.SAMLProviders = []SAMLProvider{
					{Enabled: true, Name: "okta", EntityID: "https://sp.example.com", IDPMetadataURL: "https://idp.example.com/metadata"},
				}
			},
			wantErr: "auth.enabled must be true to use SAML provider",
		},
		{
			name: "saml provider requires name",
			modify: func(c *Config) {
				c.Auth.Enabled = true
				c.Auth.JWTSecret = "this-is-a-secret-that-is-at-least-32-characters-long"
				c.Auth.SAMLProviders = []SAMLProvider{
					{Enabled: true, EntityID: "https://sp.example.com", IDPMetadataURL: "https://idp.example.com/metadata"},
				}
			},
			wantErr: "auth.saml_providers[0].name is required",
		},
		{
			name: "saml provider requires entity_id",
			modify: func(c *Config) {
				c.Auth.Enabled = true
				c.Auth.JWTSecret = "this-is-a-secret-that-is-at-least-32-characters-long"
				c.Auth.SAMLProviders = []SAMLProvider{
					{Enabled: true, Name: "okta", IDPMetadataURL: "https://idp.example.com/metadata"},
				}
			},
			wantErr: "auth.saml_providers[0].entity_id is required",
		},
		{
			name: "saml provider requires metadata source",
			modify: func(c *Config) {
				c.Auth.Enabled = true
				c.Auth.JWTSecret = "this-is-a-secret-that-is-at-least-32-characters-long"
				c.Auth.SAMLProviders = []SAMLProvider{
					{Enabled: true, Name: "okta", EntityID: "https://sp.example.com"},
				}
			},
			wantErr: "auth.saml_providers[0] requires idp_metadata_url or idp_metadata_xml",
		},
		{
			name: "valid saml provider config",
			modify: func(c *Config) {
				c.Auth.Enabled = true
				c.Auth.JWTSecret = "this-is-a-secret-that-is-at-least-32-characters-long"
				c.Auth.SAMLProviders = []SAMLProvider{
					{
						Enabled:          true,
						Name:             "okta",
						EntityID:         "https://sp.example.com",
						IDPMetadataURL:   "https://idp.example.com/metadata",
						AttributeMapping: map[string]string{"email": "mail", "name": "displayName", "groups": "groups"},
						SPCertFile:       "/tmp/sp-cert.pem",
						SPKeyFile:        "/tmp/sp-key.pem",
					},
				}
			},
		},
		{
			name: "saml provider duplicate name rejected",
			modify: func(c *Config) {
				c.Auth.Enabled = true
				c.Auth.JWTSecret = "this-is-a-secret-that-is-at-least-32-characters-long"
				c.Auth.SAMLProviders = []SAMLProvider{
					{Enabled: true, Name: "okta", EntityID: "https://sp.example.com", IDPMetadataURL: "https://idp.example.com/metadata"},
					{Enabled: true, Name: "okta", EntityID: "https://sp2.example.com", IDPMetadataURL: "https://idp2.example.com/metadata"},
				}
			},
			wantErr: `auth.saml_providers[1].name "okta" is duplicated`,
		},
		{
			name: "oauth provider mode enabled requires auth enabled",
			modify: func(c *Config) {
				c.Auth.Enabled = false
				c.Auth.OAuthProviderMode.Enabled = true
			},
			wantErr: "auth.enabled must be true to use OAuth provider mode",
		},
		{
			name: "oauth provider mode rejects non-positive access token duration",
			modify: func(c *Config) {
				c.Auth.Enabled = true
				c.Auth.JWTSecret = "this-is-a-secret-that-is-at-least-32-characters-long"
				c.Auth.OAuthProviderMode.Enabled = true
				c.Auth.OAuthProviderMode.AccessTokenDuration = 0
			},
			wantErr: "auth.oauth_provider.access_token_duration must be at least 1",
		},
		{
			name: "oauth provider mode rejects non-positive refresh token duration",
			modify: func(c *Config) {
				c.Auth.Enabled = true
				c.Auth.JWTSecret = "this-is-a-secret-that-is-at-least-32-characters-long"
				c.Auth.OAuthProviderMode.Enabled = true
				c.Auth.OAuthProviderMode.RefreshTokenDuration = 0
			},
			wantErr: "auth.oauth_provider.refresh_token_duration must be at least 1",
		},
		{
			name: "oauth provider mode rejects non-positive auth code duration",
			modify: func(c *Config) {
				c.Auth.Enabled = true
				c.Auth.JWTSecret = "this-is-a-secret-that-is-at-least-32-characters-long"
				c.Auth.OAuthProviderMode.Enabled = true
				c.Auth.OAuthProviderMode.AuthCodeDuration = 0
			},
			wantErr: "auth.oauth_provider.auth_code_duration must be at least 1",
		},
		{
			name: "oauth provider mode accepts positive durations",
			modify: func(c *Config) {
				c.Auth.Enabled = true
				c.Auth.JWTSecret = "this-is-a-secret-that-is-at-least-32-characters-long"
				c.Auth.OAuthProviderMode.Enabled = true
				c.Auth.OAuthProviderMode.AccessTokenDuration = 1800
				c.Auth.OAuthProviderMode.RefreshTokenDuration = 1209600
				c.Auth.OAuthProviderMode.AuthCodeDuration = 300
			},
		},
		{
			name: "magic link enabled without auth enabled",
			modify: func(c *Config) {
				c.Auth.Enabled = false
				c.Auth.MagicLinkEnabled = true
			},
			wantErr: "auth.enabled must be true to use magic link",
		},
		{
			name: "magic link enabled with auth enabled",
			modify: func(c *Config) {
				c.Auth.Enabled = true
				c.Auth.JWTSecret = "this-is-a-secret-that-is-at-least-32-characters-long"
				c.Auth.MagicLinkEnabled = true
			},
		},
		{
			name: "magic link disabled is fine",
			modify: func(c *Config) {
				c.Auth.MagicLinkEnabled = false
			},
		},
		{
			name:   "email log backend valid",
			modify: func(c *Config) { c.Email.Backend = "log" },
		},
		{
			name:   "email empty backend valid (defaults to log)",
			modify: func(c *Config) { c.Email.Backend = "" },
		},
		{
			name: "email smtp valid",
			modify: func(c *Config) {
				c.Email.Backend = "smtp"
				c.Email.SMTP.Host = "smtp.resend.com"
				c.Email.From = "noreply@example.com"
			},
		},
		{
			name: "email smtp missing host",
			modify: func(c *Config) {
				c.Email.Backend = "smtp"
				c.Email.From = "noreply@example.com"
			},
			wantErr: "email.smtp.host is required",
		},
		{
			name: "email smtp missing from",
			modify: func(c *Config) {
				c.Email.Backend = "smtp"
				c.Email.SMTP.Host = "smtp.resend.com"
			},
			wantErr: "email.from is required",
		},
		{
			name: "email webhook valid",
			modify: func(c *Config) {
				c.Email.Backend = "webhook"
				c.Email.Webhook.URL = "https://example.com/webhook"
			},
		},
		{
			name: "email webhook missing url",
			modify: func(c *Config) {
				c.Email.Backend = "webhook"
			},
			wantErr: "email.webhook.url is required",
		},
		{
			name:    "email invalid backend",
			modify:  func(c *Config) { c.Email.Backend = "sendgrid" },
			wantErr: `email.backend must be "log", "smtp", or "webhook"`,
		},
		{
			name: "storage enabled with local backend",
			modify: func(c *Config) {
				c.Storage.Enabled = true
				c.Storage.Backend = "local"
				c.Storage.LocalPath = "/tmp/storage"
			},
		},
		{
			name: "storage enabled with empty local path",
			modify: func(c *Config) {
				c.Storage.Enabled = true
				c.Storage.Backend = "local"
				c.Storage.LocalPath = ""
			},
			wantErr: "storage.local_path is required",
		},
		{
			name: "storage s3 backend valid",
			modify: func(c *Config) {
				c.Storage.Enabled = true
				c.Storage.Backend = "s3"
				c.Storage.S3Endpoint = "s3.amazonaws.com"
				c.Storage.S3Bucket = "my-bucket"
				c.Storage.S3AccessKey = "AKID"
				c.Storage.S3SecretKey = "secret"
			},
		},
		{
			name: "storage s3 missing endpoint",
			modify: func(c *Config) {
				c.Storage.Enabled = true
				c.Storage.Backend = "s3"
				c.Storage.S3Bucket = "my-bucket"
				c.Storage.S3AccessKey = "AKID"
				c.Storage.S3SecretKey = "secret"
			},
			wantErr: "s3_endpoint is required",
		},
		{
			name: "storage s3 missing bucket",
			modify: func(c *Config) {
				c.Storage.Enabled = true
				c.Storage.Backend = "s3"
				c.Storage.S3Endpoint = "s3.amazonaws.com"
				c.Storage.S3AccessKey = "AKID"
				c.Storage.S3SecretKey = "secret"
			},
			wantErr: "s3_bucket is required",
		},
		{
			name: "storage s3 missing access key",
			modify: func(c *Config) {
				c.Storage.Enabled = true
				c.Storage.Backend = "s3"
				c.Storage.S3Endpoint = "s3.amazonaws.com"
				c.Storage.S3Bucket = "my-bucket"
				c.Storage.S3SecretKey = "secret"
			},
			wantErr: "s3_access_key is required",
		},
		{
			name: "storage s3 missing secret key",
			modify: func(c *Config) {
				c.Storage.Enabled = true
				c.Storage.Backend = "s3"
				c.Storage.S3Endpoint = "s3.amazonaws.com"
				c.Storage.S3Bucket = "my-bucket"
				c.Storage.S3AccessKey = "AKID"
			},
			wantErr: "s3_secret_key is required",
		},
		{
			name: "storage unsupported backend",
			modify: func(c *Config) {
				c.Storage.Enabled = true
				c.Storage.Backend = "gcs"
			},
			wantErr: "storage.backend must be",
		},
		{
			name:   "storage disabled ignores validation",
			modify: func(c *Config) { c.Storage.Enabled = false },
		},
		{
			name:    "invalid api.import_max_size_mb",
			modify:  func(c *Config) { c.API.ImportMaxSizeMB = 0 },
			wantErr: "api.import_max_size_mb must be positive",
		},
		{
			name:    "invalid api.import_max_rows",
			modify:  func(c *Config) { c.API.ImportMaxRows = 0 },
			wantErr: "api.import_max_rows must be positive",
		},
		{
			name:    "invalid api.export_max_rows",
			modify:  func(c *Config) { c.API.ExportMaxRows = 0 },
			wantErr: "api.export_max_rows must be positive",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Default()
			tt.modify(cfg)
			err := cfg.Validate()
			if tt.wantErr == "" {
				testutil.NoError(t, err)
			} else {
				testutil.ErrorContains(t, err, tt.wantErr)
			}
		})
	}
}

func TestValidateLogDrainDefaults(t *testing.T) {
	t.Parallel()

	cfg := Default()
	cfg.Logging.Drains = []LogDrainConfig{{Type: "http", URL: "https://logs.example.com/ingest", BatchSize: 0, FlushIntervalSecs: 0}}

	testutil.NoError(t, cfg.Validate())
	testutil.Equal(t, 100, cfg.Logging.Drains[0].BatchSize)
	testutil.Equal(t, 5, cfg.Logging.Drains[0].FlushIntervalSecs)
}

func TestValidateLogDrainEnabledDefault(t *testing.T) {
	cfg := Default()
	cfg.Logging.Drains = []LogDrainConfig{{Type: "http", URL: "https://logs.example.com/ingest"}}

	testutil.NoError(t, cfg.Validate())
	testutil.True(t, cfg.Logging.Drains[0].Enabled != nil)
	testutil.True(t, *cfg.Logging.Drains[0].Enabled)
}

func boolPtr(v bool) *bool {
	return &v
}

func TestValidatePushFCMCredentialsJSON(t *testing.T) {
	t.Parallel()
	cfg := Default()
	cfg.Jobs.Enabled = true
	cfg.Push.Enabled = true

	credPath := filepath.Join(t.TempDir(), "fcm.json")
	testutil.NoError(t, os.WriteFile(credPath, []byte(`{"project_id":"demo-project"}`), 0o600))
	cfg.Push.FCM.CredentialsFile = credPath

	testutil.NoError(t, cfg.Validate())
}

func TestValidatePushFCMCredentialsInvalidJSON(t *testing.T) {
	t.Parallel()
	cfg := Default()
	cfg.Jobs.Enabled = true
	cfg.Push.Enabled = true

	credPath := filepath.Join(t.TempDir(), "fcm.json")
	testutil.NoError(t, os.WriteFile(credPath, []byte("{invalid-json"), 0o600))
	cfg.Push.FCM.CredentialsFile = credPath

	testutil.ErrorContains(t, cfg.Validate(), "push.fcm.credentials_file")
}

func TestLoadFromFile(t *testing.T) {
	dir := t.TempDir()
	tomlPath := filepath.Join(dir, "ayb.toml")

	content := `
[server]
host = "127.0.0.1"
port = 3000

[database]
url = "postgresql://localhost/mydb"
max_conns = 10

[logging]
level = "debug"
format = "text"
`
	err := os.WriteFile(tomlPath, []byte(content), 0o644)
	testutil.NoError(t, err)

	cfg, err := Load(tomlPath, nil)
	testutil.NoError(t, err)

	testutil.Equal(t, "127.0.0.1", cfg.Server.Host)
	testutil.Equal(t, 3000, cfg.Server.Port)
	testutil.Equal(t, "postgresql://localhost/mydb", cfg.Database.URL)
	testutil.Equal(t, 10, cfg.Database.MaxConns)
	testutil.Equal(t, "debug", cfg.Logging.Level)
	testutil.Equal(t, "text", cfg.Logging.Format)

	// Defaults preserved for unset fields.
	testutil.Equal(t, 2, cfg.Database.MinConns)
	testutil.Equal(t, true, cfg.Admin.Enabled)
}

func TestLoadAPIConfigFromFile(t *testing.T) {
	dir := t.TempDir()
	tomlPath := filepath.Join(dir, "ayb.toml")

	content := `
[api]
import_max_size_mb = 12
import_max_rows = 250
export_max_rows = 1000
aggregate_enabled = false
`
	err := os.WriteFile(tomlPath, []byte(content), 0o644)
	testutil.NoError(t, err)

	cfg, err := Load(tomlPath, nil)
	testutil.NoError(t, err)
	testutil.Equal(t, 12, cfg.API.ImportMaxSizeMB)
	testutil.Equal(t, 250, cfg.API.ImportMaxRows)
	testutil.Equal(t, 1000, cfg.API.ExportMaxRows)
	testutil.Equal(t, false, cfg.API.AggregateEnabled)
}

func TestLoadMissingAPISectionUsesDefaults(t *testing.T) {
	dir := t.TempDir()
	tomlPath := filepath.Join(dir, "ayb.toml")

	err := os.WriteFile(tomlPath, []byte("[server]\nhost = \"127.0.0.1\"\n"), 0o644)
	testutil.NoError(t, err)

	cfg, err := Load(tomlPath, nil)
	testutil.NoError(t, err)

	defaults := Default()
	testutil.Equal(t, defaults.API.ImportMaxSizeMB, cfg.API.ImportMaxSizeMB)
	testutil.Equal(t, defaults.API.ImportMaxRows, cfg.API.ImportMaxRows)
	testutil.Equal(t, defaults.API.ExportMaxRows, cfg.API.ExportMaxRows)
	testutil.Equal(t, defaults.API.AggregateEnabled, cfg.API.AggregateEnabled)
}

func TestLoadMinPasswordLengthFromFile(t *testing.T) {
	dir := t.TempDir()
	tomlPath := filepath.Join(dir, "ayb.toml")

	content := `
[auth]
min_password_length = 3
`
	err := os.WriteFile(tomlPath, []byte(content), 0o644)
	testutil.NoError(t, err)

	cfg, err := Load(tomlPath, nil)
	testutil.NoError(t, err)
	testutil.Equal(t, 3, cfg.Auth.MinPasswordLength)
}

func TestLoadOAuthProviderModeFromFile(t *testing.T) {
	dir := t.TempDir()
	tomlPath := filepath.Join(dir, "ayb.toml")

	content := `
[auth]
enabled = true
jwt_secret = "this-is-a-secret-that-is-at-least-32-characters-long"

[auth.oauth_provider]
enabled = true
access_token_duration = 1200
refresh_token_duration = 86400
auth_code_duration = 180
`
	err := os.WriteFile(tomlPath, []byte(content), 0o644)
	testutil.NoError(t, err)

	cfg, err := Load(tomlPath, nil)
	testutil.NoError(t, err)
	testutil.Equal(t, true, cfg.Auth.OAuthProviderMode.Enabled)
	testutil.Equal(t, 1200, cfg.Auth.OAuthProviderMode.AccessTokenDuration)
	testutil.Equal(t, 86400, cfg.Auth.OAuthProviderMode.RefreshTokenDuration)
	testutil.Equal(t, 180, cfg.Auth.OAuthProviderMode.AuthCodeDuration)
}

func TestLoadSiteURLFromFile(t *testing.T) {
	dir := t.TempDir()
	tomlPath := filepath.Join(dir, "ayb.toml")

	content := `
[server]
site_url = "https://prod.example.com"
`
	err := os.WriteFile(tomlPath, []byte(content), 0o644)
	testutil.NoError(t, err)

	cfg, err := Load(tomlPath, nil)
	testutil.NoError(t, err)
	testutil.Equal(t, "https://prod.example.com", cfg.Server.SiteURL)
	testutil.Equal(t, "https://prod.example.com", cfg.PublicBaseURL())
	// Address() should still use the default bind address, not site_url.
	testutil.Equal(t, "127.0.0.1:8090", cfg.Address())
}

func TestLoadMissingFileUsesDefaults(t *testing.T) {
	// Point to a non-existent file — should silently use defaults.
	cfg, err := Load("/nonexistent/ayb.toml", nil)
	testutil.NoError(t, err)
	testutil.Equal(t, 8090, cfg.Server.Port)
	testutil.Equal(t, "127.0.0.1", cfg.Server.Host)
}

func TestLoadInvalidTOML(t *testing.T) {
	dir := t.TempDir()
	tomlPath := filepath.Join(dir, "ayb.toml")
	err := os.WriteFile(tomlPath, []byte("this is not valid toml [[["), 0o644)
	testutil.NoError(t, err)

	_, err = Load(tomlPath, nil)
	testutil.ErrorContains(t, err, "parsing")
}

func TestLoadEnvOverrides(t *testing.T) {
	// Set env vars, then clean up.
	t.Setenv("AYB_SERVER_HOST", "envhost")
	t.Setenv("AYB_SERVER_PORT", "9999")
	t.Setenv("AYB_SERVER_ALLOWED_IPS", "203.0.113.10, 198.51.100.0/24")
	t.Setenv("AYB_DATABASE_URL", "postgresql://envdb")
	t.Setenv("AYB_DATABASE_REPLICA_URLS", "postgresql://replica-1/db,postgresql://replica-2/db")
	t.Setenv("AYB_ADMIN_ALLOWED_IPS", "2001:db8::1")
	t.Setenv("AYB_ADMIN_PASSWORD", "secret123")
	t.Setenv("AYB_LOG_LEVEL", "warn")
	t.Setenv("AYB_METRICS_ENABLED", "1")
	t.Setenv("AYB_METRICS_PATH", "/internal-metrics")
	t.Setenv("AYB_METRICS_AUTH_TOKEN", "metrics-token")
	t.Setenv("AYB_CORS_ORIGINS", "http://a.com,http://b.com")
	t.Setenv("AYB_AUTH_ENABLED", "true")
	t.Setenv("AYB_AUTH_JWT_SECRET", "this-is-a-secret-that-is-at-least-32-characters-long")
	t.Setenv("AYB_AUDIT_ENABLED", "true")
	t.Setenv("AYB_AUDIT_TABLES", "public.users, public.posts")
	t.Setenv("AYB_AUDIT_ALL_TABLES", "1")
	t.Setenv("AYB_AUDIT_RETENTION_DAYS", "45")

	cfg, err := Load("/nonexistent/ayb.toml", nil)
	testutil.NoError(t, err)

	testutil.Equal(t, "envhost", cfg.Server.Host)
	testutil.Equal(t, 9999, cfg.Server.Port)
	testutil.SliceLen(t, cfg.Server.AllowedIPs, 2)
	testutil.Equal(t, "203.0.113.10", cfg.Server.AllowedIPs[0])
	testutil.Equal(t, "198.51.100.0/24", cfg.Server.AllowedIPs[1])
	testutil.SliceLen(t, cfg.Admin.AllowedIPs, 1)
	testutil.Equal(t, "2001:db8::1", cfg.Admin.AllowedIPs[0])
	testutil.Equal(t, "postgresql://envdb", cfg.Database.URL)
	testutil.SliceLen(t, cfg.Database.Replicas, 2)
	testutil.Equal(t, "postgresql://replica-1/db", cfg.Database.Replicas[0].URL)
	testutil.Equal(t, 1, cfg.Database.Replicas[0].Weight)
	testutil.Equal(t, int64(10*1024*1024), cfg.Database.Replicas[0].MaxLagBytes)
	testutil.Equal(t, "postgresql://replica-2/db", cfg.Database.Replicas[1].URL)
	testutil.Equal(t, 1, cfg.Database.Replicas[1].Weight)
	testutil.Equal(t, int64(10*1024*1024), cfg.Database.Replicas[1].MaxLagBytes)
	testutil.Equal(t, "secret123", cfg.Admin.Password)
	testutil.Equal(t, "warn", cfg.Logging.Level)
	testutil.Equal(t, true, cfg.Metrics.Enabled)
	testutil.Equal(t, "/internal-metrics", cfg.Metrics.Path)
	testutil.Equal(t, "metrics-token", cfg.Metrics.AuthToken)
	testutil.SliceLen(t, cfg.Server.CORSAllowedOrigins, 2)
	testutil.Equal(t, "http://a.com", cfg.Server.CORSAllowedOrigins[0])
	testutil.Equal(t, "http://b.com", cfg.Server.CORSAllowedOrigins[1])
	testutil.Equal(t, true, cfg.Auth.Enabled)
	testutil.Equal(t, "this-is-a-secret-that-is-at-least-32-characters-long", cfg.Auth.JWTSecret)
	testutil.Equal(t, true, cfg.Audit.Enabled)
	testutil.SliceLen(t, cfg.Audit.Tables, 2)
	testutil.Equal(t, "public.users", cfg.Audit.Tables[0])
	testutil.Equal(t, "public.posts", cfg.Audit.Tables[1])
	testutil.Equal(t, true, cfg.Audit.AllTables)
	testutil.Equal(t, 45, cfg.Audit.RetentionDays)
}

func TestApplyEnvAudit_InvalidRetentionDays(t *testing.T) {
	t.Setenv("AYB_AUDIT_RETENTION_DAYS", "notanumber")

	cfg := Default()
	err := applyEnv(cfg)
	testutil.ErrorContains(t, err, "AYB_AUDIT_RETENTION_DAYS")
}

func TestApplyEnvEdgeFunctions(t *testing.T) {
	t.Setenv("AYB_EDGE_FUNCTIONS_POOL_SIZE", "4")
	t.Setenv("AYB_EDGE_FUNCTIONS_DEFAULT_TIMEOUT_MS", "9000")
	t.Setenv("AYB_EDGE_FUNCTIONS_MAX_REQUEST_BODY_BYTES", "12345")
	t.Setenv("AYB_EDGE_FUNCTIONS_FETCH_DOMAIN_ALLOWLIST", "api.example.com,cdn.example.com")

	cfg := Default()
	err := applyEnv(cfg)
	testutil.NoError(t, err)

	testutil.Equal(t, 4, cfg.EdgeFunctions.PoolSize)
	testutil.Equal(t, 9000, cfg.EdgeFunctions.DefaultTimeoutMs)
	testutil.Equal(t, int64(12345), cfg.EdgeFunctions.MaxRequestBodyBytes)
	testutil.SliceLen(t, cfg.EdgeFunctions.FetchDomainAllowlist, 2)
	testutil.Equal(t, "api.example.com", cfg.EdgeFunctions.FetchDomainAllowlist[0])
	testutil.Equal(t, "cdn.example.com", cfg.EdgeFunctions.FetchDomainAllowlist[1])
}

func TestApplyEnvEdgeFunctions_InvalidBodySize(t *testing.T) {
	t.Setenv("AYB_EDGE_FUNCTIONS_MAX_REQUEST_BODY_BYTES", "notanumber")

	cfg := Default()
	err := applyEnv(cfg)
	testutil.ErrorContains(t, err, "AYB_EDGE_FUNCTIONS_MAX_REQUEST_BODY_BYTES")
}

func TestApplyEnvEdgeFunctionsHardening(t *testing.T) {
	t.Setenv("AYB_EDGE_FUNCTIONS_MEMORY_LIMIT_MB", "256")
	t.Setenv("AYB_EDGE_FUNCTIONS_MAX_CONCURRENT_INVOCATIONS", "72")
	t.Setenv("AYB_EDGE_FUNCTIONS_CODE_CACHE_SIZE", "512")

	cfg := Default()
	err := applyEnv(cfg)
	testutil.NoError(t, err)

	got, err := GetValue(cfg, "edge_functions.memory_limit_mb")
	testutil.NoError(t, err)
	testutil.Equal(t, 256, got)

	got, err = GetValue(cfg, "edge_functions.max_concurrent_invocations")
	testutil.NoError(t, err)
	testutil.Equal(t, 72, got)

	got, err = GetValue(cfg, "edge_functions.code_cache_size")
	testutil.NoError(t, err)
	testutil.Equal(t, 512, got)
}

func TestLoadEdgeFunctionsHardeningFromTOML(t *testing.T) {
	dir := t.TempDir()
	tomlPath := filepath.Join(dir, "ayb.toml")
	tomlBody := `
[edge_functions]
memory_limit_mb = 192
max_concurrent_invocations = 64
code_cache_size = 300
`
	testutil.NoError(t, os.WriteFile(tomlPath, []byte(tomlBody), 0o644))

	cfg, err := Load(tomlPath, nil)
	testutil.NoError(t, err)

	got, err := GetValue(cfg, "edge_functions.memory_limit_mb")
	testutil.NoError(t, err)
	testutil.Equal(t, 192, got)

	got, err = GetValue(cfg, "edge_functions.max_concurrent_invocations")
	testutil.NoError(t, err)
	testutil.Equal(t, 64, got)

	got, err = GetValue(cfg, "edge_functions.code_cache_size")
	testutil.NoError(t, err)
	testutil.Equal(t, 300, got)
}

func TestLoadEdgeFunctionsHardeningValidation(t *testing.T) {
	tests := []struct {
		name     string
		tomlBody string
		wantErr  string
	}{
		{
			name: "memory_limit_mb too small",
			tomlBody: `
[edge_functions]
memory_limit_mb = 0
`,
			wantErr: "edge_functions.memory_limit_mb must be at least 1",
		},
		{
			name: "max_concurrent_invocations too small",
			tomlBody: `
[edge_functions]
max_concurrent_invocations = 0
`,
			wantErr: "edge_functions.max_concurrent_invocations must be at least 1",
		},
		{
			name: "code_cache_size too small",
			tomlBody: `
[edge_functions]
code_cache_size = 0
`,
			wantErr: "edge_functions.code_cache_size must be at least 1",
		},
		{
			name: "max_concurrent_invocations below pool_size",
			tomlBody: `
[edge_functions]
pool_size = 20
max_concurrent_invocations = 10
`,
			wantErr: "edge_functions.max_concurrent_invocations must be at least edge_functions.pool_size",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			tomlPath := filepath.Join(dir, "ayb.toml")
			testutil.NoError(t, os.WriteFile(tomlPath, []byte(tt.tomlBody), 0o644))

			_, err := Load(tomlPath, nil)
			testutil.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestLoadFlagOverrides(t *testing.T) {
	flags := map[string]string{
		"database-url": "postgresql://flagdb",
		"port":         "7777",
		"host":         "flaghost",
	}

	cfg, err := Load("/nonexistent/ayb.toml", flags)
	testutil.NoError(t, err)

	testutil.Equal(t, "postgresql://flagdb", cfg.Database.URL)
	testutil.Equal(t, 7777, cfg.Server.Port)
	testutil.Equal(t, "flaghost", cfg.Server.Host)
}

func TestLoadPriority(t *testing.T) {
	// File sets port=3000, env sets port=4000, flag sets port=5000.
	// Expected priority: flag > env > file > default.
	dir := t.TempDir()
	tomlPath := filepath.Join(dir, "ayb.toml")
	err := os.WriteFile(tomlPath, []byte("[server]\nport = 3000\n"), 0o644)
	testutil.NoError(t, err)

	t.Setenv("AYB_SERVER_PORT", "4000")
	flags := map[string]string{"port": "5000"}

	cfg, err := Load(tomlPath, flags)
	testutil.NoError(t, err)
	testutil.Equal(t, 5000, cfg.Server.Port)

	// Without flag, env wins over file.
	cfg, err = Load(tomlPath, nil)
	testutil.NoError(t, err)
	testutil.Equal(t, 4000, cfg.Server.Port)
}

func TestLoadEnvOverridesFile(t *testing.T) {
	dir := t.TempDir()
	tomlPath := filepath.Join(dir, "ayb.toml")
	err := os.WriteFile(tomlPath, []byte("[server]\nhost = \"filehost\"\n"), 0o644)
	testutil.NoError(t, err)

	t.Setenv("AYB_SERVER_HOST", "envhost")

	cfg, err := Load(tomlPath, nil)
	testutil.NoError(t, err)
	testutil.Equal(t, "envhost", cfg.Server.Host)
}

func TestGenerateDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", "ayb.toml")

	err := GenerateDefault(path)
	testutil.NoError(t, err)

	data, err := os.ReadFile(path)
	testutil.NoError(t, err)
	content := string(data)

	testutil.Contains(t, content, "[server]")
	testutil.Contains(t, content, "[database]")
	testutil.Contains(t, content, "[admin]")
	testutil.Contains(t, content, "[auth]")
	testutil.Contains(t, content, "[auth.oauth_provider]")
	testutil.Contains(t, content, "[email]")
	testutil.Contains(t, content, "[storage]")
	testutil.Contains(t, content, "[logging]")
	testutil.Contains(t, content, "port = 8090")
	testutil.Contains(t, content, "token_duration = 900")
	testutil.Contains(t, content, "refresh_token_duration = 604800")
	testutil.Contains(t, content, "min_password_length = 8")
	testutil.Contains(t, content, "access_token_duration = 3600")
	testutil.Contains(t, content, "auth_code_duration = 600")

	// Verify file permissions are 0600 (config may contain secrets after user edits).
	info, err := os.Stat(path)
	testutil.NoError(t, err)
	testutil.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestGenerateDefaultIncludesJobsSection(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ayb.toml")

	err := GenerateDefault(path)
	testutil.NoError(t, err)

	data, err := os.ReadFile(path)
	testutil.NoError(t, err)
	content := string(data)

	testutil.Contains(t, content, "[jobs]")
	testutil.Contains(t, content, "enabled = false")
	testutil.Contains(t, content, "worker_concurrency = 4")
	testutil.Contains(t, content, "poll_interval_ms = 1000")
	testutil.Contains(t, content, "lease_duration_s = 300")
	testutil.Contains(t, content, "max_retries_default = 3")
	testutil.Contains(t, content, "scheduler_enabled = true")
	testutil.Contains(t, content, "scheduler_tick_s = 15")
}

func TestGenerateDefaultIncludesOIDCSection(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ayb.toml")

	err := GenerateDefault(path)
	testutil.NoError(t, err)

	data, err := os.ReadFile(path)
	testutil.NoError(t, err)
	content := string(data)

	testutil.Contains(t, content, "[auth.oidc.keycloak]")
	testutil.Contains(t, content, "issuer_url = \"https://idp.example.com/realms/main\"")
	testutil.Contains(t, content, "client_id = \"\"")
	testutil.Contains(t, content, "client_secret = \"\"")
	testutil.Contains(t, content, "scopes = [\"openid\", \"profile\", \"email\"]")
	testutil.Contains(t, content, "display_name = \"Keycloak\"")
}

func TestGenerateDefaultIncludesSAMLSection(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ayb.toml")

	err := GenerateDefault(path)
	testutil.NoError(t, err)

	data, err := os.ReadFile(path)
	testutil.NoError(t, err)
	content := string(data)

	testutil.Contains(t, content, "[[auth.saml_providers]]")
	testutil.Contains(t, content, "idp_metadata_url = \"https://idp.example.com/metadata\"")
	testutil.Contains(t, content, "[auth.saml_providers.attribute_mapping]")
	testutil.Contains(t, content, "email = \"email\"")
}

func TestToTOML(t *testing.T) {
	cfg := Default()
	s, err := cfg.ToTOML()
	testutil.NoError(t, err)
	testutil.Contains(t, s, "host = '127.0.0.1'")
	testutil.Contains(t, s, "port = 8090")
}

func TestApplyFlagsNilSafe(t *testing.T) {
	cfg := Default()
	// Should not panic with nil flags.
	applyFlags(cfg, nil)
	testutil.Equal(t, 8090, cfg.Server.Port)
}

func TestApplyFlagsEmptyValues(t *testing.T) {
	cfg := Default()
	flags := map[string]string{
		"database-url": "",
		"port":         "",
		"host":         "",
	}
	applyFlags(cfg, flags)
	// Empty values should not override defaults.
	testutil.Equal(t, "127.0.0.1", cfg.Server.Host)
	testutil.Equal(t, 8090, cfg.Server.Port)
}

func TestApplyEnvInvalidPort(t *testing.T) {
	t.Setenv("AYB_SERVER_PORT", "notanumber")
	cfg := Default()
	err := applyEnv(cfg)
	testutil.ErrorContains(t, err, "not an integer")
	testutil.Equal(t, 8090, cfg.Server.Port) // unchanged on error
}

func TestStorageMaxFileSizeBytes(t *testing.T) {
	tests := []struct {
		input string
		want  int64
	}{
		{"10MB", 10 << 20},
		{"5MB", 5 << 20},
		{"1MB", 1 << 20},
		{"1GB", 1 << 30},
		{"2GB", 2 << 30},
		{"500KB", 500 << 10},
		{"", 10 << 20},        // default
		{"invalid", 10 << 20}, // default on parse failure
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			cfg := &StorageConfig{MaxFileSize: tt.input}
			testutil.Equal(t, tt.want, cfg.MaxFileSizeBytes())
		})
	}
}

func TestApplyStorageEnvVars(t *testing.T) {
	t.Setenv("AYB_STORAGE_ENABLED", "true")
	t.Setenv("AYB_STORAGE_BACKEND", "local")
	t.Setenv("AYB_STORAGE_LOCAL_PATH", "/tmp/custom")
	t.Setenv("AYB_STORAGE_MAX_FILE_SIZE", "50MB")

	cfg := Default()
	err := applyEnv(cfg)
	testutil.NoError(t, err)

	testutil.Equal(t, true, cfg.Storage.Enabled)
	testutil.Equal(t, "local", cfg.Storage.Backend)
	testutil.Equal(t, "/tmp/custom", cfg.Storage.LocalPath)
	testutil.Equal(t, "50MB", cfg.Storage.MaxFileSize)
}

func TestApplyS3StorageEnvVars(t *testing.T) {
	t.Setenv("AYB_STORAGE_S3_ENDPOINT", "s3.amazonaws.com")
	t.Setenv("AYB_STORAGE_S3_BUCKET", "test-bucket")
	t.Setenv("AYB_STORAGE_S3_REGION", "eu-west-1")
	t.Setenv("AYB_STORAGE_S3_ACCESS_KEY", "AKID123")
	t.Setenv("AYB_STORAGE_S3_SECRET_KEY", "secret456")
	t.Setenv("AYB_STORAGE_S3_USE_SSL", "false")

	cfg := Default()
	err := applyEnv(cfg)
	testutil.NoError(t, err)

	testutil.Equal(t, cfg.Storage.S3Endpoint, "s3.amazonaws.com")
	testutil.Equal(t, cfg.Storage.S3Bucket, "test-bucket")
	testutil.Equal(t, cfg.Storage.S3Region, "eu-west-1")
	testutil.Equal(t, cfg.Storage.S3AccessKey, "AKID123")
	testutil.Equal(t, cfg.Storage.S3SecretKey, "secret456")
	testutil.Equal(t, cfg.Storage.S3UseSSL, false)
}

func TestParseTOMLStorageCDNURL(t *testing.T) {
	t.Parallel()

	cfg, err := ParseTOML([]byte(`
[storage]
cdn_url = "https://cdn.example.com"
`))
	testutil.NoError(t, err)
	testutil.Equal(t, "https://cdn.example.com", cfg.Storage.CDNURL)
}

func TestApplyStorageCDNURLEnvOverride(t *testing.T) {
	t.Setenv("AYB_STORAGE_CDN_URL", "https://cdn.env.example.com")

	cfg, err := ParseTOML([]byte(`
[storage]
cdn_url = "https://cdn.toml.example.com"
`))
	testutil.NoError(t, err)
	testutil.NoError(t, applyEnv(cfg))
	testutil.Equal(t, "https://cdn.env.example.com", cfg.Storage.CDNURL)
}

func TestValidateEmbeddedPort(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		port    int
		wantErr string
	}{
		{"valid default port, no URL", "", 15432, ""},
		{"valid custom port, no URL", "", 9999, ""},
		{"invalid port zero, no URL", "", 0, "database.embedded_port must be between 1 and 65535"},
		{"invalid port too high, no URL", "", 99999, "database.embedded_port must be between 1 and 65535"},
		{"invalid port ignored when URL set", "postgresql://localhost/db", 0, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Default()
			cfg.Database.URL = tt.url
			cfg.Database.EmbeddedPort = tt.port
			err := cfg.Validate()
			if tt.wantErr == "" {
				testutil.NoError(t, err)
			} else {
				testutil.ErrorContains(t, err, tt.wantErr)
			}
		})
	}
}

func TestApplyEmbeddedEnvVars(t *testing.T) {
	t.Setenv("AYB_DATABASE_EMBEDDED_PORT", "19999")
	t.Setenv("AYB_DATABASE_EMBEDDED_DATA_DIR", "/custom/data")

	cfg := Default()
	err := applyEnv(cfg)
	testutil.NoError(t, err)

	testutil.Equal(t, 19999, cfg.Database.EmbeddedPort)
	testutil.Equal(t, "/custom/data", cfg.Database.EmbeddedDataDir)
}

func TestApplyEmbeddedPortInvalidEnv(t *testing.T) {
	t.Setenv("AYB_DATABASE_EMBEDDED_PORT", "notanumber")
	cfg := Default()
	err := applyEnv(cfg)
	testutil.ErrorContains(t, err, "not an integer")
	testutil.Equal(t, 15432, cfg.Database.EmbeddedPort) // unchanged on error
}

func TestGenerateDefaultContainsEmbedded(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ayb.toml")
	err := GenerateDefault(path)
	testutil.NoError(t, err)

	data, err := os.ReadFile(path)
	testutil.NoError(t, err)
	testutil.Contains(t, string(data), "embedded_port")
	testutil.Contains(t, string(data), "embedded_data_dir")
}

func TestApplyOAuthEnvVars(t *testing.T) {
	t.Setenv("AYB_AUTH_OAUTH_GOOGLE_CLIENT_ID", "env-google-id")
	t.Setenv("AYB_AUTH_OAUTH_GOOGLE_CLIENT_SECRET", "env-google-secret")
	t.Setenv("AYB_AUTH_OAUTH_GOOGLE_ENABLED", "true")
	t.Setenv("AYB_AUTH_OAUTH_GOOGLE_STORE_PROVIDER_TOKENS", "1")
	t.Setenv("AYB_AUTH_OAUTH_GITHUB_CLIENT_ID", "env-github-id")
	t.Setenv("AYB_AUTH_OAUTH_MICROSOFT_CLIENT_ID", "env-ms-id")
	t.Setenv("AYB_AUTH_OAUTH_MICROSOFT_CLIENT_SECRET", "env-ms-secret")
	t.Setenv("AYB_AUTH_OAUTH_MICROSOFT_ENABLED", "1")
	t.Setenv("AYB_AUTH_OAUTH_MICROSOFT_TENANT_ID", "contoso-tenant")
	t.Setenv("AYB_AUTH_OAUTH_REDIRECT_URL", "http://myapp.com/callback")
	t.Setenv("AYB_AUTH_OAUTH_APPLE_CLIENT_ID", "com.example.app")
	t.Setenv("AYB_AUTH_OAUTH_APPLE_TEAM_ID", "TEAM123")
	t.Setenv("AYB_AUTH_OAUTH_APPLE_KEY_ID", "KEY456")
	t.Setenv("AYB_AUTH_OAUTH_APPLE_PRIVATE_KEY", "-----BEGIN EC PRIVATE KEY-----\nfake\n-----END EC PRIVATE KEY-----")
	t.Setenv("AYB_AUTH_OAUTH_APPLE_ENABLED", "true")

	cfg := Default()
	err := applyEnv(cfg)
	testutil.NoError(t, err)

	testutil.Equal(t, "http://myapp.com/callback", cfg.Auth.OAuthRedirectURL)
	testutil.NotNil(t, cfg.Auth.OAuth)

	g := cfg.Auth.OAuth["google"]
	testutil.Equal(t, "env-google-id", g.ClientID)
	testutil.Equal(t, "env-google-secret", g.ClientSecret)
	testutil.True(t, g.Enabled, "google should be enabled")
	testutil.True(t, g.StoreProviderTokens, "google store_provider_tokens should be enabled")

	gh := cfg.Auth.OAuth["github"]
	testutil.Equal(t, "env-github-id", gh.ClientID)
	testutil.False(t, gh.Enabled, "github should not be enabled (no ENABLED env)")

	ms := cfg.Auth.OAuth["microsoft"]
	testutil.Equal(t, "env-ms-id", ms.ClientID)
	testutil.Equal(t, "env-ms-secret", ms.ClientSecret)
	testutil.Equal(t, "contoso-tenant", ms.TenantID)
	testutil.True(t, ms.Enabled, "microsoft should be enabled")

	ap := cfg.Auth.OAuth["apple"]
	testutil.Equal(t, "com.example.app", ap.ClientID)
	testutil.Equal(t, "TEAM123", ap.TeamID)
	testutil.Equal(t, "KEY456", ap.KeyID)
	testutil.Equal(t, "-----BEGIN EC PRIVATE KEY-----\nfake\n-----END EC PRIVATE KEY-----", ap.PrivateKey)
	testutil.True(t, ap.Enabled, "apple should be enabled")
}

func TestLoadOAuthStoreProviderTokensFromFile(t *testing.T) {
	dir := t.TempDir()
	tomlPath := filepath.Join(dir, "ayb.toml")

	content := `
[auth]
enabled = true
jwt_secret = "this-is-a-secret-that-is-at-least-32-characters-long"

[auth.oauth.google]
enabled = true
client_id = "google-id"
client_secret = "google-secret"
store_provider_tokens = true
`
	err := os.WriteFile(tomlPath, []byte(content), 0o644)
	testutil.NoError(t, err)

	cfg, err := Load(tomlPath, nil)
	testutil.NoError(t, err)
	testutil.Equal(t, true, cfg.Auth.OAuth["google"].StoreProviderTokens)
}

func TestApplyOAuthProviderModeEnvVars(t *testing.T) {
	t.Setenv("AYB_AUTH_OAUTH_PROVIDER_ENABLED", "true")
	t.Setenv("AYB_AUTH_OAUTH_PROVIDER_ACCESS_TOKEN_DURATION", "1200")
	t.Setenv("AYB_AUTH_OAUTH_PROVIDER_REFRESH_TOKEN_DURATION", "86400")
	t.Setenv("AYB_AUTH_OAUTH_PROVIDER_AUTH_CODE_DURATION", "180")

	cfg := Default()
	err := applyEnv(cfg)
	testutil.NoError(t, err)

	testutil.Equal(t, true, cfg.Auth.OAuthProviderMode.Enabled)
	testutil.Equal(t, 1200, cfg.Auth.OAuthProviderMode.AccessTokenDuration)
	testutil.Equal(t, 86400, cfg.Auth.OAuthProviderMode.RefreshTokenDuration)
	testutil.Equal(t, 180, cfg.Auth.OAuthProviderMode.AuthCodeDuration)
}

func TestApplyOAuthProviderModeInvalidDurationEnvVar(t *testing.T) {
	t.Setenv("AYB_AUTH_OAUTH_PROVIDER_ACCESS_TOKEN_DURATION", "notanumber")
	cfg := Default()
	err := applyEnv(cfg)
	testutil.ErrorContains(t, err, "AYB_AUTH_OAUTH_PROVIDER_ACCESS_TOKEN_DURATION")
}

func TestApplyEmailEnvVars(t *testing.T) {
	t.Setenv("AYB_EMAIL_BACKEND", "smtp")
	t.Setenv("AYB_EMAIL_FROM", "noreply@example.com")
	t.Setenv("AYB_EMAIL_FROM_NAME", "MyApp")
	t.Setenv("AYB_EMAIL_SMTP_HOST", "smtp.resend.com")
	t.Setenv("AYB_EMAIL_SMTP_PORT", "465")
	t.Setenv("AYB_EMAIL_SMTP_USERNAME", "apikey")
	t.Setenv("AYB_EMAIL_SMTP_PASSWORD", "re_secret")
	t.Setenv("AYB_EMAIL_SMTP_AUTH_METHOD", "LOGIN")
	t.Setenv("AYB_EMAIL_SMTP_TLS", "true")

	cfg := Default()
	err := applyEnv(cfg)
	testutil.NoError(t, err)

	testutil.Equal(t, "smtp", cfg.Email.Backend)
	testutil.Equal(t, "noreply@example.com", cfg.Email.From)
	testutil.Equal(t, "MyApp", cfg.Email.FromName)
	testutil.Equal(t, "smtp.resend.com", cfg.Email.SMTP.Host)
	testutil.Equal(t, 465, cfg.Email.SMTP.Port)
	testutil.Equal(t, "apikey", cfg.Email.SMTP.Username)
	testutil.Equal(t, "re_secret", cfg.Email.SMTP.Password)
	testutil.Equal(t, "LOGIN", cfg.Email.SMTP.AuthMethod)
	testutil.Equal(t, true, cfg.Email.SMTP.TLS)
}

func TestApplyAuthRateLimitEnvVar(t *testing.T) {
	t.Setenv("AYB_AUTH_RATE_LIMIT", "25")

	cfg := Default()
	err := applyEnv(cfg)
	testutil.NoError(t, err)
	testutil.Equal(t, 25, cfg.Auth.RateLimit)
}

func TestApplyAuthRateLimitInvalidEnv(t *testing.T) {
	t.Setenv("AYB_AUTH_RATE_LIMIT", "notanumber")

	cfg := Default()
	err := applyEnv(cfg)
	testutil.ErrorContains(t, err, "not an integer")
	testutil.Equal(t, 10, cfg.Auth.RateLimit) // unchanged on error
}

func TestApplyAnonymousAuthRateLimitEnvVar(t *testing.T) {
	t.Setenv("AYB_AUTH_ANONYMOUS_RATE_LIMIT", "42")

	cfg := Default()
	err := applyEnv(cfg)
	testutil.NoError(t, err)
	testutil.Equal(t, 42, cfg.Auth.AnonymousRateLimit)
}

func TestApplyAnonymousAuthRateLimitInvalidEnv(t *testing.T) {
	t.Setenv("AYB_AUTH_ANONYMOUS_RATE_LIMIT", "notanumber")

	cfg := Default()
	err := applyEnv(cfg)
	testutil.ErrorContains(t, err, "not an integer")
	testutil.Equal(t, 30, cfg.Auth.AnonymousRateLimit) // unchanged on error
}

func TestApplyAuthSensitiveRateLimitEnvVar(t *testing.T) {
	t.Setenv("AYB_AUTH_RATE_LIMIT_AUTH", "7/hour")

	cfg := Default()
	err := applyEnv(cfg)
	testutil.NoError(t, err)
	testutil.Equal(t, "7/hour", cfg.Auth.RateLimitAuth)
}

func TestApplyMinPasswordLengthEnvVar(t *testing.T) {
	t.Setenv("AYB_AUTH_MIN_PASSWORD_LENGTH", "3")

	cfg := Default()
	err := applyEnv(cfg)
	testutil.NoError(t, err)
	testutil.Equal(t, 3, cfg.Auth.MinPasswordLength)
}

func TestApplyMinPasswordLengthInvalidEnv(t *testing.T) {
	t.Setenv("AYB_AUTH_MIN_PASSWORD_LENGTH", "notanumber")

	cfg := Default()
	err := applyEnv(cfg)
	testutil.ErrorContains(t, err, "not an integer")
	testutil.Equal(t, 8, cfg.Auth.MinPasswordLength) // unchanged on error
}

func TestApplyEmailWebhookEnvVars(t *testing.T) {
	t.Setenv("AYB_EMAIL_BACKEND", "webhook")
	t.Setenv("AYB_EMAIL_WEBHOOK_URL", "https://hooks.example.com/email")
	t.Setenv("AYB_EMAIL_WEBHOOK_SECRET", "whsec_abc123")
	t.Setenv("AYB_EMAIL_WEBHOOK_TIMEOUT", "30")

	cfg := Default()
	err := applyEnv(cfg)
	testutil.NoError(t, err)

	testutil.Equal(t, "webhook", cfg.Email.Backend)
	testutil.Equal(t, "https://hooks.example.com/email", cfg.Email.Webhook.URL)
	testutil.Equal(t, "whsec_abc123", cfg.Email.Webhook.Secret)
	testutil.Equal(t, 30, cfg.Email.Webhook.Timeout)
}

func TestApplySiteURLEnvVar(t *testing.T) {
	t.Setenv("AYB_SERVER_SITE_URL", "https://myapp.example.com")

	cfg := Default()
	err := applyEnv(cfg)
	testutil.NoError(t, err)
	testutil.Equal(t, "https://myapp.example.com", cfg.Server.SiteURL)
	testutil.Equal(t, "https://myapp.example.com", cfg.PublicBaseURL())
}

func TestApplySiteURLEnvVarOverridesHost(t *testing.T) {
	t.Setenv("AYB_SERVER_HOST", "192.168.1.100")
	t.Setenv("AYB_SERVER_PORT", "3000")
	t.Setenv("AYB_SERVER_SITE_URL", "https://myapp.example.com")

	cfg := Default()
	err := applyEnv(cfg)
	testutil.NoError(t, err)
	// site_url takes precedence over host:port in PublicBaseURL
	testutil.Equal(t, "https://myapp.example.com", cfg.PublicBaseURL())
	// But Address() is still the raw bind address
	testutil.Equal(t, "192.168.1.100:3000", cfg.Address())
}

func TestApplyEnvBillingStripeMode(t *testing.T) {
	t.Setenv("AYB_BILLING_PROVIDER", "stripe")
	t.Setenv("AYB_BILLING_STRIPE_SECRET_KEY", "sk_live_123")
	t.Setenv("AYB_BILLING_STRIPE_WEBHOOK_SECRET", "whsec_123")
	t.Setenv("AYB_BILLING_STRIPE_STARTER_PRICE_ID", "price_starter")
	t.Setenv("AYB_BILLING_STRIPE_PRO_PRICE_ID", "price_pro")
	t.Setenv("AYB_BILLING_STRIPE_ENTERPRISE_PRICE_ID", "price_enterprise")
	t.Setenv("AYB_BILLING_STRIPE_METER_API_REQUESTS", "meter.api_requests")
	t.Setenv("AYB_BILLING_STRIPE_METER_STORAGE_BYTES", "meter.storage_bytes")
	t.Setenv("AYB_BILLING_STRIPE_METER_BANDWIDTH_BYTES", "meter.bandwidth_bytes")
	t.Setenv("AYB_BILLING_STRIPE_METER_FUNCTION_INVOCATIONS", "meter.function_invocations")
	t.Setenv("AYB_BILLING_USAGE_SYNC_INTERVAL_SECONDS", "900")

	cfg := Default()
	err := applyEnv(cfg)
	testutil.NoError(t, err)

	testutil.Equal(t, "stripe", cfg.Billing.Provider)
	testutil.Equal(t, "sk_live_123", cfg.Billing.StripeSecretKey)
	testutil.Equal(t, "whsec_123", cfg.Billing.StripeWebhookSecret)
	testutil.Equal(t, "price_starter", cfg.Billing.StripeStarterPriceID)
	testutil.Equal(t, "price_pro", cfg.Billing.StripeProPriceID)
	testutil.Equal(t, "price_enterprise", cfg.Billing.StripeEnterprisePriceID)
	testutil.Equal(t, "meter.api_requests", cfg.Billing.StripeMeterAPIRequests)
	testutil.Equal(t, "meter.storage_bytes", cfg.Billing.StripeMeterStorageBytes)
	testutil.Equal(t, "meter.bandwidth_bytes", cfg.Billing.StripeMeterBandwidthBytes)
	testutil.Equal(t, "meter.function_invocations", cfg.Billing.StripeMeterFunctionInvs)
	testutil.Equal(t, 900, cfg.Billing.UsageSyncIntervalSecs)
}

func TestApplyEnvSupportWebhookSecret(t *testing.T) {
	t.Setenv("AYB_SUPPORT_WEBHOOK_SECRET", "support-whsec")

	cfg := Default()
	err := applyEnv(cfg)
	testutil.NoError(t, err)
	testutil.Equal(t, "support-whsec", cfg.Support.WebhookSecret)
}

func TestApplyEnvJobsConfig(t *testing.T) {
	t.Setenv("AYB_JOBS_ENABLED", "true")
	t.Setenv("AYB_JOBS_WORKER_CONCURRENCY", "12")
	t.Setenv("AYB_JOBS_POLL_INTERVAL_MS", "1500")
	t.Setenv("AYB_JOBS_LEASE_DURATION_S", "240")
	t.Setenv("AYB_JOBS_MAX_RETRIES_DEFAULT", "8")
	t.Setenv("AYB_JOBS_SCHEDULER_ENABLED", "false")
	t.Setenv("AYB_JOBS_SCHEDULER_TICK_S", "45")

	cfg := Default()
	err := applyEnv(cfg)
	testutil.NoError(t, err)

	testutil.True(t, cfg.Jobs.Enabled)
	testutil.Equal(t, 12, cfg.Jobs.WorkerConcurrency)
	testutil.Equal(t, 1500, cfg.Jobs.PollIntervalMs)
	testutil.Equal(t, 240, cfg.Jobs.LeaseDurationS)
	testutil.Equal(t, 8, cfg.Jobs.MaxRetriesDefault)
	testutil.False(t, cfg.Jobs.SchedulerEnabled)
	testutil.Equal(t, 45, cfg.Jobs.SchedulerTickS)
}

// --- GetValue / SetValue / IsValidKey tests ---

func TestIsValidKey(t *testing.T) {
	tests := []struct {
		key  string
		want bool
	}{
		{"server.port", true},
		{"server.host", true},
		{"server.site_url", true},
		{"server.allowed_ips", true},
		{"admin.allowed_ips", true},
		{"database.url", true},
		{"auth.enabled", true},
		{"auth.jwt_secret", true},
		{"auth.oauth_provider.enabled", true},
		{"auth.oauth_provider.access_token_duration", true},
		{"auth.oauth_provider.refresh_token_duration", true},
		{"auth.oauth_provider.auth_code_duration", true},
		{"auth.min_password_length", true},
		{"auth.rate_limit_auth", true},
		{"storage.s3_bucket", true},
		{"logging.level", true},
		{"logging.format", true},
		{"auth.magic_link_enabled", true},
		{"auth.magic_link_duration", true},
		{"auth.email_mfa_enabled", true},
		{"billing.provider", true},
		{"billing.stripe_secret_key", true},
		{"billing.stripe_webhook_secret", true},
		{"billing.stripe_starter_price_id", true},
		{"billing.stripe_pro_price_id", true},
		{"billing.stripe_enterprise_price_id", true},
		{"billing.stripe_meter_api_requests", true},
		{"billing.stripe_meter_storage_bytes", true},
		{"billing.stripe_meter_bandwidth_bytes", true},
		{"billing.stripe_meter_function_invocations", true},
		{"billing.usage_sync_interval_seconds", true},
		{"server.nonexistent", false},
		{"", false},
		{"invalid", false},
		{"server", false},
		{"server.port.extra", false},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			testutil.Equal(t, tt.want, IsValidKey(tt.key))
		})
	}
}

func TestGetValue(t *testing.T) {
	cfg := Default()
	cfg.Server.AllowedIPs = []string{"203.0.113.10", "198.51.100.0/24"}
	cfg.Admin.AllowedIPs = []string{"2001:db8::1"}

	tests := []struct {
		key     string
		want    any
		wantErr bool
	}{
		{"server.host", "127.0.0.1", false},
		{"server.port", 8090, false},
		{"server.site_url", "", false},
		{"database.max_conns", 25, false},
		{"server.allowed_ips", "203.0.113.10,198.51.100.0/24", false},
		{"admin.allowed_ips", "2001:db8::1", false},
		{"admin.enabled", true, false},
		{"auth.enabled", false, false},
		{"auth.oauth_provider.enabled", false, false},
		{"auth.oauth_provider.access_token_duration", 3600, false},
		{"auth.oauth_provider.refresh_token_duration", 2592000, false},
		{"auth.oauth_provider.auth_code_duration", 600, false},
		{"auth.rate_limit_auth", "10/min", false},
		{"logging.level", "info", false},
		{"storage.backend", "local", false},
		{"auth.magic_link_enabled", false, false},
		{"auth.magic_link_duration", 600, false},
		{"auth.email_mfa_enabled", false, false},
		{"billing.provider", "", false},
		{"billing.stripe_secret_key", "", false},
		{"billing.stripe_webhook_secret", "", false},
		{"billing.usage_sync_interval_seconds", 3600, false},
		{"billing.stripe_starter_price_id", "", false},
		{"billing.stripe_pro_price_id", "", false},
		{"billing.stripe_enterprise_price_id", "", false},
		{"billing.stripe_meter_api_requests", "", false},
		{"billing.stripe_meter_storage_bytes", "", false},
		{"billing.stripe_meter_bandwidth_bytes", "", false},
		{"billing.stripe_meter_function_invocations", "", false},
		{"auth.anonymous_rate_limit", 30, false},
		{"audit.enabled", false, false},
		{"audit.tables", "", false},
		{"audit.all_tables", false, false},
		{"audit.retention_days", 90, false},
		{"unknown.key", nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			val, err := GetValue(cfg, tt.key)
			if tt.wantErr {
				testutil.NotNil(t, err)
			} else {
				testutil.NoError(t, err)
				testutil.Equal(t, tt.want, val)
			}
		})
	}
}

func TestLoad_AuthEncryptionKeyFromEnv(t *testing.T) {
	t.Setenv("AYB_AUTH_ENCRYPTION_KEY", "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff")

	cfg, err := Load("/nonexistent/ayb.toml", nil)
	testutil.NoError(t, err)
	testutil.Equal(t, "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff", cfg.Auth.EncryptionKey)
}

func TestIsValidKey_AuthEncryptionKey(t *testing.T) {
	testutil.True(t, IsValidKey("auth.encryption_key"), "auth.encryption_key should be a valid key")
}

func TestIsValidKey_AuditConfig(t *testing.T) {
	testutil.True(t, IsValidKey("audit.enabled"), "audit.enabled should be a valid key")
	testutil.True(t, IsValidKey("audit.tables"), "audit.tables should be a valid key")
	testutil.True(t, IsValidKey("audit.all_tables"), "audit.all_tables should be a valid key")
	testutil.True(t, IsValidKey("audit.retention_days"), "audit.retention_days should be a valid key")
}

func TestGetValue_AuthEncryptionKey(t *testing.T) {
	cfg := Default()
	cfg.Auth.EncryptionKey = "test-encryption-key"

	got, err := GetValue(cfg, "auth.encryption_key")
	testutil.NoError(t, err)
	testutil.Equal(t, "test-encryption-key", got)
}

func TestGetValue_AuditConfig(t *testing.T) {
	cfg := Default()
	cfg.Audit.Enabled = true
	cfg.Audit.Tables = []string{"public.users", "public.posts"}
	cfg.Audit.AllTables = true
	cfg.Audit.RetentionDays = 45

	got, err := GetValue(cfg, "audit.enabled")
	testutil.NoError(t, err)
	testutil.Equal(t, true, got)

	got, err = GetValue(cfg, "audit.tables")
	testutil.NoError(t, err)
	testutil.Equal(t, "public.users,public.posts", got)

	got, err = GetValue(cfg, "audit.all_tables")
	testutil.NoError(t, err)
	testutil.Equal(t, true, got)

	got, err = GetValue(cfg, "audit.retention_days")
	testutil.NoError(t, err)
	testutil.Equal(t, 45, got)
}

func TestGetValue_BillingConfig(t *testing.T) {
	cfg := Default()
	cfg.Billing.Provider = "stripe"
	cfg.Billing.StripeSecretKey = "sk_test_123"
	cfg.Billing.StripeWebhookSecret = "whsec_123"
	cfg.Billing.StripeStarterPriceID = "price_starter"
	cfg.Billing.StripeProPriceID = "price_pro"
	cfg.Billing.StripeEnterprisePriceID = "price_enterprise"
	cfg.Billing.StripeMeterAPIRequests = "meter.api_requests"
	cfg.Billing.StripeMeterStorageBytes = "meter.storage_bytes"
	cfg.Billing.StripeMeterBandwidthBytes = "meter.bandwidth_bytes"
	cfg.Billing.StripeMeterFunctionInvs = "meter.function_invocations"
	cfg.Billing.UsageSyncIntervalSecs = 900

	got, err := GetValue(cfg, "billing.provider")
	testutil.NoError(t, err)
	testutil.Equal(t, "stripe", got)

	got, err = GetValue(cfg, "billing.usage_sync_interval_seconds")
	testutil.NoError(t, err)
	testutil.Equal(t, 900, got)

	got, err = GetValue(cfg, "billing.stripe_starter_price_id")
	testutil.NoError(t, err)
	testutil.Equal(t, "price_starter", got)

	got, err = GetValue(cfg, "billing.stripe_pro_price_id")
	testutil.NoError(t, err)
	testutil.Equal(t, "price_pro", got)

	got, err = GetValue(cfg, "billing.stripe_meter_api_requests")
	testutil.NoError(t, err)
	testutil.Equal(t, "meter.api_requests", got)

	got, err = GetValue(cfg, "billing.stripe_meter_storage_bytes")
	testutil.NoError(t, err)
	testutil.Equal(t, "meter.storage_bytes", got)

	got, err = GetValue(cfg, "billing.stripe_meter_bandwidth_bytes")
	testutil.NoError(t, err)
	testutil.Equal(t, "meter.bandwidth_bytes", got)

	got, err = GetValue(cfg, "billing.stripe_meter_function_invocations")
	testutil.NoError(t, err)
	testutil.Equal(t, "meter.function_invocations", got)
}

func TestSetValue(t *testing.T) {
	dir := t.TempDir()
	tomlPath := filepath.Join(dir, "ayb.toml")

	// Set server.port to 3000.
	err := SetValue(tomlPath, "server.port", "3000")
	testutil.NoError(t, err)

	// Verify the file was created and contains the value.
	data, err := os.ReadFile(tomlPath)
	testutil.NoError(t, err)
	testutil.Contains(t, string(data), "port = 3000")

	// Config files may contain secrets — verify owner-only permissions.
	info, err := os.Stat(tomlPath)
	testutil.NoError(t, err)
	testutil.Equal(t, os.FileMode(0o600), info.Mode().Perm())

	// Set another value in the same file.
	err = SetValue(tomlPath, "server.host", "127.0.0.1")
	testutil.NoError(t, err)

	// Load and verify both values.
	cfg, err := Load(tomlPath, nil)
	testutil.NoError(t, err)
	testutil.Equal(t, 3000, cfg.Server.Port)
	testutil.Equal(t, "127.0.0.1", cfg.Server.Host)
}

func TestSetValueBoolean(t *testing.T) {
	dir := t.TempDir()
	tomlPath := filepath.Join(dir, "ayb.toml")

	err := SetValue(tomlPath, "auth.enabled", "true")
	testutil.NoError(t, err)

	data, err := os.ReadFile(tomlPath)
	testutil.NoError(t, err)
	testutil.Contains(t, string(data), "enabled = true")
}

func TestSetValueAuditConfig(t *testing.T) {
	dir := t.TempDir()
	tomlPath := filepath.Join(dir, "ayb.toml")

	testutil.NoError(t, SetValue(tomlPath, "audit.enabled", "true"))
	testutil.NoError(t, SetValue(tomlPath, "audit.all_tables", "false"))
	testutil.NoError(t, SetValue(tomlPath, "audit.retention_days", "45"))
	testutil.NoError(t, SetValue(tomlPath, "audit.tables", "public.users, public.posts"))

	cfg, err := Load(tomlPath, nil)
	testutil.NoError(t, err)
	testutil.True(t, cfg.Audit.Enabled)
	testutil.False(t, cfg.Audit.AllTables)
	testutil.Equal(t, 45, cfg.Audit.RetentionDays)
	testutil.SliceLen(t, cfg.Audit.Tables, 2)
	testutil.Equal(t, "public.users", cfg.Audit.Tables[0])
	testutil.Equal(t, "public.posts", cfg.Audit.Tables[1])
}

func TestSetValueJobsTypes(t *testing.T) {
	dir := t.TempDir()
	tomlPath := filepath.Join(dir, "ayb.toml")

	testutil.NoError(t, SetValue(tomlPath, "jobs.enabled", "true"))
	testutil.NoError(t, SetValue(tomlPath, "jobs.worker_concurrency", "8"))
	testutil.NoError(t, SetValue(tomlPath, "jobs.scheduler_enabled", "false"))
	testutil.NoError(t, SetValue(tomlPath, "jobs.scheduler_tick_s", "30"))

	cfg, err := Load(tomlPath, nil)
	testutil.NoError(t, err)
	testutil.True(t, cfg.Jobs.Enabled, "jobs.enabled should be TOML bool")
	testutil.Equal(t, 8, cfg.Jobs.WorkerConcurrency)
	testutil.False(t, cfg.Jobs.SchedulerEnabled, "jobs.scheduler_enabled should be TOML bool")
	testutil.Equal(t, 30, cfg.Jobs.SchedulerTickS)
}

func TestSetValueEdgeFunctionsAllowlist(t *testing.T) {
	dir := t.TempDir()
	tomlPath := filepath.Join(dir, "ayb.toml")

	testutil.NoError(t, SetValue(tomlPath, "edge_functions.fetch_domain_allowlist", "api.example.com,cdn.example.com"))

	cfg, err := Load(tomlPath, nil)
	testutil.NoError(t, err)
	testutil.SliceLen(t, cfg.EdgeFunctions.FetchDomainAllowlist, 2)
	testutil.Equal(t, "api.example.com", cfg.EdgeFunctions.FetchDomainAllowlist[0])
	testutil.Equal(t, "cdn.example.com", cfg.EdgeFunctions.FetchDomainAllowlist[1])
}

func TestSetValueServerAndAdminAllowedIPs(t *testing.T) {
	dir := t.TempDir()
	tomlPath := filepath.Join(dir, "ayb.toml")

	testutil.NoError(t, SetValue(tomlPath, "server.allowed_ips", "203.0.113.10, 198.51.100.0/24"))
	testutil.NoError(t, SetValue(tomlPath, "admin.allowed_ips", "2001:db8::1"))

	cfg, err := Load(tomlPath, nil)
	testutil.NoError(t, err)
	testutil.SliceLen(t, cfg.Server.AllowedIPs, 2)
	testutil.Equal(t, "203.0.113.10", cfg.Server.AllowedIPs[0])
	testutil.Equal(t, "198.51.100.0/24", cfg.Server.AllowedIPs[1])
	testutil.SliceLen(t, cfg.Admin.AllowedIPs, 1)
	testutil.Equal(t, "2001:db8::1", cfg.Admin.AllowedIPs[0])
}

func TestSetValueAuthSMSAllowedCountries(t *testing.T) {
	dir := t.TempDir()
	tomlPath := filepath.Join(dir, "ayb.toml")

	testutil.True(t, IsValidKey("auth.sms_allowed_countries"))
	testutil.NoError(t, SetValue(tomlPath, "auth.sms_allowed_countries", "US, CA, GB"))

	cfg, err := Load(tomlPath, nil)
	testutil.NoError(t, err)
	testutil.SliceLen(t, cfg.Auth.SMSAllowedCountries, 3)
	testutil.Equal(t, "US", cfg.Auth.SMSAllowedCountries[0])
	testutil.Equal(t, "CA", cfg.Auth.SMSAllowedCountries[1])
	testutil.Equal(t, "GB", cfg.Auth.SMSAllowedCountries[2])

	got, err := GetValue(cfg, "auth.sms_allowed_countries")
	testutil.NoError(t, err)
	testutil.Equal(t, "US,CA,GB", got)
}

func TestSetValueTelemetrySampleRateRejectsNaNAndInf(t *testing.T) {
	dir := t.TempDir()
	tomlPath := filepath.Join(dir, "ayb.toml")

	err := SetValue(tomlPath, "telemetry.sample_rate", "NaN")
	testutil.ErrorContains(t, err, "telemetry.sample_rate")

	err = SetValue(tomlPath, "telemetry.sample_rate", "Inf")
	testutil.ErrorContains(t, err, "telemetry.sample_rate")
}

func TestSetValueInvalidKey(t *testing.T) {
	dir := t.TempDir()
	tomlPath := filepath.Join(dir, "ayb.toml")

	err := SetValue(tomlPath, "invalid", "value")
	testutil.ErrorContains(t, err, "invalid key format")
}

func TestSetValuePreservesExisting(t *testing.T) {
	dir := t.TempDir()
	tomlPath := filepath.Join(dir, "ayb.toml")

	// Write initial config.
	err := os.WriteFile(tomlPath, []byte("[server]\nhost = '0.0.0.0'\nport = 8090\n"), 0o644)
	testutil.NoError(t, err)

	// Set port only.
	err = SetValue(tomlPath, "server.port", "3000")
	testutil.NoError(t, err)

	// Host should still be there.
	cfg, err := Load(tomlPath, nil)
	testutil.NoError(t, err)
	testutil.Equal(t, 3000, cfg.Server.Port)
	testutil.Equal(t, "0.0.0.0", cfg.Server.Host)
}

func TestCoerceValue(t *testing.T) {
	tests := []struct {
		key   string
		value string
		want  any
	}{
		{"server.port", "3000", 3000},
		{"auth.enabled", "true", true},
		{"auth.enabled", "false", false},
		{"storage.enabled", "1", true},
		{"storage.enabled", "0", false},
		{"server.host", "myhost", "myhost"},
		{"database.url", "postgresql://localhost", "postgresql://localhost"},
		{"auth.magic_link_enabled", "true", true},
		{"auth.magic_link_enabled", "false", false},
		{"auth.email_mfa_enabled", "true", true},
		{"auth.email_mfa_enabled", "false", false},
		{"audit.enabled", "true", true},
		{"audit.all_tables", "0", false},
		{"auth.magic_link_duration", "300", 300},
		{"auth.oauth_provider.enabled", "true", true},
		{"auth.oauth_provider.access_token_duration", "1200", 1200},
		{"auth.oauth_provider.refresh_token_duration", "86400", 86400},
		{"auth.oauth_provider.auth_code_duration", "180", 180},
		{"jobs.enabled", "true", true},
		{"jobs.scheduler_enabled", "false", false},
		{"jobs.worker_concurrency", "12", 12},
		{"jobs.poll_interval_ms", "250", 250},
		{"jobs.lease_duration_s", "120", 120},
		{"jobs.max_retries_default", "7", 7},
		{"jobs.scheduler_tick_s", "45", 45},
		{"audit.retention_days", "45", 45},
		{"server.tls_staging", "true", true},
		{"server.tls_staging", "false", false},
		{"server.port", "notanumber", "notanumber"}, // falls through to string
	}
	for _, tt := range tests {
		t.Run(tt.key+"="+tt.value, func(t *testing.T) {
			got := coerceValue(tt.key, tt.value)
			testutil.Equal(t, tt.want, got)
		})
	}
}

func TestCoerceValue_Allowlist(t *testing.T) {
	serverAllowlist := coerceValue("server.allowed_ips", "203.0.113.10,198.51.100.0/24")
	serverList, ok := serverAllowlist.([]string)
	testutil.True(t, ok, "expected []string, got %T", serverAllowlist)
	testutil.SliceLen(t, serverList, 2)
	testutil.Equal(t, "203.0.113.10", serverList[0])
	testutil.Equal(t, "198.51.100.0/24", serverList[1])

	adminAllowlist := coerceValue("admin.allowed_ips", "2001:db8::1,2001:db8::2")
	adminList, ok := adminAllowlist.([]string)
	testutil.True(t, ok, "expected []string, got %T", adminAllowlist)
	testutil.SliceLen(t, adminList, 2)
	testutil.Equal(t, "2001:db8::1", adminList[0])
	testutil.Equal(t, "2001:db8::2", adminList[1])
}

func TestCoerceValue_AuditTables(t *testing.T) {
	got := coerceValue("audit.tables", "public.users,public.posts, ")
	list, ok := got.([]string)
	testutil.True(t, ok, "expected []string, got %T", got)
	testutil.SliceLen(t, list, 2)
	testutil.Equal(t, "public.users", list[0])
	testutil.Equal(t, "public.posts", list[1])
}

func TestCoerceValue_EdgeFunctionsAllowlist(t *testing.T) {
	got := coerceValue("edge_functions.fetch_domain_allowlist", "api.example.com, cdn.example.com, ")
	list, ok := got.([]string)
	testutil.True(t, ok, "expected []string, got %T", got)
	testutil.SliceLen(t, list, 2)
	testutil.Equal(t, "api.example.com", list[0])
	testutil.Equal(t, "cdn.example.com", list[1])
}

// --- TLS config tests ---

func TestDefaultTLSFields(t *testing.T) {
	cfg := Default()
	testutil.Equal(t, cfg.Server.TLSEnabled, false)
	testutil.Equal(t, cfg.Server.TLSDomain, "")
	testutil.Equal(t, cfg.Server.TLSCertDir, "")
	testutil.Equal(t, cfg.Server.TLSEmail, "")
	testutil.Equal(t, cfg.Server.TLSStaging, false)
}

func TestValidateTLSDomainAutoEnablesTLS(t *testing.T) {
	cfg := Default()
	cfg.Server.TLSDomain = "api.myapp.com"
	err := cfg.Validate()
	testutil.NoError(t, err)
	testutil.Equal(t, cfg.Server.TLSEnabled, true)
}

func TestValidateTLSEnabledRequiresDomain(t *testing.T) {
	cfg := Default()
	cfg.Server.TLSEnabled = true
	cfg.Server.TLSDomain = ""
	err := cfg.Validate()
	testutil.ErrorContains(t, err, "server.tls_domain is required when TLS is enabled")
}

func TestValidateTLSEnabledWithDomainIsValid(t *testing.T) {
	cfg := Default()
	cfg.Server.TLSEnabled = true
	cfg.Server.TLSDomain = "api.myapp.com"
	err := cfg.Validate()
	testutil.NoError(t, err)
}

func TestApplyEnvTLSDomain(t *testing.T) {
	t.Setenv("AYB_TLS_DOMAIN", "api.example.com")
	cfg := Default()
	err := applyEnv(cfg)
	testutil.NoError(t, err)
	testutil.Equal(t, cfg.Server.TLSDomain, "api.example.com")
}

func TestApplyEnvTLSEmail(t *testing.T) {
	t.Setenv("AYB_TLS_EMAIL", "admin@example.com")
	cfg := Default()
	err := applyEnv(cfg)
	testutil.NoError(t, err)
	testutil.Equal(t, cfg.Server.TLSEmail, "admin@example.com")
}

func TestApplyEnvTLSStaging(t *testing.T) {
	t.Setenv("AYB_TLS_STAGING", "1")
	cfg := Default()
	err := applyEnv(cfg)
	testutil.NoError(t, err)
	testutil.Equal(t, cfg.Server.TLSStaging, true)
}

func TestApplyFlagsTLSDomain(t *testing.T) {
	cfg := Default()
	flags := map[string]string{"tls-domain": "api.example.com"}
	applyFlags(cfg, flags)
	testutil.Equal(t, cfg.Server.TLSDomain, "api.example.com")
}

func TestApplyFlagsTLSDomainEmpty(t *testing.T) {
	cfg := Default()
	flags := map[string]string{"tls-domain": ""}
	applyFlags(cfg, flags)
	testutil.Equal(t, cfg.Server.TLSDomain, "") // empty value should not override
}

func TestIsValidKeyTLS(t *testing.T) {
	testutil.Equal(t, IsValidKey("server.tls_domain"), true)
	testutil.Equal(t, IsValidKey("server.tls_email"), true)
	testutil.Equal(t, IsValidKey("server.tls_cert_dir"), true)
	testutil.Equal(t, IsValidKey("server.tls_enabled"), true)
	testutil.Equal(t, IsValidKey("server.tls_staging"), true)
}

func TestGetValueTLS(t *testing.T) {
	cfg := Default()
	cfg.Server.TLSDomain = "api.example.com"
	cfg.Server.TLSEmail = "admin@example.com"
	cfg.Server.TLSCertDir = "/home/user/.ayb/certs"
	cfg.Server.TLSStaging = true

	val, err := GetValue(cfg, "server.tls_domain")
	testutil.NoError(t, err)
	testutil.Equal(t, val, "api.example.com")

	val, err = GetValue(cfg, "server.tls_email")
	testutil.NoError(t, err)
	testutil.Equal(t, val, "admin@example.com")

	val, err = GetValue(cfg, "server.tls_cert_dir")
	testutil.NoError(t, err)
	testutil.Equal(t, val, "/home/user/.ayb/certs")

	val, err = GetValue(cfg, "server.tls_enabled")
	testutil.NoError(t, err)
	testutil.Equal(t, val, false)

	val, err = GetValue(cfg, "server.tls_staging")
	testutil.NoError(t, err)
	testutil.Equal(t, val, true)
}

func TestGenerateDefaultContainsTLSSection(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ayb.toml")
	err := GenerateDefault(path)
	testutil.NoError(t, err)

	data, err := os.ReadFile(path)
	testutil.NoError(t, err)
	testutil.Contains(t, string(data), "tls_domain")
	testutil.Contains(t, string(data), "tls_email")
	testutil.Contains(t, string(data), "tls_staging")
}

func TestLoadTLSFromFile(t *testing.T) {
	dir := t.TempDir()
	tomlPath := filepath.Join(dir, "ayb.toml")
	content := `
[server]
tls_domain = "api.myapp.com"
tls_email = "ops@myapp.com"
tls_cert_dir = "/var/lib/ayb/certs"
`
	err := os.WriteFile(tomlPath, []byte(content), 0o644)
	testutil.NoError(t, err)

	cfg, err := Load(tomlPath, nil)
	testutil.NoError(t, err)
	testutil.Equal(t, cfg.Server.TLSDomain, "api.myapp.com")
	testutil.Equal(t, cfg.Server.TLSEmail, "ops@myapp.com")
	testutil.Equal(t, cfg.Server.TLSCertDir, "/var/lib/ayb/certs")
	testutil.Equal(t, cfg.Server.TLSEnabled, true) // auto-set by Validate
}

// TestLoadEnvTLSDomainAutoEnablesTLS verifies that setting AYB_TLS_DOMAIN via env
// results in TLSEnabled=true through the full Load() pipeline (applyEnv + Validate).
// TestApplyEnvTLSDomain only tests applyEnv in isolation; this tests the end-to-end path.
func TestLoadEnvTLSDomainAutoEnablesTLS(t *testing.T) {
	t.Setenv("AYB_TLS_DOMAIN", "api.example.com")
	cfg, err := Load("/nonexistent/ayb.toml", nil)
	testutil.NoError(t, err)
	testutil.Equal(t, "api.example.com", cfg.Server.TLSDomain)
	testutil.Equal(t, true, cfg.Server.TLSEnabled) // Validate auto-sets this
}

// TestLoadFlagTLSDomainAutoEnablesTLS verifies that passing tls-domain via CLI flags
// results in TLSEnabled=true through the full Load() pipeline (applyFlags + Validate).
// TestApplyFlagsTLSDomain only tests applyFlags in isolation; this tests the end-to-end path.
func TestLoadFlagTLSDomainAutoEnablesTLS(t *testing.T) {
	flags := map[string]string{"tls-domain": "api.example.com"}
	cfg, err := Load("/nonexistent/ayb.toml", flags)
	testutil.NoError(t, err)
	testutil.Equal(t, "api.example.com", cfg.Server.TLSDomain)
	testutil.Equal(t, true, cfg.Server.TLSEnabled) // Validate auto-sets this
}

// --- SMS config tests ---

// validSMSConfig returns a Default config with auth enabled and SMS enabled with the log provider.
func validSMSConfig(t *testing.T) *Config {
	t.Helper()
	cfg := Default()
	cfg.Auth.Enabled = true
	cfg.Auth.JWTSecret = "this-is-a-secret-that-is-at-least-32-characters-long"
	cfg.Auth.SMSEnabled = true
	cfg.Auth.SMSProvider = "log"
	return cfg
}

func TestSMSConfigDefaults(t *testing.T) {
	cfg := Default()
	testutil.Equal(t, false, cfg.Auth.SMSEnabled)
	testutil.Equal(t, "log", cfg.Auth.SMSProvider)
	testutil.Equal(t, 6, cfg.Auth.SMSCodeLength)
	testutil.Equal(t, 300, cfg.Auth.SMSCodeExpiry)
	testutil.Equal(t, 3, cfg.Auth.SMSMaxAttempts)
	testutil.Equal(t, 1000, cfg.Auth.SMSDailyLimit)
	testutil.SliceLen(t, cfg.Auth.SMSAllowedCountries, 2)
	testutil.Equal(t, "US", cfg.Auth.SMSAllowedCountries[0])
	testutil.Equal(t, "CA", cfg.Auth.SMSAllowedCountries[1])
}

func TestSMSConfigValidation_RequiresAuthEnabled(t *testing.T) {
	cfg := Default()
	cfg.Auth.SMSEnabled = true
	cfg.Auth.Enabled = false
	err := cfg.Validate()
	testutil.ErrorContains(t, err, "sms_enabled requires auth.enabled")
}

func TestEmailMFAConfigDefaults(t *testing.T) {
	cfg := Default()
	testutil.False(t, cfg.Auth.EmailMFAEnabled, "email_mfa_enabled should default to false")
}

func TestEmailMFAConfigValidation_RequiresAuthEnabled(t *testing.T) {
	cfg := Default()
	cfg.Auth.EmailMFAEnabled = true
	cfg.Auth.Enabled = false
	err := cfg.Validate()
	testutil.ErrorContains(t, err, "email_mfa_enabled requires auth.enabled")
}

func TestSMSConfigValidation_UnknownProvider(t *testing.T) {
	cfg := validSMSConfig(t)
	cfg.Auth.SMSProvider = "carrier_pigeon"
	testutil.ErrorContains(t, cfg.Validate(), "sms_provider")
}

func TestSMSConfigValidation_TwilioRequiresCredentials(t *testing.T) {
	cfg := validSMSConfig(t)
	cfg.Auth.SMSProvider = "twilio"
	testutil.ErrorContains(t, cfg.Validate(), "twilio_sid")
}

func TestSMSConfigValidation_CodeLengthBounds(t *testing.T) {
	cfg := validSMSConfig(t)
	cfg.Auth.SMSCodeLength = 3
	testutil.ErrorContains(t, cfg.Validate(), "sms_code_length")
	cfg.Auth.SMSCodeLength = 9
	testutil.ErrorContains(t, cfg.Validate(), "sms_code_length")
	cfg.Auth.SMSCodeLength = 6
	testutil.NoError(t, cfg.Validate())
}

func TestSMSConfigValidation_ExpiryBounds(t *testing.T) {
	cfg := validSMSConfig(t)
	cfg.Auth.SMSCodeExpiry = 59
	testutil.ErrorContains(t, cfg.Validate(), "sms_code_expiry")
	cfg.Auth.SMSCodeExpiry = 601
	testutil.ErrorContains(t, cfg.Validate(), "sms_code_expiry")
}

func TestSMSConfigValidation_DailyLimitBounds(t *testing.T) {
	cfg := validSMSConfig(t)
	cfg.Auth.SMSDailyLimit = -1
	testutil.ErrorContains(t, cfg.Validate(), "sms_daily_limit")
	cfg.Auth.SMSDailyLimit = 0 // 0 = unlimited — valid
	testutil.NoError(t, cfg.Validate())
}

func TestSMSConfigValidation_AllowedCountries(t *testing.T) {
	cfg := validSMSConfig(t)
	cfg.Auth.SMSAllowedCountries = []string{"XX"}
	testutil.ErrorContains(t, cfg.Validate(), "sms_allowed_countries")
}

// --- New provider config validation tests (Stage 6, Step 8) ---

func TestValidate_SMSProvider_Plivo(t *testing.T) {
	cfg := validSMSConfig(t)
	cfg.Auth.SMSProvider = "plivo"

	// Missing all credentials.
	testutil.ErrorContains(t, cfg.Validate(), "plivo_auth_id")

	// Missing auth_token and from.
	cfg.Auth.PlivoAuthID = "PLIVO_ID"
	testutil.ErrorContains(t, cfg.Validate(), "plivo_auth_token")

	cfg.Auth.PlivoAuthToken = "PLIVO_TOKEN"
	testutil.ErrorContains(t, cfg.Validate(), "plivo_from")

	// All set — valid.
	cfg.Auth.PlivoFrom = "+15551234567"
	testutil.NoError(t, cfg.Validate())
}

func TestValidate_SMSProvider_Telnyx(t *testing.T) {
	cfg := validSMSConfig(t)
	cfg.Auth.SMSProvider = "telnyx"

	testutil.ErrorContains(t, cfg.Validate(), "telnyx_api_key")

	cfg.Auth.TelnyxAPIKey = "KEY_123"
	testutil.ErrorContains(t, cfg.Validate(), "telnyx_from")

	cfg.Auth.TelnyxFrom = "+15551234567"
	testutil.NoError(t, cfg.Validate())
}

func TestValidate_SMSProvider_MSG91(t *testing.T) {
	cfg := validSMSConfig(t)
	cfg.Auth.SMSProvider = "msg91"

	testutil.ErrorContains(t, cfg.Validate(), "msg91_auth_key")

	cfg.Auth.MSG91AuthKey = "AUTH_KEY"
	testutil.ErrorContains(t, cfg.Validate(), "msg91_template_id")

	cfg.Auth.MSG91TemplateID = "TMPL_123"
	testutil.NoError(t, cfg.Validate())
}

func TestValidate_SMSProvider_SNS(t *testing.T) {
	cfg := validSMSConfig(t)
	cfg.Auth.SMSProvider = "sns"

	testutil.ErrorContains(t, cfg.Validate(), "aws_region")

	cfg.Auth.AWSRegion = "us-east-1"
	testutil.NoError(t, cfg.Validate())
}

func TestValidate_SMSProvider_Vonage(t *testing.T) {
	cfg := validSMSConfig(t)
	cfg.Auth.SMSProvider = "vonage"

	testutil.ErrorContains(t, cfg.Validate(), "vonage_api_key")

	cfg.Auth.VonageAPIKey = "KEY"
	testutil.ErrorContains(t, cfg.Validate(), "vonage_api_secret")

	cfg.Auth.VonageAPISecret = "SECRET"
	testutil.ErrorContains(t, cfg.Validate(), "vonage_from")

	cfg.Auth.VonageFrom = "+15551234567"
	testutil.NoError(t, cfg.Validate())
}

func TestValidate_SMSProvider_Webhook(t *testing.T) {
	cfg := validSMSConfig(t)
	cfg.Auth.SMSProvider = "webhook"

	testutil.ErrorContains(t, cfg.Validate(), "sms_webhook_url")

	cfg.Auth.SMSWebhookURL = "https://example.com/sms"
	testutil.ErrorContains(t, cfg.Validate(), "sms_webhook_secret")

	cfg.Auth.SMSWebhookSecret = "whsec_abc"
	testutil.NoError(t, cfg.Validate())
}

func TestValidate_SMSProvider_Invalid(t *testing.T) {
	cfg := validSMSConfig(t)
	cfg.Auth.SMSProvider = "carrier_pigeon"
	err := cfg.Validate()
	testutil.ErrorContains(t, err, "sms_provider")
	// The error message should list all valid providers.
	testutil.ErrorContains(t, err, "plivo")
	testutil.ErrorContains(t, err, "telnyx")
	testutil.ErrorContains(t, err, "vonage")
	testutil.ErrorContains(t, err, "sns")
	testutil.ErrorContains(t, err, "msg91")
	testutil.ErrorContains(t, err, "webhook")
}

func TestNewProviderEnvVarOverrides(t *testing.T) {
	t.Setenv("AYB_AUTH_PLIVO_AUTH_ID", "env_plivo_id")
	t.Setenv("AYB_AUTH_PLIVO_AUTH_TOKEN", "env_plivo_token")
	t.Setenv("AYB_AUTH_PLIVO_FROM", "+15559990000")
	t.Setenv("AYB_AUTH_TELNYX_API_KEY", "env_telnyx_key")
	t.Setenv("AYB_AUTH_TELNYX_FROM", "+15559990001")
	t.Setenv("AYB_AUTH_MSG91_AUTH_KEY", "env_msg91_key")
	t.Setenv("AYB_AUTH_MSG91_TEMPLATE_ID", "env_tmpl_id")
	t.Setenv("AYB_AUTH_AWS_REGION", "eu-west-1")
	t.Setenv("AYB_AUTH_VONAGE_API_KEY", "env_vonage_key")
	t.Setenv("AYB_AUTH_VONAGE_API_SECRET", "env_vonage_secret")
	t.Setenv("AYB_AUTH_VONAGE_FROM", "+15559990002")
	t.Setenv("AYB_AUTH_SMS_WEBHOOK_URL", "https://env.example.com/sms")
	t.Setenv("AYB_AUTH_SMS_WEBHOOK_SECRET", "env_webhook_secret")

	cfg := Default()
	err := applyEnv(cfg)
	testutil.NoError(t, err)

	testutil.Equal(t, "env_plivo_id", cfg.Auth.PlivoAuthID)
	testutil.Equal(t, "env_plivo_token", cfg.Auth.PlivoAuthToken)
	testutil.Equal(t, "+15559990000", cfg.Auth.PlivoFrom)
	testutil.Equal(t, "env_telnyx_key", cfg.Auth.TelnyxAPIKey)
	testutil.Equal(t, "+15559990001", cfg.Auth.TelnyxFrom)
	testutil.Equal(t, "env_msg91_key", cfg.Auth.MSG91AuthKey)
	testutil.Equal(t, "env_tmpl_id", cfg.Auth.MSG91TemplateID)
	testutil.Equal(t, "eu-west-1", cfg.Auth.AWSRegion)
	testutil.Equal(t, "env_vonage_key", cfg.Auth.VonageAPIKey)
	testutil.Equal(t, "env_vonage_secret", cfg.Auth.VonageAPISecret)
	testutil.Equal(t, "+15559990002", cfg.Auth.VonageFrom)
	testutil.Equal(t, "https://env.example.com/sms", cfg.Auth.SMSWebhookURL)
	testutil.Equal(t, "env_webhook_secret", cfg.Auth.SMSWebhookSecret)
}

func TestSMSConfigEnvVarOverride(t *testing.T) {
	t.Setenv("AYB_AUTH_ENABLED", "true")
	t.Setenv("AYB_AUTH_JWT_SECRET", "this-is-a-secret-that-is-at-least-32-characters-long")
	t.Setenv("AYB_AUTH_SMS_ENABLED", "true")
	t.Setenv("AYB_AUTH_TWILIO_SID", "ACtest123")
	t.Setenv("AYB_AUTH_TWILIO_TOKEN", "token")
	t.Setenv("AYB_AUTH_TWILIO_FROM", "+15550000000")
	t.Setenv("AYB_AUTH_SMS_PROVIDER", "twilio")

	cfg, err := Load("/nonexistent/ayb.toml", nil)
	testutil.NoError(t, err)
	testutil.Equal(t, true, cfg.Auth.SMSEnabled)
	testutil.Equal(t, "ACtest123", cfg.Auth.TwilioSID)
	testutil.Equal(t, "token", cfg.Auth.TwilioToken)
	testutil.Equal(t, "+15550000000", cfg.Auth.TwilioFrom)
	testutil.Equal(t, "twilio", cfg.Auth.SMSProvider)
}

func TestEmailMFAConfigEnvVarOverride(t *testing.T) {
	t.Setenv("AYB_AUTH_ENABLED", "true")
	t.Setenv("AYB_AUTH_JWT_SECRET", "this-is-a-secret-that-is-at-least-32-characters-long")
	t.Setenv("AYB_AUTH_EMAIL_MFA_ENABLED", "true")

	cfg, err := Load("/nonexistent/ayb.toml", nil)
	testutil.NoError(t, err)
	testutil.True(t, cfg.Auth.EmailMFAEnabled, "email MFA should be enabled from env")
}

// TestGetValueCoversAllValidKeys verifies every key in validKeys has a
// corresponding GetValue handler — prevents "unknown configuration key"
// errors for keys that IsValidKey reports as valid.
func TestGetValueCoversAllValidKeys(t *testing.T) {
	cfg := Default()
	for key := range validKeys {
		t.Run(key, func(t *testing.T) {
			_, err := GetValue(cfg, key)
			testutil.NoError(t, err)
		})
	}
}

func TestParseRateLimitSpec(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantLimit int
		wantDur   time.Duration
		wantErr   string
	}{
		{name: "per minute", input: "10/min", wantLimit: 10, wantDur: time.Minute},
		{name: "per hour", input: "250/hour", wantLimit: 250, wantDur: time.Hour},
		{name: "invalid format", input: "10/sec", wantErr: "expected format N/min or N/hour"},
		{name: "non numeric", input: "x/min", wantErr: "count must be a positive integer"},
		{name: "zero count", input: "0/min", wantErr: "count must be a positive integer"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotLimit, gotDur, err := ParseRateLimitSpec(tt.input)
			if tt.wantErr != "" {
				testutil.ErrorContains(t, err, tt.wantErr)
				return
			}

			testutil.NoError(t, err)
			testutil.Equal(t, tt.wantLimit, gotLimit)
			testutil.Equal(t, tt.wantDur, gotDur)
		})
	}
}

func TestLoadVaultMasterKeyFromEnv(t *testing.T) {
	t.Setenv("AYB_VAULT_MASTER_KEY", "vault-env-key")

	cfg, err := Load("/nonexistent/ayb.toml", nil)
	testutil.NoError(t, err)
	testutil.Equal(t, "vault-env-key", cfg.Vault.MasterKey)
}

func TestGenerateDefaultIncludesVaultSection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ayb.toml")
	testutil.NoError(t, GenerateDefault(path))

	data, err := os.ReadFile(path)
	testutil.NoError(t, err)
	content := string(data)
	testutil.Contains(t, content, "[vault]")
	testutil.Contains(t, content, "master_key")
}

func TestVaultMasterKeyValidKeyAndGetValue(t *testing.T) {
	testutil.True(t, IsValidKey("vault.master_key"), "vault.master_key should be valid")

	cfg := Default()
	cfg.Vault.MasterKey = "vault-config-key"

	val, err := GetValue(cfg, "vault.master_key")
	testutil.NoError(t, err)
	testutil.Equal(t, "vault-config-key", val)
}

func TestTelemetryKeysValidAndGetValue(t *testing.T) {
	cfg := Default()
	cfg.Telemetry.Enabled = true
	cfg.Telemetry.OTLPEndpoint = "localhost:4317"
	cfg.Telemetry.ServiceName = "ayb-test"
	cfg.Telemetry.SampleRate = 0.25

	testutil.True(t, IsValidKey("telemetry.enabled"))
	testutil.True(t, IsValidKey("telemetry.otlp_endpoint"))
	testutil.True(t, IsValidKey("telemetry.service_name"))
	testutil.True(t, IsValidKey("telemetry.sample_rate"))

	v, err := GetValue(cfg, "telemetry.enabled")
	testutil.NoError(t, err)
	testutil.Equal(t, true, v)

	v, err = GetValue(cfg, "telemetry.otlp_endpoint")
	testutil.NoError(t, err)
	testutil.Equal(t, "localhost:4317", v)

	v, err = GetValue(cfg, "telemetry.service_name")
	testutil.NoError(t, err)
	testutil.Equal(t, "ayb-test", v)

	v, err = GetValue(cfg, "telemetry.sample_rate")
	testutil.NoError(t, err)
	testutil.Equal(t, 0.25, v)
}

func TestApplyEnvTelemetryConfig(t *testing.T) {
	t.Setenv("AYB_TELEMETRY_ENABLED", "1")
	t.Setenv("AYB_TELEMETRY_OTLP_ENDPOINT", "collector.internal:4317")
	t.Setenv("AYB_TELEMETRY_SERVICE_NAME", "ayb-env")
	t.Setenv("AYB_TELEMETRY_SAMPLE_RATE", "0.35")

	cfg := Default()
	err := applyEnv(cfg)
	testutil.NoError(t, err)

	testutil.Equal(t, true, cfg.Telemetry.Enabled)
	testutil.Equal(t, "collector.internal:4317", cfg.Telemetry.OTLPEndpoint)
	testutil.Equal(t, "ayb-env", cfg.Telemetry.ServiceName)
	testutil.Equal(t, 0.35, cfg.Telemetry.SampleRate)
}

func TestApplyEnvTelemetrySampleRateInvalid(t *testing.T) {
	t.Setenv("AYB_TELEMETRY_SAMPLE_RATE", "not-a-float")
	cfg := Default()
	err := applyEnv(cfg)
	testutil.ErrorContains(t, err, "AYB_TELEMETRY_SAMPLE_RATE")
}

func TestValidateTelemetrySampleRateRejectsNaN(t *testing.T) {
	cfg := Default()
	cfg.Telemetry.SampleRate = math.NaN()
	testutil.ErrorContains(t, cfg.Validate(), "telemetry.sample_rate")
}

func TestValidateTelemetrySampleRateRejectsInf(t *testing.T) {
	cfg := Default()
	cfg.Telemetry.SampleRate = math.Inf(1)
	testutil.ErrorContains(t, cfg.Validate(), "telemetry.sample_rate")
}

func TestMaskedCopyMasksVaultMasterKey(t *testing.T) {
	cfg := Default()
	cfg.Vault.MasterKey = "sensitive-vault-key"

	masked := cfg.MaskedCopy()
	testutil.Equal(t, "***", masked.Vault.MasterKey)
}

func TestMaskedCopyMasksSupportWebhookSecret(t *testing.T) {
	cfg := Default()
	cfg.Support.WebhookSecret = "support-whsec"

	masked := cfg.MaskedCopy()
	testutil.Equal(t, "***", masked.Support.WebhookSecret)
}

func TestTLSStagingConfigParsing(t *testing.T) {
	t.Parallel()

	toml := `
[server]
tls_domain = "example.com"
tls_staging = true
tls_email = "admin@example.com"
`
	cfg, err := ParseTOML([]byte(toml))
	testutil.NoError(t, err)
	testutil.True(t, cfg.Server.TLSStaging, "expected tls_staging to be true")
	testutil.True(t, cfg.Server.TLSEnabled, "tls_enabled should be auto-set when domain is present")
	testutil.Equal(t, "example.com", cfg.Server.TLSDomain)
}

func TestTLSStagingEnvVar(t *testing.T) {
	t.Setenv("AYB_TLS_STAGING", "true")

	cfg := &Config{}
	testutil.NoError(t, cfg.ApplyEnvironment())
	testutil.True(t, cfg.Server.TLSStaging, "AYB_TLS_STAGING=true should set TLSStaging")
}

func TestPublicBaseURL_TLSEnabled(t *testing.T) {
	t.Parallel()

	cfg := &Config{Server: ServerConfig{
		Host:       "0.0.0.0",
		Port:       8090,
		TLSEnabled: true,
		TLSDomain:  "api.example.com",
	}}
	testutil.Equal(t, "https://api.example.com", cfg.PublicBaseURL())
}

func TestPublicBaseURL_NoTLS(t *testing.T) {
	t.Parallel()

	cfg := &Config{Server: ServerConfig{
		Host: "0.0.0.0",
		Port: 8090,
	}}
	testutil.Equal(t, "http://localhost:8090", cfg.PublicBaseURL())
}

func TestParseTOML_SAMLProviders(t *testing.T) {
	t.Parallel()

	toml := `
[auth]
enabled = true
jwt_secret = "this-is-a-secret-that-is-at-least-32-characters-long"

[[auth.saml_providers]]
enabled = true
name = "okta"
entity_id = "https://sp.example.com/saml"
idp_metadata_url = "https://idp.example.com/metadata"
sp_cert_file = "/tmp/sp-cert.pem"
sp_key_file = "/tmp/sp-key.pem"

[auth.saml_providers.attribute_mapping]
email = "mail"
name = "displayName"
groups = "memberOf"
`
	cfg, err := ParseTOML([]byte(toml))
	testutil.NoError(t, err)
	testutil.SliceLen(t, cfg.Auth.SAMLProviders, 1)
	testutil.Equal(t, true, cfg.Auth.SAMLProviders[0].Enabled)
	testutil.Equal(t, "okta", cfg.Auth.SAMLProviders[0].Name)
	testutil.Equal(t, "https://sp.example.com/saml", cfg.Auth.SAMLProviders[0].EntityID)
	testutil.Equal(t, "https://idp.example.com/metadata", cfg.Auth.SAMLProviders[0].IDPMetadataURL)
	testutil.Equal(t, "/tmp/sp-cert.pem", cfg.Auth.SAMLProviders[0].SPCertFile)
	testutil.Equal(t, "/tmp/sp-key.pem", cfg.Auth.SAMLProviders[0].SPKeyFile)
	testutil.Equal(t, "mail", cfg.Auth.SAMLProviders[0].AttributeMapping["email"])
}

func TestParseTOMLDatabaseReplicas(t *testing.T) {
	t.Parallel()

	cfg, err := ParseTOML([]byte(`
[database]
url = "postgresql://primary/db"

[[database.replicas]]
url = "postgresql://replica-1/db"
weight = 2
max_lag_bytes = 1234

[[database.replicas]]
url = "postgresql://replica-2/db"
weight = 4
max_lag_bytes = 5678
`))
	testutil.NoError(t, err)
	testutil.SliceLen(t, cfg.Database.Replicas, 2)
	testutil.Equal(t, "postgresql://replica-1/db", cfg.Database.Replicas[0].URL)
	testutil.Equal(t, 2, cfg.Database.Replicas[0].Weight)
	testutil.Equal(t, int64(1234), cfg.Database.Replicas[0].MaxLagBytes)
	testutil.Equal(t, "postgresql://replica-2/db", cfg.Database.Replicas[1].URL)
	testutil.Equal(t, 4, cfg.Database.Replicas[1].Weight)
	testutil.Equal(t, int64(5678), cfg.Database.Replicas[1].MaxLagBytes)
}

// --- Rate Limit Config Section Tests ---

func TestRateLimitConfigDefaults(t *testing.T) {
	cfg := Default()

	testutil.Equal(t, "100/min", cfg.RateLimit.API)
	testutil.Equal(t, "30/min", cfg.RateLimit.APIAnonymous)
}

func TestRateLimitConfigTOMLParsing(t *testing.T) {
	toml := `
[rate_limit]
api = "200/min"
api_anonymous = "20/hour"
`
	cfg, err := ParseTOML([]byte(toml))
	testutil.NoError(t, err)
	testutil.Equal(t, "200/min", cfg.RateLimit.API)
	testutil.Equal(t, "20/hour", cfg.RateLimit.APIAnonymous)
}

func TestRateLimitConfigEnvOverrides(t *testing.T) {
	t.Setenv("AYB_RATE_LIMIT_API", "500/hour")
	t.Setenv("AYB_RATE_LIMIT_API_ANONYMOUS", "10/min")

	cfg := Default()
	testutil.NoError(t, cfg.ApplyEnvironment())

	testutil.Equal(t, "500/hour", cfg.RateLimit.API)
	testutil.Equal(t, "10/min", cfg.RateLimit.APIAnonymous)
}

func TestRateLimitConfigValidation(t *testing.T) {
	cfg := Default()

	// Valid specs should pass.
	testutil.NoError(t, cfg.Validate())

	// Invalid API spec.
	cfg.RateLimit.API = "bad"
	testutil.ErrorContains(t, cfg.Validate(), "rate_limit.api")

	// Fix API, invalid anonymous spec.
	cfg.RateLimit.API = "100/min"
	cfg.RateLimit.APIAnonymous = "xyz"
	testutil.ErrorContains(t, cfg.Validate(), "rate_limit.api_anonymous")
}

func TestRateLimitConfigGetValueAndIsValidKey(t *testing.T) {
	cfg := Default()

	v, err := GetValue(cfg, "rate_limit.api")
	testutil.NoError(t, err)
	testutil.Equal(t, "100/min", v)

	v, err = GetValue(cfg, "rate_limit.api_anonymous")
	testutil.NoError(t, err)
	testutil.Equal(t, "30/min", v)

	testutil.True(t, IsValidKey("rate_limit.api"), "rate_limit.api should be valid")
	testutil.True(t, IsValidKey("rate_limit.api_anonymous"), "rate_limit.api_anonymous should be valid")
}

func TestStorageDefaultQuotaMB(t *testing.T) {
	cfg := Default()
	testutil.Equal(t, 100, cfg.Storage.DefaultQuotaMB)
}

func TestStorageDefaultQuotaBytes(t *testing.T) {
	tests := []struct {
		name string
		mb   int
		want int64
	}{
		{"default", 100, 100 * 1024 * 1024},
		{"custom", 500, 500 * 1024 * 1024},
		{"zero_uses_100", 0, 100 * 1024 * 1024},
		{"negative_uses_100", -1, 100 * 1024 * 1024},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &StorageConfig{DefaultQuotaMB: tt.mb}
			testutil.Equal(t, tt.want, cfg.DefaultQuotaBytes())
		})
	}
}

func TestApplyEnvStorageDefaultQuotaMB(t *testing.T) {
	t.Setenv("AYB_STORAGE_DEFAULT_QUOTA_MB", "250")
	cfg := Default()
	err := applyEnv(cfg)
	testutil.NoError(t, err)
	testutil.Equal(t, 250, cfg.Storage.DefaultQuotaMB)
}

// ── Email Policy Config defaults ────────────────────────────────────────

func TestEffectiveAllowedFrom_FallbackToDefault(t *testing.T) {
	t.Parallel()
	ec := &EmailConfig{From: "default@example.com"}
	got := ec.EffectiveAllowedFrom()
	testutil.SliceLen(t, got, 1)
	testutil.Equal(t, "default@example.com", got[0])
}

func TestEffectiveAllowedFrom_ExplicitList(t *testing.T) {
	t.Parallel()
	ec := &EmailConfig{
		From:   "default@example.com",
		Policy: EmailPolicyConfig{AllowedFromAddresses: []string{"a@x.com", "b@x.com"}},
	}
	got := ec.EffectiveAllowedFrom()
	testutil.SliceLen(t, got, 2)
	testutil.Equal(t, "a@x.com", got[0])
	testutil.Equal(t, "b@x.com", got[1])
}

func TestEffectiveAllowedFrom_EmptyFromAndList(t *testing.T) {
	t.Parallel()
	ec := &EmailConfig{}
	got := ec.EffectiveAllowedFrom()
	testutil.SliceLen(t, got, 0)
}

func TestEffectiveMaxRecipients_Default(t *testing.T) {
	t.Parallel()
	pc := &EmailPolicyConfig{}
	testutil.Equal(t, 50, pc.EffectiveMaxRecipients())
}

func TestEffectiveMaxRecipients_Custom(t *testing.T) {
	t.Parallel()
	pc := &EmailPolicyConfig{MaxRecipientsPerRequest: 10}
	testutil.Equal(t, 10, pc.EffectiveMaxRecipients())
}

func TestEffectiveSendRateLimit_Default(t *testing.T) {
	t.Parallel()
	pc := &EmailPolicyConfig{}
	testutil.Equal(t, 100, pc.EffectiveSendRateLimit())
}

func TestEffectiveSendRateLimit_Custom(t *testing.T) {
	t.Parallel()
	pc := &EmailPolicyConfig{SendRateLimitPerKey: 200}
	testutil.Equal(t, 200, pc.EffectiveSendRateLimit())
}

func TestEffectiveSendRateWindow_Default(t *testing.T) {
	t.Parallel()
	pc := &EmailPolicyConfig{}
	testutil.Equal(t, 3600, pc.EffectiveSendRateWindow())
}

func TestEffectiveSendRateWindow_Custom(t *testing.T) {
	t.Parallel()
	pc := &EmailPolicyConfig{SendRateLimitWindow: 600}
	testutil.Equal(t, 600, pc.EffectiveSendRateWindow())
}

func TestRealtimeConfigValidationBounds(t *testing.T) {
	t.Parallel()

	cfg := Default()
	cfg.Realtime.MaxConnectionsPerUser = 0
	testutil.ErrorContains(t, cfg.Validate(), "realtime.max_connections_per_user")

	cfg = Default()
	cfg.Realtime.HeartbeatIntervalSeconds = 0
	testutil.ErrorContains(t, cfg.Validate(), "realtime.heartbeat_interval_seconds")

	cfg = Default()
	cfg.Realtime.BroadcastRateLimitPerSecond = 0
	testutil.ErrorContains(t, cfg.Validate(), "realtime.broadcast_rate_limit_per_second")

	cfg = Default()
	cfg.Realtime.BroadcastMaxMessageBytes = 0
	testutil.ErrorContains(t, cfg.Validate(), "realtime.broadcast_max_message_bytes")

	cfg = Default()
	cfg.Realtime.PresenceLeaveTimeoutSeconds = 0
	testutil.ErrorContains(t, cfg.Validate(), "realtime.presence_leave_timeout_seconds")
}

func TestParseTOMLRealtimeOverrides(t *testing.T) {
	t.Parallel()

	cfg, err := ParseTOML([]byte(`
[realtime]
max_connections_per_user = 42
heartbeat_interval_seconds = 30
broadcast_rate_limit_per_second = 250
broadcast_max_message_bytes = 524288
presence_leave_timeout_seconds = 15
`))
	testutil.NoError(t, err)
	testutil.Equal(t, 42, cfg.Realtime.MaxConnectionsPerUser)
	testutil.Equal(t, 30, cfg.Realtime.HeartbeatIntervalSeconds)
	testutil.Equal(t, 250, cfg.Realtime.BroadcastRateLimitPerSecond)
	testutil.Equal(t, 524288, cfg.Realtime.BroadcastMaxMessageBytes)
	testutil.Equal(t, 15, cfg.Realtime.PresenceLeaveTimeoutSeconds)
}

func TestApplyEnvRealtimeOverrides(t *testing.T) {
	t.Setenv("AYB_REALTIME_MAX_CONNECTIONS_PER_USER", "222")
	t.Setenv("AYB_REALTIME_HEARTBEAT_INTERVAL_SECONDS", "44")
	t.Setenv("AYB_REALTIME_BROADCAST_RATE_LIMIT_PER_SECOND", "333")
	t.Setenv("AYB_REALTIME_BROADCAST_MAX_MESSAGE_BYTES", "777777")
	t.Setenv("AYB_REALTIME_PRESENCE_LEAVE_TIMEOUT_SECONDS", "66")

	cfg := Default()
	err := applyEnv(cfg)
	testutil.NoError(t, err)

	testutil.Equal(t, 222, cfg.Realtime.MaxConnectionsPerUser)
	testutil.Equal(t, 44, cfg.Realtime.HeartbeatIntervalSeconds)
	testutil.Equal(t, 333, cfg.Realtime.BroadcastRateLimitPerSecond)
	testutil.Equal(t, 777777, cfg.Realtime.BroadcastMaxMessageBytes)
	testutil.Equal(t, 66, cfg.Realtime.PresenceLeaveTimeoutSeconds)
}

func TestApplyEnvRealtimeOverridesInvalidValue(t *testing.T) {
	for _, envName := range []string{
		"AYB_REALTIME_MAX_CONNECTIONS_PER_USER",
		"AYB_REALTIME_HEARTBEAT_INTERVAL_SECONDS",
		"AYB_REALTIME_BROADCAST_RATE_LIMIT_PER_SECOND",
		"AYB_REALTIME_BROADCAST_MAX_MESSAGE_BYTES",
		"AYB_REALTIME_PRESENCE_LEAVE_TIMEOUT_SECONDS",
	} {
		t.Run(envName, func(t *testing.T) {
			t.Setenv(envName, "not-an-int")

			cfg := Default()
			err := applyEnv(cfg)
			testutil.ErrorContains(t, err, envName)
		})
	}
}

func TestGenerateDefaultIncludesRealtimeSection(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "ayb.toml")
	testutil.NoError(t, GenerateDefault(path))
	data, err := os.ReadFile(path)
	testutil.NoError(t, err)
	content := string(data)
	for _, want := range []string{
		"[realtime]",
		"max_connections_per_user = 100",
		"heartbeat_interval_seconds = 25",
		"broadcast_rate_limit_per_second = 100",
		"broadcast_max_message_bytes = 262144",
		"presence_leave_timeout_seconds = 10",
	} {
		testutil.Contains(t, content, want)
	}
}

func TestDefaultSupportConfig(t *testing.T) {
	cfg := Default()
	testutil.Equal(t, false, cfg.Support.Enabled)
	testutil.Equal(t, "", cfg.Support.InboundEmailDomain)
	testutil.Equal(t, "", cfg.Support.WebhookSecret)
}
