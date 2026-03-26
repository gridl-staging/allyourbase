// Package config Stub summary for /Users/stuart/parallel_development/allyourbase_dev/mar19_03_go_code_quality_refactoring/allyourbase_dev/internal/config/config_env_sections.go.
package config

import (
	"os"
	"strings"
)

// TODO: Document applyServerEnv.
func applyServerEnv(cfg *Config) error {
	if v := os.Getenv("AYB_SERVER_HOST"); v != "" {
		cfg.Server.Host = v
	}
	if err := envInt("AYB_SERVER_PORT", &cfg.Server.Port); err != nil {
		return err
	}
	if v := os.Getenv("AYB_TLS_DOMAIN"); v != "" {
		cfg.Server.TLSDomain = v
	}
	if v := os.Getenv("AYB_TLS_EMAIL"); v != "" {
		cfg.Server.TLSEmail = v
	}
	if v := os.Getenv("AYB_TLS_STAGING"); v != "" {
		cfg.Server.TLSStaging = v == "true" || v == "1"
	}
	if v := os.Getenv("AYB_SERVER_SITE_URL"); v != "" {
		cfg.Server.SiteURL = v
	}
	if v := os.Getenv("AYB_SERVER_ALLOWED_IPS"); v != "" {
		cfg.Server.AllowedIPs = parseCSV(v)
	}
	return nil
}

// TODO: Document applyDatabaseEnv.
func applyDatabaseEnv(cfg *Config) error {
	if v := os.Getenv("AYB_DATABASE_URL"); v != "" {
		cfg.Database.URL = v
	}
	if v := os.Getenv("AYB_DATABASE_REPLICA_URLS"); v != "" {
		urls := parseCSV(v)
		replicas := make([]ReplicaConfig, 0, len(urls))
		for _, replicaURL := range urls {
			replicas = append(replicas, ReplicaConfig{
				URL:         replicaURL,
				Weight:      DefaultReplicaWeight,
				MaxLagBytes: DefaultReplicaMaxLagBytes,
			})
		}
		cfg.Database.Replicas = replicas
	}
	if err := envInt("AYB_DATABASE_EMBEDDED_PORT", &cfg.Database.EmbeddedPort); err != nil {
		return err
	}
	if v := os.Getenv("AYB_DATABASE_EMBEDDED_DATA_DIR"); v != "" {
		cfg.Database.EmbeddedDataDir = v
	}
	if v := os.Getenv("AYB_DATABASE_MIGRATIONS_DIR"); v != "" {
		cfg.Database.MigrationsDir = v
	}
	return nil
}

func applyAdminEnv(cfg *Config) error {
	if v := os.Getenv("AYB_ADMIN_PASSWORD"); v != "" {
		cfg.Admin.Password = v
	}
	if err := envInt("AYB_ADMIN_LOGIN_RATE_LIMIT", &cfg.Admin.LoginRateLimit); err != nil {
		return err
	}
	if v := os.Getenv("AYB_ADMIN_ALLOWED_IPS"); v != "" {
		cfg.Admin.AllowedIPs = parseCSV(v)
	}
	return nil
}

func applyLoggingEnv(cfg *Config) {
	if v := os.Getenv("AYB_LOG_LEVEL"); v != "" {
		cfg.Logging.Level = v
	}
}

func applyMetricsEnv(cfg *Config) {
	if v := os.Getenv("AYB_METRICS_ENABLED"); v != "" {
		cfg.Metrics.Enabled = v == "true" || v == "1"
	}
	if v := os.Getenv("AYB_METRICS_PATH"); v != "" {
		cfg.Metrics.Path = v
	}
	if v := os.Getenv("AYB_METRICS_AUTH_TOKEN"); v != "" {
		cfg.Metrics.AuthToken = v
	}
}

// TODO: Document applyRealtimeEnv.
func applyRealtimeEnv(cfg *Config) error {
	for _, env := range []struct {
		name  string
		value *int
	}{
		{name: "AYB_REALTIME_MAX_CONNECTIONS_PER_USER", value: &cfg.Realtime.MaxConnectionsPerUser},
		{name: "AYB_REALTIME_HEARTBEAT_INTERVAL_SECONDS", value: &cfg.Realtime.HeartbeatIntervalSeconds},
		{name: "AYB_REALTIME_BROADCAST_RATE_LIMIT_PER_SECOND", value: &cfg.Realtime.BroadcastRateLimitPerSecond},
		{name: "AYB_REALTIME_BROADCAST_MAX_MESSAGE_BYTES", value: &cfg.Realtime.BroadcastMaxMessageBytes},
		{name: "AYB_REALTIME_PRESENCE_LEAVE_TIMEOUT_SECONDS", value: &cfg.Realtime.PresenceLeaveTimeoutSeconds},
	} {
		if err := envInt(env.name, env.value); err != nil {
			return err
		}
	}
	return nil
}

func applyTelemetryEnv(cfg *Config) error {
	if v := os.Getenv("AYB_TELEMETRY_ENABLED"); v != "" {
		cfg.Telemetry.Enabled = v == "true" || v == "1"
	}
	if v := os.Getenv("AYB_TELEMETRY_OTLP_ENDPOINT"); v != "" {
		cfg.Telemetry.OTLPEndpoint = v
	}
	if v := os.Getenv("AYB_TELEMETRY_SERVICE_NAME"); v != "" {
		cfg.Telemetry.ServiceName = v
	}
	if err := envFloat64("AYB_TELEMETRY_SAMPLE_RATE", &cfg.Telemetry.SampleRate); err != nil {
		return err
	}
	return nil
}

func applyCORSOriginsEnv(cfg *Config) {
	if v := os.Getenv("AYB_CORS_ORIGINS"); v != "" {
		cfg.Server.CORSAllowedOrigins = strings.Split(v, ",")
	}
}

func applyAuditEnv(cfg *Config) error {
	if v := os.Getenv("AYB_AUDIT_ENABLED"); v != "" {
		cfg.Audit.Enabled = v == "true" || v == "1"
	}
	if v := os.Getenv("AYB_AUDIT_TABLES"); v != "" {
		cfg.Audit.Tables = parseCSV(v)
	}
	if v := os.Getenv("AYB_AUDIT_ALL_TABLES"); v != "" {
		cfg.Audit.AllTables = v == "true" || v == "1"
	}
	if err := envInt("AYB_AUDIT_RETENTION_DAYS", &cfg.Audit.RetentionDays); err != nil {
		return err
	}
	return nil
}

func applyDashboardAIEnv(cfg *Config) {
	if v := os.Getenv("AYB_DASHBOARD_AI_ENABLED"); v != "" {
		cfg.DashboardAI.Enabled = v == "true" || v == "1"
	}
	if v := os.Getenv("AYB_DASHBOARD_AI_RATE_LIMIT"); v != "" {
		cfg.DashboardAI.RateLimit = v
	}
}

func applyVaultEnv(cfg *Config) {
	if v := os.Getenv("AYB_VAULT_MASTER_KEY"); v != "" {
		cfg.Vault.MasterKey = v
	}
}

func applyRateLimitEnv(cfg *Config) {
	if v := os.Getenv("AYB_RATE_LIMIT_API"); v != "" {
		cfg.RateLimit.API = v
	}
	if v := os.Getenv("AYB_RATE_LIMIT_API_ANONYMOUS"); v != "" {
		cfg.RateLimit.APIAnonymous = v
	}
}

// TODO: Document applyEmailEnv.
func applyEmailEnv(cfg *Config) error {
	if v := os.Getenv("AYB_EMAIL_BACKEND"); v != "" {
		cfg.Email.Backend = v
	}
	if v := os.Getenv("AYB_EMAIL_FROM"); v != "" {
		cfg.Email.From = v
	}
	if v := os.Getenv("AYB_EMAIL_FROM_NAME"); v != "" {
		cfg.Email.FromName = v
	}
	if v := os.Getenv("AYB_EMAIL_SMTP_HOST"); v != "" {
		cfg.Email.SMTP.Host = v
	}
	if err := envInt("AYB_EMAIL_SMTP_PORT", &cfg.Email.SMTP.Port); err != nil {
		return err
	}
	if v := os.Getenv("AYB_EMAIL_SMTP_USERNAME"); v != "" {
		cfg.Email.SMTP.Username = v
	}
	if v := os.Getenv("AYB_EMAIL_SMTP_PASSWORD"); v != "" {
		cfg.Email.SMTP.Password = v
	}
	if v := os.Getenv("AYB_EMAIL_SMTP_AUTH_METHOD"); v != "" {
		cfg.Email.SMTP.AuthMethod = v
	}
	if v := os.Getenv("AYB_EMAIL_SMTP_TLS"); v != "" {
		cfg.Email.SMTP.TLS = v == "true" || v == "1"
	}
	if v := os.Getenv("AYB_EMAIL_WEBHOOK_URL"); v != "" {
		cfg.Email.Webhook.URL = v
	}
	if v := os.Getenv("AYB_EMAIL_WEBHOOK_SECRET"); v != "" {
		cfg.Email.Webhook.Secret = v
	}
	if err := envInt("AYB_EMAIL_WEBHOOK_TIMEOUT", &cfg.Email.Webhook.Timeout); err != nil {
		return err
	}
	return nil
}

// TODO: Document applyStorageEnv.
func applyStorageEnv(cfg *Config) error {
	if v := os.Getenv("AYB_STORAGE_ENABLED"); v != "" {
		cfg.Storage.Enabled = v == "true" || v == "1"
	}
	if v := os.Getenv("AYB_STORAGE_BACKEND"); v != "" {
		cfg.Storage.Backend = v
	}
	if v := os.Getenv("AYB_STORAGE_LOCAL_PATH"); v != "" {
		cfg.Storage.LocalPath = v
	}
	if v := os.Getenv("AYB_STORAGE_CDN_URL"); v != "" {
		cfg.Storage.CDNURL = v
	}
	if v := os.Getenv("AYB_STORAGE_CDN_PROVIDER"); v != "" {
		cfg.Storage.CDN.Provider = v
	}
	if v := os.Getenv("AYB_STORAGE_CDN_CLOUDFLARE_ZONE_ID"); v != "" {
		cfg.Storage.CDN.Cloudflare.ZoneID = v
	}
	if v := os.Getenv("AYB_STORAGE_CDN_CLOUDFLARE_API_TOKEN"); v != "" {
		cfg.Storage.CDN.Cloudflare.APIToken = v
	}
	if v := os.Getenv("AYB_STORAGE_CDN_CLOUDFRONT_DISTRIBUTION_ID"); v != "" {
		cfg.Storage.CDN.CloudFront.DistributionID = v
	}
	if v := os.Getenv("AYB_STORAGE_CDN_WEBHOOK_ENDPOINT"); v != "" {
		cfg.Storage.CDN.Webhook.Endpoint = v
	}
	if v := os.Getenv("AYB_STORAGE_CDN_WEBHOOK_SIGNING_SECRET"); v != "" {
		cfg.Storage.CDN.Webhook.SigningSecret = v
	}
	if v := os.Getenv("AYB_STORAGE_MAX_FILE_SIZE"); v != "" {
		cfg.Storage.MaxFileSize = v
	}
	if err := envInt("AYB_STORAGE_DEFAULT_QUOTA_MB", &cfg.Storage.DefaultQuotaMB); err != nil {
		return err
	}
	if v := os.Getenv("AYB_STORAGE_S3_ENDPOINT"); v != "" {
		cfg.Storage.S3Endpoint = v
	}
	if v := os.Getenv("AYB_STORAGE_S3_BUCKET"); v != "" {
		cfg.Storage.S3Bucket = v
	}
	if v := os.Getenv("AYB_STORAGE_S3_REGION"); v != "" {
		cfg.Storage.S3Region = v
	}
	if v := os.Getenv("AYB_STORAGE_S3_ACCESS_KEY"); v != "" {
		cfg.Storage.S3AccessKey = v
	}
	if v := os.Getenv("AYB_STORAGE_S3_SECRET_KEY"); v != "" {
		cfg.Storage.S3SecretKey = v
	}
	if v := os.Getenv("AYB_STORAGE_S3_USE_SSL"); v != "" {
		cfg.Storage.S3UseSSL = v == "true" || v == "1"
	}
	return nil
}

// TODO: Document applyEdgeFunctionsEnv.
func applyEdgeFunctionsEnv(cfg *Config) error {
	if err := envInt("AYB_EDGE_FUNCTIONS_POOL_SIZE", &cfg.EdgeFunctions.PoolSize); err != nil {
		return err
	}
	if err := envInt("AYB_EDGE_FUNCTIONS_DEFAULT_TIMEOUT_MS", &cfg.EdgeFunctions.DefaultTimeoutMs); err != nil {
		return err
	}
	if err := envInt64("AYB_EDGE_FUNCTIONS_MAX_REQUEST_BODY_BYTES", &cfg.EdgeFunctions.MaxRequestBodyBytes); err != nil {
		return err
	}
	if err := envInt("AYB_EDGE_FUNCTIONS_MEMORY_LIMIT_MB", &cfg.EdgeFunctions.MemoryLimitMB); err != nil {
		return err
	}
	if err := envInt("AYB_EDGE_FUNCTIONS_MAX_CONCURRENT_INVOCATIONS", &cfg.EdgeFunctions.MaxConcurrentInvocations); err != nil {
		return err
	}
	if err := envInt("AYB_EDGE_FUNCTIONS_CODE_CACHE_SIZE", &cfg.EdgeFunctions.CodeCacheSize); err != nil {
		return err
	}
	if v := os.Getenv("AYB_EDGE_FUNCTIONS_FETCH_DOMAIN_ALLOWLIST"); v != "" {
		cfg.EdgeFunctions.FetchDomainAllowlist = parseCSV(v)
	}
	return nil
}

// TODO: Document applyBillingEnv.
func applyBillingEnv(cfg *Config) error {
	if v := os.Getenv("AYB_BILLING_PROVIDER"); v != "" {
		cfg.Billing.Provider = v
	}
	if v := os.Getenv("AYB_BILLING_STRIPE_SECRET_KEY"); v != "" {
		cfg.Billing.StripeSecretKey = v
	}
	if v := os.Getenv("AYB_BILLING_STRIPE_WEBHOOK_SECRET"); v != "" {
		cfg.Billing.StripeWebhookSecret = v
	}
	if v := os.Getenv("AYB_BILLING_STRIPE_STARTER_PRICE_ID"); v != "" {
		cfg.Billing.StripeStarterPriceID = v
	}
	if v := os.Getenv("AYB_BILLING_STRIPE_PRO_PRICE_ID"); v != "" {
		cfg.Billing.StripeProPriceID = v
	}
	if v := os.Getenv("AYB_BILLING_STRIPE_ENTERPRISE_PRICE_ID"); v != "" {
		cfg.Billing.StripeEnterprisePriceID = v
	}
	if err := envInt("AYB_BILLING_USAGE_SYNC_INTERVAL_SECONDS", &cfg.Billing.UsageSyncIntervalSecs); err != nil {
		return err
	}
	if v := os.Getenv("AYB_BILLING_STRIPE_METER_API_REQUESTS"); v != "" {
		cfg.Billing.StripeMeterAPIRequests = v
	}
	if v := os.Getenv("AYB_BILLING_STRIPE_METER_STORAGE_BYTES"); v != "" {
		cfg.Billing.StripeMeterStorageBytes = v
	}
	if v := os.Getenv("AYB_BILLING_STRIPE_METER_BANDWIDTH_BYTES"); v != "" {
		cfg.Billing.StripeMeterBandwidthBytes = v
	}
	if v := os.Getenv("AYB_BILLING_STRIPE_METER_FUNCTION_INVOCATIONS"); v != "" {
		cfg.Billing.StripeMeterFunctionInvs = v
	}
	return nil
}

func applySupportEnv(cfg *Config) {
	if v := os.Getenv("AYB_SUPPORT_WEBHOOK_SECRET"); v != "" {
		cfg.Support.WebhookSecret = v
	}
}

// TODO: Document applyJobsEnv.
func applyJobsEnv(cfg *Config) error {
	if v := os.Getenv("AYB_JOBS_ENABLED"); v != "" {
		cfg.Jobs.Enabled = v == "true" || v == "1"
	}
	if err := envInt("AYB_JOBS_WORKER_CONCURRENCY", &cfg.Jobs.WorkerConcurrency); err != nil {
		return err
	}
	if err := envInt("AYB_JOBS_POLL_INTERVAL_MS", &cfg.Jobs.PollIntervalMs); err != nil {
		return err
	}
	if err := envInt("AYB_JOBS_LEASE_DURATION_S", &cfg.Jobs.LeaseDurationS); err != nil {
		return err
	}
	if err := envInt("AYB_JOBS_MAX_RETRIES_DEFAULT", &cfg.Jobs.MaxRetriesDefault); err != nil {
		return err
	}
	if v := os.Getenv("AYB_JOBS_SCHEDULER_ENABLED"); v != "" {
		cfg.Jobs.SchedulerEnabled = v == "true" || v == "1"
	}
	if err := envInt("AYB_JOBS_SCHEDULER_TICK_S", &cfg.Jobs.SchedulerTickS); err != nil {
		return err
	}
	return nil
}

func applyStatusEnv(cfg *Config) error {
	if v := os.Getenv("AYB_STATUS_ENABLED"); v != "" {
		cfg.Status.Enabled = v == "true" || v == "1"
	}
	if err := envInt("AYB_STATUS_CHECK_INTERVAL_SECONDS", &cfg.Status.CheckIntervalSeconds); err != nil {
		return err
	}
	if err := envInt("AYB_STATUS_HISTORY_SIZE", &cfg.Status.HistorySize); err != nil {
		return err
	}
	if v := os.Getenv("AYB_STATUS_PUBLIC_ENDPOINT_ENABLED"); v != "" {
		cfg.Status.PublicEndpointEnabled = v == "true" || v == "1"
	}
	return nil
}

// TODO: Document applyPushEnv.
func applyPushEnv(cfg *Config) {
	if v := os.Getenv("AYB_PUSH_ENABLED"); v != "" {
		cfg.Push.Enabled = v == "true" || v == "1"
	}
	if v := os.Getenv("AYB_PUSH_FCM_CREDENTIALS_FILE"); v != "" {
		cfg.Push.FCM.CredentialsFile = v
	}
	if v := os.Getenv("AYB_PUSH_APNS_KEY_FILE"); v != "" {
		cfg.Push.APNS.KeyFile = v
	}
	if v := os.Getenv("AYB_PUSH_APNS_TEAM_ID"); v != "" {
		cfg.Push.APNS.TeamID = v
	}
	if v := os.Getenv("AYB_PUSH_APNS_KEY_ID"); v != "" {
		cfg.Push.APNS.KeyID = v
	}
	if v := os.Getenv("AYB_PUSH_APNS_BUNDLE_ID"); v != "" {
		cfg.Push.APNS.BundleID = v
	}
	if v := os.Getenv("AYB_PUSH_APNS_ENVIRONMENT"); v != "" {
		cfg.Push.APNS.Environment = v
	}
}
