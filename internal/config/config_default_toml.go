package config

const defaultTOML = `# Allyourbase (AYB) Configuration
# Documentation: https://allyourbase.io/docs/config

[server]
# Address to listen on.
host = "127.0.0.1"
port = 8090

# Public URL for email action links (password reset, magic links, verification).
# Required for production. If unset, defaults to http://localhost:<port>.
# site_url = "https://myapp.example.com"

# CORS allowed origins. Use ["*"] to allow all.
cors_allowed_origins = ["*"]

# Optional API allowlist for /api routes. Empty means allow all.
# Supports IPv4, IPv6, and CIDR notation.
# Example: ["203.0.113.10", "198.51.100.0/24"]
# allowed_ips = []

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
# tls_staging = false             # use Let's Encrypt staging CA for testing

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

# Optional admin allowlist for /api/admin routes. Empty means allow all.
# Supports IPv4, IPv6, and CIDR notation.
# allowed_ips = []

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

# Anonymous sign-in attempts per hour per IP (default: 30).
anonymous_rate_limit = 30

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

# Email MFA step-up verification.
# When enabled, users can enroll email OTP as an MFA method.
# email_mfa_enabled = false

# Anonymous authentication.
# When enabled, users can create anonymous sessions that can later be linked to an email account.
# anonymous_auth_enabled = false

# TOTP MFA (authenticator app).
# When enabled, users can enroll TOTP as an MFA method using apps like Google Authenticator.
# totp_enabled = false
# Optional explicit 32-byte encryption key (hex or base64). If unset, AYB derives one from auth.jwt_secret.
# encryption_key = ""

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

# OAuth providers. Supported: google, github, microsoft, apple.
# [auth.oauth.google]
# enabled = false
# client_id = ""
# client_secret = ""
# store_provider_tokens = false

# [auth.oauth.github]
# enabled = false
# client_id = ""
# client_secret = ""
# store_provider_tokens = false

# [auth.oauth.microsoft]
# enabled = false
# client_id = ""
# client_secret = ""
# store_provider_tokens = false
# tenant_id = "common" # optional; defaults to common when omitted

# [auth.oauth.apple]
# enabled = false
# client_id = ""       # Apple Services ID
# store_provider_tokens = false
# team_id = ""         # Apple Developer Team ID
# key_id = ""          # Apple Sign-In private key ID
# private_key = ""     # PEM-encoded ES256 private key

# Custom OIDC providers (Keycloak, Auth0, Okta, self-hosted IdPs, etc.).
# Provider key is the login slug (e.g. "keycloak" => /api/auth/oauth/keycloak).
# [auth.oidc.keycloak]
# enabled = false
# issuer_url = "https://idp.example.com/realms/main"
# client_id = ""
# client_secret = ""
# scopes = ["openid", "profile", "email"]
# display_name = "Keycloak" # optional label shown in UI

# SAML providers.
# Use either idp_metadata_url (recommended) or idp_metadata_xml.
# [[auth.saml_providers]]
# enabled = false
# name = "okta"
# entity_id = "https://ayb.example.com/api/auth/saml/okta/metadata"
# idp_metadata_url = "https://idp.example.com/metadata"
# idp_metadata_xml = ""
# sp_cert_file = ""
# sp_key_file = ""
# [auth.saml_providers.attribute_mapping]
# email = "email"
# name = "name"
# groups = "groups"

# OAuth 2.0 provider mode (AYB as authorization server).
# Disabled by default. Requires auth.enabled = true and auth.jwt_secret set.
# PKCE is always required (S256 only) and cannot be disabled.
[auth.oauth_provider]
enabled = false
access_token_duration = 3600
refresh_token_duration = 2592000
auth_code_duration = 600

[billing]
# Billing provider plugin. Empty/omitted disables billing lifecycle.
provider = ""

# Stripe credentials and plan price IDs (required only when provider = "stripe").
# stripe_secret_key = "sk_live_..."
# stripe_webhook_secret = "whsec_..."
# stripe_starter_price_id = "price_..."
# stripe_pro_price_id = "price_..."
# stripe_enterprise_price_id = "price_..."

# Optional interval (seconds) for syncing usage counters back to billing.
usage_sync_interval_seconds = 3600

[vault]
# Optional vault master key. If unset, AYB checks AYB_VAULT_MASTER_KEY
# and then auto-generates/persists one at ~/.ayb/vault-key.
# master_key = ""

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

# Optional CDN base URL for public object URLs (for API responses only).
# Leave empty to use origin URLs.
# cdn_url = "https://cdn.example.com"

# Optional CDN purge provider settings.
# Keep empty to use URL rewrite only (no purge calls).
# [storage.cdn]
# provider = "" # "", "cloudflare", "cloudfront", "webhook"
#
# [storage.cdn.cloudflare]
# zone_id = ""
# api_token = ""
#
# [storage.cdn.cloudfront]
# distribution_id = ""
#
# [storage.cdn.webhook]
# endpoint = ""
# signing_secret = ""

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

[edge_functions]
# Maximum concurrent function executions.
pool_size = 12

# Admission cap for invocations before they enter the VM pool. When exceeded,
# requests are rejected quickly (HTTP 429) instead of blocking indefinitely.
max_concurrent_invocations = 50

# Default timeout for invocations and new deployments (milliseconds).
default_timeout_ms = 5000

# Max request body size accepted by /functions/v1 endpoints (bytes).
max_request_body_bytes = 1048576

# Best-effort per-invocation memory guard (MB). Used to scale call stack and
# stdout capture limits for Goja runtime safety valves.
memory_limit_mb = 128

# Maximum number of compiled edge programs kept in the in-memory LRU cache.
code_cache_size = 256

# Optional outbound fetch() domain allowlist.
fetch_domain_allowlist = []

[logging]
# Log level: debug, info, warn, error.
level = "info"

# Log format: json or text.
format = "json"

# External log drains. Each [[logging.drains]] block configures one destination.
# type = "http" | "datadog" | "loki"
# [[logging.drains]]
# id = "my-drift-queue"
# type = "http"
# url = "https://logs.example.com/ingest"
# enabled = true
# batch_size = 100
# flush_interval_seconds = 5
# [logging.drains.headers]
# Authorization = "Bearer token"

[metrics]
# Expose Prometheus metrics endpoint.
enabled = true

# HTTP path for metrics scraping.
path = "/metrics"

# Optional bearer token required for metrics access.
# auth_token = ""

[realtime]
# Maximum concurrent realtime (SSE + WS) connections per user key.
max_connections_per_user = 100

# WebSocket heartbeat ping interval in seconds.
heartbeat_interval_seconds = 25

# Broadcast relay rate limit in messages per second per connection.
broadcast_rate_limit_per_second = 100

# Maximum broadcast payload size in bytes.
broadcast_max_message_bytes = 262144

# Presence leave timeout in seconds before deferred cleanup.
presence_leave_timeout_seconds = 10

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

[audit]
# Audit logging for create/update/delete API mutations.
enabled = false

# Optional explicit table allowlist for audit capture when all_tables is false.
# tables = []

# Capture mutations on every table when true.
all_tables = false

# Remove audit entries older than this many days automatically.
retention_days = 90

[dashboard_ai]
# Enable dashboard AI assistant orchestration endpoints under /api/admin/ai/assistant*.
# Provider and model settings still come from the [ai] section.
enabled = false

# Per-actor assistant request limit (supports N/min or N/hour).
rate_limit = "20/min"
`
