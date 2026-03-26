package config

// LogDrainConfig configures an external log drain destination.
type LogDrainConfig struct {
	ID                string            `toml:"id" json:"id"`
	Type              string            `toml:"type" json:"type"`
	URL               string            `toml:"url" json:"url"`
	Headers           map[string]string `toml:"headers" json:"headers"`
	BatchSize         int               `toml:"batch_size" json:"batch_size"`
	FlushIntervalSecs int               `toml:"flush_interval_seconds" json:"flush_interval_seconds"`
	Enabled           *bool             `toml:"enabled" json:"enabled"`
}

type LoggingConfig struct {
	Level                       string           `toml:"level"`
	Format                      string           `toml:"format"`
	RequestLogEnabled           bool             `toml:"request_log_enabled"`
	RequestLogRetentionDays     int              `toml:"request_log_retention_days"`
	RequestLogBatchSize         int              `toml:"request_log_batch_size"`
	RequestLogFlushIntervalSecs int              `toml:"request_log_flush_interval_seconds"`
	RequestLogQueueSize         int              `toml:"request_log_queue_size"`
	Drains                      []LogDrainConfig `toml:"drains"`
}

type MetricsConfig struct {
	Enabled   bool   `toml:"enabled"`
	Path      string `toml:"path"`
	AuthToken string `toml:"auth_token"`
}

// TelemetryConfig configures OpenTelemetry distributed tracing.
type TelemetryConfig struct {
	Enabled      bool    `toml:"enabled"`
	OTLPEndpoint string  `toml:"otlp_endpoint"` // tracing disabled when empty
	ServiceName  string  `toml:"service_name"`
	SampleRate   float64 `toml:"sample_rate"` // 0.0–1.0
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

type PushConfig struct {
	Enabled bool           `toml:"enabled"` // default false
	FCM     PushFCMConfig  `toml:"fcm"`
	APNS    PushAPNSConfig `toml:"apns"`
}

type PushFCMConfig struct {
	CredentialsFile string `toml:"credentials_file"` // path to Firebase service account JSON
}

type PushAPNSConfig struct {
	KeyFile     string `toml:"key_file"` // path to .p8 private key file
	TeamID      string `toml:"team_id"`
	KeyID       string `toml:"key_id"`
	BundleID    string `toml:"bundle_id"`
	Environment string `toml:"environment"` // "production" or "sandbox" (default: "production")
}

// AuditConfig controls mutation audit logging.
type AuditConfig struct {
	Enabled       bool     `toml:"enabled"`        // master switch (default false)
	Tables        []string `toml:"tables"`         // opt-in table list (when AllTables is false)
	AllTables     bool     `toml:"all_tables"`     // log mutations on every user table
	RetentionDays int      `toml:"retention_days"` // auto-purge entries older than N days (default 90)
}

// BackupConfig configures automatic database backups to S3.
type BackupConfig struct {
	Enabled        bool       `toml:"enabled"`
	Bucket         string     `toml:"bucket"`
	Region         string     `toml:"region"`
	Prefix         string     `toml:"prefix"`
	Schedule       string     `toml:"schedule"`
	RetentionCount int        `toml:"retention_count"`
	RetentionDays  int        `toml:"retention_days"`
	Encryption     string     `toml:"encryption"` // "" | "AES256" | "aws:kms"
	Endpoint       string     `toml:"endpoint"`
	AccessKey      string     `toml:"access_key"`
	SecretKey      string     `toml:"secret_key"`
	UseSSL         bool       `toml:"use_ssl"`
	PITR           PITRConfig `toml:"pitr"`
}

// PITRConfig configures WAL archiving and point-in-time recovery for database backups. Nested under [backup.pitr] in the TOML configuration file.
type PITRConfig struct {
	Enabled                  bool   `toml:"enabled"`
	ArchiveBucket            string `toml:"archive_bucket"`
	ArchivePrefix            string `toml:"archive_prefix"`
	RetentionSchedule        string `toml:"retention_schedule"`
	WALRetentionDays         int    `toml:"wal_retention_days"`
	BaseBackupRetentionDays  int    `toml:"base_backup_retention_days"`
	ComplianceSnapshotMonths int    `toml:"compliance_snapshot_months"`
	EnvironmentClass         string `toml:"environment_class"`
	KMSKeyID                 string `toml:"kms_key_id"`
	RPOMinutes               int    `toml:"rpo_minutes"`
	StorageBudgetBytes       int64  `toml:"storage_budget_bytes"`
	ShadowMode               bool   `toml:"shadow_mode"`
	BaseBackupSchedule       string `toml:"base_backup_schedule"`
	VerifySchedule           string `toml:"verify_schedule"`
}

// GraphQLConfig configures the GraphQL API endpoint.
type GraphQLConfig struct {
	Enabled       bool   `toml:"enabled"`
	MaxDepth      int    `toml:"max_depth"`
	MaxComplexity int    `toml:"max_complexity"`
	Introspection string `toml:"introspection"` // "" (admin-gated), "open", "disabled"
}

// RealtimeConfig configures realtime WebSocket and SSE behavior.
type RealtimeConfig struct {
	MaxConnectionsPerUser       int `toml:"max_connections_per_user"`
	HeartbeatIntervalSeconds    int `toml:"heartbeat_interval_seconds"`
	BroadcastRateLimitPerSecond int `toml:"broadcast_rate_limit_per_second"`
	BroadcastMaxMessageBytes    int `toml:"broadcast_max_message_bytes"`
	PresenceLeaveTimeoutSeconds int `toml:"presence_leave_timeout_seconds"`
}

// StatusConfig controls status probing and public status endpoint behavior.
type StatusConfig struct {
	Enabled               bool `toml:"enabled"`
	CheckIntervalSeconds  int  `toml:"check_interval_seconds"`
	HistorySize           int  `toml:"history_size"`
	PublicEndpointEnabled bool `toml:"public_endpoint_enabled"`
}
