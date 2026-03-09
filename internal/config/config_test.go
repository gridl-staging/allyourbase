package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestDefault(t *testing.T) {
	cfg := Default()

	testutil.Equal(t, "0.0.0.0", cfg.Server.Host)
	testutil.Equal(t, 8090, cfg.Server.Port)
	testutil.Equal(t, "1MB", cfg.Server.BodyLimit)
	testutil.Equal(t, 10, cfg.Server.ShutdownTimeout)
	testutil.SliceLen(t, cfg.Server.CORSAllowedOrigins, 1)
	testutil.Equal(t, "*", cfg.Server.CORSAllowedOrigins[0])

	testutil.Equal(t, 25, cfg.Database.MaxConns)
	testutil.Equal(t, 2, cfg.Database.MinConns)
	testutil.Equal(t, 30, cfg.Database.HealthCheckSecs)
	testutil.Equal(t, 15432, cfg.Database.EmbeddedPort)
	testutil.Equal(t, "", cfg.Database.EmbeddedDataDir)

	testutil.Equal(t, true, cfg.Admin.Enabled)
	testutil.Equal(t, "/admin", cfg.Admin.Path)

	testutil.Equal(t, false, cfg.Auth.Enabled)
	testutil.Equal(t, "", cfg.Auth.JWTSecret)
	testutil.Equal(t, 900, cfg.Auth.TokenDuration)
	testutil.Equal(t, 604800, cfg.Auth.RefreshTokenDuration)
	testutil.Equal(t, 10, cfg.Auth.RateLimit)
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
}

func TestAddress(t *testing.T) {
	tests := []struct {
		name string
		host string
		port int
		want string
	}{
		{name: "default", host: "0.0.0.0", port: 8090, want: "0.0.0.0:8090"},
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
			name:   "debug log level",
			modify: func(c *Config) { c.Logging.Level = "debug" },
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
					"twitter": {Enabled: true, ClientID: "id", ClientSecret: "secret"},
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
					"google": {Enabled: true, ClientID: "id", ClientSecret: "secret"},
					"github": {Enabled: true, ClientID: "id2", ClientSecret: "secret2"},
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
	testutil.Equal(t, "0.0.0.0:8090", cfg.Address())
}

func TestLoadMissingFileUsesDefaults(t *testing.T) {
	// Point to a non-existent file — should silently use defaults.
	cfg, err := Load("/nonexistent/ayb.toml", nil)
	testutil.NoError(t, err)
	testutil.Equal(t, 8090, cfg.Server.Port)
	testutil.Equal(t, "0.0.0.0", cfg.Server.Host)
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
	t.Setenv("AYB_DATABASE_URL", "postgresql://envdb")
	t.Setenv("AYB_ADMIN_PASSWORD", "secret123")
	t.Setenv("AYB_LOG_LEVEL", "warn")
	t.Setenv("AYB_CORS_ORIGINS", "http://a.com,http://b.com")
	t.Setenv("AYB_AUTH_ENABLED", "true")
	t.Setenv("AYB_AUTH_JWT_SECRET", "this-is-a-secret-that-is-at-least-32-characters-long")

	cfg, err := Load("/nonexistent/ayb.toml", nil)
	testutil.NoError(t, err)

	testutil.Equal(t, "envhost", cfg.Server.Host)
	testutil.Equal(t, 9999, cfg.Server.Port)
	testutil.Equal(t, "postgresql://envdb", cfg.Database.URL)
	testutil.Equal(t, "secret123", cfg.Admin.Password)
	testutil.Equal(t, "warn", cfg.Logging.Level)
	testutil.SliceLen(t, cfg.Server.CORSAllowedOrigins, 2)
	testutil.Equal(t, "http://a.com", cfg.Server.CORSAllowedOrigins[0])
	testutil.Equal(t, "http://b.com", cfg.Server.CORSAllowedOrigins[1])
	testutil.Equal(t, true, cfg.Auth.Enabled)
	testutil.Equal(t, "this-is-a-secret-that-is-at-least-32-characters-long", cfg.Auth.JWTSecret)
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

func TestToTOML(t *testing.T) {
	cfg := Default()
	s, err := cfg.ToTOML()
	testutil.NoError(t, err)
	testutil.Contains(t, s, "host = '0.0.0.0'")
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
	testutil.Equal(t, "0.0.0.0", cfg.Server.Host)
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
	t.Setenv("AYB_AUTH_OAUTH_GITHUB_CLIENT_ID", "env-github-id")
	t.Setenv("AYB_AUTH_OAUTH_REDIRECT_URL", "http://myapp.com/callback")

	cfg := Default()
	err := applyEnv(cfg)
	testutil.NoError(t, err)

	testutil.Equal(t, "http://myapp.com/callback", cfg.Auth.OAuthRedirectURL)
	testutil.NotNil(t, cfg.Auth.OAuth)

	g := cfg.Auth.OAuth["google"]
	testutil.Equal(t, "env-google-id", g.ClientID)
	testutil.Equal(t, "env-google-secret", g.ClientSecret)
	testutil.True(t, g.Enabled, "google should be enabled")

	gh := cfg.Auth.OAuth["github"]
	testutil.Equal(t, "env-github-id", gh.ClientID)
	testutil.False(t, gh.Enabled, "github should not be enabled (no ENABLED env)")
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

// --- GetValue / SetValue / IsValidKey tests ---

func TestIsValidKey(t *testing.T) {
	tests := []struct {
		key  string
		want bool
	}{
		{"server.port", true},
		{"server.host", true},
		{"server.site_url", true},
		{"database.url", true},
		{"auth.enabled", true},
		{"auth.jwt_secret", true},
		{"auth.oauth_provider.enabled", true},
		{"auth.oauth_provider.access_token_duration", true},
		{"auth.oauth_provider.refresh_token_duration", true},
		{"auth.oauth_provider.auth_code_duration", true},
		{"auth.min_password_length", true},
		{"storage.s3_bucket", true},
		{"logging.level", true},
		{"logging.format", true},
		{"auth.magic_link_enabled", true},
		{"auth.magic_link_duration", true},
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

	tests := []struct {
		key     string
		want    any
		wantErr bool
	}{
		{"server.host", "0.0.0.0", false},
		{"server.port", 8090, false},
		{"server.site_url", "", false},
		{"database.max_conns", 25, false},
		{"admin.enabled", true, false},
		{"auth.enabled", false, false},
		{"auth.oauth_provider.enabled", false, false},
		{"auth.oauth_provider.access_token_duration", 3600, false},
		{"auth.oauth_provider.refresh_token_duration", 2592000, false},
		{"auth.oauth_provider.auth_code_duration", 600, false},
		{"logging.level", "info", false},
		{"storage.backend", "local", false},
		{"auth.magic_link_enabled", false, false},
		{"auth.magic_link_duration", 600, false},
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
		{"server.port", "notanumber", "notanumber"}, // falls through to string
	}
	for _, tt := range tests {
		t.Run(tt.key+"="+tt.value, func(t *testing.T) {
			got := coerceValue(tt.key, tt.value)
			testutil.Equal(t, tt.want, got)
		})
	}
}

// --- TLS config tests ---

func TestDefaultTLSFields(t *testing.T) {
	cfg := Default()
	testutil.Equal(t, cfg.Server.TLSEnabled, false)
	testutil.Equal(t, cfg.Server.TLSDomain, "")
	testutil.Equal(t, cfg.Server.TLSCertDir, "")
	testutil.Equal(t, cfg.Server.TLSEmail, "")
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
}

func TestGetValueTLS(t *testing.T) {
	cfg := Default()
	cfg.Server.TLSDomain = "api.example.com"
	cfg.Server.TLSEmail = "admin@example.com"
	cfg.Server.TLSCertDir = "/home/user/.ayb/certs"

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
