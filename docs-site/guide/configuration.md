# Configuration

AYB uses a layered configuration system: **defaults &rarr; ayb.toml &rarr; environment variables &rarr; CLI flags**.

## Config file

Create `ayb.toml` in the working directory:

```toml
[server]
host = "0.0.0.0"
port = 8090
cors_allowed_origins = ["*"]
body_limit = "1MB"
shutdown_timeout = 10

[database]
url = "postgresql://user:pass@localhost:5432/mydb?sslmode=disable"
max_conns = 25
min_conns = 2
health_check_interval = 30
migrations_dir = "./migrations"
# Embedded PostgreSQL (used when url is empty):
# embedded_port = 15432
# embedded_data_dir = ""

[admin]
enabled = true
path = "/admin"
# password = "your-admin-password"

[auth]
enabled = false
# jwt_secret = ""           # Required when enabled, min 32 chars
token_duration = 900         # 15 minutes
refresh_token_duration = 604800  # 7 days
# oauth_redirect_url = "http://localhost:5173/oauth-callback"

# [auth.oauth.google]
# enabled = true
# client_id = ""
# client_secret = ""

# [auth.oauth.github]
# enabled = true
# client_id = ""
# client_secret = ""

# OAuth 2.0 provider mode (AYB as authorization server).
# Disabled by default. Requires auth.enabled = true and auth.jwt_secret set.
# PKCE is always required (S256 only) and cannot be disabled.
[auth.oauth_provider]
enabled = false
access_token_duration = 3600     # 1 hour (seconds)
refresh_token_duration = 2592000 # 30 days (seconds)
auth_code_duration = 600         # 10 minutes (seconds)

[email]
backend = "log"              # "log", "smtp", or "webhook"
# from = "noreply@example.com"
from_name = "Allyourbase"

# [email.smtp]
# host = "smtp.resend.com"
# port = 465
# username = ""
# password = ""
# auth_method = "PLAIN"
# tls = true

# [email.webhook]
# url = "https://your-app.com/email-hook"
# secret = "hmac-signing-secret"
# timeout = 10

[storage]
enabled = false
backend = "local"            # "local" or "s3" (any S3-compatible object store)
local_path = "./ayb_storage"
max_file_size = "10MB"

# S3-compatible object storage (Cloudflare R2, MinIO, DigitalOcean Spaces, AWS S3):
# s3_endpoint = "s3.amazonaws.com"
# s3_bucket = "my-ayb-bucket"
# s3_region = "us-east-1"
# s3_access_key = ""
# s3_secret_key = ""
# s3_use_ssl = true

[jobs]
enabled = false              # default off, keeps legacy timer-based webhook pruner behavior
worker_concurrency = 4
poll_interval_ms = 1000
lease_duration_s = 300
max_retries_default = 3
scheduler_enabled = true
scheduler_tick_s = 15

[logging]
level = "info"               # debug, info, warn, error
format = "json"              # json or text
```

## Environment variables

Every config value can be overridden with `AYB_` prefixed environment variables:

| Variable | Config field |
|----------|-------------|
| `AYB_SERVER_HOST` | `server.host` |
| `AYB_SERVER_PORT` | `server.port` |
| `AYB_DATABASE_URL` | `database.url` |
| `AYB_DATABASE_EMBEDDED_PORT` | `database.embedded_port` |
| `AYB_DATABASE_EMBEDDED_DATA_DIR` | `database.embedded_data_dir` |
| `AYB_DATABASE_MIGRATIONS_DIR` | `database.migrations_dir` |
| `AYB_ADMIN_PASSWORD` | `admin.password` |
| `AYB_AUTH_ENABLED` | `auth.enabled` |
| `AYB_AUTH_JWT_SECRET` | `auth.jwt_secret` |
| `AYB_AUTH_REFRESH_TOKEN_DURATION` | `auth.refresh_token_duration` |
| `AYB_AUTH_OAUTH_REDIRECT_URL` | `auth.oauth_redirect_url` |
| `AYB_AUTH_OAUTH_GOOGLE_CLIENT_ID` | `auth.oauth.google.client_id` |
| `AYB_AUTH_OAUTH_GOOGLE_CLIENT_SECRET` | `auth.oauth.google.client_secret` |
| `AYB_AUTH_OAUTH_GOOGLE_ENABLED` | `auth.oauth.google.enabled` |
| `AYB_AUTH_OAUTH_GITHUB_CLIENT_ID` | `auth.oauth.github.client_id` |
| `AYB_AUTH_OAUTH_GITHUB_CLIENT_SECRET` | `auth.oauth.github.client_secret` |
| `AYB_AUTH_OAUTH_GITHUB_ENABLED` | `auth.oauth.github.enabled` |
| `AYB_AUTH_OAUTH_PROVIDER_ENABLED` | `auth.oauth_provider.enabled` |
| `AYB_AUTH_OAUTH_PROVIDER_ACCESS_TOKEN_DURATION` | `auth.oauth_provider.access_token_duration` |
| `AYB_AUTH_OAUTH_PROVIDER_REFRESH_TOKEN_DURATION` | `auth.oauth_provider.refresh_token_duration` |
| `AYB_AUTH_OAUTH_PROVIDER_AUTH_CODE_DURATION` | `auth.oauth_provider.auth_code_duration` |
| `AYB_EMAIL_BACKEND` | `email.backend` |
| `AYB_EMAIL_FROM` | `email.from` |
| `AYB_EMAIL_FROM_NAME` | `email.from_name` |
| `AYB_EMAIL_SMTP_HOST` | `email.smtp.host` |
| `AYB_EMAIL_SMTP_PORT` | `email.smtp.port` |
| `AYB_EMAIL_SMTP_USERNAME` | `email.smtp.username` |
| `AYB_EMAIL_SMTP_PASSWORD` | `email.smtp.password` |
| `AYB_EMAIL_SMTP_AUTH_METHOD` | `email.smtp.auth_method` |
| `AYB_EMAIL_SMTP_TLS` | `email.smtp.tls` |
| `AYB_EMAIL_WEBHOOK_URL` | `email.webhook.url` |
| `AYB_EMAIL_WEBHOOK_SECRET` | `email.webhook.secret` |
| `AYB_EMAIL_WEBHOOK_TIMEOUT` | `email.webhook.timeout` |
| `AYB_STORAGE_ENABLED` | `storage.enabled` |
| `AYB_STORAGE_BACKEND` | `storage.backend` |
| `AYB_STORAGE_LOCAL_PATH` | `storage.local_path` |
| `AYB_STORAGE_MAX_FILE_SIZE` | `storage.max_file_size` |
| `AYB_STORAGE_S3_ENDPOINT` | `storage.s3_endpoint` |
| `AYB_STORAGE_S3_BUCKET` | `storage.s3_bucket` |
| `AYB_STORAGE_S3_REGION` | `storage.s3_region` |
| `AYB_STORAGE_S3_ACCESS_KEY` | `storage.s3_access_key` |
| `AYB_STORAGE_S3_SECRET_KEY` | `storage.s3_secret_key` |
| `AYB_STORAGE_S3_USE_SSL` | `storage.s3_use_ssl` |
| `AYB_JOBS_ENABLED` | `jobs.enabled` |
| `AYB_JOBS_WORKER_CONCURRENCY` | `jobs.worker_concurrency` |
| `AYB_JOBS_POLL_INTERVAL_MS` | `jobs.poll_interval_ms` |
| `AYB_JOBS_LEASE_DURATION_S` | `jobs.lease_duration_s` |
| `AYB_JOBS_MAX_RETRIES_DEFAULT` | `jobs.max_retries_default` |
| `AYB_JOBS_SCHEDULER_ENABLED` | `jobs.scheduler_enabled` |
| `AYB_JOBS_SCHEDULER_TICK_S` | `jobs.scheduler_tick_s` |
| `AYB_CORS_ORIGINS` | `server.cors_allowed_origins` (comma-separated) |
| `AYB_LOG_LEVEL` | `logging.level` |

## Per-app API key scoping

Per-app API key scoping is configured through admin APIs/CLI/UI, not static server config files.

No server configuration is required to enable app-scoped keys:

- Create apps with `POST /api/admin/apps` or `ayb apps create`.
- Create app-scoped keys by setting `appId` (`POST /api/admin/api-keys` or `ayb apikeys create --app <id>`).
- Configure per-app rate limits by updating app records (`PUT /api/admin/apps/{id}`).

## Job queue and scheduler

Job queue runtime is opt-in and disabled by default:

- Set `jobs.enabled = true` to start workers and unlock admin jobs/schedules endpoints.
- Keep `jobs.enabled = false` to preserve legacy behavior (including timer-based webhook pruning).
- `jobs.scheduler_enabled` controls recurring schedule processing when jobs are enabled.

Validation ranges when `jobs.enabled = true`:

- `jobs.worker_concurrency`: `1`-`64`
- `jobs.poll_interval_ms`: `100`-`60000`
- `jobs.lease_duration_s`: `30`-`3600`
- `jobs.max_retries_default`: `0`-`100`
- `jobs.scheduler_tick_s`: `5`-`3600`

## CLI flags

```bash
ayb start --database-url URL --port 3000 --host 127.0.0.1
```

CLI flags override everything else.

## CLI commands

```
ayb start      [--database-url] [--port] [--host]   Start the server
ayb stop                                             Stop the server
ayb status                                           Show server status
ayb config     [get|set]                             Print/manage config
ayb migrate    [up|create|status]                    Run database migrations
ayb admin      [create|reset-password]               Admin utilities
ayb sql        "SELECT ..."                          Execute SQL
ayb schema                                           Inspect database schema
ayb query      <table> [--filter] [--sort]           Query records via REST
ayb version                                          Print version info
```

## Generate a default config

```bash
ayb config > ayb.toml
```

This prints the full default configuration with comments.

## Managing multiple projects

When working with multiple AYB projects on the same machine, you can isolate each project's data and configuration.

### Option 1: Separate directories (recommended)

Create a dedicated directory for each project with its own `ayb.toml`:

```bash
# Project 1
mkdir ~/my-blog && cd ~/my-blog
cat > ayb.toml <<EOF
[server]
port = 8090

[database]
embedded_data_dir = "./data"
EOF
ayb start

# Project 2 (different terminal)
mkdir ~/my-shop && cd ~/my-shop
cat > ayb.toml <<EOF
[server]
port = 8091

[database]
embedded_data_dir = "./data"
EOF
ayb start
```

Each project runs on a different port with its own managed PostgreSQL instance and data directory.

### Option 2: External PostgreSQL with separate databases

Point multiple AYB instances at the same PostgreSQL server but different databases:

```bash
# Project 1
ayb start --database-url postgresql://user:pass@localhost:5432/blog --port 8090

# Project 2
ayb start --database-url postgresql://user:pass@localhost:5432/shop --port 8091
```

### Option 3: Environment-specific configs

Use different config files for dev, staging, and production:

```bash
# Development
ayb start --config ayb.dev.toml

# Staging
ayb start --config ayb.staging.toml

# Production
ayb start --config ayb.prod.toml
```
