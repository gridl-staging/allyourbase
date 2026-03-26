// Package config Config types and functions for loading, validating, and managing AYB configuration from TOML files, environment variables, and CLI flags with comprehensive defaults and utilities.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// Config is the top-level AYB configuration.
// Config is the top-level AYB configuration struct containing all settings for the server, database, authentication, billing, logging, storage, edge functions, and other subsystems.
type Config struct {
	Server           ServerConfig            `toml:"server"`
	Database         DatabaseConfig          `toml:"database"`
	ManagedPG        ManagedPGConfig         `toml:"managed_pg"`
	Admin            AdminConfig             `toml:"admin"`
	Auth             AuthConfig              `toml:"auth"`
	Billing          BillingConfig           `toml:"billing"`
	Support          SupportConfig           `toml:"support"`
	RateLimit        RateLimitConfig         `toml:"rate_limit"`
	API              APIConfig               `toml:"api"`
	Vault            VaultConfig             `toml:"vault"`
	Email            EmailConfig             `toml:"email"`
	Storage          StorageConfig           `toml:"storage"`
	EdgeFunctions    EdgeFuncConfig          `toml:"edge_functions"`
	Logging          LoggingConfig           `toml:"logging"`
	Metrics          MetricsConfig           `toml:"metrics"`
	Telemetry        TelemetryConfig         `toml:"telemetry"`
	Jobs             JobsConfig              `toml:"jobs"`
	Status           StatusConfig            `toml:"status"`
	Push             PushConfig              `toml:"push"`
	Audit            AuditConfig             `toml:"audit"`
	AI               AIConfig                `toml:"ai"`
	DashboardAI      DashboardAIConfig       `toml:"dashboard_ai"`
	Backup           BackupConfig            `toml:"backup"`
	GraphQL          GraphQLConfig           `toml:"graphql"`
	Realtime         RealtimeConfig          `toml:"realtime"`
	EncryptedColumns []EncryptedColumnConfig `toml:"encrypted_columns"`
}

// Default returns a Config with all defaults applied.
func Default() *Config {
	return &Config{
		Server: ServerConfig{
			Host:               "127.0.0.1",
			Port:               8090,
			CORSAllowedOrigins: []string{"*"},
			AllowedIPs:         []string{},
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
			AllowedIPs:     []string{},
		},
		Auth: AuthConfig{
			TokenDuration:        900,    // 15 minutes
			RefreshTokenDuration: 604800, // 7 days
			RateLimit:            10,     // requests per minute per IP
			AnonymousRateLimit:   30,     // anonymous sign-ins per hour per IP
			RateLimitAuth:        "10/min",
			MinPasswordLength:    8,   // NIST SP 800-63B recommended minimum
			MagicLinkDuration:    600, // 10 minutes
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
		Billing: BillingConfig{
			Provider:              "",
			UsageSyncIntervalSecs: 3600,
		},
		Support: SupportConfig{
			Enabled:            false,
			InboundEmailDomain: "",
			WebhookSecret:      "",
		},
		RateLimit: RateLimitConfig{
			API:          "100/min",
			APIAnonymous: "30/min",
		},
		API: APIConfig{
			ImportMaxSizeMB:  50,
			ImportMaxRows:    100000,
			ExportMaxRows:    1000000,
			AggregateEnabled: true,
		},
		Vault: VaultConfig{},
		Email: EmailConfig{
			Backend:  "log",
			FromName: "Allyourbase",
		},
		Storage: StorageConfig{
			Backend:        "local",
			LocalPath:      "./ayb_storage",
			MaxFileSize:    "10MB",
			DefaultQuotaMB: 100,
			S3Region:       "us-east-1",
			S3UseSSL:       true,
		},
		EdgeFunctions: EdgeFuncConfig{
			PoolSize:                 12,
			DefaultTimeoutMs:         5000,
			MaxRequestBodyBytes:      1 << 20,
			FetchDomainAllowlist:     []string{},
			MemoryLimitMB:            128,
			MaxConcurrentInvocations: 50,
			CodeCacheSize:            256,
		},
		Logging: LoggingConfig{
			Level:                       "info",
			Format:                      "json",
			RequestLogEnabled:           true,
			RequestLogRetentionDays:     7,
			RequestLogBatchSize:         100,
			RequestLogFlushIntervalSecs: 5,
			RequestLogQueueSize:         10000,
		},
		Metrics: MetricsConfig{
			Enabled: true,
			Path:    "/metrics",
		},
		Telemetry: TelemetryConfig{
			Enabled:     true,
			ServiceName: "ayb",
			SampleRate:  1.0,
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
		Status: StatusConfig{
			Enabled:               false,
			CheckIntervalSeconds:  30,
			HistorySize:           1000,
			PublicEndpointEnabled: true,
		},
		Push: PushConfig{
			Enabled: false,
			APNS: PushAPNSConfig{
				Environment: "production",
			},
		},
		ManagedPG: ManagedPGConfig{
			Port:                   15432,
			PGVersion:              "16",
			Extensions:             []string{"pgvector", "pg_trgm", "pg_cron"},
			SharedPreloadLibraries: []string{"pg_stat_statements"},
		},
		Audit: AuditConfig{
			Enabled:       false,
			RetentionDays: 90,
		},
		AI: AIConfig{
			DefaultProvider: "openai",
			TimeoutSecs:     30,
			MaxRetries:      2,
			Breaker: AIBreakerConfig{
				FailureThreshold:   5,
				OpenSeconds:        30,
				HalfOpenProbeLimit: 1,
			},
			EmbeddingDimensions: map[string]int{},
			Providers:           map[string]ProviderConfig{},
		},
		DashboardAI: DashboardAIConfig{
			Enabled:   false,
			RateLimit: "20/min",
		},
		Backup: BackupConfig{
			Enabled:        false,
			Region:         "us-east-1",
			Prefix:         "backups",
			Schedule:       "0 2 * * *",
			RetentionCount: 7,
			RetentionDays:  30,
			Encryption:     "AES256",
			UseSSL:         true,
			PITR: PITRConfig{
				Enabled:                  false,
				WALRetentionDays:         14,
				BaseBackupRetentionDays:  35,
				ComplianceSnapshotMonths: 12,
				RPOMinutes:               5,
				ShadowMode:               true,
				RetentionSchedule:        "0 4 * * *",
				StorageBudgetBytes:       0,
				VerifySchedule:           "0 */6 * * *",
				BaseBackupSchedule:       "0 3 * * *",
			},
		},
		Realtime: RealtimeConfig{
			MaxConnectionsPerUser:       100,
			HeartbeatIntervalSeconds:    25,
			BroadcastRateLimitPerSecond: 100,
			BroadcastMaxMessageBytes:    262144,
			PresenceLeaveTimeoutSeconds: 10,
		},
		GraphQL: GraphQLConfig{
			MaxDepth:      0,
			MaxComplexity: 0,
			Introspection: "",
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

// ParseTOML parses raw TOML bytes into a validated Config.
func ParseTOML(data []byte) (*Config, error) {
	cfg := Default()
	if err := toml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// Validate checks the configuration for invalid values.

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
