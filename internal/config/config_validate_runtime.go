// Package config Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/internal/config/config_validate_runtime.go.
package config

import (
	"encoding/json"
	"fmt"
	"math"
	"net"
	"os"
	"strings"
)

// validateServerConfig validates server configuration settings including TLS domain, port range, and IP allowlists for both server and admin access.
func validateServerConfig(c *Config) error {
	if c.Server.TLSDomain != "" {
		c.Server.TLSEnabled = true
	}
	if c.Server.TLSEnabled && c.Server.TLSDomain == "" {
		return fmt.Errorf("server.tls_domain is required when TLS is enabled")
	}
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return fmt.Errorf("server.port must be between 1 and 65535, got %d", c.Server.Port)
	}
	if err := validateAllowlist(c.Server.AllowedIPs, "server.allowed_ips"); err != nil {
		return err
	}
	if err := validateAllowlist(c.Admin.AllowedIPs, "admin.allowed_ips"); err != nil {
		return err
	}
	return nil
}

// validateDatabaseConfig validates database connection pool settings, replica configurations, and port ranges for embedded and managed PostgreSQL.
func validateDatabaseConfig(c *Config) error {
	if c.Database.MaxConns < 1 {
		return fmt.Errorf("database.max_conns must be at least 1, got %d", c.Database.MaxConns)
	}
	if c.Database.MinConns < 0 {
		return fmt.Errorf("database.min_conns must be non-negative, got %d", c.Database.MinConns)
	}
	if c.Database.MinConns > c.Database.MaxConns {
		return fmt.Errorf("database.min_conns (%d) cannot exceed database.max_conns (%d)", c.Database.MinConns, c.Database.MaxConns)
	}
	for i, replica := range c.Database.Replicas {
		if strings.TrimSpace(replica.URL) == "" {
			return fmt.Errorf("database.replicas[%d].url must not be empty", i)
		}
		if replica.Weight < 1 {
			return fmt.Errorf("database.replicas[%d].weight must be at least 1", i)
		}
		if replica.MaxLagBytes < 0 {
			return fmt.Errorf("database.replicas[%d].max_lag_bytes must be non-negative", i)
		}
	}
	if c.Database.URL == "" && (c.Database.EmbeddedPort < 1 || c.Database.EmbeddedPort > 65535) {
		return fmt.Errorf("database.embedded_port must be between 1 and 65535, got %d", c.Database.EmbeddedPort)
	}
	if c.Database.URL == "" && (c.ManagedPG.Port < 1 || c.ManagedPG.Port > 65535) {
		return fmt.Errorf("managed_pg.port must be between 1 and 65535, got %d", c.ManagedPG.Port)
	}
	return nil
}

func validateBillingConfig(c *Config) error {
	if c.Billing.Provider != "" && c.Billing.Provider != "stripe" {
		return fmt.Errorf("billing.provider must be empty or \"stripe\", got %q", c.Billing.Provider)
	}
	if c.Billing.UsageSyncIntervalSecs <= 0 {
		return fmt.Errorf("billing.usage_sync_interval_seconds must be positive, got %d", c.Billing.UsageSyncIntervalSecs)
	}
	return nil
}

func validateGraphQLConfig(c *Config) error {
	switch c.GraphQL.Introspection {
	case "", "open", "disabled":
	default:
		return fmt.Errorf("graphql.introspection must be one of \"\", \"open\", \"disabled\", got %q", c.GraphQL.Introspection)
	}
	return nil
}

// validateBillingStripeConfig validates Stripe-specific billing configuration when the billing provider is set to Stripe, including API keys, webhook secrets, and price IDs.
func validateBillingStripeConfig(c *Config) error {
	if c.Billing.Provider != "stripe" {
		return nil
	}
	if c.Billing.StripeSecretKey == "" {
		return fmt.Errorf("billing.stripe_secret_key is required when billing.provider = stripe")
	}
	if c.Billing.StripeWebhookSecret == "" {
		return fmt.Errorf("billing.stripe_webhook_secret is required when billing.provider = stripe")
	}
	if c.Billing.StripeStarterPriceID == "" {
		return fmt.Errorf("billing.stripe_starter_price_id is required when billing.provider = stripe")
	}
	if c.Billing.StripeProPriceID == "" {
		return fmt.Errorf("billing.stripe_pro_price_id is required when billing.provider = stripe")
	}
	if c.Billing.StripeEnterprisePriceID == "" {
		return fmt.Errorf("billing.stripe_enterprise_price_id is required when billing.provider = stripe")
	}
	if c.Billing.StripeMeterAPIRequests == "" {
		return fmt.Errorf("billing.stripe_meter_api_requests is required when billing.provider = stripe")
	}
	if c.Billing.StripeMeterStorageBytes == "" {
		return fmt.Errorf("billing.stripe_meter_storage_bytes is required when billing.provider = stripe")
	}
	if c.Billing.StripeMeterBandwidthBytes == "" {
		return fmt.Errorf("billing.stripe_meter_bandwidth_bytes is required when billing.provider = stripe")
	}
	if c.Billing.StripeMeterFunctionInvs == "" {
		return fmt.Errorf("billing.stripe_meter_function_invocations is required when billing.provider = stripe")
	}
	return nil
}

// validateEmailConfig validates email backend configuration, with different requirements for log, SMTP, and webhook backends.
func validateEmailConfig(c *Config) error {
	switch c.Email.Backend {
	case "", "log":
	case "smtp":
		if c.Email.SMTP.Host == "" {
			return fmt.Errorf("email.smtp.host is required when email backend is \"smtp\"")
		}
		if c.Email.From == "" {
			return fmt.Errorf("email.from is required when email backend is \"smtp\"")
		}
	case "webhook":
		if c.Email.Webhook.URL == "" {
			return fmt.Errorf("email.webhook.url is required when email backend is \"webhook\"")
		}
	default:
		return fmt.Errorf("email.backend must be \"log\", \"smtp\", or \"webhook\", got %q", c.Email.Backend)
	}
	return nil
}

// validateStorageConfig validates storage backend configuration, with different requirements for local filesystem and S3 backends.
func validateStorageConfig(c *Config) error {
	if !c.Storage.Enabled {
		return nil
	}
	switch c.Storage.Backend {
	case "local":
		if c.Storage.LocalPath == "" {
			return fmt.Errorf("storage.local_path is required when storage backend is \"local\"")
		}
	case "s3":
		if c.Storage.S3Endpoint == "" {
			return fmt.Errorf("storage.s3_endpoint is required when storage backend is \"s3\"")
		}
		if c.Storage.S3Bucket == "" {
			return fmt.Errorf("storage.s3_bucket is required when storage backend is \"s3\"")
		}
		if c.Storage.S3AccessKey == "" {
			return fmt.Errorf("storage.s3_access_key is required when storage backend is \"s3\"")
		}
		if c.Storage.S3SecretKey == "" {
			return fmt.Errorf("storage.s3_secret_key is required when storage backend is \"s3\"")
		}
	default:
		return fmt.Errorf("storage.backend must be \"local\" or \"s3\", got %q", c.Storage.Backend)
	}
	if err := validateStorageCDNConfig(c.Storage); err != nil {
		return err
	}
	return nil
}

// TODO: Document validateStorageCDNConfig.
func validateStorageCDNConfig(storage StorageConfig) error {
	provider := storage.CDN.NormalizedProvider()
	switch provider {
	case "":
		return nil
	case "cloudflare":
		if strings.TrimSpace(storage.CDNURL) == "" {
			return fmt.Errorf("storage.cdn_url is required when storage.cdn.provider is configured")
		}
		if strings.TrimSpace(storage.CDN.Cloudflare.ZoneID) == "" {
			return fmt.Errorf("storage.cdn.cloudflare.zone_id is required when storage.cdn.provider is \"cloudflare\"")
		}
		if strings.TrimSpace(storage.CDN.Cloudflare.APIToken) == "" {
			return fmt.Errorf("storage.cdn.cloudflare.api_token is required when storage.cdn.provider is \"cloudflare\"")
		}
		return nil
	case "cloudfront":
		if strings.TrimSpace(storage.CDNURL) == "" {
			return fmt.Errorf("storage.cdn_url is required when storage.cdn.provider is configured")
		}
		if strings.TrimSpace(storage.CDN.CloudFront.DistributionID) == "" {
			return fmt.Errorf("storage.cdn.cloudfront.distribution_id is required when storage.cdn.provider is \"cloudfront\"")
		}
		return nil
	case "webhook":
		if strings.TrimSpace(storage.CDNURL) == "" {
			return fmt.Errorf("storage.cdn_url is required when storage.cdn.provider is configured")
		}
		if strings.TrimSpace(storage.CDN.Webhook.Endpoint) == "" {
			return fmt.Errorf("storage.cdn.webhook.endpoint is required when storage.cdn.provider is \"webhook\"")
		}
		if strings.TrimSpace(storage.CDN.Webhook.SigningSecret) == "" {
			return fmt.Errorf("storage.cdn.webhook.signing_secret is required when storage.cdn.provider is \"webhook\"")
		}
		return nil
	default:
		return fmt.Errorf("storage.cdn.provider must be one of: cloudflare, cloudfront, webhook, or empty; got %q", storage.CDN.Provider)
	}
}

// validateEdgeFuncConfig validates edge function settings including pool size, timeouts, memory limits, and concurrency bounds.
func validateEdgeFuncConfig(c *Config) error {
	if c.EdgeFunctions.PoolSize < 1 {
		return fmt.Errorf("edge_functions.pool_size must be at least 1, got %d", c.EdgeFunctions.PoolSize)
	}
	if c.EdgeFunctions.DefaultTimeoutMs < 1 {
		return fmt.Errorf("edge_functions.default_timeout_ms must be at least 1, got %d", c.EdgeFunctions.DefaultTimeoutMs)
	}
	if c.EdgeFunctions.MaxRequestBodyBytes < 1 {
		return fmt.Errorf("edge_functions.max_request_body_bytes must be at least 1, got %d", c.EdgeFunctions.MaxRequestBodyBytes)
	}
	if c.EdgeFunctions.MemoryLimitMB < 1 {
		return fmt.Errorf("edge_functions.memory_limit_mb must be at least 1, got %d", c.EdgeFunctions.MemoryLimitMB)
	}
	if c.EdgeFunctions.MaxConcurrentInvocations < 1 {
		return fmt.Errorf("edge_functions.max_concurrent_invocations must be at least 1, got %d", c.EdgeFunctions.MaxConcurrentInvocations)
	}
	if c.EdgeFunctions.CodeCacheSize < 1 {
		return fmt.Errorf("edge_functions.code_cache_size must be at least 1, got %d", c.EdgeFunctions.CodeCacheSize)
	}
	if c.EdgeFunctions.MaxConcurrentInvocations < c.EdgeFunctions.PoolSize {
		return fmt.Errorf(
			"edge_functions.max_concurrent_invocations must be at least edge_functions.pool_size (%d), got %d",
			c.EdgeFunctions.PoolSize,
			c.EdgeFunctions.MaxConcurrentInvocations,
		)
	}
	return nil
}

// validateLoggingConfig validates logging configuration including level, batch sizes, flush intervals, and drain configurations for multiple log delivery backends.
func validateLoggingConfig(c *Config) error {
	if c.Logging.Level != "" {
		switch c.Logging.Level {
		case "debug", "info", "warn", "error":
		default:
			return fmt.Errorf("logging.level must be one of: debug, info, warn, error; got %q", c.Logging.Level)
		}
	}
	if c.Logging.RequestLogBatchSize < 1 {
		return fmt.Errorf("logging.request_log_batch_size must be at least 1, got %d", c.Logging.RequestLogBatchSize)
	}
	if c.Logging.RequestLogFlushIntervalSecs < 1 {
		return fmt.Errorf("logging.request_log_flush_interval_seconds must be at least 1, got %d", c.Logging.RequestLogFlushIntervalSecs)
	}
	if c.Logging.RequestLogQueueSize < 1 {
		return fmt.Errorf("logging.request_log_queue_size must be at least 1, got %d", c.Logging.RequestLogQueueSize)
	}
	for i, d := range c.Logging.Drains {
		if d.Type == "" {
			return fmt.Errorf("logging.drains[%d].type is required", i)
		}
		switch d.Type {
		case "http", "datadog", "loki":
		default:
			return fmt.Errorf("logging.drains[%d].type must be http, datadog, or loki; got %q", i, d.Type)
		}
		if d.URL == "" {
			return fmt.Errorf("logging.drains[%d].url is required", i)
		}
		if d.ID == "" {
			d.ID = fmt.Sprintf("drain-%d", i)
			c.Logging.Drains[i].ID = d.ID
		}
		if d.BatchSize == 0 {
			d.BatchSize = 100
			c.Logging.Drains[i].BatchSize = d.BatchSize
		}
		if d.BatchSize < 0 {
			return fmt.Errorf("logging.drains[%d].batch_size must be non-negative, got %d", i, d.BatchSize)
		}
		if d.Enabled == nil {
			enabled := true
			c.Logging.Drains[i].Enabled = &enabled
			d.Enabled = &enabled
		}
		if d.FlushIntervalSecs == 0 {
			d.FlushIntervalSecs = 5
			c.Logging.Drains[i].FlushIntervalSecs = d.FlushIntervalSecs
		}
		if d.FlushIntervalSecs < 0 {
			return fmt.Errorf("logging.drains[%d].flush_interval_seconds must be non-negative, got %d", i, d.FlushIntervalSecs)
		}
	}
	return nil
}

func validateMetricsConfig(c *Config) error {
	if c.Metrics.Path == "" {
		c.Metrics.Path = "/metrics"
	}
	if !strings.HasPrefix(c.Metrics.Path, "/") {
		return fmt.Errorf("metrics.path must start with /, got %q", c.Metrics.Path)
	}
	return nil
}

func validateTelemetryConfig(c *Config) error {
	if !isValidTelemetrySampleRate(c.Telemetry.SampleRate) {
		return fmt.Errorf("telemetry.sample_rate must be between 0.0 and 1.0, got %v", c.Telemetry.SampleRate)
	}
	return nil
}

// validateJobsConfig validates asynchronous job queue configuration including worker concurrency, polling intervals, and lease durations.
func validateJobsConfig(c *Config) error {
	if !c.Jobs.Enabled {
		return nil
	}
	if c.Jobs.WorkerConcurrency < 1 || c.Jobs.WorkerConcurrency > 64 {
		return fmt.Errorf("jobs.worker_concurrency must be between 1 and 64, got %d", c.Jobs.WorkerConcurrency)
	}
	if c.Jobs.PollIntervalMs < 100 || c.Jobs.PollIntervalMs > 60000 {
		return fmt.Errorf("jobs.poll_interval_ms must be between 100 and 60000, got %d", c.Jobs.PollIntervalMs)
	}
	if c.Jobs.LeaseDurationS < 30 || c.Jobs.LeaseDurationS > 3600 {
		return fmt.Errorf("jobs.lease_duration_s must be between 30 and 3600, got %d", c.Jobs.LeaseDurationS)
	}
	if c.Jobs.MaxRetriesDefault < 0 || c.Jobs.MaxRetriesDefault > 100 {
		return fmt.Errorf("jobs.max_retries_default must be between 0 and 100, got %d", c.Jobs.MaxRetriesDefault)
	}
	if c.Jobs.SchedulerTickS < 5 || c.Jobs.SchedulerTickS > 3600 {
		return fmt.Errorf("jobs.scheduler_tick_s must be between 5 and 3600, got %d", c.Jobs.SchedulerTickS)
	}
	return nil
}

func validateStatusConfig(c *Config) error {
	if c.Status.CheckIntervalSeconds <= 0 {
		return fmt.Errorf("status.check_interval_seconds must be positive, got %d", c.Status.CheckIntervalSeconds)
	}
	if c.Status.HistorySize <= 0 {
		return fmt.Errorf("status.history_size must be positive, got %d", c.Status.HistorySize)
	}
	return nil
}

// validatePushConfig validates push notification configuration including provider credentials and environment settings for FCM and APNS.
func validatePushConfig(c *Config) error {
	if !c.Push.Enabled {
		return nil
	}
	if !c.Jobs.Enabled {
		return fmt.Errorf("push.enabled requires jobs.enabled (push delivery uses the job queue)")
	}

	fcmConfigured := c.Push.FCM.CredentialsFile != ""
	apnsConfigured := c.Push.APNS.KeyFile != "" && c.Push.APNS.TeamID != "" && c.Push.APNS.KeyID != "" && c.Push.APNS.BundleID != ""
	if !fcmConfigured && !apnsConfigured {
		return fmt.Errorf("push.enabled requires at least one provider (fcm or apns) to be fully configured")
	}

	if c.Push.FCM.CredentialsFile != "" {
		if _, err := os.Stat(c.Push.FCM.CredentialsFile); err != nil {
			return fmt.Errorf("push.fcm.credentials_file: %w", err)
		}
		data, err := os.ReadFile(c.Push.FCM.CredentialsFile)
		if err != nil {
			return fmt.Errorf("push.fcm.credentials_file: %w", err)
		}
		if !json.Valid(data) {
			return fmt.Errorf("push.fcm.credentials_file must contain valid JSON")
		}
	}

	if c.Push.APNS.KeyFile != "" {
		if _, err := os.Stat(c.Push.APNS.KeyFile); err != nil {
			return fmt.Errorf("push.apns.key_file: %w", err)
		}
		if c.Push.APNS.TeamID == "" {
			return fmt.Errorf("push.apns.team_id is required when key_file is set")
		}
		if c.Push.APNS.KeyID == "" {
			return fmt.Errorf("push.apns.key_id is required when key_file is set")
		}
		if c.Push.APNS.BundleID == "" {
			return fmt.Errorf("push.apns.bundle_id is required when key_file is set")
		}
	}

	switch c.Push.APNS.Environment {
	case "", "production", "sandbox":
	default:
		return fmt.Errorf("push.apns.environment must be \"production\" or \"sandbox\", got %q", c.Push.APNS.Environment)
	}
	return nil
}

// validateBackupConfig validates backup configuration when backups are enabled, including S3 credentials and retention policies.
func validateBackupConfig(c *Config) error {
	if !c.Backup.Enabled {
		return nil
	}
	if strings.TrimSpace(c.Backup.Bucket) == "" {
		return fmt.Errorf("backup.bucket is required when backups are enabled")
	}
	if strings.TrimSpace(c.Backup.Region) == "" {
		return fmt.Errorf("backup.region is required when backups are enabled")
	}
	if strings.TrimSpace(c.Backup.AccessKey) == "" {
		return fmt.Errorf("backup.access_key is required when backups are enabled")
	}
	if strings.TrimSpace(c.Backup.SecretKey) == "" {
		return fmt.Errorf("backup.secret_key is required when backups are enabled")
	}
	if c.Backup.RetentionCount < 0 {
		return fmt.Errorf("backup.retention_count must be non-negative, got %d", c.Backup.RetentionCount)
	}
	if c.Backup.RetentionDays < 0 {
		return fmt.Errorf("backup.retention_days must be non-negative, got %d", c.Backup.RetentionDays)
	}
	if c.Backup.RetentionCount == 0 && c.Backup.RetentionDays == 0 {
		return fmt.Errorf("at least one of backup.retention_count or backup.retention_days must be set")
	}
	enc := c.Backup.Encryption
	if enc != "" && enc != "AES256" && enc != "aws:kms" {
		return fmt.Errorf("backup.encryption must be empty, \"AES256\", or \"aws:kms\", got %q", enc)
	}
	return nil
}

// validateAIConfig validates AI service configuration including circuit breaker settings and embedding model dimensions.
func validateAIConfig(c *Config) error {
	if _, _, err := ParseRateLimitSpec(c.DashboardAI.RateLimit); err != nil {
		return fmt.Errorf("dashboard_ai.rate_limit: %w", err)
	}
	if c.AI.Breaker.FailureThreshold < 1 {
		return fmt.Errorf("ai.breaker.failure_threshold must be at least 1, got %d", c.AI.Breaker.FailureThreshold)
	}
	if c.AI.Breaker.OpenSeconds < 1 {
		return fmt.Errorf("ai.breaker.open_seconds must be at least 1, got %d", c.AI.Breaker.OpenSeconds)
	}
	if c.AI.Breaker.HalfOpenProbeLimit < 1 {
		return fmt.Errorf("ai.breaker.half_open_probe_limit must be at least 1, got %d", c.AI.Breaker.HalfOpenProbeLimit)
	}

	seenEmbeddingDims := make(map[string]struct{}, len(c.AI.EmbeddingDimensions))
	for key, dim := range c.AI.EmbeddingDimensions {
		if dim <= 0 {
			return fmt.Errorf("ai.embedding_dimensions[%q] must be > 0, got %d", key, dim)
		}
		normKey := strings.ToLower(strings.TrimSpace(key))
		if normKey == "" || !strings.Contains(normKey, ":") {
			return fmt.Errorf("ai.embedding_dimensions[%q] must be in provider:model format", key)
		}
		if _, exists := seenEmbeddingDims[normKey]; exists {
			return fmt.Errorf("ai.embedding_dimensions contains duplicate provider:model entry (case-insensitive): %q", key)
		}
		seenEmbeddingDims[normKey] = struct{}{}
	}
	return nil
}

// validateRealtimeConfig validates realtime WebSocket configuration including connection limits, heartbeat intervals, and message constraints.
func validateRealtimeConfig(c *Config) error {
	if c.Realtime.MaxConnectionsPerUser < 1 {
		return fmt.Errorf("realtime.max_connections_per_user must be at least 1, got %d", c.Realtime.MaxConnectionsPerUser)
	}
	if c.Realtime.HeartbeatIntervalSeconds < 1 {
		return fmt.Errorf("realtime.heartbeat_interval_seconds must be at least 1, got %d", c.Realtime.HeartbeatIntervalSeconds)
	}
	if c.Realtime.BroadcastRateLimitPerSecond < 1 {
		return fmt.Errorf("realtime.broadcast_rate_limit_per_second must be at least 1, got %d", c.Realtime.BroadcastRateLimitPerSecond)
	}
	if c.Realtime.BroadcastMaxMessageBytes < 1 {
		return fmt.Errorf("realtime.broadcast_max_message_bytes must be at least 1, got %d", c.Realtime.BroadcastMaxMessageBytes)
	}
	if c.Realtime.PresenceLeaveTimeoutSeconds < 1 {
		return fmt.Errorf("realtime.presence_leave_timeout_seconds must be at least 1, got %d", c.Realtime.PresenceLeaveTimeoutSeconds)
	}
	return nil
}

// validateAllowlist validates a list of allowed IP addresses or CIDR ranges for the given configuration section.
func validateAllowlist(entries []string, section string) error {
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if strings.Contains(entry, "/") {
			if _, _, err := net.ParseCIDR(entry); err != nil {
				return fmt.Errorf("invalid %s entry %q: %w", section, entry, err)
			}
			continue
		}
		if net.ParseIP(entry) == nil {
			return fmt.Errorf("invalid %s entry %q: not an IP or CIDR", section, entry)
		}
	}
	return nil
}

func isValidTelemetrySampleRate(rate float64) bool {
	return !math.IsNaN(rate) && !math.IsInf(rate, 0) && rate >= 0 && rate <= 1.0
}
