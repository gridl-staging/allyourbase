package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

type configKeyMetadata struct {
	getter            func(*Config) any
	coercer           func(string) any
	setValueValidator func(string) error
}

var configKeyRegistry = withConfigKeyMetadata(map[string]configKeyMetadata{
	"server.host":                                {getter: func(cfg *Config) any { return cfg.Server.Host }},
	"server.port":                                {getter: func(cfg *Config) any { return cfg.Server.Port }},
	"server.site_url":                            {getter: func(cfg *Config) any { return cfg.Server.SiteURL }},
	"server.cors_allowed_origins":                {getter: func(cfg *Config) any { return strings.Join(cfg.Server.CORSAllowedOrigins, ",") }},
	"server.allowed_ips":                         {getter: func(cfg *Config) any { return strings.Join(cfg.Server.AllowedIPs, ",") }},
	"server.body_limit":                          {getter: func(cfg *Config) any { return cfg.Server.BodyLimit }},
	"server.shutdown_timeout":                    {getter: func(cfg *Config) any { return cfg.Server.ShutdownTimeout }},
	"server.tls_enabled":                         {getter: func(cfg *Config) any { return cfg.Server.TLSEnabled }},
	"server.tls_domain":                          {getter: func(cfg *Config) any { return cfg.Server.TLSDomain }},
	"server.tls_cert_dir":                        {getter: func(cfg *Config) any { return cfg.Server.TLSCertDir }},
	"server.tls_email":                           {getter: func(cfg *Config) any { return cfg.Server.TLSEmail }},
	"server.tls_staging":                         {getter: func(cfg *Config) any { return cfg.Server.TLSStaging }},
	"database.url":                               {getter: func(cfg *Config) any { return cfg.Database.URL }},
	"database.max_conns":                         {getter: func(cfg *Config) any { return cfg.Database.MaxConns }},
	"database.min_conns":                         {getter: func(cfg *Config) any { return cfg.Database.MinConns }},
	"database.health_check_interval":             {getter: func(cfg *Config) any { return cfg.Database.HealthCheckSecs }},
	"database.embedded_port":                     {getter: func(cfg *Config) any { return cfg.Database.EmbeddedPort }},
	"database.embedded_data_dir":                 {getter: func(cfg *Config) any { return cfg.Database.EmbeddedDataDir }},
	"database.migrations_dir":                    {getter: func(cfg *Config) any { return cfg.Database.MigrationsDir }},
	"admin.enabled":                              {getter: func(cfg *Config) any { return cfg.Admin.Enabled }},
	"admin.path":                                 {getter: func(cfg *Config) any { return cfg.Admin.Path }},
	"admin.password":                             {getter: func(cfg *Config) any { return cfg.Admin.Password }},
	"admin.login_rate_limit":                     {getter: func(cfg *Config) any { return cfg.Admin.LoginRateLimit }},
	"admin.allowed_ips":                          {getter: func(cfg *Config) any { return strings.Join(cfg.Admin.AllowedIPs, ",") }},
	"auth.enabled":                               {getter: func(cfg *Config) any { return cfg.Auth.Enabled }},
	"auth.jwt_secret":                            {getter: func(cfg *Config) any { return cfg.Auth.JWTSecret }},
	"auth.token_duration":                        {getter: func(cfg *Config) any { return cfg.Auth.TokenDuration }},
	"auth.refresh_token_duration":                {getter: func(cfg *Config) any { return cfg.Auth.RefreshTokenDuration }},
	"auth.rate_limit":                            {getter: func(cfg *Config) any { return cfg.Auth.RateLimit }},
	"auth.anonymous_rate_limit":                  {getter: func(cfg *Config) any { return cfg.Auth.AnonymousRateLimit }},
	"auth.rate_limit_auth":                       {getter: func(cfg *Config) any { return cfg.Auth.RateLimitAuth }},
	"auth.min_password_length":                   {getter: func(cfg *Config) any { return cfg.Auth.MinPasswordLength }},
	"auth.oauth_redirect_url":                    {getter: func(cfg *Config) any { return cfg.Auth.OAuthRedirectURL }},
	"auth.oauth_provider.enabled":                {getter: func(cfg *Config) any { return cfg.Auth.OAuthProviderMode.Enabled }},
	"auth.oauth_provider.access_token_duration":  {getter: func(cfg *Config) any { return cfg.Auth.OAuthProviderMode.AccessTokenDuration }},
	"auth.oauth_provider.refresh_token_duration": {getter: func(cfg *Config) any { return cfg.Auth.OAuthProviderMode.RefreshTokenDuration }},
	"auth.oauth_provider.auth_code_duration":     {getter: func(cfg *Config) any { return cfg.Auth.OAuthProviderMode.AuthCodeDuration }},
	"billing.provider":                           {getter: func(cfg *Config) any { return cfg.Billing.Provider }},
	"billing.stripe_secret_key":                  {getter: func(cfg *Config) any { return cfg.Billing.StripeSecretKey }},
	"billing.stripe_webhook_secret":              {getter: func(cfg *Config) any { return cfg.Billing.StripeWebhookSecret }},
	"billing.stripe_starter_price_id":            {getter: func(cfg *Config) any { return cfg.Billing.StripeStarterPriceID }},
	"billing.stripe_pro_price_id":                {getter: func(cfg *Config) any { return cfg.Billing.StripeProPriceID }},
	"billing.stripe_enterprise_price_id":         {getter: func(cfg *Config) any { return cfg.Billing.StripeEnterprisePriceID }},
	"billing.usage_sync_interval_seconds":        {getter: func(cfg *Config) any { return cfg.Billing.UsageSyncIntervalSecs }},
	"billing.stripe_meter_api_requests":          {getter: func(cfg *Config) any { return cfg.Billing.StripeMeterAPIRequests }},
	"billing.stripe_meter_storage_bytes":         {getter: func(cfg *Config) any { return cfg.Billing.StripeMeterStorageBytes }},
	"billing.stripe_meter_bandwidth_bytes":       {getter: func(cfg *Config) any { return cfg.Billing.StripeMeterBandwidthBytes }},
	"billing.stripe_meter_function_invocations":  {getter: func(cfg *Config) any { return cfg.Billing.StripeMeterFunctionInvs }},
	"status.enabled":                             {getter: func(cfg *Config) any { return cfg.Status.Enabled }},
	"status.check_interval_seconds":              {getter: func(cfg *Config) any { return cfg.Status.CheckIntervalSeconds }},
	"status.history_size":                        {getter: func(cfg *Config) any { return cfg.Status.HistorySize }},
	"status.public_endpoint_enabled":             {getter: func(cfg *Config) any { return cfg.Status.PublicEndpointEnabled }},
	"auth.magic_link_enabled":                    {getter: func(cfg *Config) any { return cfg.Auth.MagicLinkEnabled }},
	"auth.magic_link_duration":                   {getter: func(cfg *Config) any { return cfg.Auth.MagicLinkDuration }},
	"auth.email_mfa_enabled":                     {getter: func(cfg *Config) any { return cfg.Auth.EmailMFAEnabled }},
	"auth.anonymous_auth_enabled":                {getter: func(cfg *Config) any { return cfg.Auth.AnonymousAuthEnabled }},
	"auth.totp_enabled":                          {getter: func(cfg *Config) any { return cfg.Auth.TOTPEnabled }},
	"auth.encryption_key":                        {getter: func(cfg *Config) any { return cfg.Auth.EncryptionKey }},
	"auth.sms_enabled":                           {getter: func(cfg *Config) any { return cfg.Auth.SMSEnabled }},
	"auth.sms_provider":                          {getter: func(cfg *Config) any { return cfg.Auth.SMSProvider }},
	"auth.sms_code_length":                       {getter: func(cfg *Config) any { return cfg.Auth.SMSCodeLength }},
	"auth.sms_code_expiry":                       {getter: func(cfg *Config) any { return cfg.Auth.SMSCodeExpiry }},
	"auth.sms_max_attempts":                      {getter: func(cfg *Config) any { return cfg.Auth.SMSMaxAttempts }},
	"auth.sms_daily_limit":                       {getter: func(cfg *Config) any { return cfg.Auth.SMSDailyLimit }},
	"auth.sms_allowed_countries":                 {getter: func(cfg *Config) any { return strings.Join(cfg.Auth.SMSAllowedCountries, ",") }},
	"auth.twilio_sid":                            {getter: func(cfg *Config) any { return cfg.Auth.TwilioSID }},
	"auth.twilio_token":                          {getter: func(cfg *Config) any { return cfg.Auth.TwilioToken }},
	"auth.twilio_from":                           {getter: func(cfg *Config) any { return cfg.Auth.TwilioFrom }},
	"auth.plivo_auth_id":                         {getter: func(cfg *Config) any { return cfg.Auth.PlivoAuthID }},
	"auth.plivo_auth_token":                      {getter: func(cfg *Config) any { return cfg.Auth.PlivoAuthToken }},
	"auth.plivo_from":                            {getter: func(cfg *Config) any { return cfg.Auth.PlivoFrom }},
	"auth.telnyx_api_key":                        {getter: func(cfg *Config) any { return cfg.Auth.TelnyxAPIKey }},
	"auth.telnyx_from":                           {getter: func(cfg *Config) any { return cfg.Auth.TelnyxFrom }},
	"auth.msg91_auth_key":                        {getter: func(cfg *Config) any { return cfg.Auth.MSG91AuthKey }},
	"auth.msg91_template_id":                     {getter: func(cfg *Config) any { return cfg.Auth.MSG91TemplateID }},
	"auth.aws_region":                            {getter: func(cfg *Config) any { return cfg.Auth.AWSRegion }},
	"auth.vonage_api_key":                        {getter: func(cfg *Config) any { return cfg.Auth.VonageAPIKey }},
	"auth.vonage_api_secret":                     {getter: func(cfg *Config) any { return cfg.Auth.VonageAPISecret }},
	"auth.vonage_from":                           {getter: func(cfg *Config) any { return cfg.Auth.VonageFrom }},
	"auth.sms_webhook_url":                       {getter: func(cfg *Config) any { return cfg.Auth.SMSWebhookURL }},
	"auth.sms_webhook_secret":                    {getter: func(cfg *Config) any { return cfg.Auth.SMSWebhookSecret }},
	"auth.sms_test_phone_numbers":                {getter: func(cfg *Config) any { return cfg.Auth.SMSTestPhoneNumbers }},
	"audit.enabled":                              {getter: func(cfg *Config) any { return cfg.Audit.Enabled }},
	"audit.tables":                               {getter: func(cfg *Config) any { return strings.Join(cfg.Audit.Tables, ",") }},
	"audit.all_tables":                           {getter: func(cfg *Config) any { return cfg.Audit.AllTables }},
	"audit.retention_days":                       {getter: func(cfg *Config) any { return cfg.Audit.RetentionDays }},
	"rate_limit.api":                             {getter: func(cfg *Config) any { return cfg.RateLimit.API }},
	"rate_limit.api_anonymous":                   {getter: func(cfg *Config) any { return cfg.RateLimit.APIAnonymous }},
	"vault.master_key":                           {getter: func(cfg *Config) any { return cfg.Vault.MasterKey }},
	"email.backend":                              {getter: func(cfg *Config) any { return cfg.Email.Backend }},
	"email.from":                                 {getter: func(cfg *Config) any { return cfg.Email.From }},
	"email.from_name":                            {getter: func(cfg *Config) any { return cfg.Email.FromName }},
	"storage.enabled":                            {getter: func(cfg *Config) any { return cfg.Storage.Enabled }},
	"storage.backend":                            {getter: func(cfg *Config) any { return cfg.Storage.Backend }},
	"storage.local_path":                         {getter: func(cfg *Config) any { return cfg.Storage.LocalPath }},
	"storage.cdn_url":                            {getter: func(cfg *Config) any { return cfg.Storage.CDNURL }},
	"storage.cdn.provider":                       {getter: func(cfg *Config) any { return cfg.Storage.CDN.Provider }},
	"storage.cdn.cloudflare.zone_id":             {getter: func(cfg *Config) any { return cfg.Storage.CDN.Cloudflare.ZoneID }},
	"storage.cdn.cloudflare.api_token":           {getter: func(cfg *Config) any { return cfg.Storage.CDN.Cloudflare.APIToken }},
	"storage.cdn.cloudfront.distribution_id":     {getter: func(cfg *Config) any { return cfg.Storage.CDN.CloudFront.DistributionID }},
	"storage.cdn.webhook.endpoint":               {getter: func(cfg *Config) any { return cfg.Storage.CDN.Webhook.Endpoint }},
	"storage.cdn.webhook.signing_secret":         {getter: func(cfg *Config) any { return cfg.Storage.CDN.Webhook.SigningSecret }},
	"storage.max_file_size":                      {getter: func(cfg *Config) any { return cfg.Storage.MaxFileSize }},
	"storage.s3_endpoint":                        {getter: func(cfg *Config) any { return cfg.Storage.S3Endpoint }},
	"storage.s3_bucket":                          {getter: func(cfg *Config) any { return cfg.Storage.S3Bucket }},
	"storage.s3_region":                          {getter: func(cfg *Config) any { return cfg.Storage.S3Region }},
	"storage.s3_access_key":                      {getter: func(cfg *Config) any { return cfg.Storage.S3AccessKey }},
	"storage.s3_secret_key":                      {getter: func(cfg *Config) any { return cfg.Storage.S3SecretKey }},
	"storage.s3_use_ssl":                         {getter: func(cfg *Config) any { return cfg.Storage.S3UseSSL }},
	"edge_functions.pool_size":                   {getter: func(cfg *Config) any { return cfg.EdgeFunctions.PoolSize }},
	"edge_functions.default_timeout_ms":          {getter: func(cfg *Config) any { return cfg.EdgeFunctions.DefaultTimeoutMs }},
	"edge_functions.max_request_body_bytes":      {getter: func(cfg *Config) any { return cfg.EdgeFunctions.MaxRequestBodyBytes }},
	"edge_functions.fetch_domain_allowlist":      {getter: func(cfg *Config) any { return strings.Join(cfg.EdgeFunctions.FetchDomainAllowlist, ",") }},
	"edge_functions.memory_limit_mb":             {getter: func(cfg *Config) any { return cfg.EdgeFunctions.MemoryLimitMB }},
	"edge_functions.max_concurrent_invocations":  {getter: func(cfg *Config) any { return cfg.EdgeFunctions.MaxConcurrentInvocations }},
	"edge_functions.code_cache_size":             {getter: func(cfg *Config) any { return cfg.EdgeFunctions.CodeCacheSize }},
	"logging.level":                              {getter: func(cfg *Config) any { return cfg.Logging.Level }},
	"logging.format":                             {getter: func(cfg *Config) any { return cfg.Logging.Format }},
	"metrics.enabled":                            {getter: func(cfg *Config) any { return cfg.Metrics.Enabled }},
	"metrics.path":                               {getter: func(cfg *Config) any { return cfg.Metrics.Path }},
	"metrics.auth_token":                         {getter: func(cfg *Config) any { return cfg.Metrics.AuthToken }},
	"telemetry.enabled":                          {getter: func(cfg *Config) any { return cfg.Telemetry.Enabled }},
	"telemetry.otlp_endpoint":                    {getter: func(cfg *Config) any { return cfg.Telemetry.OTLPEndpoint }},
	"telemetry.service_name":                     {getter: func(cfg *Config) any { return cfg.Telemetry.ServiceName }},
	"telemetry.sample_rate":                      {getter: func(cfg *Config) any { return cfg.Telemetry.SampleRate }},
	"jobs.enabled":                               {getter: func(cfg *Config) any { return cfg.Jobs.Enabled }},
	"jobs.worker_concurrency":                    {getter: func(cfg *Config) any { return cfg.Jobs.WorkerConcurrency }},
	"jobs.poll_interval_ms":                      {getter: func(cfg *Config) any { return cfg.Jobs.PollIntervalMs }},
	"jobs.lease_duration_s":                      {getter: func(cfg *Config) any { return cfg.Jobs.LeaseDurationS }},
	"jobs.max_retries_default":                   {getter: func(cfg *Config) any { return cfg.Jobs.MaxRetriesDefault }},
	"jobs.scheduler_enabled":                     {getter: func(cfg *Config) any { return cfg.Jobs.SchedulerEnabled }},
	"jobs.scheduler_tick_s":                      {getter: func(cfg *Config) any { return cfg.Jobs.SchedulerTickS }},
	"push.enabled":                               {getter: func(cfg *Config) any { return cfg.Push.Enabled }},
	"push.fcm.credentials_file":                  {getter: func(cfg *Config) any { return cfg.Push.FCM.CredentialsFile }},
	"push.apns.key_file":                         {getter: func(cfg *Config) any { return cfg.Push.APNS.KeyFile }},
	"push.apns.team_id":                          {getter: func(cfg *Config) any { return cfg.Push.APNS.TeamID }},
	"push.apns.key_id":                           {getter: func(cfg *Config) any { return cfg.Push.APNS.KeyID }},
	"push.apns.bundle_id":                        {getter: func(cfg *Config) any { return cfg.Push.APNS.BundleID }},
	"push.apns.environment":                      {getter: func(cfg *Config) any { return cfg.Push.APNS.Environment }},
})

var boolCoercionKeys = []string{
	"admin.enabled",
	"auth.enabled",
	"auth.magic_link_enabled",
	"auth.sms_enabled",
	"auth.email_mfa_enabled",
	"auth.anonymous_auth_enabled",
	"auth.totp_enabled",
	"storage.enabled",
	"storage.s3_use_ssl",
	"server.tls_enabled",
	"server.tls_staging",
	"auth.oauth_provider.enabled",
	"metrics.enabled",
	"telemetry.enabled",
	"jobs.enabled",
	"jobs.scheduler_enabled",
	"audit.enabled",
	"audit.all_tables",
}

var intCoercionKeys = []string{
	"server.port",
	"server.shutdown_timeout",
	"database.max_conns",
	"database.min_conns",
	"database.health_check_interval",
	"database.embedded_port",
	"admin.login_rate_limit",
	"auth.token_duration",
	"auth.refresh_token_duration",
	"auth.rate_limit",
	"auth.anonymous_rate_limit",
	"auth.min_password_length",
	"auth.magic_link_duration",
	"auth.sms_code_length",
	"auth.sms_code_expiry",
	"auth.sms_max_attempts",
	"auth.sms_daily_limit",
	"auth.oauth_provider.access_token_duration",
	"auth.oauth_provider.refresh_token_duration",
	"auth.oauth_provider.auth_code_duration",
	"edge_functions.pool_size",
	"edge_functions.default_timeout_ms",
	"edge_functions.memory_limit_mb",
	"edge_functions.max_concurrent_invocations",
	"edge_functions.code_cache_size",
	"jobs.worker_concurrency",
	"jobs.poll_interval_ms",
	"jobs.lease_duration_s",
	"jobs.max_retries_default",
	"jobs.scheduler_tick_s",
	"audit.retention_days",
	"billing.usage_sync_interval_seconds",
}

var csvCoercionKeys = []string{
	"edge_functions.fetch_domain_allowlist",
	"server.allowed_ips",
	"admin.allowed_ips",
	"audit.tables",
	"auth.sms_allowed_countries",
}

func withConfigKeyMetadata(registry map[string]configKeyMetadata) map[string]configKeyMetadata {
	assignCoercer(registry, boolCoercionKeys, coerceBoolValue)
	assignCoercer(registry, intCoercionKeys, coerceIntValue)
	assignCoercer(registry, []string{"telemetry.sample_rate"}, coerceFloatValue)
	assignCoercer(registry, []string{"edge_functions.max_request_body_bytes"}, coerceInt64Value)
	assignCoercer(registry, csvCoercionKeys, coerceCSVValue)
	assignSetValueValidator(registry, "telemetry.sample_rate", validateTelemetrySampleRateValue)
	return registry
}

func assignCoercer(registry map[string]configKeyMetadata, keys []string, coercer func(string) any) {
	for _, key := range keys {
		metadata, ok := registry[key]
		if !ok {
			panic("missing config key metadata for coercer: " + key)
		}
		metadata.coercer = coercer
		registry[key] = metadata
	}
}

func assignSetValueValidator(registry map[string]configKeyMetadata, key string, validate func(string) error) {
	metadata, ok := registry[key]
	if !ok {
		panic("missing config key metadata for validator: " + key)
	}
	metadata.setValueValidator = validate
	registry[key] = metadata
}

func coerceBoolValue(value string) any {
	return value == "true" || value == "1"
}

func coerceIntValue(value string) any {
	if n, err := strconv.Atoi(value); err == nil {
		return n
	}
	return value
}

func coerceFloatValue(value string) any {
	if n, err := strconv.ParseFloat(value, 64); err == nil {
		return n
	}
	return value
}

func coerceInt64Value(value string) any {
	if n, err := strconv.ParseInt(value, 10, 64); err == nil {
		return n
	}
	return value
}

func coerceCSVValue(value string) any {
	return parseCSV(value)
}

func validateTelemetrySampleRateValue(value string) error {
	rate, err := strconv.ParseFloat(value, 64)
	if err != nil || !isValidTelemetrySampleRate(rate) {
		return fmt.Errorf("telemetry.sample_rate must be between 0.0 and 1.0, got %q", value)
	}
	return nil
}

// IsValidKey returns true if the dotted key is a recognized config key.
func IsValidKey(key string) bool {
	return validKeys[key]
}

// GetValue returns the value for a dotted config key (e.g. "server.port").
func GetValue(cfg *Config, key string) (any, error) {
	metadata, ok := configKeyRegistry[key]
	if !ok {
		return nil, fmt.Errorf("unknown configuration key: %s", key)
	}
	return metadata.getter(cfg), nil
}

// SetValue reads the existing TOML file, updates a single key, and writes it back.
// Creates the file with just the key if it does not exist.
func SetValue(configPath, key, value string) error {
	// Read existing TOML as a generic map.
	var data map[string]any
	if raw, err := os.ReadFile(configPath); err == nil {
		if err := toml.Unmarshal(raw, &data); err != nil {
			return fmt.Errorf("parsing %s: %w", configPath, err)
		}
	}
	if data == nil {
		data = make(map[string]any)
	}

	// Split key into path segments and require at least section.field.
	parts := strings.Split(key, ".")
	if len(parts) < 2 {
		return fmt.Errorf("invalid key format: %s (expected section.field)", key)
	}
	if metadata, ok := configKeyRegistry[key]; ok && metadata.setValueValidator != nil {
		if err := metadata.setValueValidator(value); err != nil {
			return err
		}
	}

	// Traverse/create nested maps for all parent segments.
	currentMap := data
	for _, segment := range parts[:len(parts)-1] {
		nextMap, ok := currentMap[segment].(map[string]any)
		if !ok {
			nextMap = make(map[string]any)
			currentMap[segment] = nextMap
		}
		currentMap = nextMap
	}

	// Convert value to appropriate type.
	currentMap[parts[len(parts)-1]] = coerceValue(key, value)

	// Marshal back to TOML and write.
	out, err := toml.Marshal(data)
	if err != nil {
		return fmt.Errorf("serializing config: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		return fmt.Errorf("creating directory: %w", err)
	}
	return os.WriteFile(configPath, out, 0o600)
}

// coerceValue converts a string value to the appropriate Go type for TOML serialization.
func coerceValue(key, value string) any {
	metadata, ok := configKeyRegistry[key]
	if !ok || metadata.coercer == nil {
		return value
	}
	return metadata.coercer(value)
}

// AddExtension adds name to the managed_pg.extensions list in the TOML file at
// configPath. The file is created if it does not exist. The operation is
// idempotent — if name is already present no change is written.
func AddExtension(configPath, name string) error {
	var data map[string]any
	if raw, err := os.ReadFile(configPath); err == nil {
		if err := toml.Unmarshal(raw, &data); err != nil {
			return fmt.Errorf("parsing %s: %w", configPath, err)
		}
	}
	if data == nil {
		data = make(map[string]any)
	}

	section, _ := data["managed_pg"].(map[string]any)
	if section == nil {
		section = make(map[string]any)
		data["managed_pg"] = section
	}

	existing := toStringSlice(section["extensions"])
	for _, e := range existing {
		if e == name {
			return nil // already present
		}
	}
	section["extensions"] = append(existing, name)

	return writeConfig(configPath, data)
}

// RemoveExtension removes name from the managed_pg.extensions list in the TOML
// file at configPath. It is a no-op if the file does not exist or name is not
// in the list.
func RemoveExtension(configPath, name string) error {
	raw, err := os.ReadFile(configPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("reading %s: %w", configPath, err)
	}

	var data map[string]any
	if err := toml.Unmarshal(raw, &data); err != nil {
		return fmt.Errorf("parsing %s: %w", configPath, err)
	}

	section, _ := data["managed_pg"].(map[string]any)
	if section == nil {
		return nil
	}

	existing := toStringSlice(section["extensions"])
	filtered := existing[:0:0]
	for _, e := range existing {
		if e != name {
			filtered = append(filtered, e)
		}
	}
	if len(filtered) == len(existing) {
		return nil // nothing changed
	}
	section["extensions"] = filtered

	return writeConfig(configPath, data)
}

// toStringSlice coerces any TOML array value to []string.
func toStringSlice(v any) []string {
	switch val := v.(type) {
	case []string:
		return val
	case []any:
		out := make([]string, 0, len(val))
		for _, item := range val {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// writeConfig marshals data to TOML and writes it to configPath, creating
// parent directories as needed.
func writeConfig(configPath string, data map[string]any) error {
	out, err := toml.Marshal(data)
	if err != nil {
		return fmt.Errorf("serializing config: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		return fmt.Errorf("creating directory: %w", err)
	}
	return os.WriteFile(configPath, out, 0o600)
}
