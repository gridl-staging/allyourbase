package config

import (
	"strconv"
	"strings"
)

type ManagedPGConfig struct {
	Port                   int      `toml:"port"`
	DataDir                string   `toml:"data_dir"`
	BinaryURL              string   `toml:"binary_url"`
	PGVersion              string   `toml:"pg_version"`
	Extensions             []string `toml:"extensions"`
	SharedPreloadLibraries []string `toml:"shared_preload_libraries"`
	// PostGIS is syntactic sugar: when true, "postgis" is prepended to
	// Extensions if not already present. Setting PostGIS to false does NOT
	// remove "postgis" from Extensions if the user listed it there manually.
	PostGIS bool `toml:"postgis"`
}

// EffectiveExtensions returns the Extensions list with "postgis" prepended
// when PostGIS is true (deduplicated). This is the list that should be
// passed to pgmanager.Config.Extensions.
func (c *ManagedPGConfig) EffectiveExtensions() []string {
	if !c.PostGIS {
		return c.Extensions
	}
	effective := make([]string, 0, len(c.Extensions)+1)
	effective = append(effective, "postgis")
	for _, ext := range c.Extensions {
		if strings.EqualFold(strings.TrimSpace(ext), "postgis") {
			continue
		}
		effective = append(effective, ext)
	}
	return effective
}

// RateLimitConfig configures global API rate limiting.
type RateLimitConfig struct {
	API          string `toml:"api"`           // e.g. "100/min" — per-user (authenticated) or per-IP (anonymous)
	APIAnonymous string `toml:"api_anonymous"` // e.g. "30/min" — stricter limit for unauthenticated requests
}

// APIConfig configures runtime API behavior.
type APIConfig struct {
	ImportMaxSizeMB  int  `toml:"import_max_size_mb"`
	ImportMaxRows    int  `toml:"import_max_rows"`
	ExportMaxRows    int  `toml:"export_max_rows"`
	AggregateEnabled bool `toml:"aggregate_enabled"`
}

type ServerConfig struct {
	Host               string   `toml:"host"`
	Port               int      `toml:"port"`
	SiteURL            string   `toml:"site_url"` // public base URL for email action links (e.g. "https://myapp.example.com")
	CORSAllowedOrigins []string `toml:"cors_allowed_origins"`
	AllowedIPs         []string `toml:"allowed_ips"`
	BodyLimit          string   `toml:"body_limit"`
	ShutdownTimeout    int      `toml:"shutdown_timeout"`
	// TLS — set tls_domain to enable automatic HTTPS via Let's Encrypt.
	TLSEnabled bool   `toml:"tls_enabled"` // auto-set when TLSDomain is non-empty
	TLSDomain  string `toml:"tls_domain"`
	TLSCertDir string `toml:"tls_cert_dir"` // default: ~/.ayb/certs at runtime
	TLSEmail   string `toml:"tls_email"`    // ACME account email (recommended)
	TLSStaging bool   `toml:"tls_staging"`  // use Let's Encrypt staging CA (for testing)
}

type DatabaseConfig struct {
	URL             string          `toml:"url"`
	MaxConns        int             `toml:"max_conns"`
	MinConns        int             `toml:"min_conns"`
	HealthCheckSecs int             `toml:"health_check_interval"`
	EmbeddedPort    int             `toml:"embedded_port"`
	EmbeddedDataDir string          `toml:"embedded_data_dir"`
	MigrationsDir   string          `toml:"migrations_dir"`
	SeedFile        string          `toml:"seed_file"`
	Replicas        []ReplicaConfig `toml:"replicas"`
}

const (
	DefaultReplicaWeight      = 1
	DefaultReplicaMaxLagBytes = int64(10 * 1024 * 1024)
)

type ReplicaConfig struct {
	URL         string `toml:"url"`
	Weight      int    `toml:"weight"`
	MaxLagBytes int64  `toml:"max_lag_bytes"`
}

type AdminConfig struct {
	Enabled        bool     `toml:"enabled"`
	Path           string   `toml:"path"`
	Password       string   `toml:"password"`
	LoginRateLimit int      `toml:"login_rate_limit"` // admin login attempts per minute per IP (default 20)
	AllowedIPs     []string `toml:"allowed_ips"`
}

// AuthConfig holds authentication configuration including JWT secrets, token durations, OAuth providers, OIDC providers, SAML providers, MFA methods, password policies, rate limits, and various authentication hooks.
type AuthConfig struct {
	Enabled              bool                     `toml:"enabled"`
	JWTSecret            string                   `toml:"jwt_secret"`
	TokenDuration        int                      `toml:"token_duration"`
	RefreshTokenDuration int                      `toml:"refresh_token_duration"`
	RateLimit            int                      `toml:"rate_limit"`
	AnonymousRateLimit   int                      `toml:"anonymous_rate_limit"`
	RateLimitAuth        string                   `toml:"rate_limit_auth"`
	MinPasswordLength    int                      `toml:"min_password_length"`
	OAuth                map[string]OAuthProvider `toml:"oauth"`
	OAuthRedirectURL     string                   `toml:"oauth_redirect_url"`
	MagicLinkEnabled     bool                     `toml:"magic_link_enabled"`
	MagicLinkDuration    int                      `toml:"magic_link_duration"` // seconds, default 600 (10 min)
	EmailMFAEnabled      bool                     `toml:"email_mfa_enabled"`
	AnonymousAuthEnabled bool                     `toml:"anonymous_auth_enabled"`
	TOTPEnabled          bool                     `toml:"totp_enabled"`
	EncryptionKey        string                   `toml:"encryption_key"`
	SMSEnabled           bool                     `toml:"sms_enabled"`
	SMSProvider          string                   `toml:"sms_provider"`
	SMSCodeLength        int                      `toml:"sms_code_length"`
	SMSCodeExpiry        int                      `toml:"sms_code_expiry"` // seconds
	SMSMaxAttempts       int                      `toml:"sms_max_attempts"`
	SMSDailyLimit        int                      `toml:"sms_daily_limit"` // 0 = unlimited
	SMSAllowedCountries  []string                 `toml:"sms_allowed_countries"`
	TwilioSID            string                   `toml:"twilio_sid"`
	TwilioToken          string                   `toml:"twilio_token"`
	TwilioFrom           string                   `toml:"twilio_from"`
	PlivoAuthID          string                   `toml:"plivo_auth_id"`
	PlivoAuthToken       string                   `toml:"plivo_auth_token"`
	PlivoFrom            string                   `toml:"plivo_from"`
	TelnyxAPIKey         string                   `toml:"telnyx_api_key"`
	TelnyxFrom           string                   `toml:"telnyx_from"`
	MSG91AuthKey         string                   `toml:"msg91_auth_key"`
	MSG91TemplateID      string                   `toml:"msg91_template_id"`
	AWSRegion            string                   `toml:"aws_region"`
	VonageAPIKey         string                   `toml:"vonage_api_key"`
	VonageAPISecret      string                   `toml:"vonage_api_secret"`
	VonageFrom           string                   `toml:"vonage_from"`
	SMSWebhookURL        string                   `toml:"sms_webhook_url"`
	SMSWebhookSecret     string                   `toml:"sms_webhook_secret"`
	SMSTestPhoneNumbers  map[string]string        `toml:"sms_test_phone_numbers"`
	OAuthProviderMode    OAuthProviderModeConfig  `toml:"oauth_provider"`
	OIDC                 map[string]OIDCProvider  `toml:"oidc"`
	SAMLProviders        []SAMLProvider           `toml:"saml_providers"`
	Hooks                AuthHooks                `toml:"hooks"`
}

// BillingConfig controls billing provider and lifecycle configuration.
type BillingConfig struct {
	Provider                  string `toml:"provider"`
	StripeSecretKey           string `toml:"stripe_secret_key"`
	StripeWebhookSecret       string `toml:"stripe_webhook_secret"`
	StripeStarterPriceID      string `toml:"stripe_starter_price_id"`
	StripeProPriceID          string `toml:"stripe_pro_price_id"`
	StripeEnterprisePriceID   string `toml:"stripe_enterprise_price_id"`
	UsageSyncIntervalSecs     int    `toml:"usage_sync_interval_seconds"`
	StripeMeterAPIRequests    string `toml:"stripe_meter_api_requests"`
	StripeMeterStorageBytes   string `toml:"stripe_meter_storage_bytes"`
	StripeMeterBandwidthBytes string `toml:"stripe_meter_bandwidth_bytes"`
	StripeMeterFunctionInvs   string `toml:"stripe_meter_function_invocations"`
}

// SupportConfig controls support ticketing enablement and inbound email settings.
type SupportConfig struct {
	Enabled            bool   `toml:"enabled"`
	InboundEmailDomain string `toml:"inbound_email_domain"`
	WebhookSecret      string `toml:"webhook_secret"`
}

// AuthHooks configures edge functions that intercept auth lifecycle events.
type AuthHooks struct {
	BeforeSignUp        string `toml:"before_sign_up" json:"before_sign_up"`
	AfterSignUp         string `toml:"after_sign_up" json:"after_sign_up"`
	CustomAccessToken   string `toml:"custom_access_token" json:"custom_access_token"`
	BeforePasswordReset string `toml:"before_password_reset" json:"before_password_reset"`
	SendEmail           string `toml:"send_email" json:"send_email"`
	SendSMS             string `toml:"send_sms" json:"send_sms"`
}

type VaultConfig struct {
	MasterKey string `toml:"master_key"`
}

// OAuthProviderModeConfig controls AYB's OAuth 2.0 authorization server.
// When Enabled, AYB can issue access/refresh tokens to registered OAuth clients.
type OAuthProviderModeConfig struct {
	Enabled              bool `toml:"enabled"`
	AccessTokenDuration  int  `toml:"access_token_duration"`  // seconds, default 3600 (1h)
	RefreshTokenDuration int  `toml:"refresh_token_duration"` // seconds, default 2592000 (30d)
	AuthCodeDuration     int  `toml:"auth_code_duration"`     // seconds, default 600 (10min)
}

// OAuthProvider configures a single OAuth2 provider (e.g. google, github, microsoft, apple).
type OAuthProvider struct {
	Enabled             bool   `toml:"enabled"`
	ClientID            string `toml:"client_id"`
	ClientSecret        string `toml:"client_secret"`
	TenantID            string `toml:"tenant_id"`
	StoreProviderTokens bool   `toml:"store_provider_tokens"`
	// Apple-specific fields.
	TeamID     string `toml:"team_id"`     // Apple Developer Team ID
	KeyID      string `toml:"key_id"`      // Apple Sign-In private key ID
	PrivateKey string `toml:"private_key"` // PEM-encoded ES256 private key (or file path)
	// Facebook-specific: Graph API version (default "v22.0").
	FacebookAPIVersion string `toml:"facebook_api_version"`
	// GitLab-specific: base URL for self-hosted instances (default "https://gitlab.com").
	GitLabBaseURL string `toml:"gitlab_base_url"`
}

// OIDCProvider configures a custom OpenID Connect identity provider.
type OIDCProvider struct {
	Enabled      bool     `toml:"enabled"`
	IssuerURL    string   `toml:"issuer_url"`
	ClientID     string   `toml:"client_id"`
	ClientSecret string   `toml:"client_secret"`
	Scopes       []string `toml:"scopes"`
	DisplayName  string   `toml:"display_name"`
}

// SAMLProvider configures a SAML 2.0 identity provider integration.
type SAMLProvider struct {
	Enabled          bool              `toml:"enabled"`
	Name             string            `toml:"name"`
	EntityID         string            `toml:"entity_id"`
	IDPMetadataURL   string            `toml:"idp_metadata_url"`
	IDPMetadataXML   string            `toml:"idp_metadata_xml"`
	AttributeMapping map[string]string `toml:"attribute_mapping"`
	SPCertFile       string            `toml:"sp_cert_file"`
	SPKeyFile        string            `toml:"sp_key_file"`
}

// EmailConfig controls how AYB sends transactional emails (verification, password reset).
// When Backend is "" or "log", emails are printed to the console (dev mode).
type EmailConfig struct {
	Backend  string             `toml:"backend"` // "log" (default), "smtp", "webhook"
	From     string             `toml:"from"`
	FromName string             `toml:"from_name"`
	SMTP     EmailSMTPConfig    `toml:"smtp"`
	Webhook  EmailWebhookConfig `toml:"webhook"`
	Policy   EmailPolicyConfig  `toml:"policy"`
}

// EmailPolicyConfig controls abuse-prevention policy for the public email send API.
type EmailPolicyConfig struct {
	AllowedFromAddresses    []string `toml:"allowed_from_addresses"`     // empty = only Email.From allowed
	MaxRecipientsPerRequest int      `toml:"max_recipients_per_request"` // default 50
	SendRateLimitPerKey     int      `toml:"send_rate_limit_per_key"`    // max sends per key per window, default 100
	SendRateLimitWindow     int      `toml:"send_rate_limit_window"`     // window in seconds, default 3600
}

// EffectiveAllowedFrom returns the allowed from-address list, falling back to the
// configured default From address when the whitelist is empty.
func (c *EmailConfig) EffectiveAllowedFrom() []string {
	if len(c.Policy.AllowedFromAddresses) > 0 {
		return c.Policy.AllowedFromAddresses
	}
	if c.From != "" {
		return []string{c.From}
	}
	return nil
}

// EffectiveMaxRecipients returns MaxRecipientsPerRequest or the default (50).
func (c *EmailPolicyConfig) EffectiveMaxRecipients() int {
	if c.MaxRecipientsPerRequest > 0 {
		return c.MaxRecipientsPerRequest
	}
	return 50
}

// EffectiveSendRateLimit returns SendRateLimitPerKey or the default (100).
func (c *EmailPolicyConfig) EffectiveSendRateLimit() int {
	if c.SendRateLimitPerKey > 0 {
		return c.SendRateLimitPerKey
	}
	return 100
}

// EffectiveSendRateWindow returns SendRateLimitWindow or the default (3600 seconds).
func (c *EmailPolicyConfig) EffectiveSendRateWindow() int {
	if c.SendRateLimitWindow > 0 {
		return c.SendRateLimitWindow
	}
	return 3600
}

type EmailSMTPConfig struct {
	Host       string `toml:"host"`
	Port       int    `toml:"port"`
	Username   string `toml:"username"`
	Password   string `toml:"password"`
	AuthMethod string `toml:"auth_method"` // PLAIN, LOGIN, CRAM-MD5
	TLS        bool   `toml:"tls"`
}

type EmailWebhookConfig struct {
	URL     string `toml:"url"`
	Secret  string `toml:"secret"`
	Timeout int    `toml:"timeout"` // seconds, default 10
}

type StorageConfig struct {
	Enabled        bool      `toml:"enabled"`
	Backend        string    `toml:"backend"`
	LocalPath      string    `toml:"local_path"`
	CDNURL         string    `toml:"cdn_url"`
	CDN            CDNConfig `toml:"cdn"`
	MaxFileSize    string    `toml:"max_file_size"`
	DefaultQuotaMB int       `toml:"default_quota_mb"`
	S3Endpoint     string    `toml:"s3_endpoint"`
	S3Bucket       string    `toml:"s3_bucket"`
	S3Region       string    `toml:"s3_region"`
	S3AccessKey    string    `toml:"s3_access_key"`
	S3SecretKey    string    `toml:"s3_secret_key"`
	S3UseSSL       bool      `toml:"s3_use_ssl"`
}

type EdgeFuncConfig struct {
	PoolSize                 int      `toml:"pool_size"`
	DefaultTimeoutMs         int      `toml:"default_timeout_ms"`
	MaxRequestBodyBytes      int64    `toml:"max_request_body_bytes"`
	FetchDomainAllowlist     []string `toml:"fetch_domain_allowlist"`
	MemoryLimitMB            int      `toml:"memory_limit_mb"`
	MaxConcurrentInvocations int      `toml:"max_concurrent_invocations"`
	CodeCacheSize            int      `toml:"code_cache_size"`
}

// DashboardAIConfig controls dashboard-only AI assistant orchestration endpoints.
type DashboardAIConfig struct {
	Enabled   bool   `toml:"enabled"`
	RateLimit string `toml:"rate_limit"` // assistant requests per minute/hour per dashboard actor (default "20/min")
}

// ProviderConfig holds per-provider AI settings.
type ProviderConfig struct {
	APIKey       string `toml:"api_key"`
	BaseURL      string `toml:"base_url"`
	DefaultModel string `toml:"default_model"`
}

// AIBreakerConfig controls per-provider circuit-breaker behavior.
type AIBreakerConfig struct {
	FailureThreshold   int `toml:"failure_threshold"`
	OpenSeconds        int `toml:"open_seconds"`
	HalfOpenProbeLimit int `toml:"half_open_probe_limit"`
}

// AIConfig configures the AI runtime (providers, models, retry behaviour).
type AIConfig struct {
	DefaultProvider     string                    `toml:"default_provider"`
	DefaultModel        string                    `toml:"default_model"`
	EmbeddingProvider   string                    `toml:"embedding_provider"` // defaults to DefaultProvider
	EmbeddingModel      string                    `toml:"embedding_model"`    // defaults to provider's DefaultModel
	TimeoutSecs         int                       `toml:"timeout"`            // seconds, default 30
	MaxRetries          int                       `toml:"max_retries"`
	Breaker             AIBreakerConfig           `toml:"breaker"`
	EmbeddingDimensions map[string]int            `toml:"embedding_dimensions"` // key format: provider:model
	Providers           map[string]ProviderConfig `toml:"providers"`
}

// EmbeddingDimension returns a configured embedding dimension for provider:model.
// Matching is case-insensitive.
func (c AIConfig) EmbeddingDimension(provider, model string) (int, bool) {
	if provider == "" || model == "" {
		return 0, false
	}
	key := strings.ToLower(strings.TrimSpace(provider) + ":" + strings.TrimSpace(model))
	for rawKey, dim := range c.EmbeddingDimensions {
		if strings.ToLower(strings.TrimSpace(rawKey)) == key {
			return dim, true
		}
	}
	return 0, false
}

// EncryptedColumnConfig defines columns within a table that are stored encrypted at rest.
type EncryptedColumnConfig struct {
	Table   string   `toml:"table"`
	Columns []string `toml:"columns"`
}

// MaxFileSizeBytes parses MaxFileSize values like "10MB" and falls back to 10 MB.
func (c *StorageConfig) MaxFileSizeBytes() int64 {
	s := strings.TrimSpace(strings.ToUpper(c.MaxFileSize))
	s = strings.TrimSuffix(s, "B") // strip trailing B (MB->M, GB->G, KB->K)

	var shift int64
	switch {
	case strings.HasSuffix(s, "G"):
		s = strings.TrimSuffix(s, "G")
		shift = 30
	case strings.HasSuffix(s, "K"):
		s = strings.TrimSuffix(s, "K")
		shift = 10
	default:
		s = strings.TrimSuffix(s, "M")
		shift = 20
	}

	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n <= 0 {
		return 10 << 20 // 10MB default
	}
	return n << shift
}

// DefaultQuotaBytes returns the default per-user storage quota in bytes.
// Falls back to 100 MB if DefaultQuotaMB is zero or negative.
func (c *StorageConfig) DefaultQuotaBytes() int64 {
	if c.DefaultQuotaMB <= 0 {
		return 100 * 1024 * 1024
	}
	return int64(c.DefaultQuotaMB) * 1024 * 1024
}

// validISO3166Alpha2 is the set of valid ISO 3166-1 alpha-2 country codes.
var validISO3166Alpha2 = map[string]bool{
	"AD": true, "AE": true, "AF": true, "AG": true, "AI": true, "AL": true, "AM": true, "AO": true,
	"AQ": true, "AR": true, "AS": true, "AT": true, "AU": true, "AW": true, "AX": true, "AZ": true,
	"BA": true, "BB": true, "BD": true, "BE": true, "BF": true, "BG": true, "BH": true, "BI": true,
	"BJ": true, "BL": true, "BM": true, "BN": true, "BO": true, "BQ": true, "BR": true, "BS": true,
	"BT": true, "BV": true, "BW": true, "BY": true, "BZ": true,
	"CA": true, "CC": true, "CD": true, "CF": true, "CG": true, "CH": true, "CI": true, "CK": true,
	"CL": true, "CM": true, "CN": true, "CO": true, "CR": true, "CU": true, "CV": true, "CW": true,
	"CX": true, "CY": true, "CZ": true,
	"DE": true, "DJ": true, "DK": true, "DM": true, "DO": true, "DZ": true,
	"EC": true, "EE": true, "EG": true, "EH": true, "ER": true, "ES": true, "ET": true,
	"FI": true, "FJ": true, "FK": true, "FM": true, "FO": true, "FR": true,
	"GA": true, "GB": true, "GD": true, "GE": true, "GF": true, "GG": true, "GH": true, "GI": true,
	"GL": true, "GM": true, "GN": true, "GP": true, "GQ": true, "GR": true, "GS": true, "GT": true,
	"GU": true, "GW": true, "GY": true,
	"HK": true, "HM": true, "HN": true, "HR": true, "HT": true, "HU": true,
	"ID": true, "IE": true, "IL": true, "IM": true, "IN": true, "IO": true, "IQ": true, "IR": true,
	"IS": true, "IT": true,
	"JE": true, "JM": true, "JO": true, "JP": true,
	"KE": true, "KG": true, "KH": true, "KI": true, "KM": true, "KN": true, "KP": true, "KR": true,
	"KW": true, "KY": true, "KZ": true,
	"LA": true, "LB": true, "LC": true, "LI": true, "LK": true, "LR": true, "LS": true, "LT": true,
	"LU": true, "LV": true, "LY": true,
	"MA": true, "MC": true, "MD": true, "ME": true, "MF": true, "MG": true, "MH": true, "MK": true,
	"ML": true, "MM": true, "MN": true, "MO": true, "MP": true, "MQ": true, "MR": true, "MS": true,
	"MT": true, "MU": true, "MV": true, "MW": true, "MX": true, "MY": true, "MZ": true,
	"NA": true, "NC": true, "NE": true, "NF": true, "NG": true, "NI": true, "NL": true, "NO": true,
	"NP": true, "NR": true, "NU": true, "NZ": true,
	"OM": true,
	"PA": true, "PE": true, "PF": true, "PG": true, "PH": true, "PK": true, "PL": true, "PM": true,
	"PN": true, "PR": true, "PS": true, "PT": true, "PW": true, "PY": true,
	"QA": true,
	"RE": true, "RO": true, "RS": true, "RU": true, "RW": true,
	"SA": true, "SB": true, "SC": true, "SD": true, "SE": true, "SG": true, "SH": true, "SI": true,
	"SJ": true, "SK": true, "SL": true, "SM": true, "SN": true, "SO": true, "SR": true, "SS": true,
	"ST": true, "SV": true, "SX": true, "SY": true, "SZ": true,
	"TC": true, "TD": true, "TF": true, "TG": true, "TH": true, "TJ": true, "TK": true, "TL": true,
	"TM": true, "TN": true, "TO": true, "TR": true, "TT": true, "TV": true, "TW": true, "TZ": true,
	"UA": true, "UG": true, "UM": true, "US": true, "UY": true, "UZ": true,
	"VA": true, "VC": true, "VE": true, "VG": true, "VI": true, "VN": true, "VU": true,
	"WF": true, "WS": true,
	"YE": true, "YT": true,
	"ZA": true, "ZM": true, "ZW": true,
}
