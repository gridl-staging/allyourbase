package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// Config is the top-level AYB configuration.
type Config struct {
	Server   ServerConfig   `toml:"server"`
	Database DatabaseConfig `toml:"database"`
	Admin    AdminConfig    `toml:"admin"`
	Auth     AuthConfig     `toml:"auth"`
	Email    EmailConfig    `toml:"email"`
	Storage  StorageConfig  `toml:"storage"`
	Logging  LoggingConfig  `toml:"logging"`
	Jobs     JobsConfig     `toml:"jobs"`
}

type ServerConfig struct {
	Host               string   `toml:"host"`
	Port               int      `toml:"port"`
	SiteURL            string   `toml:"site_url"` // public base URL for email action links (e.g. "https://myapp.example.com")
	CORSAllowedOrigins []string `toml:"cors_allowed_origins"`
	BodyLimit          string   `toml:"body_limit"`
	ShutdownTimeout    int      `toml:"shutdown_timeout"`
	// TLS — set tls_domain to enable automatic HTTPS via Let's Encrypt.
	TLSEnabled bool   `toml:"tls_enabled"` // auto-set when TLSDomain is non-empty
	TLSDomain  string `toml:"tls_domain"`
	TLSCertDir string `toml:"tls_cert_dir"` // default: ~/.ayb/certs at runtime
	TLSEmail   string `toml:"tls_email"`    // ACME account email (recommended)
}

type DatabaseConfig struct {
	URL             string `toml:"url"`
	MaxConns        int    `toml:"max_conns"`
	MinConns        int    `toml:"min_conns"`
	HealthCheckSecs int    `toml:"health_check_interval"`
	EmbeddedPort    int    `toml:"embedded_port"`
	EmbeddedDataDir string `toml:"embedded_data_dir"`
	MigrationsDir   string `toml:"migrations_dir"`
}

type AdminConfig struct {
	Enabled        bool   `toml:"enabled"`
	Path           string `toml:"path"`
	Password       string `toml:"password"`
	LoginRateLimit int    `toml:"login_rate_limit"` // admin login attempts per minute per IP (default 20)
}

type AuthConfig struct {
	Enabled              bool                     `toml:"enabled"`
	JWTSecret            string                   `toml:"jwt_secret"`
	TokenDuration        int                      `toml:"token_duration"`
	RefreshTokenDuration int                      `toml:"refresh_token_duration"`
	RateLimit            int                      `toml:"rate_limit"`
	MinPasswordLength    int                      `toml:"min_password_length"`
	OAuth                map[string]OAuthProvider `toml:"oauth"`
	OAuthRedirectURL     string                   `toml:"oauth_redirect_url"`
	MagicLinkEnabled     bool                     `toml:"magic_link_enabled"`
	MagicLinkDuration    int                      `toml:"magic_link_duration"` // seconds, default 600 (10 min)
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
}

// OAuthProviderModeConfig controls AYB's OAuth 2.0 authorization server.
// When Enabled, AYB can issue access/refresh tokens to registered OAuth clients.
type OAuthProviderModeConfig struct {
	Enabled              bool `toml:"enabled"`
	AccessTokenDuration  int  `toml:"access_token_duration"`  // seconds, default 3600 (1h)
	RefreshTokenDuration int  `toml:"refresh_token_duration"` // seconds, default 2592000 (30d)
	AuthCodeDuration     int  `toml:"auth_code_duration"`     // seconds, default 600 (10min)
}

// OAuthProvider configures a single OAuth2 provider (e.g. google, github).
type OAuthProvider struct {
	Enabled      bool   `toml:"enabled"`
	ClientID     string `toml:"client_id"`
	ClientSecret string `toml:"client_secret"`
}

// EmailConfig controls how AYB sends transactional emails (verification, password reset).
// When Backend is "" or "log", emails are printed to the console (dev mode).
type EmailConfig struct {
	Backend  string             `toml:"backend"` // "log" (default), "smtp", "webhook"
	From     string             `toml:"from"`
	FromName string             `toml:"from_name"`
	SMTP     EmailSMTPConfig    `toml:"smtp"`
	Webhook  EmailWebhookConfig `toml:"webhook"`
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
	Enabled     bool   `toml:"enabled"`
	Backend     string `toml:"backend"`
	LocalPath   string `toml:"local_path"`
	MaxFileSize string `toml:"max_file_size"`
	S3Endpoint  string `toml:"s3_endpoint"`
	S3Bucket    string `toml:"s3_bucket"`
	S3Region    string `toml:"s3_region"`
	S3AccessKey string `toml:"s3_access_key"`
	S3SecretKey string `toml:"s3_secret_key"`
	S3UseSSL    bool   `toml:"s3_use_ssl"`
}

type LoggingConfig struct {
	Level  string `toml:"level"`
	Format string `toml:"format"`
}

type JobsConfig struct {
	Enabled           bool `toml:"enabled"`             // default false
	WorkerConcurrency int  `toml:"worker_concurrency"`  // default 4
	PollIntervalMs    int  `toml:"poll_interval_ms"`    // default 1000
	LeaseDurationS    int  `toml:"lease_duration_s"`    // default 300 (5 min)
	MaxRetriesDefault int  `toml:"max_retries_default"` // default 3
	SchedulerEnabled  bool `toml:"scheduler_enabled"`   // default true (when jobs enabled)
	SchedulerTickS    int  `toml:"scheduler_tick_s"`    // default 15
}

// Default returns a Config with all defaults applied.
func Default() *Config {
	return &Config{
		Server: ServerConfig{
			Host:               "0.0.0.0",
			Port:               8090,
			CORSAllowedOrigins: []string{"*"},
			BodyLimit:          "1MB",
			ShutdownTimeout:    10,
		},
		Database: DatabaseConfig{
			MaxConns:        25,
			MinConns:        2,
			HealthCheckSecs: 30,
			EmbeddedPort:    15432,
			MigrationsDir:   "./migrations",
		},
		Admin: AdminConfig{
			Enabled:        true,
			Path:           "/admin",
			LoginRateLimit: 20,
		},
		Auth: AuthConfig{
			TokenDuration:        900,    // 15 minutes
			RefreshTokenDuration: 604800, // 7 days
			RateLimit:            10,     // requests per minute per IP
			MinPasswordLength:    8,      // NIST SP 800-63B recommended minimum
			MagicLinkDuration:    600,    // 10 minutes
			SMSProvider:          "log",
			SMSCodeLength:        6,
			SMSCodeExpiry:        300, // 5 minutes
			SMSMaxAttempts:       3,
			SMSDailyLimit:        1000,
			SMSAllowedCountries:  []string{"US", "CA"},
			OAuthProviderMode: OAuthProviderModeConfig{
				AccessTokenDuration:  3600,    // 1 hour
				RefreshTokenDuration: 2592000, // 30 days
				AuthCodeDuration:     600,     // 10 minutes
			},
		},
		Email: EmailConfig{
			Backend:  "log",
			FromName: "Allyourbase",
		},
		Storage: StorageConfig{
			Backend:     "local",
			LocalPath:   "./ayb_storage",
			MaxFileSize: "10MB",
			S3Region:    "us-east-1",
			S3UseSSL:    true,
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "json",
		},
		Jobs: JobsConfig{
			Enabled:           false,
			WorkerConcurrency: 4,
			PollIntervalMs:    1000,
			LeaseDurationS:    300,
			MaxRetriesDefault: 3,
			SchedulerEnabled:  true,
			SchedulerTickS:    15,
		},
	}
}

// Load reads configuration with priority: defaults → ayb.toml → env vars → CLI flags.
// The flags parameter allows CLI flag overrides to be passed in.
func Load(configPath string, flags map[string]string) (*Config, error) {
	cfg := Default()

	// Load from TOML file if it exists.
	if configPath == "" {
		configPath = "ayb.toml"
	}
	if data, err := os.ReadFile(configPath); err == nil {
		if err := toml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("parsing %s: %w", configPath, err)
		}
	}

	// Apply environment variables.
	if err := applyEnv(cfg); err != nil {
		return nil, err
	}

	// Apply CLI flag overrides.
	applyFlags(cfg, flags)

	// Validate.
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation: %w", err)
	}

	return cfg, nil
}

// Validate checks the configuration for invalid values.
func (c *Config) Validate() error {
	// Auto-enable TLS when a domain is configured.
	if c.Server.TLSDomain != "" {
		c.Server.TLSEnabled = true
	}
	if c.Server.TLSEnabled && c.Server.TLSDomain == "" {
		return fmt.Errorf("server.tls_domain is required when TLS is enabled")
	}
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return fmt.Errorf("server.port must be between 1 and 65535, got %d", c.Server.Port)
	}
	if c.Database.MaxConns < 1 {
		return fmt.Errorf("database.max_conns must be at least 1, got %d", c.Database.MaxConns)
	}
	if c.Database.MinConns < 0 {
		return fmt.Errorf("database.min_conns must be non-negative, got %d", c.Database.MinConns)
	}
	if c.Database.MinConns > c.Database.MaxConns {
		return fmt.Errorf("database.min_conns (%d) cannot exceed database.max_conns (%d)", c.Database.MinConns, c.Database.MaxConns)
	}
	if c.Database.URL == "" && (c.Database.EmbeddedPort < 1 || c.Database.EmbeddedPort > 65535) {
		return fmt.Errorf("database.embedded_port must be between 1 and 65535, got %d", c.Database.EmbeddedPort)
	}
	if c.Auth.MinPasswordLength < 1 {
		return fmt.Errorf("auth.min_password_length must be at least 1, got %d", c.Auth.MinPasswordLength)
	}
	if c.Auth.Enabled && c.Auth.JWTSecret == "" {
		return fmt.Errorf("auth.jwt_secret is required when auth is enabled")
	}
	if c.Auth.JWTSecret != "" && len(c.Auth.JWTSecret) < 32 {
		return fmt.Errorf("auth.jwt_secret must be at least 32 characters, got %d", len(c.Auth.JWTSecret))
	}
	if c.Auth.MagicLinkEnabled && !c.Auth.Enabled {
		return fmt.Errorf("auth.enabled must be true to use magic link authentication")
	}
	if c.Auth.SMSEnabled {
		if !c.Auth.Enabled {
			return fmt.Errorf("sms_enabled requires auth.enabled")
		}
		switch c.Auth.SMSProvider {
		case "twilio":
			if c.Auth.TwilioSID == "" {
				return fmt.Errorf("auth.twilio_sid is required when sms_provider is \"twilio\"")
			}
			if c.Auth.TwilioToken == "" {
				return fmt.Errorf("auth.twilio_token is required when sms_provider is \"twilio\"")
			}
			if c.Auth.TwilioFrom == "" {
				return fmt.Errorf("auth.twilio_from is required when sms_provider is \"twilio\"")
			}
		case "plivo":
			if c.Auth.PlivoAuthID == "" {
				return fmt.Errorf("auth.plivo_auth_id is required when sms_provider is \"plivo\"")
			}
			if c.Auth.PlivoAuthToken == "" {
				return fmt.Errorf("auth.plivo_auth_token is required when sms_provider is \"plivo\"")
			}
			if c.Auth.PlivoFrom == "" {
				return fmt.Errorf("auth.plivo_from is required when sms_provider is \"plivo\"")
			}
		case "telnyx":
			if c.Auth.TelnyxAPIKey == "" {
				return fmt.Errorf("auth.telnyx_api_key is required when sms_provider is \"telnyx\"")
			}
			if c.Auth.TelnyxFrom == "" {
				return fmt.Errorf("auth.telnyx_from is required when sms_provider is \"telnyx\"")
			}
		case "msg91":
			if c.Auth.MSG91AuthKey == "" {
				return fmt.Errorf("auth.msg91_auth_key is required when sms_provider is \"msg91\"")
			}
			if c.Auth.MSG91TemplateID == "" {
				return fmt.Errorf("auth.msg91_template_id is required when sms_provider is \"msg91\"")
			}
		case "sns":
			if c.Auth.AWSRegion == "" {
				return fmt.Errorf("auth.aws_region is required when sms_provider is \"sns\"")
			}
		case "vonage":
			if c.Auth.VonageAPIKey == "" {
				return fmt.Errorf("auth.vonage_api_key is required when sms_provider is \"vonage\"")
			}
			if c.Auth.VonageAPISecret == "" {
				return fmt.Errorf("auth.vonage_api_secret is required when sms_provider is \"vonage\"")
			}
			if c.Auth.VonageFrom == "" {
				return fmt.Errorf("auth.vonage_from is required when sms_provider is \"vonage\"")
			}
		case "webhook":
			if c.Auth.SMSWebhookURL == "" {
				return fmt.Errorf("auth.sms_webhook_url is required when sms_provider is \"webhook\"")
			}
			if c.Auth.SMSWebhookSecret == "" {
				return fmt.Errorf("auth.sms_webhook_secret is required when sms_provider is \"webhook\"")
			}
		case "log":
		default:
			return fmt.Errorf("auth.sms_provider must be one of: \"log\", \"twilio\", \"plivo\", \"telnyx\", \"msg91\", \"sns\", \"vonage\", \"webhook\"; got %q", c.Auth.SMSProvider)
		}
		if c.Auth.SMSCodeLength < 4 || c.Auth.SMSCodeLength > 8 {
			return fmt.Errorf("auth.sms_code_length must be between 4 and 8, got %d", c.Auth.SMSCodeLength)
		}
		if c.Auth.SMSCodeExpiry < 60 || c.Auth.SMSCodeExpiry > 600 {
			return fmt.Errorf("auth.sms_code_expiry must be between 60 and 600, got %d", c.Auth.SMSCodeExpiry)
		}
		if c.Auth.SMSDailyLimit < 0 {
			return fmt.Errorf("auth.sms_daily_limit must be non-negative, got %d", c.Auth.SMSDailyLimit)
		}
		for _, code := range c.Auth.SMSAllowedCountries {
			if !validISO3166Alpha2[code] {
				return fmt.Errorf("auth.sms_allowed_countries: %q is not a valid ISO 3166-1 alpha-2 country code", code)
			}
		}
	}
	for name, p := range c.Auth.OAuth {
		if p.Enabled {
			if !c.Auth.Enabled {
				return fmt.Errorf("auth.enabled must be true to use OAuth provider %q", name)
			}
			if p.ClientID == "" {
				return fmt.Errorf("auth.oauth.%s.client_id is required when enabled", name)
			}
			if p.ClientSecret == "" {
				return fmt.Errorf("auth.oauth.%s.client_secret is required when enabled", name)
			}
			switch name {
			case "google", "github":
			default:
				return fmt.Errorf("unsupported OAuth provider %q (supported: google, github)", name)
			}
		}
	}
	if c.Auth.OAuthProviderMode.Enabled {
		if !c.Auth.Enabled {
			return fmt.Errorf("auth.enabled must be true to use OAuth provider mode")
		}
		if c.Auth.JWTSecret == "" {
			return fmt.Errorf("auth.jwt_secret is required for OAuth provider mode (used by consent flow session auth)")
		}
		if c.Auth.OAuthProviderMode.AccessTokenDuration < 1 {
			return fmt.Errorf("auth.oauth_provider.access_token_duration must be at least 1, got %d", c.Auth.OAuthProviderMode.AccessTokenDuration)
		}
		if c.Auth.OAuthProviderMode.RefreshTokenDuration < 1 {
			return fmt.Errorf("auth.oauth_provider.refresh_token_duration must be at least 1, got %d", c.Auth.OAuthProviderMode.RefreshTokenDuration)
		}
		if c.Auth.OAuthProviderMode.AuthCodeDuration < 1 {
			return fmt.Errorf("auth.oauth_provider.auth_code_duration must be at least 1, got %d", c.Auth.OAuthProviderMode.AuthCodeDuration)
		}
	}
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
	if c.Storage.Enabled {
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
	}
	if c.Logging.Level != "" {
		switch c.Logging.Level {
		case "debug", "info", "warn", "error":
		default:
			return fmt.Errorf("logging.level must be one of: debug, info, warn, error; got %q", c.Logging.Level)
		}
	}
	if c.Jobs.Enabled {
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
	}
	return nil
}

// Address returns the host:port string for the server to listen on.
func (c *Config) Address() string {
	return fmt.Sprintf("%s:%d", c.Server.Host, c.Server.Port)
}

// PublicBaseURL returns the public base URL for email action links (password reset,
// magic links, etc.). If server.site_url is configured, it is used as-is (with
// trailing slashes stripped). Otherwise, a URL is constructed from host:port,
// replacing the bind-all address 0.0.0.0 with localhost so links work in browsers.
func (c *Config) PublicBaseURL() string {
	if c.Server.SiteURL != "" {
		return strings.TrimRight(c.Server.SiteURL, "/")
	}
	if c.Server.TLSEnabled && c.Server.TLSDomain != "" {
		return fmt.Sprintf("https://%s", c.Server.TLSDomain)
	}
	host := c.Server.Host
	if host == "0.0.0.0" || host == "" {
		host = "localhost"
	}
	return fmt.Sprintf("http://%s:%d", host, c.Server.Port)
}

// GenerateDefault writes a commented default ayb.toml to the given path.
func GenerateDefault(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(defaultTOML), 0o600)
}

// ToTOML returns the config serialized as TOML.
func (c *Config) ToTOML() (string, error) {
	data, err := toml.Marshal(c)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// maskSecret replaces a non-empty secret string with "***".
func maskSecret(s string) string {
	if s == "" {
		return ""
	}
	return "***"
}

// MaskedCopy returns a deep copy of the config with all secret fields redacted.
// Use this for display purposes (e.g. ayb config) to avoid leaking credentials.
func (c *Config) MaskedCopy() *Config {
	cp := *c

	// Admin.
	cp.Admin.Password = maskSecret(c.Admin.Password)

	// Auth secrets.
	cp.Auth.JWTSecret = maskSecret(c.Auth.JWTSecret)
	cp.Auth.TwilioToken = maskSecret(c.Auth.TwilioToken)
	cp.Auth.TwilioSID = maskSecret(c.Auth.TwilioSID)
	cp.Auth.PlivoAuthToken = maskSecret(c.Auth.PlivoAuthToken)
	cp.Auth.TelnyxAPIKey = maskSecret(c.Auth.TelnyxAPIKey)
	cp.Auth.MSG91AuthKey = maskSecret(c.Auth.MSG91AuthKey)
	cp.Auth.VonageAPIKey = maskSecret(c.Auth.VonageAPIKey)
	cp.Auth.VonageAPISecret = maskSecret(c.Auth.VonageAPISecret)
	cp.Auth.SMSWebhookSecret = maskSecret(c.Auth.SMSWebhookSecret)

	// Mask OAuth client secrets (make a new map to avoid mutating the original).
	if len(c.Auth.OAuth) > 0 {
		cp.Auth.OAuth = make(map[string]OAuthProvider, len(c.Auth.OAuth))
		for name, p := range c.Auth.OAuth {
			p.ClientSecret = maskSecret(p.ClientSecret)
			cp.Auth.OAuth[name] = p
		}
	}

	// Email secrets.
	cp.Email.SMTP.Password = maskSecret(c.Email.SMTP.Password)
	cp.Email.Webhook.Secret = maskSecret(c.Email.Webhook.Secret)

	// Storage secrets.
	cp.Storage.S3AccessKey = maskSecret(c.Storage.S3AccessKey)
	cp.Storage.S3SecretKey = maskSecret(c.Storage.S3SecretKey)

	// Database URL may contain a password — redact the userinfo portion.
	cp.Database.URL = redactDatabaseURL(c.Database.URL)

	return &cp
}

// redactDatabaseURL replaces the password in a PostgreSQL connection URL with "***".
// Non-URL strings (e.g. empty) are returned as-is.
func redactDatabaseURL(raw string) string {
	if raw == "" {
		return ""
	}
	// Use net/url parsing — import is already available via go-toml.
	// We do a simple string operation to avoid an extra import in this package.
	// Format: postgresql://user:password@host/db
	const atSign = "@"
	const schemeEnd = "://"
	schIdx := strings.Index(raw, schemeEnd)
	if schIdx < 0 {
		return raw
	}
	rest := raw[schIdx+3:]
	atIdx := strings.Index(rest, atSign)
	if atIdx < 0 {
		return raw // no userinfo
	}
	userinfo := rest[:atIdx]
	colonIdx := strings.Index(userinfo, ":")
	if colonIdx < 0 {
		return raw // no password
	}
	redacted := raw[:schIdx+3] + userinfo[:colonIdx+1] + "***" + rest[atIdx:]
	return redacted
}

// envInt reads an integer from the named environment variable.
// Returns an error if the value is set but not a valid integer.
func envInt(name string, dest *int) error {
	v := os.Getenv(name)
	if v == "" {
		return nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fmt.Errorf("invalid value for %s: %q is not an integer", name, v)
	}
	*dest = n
	return nil
}

func applyEnv(cfg *Config) error {
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
	if v := os.Getenv("AYB_SERVER_SITE_URL"); v != "" {
		cfg.Server.SiteURL = v
	}
	if v := os.Getenv("AYB_DATABASE_URL"); v != "" {
		cfg.Database.URL = v
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
	if v := os.Getenv("AYB_ADMIN_PASSWORD"); v != "" {
		cfg.Admin.Password = v
	}
	if err := envInt("AYB_ADMIN_LOGIN_RATE_LIMIT", &cfg.Admin.LoginRateLimit); err != nil {
		return err
	}
	if v := os.Getenv("AYB_LOG_LEVEL"); v != "" {
		cfg.Logging.Level = v
	}
	if v := os.Getenv("AYB_CORS_ORIGINS"); v != "" {
		cfg.Server.CORSAllowedOrigins = strings.Split(v, ",")
	}
	if v := os.Getenv("AYB_AUTH_ENABLED"); v != "" {
		cfg.Auth.Enabled = v == "true" || v == "1"
	}
	if v := os.Getenv("AYB_AUTH_JWT_SECRET"); v != "" {
		cfg.Auth.JWTSecret = v
	}
	if err := envInt("AYB_AUTH_REFRESH_TOKEN_DURATION", &cfg.Auth.RefreshTokenDuration); err != nil {
		return err
	}
	if err := envInt("AYB_AUTH_RATE_LIMIT", &cfg.Auth.RateLimit); err != nil {
		return err
	}
	if err := envInt("AYB_AUTH_MIN_PASSWORD_LENGTH", &cfg.Auth.MinPasswordLength); err != nil {
		return err
	}
	if v := os.Getenv("AYB_AUTH_OAUTH_REDIRECT_URL"); v != "" {
		cfg.Auth.OAuthRedirectURL = v
	}
	if v := os.Getenv("AYB_AUTH_OAUTH_PROVIDER_ENABLED"); v != "" {
		cfg.Auth.OAuthProviderMode.Enabled = v == "true" || v == "1"
	}
	if err := envInt("AYB_AUTH_OAUTH_PROVIDER_ACCESS_TOKEN_DURATION", &cfg.Auth.OAuthProviderMode.AccessTokenDuration); err != nil {
		return err
	}
	if err := envInt("AYB_AUTH_OAUTH_PROVIDER_REFRESH_TOKEN_DURATION", &cfg.Auth.OAuthProviderMode.RefreshTokenDuration); err != nil {
		return err
	}
	if err := envInt("AYB_AUTH_OAUTH_PROVIDER_AUTH_CODE_DURATION", &cfg.Auth.OAuthProviderMode.AuthCodeDuration); err != nil {
		return err
	}
	if v := os.Getenv("AYB_AUTH_MAGIC_LINK_ENABLED"); v != "" {
		cfg.Auth.MagicLinkEnabled = v == "true" || v == "1"
	}
	if err := envInt("AYB_AUTH_MAGIC_LINK_DURATION", &cfg.Auth.MagicLinkDuration); err != nil {
		return err
	}
	// SMS config.
	if v := os.Getenv("AYB_AUTH_SMS_ENABLED"); v != "" {
		cfg.Auth.SMSEnabled = v == "true" || v == "1"
	}
	if v := os.Getenv("AYB_AUTH_SMS_PROVIDER"); v != "" {
		cfg.Auth.SMSProvider = v
	}
	if v := os.Getenv("AYB_AUTH_TWILIO_SID"); v != "" {
		cfg.Auth.TwilioSID = v
	}
	if v := os.Getenv("AYB_AUTH_TWILIO_TOKEN"); v != "" {
		cfg.Auth.TwilioToken = v
	}
	if v := os.Getenv("AYB_AUTH_TWILIO_FROM"); v != "" {
		cfg.Auth.TwilioFrom = v
	}
	// Plivo
	if v := os.Getenv("AYB_AUTH_PLIVO_AUTH_ID"); v != "" {
		cfg.Auth.PlivoAuthID = v
	}
	if v := os.Getenv("AYB_AUTH_PLIVO_AUTH_TOKEN"); v != "" {
		cfg.Auth.PlivoAuthToken = v
	}
	if v := os.Getenv("AYB_AUTH_PLIVO_FROM"); v != "" {
		cfg.Auth.PlivoFrom = v
	}
	// Telnyx
	if v := os.Getenv("AYB_AUTH_TELNYX_API_KEY"); v != "" {
		cfg.Auth.TelnyxAPIKey = v
	}
	if v := os.Getenv("AYB_AUTH_TELNYX_FROM"); v != "" {
		cfg.Auth.TelnyxFrom = v
	}
	// MSG91
	if v := os.Getenv("AYB_AUTH_MSG91_AUTH_KEY"); v != "" {
		cfg.Auth.MSG91AuthKey = v
	}
	if v := os.Getenv("AYB_AUTH_MSG91_TEMPLATE_ID"); v != "" {
		cfg.Auth.MSG91TemplateID = v
	}
	// AWS SNS
	if v := os.Getenv("AYB_AUTH_AWS_REGION"); v != "" {
		cfg.Auth.AWSRegion = v
	}
	// Vonage
	if v := os.Getenv("AYB_AUTH_VONAGE_API_KEY"); v != "" {
		cfg.Auth.VonageAPIKey = v
	}
	if v := os.Getenv("AYB_AUTH_VONAGE_API_SECRET"); v != "" {
		cfg.Auth.VonageAPISecret = v
	}
	if v := os.Getenv("AYB_AUTH_VONAGE_FROM"); v != "" {
		cfg.Auth.VonageFrom = v
	}
	// SMS Webhook
	if v := os.Getenv("AYB_AUTH_SMS_WEBHOOK_URL"); v != "" {
		cfg.Auth.SMSWebhookURL = v
	}
	if v := os.Getenv("AYB_AUTH_SMS_WEBHOOK_SECRET"); v != "" {
		cfg.Auth.SMSWebhookSecret = v
	}
	// Email config.
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
	if v := os.Getenv("AYB_STORAGE_ENABLED"); v != "" {
		cfg.Storage.Enabled = v == "true" || v == "1"
	}
	if v := os.Getenv("AYB_STORAGE_BACKEND"); v != "" {
		cfg.Storage.Backend = v
	}
	if v := os.Getenv("AYB_STORAGE_LOCAL_PATH"); v != "" {
		cfg.Storage.LocalPath = v
	}
	if v := os.Getenv("AYB_STORAGE_MAX_FILE_SIZE"); v != "" {
		cfg.Storage.MaxFileSize = v
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
	applyOAuthEnv(cfg, "google")
	applyOAuthEnv(cfg, "github")
	// Jobs config.
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

func applyOAuthEnv(cfg *Config, provider string) {
	prefix := "AYB_AUTH_OAUTH_" + strings.ToUpper(provider) + "_"
	id := os.Getenv(prefix + "CLIENT_ID")
	secret := os.Getenv(prefix + "CLIENT_SECRET")
	enabled := os.Getenv(prefix + "ENABLED")
	if id == "" && secret == "" && enabled == "" {
		return
	}
	if cfg.Auth.OAuth == nil {
		cfg.Auth.OAuth = make(map[string]OAuthProvider)
	}
	p := cfg.Auth.OAuth[provider]
	if id != "" {
		p.ClientID = id
	}
	if secret != "" {
		p.ClientSecret = secret
	}
	if enabled != "" {
		p.Enabled = enabled == "true" || enabled == "1"
	}
	cfg.Auth.OAuth[provider] = p
}

func applyFlags(cfg *Config, flags map[string]string) {
	if flags == nil {
		return
	}
	if v, ok := flags["database-url"]; ok && v != "" {
		cfg.Database.URL = v
	}
	if v, ok := flags["port"]; ok && v != "" {
		if port, err := strconv.Atoi(v); err == nil {
			cfg.Server.Port = port
		}
	}
	if v, ok := flags["host"]; ok && v != "" {
		cfg.Server.Host = v
	}
	if v, ok := flags["tls-domain"]; ok && v != "" {
		cfg.Server.TLSDomain = v
	}
}

// MaxFileSizeBytes returns the max file size in bytes, parsed from the config string.
// Supports "10MB", "5MB", "1GB", "500KB", etc. Defaults to 10MB if unparseable.
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

// validKeys is the complete set of dot-separated config keys.
var validKeys = map[string]bool{
	"server.host": true, "server.port": true, "server.site_url": true,
	"server.cors_allowed_origins": true,
	"server.body_limit":           true, "server.shutdown_timeout": true,
	"server.tls_enabled": true, "server.tls_domain": true,
	"server.tls_cert_dir": true, "server.tls_email": true,
	"database.url": true, "database.max_conns": true, "database.min_conns": true,
	"database.health_check_interval": true, "database.embedded_port": true,
	"database.embedded_data_dir": true, "database.migrations_dir": true,
	"admin.enabled": true, "admin.path": true, "admin.password": true, "admin.login_rate_limit": true,
	"auth.enabled": true, "auth.jwt_secret": true, "auth.token_duration": true,
	"auth.refresh_token_duration": true, "auth.rate_limit": true, "auth.min_password_length": true,
	"auth.oauth_redirect_url": true, "auth.magic_link_enabled": true, "auth.magic_link_duration": true,
	"auth.oauth_provider.enabled":                true,
	"auth.oauth_provider.access_token_duration":  true,
	"auth.oauth_provider.refresh_token_duration": true,
	"auth.oauth_provider.auth_code_duration":     true,
	"auth.sms_enabled":                           true, "auth.sms_provider": true, "auth.sms_code_length": true,
	"auth.sms_code_expiry": true, "auth.sms_max_attempts": true, "auth.sms_daily_limit": true,
	"auth.sms_allowed_countries": true,
	"auth.twilio_sid":            true, "auth.twilio_token": true, "auth.twilio_from": true,
	"auth.plivo_auth_id": true, "auth.plivo_auth_token": true, "auth.plivo_from": true,
	"auth.telnyx_api_key": true, "auth.telnyx_from": true,
	"auth.msg91_auth_key": true, "auth.msg91_template_id": true,
	"auth.aws_region":     true,
	"auth.vonage_api_key": true, "auth.vonage_api_secret": true, "auth.vonage_from": true,
	"auth.sms_webhook_url": true, "auth.sms_webhook_secret": true,
	"auth.sms_test_phone_numbers": true,
	"email.backend":               true, "email.from": true, "email.from_name": true,
	"storage.enabled": true, "storage.backend": true, "storage.local_path": true,
	"storage.max_file_size": true, "storage.s3_endpoint": true, "storage.s3_bucket": true,
	"storage.s3_region": true, "storage.s3_access_key": true, "storage.s3_secret_key": true,
	"storage.s3_use_ssl": true,
	"logging.level":      true, "logging.format": true,
	"jobs.enabled": true, "jobs.worker_concurrency": true, "jobs.poll_interval_ms": true,
	"jobs.lease_duration_s": true, "jobs.max_retries_default": true, "jobs.scheduler_enabled": true,
	"jobs.scheduler_tick_s": true,
}

// IsValidKey returns true if the dotted key is a recognized config key.
func IsValidKey(key string) bool {
	return validKeys[key]
}

// GetValue returns the value for a dotted config key (e.g. "server.port").
func GetValue(cfg *Config, key string) (any, error) {
	switch key {
	case "server.host":
		return cfg.Server.Host, nil
	case "server.port":
		return cfg.Server.Port, nil
	case "server.site_url":
		return cfg.Server.SiteURL, nil
	case "server.cors_allowed_origins":
		return strings.Join(cfg.Server.CORSAllowedOrigins, ","), nil
	case "server.body_limit":
		return cfg.Server.BodyLimit, nil
	case "server.shutdown_timeout":
		return cfg.Server.ShutdownTimeout, nil
	case "server.tls_enabled":
		return cfg.Server.TLSEnabled, nil
	case "server.tls_domain":
		return cfg.Server.TLSDomain, nil
	case "server.tls_cert_dir":
		return cfg.Server.TLSCertDir, nil
	case "server.tls_email":
		return cfg.Server.TLSEmail, nil
	case "database.url":
		return cfg.Database.URL, nil
	case "database.max_conns":
		return cfg.Database.MaxConns, nil
	case "database.min_conns":
		return cfg.Database.MinConns, nil
	case "database.health_check_interval":
		return cfg.Database.HealthCheckSecs, nil
	case "database.embedded_port":
		return cfg.Database.EmbeddedPort, nil
	case "database.embedded_data_dir":
		return cfg.Database.EmbeddedDataDir, nil
	case "database.migrations_dir":
		return cfg.Database.MigrationsDir, nil
	case "admin.enabled":
		return cfg.Admin.Enabled, nil
	case "admin.path":
		return cfg.Admin.Path, nil
	case "admin.password":
		return cfg.Admin.Password, nil
	case "admin.login_rate_limit":
		return cfg.Admin.LoginRateLimit, nil
	case "auth.enabled":
		return cfg.Auth.Enabled, nil
	case "auth.jwt_secret":
		return cfg.Auth.JWTSecret, nil
	case "auth.token_duration":
		return cfg.Auth.TokenDuration, nil
	case "auth.refresh_token_duration":
		return cfg.Auth.RefreshTokenDuration, nil
	case "auth.rate_limit":
		return cfg.Auth.RateLimit, nil
	case "auth.min_password_length":
		return cfg.Auth.MinPasswordLength, nil
	case "auth.oauth_redirect_url":
		return cfg.Auth.OAuthRedirectURL, nil
	case "auth.oauth_provider.enabled":
		return cfg.Auth.OAuthProviderMode.Enabled, nil
	case "auth.oauth_provider.access_token_duration":
		return cfg.Auth.OAuthProviderMode.AccessTokenDuration, nil
	case "auth.oauth_provider.refresh_token_duration":
		return cfg.Auth.OAuthProviderMode.RefreshTokenDuration, nil
	case "auth.oauth_provider.auth_code_duration":
		return cfg.Auth.OAuthProviderMode.AuthCodeDuration, nil
	case "auth.magic_link_enabled":
		return cfg.Auth.MagicLinkEnabled, nil
	case "auth.magic_link_duration":
		return cfg.Auth.MagicLinkDuration, nil
	case "auth.sms_enabled":
		return cfg.Auth.SMSEnabled, nil
	case "auth.sms_provider":
		return cfg.Auth.SMSProvider, nil
	case "auth.sms_code_length":
		return cfg.Auth.SMSCodeLength, nil
	case "auth.sms_code_expiry":
		return cfg.Auth.SMSCodeExpiry, nil
	case "auth.sms_max_attempts":
		return cfg.Auth.SMSMaxAttempts, nil
	case "auth.sms_daily_limit":
		return cfg.Auth.SMSDailyLimit, nil
	case "auth.sms_allowed_countries":
		return strings.Join(cfg.Auth.SMSAllowedCountries, ","), nil
	case "auth.twilio_sid":
		return cfg.Auth.TwilioSID, nil
	case "auth.twilio_token":
		return cfg.Auth.TwilioToken, nil
	case "auth.twilio_from":
		return cfg.Auth.TwilioFrom, nil
	case "auth.plivo_auth_id":
		return cfg.Auth.PlivoAuthID, nil
	case "auth.plivo_auth_token":
		return cfg.Auth.PlivoAuthToken, nil
	case "auth.plivo_from":
		return cfg.Auth.PlivoFrom, nil
	case "auth.telnyx_api_key":
		return cfg.Auth.TelnyxAPIKey, nil
	case "auth.telnyx_from":
		return cfg.Auth.TelnyxFrom, nil
	case "auth.msg91_auth_key":
		return cfg.Auth.MSG91AuthKey, nil
	case "auth.msg91_template_id":
		return cfg.Auth.MSG91TemplateID, nil
	case "auth.aws_region":
		return cfg.Auth.AWSRegion, nil
	case "auth.vonage_api_key":
		return cfg.Auth.VonageAPIKey, nil
	case "auth.vonage_api_secret":
		return cfg.Auth.VonageAPISecret, nil
	case "auth.vonage_from":
		return cfg.Auth.VonageFrom, nil
	case "auth.sms_webhook_url":
		return cfg.Auth.SMSWebhookURL, nil
	case "auth.sms_webhook_secret":
		return cfg.Auth.SMSWebhookSecret, nil
	case "auth.sms_test_phone_numbers":
		return cfg.Auth.SMSTestPhoneNumbers, nil
	case "email.backend":
		return cfg.Email.Backend, nil
	case "email.from":
		return cfg.Email.From, nil
	case "email.from_name":
		return cfg.Email.FromName, nil
	case "storage.enabled":
		return cfg.Storage.Enabled, nil
	case "storage.backend":
		return cfg.Storage.Backend, nil
	case "storage.local_path":
		return cfg.Storage.LocalPath, nil
	case "storage.max_file_size":
		return cfg.Storage.MaxFileSize, nil
	case "storage.s3_endpoint":
		return cfg.Storage.S3Endpoint, nil
	case "storage.s3_bucket":
		return cfg.Storage.S3Bucket, nil
	case "storage.s3_region":
		return cfg.Storage.S3Region, nil
	case "storage.s3_access_key":
		return cfg.Storage.S3AccessKey, nil
	case "storage.s3_secret_key":
		return cfg.Storage.S3SecretKey, nil
	case "storage.s3_use_ssl":
		return cfg.Storage.S3UseSSL, nil
	case "logging.level":
		return cfg.Logging.Level, nil
	case "logging.format":
		return cfg.Logging.Format, nil
	case "jobs.enabled":
		return cfg.Jobs.Enabled, nil
	case "jobs.worker_concurrency":
		return cfg.Jobs.WorkerConcurrency, nil
	case "jobs.poll_interval_ms":
		return cfg.Jobs.PollIntervalMs, nil
	case "jobs.lease_duration_s":
		return cfg.Jobs.LeaseDurationS, nil
	case "jobs.max_retries_default":
		return cfg.Jobs.MaxRetriesDefault, nil
	case "jobs.scheduler_enabled":
		return cfg.Jobs.SchedulerEnabled, nil
	case "jobs.scheduler_tick_s":
		return cfg.Jobs.SchedulerTickS, nil
	default:
		return nil, fmt.Errorf("unknown configuration key: %s", key)
	}
}

// SetValue reads the existing TOML file, updates a single key, and writes it back.
// Creates the file with just the key if it doesn't exist.
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

	// Split key into section.field.
	parts := strings.SplitN(key, ".", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid key format: %s (expected section.field)", key)
	}
	section, field := parts[0], parts[1]

	// Get or create section map.
	sectionMap, ok := data[section].(map[string]any)
	if !ok {
		sectionMap = make(map[string]any)
		data[section] = sectionMap
	}

	// Convert value to appropriate type.
	sectionMap[field] = coerceValue(key, value)

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
	// Boolean fields.
	switch key {
	case "admin.enabled", "auth.enabled", "auth.magic_link_enabled", "auth.sms_enabled",
		"storage.enabled", "storage.s3_use_ssl", "server.tls_enabled",
		"auth.oauth_provider.enabled", "jobs.enabled", "jobs.scheduler_enabled":
		return value == "true" || value == "1"
	}
	// Integer fields.
	switch key {
	case "server.port", "server.shutdown_timeout",
		"database.max_conns", "database.min_conns", "database.health_check_interval",
		"database.embedded_port",
		"admin.login_rate_limit",
		"auth.token_duration", "auth.refresh_token_duration", "auth.rate_limit",
		"auth.min_password_length", "auth.magic_link_duration",
		"auth.sms_code_length", "auth.sms_code_expiry", "auth.sms_max_attempts", "auth.sms_daily_limit",
		"auth.oauth_provider.access_token_duration", "auth.oauth_provider.refresh_token_duration",
		"auth.oauth_provider.auth_code_duration",
		"jobs.worker_concurrency", "jobs.poll_interval_ms", "jobs.lease_duration_s",
		"jobs.max_retries_default", "jobs.scheduler_tick_s":
		if n, err := strconv.Atoi(value); err == nil {
			return n
		}
	}
	return value
}

const defaultTOML = `# Allyourbase (AYB) Configuration
# Documentation: https://allyourbase.io/docs/config

[server]
# Address to listen on.
host = "0.0.0.0"
port = 8090

# Public URL for email action links (password reset, magic links, verification).
# Required for production. If unset, defaults to http://localhost:<port>.
# site_url = "https://myapp.example.com"

# CORS allowed origins. Use ["*"] to allow all.
cors_allowed_origins = ["*"]

# Maximum request body size.
body_limit = "1MB"

# Seconds to wait for in-flight requests during shutdown.
shutdown_timeout = 10

# Automatic HTTPS via Let's Encrypt (optional).
# Requires a public domain pointing at this machine, with ports 80 and 443 open.
# Set tls_domain to enable — AYB will obtain and auto-renew the certificate.
# tls_domain = "api.myapp.com"
# tls_email = "you@example.com"   # recommended for cert expiry notifications
# tls_cert_dir = ""               # certificate storage, default: ~/.ayb/certs

[database]
# PostgreSQL connection URL.
# Leave empty for embedded mode (AYB manages its own PostgreSQL).
# url = "postgresql://user:password@localhost:5432/mydb?sslmode=disable"

# Connection pool settings.
max_conns = 25
min_conns = 2

# Seconds between health check pings.
health_check_interval = 30

# Directory for user SQL migrations (applied by 'ayb migrate up').
migrations_dir = "./migrations"

# Embedded PostgreSQL settings (used when url is not set).
# Port for managed PostgreSQL.
# embedded_port = 15432
#
# Data directory for managed PostgreSQL (default: ~/.ayb/data).
# embedded_data_dir = ""

[admin]
# Enable the admin dashboard.
enabled = true

# URL path for the admin dashboard.
path = "/admin"

# Admin dashboard password. Set this to protect the admin UI.
# password = ""

# Max admin login attempts per minute per IP (default 20).
# Reduce for production, increase for local development.
# login_rate_limit = 20

[auth]
# Enable authentication. When true, API endpoints require a valid JWT.
enabled = false

# Secret key for signing JWTs. Must be at least 32 characters.
# Required when auth is enabled.
# jwt_secret = ""

# Access token duration in seconds (default: 15 minutes).
token_duration = 900

# Refresh token duration in seconds (default: 7 days).
refresh_token_duration = 604800

# Minimum password length for user registration and password reset.
# Default: 8 (NIST SP 800-63B recommended). Can be lowered to 1 for development.
# Values below 8 will trigger a startup warning.
min_password_length = 8

# URL to redirect to after OAuth login (tokens appended as hash fragment).
# oauth_redirect_url = "http://localhost:5173/oauth-callback"

# Magic link (passwordless) authentication.
# When enabled, users can request a login link via email — no password needed.
# magic_link_enabled = false
# magic_link_duration = 600

# SMS OTP authentication.
# When enabled, users can verify their phone number via a one-time code.
# sms_enabled = false
# sms_provider = "log"          # "log", "twilio", "plivo", "telnyx", "msg91", "sns", "vonage", "webhook"
# sms_code_length = 6           # 4-8 digits
# sms_code_expiry = 300         # seconds (60-600)
# sms_max_attempts = 3
# sms_daily_limit = 1000        # 0 = unlimited
# sms_allowed_countries = ["US", "CA"]

# Twilio credentials (required when sms_provider = "twilio").
# twilio_sid = ""
# twilio_token = ""
# twilio_from = ""

# Plivo credentials (required when sms_provider = "plivo").
# plivo_auth_id = ""
# plivo_auth_token = ""
# plivo_from = ""

# Telnyx credentials (required when sms_provider = "telnyx").
# telnyx_api_key = ""
# telnyx_from = ""

# MSG91 credentials (required when sms_provider = "msg91").
# msg91_auth_key = ""
# msg91_template_id = ""

# AWS SNS (required when sms_provider = "sns"). Credentials from env (AWS_ACCESS_KEY_ID, etc).
# aws_region = "us-east-1"

# Vonage credentials (required when sms_provider = "vonage").
# vonage_api_key = ""
# vonage_api_secret = ""
# vonage_from = ""

# Custom webhook (required when sms_provider = "webhook").
# sms_webhook_url = ""
# sms_webhook_secret = ""

# Test phone numbers — map of phone number to predetermined OTP code.
# Messages to these numbers skip the provider and use the given code.
# [auth.sms_test_phone_numbers]
# "+15550001234" = "000000"

# OAuth providers. Supported: google, github.
# [auth.oauth.google]
# enabled = false
# client_id = ""
# client_secret = ""

# [auth.oauth.github]
# enabled = false
# client_id = ""
# client_secret = ""

# OAuth 2.0 provider mode (AYB as authorization server).
# Disabled by default. Requires auth.enabled = true and auth.jwt_secret set.
# PKCE is always required (S256 only) and cannot be disabled.
[auth.oauth_provider]
enabled = false
access_token_duration = 3600
refresh_token_duration = 2592000
auth_code_duration = 600

[email]
# Email backend: "log" (default, prints to console), "smtp", or "webhook".
# In log mode, verification/reset links are printed to stdout — no setup needed.
backend = "log"

# Sender address and display name.
# from = "noreply@example.com"
from_name = "Allyourbase"

# SMTP settings (backend = "smtp").
# Provider presets — just paste your API key as the password:
#   Resend:  host = "smtp.resend.com", port = 465, tls = true
#   Brevo:   host = "smtp-relay.brevo.com", port = 587
#   AWS SES: host = "email-smtp.us-east-1.amazonaws.com", port = 465, tls = true
# [email.smtp]
# host = ""
# port = 587
# username = ""
# password = ""
# auth_method = "PLAIN"
# tls = false

# Webhook settings (backend = "webhook").
# AYB POSTs JSON {to, subject, html, text} to your URL.
# Signed with HMAC-SHA256 in X-AYB-Signature header if secret is set.
# [email.webhook]
# url = ""
# secret = ""
# timeout = 10

[storage]
# Enable file storage. When true, upload/serve/delete endpoints are available.
enabled = false

# Storage backend: "local" (filesystem) or "s3" (any S3-compatible object store).
backend = "local"

# Directory for local file storage (backend = "local").
local_path = "./ayb_storage"

# Maximum upload file size.
max_file_size = "10MB"

# S3-compatible object storage settings (backend = "s3").
# Works with Cloudflare R2, MinIO, DigitalOcean Spaces, AWS S3, Backblaze B2, and more.
# s3_endpoint = "s3.amazonaws.com"
# s3_bucket = "my-ayb-bucket"
# s3_region = "us-east-1"
# s3_access_key = ""
# s3_secret_key = ""
# s3_use_ssl = true

[logging]
# Log level: debug, info, warn, error.
level = "info"

# Log format: json or text.
format = "json"

[jobs]
# Enable the persistent background job queue/scheduler.
# Keep disabled for backward compatibility unless you want queue workers.
enabled = false

# Number of concurrent worker goroutines.
worker_concurrency = 4

# Worker poll interval (milliseconds).
poll_interval_ms = 1000

# Lease duration for claimed jobs (seconds).
lease_duration_s = 300

# Default max retries for jobs that do not specify max_attempts.
max_retries_default = 3

# Enable recurring schedule processing when jobs are enabled.
scheduler_enabled = true

# Scheduler scan/tick interval (seconds).
scheduler_tick_s = 15
`
